package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
)

// stubAuth injects a fixed identity, or answers 401 when id is nil.
type stubAuth struct{ id *auth.Identity }

func (s stubAuth) Mode() string { return "forward_auth" }
func (s stubAuth) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.id == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.NewContext(r.Context(), *s.id)))
	})
}

func newParityAPI(t *testing.T, a Authenticator) http.Handler {
	t.Helper()
	h, _, _ := newTestAPIWith(t, func(d *Deps) {
		s, err := settings.New(context.Background(), d.Store)
		if err != nil {
			t.Fatal(err)
		}
		d.Settings = s
		d.Auth = a
		d.UI = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("spa")) })
	})
	return h
}

func send(h http.Handler, method, path, ct, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func wantErr(t *testing.T, rec *httptest.ResponseRecorder, status int, code, msg string) {
	t.Helper()
	if rec.Code != status || !strings.Contains(rec.Body.String(), `"code":"`+code+`"`) || !strings.Contains(rec.Body.String(), strings.ReplaceAll(msg, `"`, `\"`)) {
		t.Fatalf("got %d %s; want %d %s %q", rec.Code, rec.Body, status, code, msg)
	}
}

func TestWrongMethodOnAPIRoute(t *testing.T) {
	t.Parallel()
	h := newParityAPI(t, nil)
	for _, c := range [][2]string{{"PATCH", "/api/v1/targets"}, {"HEAD", "/api/v1/me"}, {"DELETE", "/api/v1/me"}} {
		rec := send(h, c[0], c[1], "", "")
		if rec.Code != http.StatusMethodNotAllowed || rec.Body.Len() != 0 {
			t.Errorf("%s %s = %d %q, want empty 405", c[0], c[1], rec.Code, rec.Body)
		}
	}
	if rec := send(h, "GET", "/api/v1/nope", "", ""); rec.Code != 200 || rec.Body.String() != "spa" {
		t.Errorf("unknown /api/v1 path = %d %q, want SPA", rec.Code, rec.Body)
	}
	if rec := send(h, "PATCH", "/elsewhere", "", ""); rec.Code != http.StatusOK || rec.Body.String() != "spa" {
		t.Errorf("non-api path = %d %q", rec.Code, rec.Body)
	}
}

func TestWrongMethodAndUnknownPassAuthFirst(t *testing.T) {
	t.Parallel()
	h := newParityAPI(t, stubAuth{})
	for _, c := range [][2]string{{"PATCH", "/api/v1/targets"}, {"GET", "/api/v1/nope"}} {
		if rec := send(h, c[0], c[1], "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", c[0], c[1], rec.Code)
		}
	}
}

func TestSettingsTestTargetValidation(t *testing.T) {
	t.Parallel()
	h := newParityAPI(t, nil)
	wantErr(t, send(h, "POST", "/api/v1/settings/test/bogus", "application/json", "{not json"), 404, "not_found", "unknown test target bogus")
	wantErr(t, send(h, "POST", "/api/v1/settings/test/oidc", "application/json", `{"url":"http://x"}`), 400, "invalid_request", `malformed JSON body: json: unknown field "url"`)
	wantErr(t, send(h, "POST", "/api/v1/settings/test/oidc", "application/json", `{"auth_header":"x"}`), 400, "invalid_request", `unknown field "auth_header"`)
	wantErr(t, send(h, "POST", "/api/v1/settings/test/vm", "application/json", `{"issuer":"x"}`), 400, "invalid_request", `unknown field "issuer"`)
	wantErr(t, send(h, "POST", "/api/v1/settings/test/vl", "application/json", `{"client_secret":"x"}`), 400, "invalid_request", `unknown field "client_secret"`)
}

func TestHumaErrorParity(t *testing.T) {
	t.Parallel()
	h := newParityAPI(t, nil)
	wantErr(t, send(h, "POST", "/api/v1/targets/1/revisions/abc/revert", "", ""), 400, "invalid_request", "version must be a positive integer")
	wantErr(t, send(h, "GET", "/api/v1/targets/abc", "", ""), 400, "invalid_request", "id must be a positive integer")
	big := `{"name":"` + strings.Repeat("a", 2<<20) + `"}`
	wantErr(t, send(h, "POST", "/api/v1/targets", "application/json", big), 400, "invalid_request", "malformed JSON body: http: request body too large")
	wantErr(t, send(h, "POST", "/api/v1/targets", "text/plain", `{}`), 415, "unsupported_media_type", "Content-Type must be application/json")
	wantErr(t, send(h, "POST", "/api/v1/targets", "application/json; charset=utf-8", `{`), 400, "invalid_request", "malformed JSON body")
}

func TestTokenChecksBeforeBody(t *testing.T) {
	t.Parallel()
	viewer := newParityAPI(t, stubAuth{&auth.Identity{Mode: "forward_auth", Source: auth.SourceForward}})
	wantErr(t, send(viewer, "POST", "/api/v1/settings/tokens", "application/json", "{bad"), 403, "forbidden", "admin group required")
	wantErr(t, send(viewer, "DELETE", "/api/v1/settings/tokens/abc", "", ""), 403, "forbidden", "admin group required")
	tok := newParityAPI(t, stubAuth{&auth.Identity{Mode: "forward_auth", IsAdmin: true, Source: auth.SourceToken}})
	wantErr(t, send(tok, "POST", "/api/v1/settings/tokens", "application/json", "{bad"), 403, "forbidden", "creating tokens requires")
}
