package greenapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

// Poller pacing.
const (
	// defaultIdleDelay is the pause after an empty poll. GreenAPI's
	// receiveNotification already long-polls, so this only prevents a hot loop
	// when it returns immediately.
	defaultIdleDelay = time.Second

	// defaultErrorDelay backs off after a failed poll so a provider outage does
	// not spin the CPU.
	defaultErrorDelay = 5 * time.Second
)

// HandlerFunc receives each incoming message the poller picks up.
type HandlerFunc func(ctx context.Context, msg *provider.IncomingMessage)

// Poller consumes GreenAPI's notification queue.
//
// This is the default incoming mode because it needs no public URL: a home-lab
// deployment behind NAT can still receive messages (spec §4.1).
type Poller struct {
	client     *Client
	log        *slog.Logger
	handle     HandlerFunc
	idleDelay  time.Duration
	errorDelay time.Duration
}

// NewPoller builds a poller that dispatches messages to handle.
func NewPoller(c *Client, log *slog.Logger, handle HandlerFunc) *Poller {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Poller{
		client:     c,
		log:        log,
		handle:     handle,
		idleDelay:  defaultIdleDelay,
		errorDelay: defaultErrorDelay,
	}
}

// SetIdleDelay overrides the pause between empty polls. Tests use a tiny value.
func (p *Poller) SetIdleDelay(d time.Duration) {
	if d > 0 {
		p.idleDelay = d
	}
}

// SetErrorDelay overrides the back-off after a failed poll. Tests use a tiny
// value so the outage-recovery path can be exercised without a real 5s wait.
func (p *Poller) SetErrorDelay(d time.Duration) {
	if d > 0 {
		p.errorDelay = d
	}
}

type receipt struct {
	ReceiptID int64           `json:"receiptId"`
	Body      json.RawMessage `json:"body"`
}

// Run polls until ctx is cancelled, at which point it returns nil.
func (p *Poller) Run(ctx context.Context) error {
	p.log.Info("greenapi poller started")
	defer p.log.Info("greenapi poller stopped")

	for {
		if ctx.Err() != nil {
			return nil
		}

		got, err := p.pollOnce(ctx)
		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil:
			// A provider outage must never kill the poller; back off and retry.
			p.log.Warn("greenapi poll failed", "error", err)
			if waitErr := sleepCtx(ctx, p.errorDelay); waitErr != nil {
				return nil
			}
		case !got:
			if waitErr := sleepCtx(ctx, p.idleDelay); waitErr != nil {
				return nil
			}
		}
	}
}

// pollOnce fetches at most one notification. got reports whether one arrived.
func (p *Poller) pollOnce(ctx context.Context) (bool, error) {
	var r receipt
	if err := p.client.tr.Do(ctx, "GET", p.client.endpoint("receiveNotification"), nil, nil, &r); err != nil {
		return false, fmt.Errorf("receiving notification: %w", err)
	}
	if r.ReceiptID == 0 {
		return false, nil
	}

	// Delete unconditionally, even when the payload is one we ignore: an
	// un-deleted receipt sits at the head of the queue and blocks everything
	// behind it.
	defer p.deleteNotification(ctx, r.ReceiptID)

	if len(r.Body) == 0 {
		return true, nil
	}
	msg, handled, err := ParseWebhook(r.Body)
	if err != nil {
		p.log.Warn("greenapi notification could not be parsed",
			"receipt", r.ReceiptID, "error", err)
		return true, nil
	}
	if handled && p.handle != nil {
		p.handle(ctx, msg)
	}
	return true, nil
}

func (p *Poller) deleteNotification(ctx context.Context, receiptID int64) {
	// The parent context may already be cancelled during shutdown; use a short
	// independent one so the receipt is still acknowledged.
	deleteCtx := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		deleteCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
	}

	url := fmt.Sprintf("%s/%d", p.client.endpoint("deleteNotification"), receiptID)
	if err := p.client.tr.Do(deleteCtx, "DELETE", url, nil, nil, nil); err != nil {
		p.log.Warn("greenapi notification delete failed; it will be redelivered",
			"receipt", receiptID, "error", err)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.New("context cancelled")
	case <-timer.C:
		return nil
	}
}
