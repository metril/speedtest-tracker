package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/notify"
	"github.com/metril/speedtest-tracker/internal/oidcauth"
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
	Iperf3ListURL            *string          `json:"iperf3_list_url"`
}

// integrationsBody is the partial PUT document for the Integrations
// section. VMExtraLabels/VLStreamFields are pointers-to-map so an absent
// key and an explicit "{}" are distinguishable, matching the auth-header
// pointer-string fields.
type integrationsBody struct {
	VMEnabled         *bool              `json:"vm_enabled"`
	VMURL             *string            `json:"vm_url"`
	VMAuthHeader      *string            `json:"vm_auth_header"`
	VMAuthType        *string            `json:"vm_auth_type"`
	VMAuthUsername    *string            `json:"vm_auth_username"`
	VMAuthPassword    *string            `json:"vm_auth_password"`
	VMAuthToken       *string            `json:"vm_auth_token"`
	VMAuthHeaderName  *string            `json:"vm_auth_header_name"`
	VMAuthHeaderValue *string            `json:"vm_auth_header_value"`
	VMExtraLabels     *map[string]string `json:"vm_extra_labels"`
	VLEnabled         *bool              `json:"vl_enabled"`
	VLURL             *string            `json:"vl_url"`
	VLAuthHeader      *string            `json:"vl_auth_header"`
	VLAuthType        *string            `json:"vl_auth_type"`
	VLAuthUsername    *string            `json:"vl_auth_username"`
	VLAuthPassword    *string            `json:"vl_auth_password"`
	VLAuthToken       *string            `json:"vl_auth_token"`
	VLAuthHeaderName  *string            `json:"vl_auth_header_name"`
	VLAuthHeaderValue *string            `json:"vl_auth_header_value"`
	VLStreamFields    *map[string]string `json:"vl_stream_fields"`
	MetricsEnabled    *bool              `json:"metrics_enabled"`
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

// authBody is the partial PUT document for the Auth section.
type authBody struct {
	Mode            *string   `json:"mode"`
	UserHeader      *string   `json:"user_header"`
	GroupsHeader    *string   `json:"groups_header"`
	GroupsSeparator *string   `json:"groups_separator"`
	TrustedProxies  *[]string `json:"trusted_proxies"`
	AdminGroup      *string   `json:"admin_group"`
	AllowTokens     *bool     `json:"allow_tokens"`

	// OIDC* configure auth mode oidc. OIDCClientSecret is a secret: masked
	// on GET and, on PUT, an echoed-back mask means "keep stored", same as
	// the VM/VL export-auth secrets.
	OIDCIssuer          *string   `json:"oidc_issuer"`
	OIDCClientID        *string   `json:"oidc_client_id"`
	OIDCClientSecret    *string   `json:"oidc_client_secret"`
	OIDCRedirectBaseURL *string   `json:"oidc_redirect_base_url"`
	OIDCScopes          *[]string `json:"oidc_scopes"`
	OIDCGroupsClaim     *string   `json:"oidc_groups_claim"`
	OIDCAllowedGroups   *[]string `json:"oidc_allowed_groups"`
	OIDCAllowedEmails   *[]string `json:"oidc_allowed_emails"`
	SessionTTLHours     *int      `json:"session_ttl_hours"`
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

		// SLADownloadMbps/SLAUploadMbps: like every other field here, nil
		// (omitted, or an explicit JSON null — Go's encoding/json can't
		// tell those apart for a pointer-typed field) leaves the stored
		// plan untouched; a value sets it. To disable/clear a plan, PUT 0
		// (or a negative value) — store.resolvePlan treats a non-positive
		// plan speed as unset, which is the documented way to clear one,
		// since this endpoint otherwise can't distinguish omitted from
		// explicit null on a plain pointer field.
		SLADownloadMbps *float64 `json:"sla_download_mbps"`
		SLAUploadMbps   *float64 `json:"sla_upload_mbps"`
	} `json:"general"`
	Engines       *enginesBody       `json:"engines"`
	Integrations  *integrationsBody  `json:"integrations"`
	Notifications *notificationsBody `json:"notifications"`
	Auth          *authBody          `json:"auth"`
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
	a, err := d.Settings.Auth(ctx)
	if err != nil {
		internalError(w, d.Logger, "load auth settings", err)
		return
	}
	maskAuthSecrets(&a)
	locked := d.Settings.LockedKeys()
	if locked == nil {
		locked = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"general":       g,
		"engines":       e,
		"integrations":  i,
		"notifications": n,
		"auth":          a,
		"locked":        locked,
	})
}

// maskChannelTokens replaces every set channel secret — the token and each
// webhook header value — with settings.MaskedSecret, leaving unset ones as
// the empty string. Apprise URLs are redacted (not blanket-masked) via
// notify.RedactURL, which keeps the non-secret scheme/host/path visible;
// mergeChannelSecrets resolves a submitted redacted URL back to the
// stored one by identity, so deleting or reordering entries doesn't
// silently resurrect the wrong one.
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
				n.Channels[i].URLs[j] = notify.RedactURL(u)
			}
		}
	}
}

// maskSecrets replaces every set secret (the legacy auth header and each
// structured export-auth secret field) with settings.MaskedSecret, leaving
// an unset one as the empty string.
func maskSecrets(i *settings.Integrations) {
	if i.VMAuthHeader != "" {
		i.VMAuthHeader = settings.MaskedSecret
	}
	if i.VMAuthPassword != "" {
		i.VMAuthPassword = settings.MaskedSecret
	}
	if i.VMAuthToken != "" {
		i.VMAuthToken = settings.MaskedSecret
	}
	if i.VMAuthHeaderValue != "" {
		i.VMAuthHeaderValue = settings.MaskedSecret
	}
	if i.VLAuthHeader != "" {
		i.VLAuthHeader = settings.MaskedSecret
	}
	if i.VLAuthPassword != "" {
		i.VLAuthPassword = settings.MaskedSecret
	}
	if i.VLAuthToken != "" {
		i.VLAuthToken = settings.MaskedSecret
	}
	if i.VLAuthHeaderValue != "" {
		i.VLAuthHeaderValue = settings.MaskedSecret
	}
}

// maskAuthSecrets replaces a's set OIDC client secret with
// settings.MaskedSecret, leaving an unset one as the empty string.
func maskAuthSecrets(a *settings.Auth) {
	if a.OIDCClientSecret != "" {
		a.OIDCClientSecret = settings.MaskedSecret
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

	// Locked-key enforcement runs first, generically, over the whole
	// document — before any per-section validation — so a future section
	// gets it for free and a locked write is rejected without ever
	// touching the store.
	if locked := lockedKeysIn(d, body); len(locked) > 0 {
		errBadRequest(w, strings.Join(locked, ", ")+" set by the environment and cannot be changed here")
		return
	}

	// Auth section changes are admin-only. IsAdmin already accounts for
	// whether an admin group is configured: it is true in open/token mode
	// and, in forward_auth, only for members of the configured group (or
	// everyone, if no group is configured).
	if body.Auth != nil && !requestIsAdmin(r) {
		errForbidden(w, "admin access required to change auth settings")
		return
	}

	// RULING (final review #3): a token is full-access for normal API use,
	// but must not be usable to change the Auth section itself — otherwise
	// a leaked or over-broad token could flip auth to open and remove the
	// need for a token at all. Only a forward-auth or open-mode session
	// (a human, or no auth configured) may write here.
	if body.Auth != nil && requestAuthIsToken(r) {
		errForbidden(w, "auth settings require a forward-auth or open-mode session")
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
	// Merged before validation, not just before the writes: an apprise
	// channel's URLs are validated as real apprise-go target URLs, so a
	// masked "***" placeholder must already be resolved back to the
	// stored URL (or rejected as a no-op below) before validateSettings
	// ever sees it — otherwise every echoed-back masked apprise URL would
	// fail validation as a bogus target.
	var mergedChannels *[]settings.Channel
	if n := body.Notifications; n != nil && n.Channels != nil {
		merged, err := mergeChannelSecrets(*n.Channels, currentNotify.Channels)
		if err != nil {
			errBadRequest(w, err.Error())
			return
		}
		mergedChannels = &merged
		*n.Channels = merged
	}

	// currentAuth is loaded up front (not just inside the lockout guard
	// below) because validateAuthBody now needs it too: the oidc-mode
	// field checks (issuer/client_id/groups_claim non-empty) are evaluated
	// against the resulting merged config, not just the fields this PUT
	// happens to touch.
	var currentAuth settings.Auth
	if body.Auth != nil {
		var err error
		currentAuth, err = d.Settings.Auth(ctx)
		if err != nil {
			internalError(w, d.Logger, "load auth settings", err)
			return
		}
	}

	if err := validateSettings(body, current); err != nil {
		errBadRequest(w, err.Error())
		return
	}
	if err := validateAuthBody(body.Auth, currentAuth); err != nil {
		errBadRequest(w, err.Error())
		return
	}

	// The lockout guard: computed only when the request actually touches
	// the Auth section, so an unrelated PUT (say, to General) is never
	// gated behind the caller already satisfying whatever auth mode
	// happens to be configured.
	if body.Auth != nil {
		resultingAuth := mergeAuth(currentAuth, body.Auth)
		if err := d.checkAuthLockout(r, currentAuth, resultingAuth); err != nil {
			errBadRequest(w, err.Error())
			return
		}
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
			func() error { return setPtr(ctx, d.Settings, settings.KeySLADownloadMbps, g.SLADownloadMbps) },
			func() error { return setPtr(ctx, d.Settings, settings.KeySLAUploadMbps, g.SLAUploadMbps) },
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
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyIperf3ListURL, e.Iperf3ListURL)
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
			func() error { return setPtr(ctx, d.Settings, settings.KeyVMAuthType, i.VMAuthType) },
			// A request that explicitly writes vm_auth_type takes over
			// from the legacy vm_auth_header fallback (VMAuth() only
			// consults it when VMAuthType is unset/none), but the UI can
			// never send an empty vm_auth_header itself -- it always
			// echoes back the masked placeholder, which setSecret above
			// treats as "leave unchanged". So switching auth type must
			// clear the legacy header here, in the same write, or a
			// stored legacy header keeps being used forever even after
			// the caller picks "none".
			func() error {
				if i.VMAuthType == nil {
					return nil
				}
				return d.Settings.Set(ctx, settings.KeyVMAuthHeader, "")
			},
			func() error { return setPtr(ctx, d.Settings, settings.KeyVMAuthUsername, i.VMAuthUsername) },
			func() error { return setSecret(ctx, d.Settings, settings.KeyVMAuthPassword, i.VMAuthPassword) },
			func() error { return setSecret(ctx, d.Settings, settings.KeyVMAuthToken, i.VMAuthToken) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVMAuthHeaderName, i.VMAuthHeaderName) },
			func() error { return setSecret(ctx, d.Settings, settings.KeyVMAuthHeaderValue, i.VMAuthHeaderValue) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVMExtraLabels, i.VMExtraLabels) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVLEnabled, i.VLEnabled) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVLURL, i.VLURL) },
			func() error { return setSecret(ctx, d.Settings, settings.KeyVLAuthHeader, i.VLAuthHeader) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVLAuthType, i.VLAuthType) },
			// See the matching vm_auth_type write above: clears the
			// legacy header the same way when vl_auth_type is written.
			func() error {
				if i.VLAuthType == nil {
					return nil
				}
				return d.Settings.Set(ctx, settings.KeyVLAuthHeader, "")
			},
			func() error { return setPtr(ctx, d.Settings, settings.KeyVLAuthUsername, i.VLAuthUsername) },
			func() error { return setSecret(ctx, d.Settings, settings.KeyVLAuthPassword, i.VLAuthPassword) },
			func() error { return setSecret(ctx, d.Settings, settings.KeyVLAuthToken, i.VLAuthToken) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyVLAuthHeaderName, i.VLAuthHeaderName) },
			func() error { return setSecret(ctx, d.Settings, settings.KeyVLAuthHeaderValue, i.VLAuthHeaderValue) },
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

	if a := body.Auth; a != nil {
		writes := []func() error{
			func() error { return setPtr(ctx, d.Settings, settings.KeyAuthMode, a.Mode) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyAuthUserHeader, a.UserHeader) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyAuthGroupsHeader, a.GroupsHeader) },
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyAuthGroupsSeparator, a.GroupsSeparator)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyAuthTrustedProxies, a.TrustedProxies)
			},
			func() error { return setPtr(ctx, d.Settings, settings.KeyAuthAdminGroup, a.AdminGroup) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyAuthAllowTokens, a.AllowTokens) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyAuthOIDCIssuer, a.OIDCIssuer) },
			func() error { return setPtr(ctx, d.Settings, settings.KeyAuthOIDCClientID, a.OIDCClientID) },
			func() error {
				return setSecret(ctx, d.Settings, settings.KeyAuthOIDCClientSecret, a.OIDCClientSecret)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyAuthOIDCRedirectBaseURL, a.OIDCRedirectBaseURL)
			},
			func() error { return setPtr(ctx, d.Settings, settings.KeyAuthOIDCScopes, a.OIDCScopes) },
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyAuthOIDCGroupsClaim, a.OIDCGroupsClaim)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyAuthOIDCAllowedGroups, a.OIDCAllowedGroups)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyAuthOIDCAllowedEmails, a.OIDCAllowedEmails)
			},
			func() error {
				return setPtr(ctx, d.Settings, settings.KeyAuthSessionTTLHours, a.SessionTTLHours)
			},
		}
		for _, w2 := range writes {
			if err := w2(); err != nil {
				internalError(w, d.Logger, "write auth settings", err)
				return
			}
		}
	}

	d.getSettings(w, r)
}

// requestIsAdmin reports whether the caller may change admin-only
// settings. A zero-value identity means no auth middleware is mounted
// (Deps.Auth nil), which is treated as open access, matching GET
// /api/v1/me's convention.
func requestIsAdmin(r *http.Request) bool {
	id := auth.FromContext(r.Context())
	return id.Mode == "" || id.IsAdmin
}

// requestAuthIsToken reports whether the caller authenticated via a
// bearer/query API token (auth.SourceToken), as opposed to a forward-auth
// header, an oidc session or an open-mode session — an oidc-mode request
// carrying a valid session cookie has Source auth.SourceOIDC, so it passes
// this check (is not a token) the same as forward-auth. A zero-value
// identity (no auth middleware mounted) is not a token, matching
// requestIsAdmin's convention that an absent Deps.Auth means open access.
func requestAuthIsToken(r *http.Request) bool {
	id := auth.FromContext(r.Context())
	return id.Source == auth.SourceToken
}

// lockedKeysIn returns the settings keys body would write that are
// currently locked by ST_LOCK_ENV, across every section — not just Auth —
// so a future section is covered by this check for free.
func lockedKeysIn(d Deps, body settingsBody) []string {
	var locked []string
	for _, key := range setKeys(body) {
		if d.Settings.IsLocked(key) {
			locked = append(locked, key)
		}
	}
	return locked
}

// setKeys returns the settings keys body would write.
func setKeys(body settingsBody) []string {
	var keys []string
	if g := body.General; g != nil {
		if g.BaseURL != nil {
			keys = append(keys, settings.KeyBaseURL)
		}
		if g.Timezone != nil {
			keys = append(keys, settings.KeyTimezone)
		}
		if g.Units != nil {
			keys = append(keys, settings.KeyUnits)
		}
		if g.LogLevel != nil {
			keys = append(keys, settings.KeyLogLevel)
		}
		if g.RetentionDaysResults != nil {
			keys = append(keys, settings.KeyRetentionDaysResults)
		}
		if g.RetentionDaysRuns != nil {
			keys = append(keys, settings.KeyRetentionDaysRuns)
		}
		if g.RetentionPruneIntervalMinutes != nil {
			keys = append(keys, settings.KeyRetentionPruneIntervalMinutes)
		}
	}
	if e := body.Engines; e != nil {
		if e.SpeedtestBin != nil {
			keys = append(keys, settings.KeySpeedtestBin)
		}
		if e.Iperf3Bin != nil {
			keys = append(keys, settings.KeyIperf3Bin)
		}
		if e.OoklaAcceptLicense != nil {
			keys = append(keys, settings.KeyOoklaAcceptLicense)
		}
		if e.OoklaAcceptGDPR != nil {
			keys = append(keys, settings.KeyOoklaAcceptGDPR)
		}
		if e.ServerListTTLSeconds != nil {
			keys = append(keys, settings.KeyServerListTTLSeconds)
		}
		if e.DefaultOoklaOptions != nil {
			keys = append(keys, settings.KeyDefaultOoklaOptions)
		}
		if e.DefaultCloudflareOptions != nil {
			keys = append(keys, settings.KeyDefaultCloudflareOptions)
		}
		if e.DefaultIperf3Options != nil {
			keys = append(keys, settings.KeyDefaultIperf3Options)
		}
		if e.Iperf3ListURL != nil {
			keys = append(keys, settings.KeyIperf3ListURL)
		}
	}
	if i := body.Integrations; i != nil {
		if i.VMEnabled != nil {
			keys = append(keys, settings.KeyVMEnabled)
		}
		if i.VMURL != nil {
			keys = append(keys, settings.KeyVMURL)
		}
		if i.VMAuthHeader != nil {
			keys = append(keys, settings.KeyVMAuthHeader)
		}
		if i.VMAuthType != nil {
			keys = append(keys, settings.KeyVMAuthType)
		}
		if i.VMAuthUsername != nil {
			keys = append(keys, settings.KeyVMAuthUsername)
		}
		if i.VMAuthPassword != nil {
			keys = append(keys, settings.KeyVMAuthPassword)
		}
		if i.VMAuthToken != nil {
			keys = append(keys, settings.KeyVMAuthToken)
		}
		if i.VMAuthHeaderName != nil {
			keys = append(keys, settings.KeyVMAuthHeaderName)
		}
		if i.VMAuthHeaderValue != nil {
			keys = append(keys, settings.KeyVMAuthHeaderValue)
		}
		if i.VMExtraLabels != nil {
			keys = append(keys, settings.KeyVMExtraLabels)
		}
		if i.VLEnabled != nil {
			keys = append(keys, settings.KeyVLEnabled)
		}
		if i.VLURL != nil {
			keys = append(keys, settings.KeyVLURL)
		}
		if i.VLAuthHeader != nil {
			keys = append(keys, settings.KeyVLAuthHeader)
		}
		if i.VLAuthType != nil {
			keys = append(keys, settings.KeyVLAuthType)
		}
		if i.VLAuthUsername != nil {
			keys = append(keys, settings.KeyVLAuthUsername)
		}
		if i.VLAuthPassword != nil {
			keys = append(keys, settings.KeyVLAuthPassword)
		}
		if i.VLAuthToken != nil {
			keys = append(keys, settings.KeyVLAuthToken)
		}
		if i.VLAuthHeaderName != nil {
			keys = append(keys, settings.KeyVLAuthHeaderName)
		}
		if i.VLAuthHeaderValue != nil {
			keys = append(keys, settings.KeyVLAuthHeaderValue)
		}
		if i.VLStreamFields != nil {
			keys = append(keys, settings.KeyVLStreamFields)
		}
		if i.MetricsEnabled != nil {
			keys = append(keys, settings.KeyMetricsEnabled)
		}
	}
	if n := body.Notifications; n != nil {
		if n.Enabled != nil {
			keys = append(keys, settings.KeyNotifyEnabled)
		}
		if n.Channels != nil {
			keys = append(keys, settings.KeyNotifyChannels)
		}
		if n.DefaultThresholds != nil {
			keys = append(keys, settings.KeyNotifyDefaultThresholds)
		}
		if n.CooldownMinutes != nil {
			keys = append(keys, settings.KeyNotifyCooldownMinutes)
		}
		if n.QuietHoursStart != nil {
			keys = append(keys, settings.KeyNotifyQuietStart)
		}
		if n.QuietHoursEnd != nil {
			keys = append(keys, settings.KeyNotifyQuietEnd)
		}
		if n.NotifyRecovery != nil {
			keys = append(keys, settings.KeyNotifyRecovery)
		}
	}
	if a := body.Auth; a != nil {
		if a.Mode != nil {
			keys = append(keys, settings.KeyAuthMode)
		}
		if a.UserHeader != nil {
			keys = append(keys, settings.KeyAuthUserHeader)
		}
		if a.GroupsHeader != nil {
			keys = append(keys, settings.KeyAuthGroupsHeader)
		}
		if a.GroupsSeparator != nil {
			keys = append(keys, settings.KeyAuthGroupsSeparator)
		}
		if a.TrustedProxies != nil {
			keys = append(keys, settings.KeyAuthTrustedProxies)
		}
		if a.AdminGroup != nil {
			keys = append(keys, settings.KeyAuthAdminGroup)
		}
		if a.AllowTokens != nil {
			keys = append(keys, settings.KeyAuthAllowTokens)
		}
		if a.OIDCIssuer != nil {
			keys = append(keys, settings.KeyAuthOIDCIssuer)
		}
		if a.OIDCClientID != nil {
			keys = append(keys, settings.KeyAuthOIDCClientID)
		}
		if a.OIDCClientSecret != nil {
			keys = append(keys, settings.KeyAuthOIDCClientSecret)
		}
		if a.OIDCRedirectBaseURL != nil {
			keys = append(keys, settings.KeyAuthOIDCRedirectBaseURL)
		}
		if a.OIDCScopes != nil {
			keys = append(keys, settings.KeyAuthOIDCScopes)
		}
		if a.OIDCGroupsClaim != nil {
			keys = append(keys, settings.KeyAuthOIDCGroupsClaim)
		}
		if a.OIDCAllowedGroups != nil {
			keys = append(keys, settings.KeyAuthOIDCAllowedGroups)
		}
		if a.OIDCAllowedEmails != nil {
			keys = append(keys, settings.KeyAuthOIDCAllowedEmails)
		}
		if a.SessionTTLHours != nil {
			keys = append(keys, settings.KeyAuthSessionTTLHours)
		}
	}
	return keys
}

// mergeAuth applies body onto current, leaving nil fields untouched.
func mergeAuth(current settings.Auth, body *authBody) settings.Auth {
	out := current
	if body == nil {
		return out
	}
	if body.Mode != nil {
		out.Mode = *body.Mode
	}
	if body.UserHeader != nil {
		out.UserHeader = *body.UserHeader
	}
	if body.GroupsHeader != nil {
		out.GroupsHeader = *body.GroupsHeader
	}
	if body.GroupsSeparator != nil {
		out.GroupsSeparator = *body.GroupsSeparator
	}
	if body.TrustedProxies != nil {
		out.TrustedProxies = *body.TrustedProxies
	}
	if body.AdminGroup != nil {
		out.AdminGroup = *body.AdminGroup
	}
	if body.AllowTokens != nil {
		out.AllowTokens = *body.AllowTokens
	}
	if body.OIDCIssuer != nil {
		out.OIDCIssuer = *body.OIDCIssuer
	}
	if body.OIDCClientID != nil {
		out.OIDCClientID = *body.OIDCClientID
	}
	if body.OIDCClientSecret != nil && *body.OIDCClientSecret != settings.MaskedSecret {
		// A mask echoed back means "keep stored" — out.OIDCClientSecret is
		// already current.OIDCClientSecret, so nothing to do in that case.
		out.OIDCClientSecret = *body.OIDCClientSecret
	}
	if body.OIDCRedirectBaseURL != nil {
		out.OIDCRedirectBaseURL = *body.OIDCRedirectBaseURL
	}
	if body.OIDCScopes != nil {
		out.OIDCScopes = *body.OIDCScopes
	}
	if body.OIDCGroupsClaim != nil {
		out.OIDCGroupsClaim = *body.OIDCGroupsClaim
	}
	if body.OIDCAllowedGroups != nil {
		out.OIDCAllowedGroups = *body.OIDCAllowedGroups
	}
	if body.OIDCAllowedEmails != nil {
		out.OIDCAllowedEmails = *body.OIDCAllowedEmails
	}
	if body.SessionTTLHours != nil {
		out.SessionTTLHours = *body.SessionTTLHours
	}
	return out
}

// isValidHTTPHeaderName reports whether s is a syntactically valid HTTP
// header field name: a non-empty run of RFC 7230 "tchar"s only (a stricter
// check than mirroring notify.isValidHTTPToken's old CanonicalHeaderKey +
// blacklist approach, which let a comma-separated name like "Remote,User"
// through since comma was never in the blacklist).
func isValidHTTPHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isHTTPTChar(s[i]) {
			return false
		}
	}
	return true
}

// isHTTPTChar reports whether c is an RFC 7230 "tchar":
// tchar = "!" / "#" / "$" / "%" / "&" / "'" / "*" / "+" / "-" / "." /
//
//	"^" / "_" / "`" / "|" / "~" / DIGIT / ALPHA
func isHTTPTChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

// validateAuthBody checks the Auth partial document's own fields, plus (for
// the oidc-mode fields) the resulting config after merging onto current: a
// PUT that only sets mode=oidc without ever having set an issuer must still
// be rejected, not just one that clears an already-configured issuer.
func validateAuthBody(a *authBody, current settings.Auth) error {
	if a == nil {
		return nil
	}
	if a.Mode != nil {
		switch *a.Mode {
		case settings.AuthModeOpen, settings.AuthModeForward, settings.AuthModeToken, settings.AuthModeOIDC:
		default:
			return fmt.Errorf("mode must be one of open, forward_auth, token, oidc")
		}
	}
	if a.UserHeader != nil && !isValidHTTPHeaderName(*a.UserHeader) {
		return fmt.Errorf("user_header is not a valid HTTP header name")
	}
	if a.GroupsHeader != nil && !isValidHTTPHeaderName(*a.GroupsHeader) {
		return fmt.Errorf("groups_header is not a valid HTTP header name")
	}
	if a.GroupsSeparator != nil {
		if *a.GroupsSeparator == "" || len(*a.GroupsSeparator) > 4 {
			return fmt.Errorf("groups_separator must be 1-4 bytes")
		}
	}
	if a.TrustedProxies != nil {
		for _, raw := range *a.TrustedProxies {
			if _, err := netip.ParsePrefix(raw); err != nil {
				return fmt.Errorf("trusted_proxies: invalid CIDR %q", raw)
			}
		}
	}
	// session_ttl_hours is checked whenever it is set, regardless of mode:
	// it's meaningless outside oidc mode today, but a bogus value should
	// still be rejected rather than stored silently.
	if a.SessionTTLHours != nil {
		if *a.SessionTTLHours < 1 || *a.SessionTTLHours > 8760 {
			return fmt.Errorf("session_ttl_hours must be between 1 and 8760")
		}
	}
	if a.OIDCRedirectBaseURL != nil && *a.OIDCRedirectBaseURL != "" {
		if err := validateAbsoluteHTTPURL(*a.OIDCRedirectBaseURL); err != nil {
			return fmt.Errorf("oidc_redirect_base_url: %w", err)
		}
	}

	resulting := mergeAuth(current, a)
	if resulting.Mode == settings.AuthModeOIDC {
		if err := validateAbsoluteHTTPURL(resulting.OIDCIssuer); err != nil {
			return fmt.Errorf("oidc_issuer: %w", err)
		}
		if resulting.OIDCClientID == "" {
			return fmt.Errorf("oidc_client_id is required when mode is oidc")
		}
		if resulting.OIDCGroupsClaim == "" {
			return fmt.Errorf("oidc_groups_claim is required when mode is oidc")
		}
	}
	return nil
}

// validateAbsoluteHTTPURL reports an error unless raw parses as an absolute
// http(s) URL with a host.
func validateAbsoluteHTTPURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("required")
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

// checkAuthLockout is the "so you cannot lock yourself out" guard. It is
// evaluated against the resulting auth config — current merged with the
// patch — whenever a PUT touches the Auth section, whether or not the
// mode itself changed: forward_auth and token are inherently restrictive,
// so their invariants (a non-empty trusted_proxies, at least one issued
// token) must hold any time the section is written, not only on switch.
func (d Deps) checkAuthLockout(r *http.Request, current, resulting settings.Auth) error {
	switch resulting.Mode {
	case settings.AuthModeOpen:
		// Always allowed: this is the deliberate escape hatch — a
		// locked-out operator can always fall back via ST_AUTH_MODE=open.
		return nil

	case settings.AuthModeForward:
		if len(resulting.TrustedProxies) == 0 {
			return fmt.Errorf("trusted_proxies must not be empty")
		}
		switching := resulting.Mode != current.Mode
		narrowed := !slices.Equal(resulting.TrustedProxies, current.TrustedProxies) ||
			resulting.UserHeader != current.UserHeader
		if !switching && !narrowed {
			return nil
		}
		// An actual switch into forward_auth, or an already-forward_auth
		// request narrowing trusted_proxies/user_header (which could
		// otherwise exclude the operator's own request): the request
		// performing it must itself satisfy the resulting config, or the
		// operator locks themselves out immediately.
		addrPort, err := netip.ParseAddrPort(r.RemoteAddr)
		if err != nil {
			return fmt.Errorf("cannot verify this request satisfies trusted_proxies")
		}
		addr := addrPort.Addr().Unmap()
		trusted := false
		for _, raw := range resulting.TrustedProxies {
			p, err := netip.ParsePrefix(raw)
			if err != nil {
				continue
			}
			if p.Contains(addr) {
				trusted = true
				break
			}
		}
		if !trusted {
			return fmt.Errorf("this request does not originate from a trusted proxy in trusted_proxies")
		}
		header := resulting.UserHeader
		if header == "" {
			header = "Remote-User"
		}
		if r.Header.Get(header) == "" {
			return fmt.Errorf("this request does not carry the %s header from a trusted proxy", header)
		}
		return nil

	case settings.AuthModeToken:
		n, err := d.Store.CountAPITokens(r.Context())
		if err != nil {
			return fmt.Errorf("count api tokens: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("create an API token before switching to token mode")
		}
		if resulting.Mode == current.Mode {
			return nil
		}
		// An actual switch into token mode: require the switching request
		// itself to carry a valid bearer token, proving the operator
		// already has one in hand. Without this, a PUT with no
		// Authorization header succeeds and then immediately 401s every
		// subsequent request, including the SPA's own next call.
		plain, ok := bearerTokenFromRequest(r)
		if !ok {
			return fmt.Errorf("include a valid bearer token on this request to switch to token mode")
		}
		if _, found, err := d.Store.APITokenByHash(r.Context(), auth.HashToken(plain)); err != nil {
			return fmt.Errorf("verify bearer token: %w", err)
		} else if !found {
			return fmt.Errorf("include a valid bearer token on this request to switch to token mode")
		}
		return nil

	case settings.AuthModeOIDC:
		if resulting.OIDCIssuer == "" || resulting.OIDCClientID == "" || resulting.OIDCClientSecret == "" {
			return fmt.Errorf("oidc_issuer, oidc_client_id and oidc_client_secret must all be set")
		}
		if resulting.Mode == current.Mode {
			return nil
		}
		// An actual switch into oidc mode: require the switching request
		// to be an admin, forward-auth or open-mode session (not a bearer
		// token — same rationale as the token-mode switch guard above),
		// and require the configured issuer to actually be reachable and
		// speak OIDC discovery, so a typo doesn't lock every session out
		// the moment the SPA's own next request needs a working provider.
		if !requestIsAdmin(r) {
			return fmt.Errorf("admin access required to switch to oidc mode")
		}
		if requestAuthIsToken(r) {
			return fmt.Errorf("switching to oidc mode requires a forward-auth or open-mode session")
		}
		discoverCtx, cancel := context.WithTimeout(r.Context(), probeTimeout)
		defer cancel()
		if err := oidcauth.Discover(discoverCtx, resulting.OIDCIssuer, nil); err != nil {
			return fmt.Errorf("discover oidc issuer %q: %w", resulting.OIDCIssuer, err)
		}
		return nil

	default:
		return nil
	}
}

// bearerTokenFromRequest extracts a token from the Authorization header,
// mirroring auth.Middleware's own bearer extraction (unexported there).
func bearerTokenFromRequest(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return h[len(prefix):], true
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
		old, hasOld := stored[out[i].ID]
		if channelHasMaskedSecret(out[i]) {
			if !hasOld || old.Type != out[i].Type || old.URL != out[i].URL {
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
		}
		if len(out[i].URLs) > 0 {
			var oldURLs []string
			if hasOld {
				oldURLs = old.URLs
			}
			merged, err := mergeAppriseURLs(out[i].URLs, oldURLs)
			if err != nil {
				return nil, fmt.Errorf("channel %s: %w", out[i].ID, err)
			}
			out[i].URLs = merged
		}
	}
	return out, nil
}

// mergeAppriseURLs resolves each submitted apprise URL against a
// channel's stored URLs by identity, not position: a submitted string
// equal to notify.RedactURL of some not-yet-consumed stored URL *is* that
// stored URL (secrets and all), consumed in order so duplicates resolve
// deterministically. This is what lets an editor delete or reorder lines
// without resurrecting the wrong stored secret by position. Anything else
// is taken literally as a new URL, unless it still carries the "***" mask
// marker after failing to match anything — that's stale or mistyped
// input, reported as an error rather than stored verbatim.
func mergeAppriseURLs(submitted, stored []string) ([]string, error) {
	remaining := make([]string, len(stored))
	copy(remaining, stored)
	out := make([]string, len(submitted))
	for i, u := range submitted {
		matched := false
		for j, s := range remaining {
			if s != "" && u == notify.RedactURL(s) {
				out[i] = s
				remaining[j] = ""
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if strings.Contains(u, settings.MaskedSecret) {
			return nil, errors.New("re-enter the Apprise URL")
		}
		out[i] = u
	}
	return out, nil
}

// channelHasMaskedSecret reports whether ch carries settings.MaskedSecret
// in its token or any header value. Apprise URLs are handled separately
// by mergeAppriseURLs, since they're redacted (not blanket-masked) and
// resolved by identity rather than an exact "***" match.
func channelHasMaskedSecret(ch settings.Channel) bool {
	if ch.Token == settings.MaskedSecret {
		return true
	}
	for _, v := range ch.Headers {
		if v == settings.MaskedSecret {
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
		if g.SLADownloadMbps != nil && *g.SLADownloadMbps < 0 {
			return fmt.Errorf("sla_download_mbps must be >= 0")
		}
		if g.SLAUploadMbps != nil && *g.SLAUploadMbps < 0 {
			return fmt.Errorf("sla_upload_mbps must be >= 0")
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

		vmAuthType := current.VMAuthType
		if i.VMAuthType != nil {
			vmAuthType = *i.VMAuthType
		}
		vmAuthHeaderName := current.VMAuthHeaderName
		if i.VMAuthHeaderName != nil {
			vmAuthHeaderName = *i.VMAuthHeaderName
		}
		if err := validateExportAuthType(i.VMAuthType, vmAuthType, vmAuthHeaderName); err != nil {
			return fmt.Errorf("vm_auth: %w", err)
		}

		vlAuthType := current.VLAuthType
		if i.VLAuthType != nil {
			vlAuthType = *i.VLAuthType
		}
		vlAuthHeaderName := current.VLAuthHeaderName
		if i.VLAuthHeaderName != nil {
			vlAuthHeaderName = *i.VLAuthHeaderName
		}
		if err := validateExportAuthType(i.VLAuthType, vlAuthType, vlAuthHeaderName); err != nil {
			return fmt.Errorf("vl_auth: %w", err)
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
		{"sla_download_mbps", t.SLADownloadMbps, nil},
		{"sla_upload_mbps", t.SLAUploadMbps, nil},
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

// validateExportAuthType validates a structured export-auth type/header-name
// pair. bodyType is the body-supplied *_auth_type pointer (nil when the
// request didn't touch it, in which case a bad type already stored is not
// this request's problem); effectiveType/effectiveHeaderName are the
// resulting values after merging the body onto the current settings. A
// custom type requires a non-empty, syntactically valid header name.
func validateExportAuthType(bodyType *string, effectiveType, effectiveHeaderName string) error {
	if bodyType != nil && !settings.ValidExportAuthType(*bodyType) {
		return fmt.Errorf("auth_type must be one of %s, %s, %s, %s",
			settings.ExportAuthNone, settings.ExportAuthBasic, settings.ExportAuthBearer, settings.ExportAuthCustom)
	}
	if effectiveType == settings.ExportAuthCustom {
		if effectiveHeaderName == "" {
			return fmt.Errorf("auth_header_name is required when auth_type is custom")
		}
		if !isValidHTTPHeaderName(effectiveHeaderName) {
			return fmt.Errorf("auth_header_name is not a valid HTTP header name")
		}
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

// testAuthBody is the structured export-auth object accepted by
// POST /settings/test/{vm,vl}, mirroring settings.ExportAuth. A secret
// field (password, token, header_value) equal to settings.MaskedSecret
// resolves to the stored value, but only per resolveTestAuth's same-origin
// rule.
type testAuthBody struct {
	Type        *string `json:"type"`
	Username    *string `json:"username"`
	Password    *string `json:"password"`
	Token       *string `json:"token"`
	HeaderName  *string `json:"header_name"`
	HeaderValue *string `json:"header_value"`
}

// resolveTestAuth builds the settings.ExportAuth to apply to a connection
// test probe. auth (structured, preferred) takes priority over legacyHeader
// (the deprecated auth_header field); when neither is present in the
// request, the stored auth is reused. In every case, a masked secret
// (settings.MaskedSecret) — whether a field inside auth or the whole
// legacyHeader — resolves to the corresponding stored value only when
// rawURL is the same origin as storedURL; on a different origin it
// resolves to empty, since the caller could otherwise redirect the stored
// credential to an arbitrary host (SSRF + credential exfil).
func resolveTestAuth(rawURL, storedURL string, stored settings.ExportAuth, legacyHeader *string, auth *testAuthBody) settings.ExportAuth {
	same := sameOrigin(rawURL, storedURL)
	if auth != nil {
		out := settings.ExportAuth{}
		if auth.Type != nil {
			out.Type = *auth.Type
		}
		if auth.Username != nil {
			out.Username = *auth.Username
		}
		out.Password = resolveMaybeMaskedSecret(auth.Password, stored.Password, same)
		out.Token = resolveMaybeMaskedSecret(auth.Token, stored.Token, same)
		if auth.HeaderName != nil {
			out.HeaderName = *auth.HeaderName
		}
		out.HeaderValue = resolveMaybeMaskedSecret(auth.HeaderValue, stored.HeaderValue, same)
		return out
	}
	if legacyHeader != nil {
		if *legacyHeader != settings.MaskedSecret {
			return settings.ExportAuth{Type: settings.ExportAuthCustom, HeaderName: "Authorization", HeaderValue: *legacyHeader}
		}
		if same {
			return stored
		}
		return settings.ExportAuth{}
	}
	if same {
		return stored
	}
	return settings.ExportAuth{}
}

// resolveMaybeMaskedSecret resolves one secret field of a testAuthBody: nil
// (field omitted) is empty, settings.MaskedSecret resolves to storedValue
// only when same is true (otherwise empty), and anything else is taken
// literally as the caller's own input.
func resolveMaybeMaskedSecret(v *string, storedValue string, same bool) string {
	if v == nil {
		return ""
	}
	if *v == settings.MaskedSecret {
		if same {
			return storedValue
		}
		return ""
	}
	return *v
}

// testIntegration probes the configured VictoriaMetrics or VictoriaLogs
// endpoint with GET /health. A reachable-but-unhappy endpoint is reported
// in the body with ok=false rather than as an HTTP error, so the form can
// show the reason inline.
func (d Deps) testIntegration(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target")
	if target == "oidc" {
		d.testOIDC(w, r)
		return
	}
	if target != "vm" && target != "vl" {
		errNotFound(w, "unknown test target "+target)
		return
	}
	var body struct {
		URL        *string       `json:"url"`
		AuthHeader *string       `json:"auth_header"` // deprecated: use Auth
		Auth       *testAuthBody `json:"auth"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &body) {
		return
	}
	cur, err := d.Settings.Integrations(r.Context())
	if err != nil {
		internalError(w, d.Logger, "load integrations", err)
		return
	}
	storedURL, storedAuth := cur.VMURL, cur.VMAuth()
	if target == "vl" {
		storedURL, storedAuth = cur.VLURL, cur.VLAuth()
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
	// An explicit, non-masked auth is always honored since it is the
	// caller's own input, not the stored secret.
	auth := resolveTestAuth(rawURL, storedURL, storedAuth, body.AuthHeader, body.Auth)

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
	auth.Apply(req)

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

// testOIDC probes an OIDC issuer via discovery, mirroring testIntegration's
// {ok,...}/{ok,error} response shape. issuer/client_id/client_secret in the
// body default to the stored values; a client_secret equal to
// settings.MaskedSecret also resolves to the stored value. client_id and
// client_secret are accepted (and mask-resolved) for parity with the stored
// config and future use, but discovery itself only ever fetches the
// issuer's public metadata/JWKS, so neither is actually sent anywhere.
func (d Deps) testOIDC(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Issuer       *string `json:"issuer"`
		ClientID     *string `json:"client_id"`
		ClientSecret *string `json:"client_secret"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &body) {
		return
	}
	cur, err := d.Settings.Auth(r.Context())
	if err != nil {
		internalError(w, d.Logger, "load auth settings", err)
		return
	}

	issuer := cur.OIDCIssuer
	if body.Issuer != nil {
		issuer = *body.Issuer
	}
	if err := validateAbsoluteHTTPURL(issuer); err != nil {
		errBadRequest(w, "issuer: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), probeTimeout)
	defer cancel()
	start := time.Now()
	if err := oidcauth.Discover(ctx, issuer, nil); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latency_ms": time.Since(start).Milliseconds()})
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
