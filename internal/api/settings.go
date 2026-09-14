package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/metril/speedtest-tracker/internal/notify"
	"github.com/metril/speedtest-tracker/internal/settings"
)

// probeTimeout bounds a settings connection test. It is short on purpose:
// the user is staring at a spinner in the Settings form.
const probeTimeout = 5 * time.Second

// settingsKeyPattern matches a valid Prometheus/Loki-style label or field
// name: a leading letter or underscore, then letters, digits or
// underscores.
var settingsKeyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// vmReservedLabels are the labels the VictoriaMetrics push path already
// attaches to every sample; a user-supplied extra label with the same name
// would silently shadow it.
var vmReservedLabels = map[string]bool{
	"target": true, "target_id": true, "engine": true,
	"server_id": true, "server_name": true, "isp": true, "schedule": true,
}

// vlReservedFields are the stream fields VictoriaLogs pushes already use.
var vlReservedFields = map[string]bool{
	"app": true, "level": true, "_msg": true, "_time": true,
}

// enginesBody is the partial PUT document for the Engines section: a nil
// field is left alone, a non-nil one is written.
type enginesBody struct {
	SpeedtestBin             *string          `json:"speedtest_bin"`
	Iperf3Bin                *string          `json:"iperf3_bin"`
	OoklaAcceptLicense       *bool            `json:"ookla_accept_license"`
	OoklaAcceptGDPR          *bool            `json:"ookla_accept_gdpr"`
	ServerListTTLSeconds     *int             `json:"server_list_ttl_seconds"`
	DefaultOoklaOptions      *json.RawMessage `json:"default_ookla_options"`
	DefaultCloudflareOptions *json.RawMessage `json:"default_cloudflare_options"`
	DefaultIperf3Options     *json.RawMessage `json:"default_iperf3_options"`
}

// integrationsBody is the partial PUT document for the Integrations
// section. VMExtraLabels/VLStreamFields are pointers-to-map so an absent
// key and an explicit "{}" are distinguishable, matching the auth-header
// pointer-string fields.
type integrationsBody struct {
	VMEnabled      *bool              `json:"vm_enabled"`
	VMURL          *string            `json:"vm_url"`
	VMAuthHeader   *string            `json:"vm_auth_header"`
	VMExtraLabels  *map[string]string `json:"vm_extra_labels"`
	VLEnabled      *bool              `json:"vl_enabled"`
	VLURL          *string            `json:"vl_url"`
	VLAuthHeader   *string            `json:"vl_auth_header"`
	VLStreamFields *map[string]string `json:"vl_stream_fields"`
	MetricsEnabled *bool              `json:"metrics_enabled"`
}

// notificationsBody is the partial PUT document for the Notifications
// section, matching the existing pointer-field style.
type notificationsBody struct {
	Enabled           *bool                `json:"enabled"`
	Channels          *[]settings.Channel  `json:"channels"`
	DefaultThresholds *settings.Thresholds `json:"default_thresholds"`
	CooldownMinutes   *int                 `json:"cooldown_minutes"`
	QuietHoursStart   *string              `json:"quiet_hours_start"`
	QuietHoursEnd     *string              `json:"quiet_hours_end"`
	NotifyRecovery    *bool                `json:"notify_recovery"`
}

// settingsBody is the partial PUT document. Every field is a pointer: a
// nil field is left alone, a non-nil one is written — which is what makes
// clearing a secret (explicit "") different from omitting it.
type settingsBody struct {
	General *struct {
		BaseURL                       *string `json:"base_url"`
		Timezone                      *string `json:"timezone"`
		Units                         *string `json:"units"`
		LogLevel                      *string `json:"log_level"`
		RetentionDaysResults          *int    `json:"retention_days_results"`
		RetentionDaysRuns             *int    `json:"retention_days_runs"`
		RetentionPruneIntervalMinutes *int    `json:"retention_prune_interval_minutes"`
	} `json:"general"`
	Engines       *enginesBody       `json:"engines"`
	Integrations  *integrationsBody  `json:"integrations"`
	Notifications *notificationsBody `json:"notifications"`
}

// getSettings returns the full sectioned settings document, masking the
// VictoriaMetrics/VictoriaLogs auth headers.
func (d Deps) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, err := d.Settings.General(ctx)
	if err != nil {
		internalError(w, d.Logger, "load general settings", err)
		return
	}
	e, err := d.Settings.Engines(ctx)
	if err != nil {
		internalError(w, d.Logger, "load engines settings", err)
		return
	}
	i, err := d.Settings.Integrations(ctx)
	if err != nil {
		internalError(w, d.Logger, "load integrations settings", err)
		return
	}
	maskSecrets(&i)
	n, err := d.Settings.Notifications(ctx)
	if err != nil {
		internalError(w, d.Logger, "load notifications settings", err)
		return
	}
	maskChannelTokens(&n)
	writeJSON(w, http.StatusOK, map[string]any{
		"general":       g,
		"engines":       e,
		"integrations":  i,
		"notifications": n,
	})
}

// maskChannelTokens replaces every set channel secret — the token, each
// webhook header value and each apprise URL (which embeds its own
// credentials) — with settings.MaskedSecret, leaving unset ones as the
// empty string.
func maskChannelTokens(n *settings.Notifications) {
	for i := range n.Channels {
		if n.Channels[i].Token != "" {
			n.Channels[i].Token = settings.MaskedSecret
		}
		for k, v := range n.Channels[i].Headers {
			if v != "" {
				n.Channels[i].Headers[k] = settings.MaskedSecret
			}
		}
		for j, u := range n.Channels[i].URLs {
			if u != "" {
				n.Channels[i].URLs[j] = settings.MaskedSecret
			}
		}
	}
}

// maskSecrets replaces a set auth header with settings.MaskedSecret,
// leaving an unset one as the empty string.
func maskSecrets(i *settings.Integrations) {
	if i.VMAuthHeader != "" {
		i.VMAuthHeader = settings.MaskedSecret
	}
	if i.VLAuthHeader != "" {
		i.VLAuthHeader = settings.MaskedSecret
	}
}

// putSettings validates the full partial document before writing anything,
// so a rejected PUT is a no-op, then writes each provided field and
// responds with the same (masked) document getSettings would produce.
func (d Deps) putSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body settingsBody
	if !decodeJSON(w, r, &body) {
		return
	}

	current, err := d.Settings.Integrations(ctx)
	if err != nil {
		internalError(w, d.Logger, "load integrations settings", err)
		return
	}
	currentNotify, err := d.Settings.Notifications(ctx)
	if err != nil {
		internalError(w, d.Logger, "load notifications settings", err)
		return
	}
	if err := validateSettings(body, current); err != nil {
		errBadRequest(w, err.Error())
		return
	}

	// Merged up front, alongside validation, so a channel whose masked
	// secret cannot be carried forward (type/url changed) rejects the
	// whole PUT as a no-op rather than after other sections already wrote.
	var mergedChannels *[]settings.Channel
	if n := body.Notifications; n != nil && n.Channels != nil {
		merged, err := mergeChannelSecrets(*n.Channels, currentNotify.Channels)
		if err != nil {
			errBadRequest(w, err.Error())
			return
		}
		mergedChannels = &merged
	}

	// Validation above ran over the full document up front, so a Set
	// failure here is an infrastructure failure (DB write error), not a
	// user error: it is safe to report 500 even mid-way through.
	if g := body.General; g != nil {
		writes := []func() error{
			func() error { return setPtr(ctx, d.Settings, settings.KeyBaseURL, g.BaseURL) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyTimezone, g.Timezone) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyUnits, g.Units) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyLogLevel, g.LogLevel) },
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyRetentionDaysResults, g.RetentionDaysResults)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyRetentionDaysRuns, g.RetentionDaysRuns)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyRetentionPruneIntervalMinutes, g.RetentionPruneIntervalMinutes)
			},
		}
		for _, w2 := range writes {
			if err := w2(); err != nil {
				internalError(w, d.Logger, "write general settings", err)
				return
			}
		}
	}

	if e := body.Engines; e != nil {
		writes := []func() error{
			func() error { return setPtr(ctx, d.Settings, settings.KeySpeedtestBin, e.SpeedtestBin) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyIperf3Bin, e.Iperf3Bin) },
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyOoklaAcceptLicense, e.OoklaAcceptLicense)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyOoklaAcceptGDPR, e.OoklaAcceptGDPR)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyServerListTTLSeconds, e.ServerListTTLSeconds)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyDefaultOoklaOptions, e.DefaultOoklaOptions)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyDefaultCloudflareOptions, e.DefaultCloudflareOptions)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyDefaultIperf3Options, e.DefaultIperf3Options)
			},
		}
		for _, w2 := range writes {
			if err := w2(); err != nil {
				internalError(w, d.Logger, "write engines settings", err)
				return
			}
		}
	}

	if i := body.Integrations; i != nil {
		writes := []func() error{
			func() error { return setPtr(ctx, d.Settings, settings.KeyVMEnabled, i.VMEnabled) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVMURL, i.VMURL) },
			func() error { return setSecret(ctx, d.Settings, settings.KeyVMAuthHeader, i.VMAuthHeader) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVMExtraLabels, i.VMExtraLabels) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVLEnabled, i.VLEnabled) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVLURL, i.VLURL) },
			func() error { return setSecret(ctx, d.Settings, settings.KeyVLAuthHeader, i.VLAuthHeader) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVLStreamFields, i.VLStreamFields) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyMetricsEnabled, i.MetricsEnabled) },
		}
		for _, w2 := range writes {
			if err := w2(); err != nil {
				internalError(w, d.Logger, "write integrations settings", err)
				return
			}
		}
	}

	if n := body.Notifications; n != nil {
		writes := []func() error{
			func() error { return setPtr(ctx, d.Settings, settings.KeyNotifyEnabled, n.Enabled) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyNotifyChannels, mergedChannels) },
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyNotifyDefaultThresholds, n.DefaultThresholds)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyNotifyCooldownMinutes, n.CooldownMinutes)
			},
			func() error { return setPtr(ctx, d.Settings, settings.KeyNotifyQuietStart, n.QuietHoursStart) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyNotifyQuietEnd, n.QuietHoursEnd) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyNotifyRecovery, n.NotifyRecovery) },
		}
		for _, w2 := range writes {
			if err := w2(); err != nil {
				internalError(w, d.Logger, "write notifications settings", err)
				return
			}
		}
	}

	d.getSettings(w, r)
}

// setPtr writes *v under key when v is non-nil; a nil v is a no-op.
func setPtr[T any](ctx context.Context, s *settings.Store, key string, v *T) error {
	if v == nil {
		return nil
	}
	return s.Set(ctx, key, *v)
}

// mergeChannelSecrets replaces a masked secret — token, header value or
// apprise URL — with the one already stored for the same channel id, but
// only when the stored channel's type and url are unchanged: echoing
// "***" back after switching the url (or type) would otherwise forward
// the stored credential to whatever host the url now points at. When a
// channel carries a masked secret but its type/url changed (or it has no
// stored counterpart), that channel is reported so the caller can turn it
// into a 400 asking the client to re-enter the secret.
func mergeChannelSecrets(incoming []settings.Channel, current []settings.Channel) ([]settings.Channel, error) {
	stored := make(map[string]settings.Channel, len(current))
	for _, c := range current {
		stored[c.ID] = c
	}
	out := make([]settings.Channel, len(incoming))
	copy(out, incoming)
	for i := range out {
		if !channelHasMaskedSecret(out[i]) {
			continue
		}
		old, ok := stored[out[i].ID]
		if !ok || old.Type != out[i].Type || old.URL != out[i].URL {
			return nil, fmt.Errorf("channel %s: type or url changed; re-enter the token", out[i].ID)
		}
		if out[i].Token == settings.MaskedSecret {
			out[i].Token = old.Token
		}
		if len(out[i].Headers) > 0 {
			merged := make(map[string]string, len(out[i].Headers))
			for k, v := range out[i].Headers {
				if v == settings.MaskedSecret {
					v = old.Headers[k]
				}
				merged[k] = v
			}
			out[i].Headers = merged
		}
		if len(out[i].URLs) > 0 {
			merged := make([]string, len(out[i].URLs))
			for j, u := range out[i].URLs {
				if u == settings.MaskedSecret && j < len(old.URLs) {
					u = old.URLs[j]
				}
				merged[j] = u
			}
			out[i].URLs = merged
		}
	}
	return out, nil
}

// channelHasMaskedSecret reports whether ch carries settings.MaskedSecret
// in its token, any header value or any apprise url.
func channelHasMaskedSecret(ch settings.Channel) bool {
	if ch.Token == settings.MaskedSecret {
		return true
	}
	for _, v := range ch.Headers {
		if v == settings.MaskedSecret {
			return true
		}
	}
	for _, u := range ch.URLs {
		if u == settings.MaskedSecret {
			return true
		}
	}
	return false
}

// setSecret writes *v under key when v is non-nil, unless it equals
// settings.MaskedSecret — the client echoing the mask back means "keep the
// stored value", not "set the secret to the literal mask".
func setSecret(ctx context.Context, s *settings.Store, key string, v *string) error {
	if v == nil || *v == settings.MaskedSecret {
		return nil
	}
	return s.Set(ctx, key, *v)
}

// validateSettings checks the full partial document against the current
// Integrations section (used to resolve the enabled/URL cross-field rule
// for whichever of vm/vl the body does not touch).
func validateSettings(body settingsBody, current settings.Integrations) error {
	if g := body.General; g != nil {
		if g.LogLevel != nil {
			switch *g.LogLevel {
			case "debug", "info", "warn", "error":
			default:
				return fmt.Errorf("log_level must be one of debug, info, warn, error")
			}
		}
		if g.Units != nil {
			switch *g.Units {
			case "Mbps", "MB/s":
			default:
				return fmt.Errorf("units must be one of Mbps, MB/s")
			}
		}
		if g.Timezone != nil {
			if _, err := time.LoadLocation(*g.Timezone); err != nil {
				return fmt.Errorf("invalid timezone %q: %w", *g.Timezone, err)
			}
		}
		if g.RetentionDaysResults != nil && *g.RetentionDaysResults < 1 {
			return fmt.Errorf("retention_days_results must be at least 1")
		}
		if g.RetentionDaysRuns != nil && *g.RetentionDaysRuns < 1 {
			return fmt.Errorf("retention_days_runs must be at least 1")
		}
		if g.RetentionPruneIntervalMinutes != nil && *g.RetentionPruneIntervalMinutes < 1 {
			return fmt.Errorf("retention_prune_interval_minutes must be at least 1")
		}
	}

	if i := body.Integrations; i != nil {
		vmEnabled := current.VMEnabled
		if i.VMEnabled != nil {
			vmEnabled = *i.VMEnabled
		}
		vmURL := current.VMURL
		if i.VMURL != nil {
			vmURL = *i.VMURL
		}
		if err := validateEndpointURL(vmURL, vmEnabled); err != nil {
			return fmt.Errorf("vm_url: %w", err)
		}

		vlEnabled := current.VLEnabled
		if i.VLEnabled != nil {
			vlEnabled = *i.VLEnabled
		}
		vlURL := current.VLURL
		if i.VLURL != nil {
			vlURL = *i.VLURL
		}
		if err := validateEndpointURL(vlURL, vlEnabled); err != nil {
			return fmt.Errorf("vl_url: %w", err)
		}

		if i.VMExtraLabels != nil {
			if err := validateKeys(*i.VMExtraLabels, vmReservedLabels); err != nil {
				return fmt.Errorf("vm_extra_labels: %w", err)
			}
		}
		if i.VLStreamFields != nil {
			if err := validateKeys(*i.VLStreamFields, vlReservedFields); err != nil {
				return fmt.Errorf("vl_stream_fields: %w", err)
			}
		}
	}

	if n := body.Notifications; n != nil {
		if n.Channels != nil {
			seen := make(map[string]bool, len(*n.Channels))
			for _, ch := range *n.Channels {
				if err := notify.ValidateChannel(ch); err != nil {
					return err
				}
				if seen[ch.ID] {
					return fmt.Errorf("duplicate channel id %s", ch.ID)
				}
				seen[ch.ID] = true
			}
		}
		if n.CooldownMinutes != nil && *n.CooldownMinutes < 1 {
			return fmt.Errorf("cooldown_minutes must be at least 1")
		}
		start, end := "", ""
		if n.QuietHoursStart != nil {
			start = *n.QuietHoursStart
		}
		if n.QuietHoursEnd != nil {
			end = *n.QuietHoursEnd
		}
		if (n.QuietHoursStart != nil) != (n.QuietHoursEnd != nil) {
			return fmt.Errorf("quiet_hours_start and quiet_hours_end must be set together")
		}
		if n.QuietHoursStart != nil && n.QuietHoursEnd != nil && start != "" && end != "" {
			if _, err := time.Parse("15:04", start); err != nil {
				return fmt.Errorf("quiet_hours_start must be HH:MM")
			}
			if _, err := time.Parse("15:04", end); err != nil {
				return fmt.Errorf("quiet_hours_end must be HH:MM")
			}
		}
		if n.DefaultThresholds != nil {
			if err := validateThresholds(*n.DefaultThresholds); err != nil {
				return fmt.Errorf("default_thresholds: %w", err)
			}
		}
	}

	return nil
}

// validateThresholds checks that every set field is non-negative, and that
// a set loss percentage is at most 100. The error names the offending JSON
// field so a form can highlight it.
func validateThresholds(t settings.Thresholds) error {
	fields := []struct {
		name string
		v    *float64
		max  *float64
	}{
		{"download_mbps_min", t.DownloadMbpsMin, nil},
		{"upload_mbps_min", t.UploadMbpsMin, nil},
		{"ping_ms_max", t.PingMsMax, nil},
		{"jitter_ms_max", t.JitterMsMax, nil},
		{"loss_pct_max", t.LossPctMax, float64Ptr(100)},
	}
	for _, f := range fields {
		if f.v == nil {
			continue
		}
		if *f.v < 0 {
			return fmt.Errorf("%s must be >= 0", f.name)
		}
		if f.max != nil && *f.v > *f.max {
			return fmt.Errorf("%s must be <= %g", f.name, *f.max)
		}
	}
	return nil
}

// float64Ptr returns a pointer to v, for use in a struct literal.
func float64Ptr(v float64) *float64 { return &v }

// validateEndpointURL enforces the vm_url/vl_url rule: empty is only
// allowed when the integration is disabled; otherwise it must parse as an
// absolute http(s) URL with a host.
func validateEndpointURL(raw string, enabled bool) error {
	if raw == "" {
		if enabled {
			return fmt.Errorf("required when enabled")
		}
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("must be an absolute http(s) URL")
	}
	return nil
}

// sameOrigin reports whether a and b parse as URLs sharing the same scheme
// and host. A parse failure on either side is treated as not matching.
func sameOrigin(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	return ua.Scheme == ub.Scheme && ua.Host == ub.Host
}

// validateKeys checks every key against settingsKeyPattern and rejects
// reserved built-in names.
func validateKeys(m map[string]string, reserved map[string]bool) error {
	for k := range m {
		if !settingsKeyPattern.MatchString(k) {
			return fmt.Errorf("invalid key %q: must match %s", k, settingsKeyPattern.String())
		}
		if reserved[k] {
			return fmt.Errorf("key %q is reserved", k)
		}
	}
	return nil
}

// testIntegration probes the configured VictoriaMetrics or VictoriaLogs
// endpoint with GET /health. A reachable-but-unhappy endpoint is reported
// in the body with ok=false rather than as an HTTP error, so the form can
// show the reason inline.
func (d Deps) testIntegration(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target")
	if target != "vm" && target != "vl" {
		errNotFound(w, "unknown test target "+target)
		return
	}
	var body struct {
		URL        *string `json:"url"`
		AuthHeader *string `json:"auth_header"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &body) {
		return
	}
	cur, err := d.Settings.Integrations(r.Context())
	if err != nil {
		internalError(w, d.Logger, "load integrations", err)
		return
	}
	storedURL, storedAuth := cur.VMURL, cur.VMAuthHeader
	if target == "vl" {
		storedURL, storedAuth = cur.VLURL, cur.VLAuthHeader
	}
	rawURL := storedURL
	if body.URL != nil {
		rawURL = *body.URL
	}
	if err := validateEndpointURL(rawURL, true); err != nil {
		errBadRequest(w, err.Error())
		return
	}

	// The stored credential is only reused when the effective URL is the
	// same origin as the stored one; otherwise an unauthenticated caller
	// could redirect it to an arbitrary host (SSRF + credential exfil).
	// An explicit, non-masked auth_header is always honored since it is
	// the caller's own input, not the stored secret.
	var auth string
	switch {
	case body.AuthHeader != nil && *body.AuthHeader != settings.MaskedSecret:
		auth = *body.AuthHeader
	case sameOrigin(rawURL, storedURL):
		auth = storedAuth
	}

	client := d.TestClient
	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(rawURL, "/")+"/health", nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	latency := time.Since(start)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": fmt.Sprintf("GET /health returned %d", resp.StatusCode),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"status":     resp.StatusCode,
		"latency_ms": latency.Milliseconds(),
	})
}

// testNotifyChannel probes one stored notification channel. The channel is
// always taken from stored settings, never from the request body — that is
// what keeps this endpoint from being an SSRF primitive, and it matches
// the same-origin rule testIntegration uses. The user must save the
// channel before testing it.
func (d Deps) testNotifyChannel(w http.ResponseWriter, r *http.Request) {
	if d.Notifier == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "notifications are not wired")
		return
	}
	id := chi.URLParam(r, "channel_id")
	n, err := d.Settings.Notifications(r.Context())
	if err != nil {
		internalError(w, d.Logger, "load notifications", err)
		return
	}
	idx := slices.IndexFunc(n.Channels, func(c settings.Channel) bool { return c.ID == id })
	if idx < 0 {
		errNotFound(w, "unknown channel "+id)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
	defer cancel()
	start := time.Now()
	if err := d.Notifier.TestChannel(ctx, n.Channels[idx]); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency_ms": time.Since(start).Milliseconds()})
}
