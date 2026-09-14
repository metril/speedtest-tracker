// Package settings is a typed accessor over the settings table, with
// General, Engines and Integrations sections implemented and Auth and
// Notifications arriving in later milestones.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/metril/speedtest-tracker/internal/store"
)

// General is the General settings section.
type General struct {
	BaseURL                       string `json:"base_url"`
	Timezone                      string `json:"timezone"`
	Units                         string `json:"units"`
	LogLevel                      string `json:"log_level"`
	RetentionDaysResults          int    `json:"retention_days_results"`
	RetentionDaysRuns             int    `json:"retention_days_runs"`
	RetentionPruneIntervalMinutes int    `json:"retention_prune_interval_minutes"`
}

// Keys of the General section.
const (
	KeyBaseURL                       = "general.base_url"
	KeyTimezone                      = "general.timezone"
	KeyUnits                         = "general.units"
	KeyLogLevel                      = "general.log_level"
	KeyRetentionDaysResults          = "general.retention_days_results"
	KeyRetentionDaysRuns             = "general.retention_days_runs"
	KeyRetentionPruneIntervalMinutes = "general.retention_prune_interval_minutes"
)

// MaskedSecret is what the API sends instead of a stored secret, and the
// sentinel a client sends back to mean "keep the stored value".
const MaskedSecret = "***"

// New seeds defaults with ON CONFLICT DO NOTHING: a default value change
// (like the runs-retention default below, raised from 30 to 90) only takes
// effect for databases created from now on. Existing installs keep whatever
// value was already stored, seeded or set.
var defaults = map[string]any{
	KeyBaseURL:                       "",
	KeyTimezone:                      "UTC",
	KeyUnits:                         "Mbps",
	KeyLogLevel:                      "info",
	KeyRetentionDaysResults:          90,
	KeyRetentionDaysRuns:             90,
	KeyRetentionPruneIntervalMinutes: 60,

	KeySpeedtestBin:             "speedtest",
	KeyIperf3Bin:                "iperf3",
	KeyOoklaAcceptLicense:       true,
	KeyOoklaAcceptGDPR:          true,
	KeyServerListTTLSeconds:     86400,
	KeyDefaultOoklaOptions:      json.RawMessage(`{}`),
	KeyDefaultCloudflareOptions: json.RawMessage(`{}`),
	KeyDefaultIperf3Options:     json.RawMessage(`{}`),

	KeyVMEnabled:      false,
	KeyVMURL:          "",
	KeyVMAuthHeader:   "",
	KeyVMExtraLabels:  map[string]string{},
	KeyVLEnabled:      false,
	KeyVLURL:          "",
	KeyVLAuthHeader:   "",
	KeyVLStreamFields: map[string]string{},
	KeyMetricsEnabled: false,
}

// Integrations is the Integrations settings section: the VictoriaMetrics
// and VictoriaLogs clients plus the Prometheus /metrics endpoint.
type Integrations struct {
	VMEnabled      bool              `json:"vm_enabled"`
	VMURL          string            `json:"vm_url"`
	VMAuthHeader   string            `json:"vm_auth_header"`
	VMExtraLabels  map[string]string `json:"vm_extra_labels"`
	VLEnabled      bool              `json:"vl_enabled"`
	VLURL          string            `json:"vl_url"`
	VLAuthHeader   string            `json:"vl_auth_header"`
	VLStreamFields map[string]string `json:"vl_stream_fields"`
	MetricsEnabled bool              `json:"metrics_enabled"`
}

// Keys of the Integrations section.
const (
	KeyVMEnabled      = "integrations.vm_enabled"
	KeyVMURL          = "integrations.vm_url"
	KeyVMAuthHeader   = "integrations.vm_auth_header"
	KeyVMExtraLabels  = "integrations.vm_extra_labels"
	KeyVLEnabled      = "integrations.vl_enabled"
	KeyVLURL          = "integrations.vl_url"
	KeyVLAuthHeader   = "integrations.vl_auth_header"
	KeyVLStreamFields = "integrations.vl_stream_fields"
	KeyMetricsEnabled = "integrations.metrics_enabled"
)

// Engines is the Engines settings section: external binary paths, Ookla
// consent flags, server-list cache TTL and per-engine default options.
type Engines struct {
	SpeedtestBin             string          `json:"speedtest_bin"`
	Iperf3Bin                string          `json:"iperf3_bin"`
	OoklaAcceptLicense       bool            `json:"ookla_accept_license"`
	OoklaAcceptGDPR          bool            `json:"ookla_accept_gdpr"`
	ServerListTTLSeconds     int             `json:"server_list_ttl_seconds"`
	DefaultOoklaOptions      json.RawMessage `json:"default_ookla_options"`
	DefaultCloudflareOptions json.RawMessage `json:"default_cloudflare_options"`
	DefaultIperf3Options     json.RawMessage `json:"default_iperf3_options"`
}

// Keys of the Engines section.
const (
	KeySpeedtestBin             = "engines.speedtest_bin"
	KeyIperf3Bin                = "engines.iperf3_bin"
	KeyOoklaAcceptLicense       = "engines.ookla_accept_license"
	KeyOoklaAcceptGDPR          = "engines.ookla_accept_gdpr"
	KeyServerListTTLSeconds     = "engines.server_list_ttl_seconds"
	KeyDefaultOoklaOptions      = "engines.default_ookla_options"
	KeyDefaultCloudflareOptions = "engines.default_cloudflare_options"
	KeyDefaultIperf3Options     = "engines.default_iperf3_options"
)

// Store reads and writes settings and notifies subscribers on change.
type Store struct {
	db *store.Store

	mu   sync.Mutex
	subs map[int]chan string
	next int
}

// New returns a Store and seeds any General key that is not yet present.
func New(ctx context.Context, db *store.Store) (*Store, error) {
	s := &Store{db: db, subs: map[int]chan string{}}
	for key, val := range defaults {
		encoded, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("encode default %s: %w", key, err)
		}
		if _, err := db.Write.ExecContext(ctx,
			`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`,
			key, string(encoded)); err != nil {
			return nil, fmt.Errorf("seed %s: %w", key, err)
		}
	}
	return s, nil
}

// Get returns the raw JSON value for key. ok is false if the key is unset.
func (s *Store) Get(ctx context.Context, key string) (json.RawMessage, bool, error) {
	var v string
	err := s.db.Read.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get %s: %w", key, err)
	}
	return json.RawMessage(v), true, nil
}

// Set JSON-encodes value, stores it under key and notifies subscribers.
func (s *Store) Set(ctx context.Context, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", key, err)
	}
	if _, err := s.db.Write.ExecContext(ctx, `
		INSERT INTO settings(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET
			value=excluded.value,
			updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')`,
		key, string(encoded)); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	s.notify(key)
	return nil
}

// General returns the General section, falling back to the seeded defaults
// for any key that is missing (mirroring Engines).
func (s *Store) General(ctx context.Context) (General, error) {
	g := General{}
	targets := map[string]any{
		KeyBaseURL:                       &g.BaseURL,
		KeyTimezone:                      &g.Timezone,
		KeyUnits:                         &g.Units,
		KeyLogLevel:                      &g.LogLevel,
		KeyRetentionDaysResults:          &g.RetentionDaysResults,
		KeyRetentionDaysRuns:             &g.RetentionDaysRuns,
		KeyRetentionPruneIntervalMinutes: &g.RetentionPruneIntervalMinutes,
	}
	for key, dest := range targets {
		raw, ok, err := s.Get(ctx, key)
		if err != nil {
			return General{}, err
		}
		if !ok {
			encoded, err := json.Marshal(defaults[key])
			if err != nil {
				return General{}, err
			}
			raw = encoded
		}
		if err := json.Unmarshal(raw, dest); err != nil {
			return General{}, fmt.Errorf("decode %s: %w", key, err)
		}
	}
	return g, nil
}

// Engines returns the Engines section, falling back to the seeded defaults
// for any key that is missing.
func (s *Store) Engines(ctx context.Context) (Engines, error) {
	var e Engines
	for _, f := range []struct {
		key string
		dst any
	}{
		{KeySpeedtestBin, &e.SpeedtestBin},
		{KeyIperf3Bin, &e.Iperf3Bin},
		{KeyOoklaAcceptLicense, &e.OoklaAcceptLicense},
		{KeyOoklaAcceptGDPR, &e.OoklaAcceptGDPR},
		{KeyServerListTTLSeconds, &e.ServerListTTLSeconds},
		{KeyDefaultOoklaOptions, &e.DefaultOoklaOptions},
		{KeyDefaultCloudflareOptions, &e.DefaultCloudflareOptions},
		{KeyDefaultIperf3Options, &e.DefaultIperf3Options},
	} {
		raw, ok, err := s.Get(ctx, f.key)
		if err != nil {
			return Engines{}, err
		}
		if !ok {
			encoded, err := json.Marshal(defaults[f.key])
			if err != nil {
				return Engines{}, err
			}
			raw = encoded
		}
		if err := json.Unmarshal(raw, f.dst); err != nil {
			return Engines{}, fmt.Errorf("decode %s: %w", f.key, err)
		}
	}
	return e, nil
}

// Integrations returns the Integrations section, falling back to the seeded
// defaults for any key that is missing.
func (s *Store) Integrations(ctx context.Context) (Integrations, error) {
	var i Integrations
	for _, f := range []struct {
		key string
		dst any
	}{
		{KeyVMEnabled, &i.VMEnabled},
		{KeyVMURL, &i.VMURL},
		{KeyVMAuthHeader, &i.VMAuthHeader},
		{KeyVMExtraLabels, &i.VMExtraLabels},
		{KeyVLEnabled, &i.VLEnabled},
		{KeyVLURL, &i.VLURL},
		{KeyVLAuthHeader, &i.VLAuthHeader},
		{KeyVLStreamFields, &i.VLStreamFields},
		{KeyMetricsEnabled, &i.MetricsEnabled},
	} {
		raw, ok, err := s.Get(ctx, f.key)
		if err != nil {
			return Integrations{}, err
		}
		if !ok {
			encoded, err := json.Marshal(defaults[f.key])
			if err != nil {
				return Integrations{}, err
			}
			raw = encoded
		}
		if err := json.Unmarshal(raw, f.dst); err != nil {
			return Integrations{}, fmt.Errorf("decode %s: %w", f.key, err)
		}
	}
	if i.VMExtraLabels == nil {
		i.VMExtraLabels = map[string]string{}
	}
	if i.VLStreamFields == nil {
		i.VLStreamFields = map[string]string{}
	}
	return i, nil
}

// Subscribe returns a channel of changed keys and a cancel function. Sends
// are non-blocking: a subscriber that falls behind loses notifications
// rather than stalling the writer.
func (s *Store) Subscribe() (<-chan string, func()) {
	ch := make(chan string, 16)
	s.mu.Lock()
	id := s.next
	s.next++
	s.subs[id] = ch
	s.mu.Unlock()

	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if c, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(c)
		}
	}
}

func (s *Store) notify(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- key:
		default:
		}
	}
}
