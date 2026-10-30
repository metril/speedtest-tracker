package api

import (
	"net/http"

	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// SettingsResponse is the sectioned settings document. Secrets are masked.
type SettingsResponse struct {
	General       settings.General       `json:"general"`
	Engines       settings.Engines       `json:"engines"`
	Integrations  settings.Integrations  `json:"integrations"`
	Notifications settings.Notifications `json:"notifications"`
	Auth          settings.Auth          `json:"auth"`
	Locked        []string               `json:"locked" doc:"Keys pinned by environment variables and not editable."`
}

// ConnectionTest is the result of a settings connectivity test.
type ConnectionTest struct {
	OK        bool   `json:"ok"`
	Status    int    `json:"status,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TestIntegrationRequest is the POST /settings/test/{target} body. Fields
// url, auth_header and auth apply to vm and vl; issuer, client_id and
// client_secret apply to oidc. A masked secret resolves to the stored one.
type TestIntegrationRequest struct {
	URL          *string       `json:"url,omitempty"`
	AuthHeader   *string       `json:"auth_header,omitempty" deprecated:"true"`
	Auth         *testAuthBody `json:"auth,omitempty"`
	Issuer       *string       `json:"issuer,omitempty"`
	ClientID     *string       `json:"client_id,omitempty"`
	ClientSecret *string       `json:"client_secret,omitempty"`
}

// TokenList is the GET /settings/tokens body.
type TokenList struct {
	Tokens []store.APIToken `json:"tokens"`
}

// CreateTokenRequest is the POST /settings/tokens body.
type CreateTokenRequest struct {
	Name string `json:"name"`
}

// CreatedToken is returned once, when a token is created: the only time
// the plaintext token is available.
type CreatedToken struct {
	store.APIToken
	Token string `json:"token"`
}

func init() { registerSpec(settingsSpecs) }

func settingsSpecs() []opSpec {
	const t = "settings"
	return []opSpec{
		{Method: http.MethodGet, Path: "/api/v1/settings", ID: "getSettings", Tag: t, Summary: "Get all settings", Out: SettingsResponse{}},
		{Method: http.MethodPut, Path: "/api/v1/settings", ID: "updateSettings", Tag: t, Summary: "Partially update settings (omitted fields are left alone)", Body: settingsBody{}, Out: SettingsResponse{}},
		{Method: http.MethodPost, Path: "/api/v1/settings/test/notify/{channel_id}", ID: "testNotifyChannel", Tag: t, Summary: "Send a test notification through a channel",
			Params: []p{{Name: "channel_id", Type: "string", Doc: "Notification channel ID", Required: true}}, Out: ConnectionTest{}, Errors: []int{503}},
		{Method: http.MethodPost, Path: "/api/v1/settings/test/{target}", ID: "testIntegration", Tag: t, Summary: "Test connectivity of an integration",
			Params: []p{{Name: "target", Type: "string", Doc: "What to test", Required: true, Enum: []any{"vm", "vl", "oidc"}}},
			Body:   TestIntegrationRequest{}, OptionalBody: true, Out: ConnectionTest{}},
		{Method: http.MethodGet, Path: "/api/v1/settings/tokens", ID: "listTokens", Tag: t, Summary: "List API tokens", Out: TokenList{}},
		{Method: http.MethodPost, Path: "/api/v1/settings/tokens", ID: "createToken", Tag: t, Summary: "Create an API token", Body: CreateTokenRequest{}, Status: 201, Out: CreatedToken{}},
		{Method: http.MethodDelete, Path: "/api/v1/settings/tokens/{id}", ID: "deleteToken", Tag: t, Summary: "Revoke an API token", Params: []p{idParam("Token ID")}, Errors: []int{409}},
	}
}
