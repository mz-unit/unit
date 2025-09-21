package services

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type BlockPublisher struct {
	client   *ethclient.Client
	interval time.Duration

	out chan *types.Block
	err chan error

	lastFinalized uint64
}

func NewBlockListener(client *ethclient.Client, interval time.Duration) *BlockPublisher {
	return &BlockPublisher{
		client:        client,
		interval:      interval,
		out:           make(chan *types.Block),
		err:           make(chan error),
		lastFinalized: 0,
	}
}

func (bp *BlockPublisher) Start(ctx context.Context) error {
	defer close(bp.out)
	defer close(bp.err)

	ticker := time.NewTicker(bp.interval)
	defer ticker.Stop()

	// start polling from last finalized block. In production, we should maintain a checkpointing component so that polling can continue from failure
	if bp.lastFinalized == 0 {
		if block, err := bp.getLatestFinalized(ctx); err == nil {
			blockNumber := block.NumberU64()
			if blockNumber > 0 {
				bp.lastFinalized = blockNumber - 1
			}
		} else {
			select {
			case bp.err <- err:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			finalized, err := bp.getLatestFinalized(ctx)
			if err != nil {
				select {
				case bp.err <- err:
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}

			current := finalized.NumberU64()
			if current <= bp.lastFinalized {
				continue
			}

			for n := bp.lastFinalized + 1; n <= current; n++ {
				if err := bp.publishBlock(ctx, n); err != nil {
					select {
					case bp.err <- err:
					case <-ctx.Done():
						return ctx.Err()
					}
					break
				}
				bp.lastFinalized = n
			}
		}
	}
}

func (bp *BlockPublisher) Out() <-chan *types.Block { return bp.out }

func (bp *BlockPublisher) Err() <-chan error { return bp.err }

func (bp *BlockPublisher) publishBlock(ctx context.Context, blockNumber uint64) error {
	block, err := bp.client.BlockByNumber(ctx, new(big.Int).SetUint64(blockNumber))
	if err != nil {
		return err
	}

	select {
	case bp.out <- block:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (bp *BlockPublisher) getLatestFinalized(ctx context.Context) (*types.Block, error) {
	var block *types.Block
	if err := bp.client.Client().CallContext(ctx, &block, "eth_getBlockByNumber", "finalized", true /* return transactions */); err != nil {
		return nil, err
	}
	if block == nil {
		return nil, fmt.Errorf("received nil block")
	}
	return block, nil
}
