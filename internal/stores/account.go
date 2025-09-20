package stores

import (
	"context"
	"database/sql"

	"unit/agent/internal/models"
)

type AccountStore interface {
	Insert(ctx context.Context, account models.Account) error
	GetByID(ctx context.Context, id string) (*models.Account, error)
	GetByDepositAddress(ctx context.Context, address string) (*models.Account, error)
}

type PostgresAccountStore struct {
	db *sql.DB
}

func NewPostgresAccountStore(db *sql.DB) (*PostgresAccountStore, error) {
	return &PostgresAccountStore{
		db: db,
	}, nil
}

func (p *PostgresAccountStore) GetByID(ctx context.Context, id string) (*models.Account, error) {
	return nil, nil
}
