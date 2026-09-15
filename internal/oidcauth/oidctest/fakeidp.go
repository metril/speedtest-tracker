// Package oidctest provides a minimal fake OpenID Connect provider for
// tests: discovery, JWKS and token endpoints backed by an RSA key
// generated per instance.
package oidctest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

const keyID = "test-key"

// IDP is a fake OIDC provider serving discovery, JWKS and token endpoints
// over httptest. Tests set what claims a given authorization code yields
// via Issue, then drive the real oidcauth.Provider against IDP.URL.
type IDP struct {
	URL          string
	ClientID     string
	ClientSecret string

	server *httptest.Server
	key    *rsa.PrivateKey

	mu    sync.Mutex
	codes map[string]map[string]any
}

// NewIDP starts a fake IdP and registers its shutdown with t.Cleanup.
func NewIDP(t testing.TB) *IDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("oidctest: generate key: %v", err)
	}
	idp := &IDP{
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		key:          key,
		codes:        map[string]map[string]any{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", idp.discovery)
	mux.HandleFunc("/jwks", idp.jwks)
	mux.HandleFunc("/token", idp.token)
	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	idp.URL = idp.server.URL
	return idp
}

// Issue registers the claims that redeeming code at the token endpoint
// will yield in the signed ID token. Standard claims (iss, aud, exp, iat)
// are filled in automatically unless claims overrides them; sub defaults
// to "test-subject" unless claims sets it.
func (idp *IDP) Issue(code string, claims map[string]any) {
	idp.mu.Lock()
	defer idp.mu.Unlock()
	idp.codes[code] = claims
}

// Client returns an *http.Client that trusts the fake IdP's httptest
// server (needed only for TLS test servers; httptest.NewServer's plain
// HTTP client already works, this is provided for symmetry/future TLS
// use).
func (idp *IDP) Client() *http.Client {
	return idp.server.Client()
}

func (idp *IDP) discovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                idp.URL,
		"authorization_endpoint":                idp.URL + "/authorize",
		"token_endpoint":                        idp.URL + "/token",
		"jwks_uri":                              idp.URL + "/jwks",
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"scopes_supported":                      []string{"openid", "profile", "email"},
	})
}

func (idp *IDP) jwks(w http.ResponseWriter, r *http.Request) {
	jwk := jose.JSONWebKey{Key: &idp.key.PublicKey, KeyID: keyID, Algorithm: "RS256", Use: "sig"}
	writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}})
}

func (idp *IDP) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	code := r.FormValue("code")
	if r.FormValue("code_verifier") == "" {
		http.Error(w, "missing code_verifier", http.StatusBadRequest)
		return
	}

	idp.mu.Lock()
	claims, ok := idp.codes[code]
	idp.mu.Unlock()
	if !ok {
		http.Error(w, "invalid_grant", http.StatusBadRequest)
		return
	}

	now := time.Now()
	merged := map[string]any{
		"iss": idp.URL,
		"sub": "test-subject",
		"aud": idp.ClientID,
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	}
	for k, v := range claims {
		merged[k] = v
	}

	payload, err := json.Marshal(merged)
	if err != nil {
		http.Error(w, "encode claims", http.StatusInternalServerError)
		return
	}

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: idp.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID),
	)
	if err != nil {
		http.Error(w, "build signer", http.StatusInternalServerError)
		return
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		http.Error(w, "sign id_token", http.StatusInternalServerError)
		return
	}
	idToken, err := jws.CompactSerialize()
	if err != nil {
		http.Error(w, "serialize id_token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"access_token": "test-access-token",
		"token_type":   "bearer",
		"id_token":     idToken,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
