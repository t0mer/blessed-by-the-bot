package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"time"
)

// Transport defaults.
const (
	defaultTimeout     = 30 * time.Second
	defaultAttempts    = 3
	defaultBaseBackoff = 500 * time.Millisecond

	// maxErrorBodyBytes caps how much of an error response is kept for the
	// message, so a provider returning an HTML error page cannot blow up a log line.
	maxErrorBodyBytes = 2048
)

// APIError is a non-2xx provider response.
type APIError struct {
	Status int
	Body   string
	URL    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("provider request to %s failed with status %d: %s", e.URL, e.Status, e.Body)
}

// Retryable reports whether the status is worth another attempt. 5xx and 429
// are transient; every other 4xx means the request itself is wrong and retrying
// would only burn the provider's rate limit.
func (e *APIError) Retryable() bool {
	return e.Status >= http.StatusInternalServerError || e.Status == http.StatusTooManyRequests
}

// TransportOptions configures NewTransport. The zero value is usable.
type TransportOptions struct {
	Client      *http.Client
	Attempts    int
	BaseBackoff time.Duration
	Logger      *slog.Logger
}

// Transport performs JSON requests against provider APIs with capped retries.
type Transport struct {
	client      *http.Client
	attempts    int
	baseBackoff time.Duration
	log         *slog.Logger
}

// NewTransport builds a Transport, filling in defaults for anything unset.
func NewTransport(opts TransportOptions) *Transport {
	t := &Transport{
		client:      opts.Client,
		attempts:    opts.Attempts,
		baseBackoff: opts.BaseBackoff,
		log:         opts.Logger,
	}
	if t.client == nil {
		t.client = &http.Client{Timeout: defaultTimeout}
	}
	if t.attempts <= 0 {
		t.attempts = defaultAttempts
	}
	if t.baseBackoff <= 0 {
		t.baseBackoff = defaultBaseBackoff
	}
	if t.log == nil {
		t.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return t
}

// Do sends body as JSON (nil for no body) and decodes a JSON response into out
// (nil to discard it). Network errors, 5xx and 429 are retried with jittered
// exponential backoff; every other 4xx fails immediately.
func (t *Transport) Do(ctx context.Context, method, rawURL string, headers http.Header, body, out any) error {
	var encoded []byte
	if body != nil {
		var err error
		if encoded, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt < t.attempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, backoffFor(attempt-1, t.baseBackoff)); err != nil {
				return err
			}
		}

		done, err := t.attemptOnce(ctx, method, rawURL, headers, encoded, out)
		if done {
			return err
		}
		lastErr = err

		// A cancelled context is final, however the failure surfaced.
		if ctx.Err() != nil {
			return errors.Join(ctx.Err(), lastErr)
		}
		t.log.Debug("provider request failed, retrying",
			"method", method, "host", safeHost(rawURL),
			"attempt", attempt+1, "of", t.attempts, "error", err)
	}
	return lastErr
}

// attemptOnce performs a single request. done is true when the outcome is final
// (success, or a failure that must not be retried).
func (t *Transport) attemptOnce(
	ctx context.Context, method, rawURL string, headers http.Header, encoded []byte, out any,
) (done bool, err error) {
	// A consumed reader cannot be replayed, so build a fresh one per attempt.
	var reader io.Reader
	if encoded != nil {
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return true, fmt.Errorf("building request: %w", err)
	}
	for key, values := range headers {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	if encoded != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("%s %s: %w", method, safeHost(rawURL), err)
	}
	defer func() {
		// Drain before closing so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		apiErr := &APIError{Status: resp.StatusCode, Body: string(snippet), URL: safeHost(rawURL)}
		return !apiErr.Retryable(), apiErr
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return true, nil
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("reading response body: %w", err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return true, nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return true, fmt.Errorf("decoding response body: %w", err)
	}
	return true, nil
}

// backoffFor returns an "equal jitter" delay in [d/2, d] where d = base * 2^attempt.
// This paces retries, it is not a security decision, so the non-cryptographic
// generator in math/rand/v2 is the right choice — do not "fix" it to crypto/rand.
func backoffFor(attempt int, base time.Duration) time.Duration {
	ceiling := base * (1 << attempt)
	half := ceiling / 2
	return half + time.Duration(rand.Int64N(int64(half)+1)) //nolint:gosec // pacing, not security
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// safeHost reduces a URL to scheme://host for logging and error messages.
//
// This matters: GreenAPI puts the API token in the URL *path*
// ({apiUrl}/waInstance{id}/{method}/{token}), so logging a full provider URL
// would leak the credential into logs and error strings.
func safeHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "provider"
	}
	return u.Scheme + "://" + u.Host
}
