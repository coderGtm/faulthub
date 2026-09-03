// Package ratelimit provides per-IP token bucket rate limiting.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

type Limiter struct {
	rate    float64
	burst   float64
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

func New(ratePerMin, burst int) *Limiter {
	return NewWithClock(float64(ratePerMin)/60.0, burst, time.Now)
}

func NewWithClock(ratePerSec float64, burst int, now func() time.Time) *Limiter {
	if burst < 1 {
		burst = 1
	}
	if ratePerSec <= 0 {
		ratePerSec = 1.0 / 60.0
	}
	return &Limiter{
		rate:    ratePerSec,
		burst:   float64(burst),
		buckets: make(map[string]*bucket),
		now:     now,
	}
}

func (l *Limiter) Allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[ip] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens += elapsed * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	need := (1 - b.tokens) / l.rate
	return false, time.Duration(need * float64(time.Second))
}

func (l *Limiter) Sweep(maxIdle time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.now().Add(-maxIdle)
	n := 0
	for ip, b := range l.buckets {
		if b.last.Before(cutoff) {
			delete(l.buckets, ip)
			n++
		}
	}
	return n
}
