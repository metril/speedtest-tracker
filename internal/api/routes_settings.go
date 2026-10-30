package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (d Deps) registerSettings(api huma.API, all bool) {
	if !all && d.Settings == nil {
		return
	}
	const t = "settings"
	register(api, route{Method: http.MethodGet, Path: "/api/v1/settings", ID: "getSettings", Tag: t, Summary: "Get all settings"}, d.getSettings)
	register(api, route{Method: http.MethodPut, Path: "/api/v1/settings", ID: "updateSettings", Tag: t, Summary: "Partially update settings (omitted fields are left alone)"}, d.putSettings)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/settings/test/notify/{channel_id}", ID: "testNotifyChannel", Tag: t, Summary: "Send a test notification through a channel", Errors: []int{503}}, d.testNotifyChannel)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/settings/test/{target}", ID: "testIntegration", Tag: t, Summary: "Test connectivity of an integration", Pre: testTargetPre}, d.testIntegration)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/settings/tokens", ID: "listTokens", Tag: t, Summary: "List API tokens"}, d.listTokens)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/settings/tokens", ID: "createToken", Tag: t, Summary: "Create an API token", Status: 201, Pre: adminPre(true)}, d.createToken)
	register(api, route{Method: http.MethodDelete, Path: "/api/v1/settings/tokens/{id}", ID: "deleteToken", Tag: t, Summary: "Revoke an API token", Status: 204, Errors: []int{409}, Pre: adminPre(false)}, d.deleteToken)
}

// adminPre rejects a non-admin caller (and, when noToken, a token-authed
// one) before the request is parsed, in the order the chi handlers did.
func adminPre(noToken bool) func(huma.Context) (int, string, string) {
	return func(ctx huma.Context) (int, string, string) {
		if !requestIsAdmin(ctx.Context()) {
			return http.StatusForbidden, "forbidden", "admin access required"
		}
		if noToken && requestAuthIsToken(ctx.Context()) {
			return http.StatusForbidden, "forbidden", "creating tokens requires a forward-auth or open-mode session"
		}
		return 0, "", ""
	}
}

// testTargetPre answers 404 for an unknown {target} before the body is read.
func testTargetPre(ctx huma.Context) (int, string, string) {
	switch t := ctx.Param("target"); t {
	case "vm", "vl", "oidc":
		return 0, "", ""
	default:
		return http.StatusNotFound, "not_found", "unknown test target " + t
	}
}
