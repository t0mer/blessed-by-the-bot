package provider_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

func fastTransport() *provider.Transport {
	return provider.NewTransport(provider.TransportOptions{BaseBackoff: time.Millisecond})
}

func TestDoSucceedsFirstAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"idMessage":"abc123"}`))
	}))
	defer srv.Close()

	var out struct {
		IDMessage string `json:"idMessage"`
	}
	if err := fastTransport().Do(context.Background(), http.MethodGet, srv.URL, nil, nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if out.IDMessage != "abc123" {
		t.Errorf("idMessage = %q", out.IDMessage)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestDoRetriesOn500ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	if err := fastTransport().Do(context.Background(), http.MethodGet, srv.URL, nil, nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Errorf("%d requests, want 3", n)
	}
}

func TestDoGivesUpAfterAttempts(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := fastTransport().Do(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
	var apiErr *provider.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.Status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", apiErr.Status)
	}
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Errorf("%d requests, want 3", n)
	}
}

func TestDoDoesNotRetry4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	}))
	defer srv.Close()

	err := fastTransport().Do(context.Background(), http.MethodGet, srv.URL, nil, nil, nil)
	var apiErr *provider.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.Status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", apiErr.Status)
	}
	// Retrying a rejected credential just burns the provider's rate limit.
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("%d requests, want exactly 1 — 4xx must never be retried", n)
	}
}

type countingRoundTripper struct {
	calls int32
}

func (c *countingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	atomic.AddInt32(&c.calls, 1)
	return nil, errors.New("simulated network failure")
}

func TestDoRetriesNetworkErrors(t *testing.T) {
	rt := &countingRoundTripper{}
	tr := provider.NewTransport(provider.TransportOptions{
		Client:      &http.Client{Transport: rt},
		BaseBackoff: time.Millisecond,
	})

	if err := tr.Do(context.Background(), http.MethodGet, "http://example.invalid", nil, nil, nil); err == nil {
		t.Fatal("expected an error")
	}
	if n := atomic.LoadInt32(&rt.calls); n != 3 {
		t.Errorf("%d attempts, want 3", n)
	}
}

func TestDoSendsJSONBodyAndContentType(t *testing.T) {
	type payload struct {
		ChatID  string `json:"chatId"`
		Message string `json:"message"`
	}
	got := make(chan payload, 1)
	var contentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		var p payload
		if err := decodeJSON(r, &p); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		got <- p
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	in := payload{ChatID: "972501234567@c.us", Message: "hi"}
	if err := fastTransport().Do(context.Background(), http.MethodPost, srv.URL, nil, in, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if contentType != "application/json" {
		t.Errorf("content-type = %q", contentType)
	}
	if received := <-got; received != in {
		t.Errorf("body = %+v, want %+v", received, in)
	}
}

func TestDoSendsCustomHeaders(t *testing.T) {
	var auth, device string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		device = r.Header.Get("X-Device-Id")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	h := http.Header{}
	h.Set("Authorization", "Basic dXNlcjpwYXNz")
	h.Set("X-Device-Id", "device-1")
	if err := fastTransport().Do(context.Background(), http.MethodGet, srv.URL, h, nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if auth != "Basic dXNlcjpwYXNz" {
		t.Errorf("authorization = %q", auth)
	}
	if device != "device-1" {
		t.Errorf("device header = %q", device)
	}
}

func TestDoRespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := provider.NewTransport(provider.TransportOptions{BaseBackoff: 10 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := tr.Do(ctx, http.MethodGet, srv.URL, nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v; a cancelled context must not wait out the backoff", elapsed)
	}
}

func TestDoTolerates204AndEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	out := struct {
		Value string `json:"value"`
	}{Value: "untouched"}
	if err := fastTransport().Do(context.Background(), http.MethodGet, srv.URL, nil, nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if out.Value != "untouched" {
		t.Errorf("out mutated to %q on an empty body", out.Value)
	}
}

func TestBackoffGrowsAndIsJittered(t *testing.T) {
	const base = 100 * time.Millisecond

	var prevCeiling time.Duration
	for attempt := 0; attempt < 3; attempt++ {
		ceiling := base * (1 << attempt)
		seen := make(map[time.Duration]bool)
		for i := 0; i < 20; i++ {
			d := provider.BackoffFor(attempt, base)
			if d < ceiling/2 || d > ceiling {
				t.Fatalf("attempt %d: delay %v outside [%v, %v]", attempt, d, ceiling/2, ceiling)
			}
			seen[d] = true
		}
		if len(seen) == 1 {
			t.Errorf("attempt %d: 20 samples were all identical; jitter is missing", attempt)
		}
		if ceiling <= prevCeiling {
			t.Errorf("attempt %d: ceiling %v did not grow past %v", attempt, ceiling, prevCeiling)
		}
		prevCeiling = ceiling
	}
}
