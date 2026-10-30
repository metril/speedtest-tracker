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
	register(api, route{Method: http.MethodPost, Path: "/api/v1/settings/test/{target}", ID: "testIntegration", Tag: t, Summary: "Test connectivity of an integration"}, d.testIntegration)
	register(api, route{Method: http.MethodGet, Path: "/api/v1/settings/tokens", ID: "listTokens", Tag: t, Summary: "List API tokens"}, d.listTokens)
	register(api, route{Method: http.MethodPost, Path: "/api/v1/settings/tokens", ID: "createToken", Tag: t, Summary: "Create an API token", Status: 201}, d.createToken)
	register(api, route{Method: http.MethodDelete, Path: "/api/v1/settings/tokens/{id}", ID: "deleteToken", Tag: t, Summary: "Revoke an API token", Status: 204, Errors: []int{409}}, d.deleteToken)
}
