package echo_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
	"github.com/t0mer/blessed-by-the-bot/internal/logging"
	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/service/blessing"
	"github.com/t0mer/blessed-by-the-bot/internal/service/echo"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

const groupChatID = "120363001234567890@g.us"

type fakeProvider struct {
	mu   sync.Mutex
	err  error
	sent []string
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) SendText(_ context.Context, _, text string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	f.sent = append(f.sent, text)
	return "fake-id", nil
}

func (f *fakeProvider) ListGroups(context.Context) ([]provider.Group, error) { return nil, nil }

func (f *fakeProvider) Status(context.Context) (provider.ProviderStatus, error) {
	return provider.ProviderStatus{Provider: "fake", Connected: true}, nil
}

func (f *fakeProvider) messages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

type harness struct {
	engine    *echo.Engine
	store     *store.Store
	settings  *settings.Service
	provider  *fakeProvider
	providers *provider.Manager
	clock     *clock
}

func discardLogger() *slog.Logger { return logging.NewTo(io.Discard, "error", false) }

func providerManagerFor(p provider.Provider) *provider.Manager {
	mgr := provider.NewManager()
	mgr.Set(p)
	return mgr
}

func selectorFor(t *testing.T, st *store.Store) *blessing.Selector {
	t.Helper()
	sel, err := blessing.NewSelector(st, discardLogger())
	if err != nil {
		t.Fatalf("building selector: %v", err)
	}
	return sel
}

// clearBlessings removes the starter templates seeded by migration 0003.
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

// clock is a movable time source so window and cooldown behaviour is testable
// without sleeping.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newHarness(t *testing.T) *harness {
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

	c := &clock{now: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)}
	engine, err := echo.New(echo.Deps{
		Store: st, Settings: svc, Providers: mgr, Blessings: sel, Logger: log, Now: c.Now,
	})
	if err != nil {
		t.Fatalf("building engine: %v", err)
	}
	return &harness{engine: engine, store: st, settings: svc, provider: fake, providers: mgr, clock: c}
}

func (h *harness) addGroup(t *testing.T, mutate ...func(*store.Group)) *store.Group {
	t.Helper()
	g := &store.Group{Name: "Family", ChatID: groupChatID, Language: "he", Enabled: true}
	for _, m := range mutate {
		m(g)
	}
	created, err := h.store.CreateGroup(context.Background(), g)
	if err != nil {
		t.Fatalf("creating group: %v", err)
	}
	return created
}

// wish builds an inbound group message from one sender.
func wish(sender, id, text string) provider.IncomingMessage {
	return provider.IncomingMessage{
		Provider: "fake", ChatID: groupChatID, IsGroup: true,
		SenderID: sender, MessageID: id, Text: text, Timestamp: time.Now(),
	}
}

func (h *harness) deliver(t *testing.T, msgs ...provider.IncomingMessage) {
	t.Helper()
	for _, m := range msgs {
		if err := h.engine.HandleIncoming(context.Background(), m); err != nil {
			t.Fatalf("HandleIncoming(%s): %v", m.MessageID, err)
		}
	}
}

// The spec §15 acceptance case: three different people writing "מזל טוב"
// produce exactly one bot message, and a fourth wish within cooldown produces
// none.
func TestThreeDistinctSendersTriggerExactlyOneEcho(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	h.deliver(t,
		wish("aviv@c.us", "m1", "מזל טוב!"),
		wish("noa@c.us", "m2", "מזל טוב!!"),
	)
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("echoed after 2 senders (%d messages), want 0", got)
	}

	h.deliver(t, wish("yossi@c.us", "m3", "מזל טוב"))
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages after the third sender, want exactly 1", got)
	}

	// A fourth wish inside the cooldown must stay quiet.
	h.deliver(t, wish("dana@c.us", "m4", "מזל טוב"))
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want the cooldown to suppress the fourth", got)
	}
}

// One excited friend is one vote, not five.
func TestRepeatedMessagesFromOneSenderDoNotTrigger(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	h.deliver(t,
		wish("aviv@c.us", "m1", "מזל טוב"),
		wish("aviv@c.us", "m2", "מזל טוב!!"),
		wish("aviv@c.us", "m3", "🎂🎉"),
		wish("aviv@c.us", "m4", "יום הולדת שמח"),
		wish("aviv@c.us", "m5", "מזל טוב"),
	)
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages from a single sender, want 0", got)
	}
}

// A provider redelivering the same message id must not count twice.
func TestDuplicateMessageIDsAreIdempotent(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	h.deliver(t,
		wish("aviv@c.us", "m1", "מזל טוב"),
		wish("noa@c.us", "m2", "מזל טוב"),
		wish("noa@c.us", "m2", "מזל טוב"), // redelivery
		wish("noa@c.us", "m2", "מזל טוב"), // and again
	)
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages, want 0 — only 2 distinct senders", got)
	}
}

func TestOrdinaryChatterNeverTriggers(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	h.deliver(t,
		wish("aviv@c.us", "m1", "מה השעה?"),
		wish("noa@c.us", "m2", "אני בדרך"),
		wish("yossi@c.us", "m3", "ok see you"),
	)
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages for ordinary chatter, want 0", got)
	}
}

func TestUnconfiguredGroupIsIgnored(t *testing.T) {
	h := newHarness(t) // no group created

	h.deliver(t,
		wish("aviv@c.us", "m1", "מזל טוב"),
		wish("noa@c.us", "m2", "מזל טוב"),
		wish("yossi@c.us", "m3", "מזל טוב"),
	)
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages for an unwatched group, want 0", got)
	}
}

func TestDisabledGroupIsIgnored(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t, func(g *store.Group) { g.Enabled = false })

	h.deliver(t,
		wish("aviv@c.us", "m1", "מזל טוב"),
		wish("noa@c.us", "m2", "מזל טוב"),
		wish("yossi@c.us", "m3", "מזל טוב"),
	)
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages for a disabled group, want 0", got)
	}
}

func TestPrivateMessagesAreIgnored(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	msg := wish("aviv@c.us", "m1", "מזל טוב")
	msg.IsGroup = false
	h.deliver(t, msg)

	count, err := h.store.CountDistinctWishSenders(context.Background(), 1, time.Time{})
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Fatalf("recorded %d wishes from a private chat, want 0", count)
	}
}

// The bot's own blessing matches the wish patterns, so counting it would let the
// bot bootstrap its own trigger.
func TestTheBotsOwnMessagesAreIgnored(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	self := wish("bot@c.us", "m1", "מזל טוב")
	self.FromMe = true
	h.deliver(t, self,
		wish("aviv@c.us", "m2", "מזל טוב"),
		wish("noa@c.us", "m3", "מזל טוב"))

	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages, want 0 — the bot must not count itself", got)
	}
}

func TestPerGroupThresholdOverridesTheGlobalDefault(t *testing.T) {
	h := newHarness(t)
	two := 2
	h.addGroup(t, func(g *store.Group) { g.Threshold = &two })

	h.deliver(t,
		wish("aviv@c.us", "m1", "מזל טוב"),
		wish("noa@c.us", "m2", "מזל טוב"))

	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want 1 at the per-group threshold of 2", got)
	}
}

// Wishes that fall outside the rolling window must not accumulate into a
// trigger: three people congratulating on three separate days is not a burst.
func TestWishesOutsideTheWindowDoNotAccumulate(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	h.deliver(t, wish("aviv@c.us", "m1", "מזל טוב"))
	h.clock.advance(7 * time.Hour) // default window is 6h
	h.deliver(t, wish("noa@c.us", "m2", "מזל טוב"))
	h.clock.advance(7 * time.Hour)
	h.deliver(t, wish("yossi@c.us", "m3", "מזל טוב"))

	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages, want 0 — the wishes were spread across days", got)
	}
}

// After the cooldown expires the group can echo again, which is what makes the
// next person's birthday work.
func TestEchoesAgainAfterTheCooldown(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	h.deliver(t,
		wish("aviv@c.us", "m1", "מזל טוב"),
		wish("noa@c.us", "m2", "מזל טוב"),
		wish("yossi@c.us", "m3", "מזל טוב"))
	if got := len(h.provider.messages()); got != 1 {
		t.Fatalf("sent %d messages, want 1", got)
	}

	h.clock.advance(21 * time.Hour) // default cooldown is 20h
	h.deliver(t,
		wish("dana@c.us", "m4", "מזל טוב"),
		wish("ella@c.us", "m5", "מזל טוב"),
		wish("gil@c.us", "m6", "מזל טוב"))

	if got := len(h.provider.messages()); got != 2 {
		t.Fatalf("sent %d messages, want 2 after the cooldown expired", got)
	}
}

// The echo must never carry an unfilled placeholder: the bot does not know
// whose birthday it is.
func TestEchoUsesANameFreeTemplate(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	h.deliver(t,
		wish("aviv@c.us", "m1", "מזל טוב"),
		wish("noa@c.us", "m2", "מזל טוב"),
		wish("yossi@c.us", "m3", "מזל טוב"))

	sent := h.provider.messages()
	if len(sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sent))
	}
	if blessing.HasNamePlaceholder(sent[0]) {
		t.Fatalf("echo %q carries an unfilled name placeholder", sent[0])
	}
}

func TestFailedEchoIsRecorded(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)
	h.provider.err = errors.New("provider is down")

	h.deliver(t, wish("aviv@c.us", "m1", "מזל טוב"), wish("noa@c.us", "m2", "מזל טוב"))
	err := h.engine.HandleIncoming(context.Background(), wish("yossi@c.us", "m3", "מזל טוב"))
	if err == nil {
		t.Fatal("want the send failure surfaced to the caller")
	}

	entries, listErr := h.store.ListSendLog(context.Background(), store.KindGroupEcho, 10)
	if listErr != nil {
		t.Fatalf("listing send log: %v", listErr)
	}
	if len(entries) != 1 || entries[0].Status != store.StatusFailed {
		t.Fatalf("send log = %#v, want one failed entry", entries)
	}
}

func TestSweepDeletesOnlyExpiredWishes(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	h.deliver(t, wish("aviv@c.us", "old", "מזל טוב"))
	h.clock.advance(8 * 24 * time.Hour) // retention is 7 days
	h.deliver(t, wish("noa@c.us", "fresh", "מזל טוב"))

	if err := h.engine.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	remaining, err := h.store.CountDistinctWishSenders(context.Background(), 1, time.Time{})
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("%d senders remain, want only the fresh one", remaining)
	}
}

func TestNewRequiresItsDependencies(t *testing.T) {
	if _, err := echo.New(echo.Deps{}); err == nil {
		t.Fatal("want an error for empty deps")
	}
}

func TestRunJanitorSweepsUntilCancelled(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)
	h.deliver(t, wish("aviv@c.us", "old", "מזל טוב"))
	h.clock.advance(8 * 24 * time.Hour) // past the 7-day retention

	engine, err := echo.New(echo.Deps{
		Store: h.store, Settings: h.settings, Providers: providerManagerFor(h.provider),
		Blessings: selectorFor(t, h.store), Logger: discardLogger(),
		Now: h.clock.Now, JanitorEvery: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("building engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := engine.RunJanitor(ctx); err != nil {
		t.Fatalf("RunJanitor returned %v, want nil after cancellation", err)
	}

	remaining, err := h.store.CountDistinctWishSenders(context.Background(), 1, time.Time{})
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("%d wish senders remain, want the janitor to have swept them", remaining)
	}
}

// The janitor must not take the process down when the database is unreachable.
func TestRunJanitorSurvivesASweepError(t *testing.T) {
	h := newHarness(t)
	if err := h.store.Close(); err != nil {
		t.Fatalf("closing store: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	engine, err := echo.New(echo.Deps{
		Store: h.store, Settings: h.settings, Providers: providerManagerFor(h.provider),
		Blessings: selectorFor(t, h.store), Logger: discardLogger(),
		Now: h.clock.Now, JanitorEvery: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("building engine: %v", err)
	}
	if err := engine.RunJanitor(ctx); err != nil {
		t.Fatalf("RunJanitor returned %v, want nil despite failing sweeps", err)
	}
}

// A group with nothing to send must report it rather than posting an empty or
// placeholder-carrying message.
func TestEchoWithoutANameFreeTemplateReportsIt(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)
	clearBlessings(t, h.store)
	// Only a name-carrying template exists.
	if _, err := h.store.CreateBlessing(context.Background(), &store.Blessing{
		EventType: store.EventBirthday, Language: "he", Text: "מזל טוב {{name}}", Enabled: true,
	}); err != nil {
		t.Fatalf("creating blessing: %v", err)
	}

	h.deliver(t, wish("aviv@c.us", "m1", "מזל טוב"), wish("noa@c.us", "m2", "מזל טוב"))
	err := h.engine.HandleIncoming(context.Background(), wish("yossi@c.us", "m3", "מזל טוב"))
	if !errors.Is(err, blessing.ErrNoBlessing) {
		t.Fatalf("err = %v, want ErrNoBlessing", err)
	}
	if got := len(h.provider.messages()); got != 0 {
		t.Fatalf("sent %d messages, want 0", got)
	}
}

func TestEchoWithoutAProviderReportsIt(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)
	h.providers.Clear()

	h.deliver(t, wish("aviv@c.us", "m1", "מזל טוב"), wish("noa@c.us", "m2", "מזל טוב"))
	if err := h.engine.HandleIncoming(context.Background(), wish("yossi@c.us", "m3", "מזל טוב")); !errors.Is(err, provider.ErrNoProvider) {
		t.Fatalf("err = %v, want ErrNoProvider", err)
	}
}

// A message with no sender or id cannot be counted or deduplicated.
func TestMessagesWithoutSenderOrIDAreIgnored(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)

	noSender := wish("", "m1", "מזל טוב")
	noID := wish("aviv@c.us", "", "מזל טוב")
	h.deliver(t, noSender, noID)

	count, err := h.store.CountDistinctWishSenders(context.Background(), 1, time.Time{})
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Fatalf("recorded %d wishes, want 0", count)
	}
}

// A database fault while looking up the group is a real error, not chatter.
func TestStoreFailureIsReported(t *testing.T) {
	h := newHarness(t)
	h.addGroup(t)
	if err := h.store.Close(); err != nil {
		t.Fatalf("closing store: %v", err)
	}

	if err := h.engine.HandleIncoming(context.Background(), wish("aviv@c.us", "m1", "מזל טוב")); err == nil {
		t.Fatal("want the store failure surfaced")
	}
}
