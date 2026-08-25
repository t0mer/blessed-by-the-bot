package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/webui"
)

func TestServesIndex(t *testing.T) {
	h, err := webui.Handler()
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "blessed-by-the-bot") {
		t.Errorf("body did not contain the index document: %q", rec.Body.String())
	}
}

func TestUnknownPathFallsBackToIndex(t *testing.T) {
	h, _ := webui.Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/contacts/42", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (SPA fallback)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Errorf("fallback did not serve index.html: %q", rec.Body.String())
	}
}
