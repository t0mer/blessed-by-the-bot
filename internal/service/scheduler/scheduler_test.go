package scheduler_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
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
	clearBlessings(t, st)

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

// restoreSeed re-applies migration 0003 into a store the harness has cleared,
// for the tests that specifically exercise fresh-install behaviour.
func restoreSeed(t *testing.T, st *store.Store) {
	t.Helper()
	if err := st.ApplySeedBlessings(context.Background()); err != nil {
		t.Fatalf("restoring the seeded blessings: %v", err)
	}
}

// clearBlessings removes the starter templates seeded by migration 0003 so each
// test controls exactly which ones exist.
func clearBlessings(t *testing.T, st *store.Store) {
	t.Helper()
	all, err := st.ListBlessings(context.Background())
	if err != nil {
		t.Fatalf("listing seeded blessings: %v", err)
	}
	for _, b := range all {
		if err := st.DeleteBlessing(context.Background(), b.ID); err != nil {
			t.Fatalf("deleting seeded blessing %d: %v", b.ID, err)
		}
	}
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

func TestRunTicksUntilContextIsCancelled(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)),
		func(d *scheduler.Deps) { d.Interval = 10 * time.Millisecond })
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	seedContact(t, h.store)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.sched.Run(ctx) }()

	deadline := time.After(2 * time.Second)
	for len(h.provider.messages()) == 0 {
		select {
		case <-deadline:
			t.Fatal("Run never sent anything")
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil after cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of cancellation")
	}

	// The loop keeps ticking, but the yearly dedupe holds across ticks.
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want exactly 1 across many ticks", got)
	}
}

// A settings row the scheduler cannot read is a real fault, but crashing the
// process over it would take the UI down too — the loop logs and carries on.
func TestRunSurvivesATickError(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)),
		func(d *scheduler.Deps) { d.Interval = 5 * time.Millisecond })
	if err := h.store.Close(); err != nil {
		t.Fatalf("closing store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := h.sched.Run(ctx); err != nil {
		t.Fatalf("Run returned %v, want nil despite failing ticks", err)
	}
}

// Restarting mid-day must catch a send whose send time already passed.
func TestCatchUpAfterRestartSameDay(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 23, 55, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	seedContact(t, h.store) // general send time 09:00, long past

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want the missed send caught up", got)
	}
}

// ...but never yesterday's. The next day the event no longer falls today.
func TestNoCatchUpTheFollowingDay(t *testing.T) {
	h := at(t, time.Date(2026, 5, 18, 0, 5, 0, 0, jerusalem(t)))
	seedBlessing(t, h.store, "מזל טוב {{name}}")
	seedContact(t, h.store)

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages the day after, want 0", got)
	}
}

func TestNewRequiresItsDependencies(t *testing.T) {
	if _, err := scheduler.New(scheduler.Deps{}); err == nil {
		t.Fatal("want an error for empty deps")
	}
}

// An unloadable timezone is a settings problem the operator must see, and it
// makes the whole pass meaningless — unlike one contact failing.
func TestTickFailsOnAnUnknownTimezone(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)))
	s := settings.Defaults()
	s.Scheduler.Timezone = "Asia/Jerusalem"
	if err := h.settings.Save(context.Background(), s); err != nil {
		t.Fatalf("saving settings: %v", err)
	}
	// Write an invalid zone straight past the service's validation, which is the
	// only way a bad value can reach the tick in practice (a hand-edited DB).
	if err := h.store.SetSetting(context.Background(), "scheduler",
		`{"timezone":"Mars/Olympus","send_time":"09:00"}`); err != nil {
		t.Fatalf("writing setting: %v", err)
	}

	if err := h.sched.Tick(context.Background()); err == nil {
		t.Fatal("want an error for an unloadable timezone")
	}
}

// The point of seeding starter templates: a fresh install must be able to send
// without the user first authoring a blessing. This is the one test that must
// NOT clear the seed.
func TestFreshInstallCanSendWithOnlyTheSeededBlessings(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)))
	restoreSeed(t, h.store)

	// A Hebrew female contact whose birthday is today — no blessing authored.
	seedContact(t, h.store)

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	sent := h.provider.messages()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1 straight out of the box", len(sent))
	}
	if !strings.Contains(sent[0].Text, "Dana") {
		t.Errorf("text = %q, want the seeded template rendered with the name", sent[0].Text)
	}
	// The female-form template must win over the male one.
	if strings.Contains(sent[0].Text, "שתזכה") {
		t.Errorf("text = %q, want the female Hebrew form, not the male one", sent[0].Text)
	}
}

// A male contact must get the male form, which is the whole reason the seed
// carries both.
func TestFreshInstallPicksTheGenderedForm(t *testing.T) {
	h := at(t, time.Date(2026, 5, 17, 9, 30, 0, 0, jerusalem(t)))
	restoreSeed(t, h.store)
	seedContact(t, h.store, func(c *store.Contact) {
		c.Name, c.Gender = "Yossi", store.GenderMale
	})

	if err := h.sched.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	sent := h.provider.messages()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sent))
	}
	if strings.Contains(sent[0].Text, "שתזכי") || strings.Contains(sent[0].Text, "שתמשיכי") {
		t.Errorf("text = %q, want a male Hebrew form", sent[0].Text)
	}
}
