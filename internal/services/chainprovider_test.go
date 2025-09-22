package services

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unit/agent/internal/clients"
	"unit/agent/internal/mocks"
	"unit/agent/internal/models"
	hlutil "unit/agent/internal/utils/hyperliquid"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func newHLClient(ts *httptest.Server) *clients.HttpClient {
	return &clients.HttpClient{
		BaseURL:    ts.URL,
		HttpClient: ts.Client(),
	}
}

func createPrivateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return k
}

func TestChainProvider_WithChain_ReturnsCorrectCtx(t *testing.T) {
	wm := NewChainProvider(&mocks.MockKeyStore{HasKeyResp: true}, map[models.Chain]*ethclient.Client{
		models.Ethereum: nil,
	}, nil, createPrivateKey(t), &clients.HttpClient{})

	if _, ok := wm.WithChain(models.Hyperliquid).(*HlCtx); !ok {
		t.Fatalf("expected HlCtx for Hyperliquid")
	}
	if _, ok := wm.WithChain(models.Ethereum).(*EvmCtx); !ok {
		t.Fatalf("expected EvmCtx for EVM chain")
	}
}

func TestEvmCtx_BuildSendTx_NoKey(t *testing.T) {
	cp := &ChainProvider{
		ks:      &mocks.MockKeyStore{HasKeyResp: false},
		clients: map[models.Chain]*ethclient.Client{models.Ethereum: nil},
	}
	ctx := &EvmCtx{wm: cp, client: nil}

	_, err := ctx.BuildSendTx(context.Background(),
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		big.NewInt(1))
	if err == nil || !strings.Contains(err.Error(), "private key not found") {
		t.Fatalf("expected private key error, got %v", err)
	}
}

func TestEvmCtx_BroadcastTx_BadRaw(t *testing.T) {
	cp := &ChainProvider{
		ks:      &mocks.MockKeyStore{HasKeyResp: true},
		clients: map[models.Chain]*ethclient.Client{models.Ethereum: nil},
	}
	ctx := &EvmCtx{wm: cp, client: nil}

	_, err := ctx.BroadcastTx(context.Background(), "0xdeadbeef", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil || !strings.Contains(err.Error(), "unmarshaling tx") {
		t.Fatalf("expected unmarshal error, got %v", err)
	}
}

func TestHlCtx_BuildSendTx_JSONShape(t *testing.T) {
	cp := &ChainProvider{}
	h := &HlCtx{
		wm:        cp,
		info:      nil,
		hlPrivKey: createPrivateKey(t),
		hlClient:  &clients.HttpClient{},
	}
	raw, err := h.BuildSendTx(context.Background(),
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"0xABCDabcdABCDabcdABCDabcdABCDabcdABCDabcd",
		// 0.02 ETH => 20.0 USDC
		new(big.Int).Mul(big.NewInt(2), new(big.Int).Exp(big.NewInt(10), big.NewInt(16), nil)))
	if err != nil {
		t.Fatalf("BuildSendTx err: %v", err)
	}

	var got struct {
		PrimaryType string `json:"primary_type"`
		Type        string `json:"type"`
		Destination string `json:"destination"`
		Amount      string `json:"amount"`
		Token       string `json:"token"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.PrimaryType == "" || got.Type != "spotSend" {
		t.Fatalf("bad primary/type: %#v", got)
	}
	if got.Destination != strings.ToLower("0xABCDabcdABCDabcdABCDabcdABCDabcdABCDabcd") {
		t.Fatalf("destination not lowercased: %s", got.Destination)
	}
	if got.Token == "" {
		t.Fatalf("token empty")
	}
	if !strings.Contains(got.Amount, ".") {
		t.Fatalf("amount should be decimal string, got %q", got.Amount)
	}
}

func TestHlCtx_BroadcastTx_SendsToExchangeAndReturnsHash(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Action    map[string]any `json:"action"`
			Nonce     int64          `json:"nonce"`
			Signature struct {
				R string `json:"r"`
				S string `json:"s"`
				V byte   `json:"v"`
			} `json:"signature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if payload.Action["type"] != "spotSend" {
			http.Error(w, "bad type", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"txHash": "0xabc123",
		})
	}))
	defer ts.Close()

	hc := newHLClient(ts)

	priv := createPrivateKey(t)

	h := &HlCtx{
		wm:        &ChainProvider{},
		info:      nil,
		hlPrivKey: priv,
		hlClient:  hc,
	}

	action := hlutil.SpotSendAction{
		PrimaryType: "HyperliquidTransaction:SpotSend",
		Type:        "spotSend",
		Destination: "0x2222222222222222222222222222222222222222",
		Amount:      "1.000000",
		Token:       hlutil.USDCTestnet,
	}
	bs, _ := json.Marshal(action)

	hash, err := h.BroadcastTx(context.Background(), string(bs), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("BroadcastTx err: %v", err)
	}
	if hash != "0xabc123" {
		t.Fatalf("hash = %s, want 0xabc123", hash)
	}
}
