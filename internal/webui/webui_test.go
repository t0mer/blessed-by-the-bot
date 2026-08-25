package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

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
	// Assert only what holds whether or not the frontend has been built: the
	// placeholder and the real index are both HTML documents.
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Errorf("body is not an HTML document: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("content-type = %q, want text/html", ct)
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

// A binary built before the frontend must still answer, and say what to do,
// rather than 404 with no explanation.
func TestFallsBackToThePlaceholderWhenNotBuilt(t *testing.T) {
	handler, err := webui.HandlerFor(fstest.MapFS{})
	if err != nil {
		t.Fatalf("building handler: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "UI not built") {
		t.Fatalf("body = %q, want the placeholder", rec.Body.String())
	}
}
