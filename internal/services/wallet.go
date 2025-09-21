package services

import (
	"context"
	"fmt"
	"math/big"

	"unit/agent/internal/models"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type WalletManager struct {
	ks      stores.KeyStore
	clients map[models.Chain]*ethclient.Client
}

type ChainContext struct {
	wm    *WalletManager
	chain models.Chain
}

func NewWalletManager(ks stores.KeyStore, clients map[models.Chain]*ethclient.Client) *WalletManager {
	return &WalletManager{
		ks:      ks,
		clients: clients,
	}
}

func (wm *WalletManager) WithChain(chain models.Chain) *ChainContext {
	return &ChainContext{
		wm:    wm,
		chain: chain,
	}
}

func (c *ChainContext) SendTx(ctx context.Context, signedTx *types.Transaction) (hash string, err error) {
	err = c.wm.clients[c.chain].SendTransaction(ctx, signedTx)
	if err != nil {
		return "", err
	}

	return signedTx.Hash().Hex(), nil
}

func (c *ChainContext) SignTx(ctx context.Context, tx *types.Transaction, fromAddr string) (signedTx *types.Transaction, err error) {
	chainID, err := c.wm.clients[c.chain].NetworkID(ctx)
	if err != nil {
		return nil, err
	}

	signed, err := c.wm.ks.SignTx(ctx, fromAddr, tx, chainID)
	if err != nil {
		return nil, err
	}

	return signed, nil
}

// Builds an unsigned transaction to send `amount` native tokens from `fromAddr` to `toAddr`
func (c *ChainContext) BuildSendTx(ctx context.Context, fromAddr string, toAddr string, amount *big.Int) (tx *types.Transaction, err error) {
	if ok := c.wm.ks.HasKey(ctx, fromAddr); !ok {
		return nil, fmt.Errorf("private key not found for %s", fromAddr)
	}

	from := common.HexToAddress(fromAddr)
	to := common.HexToAddress(toAddr)

	nonce, err := c.wm.clients[c.chain].PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}
	balance, err := c.wm.clients[c.chain].BalanceAt(ctx, from, nil)
	if err != nil {
		return nil, err
	}

	gasPrice, gasLimit, err := c.estimateGas(ctx, from, to)
	if err != nil {
		return nil, err
	}

	gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
	value := new(big.Int).Add(amount, gasCost)

	if balance.Cmp(value) <= 0 {
		return nil, fmt.Errorf("insufficient balance")
	}

	tx = types.NewTransaction(nonce, to, value, gasLimit, gasPrice, nil)
	return tx, nil
}

// Builds an unsigned transaction to send total ETH balance (minus gas costs) from `fromAddr` to `toAddr`
func (c *ChainContext) BuildSweepTx(ctx context.Context, fromAddr string, toAddr string) (tx *types.Transaction, err error) {
	if ok := c.wm.ks.HasKey(ctx, fromAddr); !ok {
		return nil, fmt.Errorf("private key not found for %s", fromAddr)
	}

	from := common.HexToAddress(fromAddr)
	to := common.HexToAddress(toAddr)

	nonce, err := c.wm.clients[c.chain].PendingNonceAt(ctx, from)
	if err != nil {
		return nil, err
	}
	balance, err := c.wm.clients[c.chain].BalanceAt(ctx, from, nil)
	if err != nil {
		return nil, err
	}

	gasPrice, gasLimit, err := c.estimateGas(ctx, from, to)
	if err != nil {
		return nil, err
	}

	gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
	if balance.Cmp(gasCost) <= 0 {
		return nil, fmt.Errorf("insufficient balance to cover gas")
	}
	value := new(big.Int).Sub(balance, gasCost)

	tx = types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &to,
		Value:    value,
		Gas:      gasLimit,
		GasPrice: gasPrice,
		Data:     nil,
	})
	return tx, nil
}

func (c *ChainContext) estimateGas(ctx context.Context, from common.Address, to common.Address) (gasPrice *big.Int, gasLimit uint64, err error) {
	gasPrice, err = c.wm.clients[c.chain].SuggestGasPrice(ctx)
	if err != nil {
		return nil, 0, err
	}

	gasLimit, err = c.wm.clients[c.chain].EstimateGas(ctx, ethereum.CallMsg{
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
