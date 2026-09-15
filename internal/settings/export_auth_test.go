package settings

import (
	"net/http"
	"testing"
)

func TestExportAuthApply(t *testing.T) {
	cases := []struct {
		name string
		auth ExportAuth
		want string // expected Authorization header, or "" plus check below for custom
	}{
		{"none", ExportAuth{Type: ExportAuthNone}, ""},
		{"empty type", ExportAuth{}, ""},
		{"basic", ExportAuth{Type: ExportAuthBasic, Username: "u", Password: "p"}, "Basic dTpw"},
		{"bearer", ExportAuth{Type: ExportAuthBearer, Token: "tok"}, "Bearer tok"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
			c.auth.Apply(req)
			if got := req.Header.Get("Authorization"); got != c.want {
				t.Errorf("Authorization = %q, want %q", got, c.want)
			}
		})
	}

	t.Run("custom", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		ExportAuth{Type: ExportAuthCustom, HeaderName: "X-Api-Key", HeaderValue: "secret"}.Apply(req)
		if got := req.Header.Get("X-Api-Key"); got != "secret" {
			t.Errorf("X-Api-Key = %q, want secret", got)
		}
	})

	t.Run("custom no name is no-op", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		ExportAuth{Type: ExportAuthCustom, HeaderValue: "secret"}.Apply(req)
		if len(req.Header) != 0 {
			t.Errorf("headers = %v, want none", req.Header)
		}
	})

	t.Run("bearer with empty token is no-op", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		ExportAuth{Type: ExportAuthBearer}.Apply(req)
		if len(req.Header) != 0 {
			t.Errorf("headers = %v, want none", req.Header)
		}
	})

	t.Run("basic with empty username and password is no-op", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
		ExportAuth{Type: ExportAuthBasic}.Apply(req)
		if len(req.Header) != 0 {
			t.Errorf("headers = %v, want none", req.Header)
		}
	})
}

func TestExportAuthEmpty(t *testing.T) {
	cases := []struct {
		name string
		auth ExportAuth
		want bool
	}{
		{"none", ExportAuth{Type: ExportAuthNone}, true},
		{"zero value", ExportAuth{}, true},
		{"basic", ExportAuth{Type: ExportAuthBasic, Username: "u"}, false},
		{"bearer", ExportAuth{Type: ExportAuthBearer, Token: "t"}, false},
		{"custom with name", ExportAuth{Type: ExportAuthCustom, HeaderName: "X"}, false},
		{"custom without name", ExportAuth{Type: ExportAuthCustom}, true},
	}
	for _, c := range cases {
		if got := c.auth.Empty(); got != c.want {
			t.Errorf("%s: Empty() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestValidExportAuthType(t *testing.T) {
	for _, ok := range []string{ExportAuthNone, ExportAuthBasic, ExportAuthBearer, ExportAuthCustom} {
		if !ValidExportAuthType(ok) {
			t.Errorf("ValidExportAuthType(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "digest", "Basic"} {
		if ValidExportAuthType(bad) {
			t.Errorf("ValidExportAuthType(%q) = true, want false", bad)
		}
	}
}

func TestIntegrationsVMAuthLegacyFallback(t *testing.T) {
	i := Integrations{VMAuthHeader: "Bearer legacy-tok"}
	got := i.VMAuth()
	want := ExportAuth{Type: ExportAuthCustom, HeaderName: "Authorization", HeaderValue: "Bearer legacy-tok"}
	if got != want {
		t.Errorf("VMAuth() = %+v, want %+v", got, want)
	}
}

func TestIntegrationsVMAuthStructuredWinsOverLegacy(t *testing.T) {
	i := Integrations{VMAuthHeader: "Bearer legacy-tok", VMAuthType: ExportAuthBearer, VMAuthToken: "structured-tok"}
	got := i.VMAuth()
	want := ExportAuth{Type: ExportAuthBearer, Token: "structured-tok"}
	if got != want {
		t.Errorf("VMAuth() = %+v, want %+v", got, want)
	}
}

func TestIntegrationsVLAuthLegacyFallback(t *testing.T) {
	i := Integrations{VLAuthHeader: "Bearer legacy-tok"}
	got := i.VLAuth()
	want := ExportAuth{Type: ExportAuthCustom, HeaderName: "Authorization", HeaderValue: "Bearer legacy-tok"}
	if got != want {
		t.Errorf("VLAuth() = %+v, want %+v", got, want)
	}
}

func TestIntegrationsVLAuthNoneWhenUnset(t *testing.T) {
	if got := (Integrations{}).VLAuth(); !got.Empty() {
		t.Errorf("VLAuth() = %+v, want empty", got)
	}
}
