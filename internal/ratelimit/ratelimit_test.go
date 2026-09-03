package ratelimit

import (
	"testing"
	"time"
)

func fakeClock(start time.Time) (func() time.Time, *time.Time) {
	now := start
	return func() time.Time { return now }, &now
}

func TestAllowWithinBurst(t *testing.T) {
	nowFn, _ := fakeClock(time.Unix(1000, 0))
	l := NewWithClock(600, 3, nowFn)
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("1.2.3.4"); !ok {
			t.Fatalf("request %d denied", i)
		}
	}
	if ok, _ := l.Allow("1.2.3.4"); ok {
		t.Fatal("4th immediate request must be denied")
	}
}

func TestAllowRefills(t *testing.T) {
	nowFn, now := fakeClock(time.Unix(1000, 0))
	l := NewWithClock(600, 2, nowFn)
	l.Allow("ip")
	l.Allow("ip")
	if ok, _ := l.Allow("ip"); ok {
		t.Fatal("bucket must be empty")
	}
	*now = nowFn().Add(200 * time.Millisecond)
	if ok, _ := l.Allow("ip"); !ok {
		t.Fatal("token must refill after 200ms at 600/min (10/sec)")
	}
}

func TestRetryAfter(t *testing.T) {
	nowFn, _ := fakeClock(time.Unix(1000, 0))
	l := NewWithClock(600, 1, nowFn)
	l.Allow("ip")
	ok, retry := l.Allow("ip")
	if ok {
		t.Fatal("denied expected")
	}
	if retry <= 0 || retry > 150*time.Millisecond {
		t.Fatalf("retry = %v, want ~100ms", retry)
	}
}

func TestIndependentIPs(t *testing.T) {
	nowFn, _ := fakeClock(time.Unix(1000, 0))
	l := NewWithClock(600, 1, nowFn)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("a denied")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("b must have its own bucket")
	}
}

func TestSweepEvictsIdle(t *testing.T) {
	nowFn, now := fakeClock(time.Unix(1000, 0))
	l := NewWithClock(600, 1, nowFn)
	l.Allow("old")
	l.Allow("new")
	*now = nowFn().Add(11 * time.Minute)
	l.Allow("new")
	*now = nowFn().Add(2 * time.Second)
	if n := l.Sweep(10 * time.Minute); n != 1 {
		t.Fatalf("sweep evicted %d, want 1", n)
	}
	if ok, _ := l.Allow("new"); !ok {
		t.Fatal("active bucket must survive (refilled by 11min of credit)")
	}
}

func TestNewUsesRealClock(t *testing.T) {
	l := New(30, 10)
	if l == nil {
		t.Fatal("nil limiter")
	}
	for i := 0; i < 10; i++ {
		if ok, _ := l.Allow("x"); !ok {
			t.Fatalf("burst of 10 must pass, failed at %d", i)
		}
	}
	if ok, _ := l.Allow("x"); ok {
		t.Fatal("11th immediate must be denied")
	}
}
