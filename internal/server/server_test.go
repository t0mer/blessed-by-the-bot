package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/config"
	"github.com/t0mer/blessed-by-the-bot/internal/logging"
	"github.com/t0mer/blessed-by-the-bot/internal/server"
)

func newServer(t *testing.T) *server.Server {
	t.Helper()
	s, err := server.New(server.Options{
		Config:  &config.Config{Port: 0, DataDir: t.TempDir(), LogLevel: "error", Dev: true},
		Logger:  logging.New("error", false),
		Version: "2026.8.0",
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
