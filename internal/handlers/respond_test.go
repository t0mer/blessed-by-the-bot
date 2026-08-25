package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func decodeEnvelope(t *testing.T, body io.Reader) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decoding error envelope: %v", err)
	}
	return env
}

func TestWriteErrorMapsKnownErrorsToStatusCodes(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantSlug string
	}{
		{"not found", store.ErrNotFound, http.StatusNotFound, codeNotFound},
		{"wrapped not found", fmt.Errorf("loading contact: %w", store.ErrNotFound), http.StatusNotFound, codeNotFound},
		{"no provider", provider.ErrNoProvider, http.StatusServiceUnavailable, codeProviderUnavailable},
		{"api error", errorf(http.StatusTeapot, "brewing", "no coffee here"), http.StatusTeapot, "brewing"},
		{"unknown", errors.New("boom"), http.StatusInternalServerError, codeInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeError(rec, testLogger(), tc.err)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			if got := decodeEnvelope(t, rec.Body).Error.Code; got != tc.wantSlug {
				t.Fatalf("code = %q, want %q", got, tc.wantSlug)
			}
		})
	}
}

func TestWriteErrorHidesInternalDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, testLogger(), errors.New("dsn=secret password=hunter2"))

	body := rec.Body.String()
	if strings.Contains(body, "hunter2") {
		t.Fatalf("internal error detail leaked to the client: %s", body)
	}
	if got := decodeEnvelope(t, strings.NewReader(body)).Error.Message; got == "" {
		t.Fatal("want a generic message, got an empty one")
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"a","nope":1}`))
	rec := httptest.NewRecorder()

	var dst struct {
		Name string `json:"name"`
	}
	err := decodeJSON(rec, req, &dst)
	if err == nil {
		t.Fatal("want an error for an unknown field, got nil")
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("want a 400 apiError, got %#v", err)
	}
}

func TestDecodeJSONRejectsOversizedBodies(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"name":"`+strings.Repeat("x", maxBodyBytes+1)+`"}`))
	rec := httptest.NewRecorder()

	var dst struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(rec, req, &dst); err == nil {
		t.Fatal("want an error for an oversized body, got nil")
	}
}

func TestDecodeJSONRejectsTrailingContent(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"a"}{"name":"b"}`))
	rec := httptest.NewRecorder()

	var dst struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(rec, req, &dst); err == nil {
		t.Fatal("want an error for trailing content, got nil")
	}
}

func TestHealthReportsDatabaseStatus(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/healthz", nil)
	requireStatus(t, rec, http.StatusOK)

	var payload map[string]string
	decodeInto(t, rec, &payload)
	if payload["status"] != "ok" || payload["database"] != "ok" {
		t.Fatalf("unexpected health payload: %#v", payload)
	}
	if payload["version"] != "test" {
		t.Fatalf("version = %q, want %q", payload["version"], "test")
	}
}

func TestUnknownAPIPathReturnsJSONEnvelope(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/nope", nil)
	requireStatus(t, rec, http.StatusNotFound)
	if got := errorCode(t, rec); got != codeNotFound {
		t.Fatalf("code = %q, want %q", got, codeNotFound)
	}
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	if _, err := New(Deps{}); err == nil {
		t.Fatal("want an error for empty deps, got nil")
	}
}
