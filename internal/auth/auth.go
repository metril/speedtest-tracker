// Package auth resolves the identity behind an HTTP request. It supports
// three modes: open (no authentication), forward_auth (identity headers
// set by a reverse proxy, honoured only when the peer is inside a
// configured trusted-proxy CIDR) and token (a hashed API token presented
// as a bearer credential). Configuration is swapped live from the
// settings store; the config mutex is never held across a token lookup.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/metril/speedtest-tracker/internal/settings"
)

// ErrUnauthorized is returned by Identify when the request carries no
// acceptable credential. Any other error means identity could not be
// determined (e.g. the token store is unavailable) and must not be
// treated as "not authenticated".
var ErrUnauthorized = errors.New("auth: unauthorized")

// TokenLookup is the narrow dependency the auth package needs from the
// API token store, so this package never imports internal/store.
type TokenLookup interface {
	LookupToken(ctx context.Context, hash string) (id int64, ok bool, err error)
	TouchToken(ctx context.Context, id int64) error
}

// Identity is the resolved caller of a request.
type Identity struct {
	Mode    string
	User    string
	Groups  []string
	IsAdmin bool
	TokenID int64
	// Source names the credential that produced this identity: SourceOpen,
	// SourceForward or SourceToken. Unlike Mode (the configured mode, which
	// in forward_auth can still be satisfied by a bearer token when
	// AllowTokens is set), Source always names the actual method used, so
	// callers that must distinguish "authenticated via a proxy header" from
	// "authenticated via a bearer/query token" — e.g. the auth-settings
	// write guard — can rely on it regardless of the configured mode.
	Source string
}

// Identity.Source values.
const (
	SourceOpen    = "open"
	SourceForward = "forward"
	SourceToken   = "token"
)

type identityCtxKey struct{}

// NewContext returns a copy of ctx carrying id.
func NewContext(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityCtxKey{}, id)
}

// FromContext returns the Identity stored in ctx, or the zero Identity if
// none was set.
func FromContext(ctx context.Context) Identity {
	id, _ := ctx.Value(identityCtxKey{}).(Identity)
	return id
}

// config is the parsed, validated form of settings.Auth.
type config struct {
	mode         string
	userHeader   string
	groupsHeader string
	groupsSep    string
	adminGroup   string
	trusted      []netip.Prefix
	allowTokens  bool
}

// touchQueueCap bounds the number of pending token-touch updates. It is
// sized generously above expected steady-state traffic; when it is full a
// touch is dropped (the token's last-used timestamp is best-effort and
// self-heals on the next request) rather than blocking the request
// goroutine.
const touchQueueCap = 64

// Middleware resolves identity for incoming requests.
type Middleware struct {
	logger *slog.Logger
	tokens TokenLookup
	now    func() time.Time

	mu  sync.Mutex
	cfg config

	touchMu sync.Mutex
	touched map[int64]time.Time

	touchCh   chan int64
	touchDone chan struct{}
	touchWG   sync.WaitGroup
	closeOnce sync.Once

	warnedUnknownMode sync.Once
}

// New returns a Middleware and starts its single background worker that
// applies token-touch updates off the request goroutine. Configure must be
// called before use; an unconfigured Middleware treats every request as
// unauthorized. Call Close to stop the worker cleanly on shutdown.
func New(logger *slog.Logger, tokens TokenLookup, now func() time.Time) *Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Middleware{
		logger:    logger,
		tokens:    tokens,
		now:       now,
		touched:   map[int64]time.Time{},
		touchCh:   make(chan int64, touchQueueCap),
		touchDone: make(chan struct{}),
	}
	m.touchWG.Add(1)
	go m.touchWorker()
	return m
}

// touchWorker is the single goroutine that applies TouchToken calls, so a
// burst of authenticated requests never spawns unbounded concurrent writes.
func (m *Middleware) touchWorker() {
	defer m.touchWG.Done()
	for {
		select {
		case id := <-m.touchCh:
			m.doTouch(id)
		case <-m.touchDone:
			return
		}
	}
}

// doTouch actually calls TouchToken on a detached context so a cancelled
// request (or the shutdown of the worker itself) still lets an in-flight
// touch complete without blocking on any particular caller's deadline.
func (m *Middleware) doTouch(id int64) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 2*time.Second)
	defer cancel()
	if err := m.tokens.TouchToken(ctx, id); err != nil {
		m.logger.Warn("auth: touch token failed", "token_id", id, "err", err)
	}
}

// Close stops the touch worker, waiting for it to exit. Any touch update
// still queued when Close is called is dropped. Safe to call once; further
// calls are no-ops.
func (m *Middleware) Close() {
	m.closeOnce.Do(func() { close(m.touchDone) })
	m.touchWG.Wait()
}

// Configure parses cfg and, if every CIDR is valid, atomically swaps it in
// as the active configuration. On error the previous configuration is
// left untouched.
func (m *Middleware) Configure(cfg settings.Auth) error {
	trusted := make([]netip.Prefix, 0, len(cfg.TrustedProxies))
	for _, raw := range cfg.TrustedProxies {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return fmt.Errorf("auth: invalid trusted proxy CIDR %q: %w", raw, err)
		}
		trusted = append(trusted, p)
	}

	next := config{
		mode:         cfg.Mode,
		userHeader:   cfg.UserHeader,
		groupsHeader: cfg.GroupsHeader,
		groupsSep:    cfg.GroupsSeparator,
		adminGroup:   cfg.AdminGroup,
		trusted:      trusted,
		allowTokens:  cfg.AllowTokens,
	}

	m.mu.Lock()
	m.cfg = next
	m.mu.Unlock()
	return nil
}

func (m *Middleware) snapshot() config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Identify determines the caller of r under the current configuration.
func (m *Middleware) Identify(r *http.Request) (Identity, error) {
	cfg := m.snapshot()

	switch cfg.mode {
	case settings.AuthModeOpen:
		return Identity{Mode: settings.AuthModeOpen, IsAdmin: true, Source: SourceOpen}, nil

	case settings.AuthModeForward:
		if plain, ok := bearerToken(r); ok && cfg.allowTokens {
			return m.identifyToken(r.Context(), cfg.mode, plain)
		}
		return m.identifyForward(r, cfg)

	case settings.AuthModeToken:
		plain, ok := credentialToken(r)
		if !ok {
			return Identity{}, ErrUnauthorized
		}
		return m.identifyToken(r.Context(), cfg.mode, plain)

	default:
		m.warnedUnknownMode.Do(func() {
			m.logger.Warn("auth: unknown mode, treating as token", "mode", cfg.mode)
		})
		plain, ok := credentialToken(r)
		if !ok {
			return Identity{}, ErrUnauthorized
		}
		return m.identifyToken(r.Context(), cfg.mode, plain)
	}
}

func (m *Middleware) identifyForward(r *http.Request, cfg config) (Identity, error) {
	if len(cfg.trusted) == 0 {
		return Identity{}, ErrUnauthorized
	}
	addrPort, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return Identity{}, ErrUnauthorized
	}
	addr := addrPort.Addr().Unmap()
	trusted := false
	for _, p := range cfg.trusted {
		if p.Contains(addr) {
			trusted = true
			break
		}
	}
	if !trusted {
		return Identity{}, ErrUnauthorized
	}

	user := r.Header.Get(cfg.userHeader)
	if user == "" {
		return Identity{}, ErrUnauthorized
	}

	var groups []string
	if cfg.groupsHeader != "" {
		raw := r.Header.Get(cfg.groupsHeader)
		if raw != "" {
			sep := cfg.groupsSep
			if sep == "" {
				sep = ","
			}
			for _, g := range strings.Split(raw, sep) {
				g = strings.TrimSpace(g)
				if g != "" {
					groups = append(groups, g)
				}
			}
		}
	}

	isAdmin := cfg.adminGroup == "" || slices.Contains(groups, cfg.adminGroup)

	return Identity{
		Mode:    settings.AuthModeForward,
		User:    user,
		Groups:  groups,
		IsAdmin: isAdmin,
		Source:  SourceForward,
	}, nil
}

// identifyToken looks up plain's hash and, on a hit, throttle-touches the
// token's last-used timestamp on a detached context so a cancelled
// request still records usage without blocking on the caller's deadline.
func (m *Middleware) identifyToken(ctx context.Context, mode, plain string) (Identity, error) {
	hash := HashToken(plain)
	id, ok, err := m.tokens.LookupToken(ctx, hash)
	if err != nil {
		return Identity{}, fmt.Errorf("auth: token lookup: %w", err)
	}
	if !ok {
		return Identity{}, ErrUnauthorized
	}

	m.maybeTouch(id)

	label := plain
	if len(label) > 12 {
		label = label[:12]
	}

	// Tokens are full-access by design: there is no per-token scoping,
	// so a valid token always resolves to an admin identity.
	return Identity{
		Mode:    mode,
		User:    "token:" + label,
		IsAdmin: true,
		TokenID: id,
		Source:  SourceToken,
	}, nil
}

// maybeTouch coalesces touches per token id to at most one per minute, then
// hands the actual write off to touchWorker via a bounded channel so it
// never runs on the request goroutine. A full queue drops the touch rather
// than blocking the caller.
func (m *Middleware) maybeTouch(id int64) {
	now := m.now()

	m.touchMu.Lock()
	last, seen := m.touched[id]
	due := !seen || now.Sub(last) >= time.Minute
	if due {
		m.touched[id] = now
	}
	m.touchMu.Unlock()

	if !due {
		return
	}

	select {
	case m.touchCh <- id:
	default:
		m.logger.Warn("auth: touch queue full, dropping", "token_id", id)
	}
}

// bearerToken extracts a token from the Authorization header only.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return h[len(prefix):], true
}

// credentialToken extracts a token from the Authorization header, or, only
// for /api/v1/events, the "token" query parameter (EventSource cannot set
// headers).
func credentialToken(r *http.Request) (string, bool) {
	if t, ok := bearerToken(r); ok {
		return t, true
	}
	if r.URL.Path == "/api/v1/events" {
		if t := r.URL.Query().Get("token"); t != "" {
			return t, true
		}
	}
	return "", false
}

// HashToken returns the hex-encoded SHA-256 digest of plaintext.
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// GenerateToken creates a new random API token, returning its plaintext,
// its hash and a display prefix.
func GenerateToken() (plaintext, hash, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("auth: generate token: %w", err)
	}
	plaintext = "stt_" + base64.RawURLEncoding.EncodeToString(buf)
	hash = HashToken(plaintext)
	prefix = plaintext
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	return plaintext, hash, prefix, nil
}

// Handler wraps next, resolving identity and rejecting unauthorized or
// errored requests before it runs.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := m.Identify(r)
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				w.Header().Set("WWW-Authenticate", "Bearer")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"auth unavailable"}`))
			return
		}
		next.ServeHTTP(w, r.WithContext(NewContext(r.Context(), id)))
	})
}
