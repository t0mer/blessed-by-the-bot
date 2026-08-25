// Package blessing chooses which template to send to whom and fills it in.
//
// Selection is deliberately split in two: Pick is a pure function over
// candidates so the tier rules can be tested without a database, and Selector
// wraps it with the store lookup and the language fallback.
package blessing

import (
	"errors"
	"strings"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// NamePlaceholder is substituted with the contact's name.
const NamePlaceholder = "{{name}}"

// FallbackLanguage is used when a contact's own language has no template.
const FallbackLanguage = "en"

// ErrNoBlessing means nothing suitable exists. A fresh install hits this until
// the user adds templates, so callers must report it, not panic on it.
var ErrNoBlessing = errors.New("no blessing matches this contact")

// Render fills a template in for one contact.
func Render(text string, c *store.Contact) string {
	if c == nil {
		return text
	}
	return strings.ReplaceAll(text, NamePlaceholder, c.Name)
}

// HasNamePlaceholder reports whether a template addresses someone by name.
// Group echo uses it to exclude templates it cannot fill in: the bot does not
// know whose birthday a group is celebrating.
func HasNamePlaceholder(text string) bool {
	return strings.Contains(text, NamePlaceholder)
}

// Tier scores. Gender outranks relation because Hebrew blessings are
// grammatically gendered — a wrong-gender template reads as broken, while a
// generic relation merely reads as generic.
const (
	scoreGender   = 2
	scoreRelation = 1
	scoreExcluded = -1
)

// Pick chooses the best-fitting candidate.
//
// A template whose gender or relation is set but does not match the contact is
// excluded outright; one where the field is nil fits anybody. Among the highest
// scoring candidates, avoid (last year's blessing) is skipped when there is an
// alternative in that same tier — variety is worth having, but never at the cost
// of a worse-fitting template. pickIndex chooses within the winning tier and
// receives its length; production passes a random chooser, tests a fixed one.
func Pick(candidates []store.Blessing, c *store.Contact, avoid *int64, pickIndex func(n int) int) *store.Blessing {
	best := make([]store.Blessing, 0, len(candidates))
	bestScore := scoreExcluded

	for _, b := range candidates {
		score := fit(&b, c)
		switch {
		case score == scoreExcluded:
			continue
		case score > bestScore:
			bestScore = score
			best = append(best[:0], b)
		case score == bestScore:
			best = append(best, b)
		}
	}
	if len(best) == 0 {
		return nil
	}

	pool := best
	if avoid != nil && len(best) > 1 {
		filtered := make([]store.Blessing, 0, len(best))
		for _, b := range best {
			if b.ID != *avoid {
				filtered = append(filtered, b)
			}
		}
		// Every candidate being last year's is only possible in a one-template
		// tier, which the length check above already excluded; guard anyway.
		if len(filtered) > 0 {
			pool = filtered
		}
	}

	idx := pickIndex(len(pool))
	if idx < 0 || idx >= len(pool) {
		idx = 0
	}
	chosen := pool[idx]
	return &chosen
}

// fit scores one template against one contact, or reports exclusion.
func fit(b *store.Blessing, c *store.Contact) int {
	score := 0
	if b.Gender != nil {
		if *b.Gender != c.Gender {
			return scoreExcluded
		}
		score += scoreGender
	}
	if b.Relation != nil {
		if *b.Relation != c.Relation {
			return scoreExcluded
		}
		score += scoreRelation
	}
	return score
}
