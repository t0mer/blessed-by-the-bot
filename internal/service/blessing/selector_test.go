package blessing_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/logging"
	"github.com/t0mer/blessed-by-the-bot/internal/service/blessing"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	clearBlessings(t, st)
	return st
}

// clearBlessings removes the starter templates seeded by migration 0003 so each
// test controls exactly which ones exist. A separate test covers the seed.
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

func newSelector(t *testing.T, st *store.Store) *blessing.Selector {
	t.Helper()
	sel, err := blessing.NewSelector(st, logging.NewTo(io.Discard, "error", false))
	if err != nil {
		t.Fatalf("building selector: %v", err)
	}
	return sel
}

func addBlessing(t *testing.T, st *store.Store, b store.Blessing) *store.Blessing {
	t.Helper()
	b.Enabled = true
	created, err := st.CreateBlessing(context.Background(), &b)
	if err != nil {
		t.Fatalf("creating blessing: %v", err)
	}
	return created
}

func addContact(t *testing.T, st *store.Store, c store.Contact) *store.Contact {
	t.Helper()
	if c.Name == "" {
		c.Name = "Dana"
	}
	if c.Phone == "" {
		c.Phone = "972501234567"
	}
	if c.EventDate == "" {
		c.EventDate = "1990-05-17"
	}
	if c.EventType == "" {
		c.EventType = store.EventBirthday
	}
	if c.Language == "" {
		c.Language = "he"
	}
	if c.Relation == "" {
		c.Relation = store.RelationFriend
	}
	if c.Gender == "" {
		c.Gender = store.GenderFemale
	}
	if c.Importance == 0 {
		c.Importance = 3
	}
	c.Enabled = true
	created, err := st.CreateContact(context.Background(), &c)
	if err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	return created
}

func TestSelectorFindsAMatchingBlessing(t *testing.T) {
	st := newStore(t)
	want := addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "he", Text: "מזל טוב {{name}}"})
	c := addContact(t, st, store.Contact{})

	got, err := newSelector(t, st).ForContact(context.Background(), c)
	if err != nil {
		t.Fatalf("ForContact: %v", err)
	}
	if got.ID != want.ID {
		t.Fatalf("picked %d, want %d", got.ID, want.ID)
	}
}

func TestSelectorIgnoresDisabledAndWrongEventType(t *testing.T) {
	st := newStore(t)
	disabled, err := st.CreateBlessing(context.Background(), &store.Blessing{
		EventType: store.EventBirthday, Language: "he", Text: "disabled", Enabled: false,
	})
	if err != nil {
		t.Fatalf("creating blessing: %v", err)
	}
	addBlessing(t, st, store.Blessing{EventType: store.EventWedding, Language: "he", Text: "wrong type"})
	want := addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "he", Text: "right"})

	got, err := newSelector(t, st).ForContact(context.Background(), addContact(t, st, store.Contact{}))
	if err != nil {
		t.Fatalf("ForContact: %v", err)
	}
	if got.ID == disabled.ID || got.ID != want.ID {
		t.Fatalf("picked %d, want %d", got.ID, want.ID)
	}
}

func TestSelectorFallsBackToEnglish(t *testing.T) {
	st := newStore(t)
	want := addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "en", Text: "Happy birthday {{name}}"})
	c := addContact(t, st, store.Contact{Language: "ru"})

	got, err := newSelector(t, st).ForContact(context.Background(), c)
	if err != nil {
		t.Fatalf("ForContact: %v", err)
	}
	if got.ID != want.ID {
		t.Fatalf("picked %d, want the English fallback %d", got.ID, want.ID)
	}
}

func TestSelectorPrefersTheContactLanguageOverEnglish(t *testing.T) {
	st := newStore(t)
	addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "en", Text: "english"})
	want := addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "he", Text: "hebrew"})

	got, err := newSelector(t, st).ForContact(context.Background(), addContact(t, st, store.Contact{}))
	if err != nil {
		t.Fatalf("ForContact: %v", err)
	}
	if got.ID != want.ID {
		t.Fatalf("picked %d, want the contact's own language %d", got.ID, want.ID)
	}
}

func TestSelectorReportsWhenNothingMatches(t *testing.T) {
	st := newStore(t)
	c := addContact(t, st, store.Contact{})

	_, err := newSelector(t, st).ForContact(context.Background(), c)
	if !errors.Is(err, blessing.ErrNoBlessing) {
		t.Fatalf("err = %v, want ErrNoBlessing", err)
	}
}

// A wrong-gender template is excluded even when it is the only one, and that
// exclusion must surface as ErrNoBlessing rather than a wrong send.
func TestSelectorReportsWhenOnlyMismatchedTargetingExists(t *testing.T) {
	st := newStore(t)
	male := store.GenderMale
	addBlessing(t, st, store.Blessing{
		EventType: store.EventBirthday, Language: "he", Text: "male only", Gender: &male,
	})
	c := addContact(t, st, store.Contact{Gender: store.GenderFemale})

	if _, err := newSelector(t, st).ForContact(context.Background(), c); !errors.Is(err, blessing.ErrNoBlessing) {
		t.Fatalf("err = %v, want ErrNoBlessing", err)
	}
}

// The no-repeat bias reads last year's choice from the send log, which is what
// makes it survive a restart.
func TestSelectorAvoidsThePreviousBlessing(t *testing.T) {
	st := newStore(t)
	previous := addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "he", Text: "one"})
	other := addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "he", Text: "two"})
	c := addContact(t, st, store.Contact{})

	year := 2025
	if _, err := st.AppendSendLog(context.Background(), &store.SendLogEntry{
		Kind: store.KindScheduled, ContactID: &c.ID, BlessingID: &previous.ID,
		Provider: "fake", ChatID: "972501234567@c.us", Status: store.StatusSent, EventYear: &year,
	}); err != nil {
		t.Fatalf("seeding send log: %v", err)
	}

	sel := newSelector(t, st)
	for range 10 {
		got, err := sel.ForContact(context.Background(), c)
		if err != nil {
			t.Fatalf("ForContact: %v", err)
		}
		if got.ID != other.ID {
			t.Fatalf("picked %d, want %d — last year's must be avoided", got.ID, other.ID)
		}
	}
}

func TestNewSelectorRequiresItsDependencies(t *testing.T) {
	if _, err := blessing.NewSelector(nil, logging.NewTo(io.Discard, "error", false)); err == nil {
		t.Error("want an error without a store")
	}
	if _, err := blessing.NewSelector(newStore(t), nil); err == nil {
		t.Error("want an error without a logger")
	}
}

func TestForContactRejectsANilContact(t *testing.T) {
	st := newStore(t)
	if _, err := newSelector(t, st).ForContact(context.Background(), nil); err == nil {
		t.Fatal("want an error for a nil contact")
	}
}

func TestForGroupOnlyReturnsNameFreeTemplates(t *testing.T) {
	st := newStore(t)
	addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "he", Text: "מזל טוב {{name}}"})
	want := addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "he", Text: "מזל טוב לכולם!"})

	sel := newSelector(t, st)
	for range 10 {
		got, err := sel.ForGroup(context.Background(), store.EventBirthday, "he")
		if err != nil {
			t.Fatalf("ForGroup: %v", err)
		}
		if got.ID != want.ID {
			t.Fatalf("picked %d, want the name-free template %d", got.ID, want.ID)
		}
	}
}

// Targeted templates have no single recipient in a group, so they are excluded
// even when they carry no placeholder.
func TestForGroupExcludesTargetedTemplates(t *testing.T) {
	st := newStore(t)
	female := store.GenderFemale
	addBlessing(t, st, store.Blessing{
		EventType: store.EventBirthday, Language: "he", Text: "מזל טוב!", Gender: &female,
	})
	friend := store.RelationFriend
	addBlessing(t, st, store.Blessing{
		EventType: store.EventBirthday, Language: "he", Text: "מזל טוב!", Relation: &friend,
	})

	if _, err := newSelector(t, st).ForGroup(context.Background(), store.EventBirthday, "he"); !errors.Is(err, blessing.ErrNoBlessing) {
		t.Fatalf("err = %v, want ErrNoBlessing", err)
	}
}

func TestForGroupFallsBackToEnglish(t *testing.T) {
	st := newStore(t)
	want := addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "en", Text: "Happy birthday!"})

	got, err := newSelector(t, st).ForGroup(context.Background(), store.EventBirthday, "ru")
	if err != nil {
		t.Fatalf("ForGroup: %v", err)
	}
	if got.ID != want.ID {
		t.Fatalf("picked %d, want the English fallback %d", got.ID, want.ID)
	}
}

// The starter seed must satisfy group echo out of the box.
func TestForGroupWorksWithTheSeededSet(t *testing.T) {
	st := newStore(t)
	if err := st.ApplySeedBlessings(context.Background()); err != nil {
		t.Fatalf("restoring the seed: %v", err)
	}

	for _, language := range []string{"he", "en"} {
		got, err := newSelector(t, st).ForGroup(context.Background(), store.EventBirthday, language)
		if err != nil {
			t.Fatalf("ForGroup(%s): %v", language, err)
		}
		if blessing.HasNamePlaceholder(got.Text) {
			t.Fatalf("seeded group template %q carries a name placeholder", got.Text)
		}
	}
}

// Spec §6: a language fallback must reach the UI, not just the log.
func TestLanguageFallbackRaisesANotice(t *testing.T) {
	st := newStore(t)
	addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "en", Text: "Happy birthday {{name}}"})
	c := addContact(t, st, store.Contact{Language: "ru"})

	if _, err := newSelector(t, st).ForContact(context.Background(), c); err != nil {
		t.Fatalf("ForContact: %v", err)
	}

	notices, err := st.ListNotices(context.Background(), true)
	if err != nil {
		t.Fatalf("listing notices: %v", err)
	}
	if len(notices) != 1 {
		t.Fatalf("got %d notices, want 1", len(notices))
	}
	if notices[0].Code != store.NoticeLanguageFallback {
		t.Errorf("code = %q, want %q", notices[0].Code, store.NoticeLanguageFallback)
	}
	if !strings.Contains(notices[0].Message, "ru") {
		t.Errorf("message = %q, want it to name the language", notices[0].Message)
	}
	if notices[0].Detail == nil || !strings.Contains(*notices[0].Detail, "Blessings") {
		t.Errorf("detail = %v, want it to say what to do about it", notices[0].Detail)
	}
}

// A whole address book missing one language must produce one actionable notice,
// not one per contact.
func TestRepeatedFallbacksCollapseIntoOneNotice(t *testing.T) {
	st := newStore(t)
	addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "en", Text: "Happy birthday {{name}}"})
	sel := newSelector(t, st)

	for i := range 4 {
		c := addContact(t, st, store.Contact{
			Name:     fmt.Sprintf("Ivan %d", i),
			Phone:    fmt.Sprintf("7916123456%d", i),
			Language: "ru",
		})
		if _, err := sel.ForContact(context.Background(), c); err != nil {
			t.Fatalf("ForContact: %v", err)
		}
	}

	notices, err := st.ListNotices(context.Background(), true)
	if err != nil {
		t.Fatalf("listing notices: %v", err)
	}
	if len(notices) != 1 {
		t.Fatalf("got %d notices, want them collapsed into 1", len(notices))
	}
	if notices[0].Occurrences != 4 {
		t.Errorf("occurrences = %d, want 4", notices[0].Occurrences)
	}
}

// No fallback, no notice — the UI must not nag when nothing is wrong.
func TestNoNoticeWhenTheContactLanguageIsAvailable(t *testing.T) {
	st := newStore(t)
	addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "he", Text: "מזל טוב {{name}}"})
	c := addContact(t, st, store.Contact{Language: "he"})

	if _, err := newSelector(t, st).ForContact(context.Background(), c); err != nil {
		t.Fatalf("ForContact: %v", err)
	}
	notices, err := st.ListNotices(context.Background(), true)
	if err != nil {
		t.Fatalf("listing notices: %v", err)
	}
	if len(notices) != 0 {
		t.Fatalf("got %d notices, want none", len(notices))
	}
}

// Losing a notice must never turn a successful send into a failure.
func TestNoticeFailureDoesNotBreakSelection(t *testing.T) {
	st := newStore(t)
	addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "en", Text: "Happy birthday {{name}}"})
	c := addContact(t, st, store.Contact{Language: "ru"})

	sel := newSelector(t, st)
	sel.SetNotifier(failingNotifier{})

	if _, err := sel.ForContact(context.Background(), c); err != nil {
		t.Fatalf("ForContact: %v, want the send to proceed regardless", err)
	}
}

type failingNotifier struct{}

func (failingNotifier) RaiseNotice(context.Context, store.Notice) error {
	return errors.New("database is on fire")
}
