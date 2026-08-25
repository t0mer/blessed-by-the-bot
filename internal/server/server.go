// Package server wires the chi router, middleware chain and HTTP lifecycle.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/t0mer/blessed-by-the-bot/internal/config"
	"github.com/t0mer/blessed-by-the-bot/internal/webui"
)

// Options are the dependencies New needs.
type Options struct {
	Config  *config.Config
	Logger  *slog.Logger
	Version string
}

// Server owns the HTTP router and listener lifecycle.
type Server struct {
	cfg     *config.Config
	log     *slog.Logger
	version string
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
	s := &Server{cfg: opts.Config, log: opts.Logger, version: opts.Version}
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
	r.Use(middleware.RealIP)
	r.Use(s.requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", s.handleHealth)

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/healthz", s.handleHealth)
		api.NotFound(notFoundJSON)
		api.MethodNotAllowed(methodNotAllowedJSON)
	})

	ui, err := webui.Handler()
	if err != nil {
		return fmt.Errorf("building ui handler: %w", err)
	}
	r.NotFound(ui.ServeHTTP)

	s.router = r
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": s.version,
	})
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func notFoundJSON(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, errorEnvelope{errorBody{"not_found", "resource not found"}})
}

func methodNotAllowedJSON(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusMethodNotAllowed, errorEnvelope{errorBody{"method_not_allowed", "method not allowed"}})
}
