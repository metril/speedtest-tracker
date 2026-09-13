// Package api builds the HTTP router: operational endpoints, the /api/v1
// tree and the SPA fallback.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/metril/speedtest-tracker/internal/sse"
)

// Pinger reports whether the datastore is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the router's collaborators.
type Deps struct {
	Pinger Pinger
	Logger *slog.Logger
	UI     http.Handler
	Hub    *sse.Hub
}

// requestTimeout bounds every /api/v1 request except the SSE stream.
const requestTimeout = 30 * time.Second

// New builds the root HTTP handler.
func New(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(securityHeaders)
	r.Use(requestLogger(deps.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(setRequestIDHeader)

	r.Get("/healthz", healthz(deps.Pinger))

	// SSE lives outside the timeout group: the stream never ends on its own.
	if deps.Hub != nil {
		r.Get("/api/v1/events", sse.Handler(deps.Hub))
	}

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Use(middleware.Timeout(requestTimeout))
	})

	if deps.UI != nil {
		r.NotFound(deps.UI.ServeHTTP)
	}
	return r
}

func setRequestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-Id", id)
		}
		next.ServeHTTP(w, r)
	})
}

func healthz(p Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := p.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy","error":"database unreachable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}
}

// requestLogger logs one structured line per request via slog.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			defer func() {
				logger.Info("http request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"bytes", ww.BytesWritten(),
					"duration_ms", time.Since(start).Milliseconds(),
					"remote", r.RemoteAddr,
					"request_id", middleware.GetReqID(r.Context()),
				)
			}()
			next.ServeHTTP(ww, r)
		})
	}
}
