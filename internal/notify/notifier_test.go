package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// notifications builds an enabled Notifications section with the given
// cooldown and recovery notices on. Tests that need quiet hours or other
// overrides mutate the returned value before calling newHarness.
func notifications(t *testing.T, cooldownMinutes int) settings.Notifications {
	t.Helper()
	return settings.Notifications{
		Enabled:         true,
		CooldownMinutes: cooldownMinutes,
		NotifyRecovery:  true,
	}
}

// newHarness wires a Notifier against an httptest webhook sink and a real
// temp-file store, with a clock the test drives by hand.
func newHarness(t *testing.T, n settings.Notifications) (*Notifier, *store.Store, *[]Message, *time.Time) {
	t.Helper()

	var mu sync.Mutex
	got := []Message{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m Message
		json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	n.Channels = []settings.Channel{{ID: "c1", Type: "webhook", Enabled: true, URL: srv.URL}}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	clock := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	nfr := New(Config{
		Store:    db,
		Client:   srv.Client(),
		QueueCap: 8,
		Now:      func() time.Time { return clock },
	})
	nfr.Configure(n, time.UTC)

	return nfr, db, &got, &clock
}

func seedTarget(t *testing.T, db *store.Store, thresholds string) int64 {
	t.Helper()
	id, err := db.CreateTarget(context.Background(), &store.Target{
		Name: "Home", Engine: "librespeed", Enabled: true, Lane: "default",
		Options: json.RawMessage(`{}`), Thresholds: json.RawMessage(thresholds),
	})
	if err != nil {
		t.Fatalf("seed target: %v", err)
	}
	return id
}

func result(targetID int64, downloadBps float64) *store.Result {
	return &store.Result{
		ID: 1, TargetID: &targetID, Status: "ok", DownloadBps: downloadBps,
		UploadBps: downloadBps, PingMs: 10, JitterMs: 1, PacketLossPct: 0,
	}
}

// TestConfigureAppliesLive is a regression guard for Configure being
// re-read per result (via snapshot) rather than captured once at Start:
// toggling Enabled between two process calls on the same Notifier must
// change behaviour immediately, with no restart. notifications(t, 60)
// alone carries no channels, so both configs here attach one explicitly —
// the point under test is Enabled taking effect live, not delivery itself.
func TestConfigureAppliesLive(t *testing.T) {
	n, db, _, _ := newHarness(t, notifications(t, 60))
	id := seedTarget(t, db, `{"download_mbps_min":100}`)

	var mu sync.Mutex
	got := []Message{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m Message
		json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	ch := []settings.Channel{{ID: "c2", Type: "webhook", Enabled: true, URL: srv.URL}}

	off := notifications(t, 60)
	off.Enabled = false
	off.Channels = ch
	n.Configure(off, time.UTC)
	n.process(context.Background(), result(id, 1e6))
	mu.Lock()
	gotLen := len(got)
	mu.Unlock()
	if gotLen != 0 {
		t.Fatalf("Configure(disabled) did not take effect: %+v", got)
	}

	on := notifications(t, 60)
	on.Channels = ch
	n.Configure(on, time.UTC)
	n.process(context.Background(), result(id, 1e6))
	mu.Lock()
	gotLen = len(got)
	mu.Unlock()
	if gotLen != 1 {
		t.Fatalf("Configure(enabled) did not take effect: %+v", got)
	}
}

func TestFiresOnceThenRespectsCooldown(t *testing.T) {
	n, db, got, clock := newHarness(t, notifications(t, 60)) // cooldown 60m
	id := seedTarget(t, db, `{"download_mbps_min":100}`)
	ctx := context.Background()

	n.process(ctx, result(id, 50e6)) // 50 Mbps
	if len(*got) != 1 || (*got)[0].Kind != "alert" {
		t.Fatalf("first breach = %+v, want one alert", *got)
	}
	*clock = clock.Add(30 * time.Minute)
	n.process(ctx, result(id, 40e6))
	if len(*got) != 1 {
		t.Fatalf("re-fired inside the cooldown: %+v", *got)
	}
	*clock = clock.Add(31 * time.Minute)
	n.process(ctx, result(id, 40e6))
	if len(*got) != 2 {
		t.Fatalf("did not re-fire after the cooldown: %+v", *got)
	}
	st, ok, _ := db.GetNotifyState(ctx, id, "download")
	if !ok || !st.Firing {
		t.Fatalf("state = %+v ok %v, want firing", st, ok)
	}
}

func TestRecoveryNotificationAndStateClear(t *testing.T) {
	n, db, got, clock := newHarness(t, notifications(t, 60))
	id := seedTarget(t, db, `{"download_mbps_min":100}`)
	ctx := context.Background()

	n.process(ctx, result(id, 50e6))
	*clock = clock.Add(time.Minute) // recovery ignores the cooldown
	n.process(ctx, result(id, 150e6))
	if len(*got) != 2 || (*got)[1].Kind != "recovery" {
		t.Fatalf("messages = %+v, want alert then recovery", *got)
	}
	if st, ok, _ := db.GetNotifyState(ctx, id, "download"); ok && st.Firing {
		t.Fatalf("state still firing after recovery: %+v", st)
	}
	n.process(ctx, result(id, 150e6))
	if len(*got) != 2 {
		t.Fatalf("a second healthy result re-sent a recovery: %+v", *got)
	}
}

func TestPerTargetThresholdsOverrideDefaults(t *testing.T) {
	cfg := notifications(t, 60)
	min := 500.0
	cfg.DefaultThresholds = settings.Thresholds{DownloadMbpsMin: &min}
	n, db, got, _ := newHarness(t, cfg)
	loose := seedTarget(t, db, `{"download_mbps_min":10}`)
	inherit := seedTarget(t, db, `{}`)

	n.process(context.Background(), result(loose, 100e6))   // 100 > its own 10
	n.process(context.Background(), result(inherit, 100e6)) // 100 < inherited 500
	if len(*got) != 1 || (*got)[0].TargetID != inherit {
		t.Fatalf("messages = %+v, want only the inheriting target to fire", *got)
	}
}

func TestQuietHoursSuppressDeliveryButKeepState(t *testing.T) {
	cfg := notifications(t, 60)
	cfg.QuietHoursStart, cfg.QuietHoursEnd = "22:00", "07:00"
	n, db, got, clock := newHarness(t, cfg)
	id := seedTarget(t, db, `{"download_mbps_min":100}`)
	*clock = time.Date(2026, 9, 14, 23, 30, 0, 0, time.UTC)

	n.process(context.Background(), result(id, 50e6))
	if len(*got) != 0 {
		t.Fatalf("delivered during quiet hours: %+v", *got)
	}
	st, ok, _ := db.GetNotifyState(context.Background(), id, "download")
	if !ok || !st.Firing {
		t.Fatalf("quiet hours must still record firing state, got %+v ok %v", st, ok)
	}
	if n.Stats().Suppressed != 1 {
		t.Fatalf("suppressed = %d, want 1", n.Stats().Suppressed)
	}
}

func TestQuietNowWrapsMidnight(t *testing.T) {
	loc := time.UTC
	for _, tc := range []struct {
		hhmm string
		want bool
	}{{"23:30", true}, {"06:59", true}, {"07:00", false}, {"12:00", false}, {"22:00", true}} {
		at, _ := time.ParseInLocation("15:04", tc.hhmm, loc)
		if got := quietNow(at, "22:00", "07:00", loc); got != tc.want {
			t.Errorf("quietNow(%s) = %v, want %v", tc.hhmm, got, tc.want)
		}
	}
	at, _ := time.ParseInLocation("15:04", "03:00", loc)
	if quietNow(at, "", "", loc) {
		t.Error("empty quiet hours must never suppress")
	}
}

func TestFailedResultFiresAndRecovers(t *testing.T) {
	n, db, got, _ := newHarness(t, notifications(t, 60))
	id := seedTarget(t, db, `{"notify_on_failure":true,"download_mbps_min":100}`)
	failed := result(id, 0)
	failed.Status, failed.Error = "failed", "speedtest: exit status 1"

	n.process(context.Background(), failed)
	if len(*got) != 1 || (*got)[0].Metric != MetricFailure {
		t.Fatalf("messages = %+v, want a single failure alert (no metric alerts from zero values)", *got)
	}
	n.process(context.Background(), result(id, 150e6))
	if len(*got) != 2 || (*got)[1].Kind != "recovery" {
		t.Fatalf("messages = %+v, want a failure recovery", *got)
	}
}

func TestOnResultNeverBlocksAndDropsOldest(t *testing.T) {
	n, db, _, _ := newHarness(t, notifications(t, 60)) // Start() not called
	id := seedTarget(t, db, `{"download_mbps_min":100}`)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			n.OnResult(context.Background(), result(id, 50e6), Meta{})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OnResult blocked with no worker draining the queue")
	}
	if n.Stats().Dropped == 0 {
		t.Fatal("dropped counter did not move despite an overflowing queue")
	}
}

func TestDisabledNotifierDoesNothing(t *testing.T) {
	cfg := notifications(t, 60)
	cfg.Enabled = false
	n, db, got, _ := newHarness(t, cfg)
	id := seedTarget(t, db, `{"download_mbps_min":100}`)
	n.process(context.Background(), result(id, 1e6))
	if len(*got) != 0 {
		t.Fatalf("disabled notifier sent %+v", *got)
	}
	if _, ok, _ := db.GetNotifyState(context.Background(), id, "download"); ok {
		t.Fatal("disabled notifier wrote notification state")
	}
}

func TestOneFailingChannelDoesNotStopTheOthers(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()

	var mu sync.Mutex
	var goodCount int
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		goodCount++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer good.Close()

	n, db, _, _ := newHarness(t, notifications(t, 60))
	cfg := notifications(t, 60)
	cfg.Channels = []settings.Channel{
		{ID: "bad", Type: "webhook", Enabled: true, URL: failing.URL},
		{ID: "good", Type: "webhook", Enabled: true, URL: good.URL},
	}
	n.Configure(cfg, time.UTC)

	id := seedTarget(t, db, `{"download_mbps_min":100}`)
	n.process(context.Background(), result(id, 50e6))

	mu.Lock()
	gc := goodCount
	mu.Unlock()
	if gc != 1 {
		t.Fatalf("good channel received %d messages, want 1", gc)
	}
	if n.Stats().Sent != 1 || n.Stats().Failed != 1 {
		t.Fatalf("stats = %+v, want Sent=1 Failed=1", n.Stats())
	}
}

// TestCloseDrainsQueuedNotifications is the regression case for finding 2:
// Start's select used to return on <-n.stop without draining the queue, so
// notifications queued right before shutdown were silently dropped despite
// Close's doc comment promising to "wait to drain". Each result targets a
// distinct target so cooldown cannot mask a dropped one as "suppressed".
func TestCloseDrainsQueuedNotifications(t *testing.T) {
	n, db, got, _ := newHarness(t, notifications(t, 60))
	const nTargets = 5
	for i := 0; i < nTargets; i++ {
		id := seedTarget(t, db, `{"download_mbps_min":100}`)
		n.OnResult(context.Background(), result(id, 50e6), Meta{})
	}
	n.Start()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := n.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(*got) != nTargets {
		t.Fatalf("Close dropped queued notifications: got %d messages, want %d", len(*got), nTargets)
	}
}

// TestSuppressedAlertSkipsRecoveryNotice and
// TestSuppressedAlertDoesNotDelayFirstRealAlert are the regression cases
// for finding 3: firing during quiet hours writes alert state without
// delivering anything, so (a) a later recovery must not claim something
// was "recovered" that nobody was ever told about, and (b) the state
// write must not start the cooldown clock, or the first real (delivered)
// alert would be delayed by a full cooldown after quiet hours end.

func TestSuppressedAlertSkipsRecoveryNotice(t *testing.T) {
	cfg := notifications(t, 60)
	cfg.QuietHoursStart, cfg.QuietHoursEnd = "22:00", "07:00"
	n, db, got, clock := newHarness(t, cfg)
	id := seedTarget(t, db, `{"download_mbps_min":100}`)
	*clock = time.Date(2026, 9, 14, 23, 30, 0, 0, time.UTC) // inside quiet hours

	n.process(context.Background(), result(id, 50e6)) // breach, suppressed
	if len(*got) != 0 {
		t.Fatalf("delivered during quiet hours: %+v", *got)
	}

	*clock = time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC) // now outside quiet hours
	n.process(context.Background(), result(id, 150e6))    // recovers
	if len(*got) != 0 {
		t.Fatalf("sent a recovery notice for an alert that was never delivered: %+v", *got)
	}
	if _, ok, _ := db.GetNotifyState(context.Background(), id, "download"); ok {
		t.Fatal("recovered state must still be cleared even when no notice is sent")
	}
}

func TestSuppressedAlertDoesNotDelayFirstRealAlert(t *testing.T) {
	cfg := notifications(t, 60) // 60 minute cooldown
	cfg.QuietHoursStart, cfg.QuietHoursEnd = "22:00", "23:00"
	n, db, got, clock := newHarness(t, cfg)
	id := seedTarget(t, db, `{"download_mbps_min":100}`)
	*clock = time.Date(2026, 9, 14, 22, 30, 0, 0, time.UTC) // inside quiet hours

	n.process(context.Background(), result(id, 50e6)) // suppressed
	if len(*got) != 0 {
		t.Fatalf("delivered during quiet hours: %+v", *got)
	}

	*clock = clock.Add(35 * time.Minute) // 23:05: quiet hours over, only 35m since the suppressed attempt
	n.process(context.Background(), result(id, 50e6))
	if len(*got) != 1 {
		t.Fatalf("first real alert was delayed by the suppressed attempt's cooldown: %+v", *got)
	}
}
