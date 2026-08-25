package provider

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// DefaultSendInterval paces outbound messages. WhatsApp bans accounts that send
// in bursts, so the spec fixes this at one message every three seconds.
const DefaultSendInterval = 3 * time.Second

// RateLimited wraps p so SendText waits for a token before dispatching.
//
// Only sends are limited. ListGroups and Status are read-only and cannot get an
// account banned, so throttling them would just make the settings UI feel broken.
// An interval of zero or less disables limiting.
//
// This is also the single choke point every outbound message passes through,
// which is where the Prometheus counters from spec §4.3 will attach in phase 8.
func RateLimited(p Provider, interval time.Duration) Provider {
	if interval <= 0 {
		return p
	}
	return &rateLimited{
		Provider: p,
		limiter:  rate.NewLimiter(rate.Every(interval), 1),
	}
}

type rateLimited struct {
	Provider
	limiter *rate.Limiter
}

func (r *rateLimited) SendText(ctx context.Context, chatID, text string) (string, error) {
	if err := r.limiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("waiting for send rate limit: %w", err)
	}
	return r.Provider.SendText(ctx, chatID, text)
}
