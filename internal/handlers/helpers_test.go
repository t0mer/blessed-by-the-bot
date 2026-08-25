package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
	"github.com/t0mer/blessed-by-the-bot/internal/logging"
	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// testAPI bundles the API under test with the collaborators a test may want to
// inspect or drive.
type testAPI struct {
	*API
	store    *store.Store
	settings *settings.Service
	manager  *provider.Manager
	router   http.Handler
}

// testLogger is silent. Handler tests deliberately provoke 500s and rejected
// webhooks; letting those log would bury the actual test output.
func testLogger() *slog.Logger { return logging.NewTo(io.Discard, "error", false) }

// newTestAPI builds an API backed by a real temp-file SQLite store. A file is
// used rather than :memory: because WAL semantics differ and the store is
// configured for WAL.
func newTestAPI(t *testing.T, mutate ...func(*Deps)) *testAPI {
	t.Helper()

	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	cipher, err := crypto.New(key)
	if err != nil {
		t.Fatalf("building cipher: %v", err)
	}
	svc, err := settings.New(st, cipher)
	if err != nil {
		t.Fatalf("building settings service: %v", err)
	}

	mgr := provider.NewManager()
	deps := Deps{
		Store:     st,
		Settings:  svc,
		Providers: mgr,
		Logger:    testLogger(),
		Version:   "test",
	}
	for _, m := range mutate {
		m(&deps)
	}

	api, err := New(deps)
	if err != nil {
		t.Fatalf("building api: %v", err)
	}

	return &testAPI{API: api, store: st, settings: svc, manager: mgr, router: chiMux(api)}
}

// chiMux mounts the API exactly the way internal/server does — chi Mount, same
// prefixes — so tests exercise the real routing rather than a bespoke arrangement.
func chiMux(api *API) http.Handler {
	root := chi.NewRouter()
	root.Mount("/api/v1", api.Routes())
	root.Mount("/webhooks", api.WebhookRoutes())
	return root
}

// do issues a request against the mounted API. body may be nil, a string, or any
// value that will be marshalled to JSON.
func (ta *testAPI) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	switch v := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshalling request body: %v", err)
		}
		reader = strings.NewReader(string(raw))
	}

	req := httptest.NewRequest(method, path, reader)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	ta.router.ServeHTTP(rec, req)
	return rec
}

// decodeInto decodes a recorded JSON body, failing the test on malformed output.
func decodeInto(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decoding response %q: %v", rec.Body.String(), err)
	}
}

// requireStatus fails the test with the response body when the status is wrong,
// which turns an opaque "got 500 want 200" into an actionable message.
func requireStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body.String())
	}
}

// errorCode extracts the envelope's code, failing if the body is not an envelope.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env errorEnvelope
	decodeInto(t, rec, &env)
	return env.Error.Code
}
