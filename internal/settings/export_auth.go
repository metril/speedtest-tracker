package settings

import "net/http"

// Export auth types: how a request to an outbound integration (VictoriaMetrics,
// VictoriaLogs) authenticates itself.
const (
	ExportAuthNone   = "none"
	ExportAuthBasic  = "basic"
	ExportAuthBearer = "bearer"
	ExportAuthCustom = "custom"
)

// ExportAuth is the resolved auth to apply to an outbound export request.
type ExportAuth struct {
	Type        string
	Username    string
	Password    string
	Token       string
	HeaderName  string
	HeaderValue string
}

// Apply sets the auth-related header(s) on req according to a.Type. It is a
// no-op for ExportAuthNone, an empty Type, or a custom header with no name.
func (a ExportAuth) Apply(req *http.Request) {
	switch a.Type {
	case ExportAuthBasic:
		req.SetBasicAuth(a.Username, a.Password)
	case ExportAuthBearer:
		req.Header.Set("Authorization", "Bearer "+a.Token)
	case ExportAuthCustom:
		if a.HeaderName != "" {
			req.Header.Set(a.HeaderName, a.HeaderValue)
		}
	}
}

// Empty reports whether Apply would set no header at all.
func (a ExportAuth) Empty() bool {
	switch a.Type {
	case ExportAuthBasic, ExportAuthBearer:
		return false
	case ExportAuthCustom:
		return a.HeaderName == ""
	default:
		return true
	}
}

// ValidExportAuthType reports whether s is a recognized export auth type.
func ValidExportAuthType(s string) bool {
	switch s {
	case ExportAuthNone, ExportAuthBasic, ExportAuthBearer, ExportAuthCustom:
		return true
	default:
		return false
	}
}

// VMAuth resolves the effective export auth for VictoriaMetrics. When
// VMAuthType is unset ("") or "none" but the legacy VMAuthHeader is set, it
// falls back to a custom Authorization header for back-compat with installs
// configured before structured export auth existed.
func (i Integrations) VMAuth() ExportAuth {
	if (i.VMAuthType == "" || i.VMAuthType == ExportAuthNone) && i.VMAuthHeader != "" {
		return ExportAuth{Type: ExportAuthCustom, HeaderName: "Authorization", HeaderValue: i.VMAuthHeader}
	}
	return ExportAuth{
		Type:        i.VMAuthType,
		Username:    i.VMAuthUsername,
		Password:    i.VMAuthPassword,
		Token:       i.VMAuthToken,
		HeaderName:  i.VMAuthHeaderName,
		HeaderValue: i.VMAuthHeaderValue,
	}
}

// VLAuth resolves the effective export auth for VictoriaLogs, mirroring
// VMAuth's legacy fallback.
func (i Integrations) VLAuth() ExportAuth {
	if (i.VLAuthType == "" || i.VLAuthType == ExportAuthNone) && i.VLAuthHeader != "" {
		return ExportAuth{Type: ExportAuthCustom, HeaderName: "Authorization", HeaderValue: i.VLAuthHeader}
	}
	return ExportAuth{
		Type:        i.VLAuthType,
		Username:    i.VLAuthUsername,
		Password:    i.VLAuthPassword,
		Token:       i.VLAuthToken,
		HeaderName:  i.VLAuthHeaderName,
		HeaderValue: i.VLAuthHeaderValue,
	}
}
