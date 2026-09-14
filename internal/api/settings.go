package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
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
	Engines      *enginesBody      `json:"engines"`
	Integrations *integrationsBody `json:"integrations"`
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
	writeJSON(w, http.StatusOK, map[string]any{
		"general":      g,
		"engines":      e,
		"integrations": i,
	})
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
	if err := validateSettings(body, current); err != nil {
		errBadRequest(w, err.Error())
		return
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

	d.getSettings(w, r)
}

// setPtr writes *v under key when v is non-nil; a nil v is a no-op.
func setPtr[T any](ctx context.Context, s *settings.Store, key string, v *T) error {
	if v == nil {
		return nil
	}
	return s.Set(ctx, key, *v)
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
		if i.VMURL != nil {
			if err := validateEndpointURL(*i.VMURL, vmEnabled); err != nil {
				return fmt.Errorf("vm_url: %w", err)
			}
		}

		vlEnabled := current.VLEnabled
		if i.VLEnabled != nil {
			vlEnabled = *i.VLEnabled
		}
		if i.VLURL != nil {
			if err := validateEndpointURL(*i.VLURL, vlEnabled); err != nil {
				return fmt.Errorf("vl_url: %w", err)
			}
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

	return nil
}

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
	rawURL, auth := cur.VMURL, cur.VMAuthHeader
	if target == "vl" {
		rawURL, auth = cur.VLURL, cur.VLAuthHeader
	}
	if body.URL != nil {
		rawURL = *body.URL
	}
	if body.AuthHeader != nil && *body.AuthHeader != settings.MaskedSecret {
		auth = *body.AuthHeader
	}
	if err := validateEndpointURL(rawURL, true); err != nil {
		errBadRequest(w, err.Error())
		return
	}

	client := d.TestClient
	if client == nil {
		client = &http.Client{Timeout: probeTimeout}
	}
	ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL+"/health", nil)
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
