package service

import (
	"testing"
	"time"
)

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := newCircuitBreaker(2, time.Minute)
	host := "example.com"

	if !cb.allow(host) {
		t.Fatalf("expected allow before failures")
	}

	cb.failure(host)
	if !cb.allow(host) {
		t.Fatalf("expected allow before reaching threshold")
	}

	cb.failure(host)
	if cb.allow(host) {
		t.Fatalf("expected breaker to block after threshold")
	}

	cb.success(host)
	if !cb.allow(host) {
		t.Fatalf("expected allow after success reset")
	}
}

func TestCircuitBreaker_ClosesAfterCooldown(t *testing.T) {
	cooldown := 10 * time.Millisecond
	cb := newCircuitBreaker(1, cooldown)
	host := "cooldown.test"

	cb.failure(host)
	if cb.allow(host) {
		t.Fatalf("expected breaker open immediately after failure")
	}

	time.Sleep(cooldown + 5*time.Millisecond)
	if !cb.allow(host) {
		t.Fatalf("expected breaker to close after cooldown")
	}
}
