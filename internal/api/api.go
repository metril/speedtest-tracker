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
	"github.com/metril/speedtest-tracker/internal/engine"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/sse"
	"github.com/metril/speedtest-tracker/internal/store"
)

// Pinger reports whether the datastore is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// NextRunner reports a registered schedule's next fire time. The running
// scheduler is the authority: it knows which schedules are actually
// registered, and its cron entries account for the reload history that a
// bare expression parse cannot see.
type NextRunner interface {
	Next(scheduleID int64) (time.Time, bool)
}

// Deps are the router's collaborators.
type Deps struct {
	Pinger     Pinger
	Logger     *slog.Logger
	UI         http.Handler
	Hub        *sse.Hub
	Store      *store.Store
	Registry   *engine.Registry
	Runner     Runner
	ServerList ServerLister

	// OoklaSearch widens GET /ookla/servers with a remote speedtest.net
	// search when a query is given. Optional: nil serves the local list
	// only.
	OoklaSearch ServerSearcher

	// ReloadSchedules asks the scheduler to rebuild its cron entries after
	// a schedule mutation. Optional: nil means no scheduler is running.
	ReloadSchedules func(context.Context) error

	// Scheduler reports registered schedules' next fire times. Optional:
	// nil falls back to parsing the schedule's cron expression.
	Scheduler NextRunner

	// MetricsHandler serves Prometheus exposition at /metrics. The route
	// exists whenever it is non-nil but answers 404 unless MetricsEnabled
	// reports true, so the toggle takes effect live without a restart.
	MetricsHandler http.Handler
	MetricsEnabled func() bool

	// Metrics counts cache behaviour. Optional: nil disables counting.
	Metrics CacheMetrics

	// Settings is the typed settings store backing GET/PUT
	// /api/v1/settings. Optional: nil means those routes are not mounted.
	Settings *settings.Store

	// Iperf3 triggers an immediate refresh for POST
	// /api/v1/iperf3/servers/refresh. Optional: nil makes that route
	// answer 503; GET /api/v1/iperf3/servers still works either way.
	Iperf3 Iperf3Refresher

	// TestClient is the HTTP client used to probe VM/VL endpoints for
	// POST /api/v1/settings/test/{target}. Optional: nil means a client
	// with a probeTimeout timeout is used.
	TestClient *http.Client

	// Notifier probes one notification channel for
	// POST /api/v1/settings/test/notify/{channel_id}. Optional: nil means
	// that route answers 503.
	Notifier ChannelTester

	// Auth resolves the identity behind each /api/v1 request. Optional:
	// nil leaves every route open, which is what the existing handler
	// tests and the `run --engine` CLI path rely on.
	Auth Authenticator

	// summary caches /stats/summary bodies; New fills it in.
	summary *summaryCache
}

// Authenticator gates the /api/v1 tree. Mode reports the configured mode
// so GET /api/v1/me can answer without a second settings read.
type Authenticator interface {
	Handler(next http.Handler) http.Handler
	Mode() string
}

// ChannelTester probes one notification channel for
// POST /api/v1/settings/test/notify/{channel_id}.
type ChannelTester interface {
	TestChannel(ctx context.Context, ch settings.Channel) error
}

// CacheMetrics records /stats/summary cache behaviour.
type CacheMetrics interface {
	SummaryCacheHit()
	SummaryCacheMiss()
}

// requestTimeout bounds every /api/v1 request except the SSE stream.
const requestTimeout = 30 * time.Second

// csvExportTimeout bounds /api/v1/results.csv, which runs outside the
// normal v1 timeout group because a large export can legitimately take
// longer than requestTimeout.
const csvExportTimeout = 5 * time.Minute

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

	if deps.MetricsHandler != nil {
		r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
			if deps.MetricsEnabled != nil && !deps.MetricsEnabled() {
				errNotFound(w, "metrics endpoint disabled")
				return
			}
			deps.MetricsHandler.ServeHTTP(w, req)
		})
	}

	// authMW gates the /api/v1 tree and every route mounted alongside it
	// (SSE, the CSV export). Deps.Auth nil leaves every route open, which
	// is what the existing handler tests and the `run --engine` CLI path
	// rely on. Forgetting to apply authMW to a route mounted outside the
	// v1.Use call below is how a data endpoint silently stays public.
	authMW := func(next http.Handler) http.Handler { return next }
	if deps.Auth != nil {
		authMW = deps.Auth.Handler
	}

	// SSE lives outside the timeout group: the stream never ends on its own.
	if deps.Hub != nil {
		r.With(authMW).Get("/api/v1/events", sse.Handler(deps.Hub))
	}

	// The CSV export can run long on a large dataset, so it gets its own,
	// much longer timeout instead of sharing the 30s v1 group (which would
	// silently truncate the file mid-stream).
	if deps.Store != nil {
		r.With(authMW, middleware.Timeout(csvExportTimeout)).Get("/api/v1/results.csv", deps.resultsCSV)
	}

	r.Route("/api/v1", func(v1 chi.Router) {
		v1.Use(middleware.Timeout(requestTimeout))
		v1.Use(authMW)
		// /me must work even on a store-less router (e.g. the `run
		// --engine` CLI path), since the SPA always calls it first to
		// decide what to render.
		v1.Get("/me", deps.me)
		if deps.Store == nil {
			return
		}
		if deps.summary == nil {
			deps.summary = newSummaryCache(summaryTTL)
		}
		v1.Route("/targets", func(t chi.Router) {
			t.Get("/", etagJSON(deps.listTargets))
			t.Post("/", deps.createTarget)
			t.Post("/test", deps.testTarget)
			// Registered before "/{id}" so chi's router does not treat
			// "deleted" as an {id} value.
			t.Route("/deleted", func(dl chi.Router) {
				dl.Get("/", deps.listDeletedTargets)
				dl.Post("/{id}/restore", deps.restoreDeletedTarget)
			})
			t.Get("/{id}", deps.getTarget)
			t.Put("/{id}", deps.updateTarget)
			t.Delete("/{id}", deps.deleteTarget)
			t.Post("/{id}/run", deps.runTarget)
			t.Get("/{id}/latest", deps.targetLatest)
			t.Get("/{id}/history", deps.targetHistory)
			t.Get("/{id}/revisions", deps.listTargetRevisions)
			t.Post("/{id}/revisions/{version}/revert", deps.revertTargetRevision)
		})
		v1.Route("/schedules", func(s chi.Router) {
			s.Get("/", etagJSON(deps.listSchedules))
			s.Post("/", deps.createSchedule)
			s.Post("/validate", deps.validateCron)
			s.Get("/{id}", deps.getSchedule)
			s.Put("/{id}", deps.updateSchedule)
			s.Delete("/{id}", deps.deleteSchedule)
			s.Post("/{id}/run", deps.runSchedule)
			s.Get("/{id}/next", deps.scheduleNext)
		})
		v1.Get("/ookla/servers", deps.listOoklaServers)
		v1.Route("/iperf3/servers", func(ip chi.Router) {
			ip.Get("/", deps.listIperf3Servers)
			ip.Post("/refresh", deps.refreshIperf3Servers)
		})
		v1.Route("/runs", func(rt chi.Router) {
			rt.Get("/", etagJSON(deps.listRuns))
			rt.Post("/", deps.createRun)
			rt.Get("/{id}", deps.getRun)
			rt.Delete("/{id}", deps.cancelRun)
		})
		v1.Route("/results", func(rs chi.Router) {
			rs.Get("/", etagJSON(deps.listResults))
			rs.Get("/{id}", deps.getResult)
			rs.Delete("/{id}", deps.deleteResult)
			rs.Post("/{id}/reexecute", deps.reexecuteResult)
			rs.Put("/{id}/tags", deps.setResultTags)
		})
		v1.Route("/tags", func(tg chi.Router) {
			tg.Get("/", deps.listTags)
			tg.Put("/{id}", deps.renameTag)
			tg.Delete("/{id}", deps.deleteTag)
		})
		v1.Get("/outages", deps.outages)
		v1.Get("/stats/summary", deps.statsSummary)
		if deps.Settings != nil {
			v1.Get("/settings", deps.getSettings)
			v1.Put("/settings", deps.putSettings)
			v1.Post("/settings/test/notify/{channel_id}", deps.testNotifyChannel)
			v1.Post("/settings/test/{target}", deps.testIntegration)
			v1.Get("/settings/tokens", deps.listTokens)
			v1.Post("/settings/tokens", deps.createToken)
			v1.Delete("/settings/tokens/{id}", deps.deleteToken)
		}
	})

	// The SPA is not gated: it is a static shell that fetches
	// /api/v1/me itself and renders a sign-in hint on 401.
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
				level := slog.LevelInfo
				switch {
				case ww.Status() >= 500:
					level = slog.LevelError
				case ww.Status() >= 400:
					level = slog.LevelWarn
				}
				logger.Log(r.Context(), level, "http request",
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
