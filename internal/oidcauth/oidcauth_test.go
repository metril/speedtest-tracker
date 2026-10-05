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

	// No userinfo configured on the fake: the fallback 404s, which must
	// not fail the login, only be reported on the claims.
	idp.Issue("code-none", map[string]any{"sub": "user-3"})
	claims, err := p.Exchange(context.Background(), "code-none", "verifier-1", "https://app.example/auth/oidc/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if len(claims.Groups) != 0 {
		t.Fatalf("groups = %v, want empty", claims.Groups)
	}
	if claims.UserInfoErr == nil {
		t.Fatal("UserInfoErr = nil, want the failed userinfo fetch reported")
	}
	if n := idp.Requests("/userinfo"); n != 1 {
		t.Fatalf("userinfo requests = %d, want 1", n)
	}
}

func TestExchangeGroupsFromUserInfo(t *testing.T) {
	idp := oidctest.NewIDP(t)
	p := newProvider(t, idp, Config{GroupsClaim: "groups"})

	// Authentik with "Include claims in ID token" off: the ID token has
	// only sub/email, everything else comes from userinfo.
	idp.Issue("code-ui", map[string]any{"sub": "user-4", "email": "idtoken@example.com"})
	idp.SetUserInfo(map[string]any{
		"sub":                "user-4",
		"email":              "userinfo@example.com",
		"name":               "User Four",
		"preferred_username": "user4",
		"groups":             []any{"localadmin"},
	})
	claims, err := p.Exchange(context.Background(), "code-ui", "verifier-1", "https://app.example/auth/oidc/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if claims.UserInfoErr != nil {
		t.Fatalf("UserInfoErr = %v", claims.UserInfoErr)
	}
	if len(claims.Groups) != 1 || claims.Groups[0] != "localadmin" {
		t.Fatalf("groups = %v, want [localadmin]", claims.Groups)
	}
	if claims.Email != "idtoken@example.com" {
		t.Fatalf("email = %q, want the ID token value to win", claims.Email)
	}
	if claims.Name != "User Four" || claims.PreferredUsername != "user4" {
		t.Fatalf("claims = %+v, want name/username filled from userinfo", claims)
	}
}

func TestExchangeSkipsUserInfoWhenIDTokenHasGroups(t *testing.T) {
	idp := oidctest.NewIDP(t)
	p := newProvider(t, idp, Config{GroupsClaim: "groups"})

	idp.Issue("code-skip", map[string]any{"sub": "user-5", "groups": []any{}})
	idp.SetUserInfo(map[string]any{"sub": "user-5", "groups": []any{"localadmin"}})
	claims, err := p.Exchange(context.Background(), "code-skip", "verifier-1", "https://app.example/auth/oidc/callback")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if len(claims.Groups) != 0 {
		t.Fatalf("groups = %v, want the (empty) ID token claim", claims.Groups)
	}
	if n := idp.Requests("/userinfo"); n != 0 {
		t.Fatalf("userinfo requests = %d, want 0", n)
	}
}

func TestExchangeUserInfoSubjectMismatch(t *testing.T) {
	idp := oidctest.NewIDP(t)
	p := newProvider(t, idp, Config{GroupsClaim: "groups"})

	idp.Issue("code-sub", map[string]any{"sub": "user-6"})
	idp.SetUserInfo(map[string]any{"sub": "someone-else", "groups": []any{"localadmin"}})
	if _, err := p.Exchange(context.Background(), "code-sub", "verifier-1", "https://app.example/auth/oidc/callback"); err == nil {
		t.Fatal("Exchange accepted a userinfo document for a different subject")
	}
}

func TestExchangeEmailVerified(t *testing.T) {
	idp := oidctest.NewIDP(t)
	// AllowedEmails is set: that is the only rule email_verified gates.
	p := newProvider(t, idp, Config{AllowedEmails: []string{"a@example.com"}})
	for name, tc := range map[string]struct {
		claims     map[string]any
		wantForbid bool
	}{
		"false":   {map[string]any{"sub": "u", "email": "a@example.com", "email_verified": false}, true},
		"true":    {map[string]any{"sub": "u", "email": "a@example.com", "email_verified": true}, false},
		"missing": {map[string]any{"sub": "u", "email": "a@example.com"}, false},
	} {
		idp.Issue("code-"+name, tc.claims)
		cl, err := p.Exchange(context.Background(), "code-"+name, "v", "https://app.example/auth/oidc/callback")
		if err != nil {
			t.Fatalf("%s: Exchange: %v", name, err)
		}
		_, err = p.Config().Authorize(cl)
		if (err == ErrForbidden) != tc.wantForbid {
			t.Fatalf("%s: Authorize err = %v, wantForbid %v", name, err, tc.wantForbid)
		}
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
		{"allowed groups matched case-insensitive", Config{AllowedGroups: []string{"LocalAdmin"}}, Claims{Groups: []string{"localadmin"}}, true, false},
		{"admin group matched case-insensitive", Config{AdminGroup: "LocalAdmin"}, Claims{Groups: []string{"localadmin"}}, true, false},
		{"missing groups with allowed groups", Config{AllowedGroups: []string{"users"}}, Claims{}, false, true},
		{"allowed emails matched case-insensitive", Config{AllowedEmails: []string{"A@Example.com"}}, Claims{Email: "a@example.com"}, true, false},
		{"email unverified, no restrictions", Config{}, Claims{Email: "a@example.com", EmailUnverified: true}, true, false},
		{"email unverified, allowed groups only", Config{AllowedGroups: []string{"localadmin"}, AdminGroup: "localadmin"}, Claims{Email: "a@example.com", EmailUnverified: true, Groups: []string{"localadmin"}}, true, false},
		{"email unverified with allowed emails", Config{AllowedEmails: []string{"a@example.com"}}, Claims{Email: "a@example.com", EmailUnverified: true}, false, true},
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
