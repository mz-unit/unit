package stores

import (
	"context"
	"encoding/json"
	"errors"

	"unit/agent/internal/models"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketByID   = []byte("accounts_by_id")
	bucketByAddr = []byte("accounts_by_addr")

	ErrAccountNotFound = errors.New("account not found")
)

type AccountStore interface {
	Insert(ctx context.Context, account models.Account) error
	Get(ctx context.Context, id string) (*models.Account, error)
	GetByDepositAddress(ctx context.Context, address string) (*models.Account, error)
	Close() error
}

type LocalAccountStore struct {
	db *bolt.DB
}

func NewLocalAccountStore(path string) (*LocalAccountStore, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		if _, e := tx.CreateBucketIfNotExists(bucketByID); e != nil {
			return e
		}
		if _, e := tx.CreateBucketIfNotExists(bucketByAddr); e != nil {
			return e
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &LocalAccountStore{
		db: db,
	}, nil
}

func (a *LocalAccountStore) Insert(ctx context.Context, account models.Account) error {
	data, err := json.Marshal(account)
	if err != nil {
		return err
	}

	err = a.db.Update(func(tx *bolt.Tx) error {
		byID := tx.Bucket(bucketByID)
		byAddr := tx.Bucket(bucketByAddr)

		if err := byID.Put([]byte(account.ID), data); err != nil {
			return err
		}

		if err := byAddr.Put([]byte(account.DepositAddr.Hex()), []byte(account.ID)); err != nil {
			return err
		}
		return nil
	})

	return err
}

func (a *LocalAccountStore) Get(ctx context.Context, id string) (*models.Account, error) {
	var acct models.Account

	err := a.db.View(func(tx *bolt.Tx) error {
		byID := tx.Bucket(bucketByID)
		v := byID.Get([]byte(id))
		if v == nil {
			return ErrAccountNotFound
		}
		return json.Unmarshal(v, &acct)
	})
	if err != nil {
		return nil, err
	}

	return &acct, nil
}

func (a *LocalAccountStore) GetByDepositAddress(ctx context.Context, address string) (*models.Account, error) {
	var acct *models.Account
	err := a.db.View(func(tx *bolt.Tx) error {
		byAddr := tx.Bucket(bucketByAddr)
		idBytes := byAddr.Get([]byte(address))
		if idBytes == nil {
			return ErrAccountNotFound
		}
		byID := tx.Bucket(bucketByID)
		v := byID.Get(idBytes)
		if v == nil {
			return ErrAccountNotFound
		}
		var a models.Account
		if err := json.Unmarshal(v, &a); err != nil {
			return err
		}
		acct = &a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return acct, nil
}

func (a *LocalAccountStore) Close() error {
	return a.db.Close()
}
