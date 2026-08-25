package echo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/service/blessing"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// RetentionPeriod is how long wish evidence is kept. The trigger window is
// hours, so a week is generous; the rows exist afterwards only for debugging.
const RetentionPeriod = 7 * 24 * time.Hour

// JanitorInterval is how often expired wish events are swept.
const JanitorInterval = 24 * time.Hour

// echoEventType is the event a group burst is assumed to be celebrating.
//
// v1 always treats it as a birthday: that is overwhelmingly the common case,
// and guessing wedding-vs-birthday from "mazal tov" is not something we can do
// reliably. eventTypeFor is the hook where a pattern-to-event mapping would go.
const echoEventType = store.EventBirthday

// Deps are the collaborators the engine needs.
type Deps struct {
	Store     *store.Store
	Settings  *settings.Service
	Providers *provider.Manager
	Blessings *blessing.Selector
	Logger    *slog.Logger

	// Now is the clock, injectable so tests can control the window and cooldown.
	Now func() time.Time
	// JanitorEvery overrides JanitorInterval.
	JanitorEvery time.Duration
}

// Engine counts wishes per group and posts one blessing when a burst is real.
type Engine struct {
	store        *store.Store
	settings     *settings.Service
	providers    *provider.Manager
	blessings    *blessing.Selector
	log          *slog.Logger
	now          func() time.Time
	janitorEvery time.Duration
}

// New validates deps and returns an Engine.
func New(deps Deps) (*Engine, error) {
	switch {
	case deps.Store == nil:
		return nil, errors.New("echo: store is required")
	case deps.Settings == nil:
		return nil, errors.New("echo: settings service is required")
	case deps.Providers == nil:
		return nil, errors.New("echo: provider manager is required")
	case deps.Blessings == nil:
		return nil, errors.New("echo: blessing selector is required")
	case deps.Logger == nil:
		return nil, errors.New("echo: logger is required")
	}

	now := deps.Now
	if now == nil {
		now = time.Now
	}
	every := deps.JanitorEvery
	if every <= 0 {
		every = JanitorInterval
	}
	return &Engine{
		store: deps.Store, settings: deps.Settings, providers: deps.Providers,
		blessings: deps.Blessings, log: deps.Logger, now: now, janitorEvery: every,
	}, nil
}

// HandleIncoming consumes one normalized inbound message, satisfying
// handlers.IncomingHandler.
//
// It returns an error only for genuine faults. A message that is simply not
// interesting — a private chat, an unconfigured group, ordinary conversation —
// is not an error, because the webhook endpoint answers 200 regardless and
// treating chatter as failure would fill the log with noise.
func (e *Engine) HandleIncoming(ctx context.Context, msg provider.IncomingMessage) error {
	if !msg.IsGroup || msg.FromMe {
		return nil
	}
	if msg.SenderID == "" || msg.MessageID == "" {
		// Without a sender the distinct-sender count is meaningless, and without
		// a message id the idempotency guard cannot work.
		e.log.Debug("ignoring a group message with no sender or id", "chat_id", msg.ChatID)
		return nil
	}

	group, err := e.store.GetGroupByChatID(ctx, msg.ChatID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("looking up group %s: %w", msg.ChatID, err)
	}
	if !group.Enabled {
		return nil
	}

	matcher, err := e.matcher(ctx)
	if err != nil {
		return err
	}
	matched := matcher.Match(msg.Text)
	if matched == "" {
		return nil
	}

	inserted, err := e.store.RecordWishEvent(ctx, &store.WishEvent{
		GroupID: group.ID, SenderID: msg.SenderID, MessageID: msg.MessageID,
		Matched: matched, CreatedAt: e.now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("recording wish event: %w", err)
	}
	if !inserted {
		// A provider redelivery. Re-evaluating would be harmless but pointless.
		return nil
	}
	e.log.Debug("wish recorded", "group_id", group.ID, "sender", msg.SenderID, "matched", matched)

	return e.maybeEcho(ctx, group)
}

// matcher builds a Matcher from the current patterns.
//
// It is rebuilt per message rather than cached because the patterns are
// editable in the UI and must take effect immediately; the table is tiny and
// group traffic is low, so the cost is irrelevant next to the surprise of an
// edit that does nothing until restart.
func (e *Engine) matcher(ctx context.Context) (*Matcher, error) {
	patterns, err := e.store.ListEnabledWishPatterns(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading wish patterns: %w", err)
	}
	m, problems := NewMatcher(patterns)
	for _, p := range problems {
		e.log.Error("skipping an uncompilable wish pattern", "error", p)
	}
	return m, nil
}

// maybeEcho posts a blessing when the group has crossed its threshold and is
// out of cooldown.
func (e *Engine) maybeEcho(ctx context.Context, group *store.Group) error {
	current, err := e.settings.Load(ctx)
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}

	threshold := current.GroupEcho.Threshold
	if group.Threshold != nil && *group.Threshold > 0 {
		threshold = *group.Threshold
	}

	now := e.now()
	windowStart := now.Add(-time.Duration(current.GroupEcho.WindowHours) * time.Hour)

	// Distinct senders, not messages: one excited friend sending five messages
	// is one vote (spec §2.2).
	senders, err := e.store.CountDistinctWishSenders(ctx, group.ID, windowStart)
	if err != nil {
		return fmt.Errorf("counting wish senders: %w", err)
	}
	if senders < threshold {
		e.log.Debug("below the echo threshold",
			"group_id", group.ID, "senders", senders, "threshold", threshold)
		return nil
	}

	cooling, err := e.inCooldown(ctx, group.ID, now, current.GroupEcho.CooldownHours)
	if err != nil {
		return err
	}
	if cooling {
		e.log.Debug("still cooling down", "group_id", group.ID)
		return nil
	}

	return e.postEcho(ctx, group, senders)
}

// inCooldown reports whether this group had an echo too recently. Together with
// the threshold this is what makes one birthday produce exactly one bot message.
func (e *Engine) inCooldown(ctx context.Context, groupID int64, now time.Time, cooldownHours int) (bool, error) {
	last, err := e.store.LastGroupEcho(ctx, groupID)
	if errors.Is(err, store.ErrNotFound) {
		// This group has never echoed, so nothing is holding it back.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the last group echo: %w", err)
	}
	if last == nil {
		return false, nil
	}
	return now.Sub(last.SentAt) < time.Duration(cooldownHours)*time.Hour, nil
}

// postEcho selects a name-free blessing and sends it to the group.
func (e *Engine) postEcho(ctx context.Context, group *store.Group, senders int) error {
	chosen, err := e.blessings.ForGroup(ctx, eventTypeFor(group), group.Language)
	if err != nil {
		return err
	}

	active, err := e.providers.Active()
	if err != nil {
		return err
	}

	entry := &store.SendLogEntry{
		Kind: store.KindGroupEcho, GroupID: &group.ID, BlessingID: &chosen.ID,
		Provider: active.Name(), ChatID: group.ChatID, SentAt: e.now().UTC(),
	}

	if _, sendErr := active.SendText(ctx, group.ChatID, chosen.Text); sendErr != nil {
		reason := sendErr.Error()
		entry.Status, entry.Error = store.StatusFailed, &reason
		if _, logErr := e.store.AppendSendLog(ctx, entry); logErr != nil {
			return fmt.Errorf("recording a failed group echo: %w", logErr)
		}
		return sendErr
	}

	entry.Status = store.StatusSent
	if _, err := e.store.AppendSendLog(ctx, entry); err != nil {
		// The message is already posted. Without the log row the cooldown cannot
		// see it, so the next wish would trigger a second echo — hence the loud
		// error rather than a warning.
		return fmt.Errorf("recording the group echo: %w", err)
	}

	e.log.Info("group echo sent",
		"group_id", group.ID, "chat_id", group.ChatID, "senders", senders, "blessing_id", chosen.ID)
	return nil
}

// eventTypeFor is the hook for mapping a group's burst to an event type. v1
// always answers birthday; a future version can inspect which pattern matched.
func eventTypeFor(_ *store.Group) string { return echoEventType }

// RunJanitor deletes expired wish evidence until ctx is cancelled.
//
// wish_events grows with every congratulation in every watched group and is only
// ever read over a window of hours, so without this the table would grow without
// bound for the life of the install.
func (e *Engine) RunJanitor(ctx context.Context) error {
	e.log.Info("wish-event janitor started", "interval", e.janitorEvery, "retention", RetentionPeriod)
	defer e.log.Info("wish-event janitor stopped")

	ticker := time.NewTicker(e.janitorEvery)
	defer ticker.Stop()

	for {
		if err := e.Sweep(ctx); err != nil && ctx.Err() == nil {
			e.log.Error("sweeping wish events failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Sweep deletes wish events older than the retention period.
func (e *Engine) Sweep(ctx context.Context) error {
	cutoff := e.now().Add(-RetentionPeriod)
	removed, err := e.store.DeleteWishEventsBefore(ctx, cutoff)
	if err != nil {
		return err
	}
	if removed > 0 {
		e.log.Info("swept expired wish events", "removed", removed, "cutoff", cutoff)
	}
	return nil
}
