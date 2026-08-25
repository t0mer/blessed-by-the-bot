package blessing_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
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
	return st
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
