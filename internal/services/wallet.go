package services

import (
	"context"
	"fmt"
	"math/big"

	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type WalletManager struct {
	ks     stores.KeyStore
	client *ethclient.Client
}

func NewWalletManager(ks stores.KeyStore, client *ethclient.Client) *WalletManager {
	return &WalletManager{
		ks:     ks,
		client: client,
	}
}

func (w *WalletManager) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	err := w.client.SendTransaction(ctx, signedTx)
	if err != nil {
		return err
	}

	return nil
}

// Constructs and signs a transaction to send `amount` native token to `toAddr` from `fromAddr`
func (w *WalletManager) PrepareSendTx(ctx context.Context, fromAddr string, toAddr string, amount *big.Int) (signedTx *types.Transaction, err error) {
	_, err = w.ks.GetAccount(ctx, fromAddr)
	if err != nil {
		return nil, fmt.Errorf("private key not found for %s", fromAddr)
	}

	from := common.HexToAddress(fromAddr)
	to := common.HexToAddress(toAddr)

	nonce, err := w.client.PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}
	balance, err := w.client.BalanceAt(ctx, from, nil)
	if err != nil {
		return nil, err
	}

	gasPrice, gasLimit, err := w.EstimateGas(ctx, from, to)
	if err != nil {
		return nil, err
	}

	gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
	value := new(big.Int).Add(amount, gasCost)

	if balance.Cmp(value) <= 0 {
		return nil, fmt.Errorf("insufficient balance")
	}

	tx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, nil)
	chainID, err := w.client.NetworkID(ctx)
	if err != nil {
		return nil, err
	}

	signed, err := w.ks.SignTx(ctx, fromAddr, tx, chainID)
	if err != nil {
		return nil, err
	}

	return signed, nil
}

// Constructs and signs a transaction to send total balance (minus gas costs) to `toAddr` from `fromAddr`
func (w *WalletManager) PrepareSweepTx(ctx context.Context, fromAddr string, toAddr string) (signedTx *types.Transaction, err error) {
	_, err = w.ks.GetAccount(ctx, fromAddr)
	if err != nil {
		return nil, fmt.Errorf("private key not found for %s", fromAddr)
	}

	from := common.HexToAddress(fromAddr)
	to := common.HexToAddress(toAddr)

	nonce, err := w.client.PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}
	balance, err := w.client.BalanceAt(ctx, from, nil)
	if err != nil {
		return nil, err
	}

	gasPrice, gasLimit, err := w.EstimateGas(ctx, from, to)
	if err != nil {
		return nil, err
	}

	gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
	if balance.Cmp(gasCost) <= 0 {
		return nil, fmt.Errorf("insufficient balance to cover gas")
	}
	value := new(big.Int).Sub(balance, gasCost)

	tx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, nil)
	chainID, err := w.client.NetworkID(ctx)
	if err != nil {
		return nil, err
	}

	signed, err := w.ks.SignTx(ctx, fromAddr, tx, chainID)
	if err != nil {
		return nil, err
	}

	return signed, nil
}

func (w *WalletManager) EstimateGas(ctx context.Context, from common.Address, to common.Address) (gasPrice *big.Int, gasLimit uint64, err error) {
	// NOTE: should use EIP-1559 compatible estimation in production
	gasPrice, err = w.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, 0, err
	}

	// basic ETH send
	gasLimit, err = w.client.EstimateGas(ctx, ethereum.CallMsg{
		From:  from,
		To:    &to,
		Data:  nil,
		Value: big.NewInt(1e18),
	})
	if err != nil {
		return nil, 0, err
	}

	return gasPrice, gasLimit, nil
}
