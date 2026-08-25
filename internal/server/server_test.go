package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/config"
	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
	"github.com/t0mer/blessed-by-the-bot/internal/logging"
	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/server"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// newServer builds a fully wired server backed by a temp-file SQLite store.
func newServer(t *testing.T) *server.Server {
	t.Helper()

	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	return newServerWithStore(t, st)
}

// newServerWithStore lets a test supply its own store, which the health checks
// need so they can close it out from under the server.
func newServerWithStore(t *testing.T, st *store.Store) *server.Server {
	t.Helper()

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	cipher, err := crypto.New(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	svc, err := settings.New(st, cipher)
	if err != nil {
		t.Fatalf("new settings service: %v", err)
	}

	s, err := server.New(server.Options{
		Config: &config.Config{Port: 0, DataDir: t.TempDir(), LogLevel: "error", Dev: true},
		// Silent: these tests provoke 503s and rejected webhooks on purpose.
		Logger:    logging.NewTo(io.Discard, "error", false),
		Version:   "2026.8.0",
		Store:     st,
		Settings:  svc,
		Providers: provider.NewManager(),
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return s
}

func TestHealthz(t *testing.T) {
	for _, path := range []string{"/healthz", "/api/v1/healthz"} {
		rec := httptest.NewRecorder()
		newServer(t).Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, rec.Code)
		}
		var body struct {
			Status  string `json:"status"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s body is not JSON: %v", path, err)
		}
		if body.Status != "ok" {
			t.Errorf("%s status field = %q, want ok", path, body.Status)
		}
		if body.Version != "2026.8.0" {
			t.Errorf("%s version = %q, want 2026.8.0", path, body.Version)
		}
	}
}

func TestUnknownAPIPathIs404JSON(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t).Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type = %q, want JSON", ct)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body is not the error envelope: %v", err)
	}
	if env.Error.Code != "not_found" {
		t.Errorf("code = %q, want not_found", env.Error.Code)
	}
}

func TestUnknownUIPathServesSPA(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t).Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Errorf("did not serve the SPA index: %q", rec.Body.String())
	}
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	s := newServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil after clean shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s of context cancellation")
	}
}

func TestHealthzReportsDatabase(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	s := newServerWithStore(t, st)

	rec := httptest.NewRecorder()
	s.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Status   string `json:"status"`
		Database string `json:"database"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Database != "ok" {
		t.Errorf("status=%q database=%q, want both ok", body.Status, body.Database)
	}
}

func TestHealthzIsUnhealthyWhenDatabaseIsClosed(t *testing.T) {
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	// The settings service is built before the close so the server can be
	// constructed at all; the health probe then hits the closed handle.
	s := newServerWithStore(t, st)

	rec := httptest.NewRecorder()
	s.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when the database is unreachable", rec.Code)
	}
}

func TestServerMountsTheAPIAndWebhooks(t *testing.T) {
	srv := newServer(t)

	cases := []struct {
		path       string
		method     string
		wantStatus int
	}{
		{"/healthz", http.MethodGet, http.StatusOK},
		{"/api/v1/healthz", http.MethodGet, http.StatusOK},
		{"/api/v1/contacts", http.MethodGet, http.StatusOK},
		{"/api/v1/blessings", http.MethodGet, http.StatusOK},
		{"/api/v1/groups", http.MethodGet, http.StatusOK},
		{"/api/v1/wish-patterns", http.MethodGet, http.StatusOK},
		{"/api/v1/settings", http.MethodGet, http.StatusOK},
		{"/api/v1/history", http.MethodGet, http.StatusOK},
		// No provider is configured in this test server.
		{"/api/v1/provider/status", http.MethodGet, http.StatusServiceUnavailable},
		// No signature, so GOWA fails closed.
		{"/webhooks/gowa", http.MethodPost, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			rec := httptest.NewRecorder()
			srv.Router().ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
