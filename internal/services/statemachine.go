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
)

type StateMachine struct {
	wm       *WalletManager
	accounts stores.AccountStore
	states   stores.StateStore

	hotWallets       map[models.Chain]string
	interval         time.Duration
	minConfirmations uint64
	minDepositWei    *big.Int
	maxAttempts      int
}

func NewStateMachine(wm *WalletManager, as stores.AccountStore, ss stores.StateStore, hotWallets map[models.Chain]string) (*StateMachine, error) {
	sm := &StateMachine{
		wm:               wm,
		accounts:         as,
		states:           ss,
		hotWallets:       hotWallets,
		interval:         5 * time.Second,
		minConfirmations: 14, // Ethereum mainnet specific
		maxAttempts:      1000,
		minDepositWei:    new(big.Int).SetUint64(1000000000000000000), // .01
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

			depositCount := 0
			var updates []*models.DepositState

			if err := sm.states.Scan(ctx, func(st *models.DepositState) error {
				depositCount++

				switch st.State {
				case models.StateDone, models.StateFailed:
					return nil
				}

				fmt.Printf("deposit to %s current state: %s\n", st.DepositAddr.Hex(), st.State)

				if st.Attempts >= sm.maxAttempts {
					st.State = models.StateFailed
					st.Error = "retries exhausted"
					st.UpdatedAt = now
					fmt.Printf("deposit to %s retries exhausted at state %s\n", st.DepositAddr.Hex(), st.State)
					updates = append(updates, st)
					return nil
				}

				next, changed, err := sm.TransitionDeposit(ctx, st)
				if err != nil {
					st.Attempts++
					st.Error = err.Error()
					st.UpdatedAt = now
					fmt.Printf("deposit to %s failed at state %s: %s, %d/%d attempts\n",
						st.DepositAddr.Hex(), st.State, err.Error(), st.Attempts, sm.maxAttempts)
					updates = append(updates, st)
					return nil
				}
				if !changed {
					st.UpdatedAt = now
					updates = append(updates, st)
					return nil
				}

				st.State = next
				st.Attempts = 0
				st.Error = ""
				st.UpdatedAt = now
				fmt.Printf("deposit %s to %s transitioning to state %s\n",
					st.TxHash, st.DepositAddr.Hex(), st.State)
				updates = append(updates, st)
				return nil
			}); err != nil {
				fmt.Printf("scan error: %v\n", err)
				continue
			}

			for _, update := range updates {
				if err := sm.states.Put(ctx, update); err != nil {
					fmt.Printf("put error: %v\n", err)
				}
			}

			fmt.Printf("scanned %d deposits, updated %d\n", depositCount, len(updates))
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

		// NOTE: no minimum deposit amount for testing
		amount := new(big.Int).Set(tx.Value())
		if account != nil {
			fmt.Printf("found deposit to: %s amount: %s\n", to.Hex(), amount.String())
			deposit := &models.DepositState{
				ID:          fmt.Sprintf("%s|%s", account.DepositAddr, tx.Hash().Hex()),
				TxHash:      tx.Hash().Hex(),
				DepositAddr: account.DepositAddr,
				DstAddr:     account.DstAddr,
				DstChain:    account.DstChain,
				SrcChain:    account.SrcChain,
				AmountWei:   amount,
				State:       models.StateSrcTxDiscovered,
				UpdatedAt:   time.Now(),
				CreatedAt:   time.Now(),
			}

			err := sm.states.PutIfAbsent(ctx, deposit)
			if err != nil {
				return err
			}

			fmt.Printf("found deposit for address %s tx %s\n", account.DepositAddr, tx.Hash().Hex())
		}
	}
	return nil
}

func (sm *StateMachine) TransitionDeposit(ctx context.Context, st *models.DepositState) (next models.State, changed bool, err error) {
	switch st.State {

	case models.StateSrcTxDiscovered:
		confirmed, err := sm.wm.WithChain(st.SrcChain).WaitForConfirmations(ctx, st.TxHash, sm.minConfirmations)
		if err != nil {
			if errors.Is(err, ErrorRejectedTransaction) {
				st.State = models.StateFailed
				return st.State, true, nil
			}
			return st.State, false, fmt.Errorf("error waiting for confirmations")
		}
		if !confirmed {
			fmt.Println("waiting for more confirmations")
			return st.State, false, nil
		}
		st.State = models.StateSrcTxConfirmed
		return st.State, true, nil

	case models.StateSrcTxConfirmed, models.StateDstTxResend:
		addr, err := sm.getHotWallet(st.DstChain)
		if err != nil {
			return st.State, false, err
		}
		tx, err := sm.wm.WithChain(st.DstChain).BuildSendTx(ctx, addr, st.DstAddr.Hex(), st.AmountWei)
		if err != nil {
			return st.State, false, fmt.Errorf("error building tx: %v", err)
		}
		st.UnsignedDstTx = tx
		st.State = models.StateDstTxBuilt
		return st.State, true, nil

	case models.StateDstTxBuilt:
		addr, err := sm.getHotWallet(st.DstChain)
		if err != nil {
			return st.State, false, err
		}
		hash, err := sm.wm.WithChain(st.DstChain).BroadcastTx(ctx, st.UnsignedDstTx, addr)
		if err != nil {
			return st.State, false, fmt.Errorf("error sending tx: %v", err)
		}
		st.SentDstTxHash = hash
		st.State = models.StateDstTxSent
		return st.State, true, nil

	case models.StateDstTxSent:
		confirmed, err := sm.wm.WithChain(st.DstChain).WaitForConfirmations(ctx, st.SentDstTxHash, sm.minConfirmations)
		if err != nil {
			if errors.Is(err, ErrorRejectedTransaction) {
				st.State = models.StateDstTxRejected
				return st.State, true, nil
			}
			return st.State, false, fmt.Errorf("error waiting for confirmations")
		}
		if !confirmed {
			return st.State, false, fmt.Errorf("needs more confirmations")
		}
		st.State = models.StateDstTxConfirmed
		return st.State, true, nil

	case models.StateDstTxConfirmed, models.StateSweepTxResend:
		addr, err := sm.getHotWallet(st.SrcChain)
		if err != nil {
			return st.State, false, err
		}
		tx, err := sm.wm.WithChain(st.SrcChain).BuildSweepTx(ctx, st.DepositAddr.Hex(), addr)
		if err != nil {
			return st.State, false, err
		}
		st.UnsignedSweepTx = tx
		st.State = models.StateSweepTxBuilt
		return st.State, true, nil

	case models.StateSweepTxBuilt:
		hash, err := sm.wm.WithChain(st.SrcChain).BroadcastTx(ctx, st.UnsignedSweepTx, st.DepositAddr.Hex())
		if err != nil {
			return st.State, false, fmt.Errorf("error sending tx: %v", err)
		}
		st.SentSweepTxHash = hash
		st.State = models.StateSweepTxSent
		return st.State, true, nil

	case models.StateSweepTxSent:
		confirmed, err := sm.wm.WithChain(st.SrcChain).WaitForConfirmations(ctx, st.SentSweepTxHash, sm.minConfirmations)
		if err != nil {
			if errors.Is(err, ErrorRejectedTransaction) {
				st.State = models.StateSweepTxRejected
				return st.State, true, nil
			}
			return st.State, false, fmt.Errorf("error waiting for confirmations")
		}
		if !confirmed {
			fmt.Println("waiting for more confirmations")
			return st.State, false, nil
		}
		st.State = models.StateSweepTxConfirmed
		return st.State, true, nil

	case models.StateSweepTxConfirmed:
		st.State = models.StateDone
		return st.State, true, nil

	case models.StateDone, models.StateFailed:
		return st.State, false, nil

	case models.StateDstTxRejected:
		st.State = models.StateDstTxResend
		return st.State, true, nil

	case models.StateSweepTxRejected:
		st.State = models.StateSweepTxResend
		return st.State, true, nil

	default:
		return st.State, false, fmt.Errorf("unknown state %s", st.State)
	}
}

func (sm *StateMachine) getHotWallet(chain models.Chain) (string, error) {
	address, ok := sm.hotWallets[chain]
	if !ok {
		return "", fmt.Errorf("no hot wallet for chain %s", chain)
	}

	return address, nil
}
