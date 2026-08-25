package blessing_test

import (
	"context"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/service/blessing"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func ptr[T any](v T) *T { return &v }

func contact() *store.Contact {
	return &store.Contact{
		ID: 1, Name: "Dana", EventType: store.EventBirthday, Language: "he",
		Relation: store.RelationCloseFriend, Gender: store.GenderFemale,
	}
}

// first is a deterministic stand-in for the random pick.
func first(int) int { return 0 }

func TestRenderSubstitutesName(t *testing.T) {
	got := blessing.Render("יום הולדת שמח {{name}}!", contact())
	if got != "יום הולדת שמח Dana!" {
		t.Fatalf("Render = %q", got)
	}
}

func TestRenderReplacesEveryOccurrence(t *testing.T) {
	got := blessing.Render("{{name}}, {{name}}!", contact())
	if got != "Dana, Dana!" {
		t.Fatalf("Render = %q", got)
	}
}

func TestRenderLeavesTemplateWithoutPlaceholderAlone(t *testing.T) {
	const text = "מזל טוב!"
	if got := blessing.Render(text, contact()); got != text {
		t.Fatalf("Render = %q, want it unchanged", got)
	}
}

func TestHasNamePlaceholder(t *testing.T) {
	if !blessing.HasNamePlaceholder("hi {{name}}") {
		t.Error("want true for a template with the placeholder")
	}
	if blessing.HasNamePlaceholder("mazal tov") {
		t.Error("want false for a name-free template")
	}
}

func TestPickPrefersGenderMatchOverWildcard(t *testing.T) {
	candidates := []store.Blessing{
		{ID: 1, Text: "wildcard"},
		{ID: 2, Text: "female", Gender: ptr(store.GenderFemale)},
	}
	got := blessing.Pick(candidates, contact(), nil, first)
	if got == nil || got.ID != 2 {
		t.Fatalf("picked %v, want the gender-matched template", got)
	}
}

func TestPickPrefersGenderOverRelation(t *testing.T) {
	// Gender outranks relation: Hebrew blessings are grammatically gendered, so
	// a wrong-gender template reads as broken in a way a generic relation does not.
	candidates := []store.Blessing{
		{ID: 1, Text: "relation only", Relation: ptr(store.RelationCloseFriend)},
		{ID: 2, Text: "gender only", Gender: ptr(store.GenderFemale)},
	}
	got := blessing.Pick(candidates, contact(), nil, first)
	if got == nil || got.ID != 2 {
		t.Fatalf("picked %v, want the gender-matched template", got)
	}
}

func TestPickPrefersBothOverEither(t *testing.T) {
	candidates := []store.Blessing{
		{ID: 1, Gender: ptr(store.GenderFemale)},
		{ID: 2, Gender: ptr(store.GenderFemale), Relation: ptr(store.RelationCloseFriend)},
		{ID: 3, Relation: ptr(store.RelationCloseFriend)},
	}
	got := blessing.Pick(candidates, contact(), nil, first)
	if got == nil || got.ID != 2 {
		t.Fatalf("picked %v, want the doubly-matched template", got)
	}
}

func TestPickExcludesMismatchedTargeting(t *testing.T) {
	// A male-only template must never reach a female contact, even if it is the
	// only candidate — sending it is worse than sending nothing.
	candidates := []store.Blessing{
		{ID: 1, Gender: ptr(store.GenderMale)},
		{ID: 2, Relation: ptr(store.RelationCoworker)},
	}
	if got := blessing.Pick(candidates, contact(), nil, first); got != nil {
		t.Fatalf("picked %v, want nil when nothing matches", got)
	}
}

func TestPickAvoidsLastYearsBlessing(t *testing.T) {
	candidates := []store.Blessing{{ID: 1}, {ID: 2}, {ID: 3}}
	got := blessing.Pick(candidates, contact(), ptr(int64(1)), first)
	if got == nil || got.ID == 1 {
		t.Fatalf("picked %v, want anything but last year's", got)
	}
}

// With one template, repeating it beats sending nothing.
func TestPickFallsBackToTheOnlyCandidate(t *testing.T) {
	candidates := []store.Blessing{{ID: 7}}
	got := blessing.Pick(candidates, contact(), ptr(int64(7)), first)
	if got == nil || got.ID != 7 {
		t.Fatalf("picked %v, want the sole candidate repeated", got)
	}
}

// The avoid rule applies within the winning tier only: dropping to a worse-
// fitting template just to avoid a repeat would be the wrong trade.
func TestPickKeepsTierOverVariety(t *testing.T) {
	candidates := []store.Blessing{
		{ID: 1, Gender: ptr(store.GenderFemale)},
		{ID: 2},
	}
	got := blessing.Pick(candidates, contact(), ptr(int64(1)), first)
	if got == nil || got.ID != 1 {
		t.Fatalf("picked %v, want the best-tier template even though it repeats", got)
	}
}

func TestPickReturnsNilForNoCandidates(t *testing.T) {
	if got := blessing.Pick(nil, contact(), nil, first); got != nil {
		t.Fatalf("picked %v, want nil", got)
	}
}

func TestPickUsesTheSuppliedChooser(t *testing.T) {
	candidates := []store.Blessing{{ID: 1}, {ID: 2}, {ID: 3}}
	got := blessing.Pick(candidates, contact(), nil, func(int) int { return 2 })
	if got == nil || got.ID != 3 {
		t.Fatalf("picked %v, want the chooser's index honoured", got)
	}
}

func TestRenderToleratesANilContact(t *testing.T) {
	const text = "מזל טוב {{name}}"
	if got := blessing.Render(text, nil); got != text {
		t.Fatalf("Render = %q, want the template untouched", got)
	}
}

// The chooser must actually spread across the pool, or the no-repeat bias is
// the only thing producing variety and two templates alternate forever.
func TestSelectorSpreadsAcrossEquallyGoodTemplates(t *testing.T) {
	st := newStore(t)
	for _, text := range []string{"one", "two", "three"} {
		addBlessing(t, st, store.Blessing{EventType: store.EventBirthday, Language: "he", Text: text})
	}
	c := addContact(t, st, store.Contact{})

	sel := newSelector(t, st)
	seen := map[int64]bool{}
	for range 60 {
		got, err := sel.ForContact(context.Background(), c)
		if err != nil {
			t.Fatalf("ForContact: %v", err)
		}
		seen[got.ID] = true
	}
	if len(seen) < 2 {
		t.Fatalf("only ever picked %d distinct template(s) in 60 tries", len(seen))
	}
}
