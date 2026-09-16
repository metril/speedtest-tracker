// Package settings is a typed accessor over the settings table, with
// General, Engines, Integrations, Notifications and Auth sections
// implemented.
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

	// SLADownloadMbps/SLAUploadMbps are the general SLA plan speeds used
	// by the /stats/summary sla_compliance computation, for any target
	// that doesn't set its own override via Thresholds. nil means no
	// general plan.
	SLADownloadMbps *float64 `json:"sla_download_mbps,omitempty"`
	SLAUploadMbps   *float64 `json:"sla_upload_mbps,omitempty"`
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
	KeySLADownloadMbps               = "general.sla_download_mbps"
	KeySLAUploadMbps                 = "general.sla_upload_mbps"
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
	KeySLADownloadMbps:               (*float64)(nil),
	KeySLAUploadMbps:                 (*float64)(nil),

	KeySpeedtestBin:             "speedtest",
	KeyIperf3Bin:                "iperf3",
	KeyOoklaAcceptLicense:       true,
	KeyOoklaAcceptGDPR:          true,
	KeyServerListTTLSeconds:     86400,
	KeyDefaultOoklaOptions:      json.RawMessage(`{}`),
	KeyDefaultCloudflareOptions: json.RawMessage(`{}`),
	KeyDefaultIperf3Options:     json.RawMessage(`{}`),
	KeyIperf3ListURL:            defaultIperf3ListURL,

	KeyVMEnabled:         false,
	KeyVMURL:             "",
	KeyVMAuthHeader:      "",
	KeyVMAuthType:        ExportAuthNone,
	KeyVMAuthUsername:    "",
	KeyVMAuthPassword:    "",
	KeyVMAuthToken:       "",
	KeyVMAuthHeaderName:  "",
	KeyVMAuthHeaderValue: "",
	KeyVMExtraLabels:     map[string]string{},
	KeyVLEnabled:         false,
	KeyVLURL:             "",
	KeyVLAuthHeader:      "",
	KeyVLAuthType:        ExportAuthNone,
	KeyVLAuthUsername:    "",
	KeyVLAuthPassword:    "",
	KeyVLAuthToken:       "",
	KeyVLAuthHeaderName:  "",
	KeyVLAuthHeaderValue: "",
	KeyVLStreamFields:    map[string]string{},
	KeyMetricsEnabled:    false,

	KeyNotifyEnabled:           false,
	KeyNotifyChannels:          []Channel{},
	KeyNotifyDefaultThresholds: Thresholds{},
	KeyNotifyCooldownMinutes:   60,
	KeyNotifyQuietStart:        "",
	KeyNotifyQuietEnd:          "",
	KeyNotifyRecovery:          true,

	KeyAuthMode:            AuthModeOpen,
	KeyAuthUserHeader:      "Remote-User",
	KeyAuthGroupsHeader:    "Remote-Groups",
	KeyAuthGroupsSeparator: ",",
	KeyAuthTrustedProxies:  []string{},
	KeyAuthAdminGroup:      "",
	KeyAuthAllowTokens:     false,

	KeyAuthOIDCIssuer:          "",
	KeyAuthOIDCClientID:        "",
	KeyAuthOIDCClientSecret:    "",
	KeyAuthOIDCRedirectBaseURL: "",
	KeyAuthOIDCScopes:          []string{},
	KeyAuthOIDCGroupsClaim:     "groups",
	KeyAuthOIDCAllowedGroups:   []string{},
	KeyAuthOIDCAllowedEmails:   []string{},
	KeyAuthOIDCDisplayClaim:    OIDCDisplayClaimName,
	KeyAuthSessionTTLHours:     24,
}

// Integrations is the Integrations settings section: the VictoriaMetrics
// and VictoriaLogs clients plus the Prometheus /metrics endpoint.
type Integrations struct {
	VMEnabled         bool              `json:"vm_enabled"`
	VMURL             string            `json:"vm_url"`
	VMAuthHeader      string            `json:"vm_auth_header"`
	VMAuthType        string            `json:"vm_auth_type"`
	VMAuthUsername    string            `json:"vm_auth_username"`
	VMAuthPassword    string            `json:"vm_auth_password"`
	VMAuthToken       string            `json:"vm_auth_token"`
	VMAuthHeaderName  string            `json:"vm_auth_header_name"`
	VMAuthHeaderValue string            `json:"vm_auth_header_value"`
	VMExtraLabels     map[string]string `json:"vm_extra_labels"`
	VLEnabled         bool              `json:"vl_enabled"`
	VLURL             string            `json:"vl_url"`
	VLAuthHeader      string            `json:"vl_auth_header"`
	VLAuthType        string            `json:"vl_auth_type"`
	VLAuthUsername    string            `json:"vl_auth_username"`
	VLAuthPassword    string            `json:"vl_auth_password"`
	VLAuthToken       string            `json:"vl_auth_token"`
	VLAuthHeaderName  string            `json:"vl_auth_header_name"`
	VLAuthHeaderValue string            `json:"vl_auth_header_value"`
	VLStreamFields    map[string]string `json:"vl_stream_fields"`
	MetricsEnabled    bool              `json:"metrics_enabled"`
}

// Keys of the Integrations section.
const (
	KeyVMEnabled         = "integrations.vm_enabled"
	KeyVMURL             = "integrations.vm_url"
	KeyVMAuthHeader      = "integrations.vm_auth_header"
	KeyVMAuthType        = "integrations.vm_auth_type"
	KeyVMAuthUsername    = "integrations.vm_auth_username"
	KeyVMAuthPassword    = "integrations.vm_auth_password"
	KeyVMAuthToken       = "integrations.vm_auth_token"
	KeyVMAuthHeaderName  = "integrations.vm_auth_header_name"
	KeyVMAuthHeaderValue = "integrations.vm_auth_header_value"
	KeyVMExtraLabels     = "integrations.vm_extra_labels"
	KeyVLEnabled         = "integrations.vl_enabled"
	KeyVLURL             = "integrations.vl_url"
	KeyVLAuthHeader      = "integrations.vl_auth_header"
	KeyVLAuthType        = "integrations.vl_auth_type"
	KeyVLAuthUsername    = "integrations.vl_auth_username"
	KeyVLAuthPassword    = "integrations.vl_auth_password"
	KeyVLAuthToken       = "integrations.vl_auth_token"
	KeyVLAuthHeaderName  = "integrations.vl_auth_header_name"
	KeyVLAuthHeaderValue = "integrations.vl_auth_header_value"
	KeyVLStreamFields    = "integrations.vl_stream_fields"
	KeyMetricsEnabled    = "integrations.metrics_enabled"
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

	// Iperf3ListURL is the export.iperf3serverlist.net-shaped JSON feed
	// internal/iperf3list.Refresher polls to keep the public iperf3
	// server picker populated. Empty disables the refresher entirely
	// (see internal/iperf3list): no periodic fetch, and POST
	// /iperf3/servers/refresh answers 409.
	Iperf3ListURL string `json:"iperf3_list_url"`
}

// defaultIperf3ListURL is the seeded default for Iperf3ListURL.
const defaultIperf3ListURL = "https://export.iperf3serverlist.net/listed_iperf3_servers.json"

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
	KeyIperf3ListURL            = "engines.iperf3_list_url"
)

// Thresholds is one set of alerting limits. Every field is a pointer so a
// per-target document can leave a field unset and inherit the global
// default rather than meaning "zero".
type Thresholds struct {
	DownloadMbpsMin *float64 `json:"download_mbps_min,omitempty"`
	UploadMbpsMin   *float64 `json:"upload_mbps_min,omitempty"`
	PingMsMax       *float64 `json:"ping_ms_max,omitempty"`
	JitterMsMax     *float64 `json:"jitter_ms_max,omitempty"`
	LossPctMax      *float64 `json:"loss_pct_max,omitempty"`
	NotifyOnFailure *bool    `json:"notify_on_failure,omitempty"`
	NotifyAlways    *bool    `json:"notify_always,omitempty"`

	// SLADownloadMbps/SLAUploadMbps override the general SLA plan
	// (General.SLADownloadMbps/SLAUploadMbps) for this target only, per
	// field independently. nil means "inherit the general plan for this
	// field", not "no SLA".
	SLADownloadMbps *float64 `json:"sla_download_mbps,omitempty"`
	SLAUploadMbps   *float64 `json:"sla_upload_mbps,omitempty"`
}

// Channel is one notification destination. Which fields matter depends on
// Type: "webhook" uses URL and Headers; "apprise" uses URLs — one or more
// github.com/unraid/apprise-go target URLs, e.g. "ntfy://host/topic" or
// "discord://webhook_id/webhook_token" — and Tags. URL, Token and Priority
// are unused for apprise; they only remain on the struct so stored data
// from before the ntfy channel type was removed still decodes.
type Channel struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Name     string            `json:"name"`
	Enabled  bool              `json:"enabled"`
	URL      string            `json:"url"`
	Token    string            `json:"token,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Priority string            `json:"priority,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	URLs     []string          `json:"urls,omitempty"`
}

// Notifications is the Notifications settings section.
type Notifications struct {
	Enabled           bool       `json:"enabled"`
	Channels          []Channel  `json:"channels"`
	DefaultThresholds Thresholds `json:"default_thresholds"`
	CooldownMinutes   int        `json:"cooldown_minutes"`
	QuietHoursStart   string     `json:"quiet_hours_start"`
	QuietHoursEnd     string     `json:"quiet_hours_end"`
	NotifyRecovery    bool       `json:"notify_recovery"`
}

// Keys of the Notifications section.
const (
	KeyNotifyEnabled           = "notifications.enabled"
	KeyNotifyChannels          = "notifications.channels"
	KeyNotifyDefaultThresholds = "notifications.default_thresholds"
	KeyNotifyCooldownMinutes   = "notifications.cooldown_minutes"
	KeyNotifyQuietStart        = "notifications.quiet_hours_start"
	KeyNotifyQuietEnd          = "notifications.quiet_hours_end"
	KeyNotifyRecovery          = "notifications.notify_recovery"
)

// Auth is the Auth settings section. Mode open means every request is
// allowed; forward_auth trusts identity headers, but only from a peer
// inside TrustedProxies; token requires a bearer API token. AllowTokens
// additionally accepts bearer tokens while in forward_auth mode, which is
// how scripts and Home Assistant talk to an SSO-protected instance.
type Auth struct {
	Mode            string   `json:"mode"`
	UserHeader      string   `json:"user_header"`
	GroupsHeader    string   `json:"groups_header"`
	GroupsSeparator string   `json:"groups_separator"`
	TrustedProxies  []string `json:"trusted_proxies"`
	AdminGroup      string   `json:"admin_group"`
	AllowTokens     bool     `json:"allow_tokens"`

	// OIDC* configure the oidc auth mode: OpenID Connect login against an
	// external provider. OIDCScopes/OIDCAllowedGroups/OIDCAllowedEmails
	// nil-normalise to empty like TrustedProxies.
	OIDCIssuer          string   `json:"oidc_issuer"`
	OIDCClientID        string   `json:"oidc_client_id"`
	OIDCClientSecret    string   `json:"oidc_client_secret"`
	OIDCRedirectBaseURL string   `json:"oidc_redirect_base_url"`
	OIDCScopes          []string `json:"oidc_scopes"`
	OIDCGroupsClaim     string   `json:"oidc_groups_claim"`
	OIDCAllowedGroups   []string `json:"oidc_allowed_groups"`
	OIDCAllowedEmails   []string `json:"oidc_allowed_emails"`

	// OIDCDisplayClaim selects which OIDC claim /auth/me reports as the
	// caller's display name: OIDCDisplayClaimName, OIDCDisplayClaimUsername
	// or OIDCDisplayClaimEmail.
	OIDCDisplayClaim string `json:"oidc_display_claim"`

	// SessionTTLHours is how long an OIDC-established session lasts before
	// re-authentication is required.
	SessionTTLHours int `json:"session_ttl_hours"`
}

// Auth modes. The values are persisted in the settings table, so do not
// rename them without a migration.
const (
	AuthModeOpen    = "open"
	AuthModeForward = "forward_auth"
	AuthModeToken   = "token"
	AuthModeOIDC    = "oidc"
)

// OIDC display-claim values, selecting which claim /auth/me reports as
// the caller's display name.
const (
	OIDCDisplayClaimName     = "name"
	OIDCDisplayClaimUsername = "preferred_username"
	OIDCDisplayClaimEmail    = "email"
)

// Keys of the Auth section.
const (
	KeyAuthMode            = "auth.mode"
	KeyAuthUserHeader      = "auth.user_header"
	KeyAuthGroupsHeader    = "auth.groups_header"
	KeyAuthGroupsSeparator = "auth.groups_separator"
	KeyAuthTrustedProxies  = "auth.trusted_proxies"
	KeyAuthAdminGroup      = "auth.admin_group"
	KeyAuthAllowTokens     = "auth.allow_tokens"

	KeyAuthOIDCIssuer          = "auth.oidc_issuer"
	KeyAuthOIDCClientID        = "auth.oidc_client_id"
	KeyAuthOIDCClientSecret    = "auth.oidc_client_secret"
	KeyAuthOIDCRedirectBaseURL = "auth.oidc_redirect_base_url"
	KeyAuthOIDCScopes          = "auth.oidc_scopes"
	KeyAuthOIDCGroupsClaim     = "auth.oidc_groups_claim"
	KeyAuthOIDCAllowedGroups   = "auth.oidc_allowed_groups"
	KeyAuthOIDCAllowedEmails   = "auth.oidc_allowed_emails"
	KeyAuthOIDCDisplayClaim    = "auth.oidc_display_claim"
	KeyAuthSessionTTLHours     = "auth.session_ttl_hours"
)

// Store reads and writes settings and notifies subscribers on change.
type Store struct {
	db *store.Store

	mu     sync.Mutex
	subs   map[int]chan string
	next   int
	locked map[string]bool
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
		KeySLADownloadMbps:               &g.SLADownloadMbps,
		KeySLAUploadMbps:                 &g.SLAUploadMbps,
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
		{KeyIperf3ListURL, &e.Iperf3ListURL},
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
		{KeyVMAuthType, &i.VMAuthType},
		{KeyVMAuthUsername, &i.VMAuthUsername},
		{KeyVMAuthPassword, &i.VMAuthPassword},
		{KeyVMAuthToken, &i.VMAuthToken},
		{KeyVMAuthHeaderName, &i.VMAuthHeaderName},
		{KeyVMAuthHeaderValue, &i.VMAuthHeaderValue},
		{KeyVMExtraLabels, &i.VMExtraLabels},
		{KeyVLEnabled, &i.VLEnabled},
		{KeyVLURL, &i.VLURL},
		{KeyVLAuthHeader, &i.VLAuthHeader},
		{KeyVLAuthType, &i.VLAuthType},
		{KeyVLAuthUsername, &i.VLAuthUsername},
		{KeyVLAuthPassword, &i.VLAuthPassword},
		{KeyVLAuthToken, &i.VLAuthToken},
		{KeyVLAuthHeaderName, &i.VLAuthHeaderName},
		{KeyVLAuthHeaderValue, &i.VLAuthHeaderValue},
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

// Notifications returns the Notifications section, falling back to the
// seeded defaults for any key that is missing. New seeds keys with
// ON CONFLICT DO NOTHING, so these defaults only apply to databases that
// have not seen the keys before; existing installs keep whatever value
// they already had.
func (s *Store) Notifications(ctx context.Context) (Notifications, error) {
	var n Notifications
	for _, f := range []struct {
		key string
		dst any
	}{
		{KeyNotifyEnabled, &n.Enabled},
		{KeyNotifyChannels, &n.Channels},
		{KeyNotifyDefaultThresholds, &n.DefaultThresholds},
		{KeyNotifyCooldownMinutes, &n.CooldownMinutes},
		{KeyNotifyQuietStart, &n.QuietHoursStart},
		{KeyNotifyQuietEnd, &n.QuietHoursEnd},
		{KeyNotifyRecovery, &n.NotifyRecovery},
	} {
		raw, ok, err := s.Get(ctx, f.key)
		if err != nil {
			return Notifications{}, err
		}
		if !ok {
			encoded, err := json.Marshal(defaults[f.key])
			if err != nil {
				return Notifications{}, err
			}
			raw = encoded
		}
		if err := json.Unmarshal(raw, f.dst); err != nil {
			return Notifications{}, fmt.Errorf("decode %s: %w", f.key, err)
		}
	}
	if n.Channels == nil {
		n.Channels = []Channel{}
	}
	if n.CooldownMinutes < 1 {
		n.CooldownMinutes = 1
	}
	return n, nil
}

// Auth returns the Auth section, falling back to the seeded defaults for
// any key that is missing.
func (s *Store) Auth(ctx context.Context) (Auth, error) {
	var a Auth
	for _, f := range []struct {
		key string
		dst any
	}{
		{KeyAuthMode, &a.Mode},
		{KeyAuthUserHeader, &a.UserHeader},
		{KeyAuthGroupsHeader, &a.GroupsHeader},
		{KeyAuthGroupsSeparator, &a.GroupsSeparator},
		{KeyAuthTrustedProxies, &a.TrustedProxies},
		{KeyAuthAdminGroup, &a.AdminGroup},
		{KeyAuthAllowTokens, &a.AllowTokens},
		{KeyAuthOIDCIssuer, &a.OIDCIssuer},
		{KeyAuthOIDCClientID, &a.OIDCClientID},
		{KeyAuthOIDCClientSecret, &a.OIDCClientSecret},
		{KeyAuthOIDCRedirectBaseURL, &a.OIDCRedirectBaseURL},
		{KeyAuthOIDCScopes, &a.OIDCScopes},
		{KeyAuthOIDCGroupsClaim, &a.OIDCGroupsClaim},
		{KeyAuthOIDCAllowedGroups, &a.OIDCAllowedGroups},
		{KeyAuthOIDCAllowedEmails, &a.OIDCAllowedEmails},
		{KeyAuthOIDCDisplayClaim, &a.OIDCDisplayClaim},
		{KeyAuthSessionTTLHours, &a.SessionTTLHours},
	} {
		raw, ok, err := s.Get(ctx, f.key)
		if err != nil {
			return Auth{}, err
		}
		if !ok {
			encoded, err := json.Marshal(defaults[f.key])
			if err != nil {
				return Auth{}, err
			}
			raw = encoded
		}
		if err := json.Unmarshal(raw, f.dst); err != nil {
			return Auth{}, fmt.Errorf("decode %s: %w", f.key, err)
		}
	}
	if a.TrustedProxies == nil {
		a.TrustedProxies = []string{}
	}
	if a.OIDCScopes == nil {
		a.OIDCScopes = []string{}
	}
	if a.OIDCAllowedGroups == nil {
		a.OIDCAllowedGroups = []string{}
	}
	if a.OIDCAllowedEmails == nil {
		a.OIDCAllowedEmails = []string{}
	}
	if a.GroupsSeparator == "" {
		a.GroupsSeparator = ","
	}
	return a, nil
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
