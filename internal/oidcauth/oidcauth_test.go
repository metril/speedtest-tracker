package oidcauth

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/oidcauth/oidctest"
)

func newProvider(t *testing.T, idp *oidctest.IDP, cfg Config) *Provider {
	t.Helper()
	cfg.Issuer = idp.URL
	cfg.ClientID = idp.ClientID
	cfg.ClientSecret = idp.ClientSecret
	p, err := New(context.Background(), cfg, idp.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestDiscover(t *testing.T) {
	idp := oidctest.NewIDP(t)
	if err := Discover(context.Background(), idp.URL, idp.Client()); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if err := Discover(context.Background(), idp.URL+"/wrong", idp.Client()); err == nil {
		t.Fatal("Discover of a bad issuer succeeded")
	}
}

func TestAuthCodeURLHasPKCEChallenge(t *testing.T) {
	idp := oidctest.NewIDP(t)
	p := newProvider(t, idp, Config{GroupsClaim: "groups"})

	u := p.AuthCodeURL("state-1", "verifier-1", "https://app.example/auth/oidc/callback")
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Fatalf("AuthCodeURL = %s, missing PKCE challenge", u)
	}
	if q.Get("state") != "state-1" {
		t.Fatalf("state = %q", q.Get("state"))
	}
	if !strings.Contains(q.Get("scope"), "openid") {
		t.Fatalf("scope = %q, want openid", q.Get("scope"))
	}
}

func TestExchangeGroupsFromList(t *testing.T) {
	idp := oidctest.NewIDP(t)
	p := newProvider(t, idp, Config{GroupsClaim: "groups"})

	idp.Issue("code-list", map[string]any{
		"sub":                "user-1",
		"email":              "user@example.com",
		"name":               "User One",
		"preferred_username": "user1",
		"groups":             []any{"admins", "users"},
	})

	claims, err := p.Exchange(context.Background(), "code-list", "verifier-1", "https://app.example/auth/oidc/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if claims.Subject != "user-1" || claims.Email != "user@example.com" || claims.Name != "User One" || claims.PreferredUsername != "user1" {
		t.Fatalf("claims = %+v", claims)
	}
	if len(claims.Groups) != 2 || claims.Groups[0] != "admins" || claims.Groups[1] != "users" {
		t.Fatalf("groups = %v", claims.Groups)
	}
}

func TestExchangeGroupsFromString(t *testing.T) {
	idp := oidctest.NewIDP(t)
	p := newProvider(t, idp, Config{GroupsClaim: "groups"})

	idp.Issue("code-str", map[string]any{
		"sub":    "user-2",
		"groups": "admins, users",
	})
	claims, err := p.Exchange(context.Background(), "code-str", "verifier-1", "https://app.example/auth/oidc/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if len(claims.Groups) != 2 || claims.Groups[0] != "admins" || claims.Groups[1] != "users" {
		t.Fatalf("groups = %v", claims.Groups)
	}
}

func TestExchangeMissingGroupsClaim(t *testing.T) {
	idp := oidctest.NewIDP(t)
	p := newProvider(t, idp, Config{GroupsClaim: "groups"})

	idp.Issue("code-none", map[string]any{"sub": "user-3"})
	claims, err := p.Exchange(context.Background(), "code-none", "verifier-1", "https://app.example/auth/oidc/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if len(claims.Groups) != 0 {
		t.Fatalf("groups = %v, want empty", claims.Groups)
	}
}

func TestExchangeUnknownCode(t *testing.T) {
	idp := oidctest.NewIDP(t)
	p := newProvider(t, idp, Config{})
	if _, err := p.Exchange(context.Background(), "no-such-code", "v", "https://app.example/auth/oidc/callback"); err == nil {
		t.Fatal("Exchange of an unknown code succeeded")
	}
}

func TestAuthorizeMatrix(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		claims     Claims
		wantAdmin  bool
		wantForbid bool
	}{
		{"no restrictions, no admin group", Config{}, Claims{Groups: []string{"x"}}, true, false},
		{"admin group matched", Config{AdminGroup: "admins"}, Claims{Groups: []string{"admins", "x"}}, true, false},
		{"admin group not matched", Config{AdminGroup: "admins"}, Claims{Groups: []string{"x"}}, false, false},
		{"allowed groups matched", Config{AllowedGroups: []string{"users"}}, Claims{Groups: []string{"users"}}, true, false},
		{"allowed groups not matched", Config{AllowedGroups: []string{"users"}}, Claims{Groups: []string{"other"}}, false, true},
		{"allowed emails matched case-insensitive", Config{AllowedEmails: []string{"A@Example.com"}}, Claims{Email: "a@example.com"}, true, false},
		{"allowed emails not matched", Config{AllowedEmails: []string{"a@example.com"}}, Claims{Email: "b@example.com"}, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isAdmin, err := tc.cfg.Authorize(tc.claims)
			if tc.wantForbid {
				if err != ErrForbidden {
					t.Fatalf("err = %v, want ErrForbidden", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if isAdmin != tc.wantAdmin {
				t.Fatalf("isAdmin = %v, want %v", isAdmin, tc.wantAdmin)
			}
		})
	}
}

func TestStateCodecRoundTrip(t *testing.T) {
	c, err := NewStateCodec()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	enc := c.Encode("state-1", "verifier-1", "/dashboard")
	state, verifier, returnTo, err := c.Decode(enc, now)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if state != "state-1" || verifier != "verifier-1" || returnTo != "/dashboard" {
		t.Fatalf("decoded = %q %q %q", state, verifier, returnTo)
	}
}

func TestStateCodecExpiry(t *testing.T) {
	c, err := NewStateCodec()
	if err != nil {
		t.Fatal(err)
	}
	enc := c.Encode("s", "v", "/r")
	if _, _, _, err := c.Decode(enc, time.Now().Add(6*time.Minute)); err != ErrStateInvalid {
		t.Fatalf("err = %v, want ErrStateInvalid", err)
	}
}

func TestStateCodecTamper(t *testing.T) {
	c, err := NewStateCodec()
	if err != nil {
		t.Fatal(err)
	}
	enc := c.Encode("s", "v", "/r")
	tampered := enc[:len(enc)-1] + "x"
	if _, _, _, err := c.Decode(tampered, time.Now()); err != ErrStateInvalid {
		t.Fatalf("err = %v, want ErrStateInvalid", err)
	}

	other, err := NewStateCodec()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := other.Decode(enc, time.Now()); err != ErrStateInvalid {
		t.Fatalf("cross-key decode err = %v, want ErrStateInvalid", err)
	}
}

func TestStateCodecMalformed(t *testing.T) {
	c, err := NewStateCodec()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "no-dot-here", "abc.def", "!!!.!!!"} {
		if _, _, _, err := c.Decode(s, time.Now()); err != ErrStateInvalid {
			t.Errorf("Decode(%q) err = %v, want ErrStateInvalid", s, err)
		}
	}
}
