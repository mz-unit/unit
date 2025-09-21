package services

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"unit/agent/internal/models"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	hyperliquid "github.com/sonirico/go-hyperliquid"
)

type StateMachine struct {
	client   *ethclient.Client
	wm       *WalletManager
	accounts stores.AccountStore
	states   stores.StateStore
	hl       *hyperliquid.Exchange

	hotWallets       map[models.Chain]string
	interval         time.Duration
	minConfirmations uint64
	minDepositWei    *big.Int
	maxAttempts      int
	backoff          func(n int) time.Duration
}

func NewStateMachine(client *ethclient.Client, wm *WalletManager, as stores.AccountStore, ss stores.StateStore, hl *hyperliquid.Exchange, hotWallets map[models.Chain]string) (*StateMachine, error) {
	sm := &StateMachine{
		client:           client,
		wm:               wm,
		accounts:         as,
		states:           ss,
		hl:               hl,
		hotWallets:       hotWallets,
		interval:         2 * time.Second,
		minConfirmations: 14, // Ethereum mainnet specific
		maxAttempts:      5,
		minDepositWei:    new(big.Int).SetUint64(1000000000000000000), // .01
		backoff: func(n int) time.Duration {
			d := time.Duration(1<<min(n, 10)) * time.Second
			return min(d, 2*time.Minute)
		},
	}
	return sm, nil
}

func (sm *StateMachine) Start(ctx context.Context) error {
	ticker := time.NewTicker(sm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			now := time.Now()

			if err := sm.states.Scan(ctx, func(wf *models.DepositState) error {
				switch wf.State {
				case models.StateDone, models.StateFailed:
					return nil
				}

				if wf.Attempts >= sm.maxAttempts {
					wf.State = models.StateFailed
					wf.Error = "retries exhausted"
					wf.UpdatedAt = now
					fmt.Printf("deposit to %s retries exhausted at state %s\n", wf.DepositAddr.Hex(), wf.State)
					return sm.states.Put(ctx, wf)
				}

				if since := now.Sub(wf.UpdatedAt); since < sm.backoff(wf.Attempts) {
					return nil
				}

				changed, err := sm.TransitionDeposit(ctx, wf)
				if err != nil {
					wf.Attempts++
					wf.Error = err.Error()
					wf.UpdatedAt = now
					fmt.Printf("deposit to %s failed at state %s: %s, %d/%d attempts\n", wf.DepositAddr.Hex(), wf.State, err.Error(), wf.Attempts, sm.maxAttempts)
					return sm.states.Put(ctx, wf)
				}
				if !changed {
					return nil
				}

				wf.Error = ""
				wf.UpdatedAt = now
				return sm.states.Put(ctx, wf)
			}); err != nil {
				return err
			}
		}
	}
}

func (sm *StateMachine) ProcessBlock(ctx context.Context, block *types.Block) error {
	for _, tx := range block.Transactions() {
		to := tx.To()
		if to == nil {
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

		amount := new(big.Int).Set(tx.Value())
		if account != nil && amount.Cmp(sm.minDepositWei) > 0 {
			deposit := &models.DepositState{
				ID:          fmt.Sprintf("%s|%s", account.DepositAddr, tx.Hash().Hex()),
				TxHash:      tx.Hash().Hex(),
				DepositAddr: account.DepositAddr,
				DstAddr:     account.DstAddr,
				DstChain:    account.DstChain,
				SrcChain:    account.SrcChain,
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

func (sm *StateMachine) TransitionDeposit(ctx context.Context, st *models.DepositState) (changed bool, err error) {
	switch st.State {
	case models.StateDiscovered, models.StateDstTxResend:
		addr, err := sm.getHotWallet(st.DstChain)
		if err != nil {
			return false, err
		}
		tx, err := sm.wm.WithChain(st.DstChain).BuildSendTx(ctx, addr, st.DstAddr.Hex(), st.AmountWei) // TODO: convert eth wei to usdc
		if err != nil {
			return false, fmt.Errorf("error building tx: %v", err)
		}

		st.UnsignedDstTx = tx
		st.State = models.StateDstTxBuilt
		return true, nil

	case models.StateDstTxBuilt:
		addr, err := sm.getHotWallet(st.DstChain)
		if err != nil {
			return false, err
		}

		hash, err := sm.wm.WithChain(st.DstChain).SendTx(ctx, st.UnsignedDstTx, addr)
		if err != nil {
			return false, fmt.Errorf("error signing tx: %v", err)
		}

		st.SentDstTxHash = hash
		st.State = models.StateDstTxSent
		return true, nil

	case models.StateDstTxSent:
		confirmed, err := sm.wm.WithChain(st.DstChain).WaitForConfirmations(ctx, st.SentDstTxHash, sm.minConfirmations)
		if err != nil {
			if errors.Is(err, ErrorRejectedTransaction) {
				st.State = models.StateDstTxRejected
				return true, nil
			}
			return false, fmt.Errorf("error waiting for confirmations")
		}

		if !confirmed {
			return false, fmt.Errorf("needs more confirmations")
		}

		st.State = models.StateDstTxConfirmed
		return true, nil

	case models.StateDstTxConfirmed, models.StateSweepTxResend:
		addr, err := sm.getHotWallet(st.SrcChain)
		if err != nil {
			return false, err
		}
		tx, err := sm.wm.WithChain(st.SrcChain).BuildSweepTx(ctx, st.DepositAddr.Hex(), addr)
		if err != nil {
			return false, err
		}

		st.UnsignedSweepTx = tx
		st.State = models.StateSweepTxBuilt
		return true, nil

	case models.StateSweepTxBuilt:
		hash, err := sm.wm.WithChain(st.SrcChain).SendTx(ctx, st.UnsignedSweepTx, st.DepositAddr.Hex())
		if err != nil {
			return false, fmt.Errorf("error signing tx: %v", err)
		}

		st.SentSweepTxHash = hash
		st.State = models.StateSweepTxSent
		return true, nil

	case models.StateSweepTxSent:
		confirmed, err := sm.wm.WithChain(st.SrcChain).WaitForConfirmations(ctx, st.SentSweepTxHash, sm.minConfirmations)
		if err != nil {
			if errors.Is(err, ErrorRejectedTransaction) {
				st.State = models.StateSweepTxRejected
				return true, nil
			}
			return false, fmt.Errorf("error waiting for confirmations")
		}

		if !confirmed {
			return false, fmt.Errorf("needs more confirmations")
		}

		st.State = models.StateSweepTxConfirmed
		return true, nil

	case models.StateSweepTxConfirmed:
		st.State = models.StateDone
		return true, nil

	case models.StateDone, models.StateFailed:
		return false, nil

	case models.StateDstTxRejected:
		st.State = models.StateDstTxResend
		return true, nil

	case models.StateSweepTxRejected:
		st.State = models.StateSweepTxResend
		return true, nil

	default:
		return false, fmt.Errorf("unknown state %s", st.State)
	}
}

func (sm *StateMachine) getHotWallet(chain models.Chain) (string, error) {
	address, ok := sm.hotWallets[chain]
	if !ok {
		return "", fmt.Errorf("no hot wallet for chain %s", chain)
	}

	return address, nil
}
