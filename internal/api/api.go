// Package api serves Cronus's HTTP surface: the versioned REST API under
// /api/v1, Prometheus /metrics, /healthz, and (in production) the embedded SPA.
// The UI is a pure client of the API. Handlers validate all input, cap request
// sizes, and set conservative timeouts and security headers.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/t0mer/cronus/internal/metrics"
	"github.com/t0mer/cronus/internal/ntp"
	"github.com/t0mer/cronus/internal/settings"
	"github.com/t0mer/cronus/internal/store"
)

// StoreAPI is the persistence surface the HTTP handlers require.
type StoreAPI interface {
	CreateServer(ctx context.Context, s store.Server) (store.Server, error)
	ListServers(ctx context.Context) ([]store.Server, error)
	GetServer(ctx context.Context, id string) (store.Server, error)
	UpdateServer(ctx context.Context, s store.Server) (store.Server, error)
	DeleteServer(ctx context.Context, id string) error
	Measurements(ctx context.Context, serverID string, from, to time.Time, limit int) ([]store.Measurement, error)
	Stats(ctx context.Context) (store.DBStats, error)
}

// EngineAPI runs on-demand tests, with an optional per-request sample count.
type EngineAPI interface {
	RunWithSamples(ctx context.Context, targets []string, samples int) []ntp.ServerResult
}

// SettingsAPI reads and updates runtime-editable settings.
type SettingsAPI interface {
	Get() settings.Values
	Update(ctx context.Context, v settings.Values) error
}

// Deps are the dependencies of the API server.
type Deps struct {
	Store            StoreAPI
	Engine           EngineAPI
	Settings         SettingsAPI
	Metrics          *metrics.Metrics
	OutlierThreshold time.Duration
	DefaultSamples   int
	Log              *slog.Logger
	StartTime        time.Time
	SchedulerRunning func() bool
	UI               http.Handler // optional embedded SPA; nil serves API only
}

// API is the HTTP server for Cronus.
type API struct {
	deps    Deps
	router  chi.Router
	limiter *rateLimiter
}

const (
	maxTestServers = 20
	maxBodyBytes   = 1 << 20 // 1 MiB
	requestTimeout = 60 * time.Second
)

// New builds the API with all routes and middleware wired.
func New(deps Deps) *API {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	if deps.DefaultSamples <= 0 {
		deps.DefaultSamples = 4
	}
	if deps.SchedulerRunning == nil {
		deps.SchedulerRunning = func() bool { return false }
	}
	a := &API{
		deps:    deps,
		limiter: newRateLimiter(10, time.Minute),
	}
	a.routes()
	return a
}

// Handler returns the root HTTP handler.
func (a *API) Handler() http.Handler { return a.router }

func (a *API) routes() {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// NOTE: middleware.RealIP is deliberately NOT used. Cronus is reached
	// directly (no trusted reverse proxy), so honouring client-supplied
	// X-Forwarded-For / X-Real-IP headers would let a caller forge its source
	// address and bypass the per-IP rate limit that bounds /test's UDP probes.
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(middleware.Timeout(requestTimeout))
	r.Use(a.requestLogger)

	r.Get("/healthz", a.handleHealthz)
	if a.deps.Metrics != nil {
		r.Handle("/metrics", a.deps.Metrics.Handler())
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.With(a.limiter.middleware).Post("/test", a.handleTest)

		r.Get("/servers", a.handleListServers)
		r.Post("/servers", a.handleCreateServer)
		r.Route("/servers/{id}", func(r chi.Router) {
			r.Get("/", a.handleGetServer)
			r.Put("/", a.handleUpdateServer)
			r.Delete("/", a.handleDeleteServer)
			r.Get("/measurements", a.handleMeasurements)
			r.Get("/drift", a.handleDrift)
		})

		r.Get("/status", a.handleStatus)

		if a.deps.Settings != nil {
			r.Get("/settings", a.handleGetSettings)
			r.Put("/settings", a.handleUpdateSettings)
		}
	})

	if a.deps.UI != nil {
		r.NotFound(a.deps.UI.ServeHTTP)
	}

	a.router = r
}

// securityHeaders sets conservative headers on every response, including the SPA.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (a *API) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		a.deps.Log.Debug("http",
			"method", r.Method, "path", r.URL.Path,
			"status", ww.Status(), "bytes", ww.BytesWritten(),
			"dur", time.Since(start).String())
	})
}

func (a *API) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
