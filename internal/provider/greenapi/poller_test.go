package greenapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/greenapi"
)

type pollerHarness struct {
	mu       sync.Mutex
	deleted  []string
	received []*provider.IncomingMessage
}

func (h *pollerHarness) recordDelete(path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deleted = append(h.deleted, path)
}

func (h *pollerHarness) handle(_ context.Context, msg *provider.IncomingMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.received = append(h.received, msg)
}

func (h *pollerHarness) snapshot() ([]string, []*provider.IncomingMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.deleted...), append([]*provider.IncomingMessage(nil), h.received...)
}

// runPoller starts the poller and stops it once cond holds or the deadline passes.
func runPoller(t *testing.T, p *greenapi.Poller, cond func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	deadline := time.After(3 * time.Second)
	for !cond() {
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatal("condition not met within 3s")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v, want nil on cancellation", err)
	}
}

func newPoller(t *testing.T, h *pollerHarness, handler http.HandlerFunc) *greenapi.Poller {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := greenapi.New(greenapi.Config{
		APIURL: srv.URL, IDInstance: testInstance, APIToken: testToken,
	}, provider.NewTransport(provider.TransportOptions{BaseBackoff: time.Millisecond}), nil)
	if err != nil {
		t.Fatal(err)
	}
	p := greenapi.NewPoller(c, nil, h.handle)
	p.SetIdleDelay(time.Millisecond)
	p.SetErrorDelay(time.Millisecond)
	return p
}

func TestPollerHandlesAndDeletesNotification(t *testing.T) {
	h := &pollerHarness{}
	var served bool
	var mu sync.Mutex

	p := newPoller(t, h, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "deleteNotification") {
			h.recordDelete(r.URL.Path)
			_, _ = w.Write([]byte(`{"result":true}`))
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if served {
			_, _ = w.Write([]byte(`null`))
			return
		}
		served = true
		_, _ = w.Write([]byte(`{"receiptId":4242,"body":` + textMessagePayload + `}`))
	})

	runPoller(t, p, func() bool {
		deleted, received := h.snapshot()
		return len(received) > 0 && len(deleted) > 0
	})

	deleted, received := h.snapshot()
	if len(received) != 1 {
		t.Fatalf("%d messages handled, want 1", len(received))
	}
	if received[0].Text != "מזל טוב" {
		t.Errorf("text = %q", received[0].Text)
	}
	// Without the delete, GreenAPI replays the same notification forever.
	if !strings.HasSuffix(deleted[0], "/deleteNotification/"+testToken+"/4242") {
		t.Errorf("delete path = %q, want it to carry receipt 4242", deleted[0])
	}
}

func TestPollerDeletesUnhandledNotificationTypes(t *testing.T) {
	h := &pollerHarness{}
	var served bool
	var mu sync.Mutex

	p := newPoller(t, h, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "deleteNotification") {
			h.recordDelete(r.URL.Path)
			_, _ = w.Write([]byte(`{"result":true}`))
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if served {
			_, _ = w.Write([]byte(`null`))
			return
		}
		served = true
		_, _ = w.Write([]byte(`{"receiptId":77,"body":{"typeWebhook":"outgoingMessageStatus","timestamp":1}}`))
	})

	runPoller(t, p, func() bool {
		deleted, _ := h.snapshot()
		return len(deleted) > 0
	})

	deleted, received := h.snapshot()
	if len(received) != 0 {
		t.Errorf("%d messages dispatched for an ignored type, want 0", len(received))
	}
	// An un-deleted receipt blocks the head of the queue forever.
	if !strings.HasSuffix(deleted[0], "/77") {
		t.Errorf("delete path = %q, want receipt 77 deleted anyway", deleted[0])
	}
}

func TestPollerStopsOnContextCancel(t *testing.T) {
	h := &pollerHarness{}
	p := newPoller(t, h, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`null`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("poller did not stop within 3s of cancellation")
	}
}

func TestPollerSurvivesTransientErrors(t *testing.T) {
	h := &pollerHarness{}
	var calls int
	var mu sync.Mutex

	p := newPoller(t, h, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "deleteNotification") {
			h.recordDelete(r.URL.Path)
			_, _ = w.Write([]byte(`{"result":true}`))
			return
		}
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()

		// The transport retries a 500 three times, so the first three requests
		// are all consumed by one failed poll; the fourth is the next poll.
		switch {
		case n <= 3:
			w.WriteHeader(http.StatusInternalServerError)
		case n == 4:
			_, _ = w.Write([]byte(`{"receiptId":9,"body":` + textMessagePayload + `}`))
		default:
			_, _ = w.Write([]byte(`null`))
		}
	})

	runPoller(t, p, func() bool {
		_, received := h.snapshot()
		return len(received) > 0
	})

	if _, received := h.snapshot(); len(received) != 1 {
		t.Errorf("%d messages handled after a provider outage, want 1", len(received))
	}
}

func TestPollerIgnoresEmptyResponses(t *testing.T) {
	h := &pollerHarness{}
	var polls int
	var mu sync.Mutex

	p := newPoller(t, h, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		polls++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	runPoller(t, p, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return polls >= 3
	})

	if _, received := h.snapshot(); len(received) != 0 {
		t.Errorf("%d messages from empty responses, want 0", len(received))
	}
}
