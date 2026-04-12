package ratelimit

import (
	"sync"
	"time"
)

// bucket tracks tokens for a single IP.
type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// Limiter implements a per-IP token bucket rate limiter.
type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64       // tokens added per second
	capacity float64       // max tokens
	window   time.Duration // used for cleanup staleness check
}

// NewLimiter creates a rate limiter that allows maxRequests per window per IP.
// It starts a background goroutine to clean up stale entries every 5 minutes.
func NewLimiter(maxRequests int, window time.Duration) *Limiter {
	l := &Limiter{
		buckets:  make(map[string]*bucket),
		rate:     float64(maxRequests) / window.Seconds(),
		capacity: float64(maxRequests),
		window:   window,
	}
	go l.cleanup()
	return l
}

// Allow returns true if the request from the given IP should be allowed.
func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: l.capacity, lastCheck: now}
		l.buckets[ip] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.lastCheck = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// cleanup removes stale IPs every 5 minutes.
func (l *Limiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		cutoff := time.Now().Add(-2 * l.window)
		for ip, b := range l.buckets {
			if b.lastCheck.Before(cutoff) {
				delete(l.buckets, ip)
			}
		}
		l.mu.Unlock()
	}
}
