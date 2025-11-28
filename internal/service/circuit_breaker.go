package service

import (
	"sync"
	"time"
)

// circuitBreaker limits outbound requests to hosts that consistently fail.
type circuitBreaker struct {
	mu        sync.Mutex
	failures  map[string]uint32
	lastSeen  map[string]time.Time
	threshold uint32
	cooldown  time.Duration
}

func newCircuitBreaker(threshold uint32, cooldown time.Duration) *circuitBreaker {
	if threshold == 0 {
		threshold = 3
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &circuitBreaker{
		failures:  make(map[string]uint32),
		lastSeen:  make(map[string]time.Time),
		threshold: threshold,
		cooldown:  cooldown,
	}
}

func (cb *circuitBreaker) allow(host string) bool {
	if host == "" {
		return true
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()

	failures := cb.failures[host]
	if failures < cb.threshold {
		return true
	}
	last := cb.lastSeen[host]
	if time.Since(last) > cb.cooldown {
		delete(cb.failures, host)
		delete(cb.lastSeen, host)
		return true
	}
	return false
}

func (cb *circuitBreaker) success(host string) {
	if host == "" {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.failures, host)
	delete(cb.lastSeen, host)
}

func (cb *circuitBreaker) failure(host string) {
	if host == "" {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures[host]++
	cb.lastSeen[host] = time.Now()
}
