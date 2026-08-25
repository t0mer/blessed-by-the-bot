package scheduler_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
	"github.com/t0mer/blessed-by-the-bot/internal/logging"
	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/service/blessing"
	"github.com/t0mer/blessed-by-the-bot/internal/service/scheduler"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

type fakeProvider struct {
	mu   sync.Mutex
	err  error
	sent []sentMessage
}

type sentMessage struct{ ChatID, Text string }

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) SendText(_ context.Context, chatID, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	f.sent = append(f.sent, sentMessage{chatID, text})
	return "fake-id", nil
}

func (f *fakeProvider) ListGroups(context.Context) ([]provider.Group, error) { return nil, nil }

func (f *fakeProvider) Status(context.Context) (provider.ProviderStatus, error) {
	return provider.ProviderStatus{Provider: "fake", Connected: true}, nil
}

func (f *fakeProvider) messages() []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentMessage(nil), f.sent...)
}

func (f *fakeProvider) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

type harness struct {
	sched    *scheduler.Scheduler
	store    *store.Store
	settings *settings.Service
	provider *fakeProvider
}

// at builds a harness whose clock is frozen at the given local time.
func at(t *testing.T, when time.Time, mutate ...func(*scheduler.Deps)) *harness {
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
	sel, err := blessing.NewSelector(st, log)
	if err != nil {
		t.Fatalf("building selector: %v", err)
	}

	fake := &fakeProvider{}
	mgr := provider.NewManager()
	mgr.Set(fake)

	deps := scheduler.Deps{
		Store: st, Settings: svc, Providers: mgr, Blessings: sel, Logger: log,
		Now: func() time.Time { return when },
	}
	for _, m := range mutate {
		m(&deps)
	}

	sched, err := scheduler.New(deps)
	if err != nil {
		t.Fatalf("building scheduler: %v", err)
	}
	return &harness{sched: sched, store: st, settings: svc, provider: fake}
}

func jerusalem(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jerusalem")
	if err != nil {
		t.Fatalf("loading zone: %v", err)
	}
	return loc
}

func seedBlessing(t *testing.T, st *store.Store, text string) *store.Blessing {
	t.Helper()
	b, err := st.CreateBlessing(context.Background(), &store.Blessing{
		EventType: store.EventBirthday, Language: "he", Text: text, Enabled: true,
	})
	if err != nil {
		t.Fatalf("creating blessing: %v", err)
	}
	return b
}

func seedContact(t *testing.T, st *store.Store, mutate ...func(*store.Contact)) *store.Contact {
	t.Helper()
	c := &store.Contact{
		Name: "Dana", Phone: "972501234567", EventDate: "1990-05-17",
		EventType: store.EventBirthday, Language: "he", Relation: store.RelationFriend,
		Importance: 3, Gender: store.GenderFemale, Enabled: true,
	}
	for _, m := range mutate {
		m(c)
	}
	created, err := st.CreateContact(context.Background(), c)
	if err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	return created
}

func TestTickSendsToADueContact(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "יום הולדת שמח {{name}}!")
	seedContact(t, h.store)

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	sent := h.provider.messages()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sent))
	}
	if sent[0].ChatID != "972501234567@c.us" {
		t.Errorf("chat id = %q", sent[0].ChatID)
	}
	if sent[0].Text != "יום הולדת שמח Dana!" {
		t.Errorf("text = %q, want the placeholder rendered", sent[0].Text)
	}

	entries, err := h.store.ListSendLog(context.Background(), store.KindScheduled, 10)
	if err != nil {
		t.Fatalf("listing send log: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != store.StatusSent {
		t.Fatalf("send log = %#v, want one sent entry", entries)
	}
	if entries[0].EventYear == nil || *entries[0].EventYear != 2026 {
		t.Fatalf("event_year = %v, want 2026", entries[0].EventYear)
	}
}

func TestTickIsIdempotentWithinTheSameYear(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	seedContact(t, h.store)

	// The tick runs every minute all day; only the first may send.
	for range 5 {
		if err := h.sched.Tick(context.Background()); err != nil {
			t.Fatalf("Tick: %v", err)
		}
	}
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want exactly 1", got)
	}
}

func TestTickSkipsBeforeTheSendTime(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 8, 59, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב")
	seedContact(t, h.store)

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages before the send time, want 0", got)
	}
}

func TestTickHonoursThePerContactSendTime(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 7, 45, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב")
	override := "07:30"
	seedContact(t, h.store, func(c *store.Contact) { c.SendTime = &override })

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want 1 — the override had passed", got)
	}
}

func TestTickSkipsDisabledContacts(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב")
	seedContact(t, h.store, func(c *store.Contact) { c.Enabled = false })

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages for a disabled contact, want 0", got)
	}
}

func TestTickSkipsContactsWhoseEventIsNotToday(t *testing.T) {
	h := at(t, time.Date(2026, 5, 18, 9, 30, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב")
	seedContact(t, h.store)

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages on the wrong day, want 0", got)
	}
}

// A failed send must be recorded, must NOT satisfy the yearly dedupe, and must
// therefore be retried by the next tick.
func TestFailedSendIsLoggedAndRetried(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	seedContact(t, h.store)

	h.provider.fail(errors.New("provider is down"))
	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick must not fail because a send failed: %v", err)
	}

	entries, err := h.store.ListSendLog(context.Background(), store.KindScheduled, 10)
	if err != nil {
		t.Fatalf("listing send log: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != store.StatusFailed {
		t.Fatalf("send log = %#v, want one failed entry", entries)
	}
	if entries[0].Error == nil || *entries[0].Error == "" {
		t.Error("want the failure reason recorded")
	}

	h.provider.fail(nil)
	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages on retry, want 1", got)
	}
}

// No template is a configuration problem, not a crash, and it must not stop the
// tick from serving other contacts.
func TestTickContinuesWhenOneContactHasNoBlessing(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	seedContact(t, h.store, func(c *store.Contact) { c.Name = "Dana" })
	seedContact(t, h.store, func(c *store.Contact) {
		c.Name, c.Phone, c.Language = "Ivan", "79161234567", "ru"
	})

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	// Dana is served; Ivan has no Russian template and no English fallback.
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want 1", got)
	}
}

func TestTickWithNoProviderDoesNotFail(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב")
	seedContact(t, h.store)

	// A fresh install has no provider until the user configures one; the tick
	// must survive that rather than crash-looping.
	h.sched.Providers().Clear()
	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
}
