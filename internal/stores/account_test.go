package stores

import (
	"context"
	"path/filepath"
	"testing"

	"unit/agent/internal/models"

	"github.com/ethereum/go-ethereum/common"
)

func newTestStore(t *testing.T) *LocalAccountStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.db")
	s, err := NewLocalAccountStore(path)
	if err != nil {
		t.Fatalf("NewLocalAccountStore error: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s
}

func TestLocalAccountStore_InsertAndGet(t *testing.T) {
	store := newTestStore(t)

	ctx := context.Background()
	addr := common.HexToAddress("0x000000000000000000000000000000000000dEaD")
	acct := models.Account{
		ID:          "acct_1",
		DepositAddr: addr,
	}

	if err := store.Insert(ctx, acct); err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	got, err := store.Get(ctx, "acct_1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	if got.ID != acct.ID {
		t.Fatalf("Get ID mismatch: got %q want %q", got.ID, acct.ID)
	}
	if got.DepositAddr != acct.DepositAddr {
		t.Fatalf("Get DepositAddr mismatch: got %s want %s", got.DepositAddr.Hex(), acct.DepositAddr.Hex())
	}
}

func TestLocalAccountStore_Get_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.Get(context.Background(), "does_not_exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != ErrAccountNotFound {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestLocalAccountStore_GetByDepositAddress(t *testing.T) {
	store := newTestStore(t)

	ctx := context.Background()
	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	acct := models.Account{
		ID:          "acct_2",
		DepositAddr: addr,
	}

	if err := store.Insert(ctx, acct); err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	got, err := store.GetByDepositAddress(ctx, addr.Hex())
	if err != nil {
		t.Fatalf("GetByDepositAddress error: %v", err)
	}

	if got.ID != acct.ID {
		t.Fatalf("GetByDepositAddress ID mismatch: got %q want %q", got.ID, acct.ID)
	}
	if got.DepositAddr != acct.DepositAddr {
		t.Fatalf("GetByDepositAddress DepositAddr mismatch: got %s want %s", got.DepositAddr.Hex(), acct.DepositAddr.Hex())
	}
}

func TestLocalAccountStore_GetByDepositAddress_NotFound(t *testing.T) {
	store := newTestStore(t)

	_, err := store.GetByDepositAddress(context.Background(), common.HexToAddress("0x2222222222222222222222222222222222222222").Hex())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != ErrAccountNotFound {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestLocalAccountStore_Close(t *testing.T) {
	store := newTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}
