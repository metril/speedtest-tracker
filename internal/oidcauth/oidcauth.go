// Package oidcauth implements OpenID Connect login: provider discovery,
// the authorization-code + PKCE flow, ID token verification and claim
// based authorisation (allowed groups/emails, admin group).
package oidcauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// ErrForbidden is returned by Config.Authorize when the caller's claims
// fail the allowed-groups or allowed-emails check.
var ErrForbidden = errors.New("oidcauth: forbidden")

// Config configures an OIDC provider and the authorisation rules applied
// to the claims it returns.
type Config struct {
	Issuer          string
	ClientID        string
	ClientSecret    string
	RedirectBaseURL string
	Scopes          []string
	GroupsClaim     string
	AdminGroup      string
	AllowedGroups   []string
	AllowedEmails   []string
	SessionTTL      time.Duration
}

// Claims is the identity extracted from a verified ID token.
type Claims struct {
	Subject string
	Email   string
	Name    string
	Groups  []string
}

// Provider is a discovered OIDC provider ready to run the authorization
// code flow.
type Provider struct {
	cfg      Config
	client   *http.Client
	verifier *oidc.IDTokenVerifier
	oauthCfg oauth2.Config
}

// clientContext associates client with ctx for both go-oidc (discovery,
// JWKS fetch) and oauth2 (token exchange) HTTP calls, when client is
// non-nil.
func clientContext(ctx context.Context, client *http.Client) context.Context {
	if client == nil {
		return ctx
	}
	ctx = oidc.ClientContext(ctx, client)
	return context.WithValue(ctx, oauth2.HTTPClient, client)
}

// New discovers the provider at cfg.Issuer and returns a Provider ready to
// build authorization URLs and exchange codes. client, when non-nil, is
// used for every HTTP call the provider makes (discovery, JWKS, token
// exchange) — tests use this to point at a fake IdP.
func New(ctx context.Context, cfg Config, client *http.Client) (*Provider, error) {
	ctx = clientContext(ctx, client)
	p, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidcauth: discover %q: %w", cfg.Issuer, err)
	}
	verifier := p.VerifierContext(ctx, &oidc.Config{ClientID: cfg.ClientID})

	return &Provider{
		cfg:      cfg,
		client:   client,
		verifier: verifier,
		oauthCfg: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     p.Endpoint(),
			Scopes:       dedupScopes(cfg.Scopes),
		},
	}, nil
}

// Discover performs discovery only, without keeping the result — used to
// validate an issuer URL (e.g. from a settings form) before saving it.
func Discover(ctx context.Context, issuer string, client *http.Client) error {
	ctx = clientContext(ctx, client)
	if _, err := oidc.NewProvider(ctx, issuer); err != nil {
		return fmt.Errorf("oidcauth: discover %q: %w", issuer, err)
	}
	return nil
}

// dedupScopes returns "openid profile email" plus extra, each scope kept
// once, in first-seen order.
func dedupScopes(extra []string) []string {
	seen := make(map[string]bool, 3+len(extra))
	out := make([]string, 0, 3+len(extra))
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add("openid")
	add("profile")
	add("email")
	for _, s := range extra {
		add(s)
	}
	return out
}

// Config returns the configuration the Provider was built with.
func (p *Provider) Config() Config { return p.cfg }

// AuthCodeURL returns the URL to redirect the browser to, beginning an
// authorization-code + PKCE (S256) flow.
func (p *Provider) AuthCodeURL(state, verifier, redirectURI string) string {
	cfg := p.oauthCfg
	cfg.RedirectURL = redirectURI
	return cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

// Exchange trades an authorization code for tokens, verifies the returned
// ID token and decodes its claims.
func (p *Provider) Exchange(ctx context.Context, code, verifier, redirectURI string) (Claims, error) {
	ctx = clientContext(ctx, p.client)
	cfg := p.oauthCfg
	cfg.RedirectURL = redirectURI

	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Claims{}, fmt.Errorf("oidcauth: exchange: %w", err)
	}
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return Claims{}, errors.New("oidcauth: exchange: no id_token in token response")
	}
	idTok, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return Claims{}, fmt.Errorf("oidcauth: verify id_token: %w", err)
	}

	var raw map[string]any
	if err := idTok.Claims(&raw); err != nil {
		return Claims{}, fmt.Errorf("oidcauth: decode claims: %w", err)
	}

	claims := Claims{Subject: idTok.Subject}
	if v, ok := raw["email"].(string); ok {
		claims.Email = v
	}
	if v, ok := raw["name"].(string); ok {
		claims.Name = v
	}
	claims.Groups = extractGroups(raw, p.cfg.GroupsClaim)
	return claims, nil
}

// extractGroups reads claim from raw, accepting a JSON array of strings,
// a JSON array of arbitrary values (non-string entries are dropped) or a
// space-or-comma separated string. A missing claim yields nil.
func extractGroups(raw map[string]any, claim string) []string {
	if claim == "" {
		return nil
	}
	v, ok := raw[claim]
	if !ok {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		var out []string
		for _, part := range strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ' ' }) {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

// Authorize checks cl against c's allowed-groups/allowed-emails rules and
// reports whether cl belongs to the admin group. It returns ErrForbidden
// when a non-empty AllowedGroups has no overlap with cl.Groups, or a
// non-empty AllowedEmails does not contain cl.Email (case-insensitively).
func (c Config) Authorize(cl Claims) (isAdmin bool, err error) {
	if len(c.AllowedGroups) > 0 && !intersects(cl.Groups, c.AllowedGroups) {
		return false, ErrForbidden
	}
	if len(c.AllowedEmails) > 0 && !containsFold(c.AllowedEmails, cl.Email) {
		return false, ErrForbidden
	}
	isAdmin = c.AdminGroup == "" || slices.Contains(cl.Groups, c.AdminGroup)
	return isAdmin, nil
}

func intersects(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

func containsFold(list []string, target string) bool {
	for _, s := range list {
		if strings.EqualFold(s, target) {
			return true
		}
	}
	return false
}
