// Package echo watches configured WhatsApp groups and joins in when enough
// people start congratulating someone.
//
// The rule from the spec: when N distinct senders post a wish-like message
// inside a rolling window, the bot posts one blessing and then goes quiet for a
// cooldown, so a single birthday produces exactly one bot message.
package echo

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// Matcher tests message text against the enabled wish patterns.
//
// Patterns from every language are tried against every message: groups are
// multilingual, and a Hebrew group routinely carries "mazal tov" typed in Latin
// characters alongside "מזל טוב".
type Matcher struct {
	substrings []patternEntry
	regexps    []regexpEntry
}

type patternEntry struct {
	raw        string
	normalized string
}

type regexpEntry struct {
	raw string
	re  *regexp.Regexp
}

// NewMatcher compiles the enabled patterns. A pattern that fails to compile is
// skipped and reported, so one bad rule cannot disable wish detection entirely —
// the API validates regexes on save, but a hand-edited database can still carry
// a broken one.
func NewMatcher(patterns []store.WishPattern) (*Matcher, []error) {
	m := &Matcher{}
	var problems []error

	for _, p := range patterns {
		if !p.Enabled {
			continue
		}
		raw := strings.TrimSpace(p.Pattern)
		if raw == "" {
			continue
		}

		if body, ok := regexBody(raw); ok {
			// Case-insensitive: patterns are written for humans, not for a parser.
			re, err := regexp.Compile("(?i)" + body)
			if err != nil {
				problems = append(problems, err)
				continue
			}
			m.regexps = append(m.regexps, regexpEntry{raw: raw, re: re})
			continue
		}
		m.substrings = append(m.substrings, patternEntry{raw: raw, normalized: Normalize(raw)})
	}
	return m, problems
}

// regexBody reports whether raw is a /.../-delimited regex and returns its body.
func regexBody(raw string) (string, bool) {
	if len(raw) >= 3 && strings.HasPrefix(raw, "/") && strings.HasSuffix(raw, "/") {
		return raw[1 : len(raw)-1], true
	}
	return "", false
}

// Match reports the first pattern the text satisfies, or "" for no match. The
// matched pattern is returned so it can be recorded on the wish event, which
// makes "why did the bot fire?" answerable from the database alone.
func (m *Matcher) Match(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	normalized := Normalize(text)

	for _, p := range m.substrings {
		if strings.Contains(normalized, p.normalized) {
			return p.raw
		}
	}
	// Regexes run against the normalized text too, so /מזל טוב/ matches a
	// vocalised message. A pattern needing raw text can still use \p{...}.
	for _, r := range m.regexps {
		if r.re.MatchString(normalized) {
			return r.raw
		}
	}
	return ""
}

// Normalize prepares text for comparison: decomposed, stripped of combining
// marks, casefolded and whitespace-collapsed.
//
// Dropping combining marks is what makes Hebrew work. Niqqud (the vowel points
// in "מַזָּל טוֹב") are separate combining code points, so a vocalised message
// would never match the plain pattern "מזל טוב" without this. The same pass
// makes "Café" match "cafe".
func Normalize(text string) string {
	decomposed := norm.NFD.String(text)

	var b strings.Builder
	b.Grow(len(decomposed))
	lastWasSpace := false
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
			// A combining mark: niqqud, an accent, an Arabic harakat.
			continue
		case unicode.IsSpace(r):
			if !lastWasSpace {
				b.WriteRune(' ')
				lastWasSpace = true
			}
		default:
			b.WriteRune(unicode.ToLower(r))
			lastWasSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}
