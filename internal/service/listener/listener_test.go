package listener_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
	"github.com/t0mer/blessed-by-the-bot/internal/logging"
	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/service/listener"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// fakeGreenAPI serves the two endpoints the poller uses. It hands out each
// queued notification once, then reports an empty queue.
type fakeGreenAPI struct {
	mu       sync.Mutex
	queue    []string
	polls    atomic.Int64
	deleted  atomic.Int64
	server   *httptest.Server
	lastPath atomic.Value
}

func newFakeGreenAPI(t *testing.T, bodies ...string) *fakeGreenAPI {
	t.Helper()
	f := &fakeGreenAPI{queue: append([]string(nil), bodies...)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.lastPath.Store(r.URL.Path)
		switch {
		case strings.Contains(r.URL.Path, "receiveNotification"):
			f.polls.Add(1)
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.queue) == 0 {
				// GreenAPI answers an empty queue with a bare null.
				_, _ = w.Write([]byte("null"))
				return
			}
			body := f.queue[0]
			f.queue = f.queue[1:]
			_, _ = fmt.Fprintf(w, `{"receiptId": %d, "body": %s}`, 100+f.deleted.Load(), body)
		case strings.Contains(r.URL.Path, "deleteNotification"):
			f.deleted.Add(1)
			_, _ = w.Write([]byte(`{"result": true}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

// recorder stands in for the group-echo engine.
type recorder struct {
	mu  sync.Mutex
	got []provider.IncomingMessage
}

func (r *recorder) HandleIncoming(_ context.Context, msg provider.IncomingMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, msg)
	return nil
}

func (r *recorder) messages() []provider.IncomingMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]provider.IncomingMessage(nil), r.got...)
}

type harness struct {
	sup      *listener.Supervisor
	settings *settings.Service
	recorder *recorder
}

func newHarness(t *testing.T, mutate ...func(*listener.Deps)) *harness {
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

	log := logging.NewTo(io.Discard, "error", false)
	rec := &recorder{}
	deps := listener.Deps{
		Settings:  svc,
		Transport: provider.NewTransport(provider.TransportOptions{Logger: log}),
		Incoming:  rec,
		Logger:    log,
		Interval:  20 * time.Millisecond,
		PollDelay: 5 * time.Millisecond,
	}
	for _, m := range mutate {
		m(&deps)
	}

	sup, err := listener.New(deps)
	if err != nil {
		t.Fatalf("building supervisor: %v", err)
	}
	return &harness{sup: sup, settings: svc, recorder: rec}
}

// configure writes a settings document, which is what the supervisor reconciles against.
func (h *harness) configure(t *testing.T, mutate func(*settings.Settings)) {
	t.Helper()
	s := settings.Defaults()
	mutate(s)
	if err := h.settings.Save(context.Background(), s); err != nil {
		t.Fatalf("saving settings: %v", err)
	}
}

func (h *harness) start(t *testing.T) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.sup.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run returned %v, want nil", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Run did not return within 3s of cancellation")
		}
	})
	return cancel
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

const groupNotification = `{
  "typeWebhook": "incomingMessageReceived",
  "timestamp": 1750000000,
  "idMessage": "POLLED-1",
  "senderData": {"chatId": "120363001234567890@g.us", "sender": "972501234567@c.us", "senderName": "Dana"},
  "messageData": {"typeMessage": "textMessage", "textMessageData": {"textMessage": "מזל טוב"}}
}`

// The core gap this fixes: with GreenAPI in polling mode — the default — inbound
// messages must actually reach the echo engine.
func TestPollingDeliversMessagesToTheHandler(t *testing.T) {
	fake := newFakeGreenAPI(t, groupNotification)
	h := newHarness(t)
	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGreenAPI
		s.GreenAPI.Mode = settings.ModePolling
		s.GreenAPI.APIURL = fake.server.URL
		s.GreenAPI.IDInstance = "7103123456"
		s.GreenAPI.APIToken = "token"
	})
	h.start(t)

	eventually(t, "a polled message to reach the handler", func() bool {
		return len(h.recorder.messages()) == 1
	})

	got := h.recorder.messages()[0]
	if got.Text != "מזל טוב" || !got.IsGroup {
		t.Fatalf("unexpected message: %#v", got)
	}
	// Receipts must be acknowledged or the queue head blocks everything behind it.
	eventually(t, "the receipt to be deleted", func() bool { return fake.deleted.Load() >= 1 })
}

func TestWebhookModeDoesNotPoll(t *testing.T) {
	fake := newFakeGreenAPI(t, groupNotification)
	h := newHarness(t)
	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGreenAPI
		s.GreenAPI.Mode = settings.ModeWebhook
		s.GreenAPI.APIURL = fake.server.URL
		s.GreenAPI.IDInstance = "7103123456"
		s.GreenAPI.APIToken = "token"
	})
	h.start(t)

	time.Sleep(200 * time.Millisecond)
	if n := fake.polls.Load(); n != 0 {
		t.Fatalf("polled %d times in webhook mode, want 0", n)
	}
}

func TestGOWAProviderDoesNotPoll(t *testing.T) {
	fake := newFakeGreenAPI(t, groupNotification)
	h := newHarness(t)
	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGOWA
		// GreenAPI stays configured but inactive; it must not be polled.
		s.GreenAPI.Mode = settings.ModePolling
		s.GreenAPI.APIURL = fake.server.URL
		s.GreenAPI.IDInstance = "7103123456"
		s.GreenAPI.APIToken = "token"
		s.GOWA.BaseURL = "http://127.0.0.1:1"
	})
	h.start(t)

	time.Sleep(200 * time.Millisecond)
	if n := fake.polls.Load(); n != 0 {
		t.Fatalf("polled %d times while GOWA was active, want 0", n)
	}
}

// Incomplete credentials must not start a poller that can only ever 401.
func TestIncompleteCredentialsDoNotStartAPoller(t *testing.T) {
	h := newHarness(t)
	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGreenAPI
		s.GreenAPI.Mode = settings.ModePolling
		s.GreenAPI.IDInstance = "" // never configured
	})
	h.start(t)

	time.Sleep(150 * time.Millisecond)
	if h.sup.Polling() {
		t.Fatal("want no poller running without credentials")
	}
}

// Spec §4: switching providers must take effect without a restart.
func TestSwitchingAwayFromPollingStopsThePoller(t *testing.T) {
	fake := newFakeGreenAPI(t)
	h := newHarness(t)
	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGreenAPI
		s.GreenAPI.Mode = settings.ModePolling
		s.GreenAPI.APIURL = fake.server.URL
		s.GreenAPI.IDInstance = "7103123456"
		s.GreenAPI.APIToken = "token"
	})
	h.start(t)
	eventually(t, "the poller to start", func() bool { return h.sup.Polling() })

	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGOWA
		s.GOWA.BaseURL = "http://127.0.0.1:1"
	})
	h.sup.Notify()

	eventually(t, "the poller to stop", func() bool { return !h.sup.Polling() })

	before := fake.polls.Load()
	time.Sleep(150 * time.Millisecond)
	if after := fake.polls.Load(); after != before {
		t.Fatalf("polling continued after the switch: %d -> %d", before, after)
	}
}

// ...and switching to it starts one.
func TestSwitchingToPollingStartsThePoller(t *testing.T) {
	fake := newFakeGreenAPI(t)
	h := newHarness(t)
	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGOWA
		s.GOWA.BaseURL = "http://127.0.0.1:1"
	})
	h.start(t)

	time.Sleep(100 * time.Millisecond)
	if h.sup.Polling() {
		t.Fatal("precondition: no poller should be running for GOWA")
	}

	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGreenAPI
		s.GreenAPI.Mode = settings.ModePolling
		s.GreenAPI.APIURL = fake.server.URL
		s.GreenAPI.IDInstance = "7103123456"
		s.GreenAPI.APIToken = "token"
	})
	h.sup.Notify()

	eventually(t, "the poller to start", func() bool { return h.sup.Polling() })
	eventually(t, "polling to actually happen", func() bool { return fake.polls.Load() > 0 })
}

// A changed token must rebuild the poller: the old one holds the stale
// credential in its client and would keep failing forever.
func TestChangedCredentialsRestartThePoller(t *testing.T) {
	first := newFakeGreenAPI(t)
	second := newFakeGreenAPI(t)
	h := newHarness(t)
	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGreenAPI
		s.GreenAPI.Mode = settings.ModePolling
		s.GreenAPI.APIURL = first.server.URL
		s.GreenAPI.IDInstance = "7103123456"
		s.GreenAPI.APIToken = "token"
	})
	h.start(t)
	eventually(t, "the first poller to poll", func() bool { return first.polls.Load() > 0 })

	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGreenAPI
		s.GreenAPI.Mode = settings.ModePolling
		s.GreenAPI.APIURL = second.server.URL
		s.GreenAPI.IDInstance = "7103123456"
		s.GreenAPI.APIToken = "token"
	})
	h.sup.Notify()

	eventually(t, "the new endpoint to be polled", func() bool { return second.polls.Load() > 0 })
}

// The poller must survive a provider outage rather than exiting for good.
func TestPollerSurvivesAnUnreachableProvider(t *testing.T) {
	h := newHarness(t)
	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGreenAPI
		s.GreenAPI.Mode = settings.ModePolling
		s.GreenAPI.APIURL = "http://127.0.0.1:1" // nothing listening
		s.GreenAPI.IDInstance = "7103123456"
		s.GreenAPI.APIToken = "token"
	})
	h.start(t)

	eventually(t, "the poller to start", func() bool { return h.sup.Polling() })
	time.Sleep(200 * time.Millisecond)
	if !h.sup.Polling() {
		t.Fatal("the poller exited on an unreachable provider; it must back off and retry")
	}
}

func TestNewRequiresItsDependencies(t *testing.T) {
	if _, err := listener.New(listener.Deps{}); err == nil {
		t.Fatal("want an error for empty deps")
	}
}

// A message the poller cannot parse must not take the loop down.
func TestUnparsableNotificationIsSkipped(t *testing.T) {
	fake := newFakeGreenAPI(t, `{"typeWebhook":"outgoingMessageStatus"}`, groupNotification)
	h := newHarness(t)
	h.configure(t, func(s *settings.Settings) {
		s.Provider = settings.ProviderGreenAPI
		s.GreenAPI.Mode = settings.ModePolling
		s.GreenAPI.APIURL = fake.server.URL
		s.GreenAPI.IDInstance = "7103123456"
		s.GreenAPI.APIToken = "token"
	})
	h.start(t)

	eventually(t, "the second notification to be delivered", func() bool {
		msgs := h.recorder.messages()
		return len(msgs) == 1 && msgs[0].MessageID == "POLLED-1"
	})
}

var _ = json.Marshal // keep encoding/json imported for fixture edits
