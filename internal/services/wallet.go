package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"unit/agent/internal/models"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	hyperliquid "github.com/sonirico/go-hyperliquid"
)

var (
	ErrorRejectedTransaction = errors.New("rejected transaction")
)

type WalletManager struct {
	ks       stores.KeyStore
	clients  map[models.Chain]*ethclient.Client
	exchange *hyperliquid.Exchange
	info     *hyperliquid.Info
}

func NewWalletManager(ks stores.KeyStore, clients map[models.Chain]*ethclient.Client, exchange *hyperliquid.Exchange, info *hyperliquid.Info) *WalletManager {
	return &WalletManager{
		ks:       ks,
		clients:  clients,
		exchange: exchange,
		info:     info,
	}
}

func (wm *WalletManager) WithChain(chain models.Chain) ChainCtx {
	if chain == models.Hyperliquid {
		return &HlCtx{
			wm:       wm,
			exchange: wm.exchange,
			info:     wm.info,
		}
	}
	return &EvmCtx{
		wm:     wm,
		client: wm.clients[chain],
	}
}

type ChainCtx interface {
	// Signs and broadcasts transaction. For a system with consensus layer, break these 2 steps up.
	BroadcastTx(ctx context.Context, rawTx string, fromAddr string) (hash string, err error)
	// Builds an unsigned transaction to send `amount` from `fromAddr` to `toAddr`. Used to credit a deposit on destination chain
	BuildSendTx(ctx context.Context, fromAddr string, toAddr string, amount *big.Int) (rawTx string, err error)
	// Builds an unsigned transaction to send total balance (minus gas costs) from `fromAddr` to `toAddr`. Used to sweep from deposit addresses
	BuildSweepTx(ctx context.Context, fromAddr string, toAddr string) (rawTx string, err error)
	// Waits for `minConfirmations` confirmations for transaction `txHash`
	WaitForConfirmations(ctx context.Context, txHash string, minConfirmations uint64) (bool, error)
}

type EvmCtx struct {
	wm     *WalletManager
	client *ethclient.Client
}

func (c *EvmCtx) BroadcastTx(ctx context.Context, rawTx string, fromAddr string) (hash string, err error) {
	var tx *types.Transaction
	if err := tx.UnmarshalBinary(common.Hex2Bytes(rawTx)); err != nil {
		return "", fmt.Errorf("error unmarshaling tx: %v", err)
	}

	chainID, err := c.client.NetworkID(ctx)
	if err != nil {
		return "", fmt.Errorf("NetworkID: %v", err)
	}

	signed, err := c.wm.ks.SignTx(ctx, fromAddr, tx, chainID)
	if err != nil {
		return "", fmt.Errorf("SignTx: %v", err)
	}

	err = c.client.SendTransaction(ctx, signed)
	if err != nil {
		return "", fmt.Errorf("SendTransaction: %v", err)
	}

	return signed.Hash().Hex(), nil
}

func (c *EvmCtx) BuildSendTx(ctx context.Context, fromAddr string, toAddr string, amount *big.Int) (rawTx string, err error) {
	if ok := c.wm.ks.HasKey(ctx, fromAddr); !ok {
		return "", fmt.Errorf("private key not found for %s", fromAddr)
	}

	from := common.HexToAddress(fromAddr)
	to := common.HexToAddress(toAddr)

	nonce, err := c.client.PendingNonceAt(ctx, from)
	if err != nil {
		return "", err
	}
	balance, err := c.client.BalanceAt(ctx, from, nil)
	if err != nil {
		return "", err
	}

	gasPrice, gasLimit, err := c.estimateGas(ctx, from, to)
	if err != nil {
		return "", err
	}

	gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
	value := new(big.Int).Add(amount, gasCost)

	if balance.Cmp(value) <= 0 {
		return "", fmt.Errorf("insufficient balance")
	}

	tx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, nil)
	rawTxBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("error marshaling tx: %v", err)
	}

	return common.Bytes2Hex(rawTxBytes), nil
}

func (c *EvmCtx) BuildSweepTx(ctx context.Context, fromAddr string, toAddr string) (rawTx string, err error) {
	if ok := c.wm.ks.HasKey(ctx, fromAddr); !ok {
		return "", fmt.Errorf("private key not found for %s", fromAddr)
	}

	from := common.HexToAddress(fromAddr)
	to := common.HexToAddress(toAddr)

	nonce, err := c.client.PendingNonceAt(ctx, from)
	if err != nil {
		return "", err
	}
	balance, err := c.client.BalanceAt(ctx, from, nil)
	if err != nil {
		return "", err
	}

	gasPrice, gasLimit, err := c.estimateGas(ctx, from, to)
	if err != nil {
		return "", err
	}

	gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
	if balance.Cmp(gasCost) <= 0 {
		return "", fmt.Errorf("insufficient balance to cover gas, balance %s gasCost %s", balance.String(), gasCost.String())
	}
	value := new(big.Int).Sub(balance, gasCost)

	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &to,
		Value:    value,
		Gas:      gasLimit,
		GasPrice: gasPrice,
		Data:     nil,
	})

	rawTxBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("error marshaling tx: %v", err)
	}

	return common.Bytes2Hex(rawTxBytes), nil
}

func (c *EvmCtx) WaitForConfirmations(ctx context.Context, txHash string, minConfirmations uint64) (bool, error) {
	rcpt, err := c.client.TransactionReceipt(ctx, common.HexToHash(txHash))
	if err != nil {
		return false, fmt.Errorf("error getting receipt: %v", err)
	}
	if rcpt.Status != types.ReceiptStatusSuccessful {
		return false, ErrorRejectedTransaction
	}
	head, err := c.client.BlockNumber(ctx)
	if err != nil {
		return false, fmt.Errorf("error getting latest block number: %v", err)
	}
	if head < rcpt.BlockNumber.Uint64()+minConfirmations {
		return false, nil
	}

	return true, nil
}

func (c *EvmCtx) estimateGas(ctx context.Context, from common.Address, to common.Address) (gasPrice *big.Int, gasLimit uint64, err error) {
	gasPrice, err = c.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, 0, err
	}

	gasLimit, err = c.client.EstimateGas(ctx, ethereum.CallMsg{
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

type HlCtx struct {
	wm       *WalletManager
	exchange *hyperliquid.Exchange
	info     *hyperliquid.Info
}

func (c *HlCtx) BroadcastTx(ctx context.Context, rawTx string, fromAddr string) (hash string, err error) {
	var action hyperliquid.SpotTransferAction
	if err := json.Unmarshal([]byte(rawTx), &action); err != nil {
		return "", fmt.Errorf("unmarshalling USD transfer action %v", err)
	}

	f, err := strconv.ParseFloat(action.Amount, 64)
	if err != nil {
		return "", fmt.Errorf("invalid send amount %s", action.Amount)
	}

	// signs and posts payload
	// response, err := c.exchange.UsdTransfer(ctx, f, action.Destination)
	response, err := c.exchange.SpotTransfer(ctx, f, action.Destination, action.Token)
	if err != nil {
		return "", fmt.Errorf("SpotTransfer: %v", err)
	}

	return response.TxHash, nil
}

func (c *HlCtx) BuildSendTx(ctx context.Context, fromAddr string, toAddr string, amount *big.Int) (rawTx string, err error) {
	// hardcoded conversion of eth wei to usdc amount for POC. Assume 0.01 ETH = 10 USDC for testing
	divisor := new(big.Int)
	divisor.SetString("1000000000000000000", 10) // 18 decimals
	ethValue := new(big.Float).SetInt(amount)
	ethValue.Quo(ethValue, new(big.Float).SetInt(divisor))
	usdcValue := new(big.Float).Mul(ethValue, big.NewFloat(1000))

	action := hyperliquid.SpotTransferAction{
		Type:        "usdSend",
		Destination: toAddr,
		Amount:      fmt.Sprintf("%.6f", usdcValue),
		Token:       "USDC:0xeb62eee3685fc4c43992febcd9e75443", // USDC testnet token id
	}
	bytes, err := json.Marshal(action)
	if err != nil {
		return "", fmt.Errorf("marshalling USD transfer action %v", err)
	}
	return string(bytes), nil
}

// non functional for withdrawals currently as SDK's exchange instance takes in an account address. The exchange instance in HlCtx is the hot wallet instance.
// would need to create a new instance of hyperliquid.Exchange with deposit address (`fromAddr`)
func (c *HlCtx) BuildSweepTx(ctx context.Context, fromAddr string, toAddr string) (rawTx string, err error) {
	response, err := c.info.SpotUserState(ctx, fromAddr)
	if err != nil {
		return "", fmt.Errorf("error fetching balances %v", err)
	}

	var usdcBalance string = "0.0"
	for i := 0; i < len(response.Balances); i++ {
		if response.Balances[i].Coin == "USDC" {
			usdcBalance = response.Balances[i].Total
		}
	}

	if usdcBalance == "0.0" {
		return "", fmt.Errorf("zero balance for address %s", fromAddr)
	}

	action := hyperliquid.UsdTransferAction{
		Type:        "usdSend",
		Destination: toAddr,
		Amount:      usdcBalance,
	}
	bytes, err := json.Marshal(action)
	if err != nil {
		return "", fmt.Errorf("marshalling USD transfer action %v", err)
	}
	return string(bytes), nil
}

func (c *HlCtx) WaitForConfirmations(ctx context.Context, txHash string, minConfirmations uint64) (bool, error) {
	// hyperliquid core has one block finality with block times of 200ms.
	// for this POC effectively consider transfer finalized after 200ms, for correctess we need a way to get core's block number
	time.Sleep(200 * time.Millisecond)
	return true, nil
}
