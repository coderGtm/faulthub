package store

import (
	"context"
	"testing"
	"time"
)

func TestSessions(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	now := time.Now()

	if err := st.CreateSession(ctx, "tok1", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ok, err := st.ValidSession(ctx, "tok1", now)
	if err != nil || !ok {
		t.Fatalf("want valid, got ok=%v err=%v", ok, err)
	}
	ok, _ = st.ValidSession(ctx, "tok1", now.Add(2*time.Hour))
	if ok {
		t.Fatal("expired session must be invalid")
	}
	ok, _ = st.ValidSession(ctx, "missing", now)
	if ok {
		t.Fatal("unknown token must be invalid")
	}

	if err := st.TouchSession(ctx, "tok1", now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	ok, _ = st.ValidSession(ctx, "tok1", now.Add(90*time.Minute))
	if !ok {
		t.Fatal("touched session must be valid past old expiry")
	}

	if err := st.DeleteSession(ctx, "tok1"); err != nil {
		t.Fatal(err)
	}
	ok, _ = st.ValidSession(ctx, "tok1", now)
	if ok {
		t.Fatal("deleted session must be invalid")
	}

	st.CreateSession(ctx, "old", now.Add(-time.Minute))
	st.CreateSession(ctx, "fresh", now.Add(time.Hour))
	n, err := st.PruneSessions(ctx, now)
	if err != nil || n != 1 {
		t.Fatalf("PruneSessions: n=%d err=%v", n, err)
	}
	ok, _ = st.ValidSession(ctx, "fresh", now)
	if !ok {
		t.Fatal("fresh session must survive prune")
	}
}
