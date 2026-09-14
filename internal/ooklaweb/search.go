// Package ooklaweb searches speedtest.net's unofficial server-search API to
// widen the Ookla server picker beyond the ~10 servers the speedtest CLI
// returns for `-L`. When the query looks like a postcode, or the direct
// search comes back thin, it also geocodes the query via Open-Meteo and
// re-searches speedtest.net using the resolved place name, sorting the
// merged results by distance from that point.
package ooklaweb

import (
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
	defaultSearchBase = "https://www.speedtest.net/api/js/servers"
	defaultGeoBase    = "https://geocoding-api.open-meteo.com/v1/search"

	// perRequestTimeout bounds one outbound HTTP call; overallTimeout
	// bounds a whole Search call, which can make up to two search
	// requests plus one geocode request.
	perRequestTimeout = 5 * time.Second
	overallTimeout    = 10 * time.Second

	cacheTTL        = 15 * time.Minute
	cacheMaxEntries = 256

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
	HTTP    *http.Client
	Base    string
	GeoBase string

	// Logger, when set, receives debug-level notes about geocode and
	// geocoded-re-search failures (both are swallowed otherwise — Search
	// still succeeds with the direct-search results). Optional; nil is
	// safe and logs nothing.
	Logger *slog.Logger

	mu    sync.Mutex
	cache map[string]cacheEntry
	now   func() time.Time
}

type cacheEntry struct {
	servers []ookla.Server
	at      time.Time
}

// NewClient returns a Client pointed at the real speedtest.net search API
// and Open-Meteo geocoder, with an HTTP client timing out at
// perRequestTimeout.
func NewClient() *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: perRequestTimeout},
		Base:    defaultSearchBase,
		GeoBase: defaultGeoBase,
		cache:   make(map[string]cacheEntry),
		now:     time.Now,
	}
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
func (c *Client) Search(ctx context.Context, q string, limit int) ([]ookla.Server, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	key := strings.ToLower(q)

	if servers, ok := c.cacheGet(key); ok {
		return capServers(servers, limit), nil
	}

	ctx, cancel := context.WithTimeout(ctx, overallTimeout)
	defer cancel()

	servers, err := c.search(ctx, q)
	if err != nil {
		return nil, err
	}

	var point *geoPoint
	if looksLikePostcode(q) || len(servers) < geocodeHitThreshold {
		gp, gerr := c.geocode(ctx, q)
		if gerr != nil {
			c.logDebug("ooklaweb: geocode failed", "query", q, "error", gerr)
		} else if gp != nil {
			point = gp
			if more, merr := c.search(ctx, gp.Name); merr != nil {
				c.logDebug("ooklaweb: geocoded re-search failed", "query", q, "place", gp.Name, "error", merr)
			} else {
				servers = mergeServers(servers, more)
			}
		}
	}

	if point != nil {
		applyDistances(servers, *point)
		sort.SliceStable(servers, func(i, j int) bool { return sortDistance(servers[i]) < sortDistance(servers[j]) })
	}

	c.cacheSet(key, servers)
	return capServers(servers, limit), nil
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

func (c *Client) cacheGet(key string) ([]ookla.Server, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || c.now().Sub(e.at) >= cacheTTL {
		return nil, false
	}
	return e.servers, true
}

func (c *Client) cacheSet(key string, servers []ookla.Server) {
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
	c.cache[key] = cacheEntry{servers: servers, at: c.now()}
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read speedtest.net search response: %w", err)
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
	var gr geoResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, fmt.Errorf("parse open-meteo geocode response: %w", err)
	}
	if len(gr.Results) == 0 {
		return nil, nil
	}
	first := gr.Results[0]
	return &geoPoint{Name: first.Name, Lat: first.Latitude, Lon: first.Longitude}, nil
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
