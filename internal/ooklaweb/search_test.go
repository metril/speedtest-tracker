package ooklaweb

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

const denverFixture = `[
  {"id":1,"name":"Denver, CO","country":"United States","cc":"US","sponsor":"Comcast","host":"denver1.example.net:8080","lat":"39.74","lon":"-104.98","distance":"1.2"}
]`

const denverMetroFixture = `[
  {"id":1,"name":"Denver, CO","country":"United States","cc":"US","sponsor":"Comcast","host":"denver1.example.net:8080","lat":"39.74","lon":"-104.98","distance":"1.2"},
  {"id":2,"name":"Aurora, CO","country":"United States","cc":"US","sponsor":"CenturyLink","host":"aurora1.example.net:8080","lat":"39.73","lon":"-104.83","distance":"9.9"},
  {"id":3,"name":"Boulder, CO","country":"United States","cc":"US","sponsor":"Xfinity","host":"boulder1.example.net:8080","lat":"40.01","lon":"-105.27","distance":"25.5"}
]`

const geoDenverFixture = `{"results":[{"name":"Denver","latitude":39.7392,"longitude":-104.9903,"country_code":"US","postcodes":["80202"]}]}`

func TestSearchPostcodeGeocodesAndMerges(t *testing.T) {
	sp := speedtestServer(t, map[string]string{
		"80202": `[]`,
		"Denver": denverMetroFixture,
	}, nil)
	defer sp.Close()
	geo := geoServer(t, map[string]string{"80202": geoDenverFixture})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.GeoBase = geo.URL

	got, err := c.Search(context.Background(), "80202", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d servers, want 3: %+v", len(got), got)
	}
	for _, s := range got {
		if s.DistanceKm == 0 {
			t.Errorf("server %+v missing distance_km", s)
		}
	}
}

func TestSearchSortsByDistanceFromGeocodedPoint(t *testing.T) {
	// Denver metro fixture is unsorted by true distance from the geocoded
	// point (39.7392,-104.9903): Aurora is nearest, then Denver, then
	// Boulder farthest.
	sp := speedtestServer(t, map[string]string{
		"denver":  denverMetroFixture,
		"Denver":  denverMetroFixture,
	}, nil)
	defer sp.Close()
	geo := geoServer(t, map[string]string{"denver": geoDenverFixture})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.GeoBase = geo.URL

	got, err := c.Search(context.Background(), "denver", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
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

	got, err := c.Search(context.Background(), "denver", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
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
	c.GeoBase = sp.URL // unused; comcast returns 1 hit < 5 would trigger geocode too
	// avoid geocode branch entirely by stubbing geo to return no results
	geo := geoServer(t, map[string]string{})
	defer geo.Close()
	c.GeoBase = geo.URL

	fixedNow := time.Now()
	c.now = func() time.Time { return fixedNow }

	if _, err := c.Search(context.Background(), "comcast", 10); err != nil {
		t.Fatalf("Search: %v", err)
	}
	firstReqs := atomic.LoadInt32(&reqs)
	if firstReqs == 0 {
		t.Fatal("expected at least one request")
	}

	if _, err := c.Search(context.Background(), "COMCAST", 10); err != nil {
		t.Fatalf("Search (cached): %v", err)
	}
	if got := atomic.LoadInt32(&reqs); got != firstReqs {
		t.Errorf("cache hit made %d new requests, want 0 (case-insensitive key)", got-firstReqs)
	}

	// Advance past the TTL: cache must be bypassed.
	c.now = func() time.Time { return fixedNow.Add(16 * time.Minute) }
	if _, err := c.Search(context.Background(), "comcast", 10); err != nil {
		t.Fatalf("Search (expired): %v", err)
	}
	if got := atomic.LoadInt32(&reqs); got <= firstReqs {
		t.Errorf("expired cache entry was not refetched: reqs=%d", got)
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

	_, err := c.Search(context.Background(), "denver", 10)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestLooksLikePostcode(t *testing.T) {
	cases := map[string]bool{
		"80202":     true,
		"SW1A 1AA":  true,
		"denver":    false,
		"comcast":   false,
		"":          false,
		"Denver, CO": false,
	}
	for q, want := range cases {
		if got := looksLikePostcode(q); got != want {
			t.Errorf("looksLikePostcode(%q) = %v, want %v", q, got, want)
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

func TestSearchLimitCaps(t *testing.T) {
	sp := speedtestServer(t, map[string]string{"denver": denverMetroFixture}, nil)
	defer sp.Close()
	geo := geoServer(t, map[string]string{})
	defer geo.Close()

	c := NewClient()
	c.Base = sp.URL
	c.GeoBase = geo.URL

	got, err := c.Search(context.Background(), "denver", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d servers, want 1 (limit)", len(got))
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

	got, err := c.Search(context.Background(), "comcast", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d servers, want 1 (direct hit, geocode failure ignored)", len(got))
	}
	if !strings.Contains(buf.String(), "level=DEBUG") || !strings.Contains(buf.String(), "geocode failed") {
		t.Errorf("log output = %q, want a DEBUG line about the geocode failure", buf.String())
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

	if _, err := c.Search(context.Background(), "comcast", 10); err != nil {
		t.Fatalf("Search: %v", err)
	}
}
