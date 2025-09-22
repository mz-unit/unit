package stores

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"unit/agent/internal/models"
)

func newTestStateStore(t *testing.T) *LocalStateStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	s, err := NewLocalStateStore(dbPath)
	if err != nil {
		t.Fatalf("NewLocalStateStore error: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStateStore_PutAndGet(t *testing.T) {
	store := newTestStateStore(t)
	ctx := context.Background()

	in := &models.DepositState{ID: "dep_1"}

	if err := store.Put(ctx, in); err != nil {
		t.Fatalf("Put error: %v", err)
	}

	out, err := store.Get(ctx, "dep_1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if out == nil || out.ID != in.ID {
		t.Fatalf("Get mismatch: got %+v, want ID=%q", out, in.ID)
	}
}

func TestStateStore_Get_NotFound(t *testing.T) {
	store := newTestStateStore(t)

	_, err := store.Get(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != ErrExecutionNotFound {
		t.Fatalf("expected ErrExecutionNotFound, got %v", err)
	}
}

func TestStateStore_PutIfAbsent_InsertOnce(t *testing.T) {
	store := newTestStateStore(t)
	ctx := context.Background()

	s1 := &models.DepositState{ID: "same_id"}

	if err := store.PutIfAbsent(ctx, s1); err != nil {
		t.Fatalf("PutIfAbsent(1) error: %v", err)
	}

	s2 := &models.DepositState{ID: "same_id"}
	if err := store.PutIfAbsent(ctx, s2); err != nil {
		t.Fatalf("PutIfAbsent(2) error: %v", err)
	}

	var ids []string
	err := store.Scan(ctx, func(st *models.DepositState) error {
		ids = append(ids, st.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	var want = []string{"same_id"}
	sort.Strings(ids)
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("Scan IDs = %v, want %v", ids, want)
	}
}

func TestStateStore_Scan_VisitsAll(t *testing.T) {
	store := newTestStateStore(t)
	ctx := context.Background()

	wantIDs := []string{"a", "b", "c", "d", "e"}
	for _, id := range wantIDs {
		if err := store.Put(ctx, &models.DepositState{ID: id}); err != nil {
			t.Fatalf("Put(%s) error: %v", id, err)
		}
	}

	var gotIDs []string
	if err := store.Scan(ctx, func(st *models.DepositState) error {
		gotIDs = append(gotIDs, st.ID)
		return nil
	}); err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	sort.Strings(gotIDs)
	sort.Strings(wantIDs)
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("Scan IDs = %v, want %v", gotIDs, wantIDs)
	}
}

func TestStateStore_Scan_ContextCanceled(t *testing.T) {
	store := newTestStateStore(t)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := store.Put(ctx, &models.DepositState{ID: string(rune('x' + i))}); err != nil {
			t.Fatalf("Put error: %v", err)
		}
	}

	cctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := store.Scan(cctx, func(st *models.DepositState) error {
		calls++
		return nil
	})

	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}

	if calls != 0 {
		t.Fatalf("visitor called %d times, expected 0 due to cancellation", calls)
	}
}

func TestStateStore_Close(t *testing.T) {
	store := newTestStateStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}
