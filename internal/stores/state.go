package stores

import (
	"context"
	"encoding/json"
	"errors"

	"unit/agent/internal/models"

	bolt "go.etcd.io/bbolt"
)

var (
	bucketWorkflows = []byte("workflows")

	ErrExecutionNotFound = errors.New("execution not found")
)

type StateStore interface {
	PutIfAbsent(ctx context.Context, job *models.DepositState) error
	Put(ctx context.Context, job *models.DepositState) error
	Get(ctx context.Context, id string) (*models.DepositState, error)
	Scan(ctx context.Context, visit func(*models.DepositState) error) error
}

type LocalStateStore struct {
	db *bolt.DB
}

func NewLocalStateStore(path string) (*LocalStateStore, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, e := tx.CreateBucketIfNotExists(bucketWorkflows); e != nil {
			return e
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &LocalStateStore{db: db}, nil
}

func (s *LocalStateStore) PutIfAbsent(ctx context.Context, job *models.DepositState) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkflows)
		if b.Get([]byte(job.ID)) != nil {
			return nil
		}
		blob, err := json.Marshal(job)
		if err != nil {
			return err
		}
		return b.Put([]byte(job.ID), blob)
	})
}

func (s *LocalStateStore) Put(ctx context.Context, job *models.DepositState) error {
	blob, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketWorkflows).Put([]byte(job.ID), blob)
	})
}

func (s *LocalStateStore) Get(ctx context.Context, id string) (*models.DepositState, error) {
	var out models.DepositState
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketWorkflows).Get([]byte(id))
		if v == nil {
			return ErrExecutionNotFound
		}
		return json.Unmarshal(v, &out)
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Scans all entries in store
func (s *LocalStateStore) Scan(ctx context.Context, visit func(*models.DepositState) error) error {
	return s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bucketWorkflows).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			var job models.DepositState
			if err := json.Unmarshal(v, &job); err != nil {
				return err
			}
			if err := visit(&job); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *LocalStateStore) Close() error {
	return s.db.Close()
}
