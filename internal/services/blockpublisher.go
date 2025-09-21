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

	lastBlock uint64
}

func NewBlockPublisher(client *ethclient.Client) *BlockPublisher {
	return &BlockPublisher{
		client:    client,
		interval:  2 * time.Second,
		out:       make(chan *types.Block, 20),
		err:       make(chan error, 1),
		lastBlock: 0,
	}
}

func (bp *BlockPublisher) Start(ctx context.Context) error {
	defer close(bp.out)
	defer close(bp.err)

	ticker := time.NewTicker(bp.interval)
	defer ticker.Stop()

	// start polling from last finalized block. In production, we should maintain a checkpointing component so that polling can continue from failure
	if bp.lastBlock == 0 {
		if block, err := bp.getLatestBlock(ctx); err == nil && block != nil {
			blockNumber := block.NumberU64()
			if blockNumber > 0 {
				bp.lastBlock = blockNumber - 1
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
			block, err := bp.getLatestBlock(ctx)
			if err != nil {
				select {
				case bp.err <- err:
				case <-ctx.Done():
					return ctx.Err()
				}
				continue
			}

			current := block.NumberU64()
			if current <= bp.lastBlock {
				continue
			}

			// publish all blocks between lastBlock and current
			for n := bp.lastBlock + 1; n <= current; n++ {
				if err := bp.publishBlock(ctx, n); err != nil {
					select {
					case bp.err <- err:
					case <-ctx.Done():
						return ctx.Err()
					}
					break
				}
				bp.lastBlock = n
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

func (bp *BlockPublisher) getLatestBlock(ctx context.Context) (*types.Block, error) {
	header, err := bp.client.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting latest header: %w", err)
	}
	if header == nil || header.Number == nil {
		return nil, fmt.Errorf("nil header")
	}

	block, err := bp.client.BlockByNumber(ctx, header.Number)
	if err != nil {
		return nil, fmt.Errorf("ethClient.BlockByNumber(%d): %w", header.Number.Uint64(), err)
	}
	return block, nil
}
