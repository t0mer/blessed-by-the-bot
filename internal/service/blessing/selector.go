package blessing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// Notifier records a condition worth surfacing in the UI. *store.Store
// satisfies it; it is an interface here so the selector stays testable and so
// this package does not depend on notices being stored at all.
type Notifier interface {
	RaiseNotice(ctx context.Context, n store.Notice) error
}

// Selector resolves a contact to a concrete template.
type Selector struct {
	store    *store.Store
	log      *slog.Logger
	notifier Notifier
}

// NewSelector builds a Selector. Both dependencies are required.
func NewSelector(st *store.Store, log *slog.Logger) (*Selector, error) {
	if st == nil {
		return nil, errors.New("blessing: store is required")
	}
	if log == nil {
		return nil, errors.New("blessing: logger is required")
	}
	// The store is the default notifier: a language fallback must reach the UI,
	// not just the log (spec §6). SetNotifier can replace it.
	return &Selector{store: st, log: log, notifier: st}, nil
}

// SetNotifier overrides where notices are recorded. Passing nil disables them,
// which is what a caller that only wants selection should do.
func (s *Selector) SetNotifier(n Notifier) { s.notifier = n }

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
			s.raiseFallbackNotice(ctx, c.EventType, c.Language)
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

// ForGroup returns a template suitable for posting into a group.
//
// Group echo must use a name-free template: the bot sees a burst of
// congratulation but does not know whose birthday it is, so a "{{name}}"
// placeholder has nothing to fill it with. Templates that carry one are
// filtered out here rather than rendered with an empty name.
//
// Gender and relation targeting are ignored for the same reason — there is no
// single recipient to target — so only unrestricted templates qualify.
func (s *Selector) ForGroup(ctx context.Context, eventType, language string) (*store.Blessing, error) {
	chosen, err := s.pickNameFree(ctx, eventType, language)
	if err != nil {
		return nil, err
	}
	if chosen != nil {
		return chosen, nil
	}

	if language != FallbackLanguage {
		chosen, err = s.pickNameFree(ctx, eventType, FallbackLanguage)
		if err != nil {
			return nil, err
		}
		if chosen != nil {
			s.log.Warn("no name-free group blessing in the group's language; falling back",
				"language", language, "fallback", FallbackLanguage)
			s.raiseFallbackNotice(ctx, eventType, language)
			return chosen, nil
		}
	}

	return nil, fmt.Errorf("%w: no name-free %s template in %s for group use",
		ErrNoBlessing, eventType, language)
}

func (s *Selector) pickNameFree(ctx context.Context, eventType, language string) (*store.Blessing, error) {
	candidates, err := s.store.FindBlessings(ctx, eventType, language)
	if err != nil {
		return nil, err
	}

	eligible := make([]store.Blessing, 0, len(candidates))
	for _, b := range candidates {
		if HasNamePlaceholder(b.Text) || b.Gender != nil || b.Relation != nil {
			continue
		}
		eligible = append(eligible, b)
	}
	if len(eligible) == 0 {
		return nil, nil
	}
	chosen := eligible[randomIndex(len(eligible))]
	return &chosen, nil
}

// raiseFallbackNotice surfaces a language fallback in the UI (spec §6).
//
// The key is the event type and language rather than the contact, so a whole
// address book missing Russian birthday templates produces one actionable
// notice instead of one per person. Recording it is best-effort: the message
// itself went out, and losing the notice must not turn a successful send into
// a failure.
func (s *Selector) raiseFallbackNotice(ctx context.Context, eventType, language string) {
	if s.notifier == nil {
		return
	}
	detail := fmt.Sprintf(
		"Add a %s template in %q under Blessings, or the English one keeps being used.",
		eventType, language)
	err := s.notifier.RaiseNotice(ctx, store.Notice{
		Key:   fmt.Sprintf("%s:%s:%s", store.NoticeLanguageFallback, eventType, language),
		Level: store.NoticeWarning,
		Code:  store.NoticeLanguageFallback,
		// "Using", not "sent": selection runs before delivery, so the message may
		// still fail afterwards. The notice is about the missing template, which
		// is true either way.
		Message: fmt.Sprintf("No %s blessing in %q; using the %s template instead.",
			eventType, language, FallbackLanguage),
		Detail: &detail,
	})
	if err != nil {
		s.log.Error("recording the language-fallback notice", "error", err)
	}
}
