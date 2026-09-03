package store

import (
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestOpenCreatesSchema(t *testing.T) {
	st := testStore(t)
	for _, table := range []string{"apps", "issues", "reports", "sessions"} {
		var n int
		err := st.db.QueryRow("SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&n)
		if err != nil || n != 1 {
			t.Fatalf("table %s missing (n=%d, err=%v)", table, n, err)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	if _, err := Open(path); err != nil {
		t.Fatal(err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer st.Close()
}
