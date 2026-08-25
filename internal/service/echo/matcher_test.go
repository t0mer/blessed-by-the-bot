package echo_test

import (
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/service/echo"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func patterns(values ...string) []store.WishPattern {
	out := make([]store.WishPattern, 0, len(values))
	for i, v := range values {
		out = append(out, store.WishPattern{ID: int64(i + 1), Language: "he", Pattern: v, Enabled: true})
	}
	return out
}

func newMatcher(t *testing.T, values ...string) *echo.Matcher {
	t.Helper()
	m, problems := echo.NewMatcher(patterns(values...))
	if len(problems) != 0 {
		t.Fatalf("unexpected compile problems: %v", problems)
	}
	return m
}

func TestMatchesPlainSubstring(t *testing.T) {
	m := newMatcher(t, "מזל טוב")
	if got := m.Match("מזל טוב לך!"); got != "מזל טוב" {
		t.Fatalf("Match = %q, want the pattern", got)
	}
}

func TestMatchIsCaseInsensitive(t *testing.T) {
	m := newMatcher(t, "happy birthday")
	for _, text := range []string{"Happy Birthday!", "HAPPY BIRTHDAY", "happy birthday"} {
		if m.Match(text) == "" {
			t.Errorf("Match(%q) found nothing", text)
		}
	}
}

// Niqqud are separate combining code points, so a vocalised message would never
// match the plain stored pattern without normalization. This is the single most
// important case for a Hebrew group.
func TestMatchIgnoresHebrewNiqqud(t *testing.T) {
	m := newMatcher(t, "מזל טוב")
	if got := m.Match("מַזָּל טוֹב!"); got == "" {
		t.Fatal("a vocalised message must match the unvocalised pattern")
	}
}

// ...and the reverse: a vocalised pattern must match plain text.
func TestVocalisedPatternMatchesPlainText(t *testing.T) {
	m := newMatcher(t, "מַזָּל טוֹב")
	if got := m.Match("מזל טוב לכולם"); got == "" {
		t.Fatal("a vocalised pattern must match plain text")
	}
}

func TestMatchStripsLatinAccents(t *testing.T) {
	m := newMatcher(t, "felicidades")
	if m.Match("¡Felicidades!") == "" {
		t.Fatal("want an accent- and case-insensitive match")
	}
}

func TestMatchCollapsesWhitespace(t *testing.T) {
	m := newMatcher(t, "mazal tov")
	if m.Match("mazal    tov\n\nto you") == "" {
		t.Fatal("runs of whitespace must not defeat a match")
	}
}

func TestMatchesEmoji(t *testing.T) {
	m := newMatcher(t, "🎂")
	if m.Match("🎂🎉") == "" {
		t.Fatal("want an emoji pattern to match")
	}
}

func TestMatchesDelimitedRegex(t *testing.T) {
	m := newMatcher(t, `/happy\s+b-?day/`)
	for _, text := range []string{"happy bday", "Happy  B-Day!", "happy   bday"} {
		if m.Match(text) == "" {
			t.Errorf("Match(%q) found nothing", text)
		}
	}
	if m.Match("happy holidays") != "" {
		t.Error("want no match for unrelated text")
	}
}

func TestNoMatchForOrdinaryChatter(t *testing.T) {
	m := newMatcher(t, "מזל טוב", "happy birthday", "🎂")
	for _, text := range []string{"what time is dinner?", "אני בדרך", "ok"} {
		if got := m.Match(text); got != "" {
			t.Errorf("Match(%q) = %q, want no match", text, got)
		}
	}
}

func TestDisabledPatternsAreIgnored(t *testing.T) {
	m, problems := echo.NewMatcher([]store.WishPattern{
		{ID: 1, Language: "he", Pattern: "מזל טוב", Enabled: false},
	})
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if m.Match("מזל טוב") != "" {
		t.Fatal("a disabled pattern must not match")
	}
}

// One broken rule must not disable wish detection for everything else.
func TestBrokenRegexIsReportedButOthersStillWork(t *testing.T) {
	m, problems := echo.NewMatcher([]store.WishPattern{
		{ID: 1, Language: "en", Pattern: "/[unclosed/", Enabled: true},
		{ID: 2, Language: "he", Pattern: "מזל טוב", Enabled: true},
	})
	if len(problems) != 1 {
		t.Fatalf("problems = %v, want exactly one", problems)
	}
	if m.Match("מזל טוב") == "" {
		t.Fatal("the working pattern must still match")
	}
}

func TestEmptyTextNeverMatches(t *testing.T) {
	m := newMatcher(t, "מזל טוב")
	for _, text := range []string{"", "   ", "\n\t"} {
		if m.Match(text) != "" {
			t.Errorf("Match(%q) matched, want nothing", text)
		}
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	once := echo.Normalize("מַזָּל  טוֹב")
	if twice := echo.Normalize(once); twice != once {
		t.Fatalf("Normalize is not idempotent: %q then %q", once, twice)
	}
}
