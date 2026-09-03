package store

import (
	"context"
	"errors"
	"testing"
)

func TestAppCRUD(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	a, err := st.CreateApp(ctx, "Yantra", "hash1")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || a.Name != "Yantra" || a.CreatedAt.IsZero() {
		t.Fatalf("got %+v", a)
	}
	if _, err := st.CreateApp(ctx, "Yantra", "hash2"); err == nil {
		t.Fatal("duplicate name must fail")
	}

	got, err := st.GetAppByID(ctx, a.ID)
	if err != nil || got.Name != "Yantra" {
		t.Fatalf("GetAppByID: %+v %v", got, err)
	}
	byKey, err := st.GetAppByKeyHash(ctx, "hash1")
	if err != nil || byKey.ID != a.ID {
		t.Fatalf("GetAppByKeyHash: %+v %v", byKey, err)
	}
	if _, err := st.GetAppByKeyHash(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := st.GetAppByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	if err := st.RotateAppKey(ctx, a.ID, "hash3"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetAppByKeyHash(ctx, "hash1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("old key must be gone")
	}
	if _, err := st.GetAppByKeyHash(ctx, "hash3"); err != nil {
		t.Fatal("new key must resolve")
	}

	list, err := st.ListApps(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListApps: %+v %v", list, err)
	}
	if err := st.DeleteApp(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetAppByID(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("app must be deleted")
	}
}
