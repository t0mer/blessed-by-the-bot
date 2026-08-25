package provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
)

func TestRateLimitedDelegates(t *testing.T) {
	stub := &stubProvider{name: "greenapi"}
	limited := provider.RateLimited(stub, time.Millisecond)

	id, err := limited.SendText(context.Background(), "972501234567@c.us", "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if id != "msg-greenapi" {
		t.Errorf("message id = %q", id)
	}
	if stub.lastChatID != "972501234567@c.us" || stub.lastText != "hi" {
		t.Errorf("arguments not forwarded: chatID=%q text=%q", stub.lastChatID, stub.lastText)
	}
	if limited.Name() != "greenapi" {
		t.Errorf("Name() = %q, want greenapi", limited.Name())
	}
}

func TestRateLimitedPacesSends(t *testing.T) {
	stub := &stubProvider{name: "gowa"}
	limited := provider.RateLimited(stub, 50*time.Millisecond)

	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := limited.SendText(context.Background(), "x@c.us", "hi"); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	// The first send is free; the next two each wait one interval. No upper
	// bound is asserted — that only makes the test flaky on loaded CI.
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Errorf("three sends took %v, want at least 100ms of pacing", elapsed)
	}
	if stub.sends != 3 {
		t.Errorf("%d sends reached the provider, want 3", stub.sends)
	}
}

func TestRateLimitedDoesNotThrottleReads(t *testing.T) {
	stub := &stubProvider{name: "gowa"}
	limited := provider.RateLimited(stub, 10*time.Second)

	start := time.Now()
	for i := 0; i < 5; i++ {
		if _, err := limited.Status(context.Background()); err != nil {
			t.Fatalf("status %d: %v", i, err)
		}
		if _, err := limited.ListGroups(context.Background()); err != nil {
			t.Fatalf("list groups %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("read-only calls took %v; only sends can get an account banned", elapsed)
	}
}

func TestRateLimitedRespectsContext(t *testing.T) {
	limited := provider.RateLimited(&stubProvider{name: "gowa"}, time.Hour)

	if _, err := limited.SendText(context.Background(), "x@c.us", "first"); err != nil {
		t.Fatalf("first send: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := limited.SendText(ctx, "x@c.us", "second"); err == nil {
		t.Fatal("second send succeeded; it should have been blocked by the limiter")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("blocked for %v; a cancelled context must return promptly", elapsed)
	}
}

func TestRateLimitedZeroIntervalIsUnlimited(t *testing.T) {
	stub := &stubProvider{name: "gowa"}
	limited := provider.RateLimited(stub, 0)

	start := time.Now()
	for i := 0; i < 20; i++ {
		if _, err := limited.SendText(context.Background(), "x@c.us", "hi"); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("20 unlimited sends took %v", elapsed)
	}
}
