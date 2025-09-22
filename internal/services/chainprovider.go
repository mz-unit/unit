package services

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"unit/agent/internal/clients"
	"unit/agent/internal/models"
	"unit/agent/internal/stores"
	hlutil "unit/agent/internal/utils/hyperliquid"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	hyperliquid "github.com/sonirico/go-hyperliquid"
)

var (
	ErrorRejectedTransaction = errors.New("rejected transaction")
)

type IChainProvider interface {
	WithChain(chain models.Chain) ChainCtx
}

type ChainProvider struct {
	ks        stores.IKeyStore
	clients   map[models.Chain]*ethclient.Client
	info      *hyperliquid.Info
	hlPrivKey *ecdsa.PrivateKey
	hlClient  *clients.HttpClient
}

func NewChainProvider(ks stores.IKeyStore, clients map[models.Chain]*ethclient.Client, info *hyperliquid.Info, privKey *ecdsa.PrivateKey, hlClient *clients.HttpClient) *ChainProvider {
	return &ChainProvider{
		ks:        ks,
		clients:   clients,
		info:      info,
		hlPrivKey: privKey,
		hlClient:  hlClient,
	}
}

func (wm *ChainProvider) WithChain(chain models.Chain) ChainCtx {
	if chain == models.Hyperliquid {
		return &HlCtx{
			wm:        wm,
			info:      wm.info,
			hlPrivKey: wm.hlPrivKey,
			hlClient:  wm.hlClient,
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
	// Waits for `minConfirmations` confirmations on `txHash`
	IsTxConfirmed(ctx context.Context, txHash string, minConfirmations uint64) (bool, error)
}

type EvmCtx struct {
	wm     *ChainProvider
	client *ethclient.Client
}

func (c *EvmCtx) BroadcastTx(ctx context.Context, rawTx string, fromAddr string) (hash string, err error) {
	tx := new(types.Transaction)
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

	gasPrice, gasLimit, err := c.estimateGas(ctx, from, to, amount)
	if err != nil {
		return "", err
	}

	gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(gasLimit))
	total := new(big.Int).Add(amount, gasCost)
	if balance.Cmp(total) < 0 {
		return "", fmt.Errorf("insufficient balance: have %s, need %s",
			balance.String(), total.String())
	}

	tx := types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil)
	rawTxBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("error marshaling tx: %v", err)
	}

	return common.Bytes2Hex(rawTxBytes), nil
}

func (c *EvmCtx) BuildSweepTx(ctx context.Context, fromAddr, toAddr string) (string, error) {
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

	gasPrice, err := c.client.SuggestGasPrice(ctx)
	if err != nil {
		return "", err
	}

	const ethTransferGas = uint64(21000)
	gasCost := new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(ethTransferGas))
	if balance.Cmp(gasCost) <= 0 {
		return "", fmt.Errorf("insufficient balance: have %s need %s", balance, gasCost)
	}
	value := new(big.Int).Sub(balance, gasCost)

	tx := types.NewTransaction(nonce, to, value, ethTransferGas, gasPrice, nil)
	raw, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("marshal tx: %w", err)
	}
	return common.Bytes2Hex(raw), nil
}

func (c *EvmCtx) IsTxConfirmed(ctx context.Context, txHash string, minConfirmations uint64) (bool, error) {
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

func (c *EvmCtx) estimateGas(ctx context.Context, from, to common.Address, value *big.Int) (gasPrice *big.Int, gasLimit uint64, err error) {
	gasPrice, err = c.client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, 0, err
	}
	gasLimit, err = c.client.EstimateGas(ctx, ethereum.CallMsg{
		From:  from,
		To:    &to,
		Data:  nil,
		Value: value,
	})
	if err != nil {
		return nil, 0, err
	}
	return gasPrice, gasLimit, nil
}

type HlCtx struct {
	wm        *ChainProvider
	info      *hyperliquid.Info
	hlPrivKey *ecdsa.PrivateKey
	hlClient  *clients.HttpClient
}

func (c *HlCtx) BroadcastTx(ctx context.Context, rawTx string, fromAddr string) (hash string, err error) {
	var action hlutil.SpotSendAction
	if err := json.Unmarshal([]byte(rawTx), &action); err != nil {
		return "", fmt.Errorf("unmarshalling spot send action %v", err)
	}

	nonce := time.Now().UnixMilli()

	payloadTypes := []hlutil.TypeProperty{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "destination", Type: "string"},
		{Name: "token", Type: "string"},
		{Name: "amount", Type: "string"},
		{Name: "time", Type: "uint64"},
	}

	actionPayload := map[string]interface{}{
		"type":        action.Type,
		"destination": strings.ToLower(action.Destination), // must be lowercased https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/signing
		"token":       action.Token,
		"amount":      action.Amount,
		"time":        new(big.Int).SetUint64(uint64(nonce)),
	}

	sig, err := hlutil.SignUserSignedAction(c.hlPrivKey, actionPayload, payloadTypes, action.PrimaryType, false /* isMainnet */)
	if err != nil {
		return "", fmt.Errorf("SignUserSignedAction: %v", err)
	}

	payload := map[string]any{
		"action":    actionPayload,
		"nonce":     nonce,
		"signature": *sig,
	}

	resp, err := c.hlClient.Post(ctx, "/exchange", payload)

	if err != nil {
		return "", err
	}

	var result clients.TransferResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}
	if result.Status != "ok" {
		return "", fmt.Errorf("api response error %v", result)
	}

	return result.TxHash, nil
}

func (c *HlCtx) BuildSendTx(ctx context.Context, fromAddr string, toAddr string, amount *big.Int) (rawTx string, err error) {
	// hardcoded conversion of eth wei to usdc amount for POC. Assume 0.01 ETH = 10 USDC for testing
	divisor := new(big.Int)
	divisor.SetString("1000000000000000000", 10) // 18 decimals
	ethValue := new(big.Float).SetInt(amount)
	ethValue.Quo(ethValue, new(big.Float).SetInt(divisor))
	usdcValue := new(big.Float).Mul(ethValue, big.NewFloat(1000))

	action := hlutil.SpotSendAction{
		PrimaryType: "HyperliquidTransaction:SpotSend",
		Type:        "spotSend",
		Destination: strings.ToLower(toAddr),
		Amount:      fmt.Sprintf("%.6f", usdcValue),
		Token:       hlutil.USDCTestnet,
	}
	bytes, err := json.Marshal(action)
	if err != nil {
		return "", fmt.Errorf("marshalling USD transfer action %v", err)
	}
	return string(bytes), nil
}

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

	action := hlutil.SpotSendAction{
		PrimaryType: "HyperliquidTransaction:SpotSend",
		Type:        "spotSend",
		Destination: toAddr,
		Amount:      usdcBalance,
		Token:       hlutil.USDCTestnet,
	}
	bytes, err := json.Marshal(action)
	if err != nil {
		return "", fmt.Errorf("marshalling spot send action %v", err)
	}
	return string(bytes), nil
}

func (c *HlCtx) IsTxConfirmed(ctx context.Context, txHash string, minConfirmations uint64) (bool, error) {
	// hyperliquid core has one block finality with block times of 200ms.
	// for this POC effectively consider transfer finalized, for correctess we need a way to get core's block number
	return true, nil
}
