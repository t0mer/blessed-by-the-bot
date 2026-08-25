package provider

import "time"

// BackoffFor exposes the unexported backoff calculation to the external test
// package so the jitter distribution can be asserted directly.
func BackoffFor(attempt int, base time.Duration) time.Duration {
	return backoffFor(attempt, base)
}
