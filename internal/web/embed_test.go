package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandlerReturns503WhenUINotBuilt(t *testing.T) {
	h := handlerFor(fstest.MapFS{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body := rec.Body.String(); body != "UI not built\n" {
		t.Errorf("body = %q, want \"UI not built\\n\"", body)
	}
}

func builtFS() fs.FS {
	return fstest.MapFS{
		"index.html":           {Data: []byte("<html>app</html>")},
		"assets/app-abc123.js": {Data: []byte("console.log(1)")},
		"favicon.svg":          {Data: []byte("<svg/>")},
	}
}

func TestHandlerServesIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerFor(builtFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "<html>app</html>" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestHandlerSPAFallbackForUnknownRoute(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerFor(builtFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/results/42", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "<html>app</html>" {
		t.Errorf("fallback body = %q, want index.html", rec.Body.String())
	}
}

func TestAssetsGetImmutableCacheHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerFor(builtFS()).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := "public, max-age=31536000, immutable"
	if got := rec.Header().Get("Cache-Control"); got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}

func TestIndexIsNotCachedImmutably(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerFor(builtFS()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
}

func TestMissingAssetIsNotSPAFallback(t *testing.T) {
	rec := httptest.NewRecorder()
	handlerFor(builtFS()).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
