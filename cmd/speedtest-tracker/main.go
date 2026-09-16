// Command speedtest-tracker serves the API and the embedded web UI.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/metril/speedtest-tracker/internal/api"
	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/config"
	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/engine/ookla"
	"github.com/metril/speedtest-tracker/internal/iperf3list"
	"github.com/metril/speedtest-tracker/internal/metrics"
	"github.com/metril/speedtest-tracker/internal/notify"
	"github.com/metril/speedtest-tracker/internal/oidcauth"
	"github.com/metril/speedtest-tracker/internal/ooklaweb"
	"github.com/metril/speedtest-tracker/internal/prune"
	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/scheduler"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/sse"
	"github.com/metril/speedtest-tracker/internal/store"
	"github.com/metril/speedtest-tracker/internal/vlpush"
	"github.com/metril/speedtest-tracker/internal/vmpush"
	"github.com/metril/speedtest-tracker/internal/web"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(healthcheckMain(config.Load().Listen))
	}
	if len(os.Args) > 1 && os.Args[1] == "run" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code := runCmd(ctx, os.Args[2:], os.Stdout, os.Stderr)
		stop()
		os.Exit(code)
	}

	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger, level); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// healthcheckMain runs the -healthcheck probe against the given ST_LISTEN
// address and returns a process exit code: 0 if the server answered
// /healthz with 200, 1 otherwise. It exists so the Docker HEALTHCHECK
// needs no curl/wget in the runtime image.
func healthcheckMain(listen string) int {
	if err := healthcheck(listen); err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	return 0
}

func healthcheck(listen string) error {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("parse listen address %q: %w", listen, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s:%s/healthz", host, port))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /healthz: status %d", resp.StatusCode)
	}
	return nil
}

// storeTokens adapts *store.Store to auth.TokenLookup so internal/auth
// never imports internal/store.
type storeTokens struct{ db *store.Store }

func (s storeTokens) LookupToken(ctx context.Context, hash string) (int64, bool, error) {
	t, ok, err := s.db.APITokenByHash(ctx, hash)
	return t.ID, ok, err
}

func (s storeTokens) TouchToken(ctx context.Context, id int64) error {
	return s.db.TouchAPIToken(ctx, id)
}

// storeSessions adapts *store.Store's session lookup to auth.SessionLookup,
// whose SessionInfo return type differs from store.Session (and lives in
// internal/auth precisely so that package never imports internal/store).
type storeSessions struct{ db *store.Store }

func (s storeSessions) LookupSession(ctx context.Context, hashedID string, now time.Time) (auth.SessionInfo, bool, error) {
	sess, ok, err := s.db.LookupSession(ctx, hashedID, now)
	if err != nil || !ok {
		return auth.SessionInfo{}, ok, err
	}
	return auth.SessionInfo{
		Subject:  sess.Subject,
		Email:    sess.Email,
		Name:     sess.Name,
		Username: sess.Username,
		Groups:   sess.Groups,
		IsAdmin:  sess.IsAdmin,
	}, true, nil
}

// oidcDiscoverTimeout bounds building/rebuilding the OIDC provider, which
// performs a live discovery request against the configured issuer.
const oidcDiscoverTimeout = 10 * time.Second

// oidcConfigFromAuth extracts the oidcauth.Config a builds an OIDC provider
// from settings.Auth. AdminGroup is shared with forward_auth (settings.Auth
// has one AdminGroup field, not a separate oidc-only one).
func oidcConfigFromAuth(a settings.Auth) oidcauth.Config {
	return oidcauth.Config{
		Issuer:          a.OIDCIssuer,
		ClientID:        a.OIDCClientID,
		ClientSecret:    a.OIDCClientSecret,
		RedirectBaseURL: a.OIDCRedirectBaseURL,
		Scopes:          a.OIDCScopes,
		GroupsClaim:     a.OIDCGroupsClaim,
		AdminGroup:      a.AdminGroup,
		AllowedGroups:   a.OIDCAllowedGroups,
		AllowedEmails:   a.OIDCAllowedEmails,
		SessionTTL:      time.Duration(a.SessionTTLHours) * time.Hour,
	}
}

// oidcProviderHolder holds the currently active OIDC provider (nil when
// auth mode is not oidc, or the last build attempt failed), rebuilt only
// when the oidc-relevant settings fields actually change — so that, say,
// narrowing trusted_proxies for forward_auth (an unrelated auth.* change)
// never triggers a fresh discovery request against the oidc issuer.
type oidcProviderHolder struct {
	ptr atomic.Pointer[oidcauth.Provider]

	mu      sync.Mutex
	applied oidcauth.Config
	built   bool
}

// Load returns the currently active provider, or nil. It is Deps.OIDC.
func (h *oidcProviderHolder) Load() *oidcauth.Provider { return h.ptr.Load() }

// apply rebuilds the provider from a when a.Mode is oidc and the resulting
// oidcauth.Config differs from the one last successfully applied; it is a
// no-op otherwise. When a.Mode is not oidc, any existing provider is
// cleared. A discovery failure clears the provider (rather than leaving a
// stale one in place) and returns the error for the caller to log.
func (h *oidcProviderHolder) apply(ctx context.Context, a settings.Auth) error {
	if a.Mode != settings.AuthModeOIDC {
		h.mu.Lock()
		h.built = false
		h.mu.Unlock()
		h.ptr.Store(nil)
		return nil
	}

	cfg := oidcConfigFromAuth(a)

	h.mu.Lock()
	unchanged := h.built && reflect.DeepEqual(h.applied, cfg)
	h.mu.Unlock()
	if unchanged {
		return nil
	}

	discoverCtx, cancel := context.WithTimeout(ctx, oidcDiscoverTimeout)
	defer cancel()
	p, err := oidcauth.New(discoverCtx, cfg, nil)
	if err != nil {
		h.mu.Lock()
		h.built = false
		h.mu.Unlock()
		h.ptr.Store(nil)
		return fmt.Errorf("build oidc provider: %w", err)
	}

	h.mu.Lock()
	h.applied = cfg
	h.built = true
	h.mu.Unlock()
	h.ptr.Store(p)
	return nil
}

// authConfigurer is the subset of *auth.Middleware that applyAuth needs.
// It exists so authAdapter (below) can also satisfy it: *auth.Middleware
// has no way to report its own configured mode, so production code routes
// every Configure call through authAdapter, which remembers the mode
// alongside delegating to the real middleware.
type authConfigurer interface {
	Configure(settings.Auth) error
}

// authAdapter implements api.Authenticator over *auth.Middleware, tracking
// the last successfully configured mode so GET /api/v1/me can answer
// without a second settings read.
type authAdapter struct {
	mw   *auth.Middleware
	mode atomic.Value // string
}

func newAuthAdapter(mw *auth.Middleware) *authAdapter {
	a := &authAdapter{mw: mw}
	a.mode.Store(settings.AuthModeOpen)
	return a
}

func (a *authAdapter) Handler(next http.Handler) http.Handler { return a.mw.Handler(next) }

func (a *authAdapter) Mode() string {
	mode, _ := a.mode.Load().(string)
	return mode
}

func (a *authAdapter) Configure(cfg settings.Auth) error {
	if err := a.mw.Configure(cfg); err != nil {
		return err
	}
	a.mode.Store(cfg.Mode)
	return nil
}

// applyAuth pushes the stored Auth section into the live auth middleware
// and, when oidc is non-nil, the live OIDC provider. It mirrors
// applyIntegrations/applyNotifications: called once at startup and again
// on every auth.* settings change. On a bad CIDR, m.Configure leaves the
// previously applied configuration in place and this returns the wrapped
// error, so the caller decides whether that is fatal. A failure building
// the OIDC provider is logged but never returned: it must not abort
// startup or the auth-settings watcher, since login simply shows
// oidc_not_configured until the issuer is reachable/fixed.
func applyAuth(ctx context.Context, st *settings.Store, m authConfigurer, oidc *oidcProviderHolder, logger *slog.Logger) error {
	a, err := st.Auth(ctx)
	if err != nil {
		return err
	}
	if err := m.Configure(a); err != nil {
		return fmt.Errorf("configure auth: %w", err)
	}
	if oidc != nil {
		if err := oidc.apply(ctx, a); err != nil {
			logger.Error("build oidc provider", "error", err)
		}
	}
	logger.Debug("auth applied", "mode", a.Mode)
	return nil
}

// parseLevel maps a settings log level string onto a slog.Level,
// defaulting to info for anything unrecognised.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// watchSettings applies live settings changes: general.log_level retunes
// the logger in place and any engines.* change rebuilds the engine
// registry, invalidates the Ookla server-list cache, and kicks the iperf3
// list refresher (so a cleared/changed engines.iperf3_list_url takes
// effect immediately rather than on the next 24h tick). It returns when
// ctx is done or changes is closed.
//
// The caller subscribes (st.Subscribe) and passes the resulting channel in,
// rather than watchSettings subscribing itself: that makes the subscription
// exist synchronously before this goroutine is even started, so a caller
// (or test) that writes a setting right after starting watchSettings can
// never race the notification past a subscriber that isn't listening yet.
func watchSettings(ctx context.Context, st *settings.Store, changes <-chan string, level *slog.LevelVar,
	reg *engine.Registry, servers *ookla.ServerList, sch *scheduler.Scheduler,
	vm *vmpush.Writer, vl *vlpush.Handler, nt *notify.Notifier, am authConfigurer, oidcHolder *oidcProviderHolder, metricsEnabled *atomic.Bool,
	ir *iperf3list.Refresher, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case key, open := <-changes:
			if !open {
				return
			}
			switch {
			case key == settings.KeyLogLevel:
				g, err := st.General(ctx)
				if err != nil {
					logger.Error("reload general settings", "error", err)
					continue
				}
				level.Set(parseLevel(g.LogLevel))
				logger.Info("log level changed", "level", g.LogLevel)
			case strings.HasPrefix(key, "engines."):
				eng, err := st.Engines(ctx)
				if err != nil {
					logger.Error("reload engine settings", "error", err)
					continue
				}
				reg.Replace(buildEngines(eng))
				servers.Invalidate()
				// A kick is cheap (it just re-reads URLFunc and, at most,
				// runs one refresh) and covers every engines.* change,
				// not only KeyIperf3ListURL, so this doesn't need to
				// special-case the key like the switch above does.
				ir.Kick()
				logger.Info("engines rebuilt", "changed_key", key)
			case key == settings.KeyTimezone:
				// Schedules with no explicit timezone follow the general
				// setting, so a change means every cron entry is rebuilt.
				if err := sch.Reload(ctx); err != nil {
					logger.Error("reload schedules after timezone change", "error", err)
					continue
				}
				// Quiet hours are also evaluated in the general timezone.
				if err := applyNotifications(ctx, st, nt, logger); err != nil {
					logger.Error("reload notifications after timezone change", "error", err)
					continue
				}
				logger.Info("schedules reloaded after timezone change")
			case strings.HasPrefix(key, "notifications."):
				if err := applyNotifications(ctx, st, nt, logger); err != nil {
					logger.Error("reload notifications", "error", err)
					continue
				}
				logger.Info("notifications reloaded", "changed_key", key)
			case strings.HasPrefix(key, "integrations."):
				if err := applyIntegrations(ctx, st, vm, vl, metricsEnabled, logger); err != nil {
					logger.Error("reload integrations", "error", err)
					continue
				}
				logger.Info("integrations reloaded", "changed_key", key)
			case strings.HasPrefix(key, "auth."):
				if err := applyAuth(ctx, st, am, oidcHolder, logger); err != nil {
					logger.Error("reload auth", "error", err)
					continue
				}
				logger.Info("auth reloaded", "changed_key", key)
			}
		}
	}
}

// applyIntegrations pushes the stored Integrations section into the live
// VictoriaMetrics and VictoriaLogs clients, and caches MetricsEnabled in
// metricsEnabled so the /metrics handler never needs a settings DB read per
// scrape. It is called once at startup and again on every integrations.*
// settings change, which is what makes the toggles take effect without a
// restart.
func applyIntegrations(ctx context.Context, st *settings.Store, vm *vmpush.Writer, vl *vlpush.Handler, metricsEnabled *atomic.Bool, logger *slog.Logger) error {
	i, err := st.Integrations(ctx)
	if err != nil {
		return err
	}
	vm.Configure(i.VMEnabled, i.VMURL, i.VMAuth(), i.VMExtraLabels)
	vl.Configure(i.VLEnabled, i.VLURL, i.VLAuth(), i.VLStreamFields)
	metricsEnabled.Store(i.MetricsEnabled)
	logger.Debug("integrations applied", "vm_enabled", i.VMEnabled, "vl_enabled", i.VLEnabled)
	return nil
}

// applyNotifications pushes the stored Notifications section into the live
// notifier, resolving quiet hours against the general timezone. It runs at
// startup and on every notifications.* or general.timezone change, which
// is what makes channel edits take effect without a restart.
func applyNotifications(ctx context.Context, st *settings.Store, n *notify.Notifier, logger *slog.Logger) error {
	cfg, err := st.Notifications(ctx)
	if err != nil {
		return err
	}
	g, err := st.General(ctx)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(g.Timezone)
	if err != nil {
		logger.Warn("notification quiet hours falling back to UTC", "timezone", g.Timezone, "error", err)
		loc = time.UTC
	}
	n.Configure(cfg, loc)
	logger.Debug("notifications applied", "enabled", cfg.Enabled, "channels", len(cfg.Channels))
	return nil
}

// migrateNtfyChannels rewrites any stored "ntfy" channels to "apprise" and
// persists the result, once at startup after settings are loaded and
// before the notifier is configured from them. A no-op when there is
// nothing to migrate.
func migrateNtfyChannels(ctx context.Context, st *settings.Store, logger *slog.Logger) error {
	cfg, err := st.Notifications(ctx)
	if err != nil {
		return err
	}
	migrated, changed := notify.MigrateNtfyChannels(cfg.Channels, logger)
	if !changed {
		return nil
	}
	if err := st.Set(ctx, settings.KeyNotifyChannels, migrated); err != nil {
		return fmt.Errorf("persist migrated notify channels: %w", err)
	}
	logger.Info("notify: migrated ntfy channels to apprise", "channels", len(migrated))
	return nil
}

func run(ctx context.Context, logger *slog.Logger, level *slog.LevelVar) error {
	cfg := config.Load()
	logger.Info("starting", "version", version, "db_path", cfg.DBPath, "listen", cfg.Listen)

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	st, err := settings.New(ctx, db)
	if err != nil {
		return err
	}

	// Seed the settings table from ST_<SECTION>_<KEY> env vars before
	// anything reads a setting: a mistyped ST_ value must abort startup
	// rather than boot with a half-applied config. Values are logged as
	// key names only, never the values themselves.
	locked, err := st.SeedFromEnv(ctx, os.Environ())
	if err != nil {
		return fmt.Errorf("seed settings from env: %w", err)
	}
	if len(locked) > 0 {
		logger.Info("settings seeded from env", "locked_count", len(locked), "locked_keys", locked)
	} else {
		logger.Info("settings seeded from env", "locked_count", 0)
	}

	general, err := st.General(ctx)
	if err != nil {
		return err
	}
	level.Set(parseLevel(general.LogLevel))

	// Logger chain: every subsequent log record flows through the
	// VictoriaLogs handler, which mirrors to stdout JSON and ships a copy
	// to VictoriaLogs when configured.
	vlHandler := vlpush.New(vlpush.Config{
		Next: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}),
		App:  "speedtest-tracker",
	})
	vlHandler.Start()
	logger = slog.New(vlHandler)
	slog.SetDefault(logger)

	m := metrics.New()

	vm := vmpush.New(vmpush.Config{Logger: logger})
	vm.Start()

	var metricsEnabled atomic.Bool
	if err := applyIntegrations(ctx, st, vm, vlHandler, &metricsEnabled, logger); err != nil {
		return err
	}

	if err := migrateNtfyChannels(ctx, st, logger); err != nil {
		return err
	}

	nt := notify.New(notify.Config{Store: db, Logger: logger})
	nt.Start()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := nt.Close(shutCtx); err != nil {
			logger.Warn("notifier shutdown", "error", err)
		}
	}()
	if err := applyNotifications(ctx, st, nt, logger); err != nil {
		return err
	}

	authMW := auth.New(logger, storeTokens{db}, time.Now)
	authMW.SetSessions(storeSessions{db})
	am := newAuthAdapter(authMW)
	oidcHolder := &oidcProviderHolder{}
	stateCodec, err := oidcauth.NewStateCodec()
	if err != nil {
		return fmt.Errorf("create oidc state codec: %w", err)
	}
	// Apply the stored auth config once at boot, but log and continue on
	// error rather than aborting: a bad stored CIDR (e.g. hand-edited in
	// the DB) must not make the instance unbootable. authAdapter starts
	// in open mode, which is the safe-to-serve fallback only because the
	// stored config was never successfully applied yet; once applyAuth
	// has succeeded at least once, a later failure (from watchSettings)
	// instead keeps whatever config was last successfully applied.
	if err := applyAuth(ctx, st, am, oidcHolder, logger); err != nil {
		logger.Error("apply auth settings", "error", err)
	}

	engineCfg, err := st.Engines(ctx)
	if err != nil {
		return err
	}
	reg := buildRegistry(engineCfg)
	servers := ookla.NewServerList(engineCfg.SpeedtestBin,
		time.Duration(engineCfg.ServerListTTLSeconds)*time.Second)
	ooklaSearch := ooklaweb.NewClient()
	ooklaSearch.Logger = logger
	ooklaSearch.UserAgent = fmt.Sprintf("speedtest-tracker/%s (+https://github.com/metril/speedtest-tracker)", version)

	hub := sse.NewHub()
	rn := runner.New(runner.Config{
		Store: db, Registry: reg, Hub: hub, Logger: logger,
		Sink: runner.Sinks{
			runner.SinkFunc(func(ctx context.Context, res *store.Result, meta runner.ResultMeta) {
				vm.OnResult(ctx, res, vmpush.Meta{Schedule: meta.ScheduleName})
			}),
			runner.SinkFunc(func(_ context.Context, res *store.Result, meta runner.ResultMeta) {
				m.ObserveResult(res, meta.ScheduleName)
			}),
			runner.SinkFunc(func(ctx context.Context, res *store.Result, meta runner.ResultMeta) {
				nt.OnResult(ctx, res, notify.Meta{ScheduleName: meta.ScheduleName})
			}),
		},
	})
	rn.Start()

	m.AddLabelledGaugeFunc("speedtest_runner_queue_depth", "jobs queued per queue", "queue",
		func() map[string]float64 {
			out := map[string]float64{}
			for queue, n := range rn.QueueDepths() {
				out[queue] = float64(n)
			}
			return out
		})
	m.AddGaugeFunc("speedtest_vm_push_pushed_total", "batches accepted by VictoriaMetrics",
		func() float64 { return float64(vm.Stats().Pushed) })
	m.AddGaugeFunc("speedtest_vm_push_failed_total", "batches rejected by VictoriaMetrics",
		func() float64 { return float64(vm.Stats().Failed) })
	m.AddGaugeFunc("speedtest_vm_push_dropped_total", "results dropped from the VictoriaMetrics ring buffer",
		func() float64 { return float64(vm.Stats().Dropped) })
	m.AddGaugeFunc("speedtest_vm_push_queued", "results currently queued for VictoriaMetrics",
		func() float64 { return float64(vm.Stats().Queued) })
	m.AddGaugeFunc("speedtest_vl_lines_dropped_total", "log lines dropped from the VictoriaLogs queue",
		func() float64 { return float64(vlHandler.Dropped()) })
	m.AddGaugeFunc("speedtest_notifications_sent_total", "notifications delivered to a channel",
		func() float64 { return float64(nt.Stats().Sent) })
	m.AddGaugeFunc("speedtest_notifications_failed_total", "notification deliveries that returned an error",
		func() float64 { return float64(nt.Stats().Failed) })
	m.AddGaugeFunc("speedtest_notifications_suppressed_total", "notifications withheld by quiet hours",
		func() float64 { return float64(nt.Stats().Suppressed) })
	m.AddGaugeFunc("speedtest_notifications_dropped_total", "results dropped from the notifier queue",
		func() float64 { return float64(nt.Stats().Dropped) })
	m.AddGaugeFunc("speedtest_notifications_queued", "results currently queued for notification",
		func() float64 { return float64(nt.Stats().Queued) })

	sch := scheduler.New(scheduler.Config{Store: db, Runner: rn, Logger: logger})
	if err := sch.Reload(ctx); err != nil {
		return err
	}

	pj := prune.New(prune.Config{Store: db, Settings: st, Logger: logger})

	iperf3Refresher := iperf3list.New(iperf3list.Config{
		Store: db,
		URLFunc: func() string {
			e, err := st.Engines(context.Background())
			if err != nil {
				logger.Warn("read engines settings for iperf3 list URL", "error", err)
				return ""
			}
			return e.Iperf3ListURL
		},
		Logger: logger,
	})

	changes, unsubscribe := st.Subscribe()
	defer unsubscribe()
	watchCtx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		watchSettings(watchCtx, st, changes, level, reg, servers, sch, vm, vlHandler, nt, am, oidcHolder, &metricsEnabled, iperf3Refresher, logger)
	}()
	go pj.Run(watchCtx)
	go iperf3Refresher.Run(watchCtx)

	srv := &http.Server{
		Addr: cfg.Listen,
		Handler: api.New(api.Deps{
			Pinger:          db,
			Logger:          logger,
			UI:              web.Handler(),
			Hub:             hub,
			Store:           db,
			Registry:        reg,
			Runner:          rn,
			ServerList:      servers,
			OoklaSearch:     ooklaSearch,
			OoklaLimiter:    api.NewOoklaLimiter(),
			Iperf3:          iperf3Refresher,
			ReloadSchedules: sch.Reload,
			Scheduler:       sch,
			Settings:        st,
			Notifier:        nt,
			Metrics:         m,
			MetricsHandler:  m.Handler(),
			MetricsEnabled:  metricsEnabled.Load,
			Auth:            am,
			OIDC:            oidcHolder.Load,
			Sessions:        db,
			StateCodec:      stateCodec,
			Now:             time.Now,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		// Order: stop accepting HTTP, drain in-flight tests, then close
		// the database (deferred above).
		httpCtx, cancelHTTP := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelHTTP()
		if err := srv.Shutdown(httpCtx); err != nil {
			logger.Error("http shutdown", "error", err)
		}
		authMW.Close()
		stopWatch()
		watchTimeout, cancelWatch := context.WithTimeout(context.Background(), 5*time.Second)
		select {
		case <-watchDone:
		case <-watchTimeout.Done():
			logger.Warn("watchSettings did not stop before shutdown timeout")
		}
		cancelWatch()
		schedCtx, cancelSched := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelSched()
		if err := sch.Stop(schedCtx); err != nil {
			logger.Error("scheduler shutdown", "error", err)
		}
		runnerCtx, cancelRunner := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancelRunner()
		if err := rn.Shutdown(runnerCtx); err != nil {
			logger.Error("runner shutdown", "error", err)
		}
		flushCtx, cancelFlush := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelFlush()
		if err := vm.Close(flushCtx); err != nil {
			logger.Error("victoriametrics flush", "error", err)
		}
		if err := vlHandler.Close(flushCtx); err != nil {
			fmt.Fprintln(os.Stderr, "victorialogs flush:", err) // the logger is going away
		}
		return <-errCh
	}
}
