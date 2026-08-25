package blessing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// Selector resolves a contact to a concrete template.
type Selector struct {
	store *store.Store
	log   *slog.Logger
}

// NewSelector builds a Selector. Both dependencies are required.
func NewSelector(st *store.Store, log *slog.Logger) (*Selector, error) {
	if st == nil {
		return nil, errors.New("blessing: store is required")
	}
	if log == nil {
		return nil, errors.New("blessing: logger is required")
	}
	return &Selector{store: st, log: log}, nil
}

// ForContact returns the template to send, or ErrNoBlessing.
//
// The contact's own language wins; English is tried only when that yields
// nothing, because a blessing in the wrong language is still better than
// silence — but it is worth a warning, since it usually means a missing template
// rather than a deliberate choice.
func (s *Selector) ForContact(ctx context.Context, c *store.Contact) (*store.Blessing, error) {
	if c == nil {
		return nil, errors.New("blessing: contact is required")
	}

	avoid, err := s.store.LastScheduledBlessing(ctx, c.ID)
	if err != nil {
		return nil, err
	}

	chosen, err := s.pickForLanguage(ctx, c, c.Language, avoid)
	if err != nil {
		return nil, err
	}
	if chosen != nil {
		return chosen, nil
	}

	if c.Language != FallbackLanguage {
		chosen, err = s.pickForLanguage(ctx, c, FallbackLanguage, avoid)
		if err != nil {
			return nil, err
		}
		if chosen != nil {
			s.log.Warn("no blessing in the contact's language; falling back",
				"contact_id", c.ID, "language", c.Language, "fallback", FallbackLanguage)
			return chosen, nil
		}
	}

	return nil, fmt.Errorf("%w: contact %d, event %s, language %s",
		ErrNoBlessing, c.ID, c.EventType, c.Language)
}

// pickForLanguage returns nil (not an error) when the language has no fit.
func (s *Selector) pickForLanguage(ctx context.Context, c *store.Contact, language string, avoid *int64) (*store.Blessing, error) {
	candidates, err := s.store.FindBlessings(ctx, c.EventType, language)
	if err != nil {
		return nil, err
	}
	return Pick(candidates, c, avoid, randomIndex), nil
}

// randomIndex is the production chooser. Spreading repeat sends across a tier is
// a cosmetic concern, so a non-cryptographic source is the right tool.
func randomIndex(n int) int {
	if n <= 1 {
		return 0
	}
	return rand.IntN(n) //nolint:gosec // variety in blessings, not security
}
