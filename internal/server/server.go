// Package server wires the chi router, middleware chain and HTTP lifecycle.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/t0mer/blessed-by-the-bot/internal/config"
	"github.com/t0mer/blessed-by-the-bot/internal/handlers"
	"github.com/t0mer/blessed-by-the-bot/internal/metrics"
	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
	"github.com/t0mer/blessed-by-the-bot/internal/webui"
)

// Options are the dependencies New needs.
type Options struct {
	Config    *config.Config
	Logger    *slog.Logger
	Version   string
	Store     *store.Store
	Settings  *settings.Service
	Providers *provider.Manager

	// Rebuild re-applies the configuration to the active provider after a
	// settings change. Nil leaves the running provider alone.
	Rebuild func(ctx context.Context, s *settings.Settings) error

	// Sender backs POST /contacts/{id}/send-now. Nil leaves that endpoint
	// answering 501.
	Sender handlers.Sender

	// Incoming consumes normalized provider webhook messages. Nil leaves the
	// webhook endpoints verifying and logging without a consumer.
	Incoming handlers.IncomingHandler
}

// Server owns the HTTP router and listener lifecycle. Route handling lives in
// internal/handlers; this type is wiring only.
type Server struct {
	cfg     *config.Config
	log     *slog.Logger
	api     *handlers.API
	version string // for the startup log line only; the API owns /healthz
	router  chi.Router
	addr    string
}

// New builds a Server with its routes registered.
func New(opts Options) (*Server, error) {
	if opts.Config == nil {
		return nil, errors.New("server: config is required")
	}
	if opts.Logger == nil {
		return nil, errors.New("server: logger is required")
	}

	api, err := handlers.New(handlers.Deps{
		Store:     opts.Store,
		Settings:  opts.Settings,
		Providers: opts.Providers,
		Logger:    opts.Logger,
		Version:   opts.Version,
		Rebuild:   opts.Rebuild,
		Sender:    opts.Sender,
		Incoming:  opts.Incoming,
	})
	if err != nil {
		return nil, err
	}

	s := &Server{cfg: opts.Config, log: opts.Logger, api: api, version: opts.Version}
	if err := s.routes(); err != nil {
		return nil, err
	}
	return s, nil
}

// Router exposes the handler for tests and for embedding.
func (s *Server) Router() http.Handler { return s.router }

func (s *Server) routes() error {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// middleware.RealIP is deliberately not used: it rewrites RemoteAddr from
	// client-controlled headers (GHSA-3fxj-6jh8-hvhx) and nothing here needs the
	// client IP. Revisit only behind a proxy with a trusted-header allowlist.
	r.Use(s.requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Root health for the container healthcheck; the same handler also serves
	// /api/v1/healthz inside the API router.
	r.Get("/healthz", s.api.Health)

	r.Mount("/api/v1", s.api.Routes())
	r.Mount("/webhooks", s.api.WebhookRoutes())

	// Prometheus scrape target. Deliberately outside /api/v1: it is an operator
	// surface, not part of the versioned API the SPA consumes.
	metrics.Init()
	r.Handle("/metrics", promhttp.Handler())

	ui, err := webui.Handler()
	if err != nil {
		return fmt.Errorf("building ui handler: %w", err)
	}
	// Anything not matched above is a client-side route: serve the SPA so a deep
	// link such as /settings survives a refresh.
	r.NotFound(ui.ServeHTTP)

	s.router = r
	return nil
}

// Run listens on the configured port and blocks until ctx is cancelled, then
// drains in-flight requests.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Port))
	if err != nil {
		return fmt.Errorf("listening on port %d: %w", s.cfg.Port, err)
	}
	s.addr = ln.Addr().String()

	srv := &http.Server{
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		s.log.Info("http server listening", "addr", s.addr, "version", s.version)
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errc <- serveErr
			return
		}
		errc <- nil
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		s.log.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down: %w", err)
		}
		<-errc
		return nil
	}
}

// Addr reports the bound address once Run has started listening.
func (s *Server) Addr() string { return s.addr }

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.log.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}
