package indexer

import (
	"context"

	"unit/agent/internal/models"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Indexer interface {
	Start() error
	Stop() error
	Out() <-chan models.Block
    Err() <-chan error
}

type Indexer struct {
	client *ethclient.Client
	interval time.Duration
	out chan models.Block
	err chan error
}

func NewIndexer(client *ethclient.Client, interval time.Duration) *Indexer {
	return &Indexer{
		client: client,
		interval: interval,
		out: make(chan models.Block),
		err: make(chan error),
	}
}

func (e *Indexer) Start(ctx context.Context, fromBlockNumber uint64) error {
	defer close(e.out)
	defer close(e.err)
}

func (e *Indexer) Out() <-chan models.Block { return e.out }

func (e *Indexer) Err() <-chan error { return e.err }

