// Package ooklaweb searches speedtest.net's unofficial server-search API to
// widen the Ookla server picker beyond the ~10 servers the speedtest CLI
// returns for `-L`. When the query looks like a postcode, or the direct
// search comes back thin, it also geocodes the query via Open-Meteo and
// re-searches speedtest.net using the resolved place name, sorting the
// merged results by distance from that point.
package ooklaweb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/metril/speedtest-tracker/internal/engine/ookla"
)

const (
	defaultSearchBase    = "https://www.speedtest.net/api/js/servers"
	defaultGeoBase       = "https://geocoding-api.open-meteo.com/v1/search"
	defaultNominatimBase = "https://nominatim.openstreetmap.org/search"

	// defaultUserAgent is sent on every Nominatim request. Nominatim's
	// usage policy requires a real, identifying User-Agent; requests
	// without one are liable to be blocked.
	defaultUserAgent = "speedtest-tracker/dev (+https://github.com/metril/speedtest-tracker)"

	// nominatimMinInterval enforces Nominatim's usage policy of at most 1
	// request/second, independent of the result cache below (which only
	// helps once a query has already been resolved once).
	nominatimMinInterval = time.Second

	// perRequestTimeout bounds one outbound HTTP call; overallTimeout
	// bounds a whole Search call, which can make up to two search
	// requests plus a handful of geocode requests.
	perRequestTimeout = 5 * time.Second
	overallTimeout    = 10 * time.Second

	cacheTTL        = 15 * time.Minute
	cacheMaxEntries = 256

	// maxResponseBytes caps how much of an outbound response body (search
	// or geocode) is read, guarding against a misbehaving or malicious
	// upstream streaming an unbounded body.
	maxResponseBytes = 4 << 20 // 4 MiB

	// geocodeHitThreshold: a direct search returning fewer hits than this
	// is treated as "thin" and widened via geocoding.
	geocodeHitThreshold = 5

	searchRequestLimit = 50
)

// Client searches speedtest.net (and, when useful, Open-Meteo's geocoder)
// for Ookla servers. HTTP, Base and GeoBase are exported so callers and
// tests can override them; NewClient wires sane defaults pointed at the
// real services. The zero Client is not usable — construct with NewClient.
type Client struct {
	HTTP          *http.Client
	Base          string
	GeoBase       string
	NominatimBase string

	// UserAgent is sent on every Nominatim request (required by its usage
	// policy). NewClient defaults it to a generic identifying string;
	// callers that know their build version should override it, e.g.
	// "speedtest-tracker/1.2.3 (+https://github.com/metril/speedtest-tracker)".
	UserAgent string

	// Logger, when set, receives debug-level notes about geocode and
	// geocoded-re-search failures (both are swallowed otherwise — Search
	// still succeeds with the direct-search results). Optional; nil is
	// safe and logs nothing.
	Logger *slog.Logger

	mu    sync.Mutex
	cache map[string]cacheEntry
	now   func() time.Time

	sf singleflightGroup

	nomMu    sync.Mutex
	nomLast  time.Time
	nomNow   func() time.Time
	nomSleep func(ctx context.Context, d time.Duration) error
}

type cacheEntry struct {
	result SearchResult
	at     time.Time
}

// SearchRequest is Search's input: the raw query, an optional 2-letter
// lowercase country hint (used to scope postcode geocoding), and a result
// limit (<= 0 means unbounded).
type SearchRequest struct {
	Q       string
	Country string
	Limit   int
}

// SearchResult is Search's output: the merged, possibly distance-sorted
// server list, plus Near — the resolved place name (first two
// comma-separated parts of the geocoder's display name), empty when no
// geocode point was resolved.
type SearchResult struct {
	Servers []ookla.Server `json:"servers"`
	Near    string         `json:"near,omitempty"`
}

// NewClient returns a Client pointed at the real speedtest.net search API,
// Nominatim and Open-Meteo geocoders, with an HTTP client timing out at
// perRequestTimeout.
func NewClient() *Client {
	return &Client{
		HTTP:          &http.Client{Timeout: perRequestTimeout, CheckRedirect: rejectCrossHostRedirect},
		Base:          defaultSearchBase,
		GeoBase:       defaultGeoBase,
		NominatimBase: defaultNominatimBase,
		UserAgent:     defaultUserAgent,
		cache:         make(map[string]cacheEntry),
		now:           time.Now,
		nomNow:        time.Now,
		nomSleep:      ctxSleep,
	}
}

// ctxSleep is the production nomSleep: it waits for d, or returns
// ctx.Err() early if ctx is canceled first, so a canceled caller doesn't
// block the full rate-limit wait.
func ctxSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// waitNominatim blocks, if necessary, so that no two Nominatim requests
// from this Client start less than nominatimMinInterval apart — enforced
// here, independent of the result cache, since the cache only helps once
// a query has already been resolved once. It is ctx-aware: a canceled ctx
// interrupts the wait instead of blocking regardless of cancellation.
func (c *Client) waitNominatim(ctx context.Context) error {
	c.nomMu.Lock()
	if c.nomNow == nil {
		c.nomNow = time.Now
	}
	if c.nomSleep == nil {
		c.nomSleep = ctxSleep
	}
	now := c.nomNow()
	var wait time.Duration
	if !c.nomLast.IsZero() {
		wait = nominatimMinInterval - now.Sub(c.nomLast)
	}
	if wait < 0 {
		wait = 0
	}
	// Reserve the slot up front (rather than after sleeping) so concurrent
	// callers pace off each other correctly instead of all measuring wait
	// against the same stale nomLast.
	c.nomLast = now.Add(wait)
	sleep := c.nomSleep
	c.nomMu.Unlock()

	if wait <= 0 {
		return nil
	}
	return sleep(ctx, wait)
}

// rejectCrossHostRedirect is a http.Client CheckRedirect func that refuses
// to follow a redirect whose target host differs from the original
// request's host, so a compromised or misconfigured upstream can't
// redirect us at an arbitrary internal or third-party host.
func rejectCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("refusing redirect from %s to different host %s", via[0].URL.Host, req.URL.Host)
	}
	return nil
}

type geoPoint struct {
	Name string
	Lat  float64
	Lon  float64
}

// Search queries speedtest.net for q. If q looks like a postcode, or the
// direct search returns fewer than geocodeHitThreshold hits, it also
// geocodes q via Open-Meteo and re-searches using the resolved place name;
// results are merged (deduped by ID) and, when a geocode point was
// resolved, sorted by distance from it. Results are cached for cacheTTL,
// keyed by the lowercased query, in a cache capped at cacheMaxEntries
// (oldest evicted first). limit caps the number of servers returned; <= 0
// means unbounded.
//
// Concurrent calls for the same lowercased query are single-flighted: only
// one of them actually reaches speedtest.net (and, if needed, Open-Meteo);
// the rest wait for and share its result. This protects the upstream from
// a thundering herd of identical searches (e.g. several browser tabs
// typing the same query) independently of the TTL cache above, which only
// helps once a result already exists.
func (c *Client) Search(ctx context.Context, req SearchRequest) (SearchResult, error) {
	q := strings.TrimSpace(req.Q)
	if q == "" {
		return SearchResult{Servers: []ookla.Server{}}, nil
	}
	country := strings.ToLower(strings.TrimSpace(req.Country))
	key := strings.ToLower(q) + "|" + country

	if res, ok := c.cacheGet(key); ok {
		res.Servers = capServers(res.Servers, req.Limit)
		return res, nil
	}

	res, err := c.sf.Do(key, func() (SearchResult, error) {
		// Re-check: a concurrent call for the same key may have already
		// populated the cache while this call waited to become the
		// leader (or waited on another leader that has since finished).
		if res, ok := c.cacheGet(key); ok {
			return res, nil
		}
		return c.searchAndCache(q, country, key)
	})
	if err != nil {
		return SearchResult{}, err
	}
	res.Servers = capServers(res.Servers, req.Limit)
	return res, nil
}

// searchAndCache performs the actual direct-search(+geocode) flow for q
// and caches the result under key. It is only ever run once per key at a
// time, via Client.sf (see Search). It intentionally does not inherit any
// particular caller's context — a single-flighted call is shared by every
// concurrent caller, so it must not be cancelable by whichever caller
// happened to start it; overallTimeout still bounds its total duration.
func (c *Client) searchAndCache(q, country, key string) (SearchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), overallTimeout)
	defer cancel()

	servers, err := c.search(ctx, q)
	if err != nil {
		return SearchResult{}, err
	}

	var point *geoPoint
	if looksLikePostcode(q) {
		gp, gerr := c.geocodePostcode(ctx, q, country)
		if gerr != nil {
			c.logDebug("ooklaweb: nominatim geocode failed", "query", q, "country", country, "error", gerr)
		} else if gp != nil {
			point = gp
		}
	}
	if point == nil && len(servers) < geocodeHitThreshold {
		gp, gerr := c.geocode(ctx, q)
		if gerr != nil {
			c.logDebug("ooklaweb: geocode failed", "query", q, "error", gerr)
		} else if gp != nil {
			point = gp
		}
	}

	if point != nil {
		if more, merr := c.search(ctx, point.Name); merr != nil {
			c.logDebug("ooklaweb: geocoded re-search failed", "query", q, "place", point.Name, "error", merr)
		} else {
			servers = mergeServers(servers, more)
		}
		applyDistances(servers, *point)
		sort.SliceStable(servers, func(i, j int) bool { return sortDistance(servers[i]) < sortDistance(servers[j]) })
	}

	res := SearchResult{Servers: servers}
	if point != nil {
		res.Near = nearName(point.Name)
	}
	c.cacheSet(key, res)
	return res, nil
}

// nearName trims a geocoder display name down to its first two
// comma-separated parts (e.g. "Denver, Colorado, United States" ->
// "Denver, Colorado"), which is plenty to orient a user without repeating
// the whole administrative hierarchy.
func nearName(displayName string) string {
	parts := strings.Split(displayName, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, ", ")
}

func applyDistances(servers []ookla.Server, point geoPoint) {
	for i := range servers {
		if !hasDistance(servers[i]) {
			continue
		}
		servers[i].DistanceKm = haversineKm(point.Lat, point.Lon, servers[i].Lat, servers[i].Lon)
	}
}

// hasDistance reports whether a server carries coordinates a distance can
// be computed from. (0,0) is treated as "no coordinates" rather than a
// legitimate point in the Gulf of Guinea — the speedtest.net search API
// leaves lat/lon blank (which unmarshals to 0) for hits it can't place.
func hasDistance(s ookla.Server) bool {
	return s.Lat != 0 || s.Lon != 0
}

// sortDistance orders coordinate-less servers after every server with a
// known distance, instead of ranking their zero-value DistanceKm as
// nearest (see applyDistances).
func sortDistance(s ookla.Server) float64 {
	if !hasDistance(s) {
		return math.Inf(1)
	}
	return s.DistanceKm
}

func capServers(servers []ookla.Server, limit int) []ookla.Server {
	if limit > 0 && len(servers) > limit {
		out := make([]ookla.Server, limit)
		copy(out, servers)
		return out
	}
	return servers
}

func mergeServers(a, b []ookla.Server) []ookla.Server {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]ookla.Server, 0, len(a)+len(b))
	for _, list := range [][]ookla.Server{a, b} {
		for _, s := range list {
			if seen[s.ID] {
				continue
			}
			seen[s.ID] = true
			out = append(out, s)
		}
	}
	return out
}

func (c *Client) cacheGet(key string) (SearchResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || c.now().Sub(e.at) >= cacheTTL {
		return SearchResult{}, false
	}
	return e.result, true
}

func (c *Client) cacheSet(key string, result SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cache == nil {
		c.cache = make(map[string]cacheEntry)
	}
	if _, exists := c.cache[key]; !exists && len(c.cache) >= cacheMaxEntries {
		var oldestKey string
		var oldestAt time.Time
		first := true
		for k, e := range c.cache {
			if first || e.at.Before(oldestAt) {
				oldestKey, oldestAt, first = k, e.at, false
			}
		}
		delete(c.cache, oldestKey)
	}
	c.cache[key] = cacheEntry{result: result, at: c.now()}
}

func (c *Client) logDebug(msg string, args ...any) {
	if c.Logger != nil {
		c.Logger.Debug(msg, args...)
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// search performs one speedtest.net text search for q.
func (c *Client) search(ctx context.Context, q string) ([]ookla.Server, error) {
	ctx, cancel := context.WithTimeout(ctx, perRequestTimeout)
	defer cancel()

	u, err := url.Parse(c.Base)
	if err != nil {
		return nil, fmt.Errorf("parse search base: %w", err)
	}
	qs := u.Query()
	qs.Set("engine", "js")
	qs.Set("search", q)
	qs.Set("limit", strconv.Itoa(searchRequestLimit))
	u.RawQuery = qs.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("speedtest.net search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("speedtest.net search: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read speedtest.net search response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("speedtest.net search response exceeds %d bytes", maxResponseBytes)
	}
	var raw []rawServer
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse speedtest.net search response: %w", err)
	}
	out := make([]ookla.Server, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.toServer())
	}
	return out, nil
}

type geoResponse struct {
	Results []struct {
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"results"`
}

// geocode resolves q to a place via Open-Meteo, returning the first (best)
// match, or a nil point when there are no results.
func (c *Client) geocode(ctx context.Context, q string) (*geoPoint, error) {
	ctx, cancel := context.WithTimeout(ctx, perRequestTimeout)
	defer cancel()

	u, err := url.Parse(c.GeoBase)
	if err != nil {
		return nil, fmt.Errorf("parse geocode base: %w", err)
	}
	qs := u.Query()
	qs.Set("name", q)
	qs.Set("count", "5")
	u.RawQuery = qs.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("open-meteo geocode: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo geocode: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read open-meteo geocode response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("open-meteo geocode response exceeds %d bytes", maxResponseBytes)
	}
	var gr geoResponse
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&gr); err != nil {
		return nil, fmt.Errorf("parse open-meteo geocode response: %w", err)
	}
	if len(gr.Results) == 0 {
		return nil, nil
	}
	first := gr.Results[0]
	return &geoPoint{Name: first.Name, Lat: first.Latitude, Lon: first.Longitude}, nil
}

// nominatimResult is one hit from Nominatim's jsonv2 search response.
// lat/lon come back as JSON strings.
type nominatimResult struct {
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
}

// geocodePostcode resolves a postcode-shaped query q to a place via
// Nominatim, trying progressively looser queries: postalcode+countrycodes
// (when country is set), then postalcode alone, then a free-form q=
// search (still scoped to country when set). It returns a nil point
// (with no error) when none of those find a match — the caller falls
// back to the existing Open-Meteo geocoder in that case.
func (c *Client) geocodePostcode(ctx context.Context, q, country string) (*geoPoint, error) {
	if country != "" {
		gp, err := c.nominatimSearch(ctx, url.Values{"postalcode": {q}, "countrycodes": {country}})
		if err != nil {
			return nil, err
		}
		if gp != nil {
			return gp, nil
		}
	}
	gp, err := c.nominatimSearch(ctx, url.Values{"postalcode": {q}})
	if err != nil {
		return nil, err
	}
	if gp != nil {
		return gp, nil
	}
	params := url.Values{"q": {q}}
	if country != "" {
		params.Set("countrycodes", country)
	}
	return c.nominatimSearch(ctx, params)
}

// nominatimSearch performs one rate-limited Nominatim search request with
// the given query params (format=jsonv2&limit=1 are added automatically),
// returning the first (best) match, or a nil point when there are none.
func (c *Client) nominatimSearch(ctx context.Context, params url.Values) (*geoPoint, error) {
	// Rate-limit wait happens against the caller's own ctx, before the
	// per-request timeout below starts — otherwise a rate-limit wait
	// close to perRequestTimeout would eat into the budget meant for the
	// actual HTTP round trip, and a canceled caller ctx wouldn't
	// interrupt the wait at all.
	if err := c.waitNominatim(ctx); err != nil {
		return nil, fmt.Errorf("nominatim rate limit wait: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, perRequestTimeout)
	defer cancel()

	u, err := url.Parse(c.nominatimBase())
	if err != nil {
		return nil, fmt.Errorf("parse nominatim base: %w", err)
	}
	qs := u.Query()
	for k, vs := range params {
		for _, v := range vs {
			qs.Set(k, v)
		}
	}
	qs.Set("format", "jsonv2")
	qs.Set("limit", "1")
	u.RawQuery = qs.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("nominatim search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nominatim search: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read nominatim search response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("nominatim search response exceeds %d bytes", maxResponseBytes)
	}
	var results []nominatimResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("parse nominatim search response: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	first := results[0]
	lat, err := strconv.ParseFloat(first.Lat, 64)
	if err != nil {
		return nil, fmt.Errorf("parse nominatim lat %q: %w", first.Lat, err)
	}
	lon, err := strconv.ParseFloat(first.Lon, 64)
	if err != nil {
		return nil, fmt.Errorf("parse nominatim lon %q: %w", first.Lon, err)
	}
	return &geoPoint{Name: first.DisplayName, Lat: lat, Lon: lon}, nil
}

func (c *Client) nominatimBase() string {
	if c.NominatimBase != "" {
		return c.NominatimBase
	}
	return defaultNominatimBase
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return defaultUserAgent
}

// rawServer is one speedtest.net search hit. id/lat/lon/distance come back
// as either JSON numbers or JSON strings depending on the field and Ookla's
// mood, so they use flexible unmarshalers.
type rawServer struct {
	ID      flexID  `json:"id"`
	Name    string  `json:"name"`
	Country string  `json:"country"`
	Sponsor string  `json:"sponsor"`
	Host    string  `json:"host"`
	Lat     flexNum `json:"lat"`
	Lon     flexNum `json:"lon"`
}

// toServer maps a speedtest.net hit onto ookla.Server, matching the local
// `speedtest -L` shape: Name is the ISP/sponsor (Server.Name in the CLI
// list), Location is the place ("Denver, CO"). Sponsor duplicates Name
// explicitly so the picker can show it even if callers only read the new
// field.
func (r rawServer) toServer() ookla.Server {
	return ookla.Server{
		ID:       string(r.ID),
		Name:     r.Sponsor,
		Location: r.Name,
		Country:  r.Country,
		Host:     r.Host,
		Sponsor:  r.Sponsor,
		Lat:      float64(r.Lat),
		Lon:      float64(r.Lon),
	}
}

type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	*f = flexID(strings.Trim(string(b), `"`))
	return nil
}

type flexNum float64

func (f *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = flexNum(v)
	return nil
}

// postcodeRe matches short alphanumeric tokens (with optional internal
// spaces/hyphens) typical of postal codes ("80202", "SW1A 1AA"), as
// distinct from city/sponsor names.
var postcodeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 -]{0,9}$`)

// looksLikePostcode reports whether q looks like a postal code rather than
// a place or sponsor name: short, alphanumeric, and containing a digit.
func looksLikePostcode(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" || !postcodeRe.MatchString(q) {
		return false
	}
	for _, r := range q {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

const earthRadiusKm = 6371.0

// haversineKm returns the great-circle distance between two lat/lon points
// in kilometers.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := degToRad(lat2 - lat1)
	dLon := degToRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(degToRad(lat1))*math.Cos(degToRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

func degToRad(deg float64) float64 { return deg * math.Pi / 180 }
