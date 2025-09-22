package mocks

import (
	"context"
	"math/big"
	"unit/agent/internal/models"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/core/types"
)

type MockKeyStore struct {
	Addr   string
	Err    error
	Called int
}

func (f *MockKeyStore) CreateKey(ctx context.Context) (string, error) {
	f.Called++
	return f.Addr, f.Err
}
func (f *MockKeyStore) HasKey(ctx context.Context, addr string) bool {
	return addr == f.Addr
}
func (f *MockKeyStore) SignTx(ctx context.Context, address string, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	return tx, nil
}

type MockAccountStore struct {
	GetFn     func(ctx context.Context, id string) (*models.Account, error)
	InsertFn  func(ctx context.Context, a models.Account) error
	Inserted  *models.Account
	InsertErr error
}

func (f *MockAccountStore) Get(ctx context.Context, id string) (*models.Account, error) {
	if f.GetFn != nil {
		return f.GetFn(ctx, id)
	}
	return nil, stores.ErrAccountNotFound
}

func (f *MockAccountStore) Insert(ctx context.Context, a models.Account) error {
	f.Inserted = &a
	if f.InsertFn != nil {
		return f.InsertFn(ctx, a)
	}
	return f.InsertErr
}
func (f *MockAccountStore) GetByDepositAddress(ctx context.Context, address string) (*models.Account, error) {
	if f.GetFn != nil {
		return f.GetFn(ctx, address)
	}
	return nil, stores.ErrAccountNotFound
}
