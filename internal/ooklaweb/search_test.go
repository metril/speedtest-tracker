package ooklaweb

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// speedtestServer builds an httptest server answering speedtest.net's
// server-search shape, using the fixture that matches the "search" query
// param, or an empty array otherwise. reqCount, if non-nil, is incremented
// on every request.
func speedtestServer(t *testing.T, fixtures map[string]string, reqCount *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqCount != nil {
			atomic.AddInt32(reqCount, 1)
		}
		q := r.URL.Query().Get("search")
		body, ok := fixtures[q]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
}

func geoServer(t *testing.T, fixtures map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		body, ok := fixtures[name]
		if !ok {
			w.Write([]byte(`{"results":[]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
}

// nominatimReq records one request Nominatim received, for assertions on
// which params/headers were sent.
type nominatimReq struct {
	postalcode   string
	q            string
	countrycodes string
	userAgent    string
}

// nominatimServer builds an httptest server answering Nominatim's jsonv2
// search shape. handle is called for every request and returns the JSON
// body to send; every request is also recorded in reqs (if non-nil).
func nominatimServer(t *testing.T, reqs *[]nominatimReq, handle func(url.Values) string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if reqs != nil {
			mu.Lock()
			*reqs = append(*reqs, nominatimReq{
				postalcode: q.Get("postalcode"), q: q.Get("q"),
				countrycodes: q.Get("countrycodes"), userAgent: r.Header.Get("User-Agent"),
			})
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(handle(q)))
	}))
}

const denverFixture = `[
  {"id":1,"name":"Denver, CO","country":"United States","cc":"US","sponsor":"Comcast","host":"denver1.example.net:8080","lat":"39.74","lon":"-104.98","distance":"1.2"}
]`

const denverMetroFixture = `[
  {"id":1,"name":"Denver, CO","country":"United States","cc":"US","sponsor":"Comcast","host":"denver1.example.net:8080","lat":"39.74","lon":"-104.98","distance":"1.2"},
  {"id":2,"name":"Aurora, CO","country":"United States","cc":"US","sponsor":"CenturyLink","host":"aurora1.example.net:8080","lat":"39.73","lon":"-104.83","distance":"9.9"},
  {"id":3,"name":"Boulder, CO","country":"United States","cc":"US","sponsor":"Xfinity","host":"boulder1.example.net:8080","lat":"40.01","lon":"-105.27","distance":"25.5"}
]`

const geoDenverFixture = `{"results":[{"name":"Denver","latitude":39.7392,"longitude":-104.9903,"country_code":"US","postcodes":["80202"]}]}`

const nominatimDenverFixture = `[{"display_name":"Denver, Colorado, United States","lat":"39.7392","lon":"-104.9903"}]`

// noNominatimSleep is a nomSleep stub that never actually sleeps, so tests
// exercising multiple sequential Nominatim requests (which would
// otherwise pace themselves 1s apart, per nominatimMinInterval) run
// instantly. The rate-limiting behavior itself is covered separately by
// TestNominatimRateLimitedToOnePerSecond, which asserts on wait durations
// directly instead.
func noNominatimSleep(context.Context, time.Duration) error { return nil }

func TestSearchPostcodeGeocodesViaNominatimAndMerges(t *testing.T) {
	sp := speedtestServer(t, map[string]string{
		"80202":                          `[]`,
		"Denver, Colorado, United States": denverMetroFixture,
	}, nil)
	defer sp.Close()
	nom := nominatimServer(t, nil, func(q url.Values) string {
		if q.Get("postalcode") == "80202" {
			return nominatimDenverFixture
		}
		return `[]`
	})
	defer nom.Close()

	c := NewClient()
	c.Base = sp.URL
	c.NominatimBase = nom.URL
	c.nomSleep = noNominatimSleep

	res, err := c.Search(context.Background(), SearchRequest{Q: "80202", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Servers) != 3 {
		t.Fatalf("got %d servers, want 3: %+v", len(res.Servers), res.Servers)
	}
	for _, s := range res.Servers {
		if s.DistanceKm == 0 {
			t.Errorf("server %+v missing distance_km", s)
		}
	}
	if res.Near != "Denver, Colorado" {
		t.Errorf("near = %q, want %q (first two comma parts)", res.Near, "Denver, Colorado")
	}
}

func TestSearchPostcodeWithCountryUsesCountrycodesThenFallsBackWithoutIt(t *testing.T) {
	sp := speedtestServer(t, nil, nil)
	defer sp.Close()
	var reqs []nominatimReq
	nom := nominatimServer(t, &reqs, func(q url.Values) string {
		if q.Get("postalcode") != "" && q.Get("countrycodes") == "us" {
			return nominatimDenverFixture
		}
		return `[]`
	})
	defer nom.Close()

	c := NewClient()
	c.Base = sp.URL
	c.NominatimBase = nom.URL
	c.nomSleep = noNominatimSleep

	res, err := c.Search(context.Background(), SearchRequest{Q: "80202", Country: "us", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Near == "" {
		t.Fatal("want a resolved Near place")
	}
	if len(reqs) != 1 || reqs[0].countrycodes != "us" || reqs[0].postalcode != "80202" {
		t.Fatalf("nominatim requests = %+v, want exactly 1 with postalcode=80202&countrycodes=us", reqs)
	}
}

func TestSearchPostcodeFallsBackWithoutCountryThenFreeForm(t *testing.T) {
	sp := speedtestServer(t, nil, nil)
	defer sp.Close()
	var reqs []nominatimReq
	nom := nominatimServer(t, &reqs, func(q url.Values) string {
		// Only the third, free-form q= attempt (no postalcode) matches.
		if q.Get("q") != "" && q.Get("postalcode") == "" {
			return nominatimDenverFixture
		}
		return `[]`
	})
	defer nom.Close()

	c := NewClient()
	c.Base = sp.URL
	c.NominatimBase = nom.URL
	c.nomSleep = noNominatimSleep

	res, err := c.Search(context.Background(), SearchRequest{Q: "80202", Country: "us", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Near == "" {
		t.Fatal("want a resolved Near place from the free-form fallback")
	}
	// postalcode+countrycodes, postalcode alone, then free-form q=.
	if len(reqs) != 3 {
		t.Fatalf("nominatim requests = %+v, want exactly 3 (progressively looser)", reqs)
	}
	if reqs[0].countrycodes != "us" || reqs[0].postalcode != "80202" {
		t.Errorf("request 0 = %+v, want postalcode+countrycodes", reqs[0])
	}
	if reqs[1].postalcode != "80202" || reqs[1].countrycodes != "" {
		t.Errorf("request 1 = %+v, want postalcode alone", reqs[1])
	}
	if reqs[2].q != "80202" || reqs[2].postalcode != "" {
		t.Errorf("request 2 = %+v, want free-form q=", reqs[2])
	}
}

func TestSearchPostcodeSendsUserAgentToNominatim(t *testing.T) {
	sp := speedtestServer(t, nil, nil)
	defer sp.Close()
	var reqs []nominatimReq
	nom := nominatimServer(t, &reqs, func(q url.Values) string { return nominatimDenverFixture })
	defer nom.Close()

	c := NewClient()
	c.Base = sp.URL
	c.NominatimBase = nom.URL
	c.nomSleep = noNominatimSleep
	c.UserAgent = "speedtest-tracker/1.2.3 (+https://example.com)"

	if _, err := c.Search(context.Background(), SearchRequest{Q: "80202", Limit: 10}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(reqs) == 0 || reqs[0].userAgent != "speedtest-tracker/1.2.3 (+https://example.com)" {
		t.Fatalf("nominatim requests = %+v, want the configured User-Agent", reqs)
	}
}

func TestSearchPostcodeFallsBackToOpenMeteoWhenNominatimEmpty(t *testing.T) {
	sp := speedtestServer(t, map[string]string{"Denver": denverMetroFixture}, nil)
	defer sp.Close()
	nom := nominatimServer(t, nil, func(q url.Values) string { return `[]` })
	defer nom.Close()
	geo := geoServer(t, map[string]string{"80202": geoDenverFixture})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.NominatimBase = nom.URL
	c.nomSleep = noNominatimSleep
	c.GeoBase = geo.URL

	res, err := c.Search(context.Background(), SearchRequest{Q: "80202", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Near != "Denver" {
		t.Errorf("near = %q, want %q (Open-Meteo fallback)", res.Near, "Denver")
	}
	if len(res.Servers) != 3 {
		t.Fatalf("got %d servers, want 3 (merged via Open-Meteo fallback)", len(res.Servers))
	}
}

func TestNominatimRateLimitedToOnePerSecond(t *testing.T) {
	nom := nominatimServer(t, nil, func(q url.Values) string { return `[]` })
	defer nom.Close()
	sp := speedtestServer(t, nil, nil)
	defer sp.Close()
	// Nominatim never matches in this test, so Search falls through to the
	// Open-Meteo geocoder too; stub it locally so the test never touches
	// the real network.
	geo := geoServer(t, map[string]string{})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.NominatimBase = nom.URL
	c.GeoBase = geo.URL

	fixedNow := time.Now()
	c.nomNow = func() time.Time { return fixedNow }
	var slept []time.Duration
	c.nomSleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		fixedNow = fixedNow.Add(d)
		return nil
	}

	// Two postcode searches for different queries (so they aren't
	// single-flighted or cache-hit) must still be paced >= 1s apart.
	if _, err := c.Search(context.Background(), SearchRequest{Q: "80202", Limit: 10}); err != nil {
		t.Fatalf("Search 1: %v", err)
	}
	if _, err := c.Search(context.Background(), SearchRequest{Q: "10001", Limit: 10}); err != nil {
		t.Fatalf("Search 2: %v", err)
	}
	if len(slept) == 0 {
		t.Fatal("want at least one enforced wait between Nominatim requests")
	}
	for _, d := range slept {
		if d <= 0 || d > nominatimMinInterval {
			t.Errorf("slept %v, want (0, %v]", d, nominatimMinInterval)
		}
	}
}

// TestWaitNominatimRespectsContextCancellation covers the fix-round-1
// finding: waitNominatim must be ctx-aware, returning promptly with the
// ctx's error instead of blocking for the full rate-limit wait when the
// caller's context is canceled. Uses the real (default) ctxSleep — not a
// stubbed nomSleep — since the point is to exercise the actual
// select-on-ctx.Done() behavior.
func TestWaitNominatimRespectsContextCancellation(t *testing.T) {
	c := NewClient()
	fixedNow := time.Now()
	c.nomNow = func() time.Time { return fixedNow }
	c.nomLast = fixedNow // a request "just happened": the next wait is ~nominatimMinInterval

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before waitNominatim is even called

	start := time.Now()
	err := c.waitNominatim(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitNominatim = %v, want context.Canceled", err)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("waitNominatim blocked %v despite an already-canceled ctx, want a prompt return well under nominatimMinInterval (%v)",
			elapsed, nominatimMinInterval)
	}
}

func TestSearchSortsByDistanceFromGeocodedPoint(t *testing.T) {
	// Denver metro fixture is unsorted by true distance from the geocoded
	// point (39.7392,-104.9903): Aurora is nearest, then Denver, then
	// Boulder farthest.
	sp := speedtestServer(t, map[string]string{
		"denver": denverMetroFixture,
		"Denver": denverMetroFixture,
	}, nil)
	defer sp.Close()
	geo := geoServer(t, map[string]string{"denver": geoDenverFixture})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.GeoBase = geo.URL

	res, err := c.Search(context.Background(), SearchRequest{Q: "denver", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := res.Servers
	if len(got) != 3 {
		t.Fatalf("got %d servers, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].DistanceKm > got[i].DistanceKm {
			t.Fatalf("not sorted by distance: %+v", got)
		}
	}
}

// denverMetroWithUnplaceableFixture adds a 4th hit ("id":4) with no lat/lon
// at all — speedtest.net returns this for servers it can't place. Its
// distance is unknown, not zero, so it must not be ranked as the nearest
// result.
const denverMetroWithUnplaceableFixture = `[
  {"id":1,"name":"Denver, CO","country":"United States","cc":"US","sponsor":"Comcast","host":"denver1.example.net:8080","lat":"39.74","lon":"-104.98","distance":"1.2"},
  {"id":2,"name":"Aurora, CO","country":"United States","cc":"US","sponsor":"CenturyLink","host":"aurora1.example.net:8080","lat":"39.73","lon":"-104.83","distance":"9.9"},
  {"id":3,"name":"Boulder, CO","country":"United States","cc":"US","sponsor":"Xfinity","host":"boulder1.example.net:8080","lat":"40.01","lon":"-105.27","distance":"25.5"},
  {"id":4,"name":"Unknown","country":"United States","cc":"US","sponsor":"Mystery ISP","host":"mystery.example.net:8080"}
]`

func TestSearchSortsCoordinateLessServersLast(t *testing.T) {
	sp := speedtestServer(t, map[string]string{
		"denver": denverMetroWithUnplaceableFixture,
		"Denver": denverMetroWithUnplaceableFixture,
	}, nil)
	defer sp.Close()
	geo := geoServer(t, map[string]string{"denver": geoDenverFixture})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.GeoBase = geo.URL

	res, err := c.Search(context.Background(), SearchRequest{Q: "denver", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := res.Servers
	if len(got) != 4 {
		t.Fatalf("got %d servers, want 4: %+v", len(got), got)
	}
	last := got[len(got)-1]
	if last.ID != "4" {
		t.Fatalf("coordinate-less server not sorted last: got order %+v", got)
	}
	if last.DistanceKm != 0 {
		t.Errorf("coordinate-less server DistanceKm = %v, want 0 (omitted)", last.DistanceKm)
	}
	for i := 1; i < len(got)-1; i++ {
		if got[i-1].DistanceKm > got[i].DistanceKm {
			t.Fatalf("known-distance servers not sorted ascending: %+v", got)
		}
	}
}

func TestSearchCachesWithinTTL(t *testing.T) {
	var reqs int32
	sp := speedtestServer(t, map[string]string{"comcast": denverFixture}, &reqs)
	defer sp.Close()

	c := NewClient()
	c.Base = sp.URL
	// avoid geocode branch entirely by stubbing geo to return no results
	geo := geoServer(t, map[string]string{})
	defer geo.Close()
	c.GeoBase = geo.URL

	fixedNow := time.Now()
	c.now = func() time.Time { return fixedNow }

	if _, err := c.Search(context.Background(), SearchRequest{Q: "comcast", Limit: 10}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	firstReqs := atomic.LoadInt32(&reqs)
	if firstReqs == 0 {
		t.Fatal("expected at least one request")
	}

	if _, err := c.Search(context.Background(), SearchRequest{Q: "COMCAST", Limit: 10}); err != nil {
		t.Fatalf("Search (cached): %v", err)
	}
	if got := atomic.LoadInt32(&reqs); got != firstReqs {
		t.Errorf("cache hit made %d new requests, want 0 (case-insensitive key)", got-firstReqs)
	}

	// Advance past the TTL: cache must be bypassed.
	c.now = func() time.Time { return fixedNow.Add(16 * time.Minute) }
	if _, err := c.Search(context.Background(), SearchRequest{Q: "comcast", Limit: 10}); err != nil {
		t.Fatalf("Search (expired): %v", err)
	}
	if got := atomic.LoadInt32(&reqs); got <= firstReqs {
		t.Errorf("expired cache entry was not refetched: reqs=%d", got)
	}
}

func TestSearchCacheKeyIncludesCountry(t *testing.T) {
	var reqs int32
	sp := speedtestServer(t, nil, &reqs)
	defer sp.Close()
	nom := nominatimServer(t, nil, func(q url.Values) string {
		if q.Get("countrycodes") == "us" {
			return nominatimDenverFixture
		}
		return `[]`
	})
	defer nom.Close()
	// The no-country call below falls through to Open-Meteo (Nominatim
	// never matches without countrycodes=us); stub it locally.
	geo := geoServer(t, map[string]string{})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.NominatimBase = nom.URL
	c.GeoBase = geo.URL
	c.nomSleep = noNominatimSleep

	res1, err := c.Search(context.Background(), SearchRequest{Q: "80202", Country: "us", Limit: 10})
	if err != nil {
		t.Fatalf("Search (us): %v", err)
	}
	res2, err := c.Search(context.Background(), SearchRequest{Q: "80202", Limit: 10})
	if err != nil {
		t.Fatalf("Search (no country): %v", err)
	}
	if res1.Near == res2.Near {
		t.Errorf("same-query different-country results were not distinguished: %q == %q", res1.Near, res2.Near)
	}
}

func TestSearchTimeoutFallsBackWithError(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`[]`))
	}))
	defer slow.Close()

	c := NewClient()
	c.Base = slow.URL
	c.GeoBase = slow.URL
	c.HTTP = &http.Client{Timeout: 50 * time.Millisecond}

	_, err := c.Search(context.Background(), SearchRequest{Q: "denver", Limit: 10})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestLooksLikePostcode(t *testing.T) {
	cases := map[string]bool{
		"80202":      true,
		"SW1A 1AA":   true,
		"denver":     false,
		"comcast":    false,
		"":           false,
		"Denver, CO": false,
	}
	for q, want := range cases {
		if got := looksLikePostcode(q); got != want {
			t.Errorf("looksLikePostcode(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestNearName(t *testing.T) {
	cases := map[string]string{
		"Denver, Colorado, United States": "Denver, Colorado",
		"Denver":                          "Denver",
		"Denver, Colorado":                "Denver, Colorado",
		"":                                "",
	}
	for in, want := range cases {
		if got := nearName(in); got != want {
			t.Errorf("nearName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHaversineKm(t *testing.T) {
	// Denver to Boulder is roughly 40km.
	km := haversineKm(39.7392, -104.9903, 40.0150, -105.2705)
	if km < 30 || km > 50 {
		t.Errorf("haversineKm = %v, want ~40", km)
	}
	if got := haversineKm(10, 10, 10, 10); got != 0 {
		t.Errorf("haversineKm same point = %v, want 0", got)
	}
}

// TestSearchSingleFlightsConcurrentIdenticalQueries covers task-1-brief
// item 1: several concurrent Search calls for the same query (differing
// only in case) must share one upstream request, not fire one each.
func TestSearchSingleFlightsConcurrentIdenticalQueries(t *testing.T) {
	var reqs int32
	release := make(chan struct{})
	var inFlight int32
	sp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqs, 1)
		if atomic.AddInt32(&inFlight, 1) > 1 {
			t.Errorf("more than one request in flight concurrently")
		}
		<-release
		atomic.AddInt32(&inFlight, -1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(denverFixture))
	}))
	defer sp.Close()
	geo := geoServer(t, map[string]string{})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.GeoBase = geo.URL

	const n = 10
	queries := []string{"comcast", "COMCAST", "Comcast", "comCast"}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			if _, err := c.Search(context.Background(), SearchRequest{Q: q, Limit: 10}); err != nil {
				errs <- err
			}
		}(queries[i%len(queries)])
	}

	// Give every goroutine a chance to reach the singleflight/HTTP layer
	// before releasing the one in-flight request.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Search: %v", err)
	}

	if got := atomic.LoadInt32(&reqs); got != 1 {
		t.Errorf("upstream received %d requests, want exactly 1 (single-flighted)", got)
	}
}

func TestSearchLimitCaps(t *testing.T) {
	sp := speedtestServer(t, map[string]string{"denver": denverMetroFixture}, nil)
	defer sp.Close()
	geo := geoServer(t, map[string]string{})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.GeoBase = geo.URL

	res, err := c.Search(context.Background(), SearchRequest{Q: "denver", Limit: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Servers) != 1 {
		t.Fatalf("got %d servers, want 1 (limit)", len(res.Servers))
	}
}

func TestSearchLogsGeocodeFailureAtDebug(t *testing.T) {
	// Fewer than geocodeHitThreshold hits, so a geocode is attempted; the
	// geo server always answers 500, so gerr != nil and Search must not
	// fail — it just logs the geocode error and returns the direct hits.
	sp := speedtestServer(t, map[string]string{"comcast": denverFixture}, nil)
	defer sp.Close()
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer geo.Close()

	var buf bytes.Buffer
	c := NewClient()
	c.Base = sp.URL
	c.GeoBase = geo.URL
	c.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	res, err := c.Search(context.Background(), SearchRequest{Q: "comcast", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Servers) != 1 {
		t.Fatalf("got %d servers, want 1 (direct hit, geocode failure ignored)", len(res.Servers))
	}
	if !strings.Contains(buf.String(), "level=DEBUG") || !strings.Contains(buf.String(), "geocode failed") {
		t.Errorf("log output = %q, want a DEBUG line about the geocode failure", buf.String())
	}
}

// oversizedJSONServer answers every request with a JSON array opener
// followed by more than maxResponseBytes of well-formed-looking but never
// closed content, simulating a misbehaving or malicious upstream
// streaming an unbounded body.
func oversizedJSONServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[`))
		chunk := bytes.Repeat([]byte(`{"id":1,"name":"x"},`), 4096)
		for i := 0; i < (maxResponseBytes/len(chunk))+2; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
}

func TestSearchRejectsOversizedResponse(t *testing.T) {
	srv := oversizedJSONServer(t)
	defer srv.Close()

	c := NewClient()
	c.Base = srv.URL

	// A truncated body must fail cleanly, not succeed on a partial parse.
	if _, err := c.search(context.Background(), "x"); err == nil {
		t.Fatal("search: want error for oversized response, got nil")
	}
}

func TestGeocodeRejectsOversizedResponse(t *testing.T) {
	srv := oversizedJSONServer(t)
	defer srv.Close()

	c := NewClient()
	c.GeoBase = srv.URL

	if _, err := c.geocode(context.Background(), "x"); err == nil {
		t.Fatal("geocode: want error for oversized response, got nil")
	}
}

func TestNominatimSearchRejectsOversizedResponse(t *testing.T) {
	srv := oversizedJSONServer(t)
	defer srv.Close()

	c := NewClient()
	c.NominatimBase = srv.URL
	c.nomSleep = noNominatimSleep

	if _, err := c.nominatimSearch(context.Background(), url.Values{"q": {"x"}}); err == nil {
		t.Fatal("nominatimSearch: want error for oversized response, got nil")
	}
}

func TestSearchRefusesCrossHostRedirect(t *testing.T) {
	target := speedtestServer(t, map[string]string{"comcast": denverFixture}, nil)
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	defer redirector.Close()

	c := NewClient()
	c.Base = redirector.URL

	if _, err := c.search(context.Background(), "comcast"); err == nil {
		t.Fatal("search: want error for cross-host redirect, got nil")
	}
}

func TestSearchWithNilLoggerDoesNotPanicOnGeocodeFailure(t *testing.T) {
	sp := speedtestServer(t, map[string]string{"comcast": denverFixture}, nil)
	defer sp.Close()
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer geo.Close()

	c := NewClient() // c.Logger left nil
	c.Base = sp.URL
	c.GeoBase = geo.URL

	if _, err := c.Search(context.Background(), SearchRequest{Q: "comcast", Limit: 10}); err != nil {
		t.Fatalf("Search: %v", err)
	}
}
