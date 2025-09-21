package services

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"unit/agent/internal/models"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type StateMachine struct {
	client   *ethclient.Client
	wm       *WalletManager // TODO instantiate wallet manager per chain
	accounts stores.AccountStore
	states   stores.StateStore

	treasuryAddr     string
	interval         time.Duration
	minConfirmations uint64
	maxAttempts      int
	backoff          func(n int) time.Duration
}

func NewStateMachine(client *ethclient.Client, wm *WalletManager, as stores.AccountStore, ss stores.StateStore, treasuryAddr string) (*StateMachine, error) {
	sm := &StateMachine{
		client:           client,
		wm:               wm,
		accounts:         as,
		states:           ss,
		treasuryAddr:     treasuryAddr,
		interval:         2 * time.Second,
		minConfirmations: 14, // Ethereum mainnet specific. For extensibility, create chain configs instead
		maxAttempts:      8,
		backoff: func(n int) time.Duration {
			d := time.Duration(1<<min(n, 10)) * time.Second
			return min(d, 2*time.Minute)
		},
	}
	return sm, nil
}

func (sm *StateMachine) Start(ctx context.Context) error {
	tick := time.NewTicker(sm.interval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			now := time.Now()
			err := sm.states.Scan(ctx, func(workflow *models.DepositState) error {
				if workflow.State == models.StateDone {
					return nil
				}

				if workflow.Attempts == sm.maxAttempts {
					workflow.State = models.StateFailed
					workflow.Error = "attempts exhausted"
					workflow.UpdatedAt = now
					_ = sm.states.Put(ctx, workflow)
					return nil
				}

				if dur := sm.backoff(workflow.Attempts); time.Since(workflow.UpdatedAt) < dur {
					return nil
				}
				changed, err := sm.Transition(ctx, workflow)
				if err != nil {
					workflow.Attempts++
					workflow.Error = err.Error()
					workflow.UpdatedAt = now
					_ = sm.states.Put(ctx, workflow)
					return nil
				}
				if changed {
					workflow.Error = ""
					workflow.UpdatedAt = now
					return sm.states.Put(ctx, workflow)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
}

func (sm *StateMachine) ProcessBlock(ctx context.Context, block *types.Block) error {
	for _, tx := range block.Transactions() {
		to := tx.To()
		if to == nil {
			// to is nil for contract deployments
			continue
		}

		// only checks of transaction.to is a deposit address. For a more thorough check, retrieve receipts and check logs for transfer events with deposit address as `to`
		account, err := sm.accounts.GetByDepositAddress(ctx, to.Hex())
		if err != nil {
			if errors.Is(err, stores.ErrAccountNotFound) {
				continue
			}
			return err
		}

		if account != nil && account.DepositAddr != nil {
			amount := new(big.Int).Set(tx.Value())
			deposit := &models.DepositState{
				ID:          fmt.Sprintf("%s|%s", *account.DepositAddr, tx.Hash().Hex()),
				TxHash:      tx.Hash().Hex(),
				DepositAddr: *account.DepositAddr,
				DstAddr:     account.DstAddr,
				DstChain:    account.DstChain,
				AmountWei:   amount,
				State:       models.StateDiscovered,
				UpdatedAt:   time.Now(),
				CreatedAt:   time.Now(),
			}

			err := sm.states.PutIfAbsent(ctx, deposit)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (sm *StateMachine) Transition(ctx context.Context, st *models.DepositState) (bool, error) {
	switch st.State {
	case models.StateDiscovered:
		// TODO: chain id is important, need to ensure we are creating tx on appropriate chain and sending on appropriate chain
		tx, err := sm.wm.PrepareSendTx(ctx, sm.treasuryAddr, st.DstAddr, st.AmountWei)
		if err != nil {
			return false, err
		}

		rawTxBytes, err := tx.MarshalBinary()
		if err != nil {
			return false, fmt.Errorf("error marshaling tx: %w", err)
		}

		st.UnsignedDstTx = common.Bytes2Hex(rawTxBytes)
		st.State = models.StateDstTxPrepared
		return true, nil

	case models.StateDstTxPrepared, models.StateDstTxRetry:
		var tx *types.Transaction
		if err := tx.UnmarshalBinary(common.Hex2Bytes(st.UnsignedDstTx)); err != nil {
			return false, fmt.Errorf("error unmarshaling tx: %w", err)
		}

		signed, err := sm.wm.SignTx(ctx, tx, sm.treasuryAddr)
		if err != nil {
			return false, fmt.Errorf("error signing tx: %w", err)
		}

		hash, err := sm.wm.SendTx(ctx, signed)
		if err != nil {
			return false, fmt.Errorf("error sending tx: %w", err)
		}

		st.SentDstTxHash = hash
		st.State = models.StateDstTxSent
		return true, nil

	case models.StateDstTxSent:
		rcpt, err := sm.client.TransactionReceipt(ctx, common.HexToHash(st.SentDstTxHash))
		if err != nil {
			return false, fmt.Errorf("error getting receipt: %w", err)
		}
		if rcpt.Status != types.ReceiptStatusSuccessful {
			st.State = models.StateDstTxRejected
			return true, fmt.Errorf("tx rejected, status=%d", rcpt.Status)
		}
		head, err := sm.client.BlockNumber(ctx)
		if err != nil {
			return false, fmt.Errorf("error getting latest block number: %w", err)
		}
		if head < rcpt.BlockNumber.Uint64()+sm.minConfirmations-1 {
			return false, fmt.Errorf("need more confirmations")
		}
		st.State = models.StateDstTxConfirmed
		return true, nil

	case models.StateDstTxConfirmed, models.StateSweepTxRetry:
		// TODO: chain id is important, need to ensure we are creating tx on appropriate chain
		tx, err := sm.wm.PrepareSweepTx(ctx, st.DepositAddr, sm.treasuryAddr)
		if err != nil {
			return false, err
		}

		rawTxBytes, err := tx.MarshalBinary()
		if err != nil {
			return false, fmt.Errorf("error marshaling tx: %w", err)
		}

		st.UnsignedSweepTx = common.Bytes2Hex(rawTxBytes)

		st.State = models.StateSweepTxPrepared
		return true, nil

	case models.StateSweepTxPrepared:
		var tx *types.Transaction
		if err := tx.UnmarshalBinary(common.Hex2Bytes(st.UnsignedSweepTx)); err != nil {
			return false, fmt.Errorf("error unmarshaling tx: %w", err)
		}

		signed, err := sm.wm.SignTx(ctx, tx, st.DepositAddr)
		if err != nil {
			return false, fmt.Errorf("error signing tx: %w", err)
		}

		hash, err := sm.wm.SendTx(ctx, signed)
		if err != nil {
			return false, fmt.Errorf("error sending tx: %w", err)
		}

		st.SentDstTxHash = hash
		st.State = models.StateSweepTxSent
		return true, nil

	case models.StateSweepTxSent:
		rcpt, err := sm.client.TransactionReceipt(ctx, common.HexToHash(st.SentSweepTxHash))
		if err != nil {
			return false, fmt.Errorf("error getting receipt: %w", err)
		}
		if rcpt.Status != types.ReceiptStatusSuccessful {
			st.State = models.StateSweepTxRejected
			return true, fmt.Errorf("tx rejected, status=%d", rcpt.Status)
		}
		head, err := sm.client.BlockNumber(ctx)
		if err != nil {
			return false, fmt.Errorf("error getting latest block number: %w", err)
		}
		if head < rcpt.BlockNumber.Uint64()+sm.minConfirmations-1 {
			return false, fmt.Errorf("need more confirmations")
		}
		st.State = models.StateSweepTxConfirmed
		return true, nil

	case models.StateSweepTxConfirmed:
		st.State = models.StateDone
		return true, nil

	case models.StateDone, models.StateFailed:
		// terminal states
		return false, nil

	case models.StateDstTxRejected:
		st.State = models.StateDstTxRetry
		return true, nil

	case models.StateSweepTxRejected:
		st.State = models.StateSweepTxRetry
		return true, nil

	default:
		return false, fmt.Errorf("unknown state %s", st.State)
	}
}
