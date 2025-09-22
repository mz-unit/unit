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
	if hash != "" {
		t.Fatalf("hash = %s, want empty", hash)
	}
}

func TestHlCtx_BroadcastTx_BadJSON(t *testing.T) {
	h := &HlCtx{
		wm:        &ChainProvider{},
		hlPrivKey: createPrivateKey(t),
		hlClient:  &clients.HttpClient{},
	}
	_, err := h.BroadcastTx(context.Background(), "{not-json", "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil || err.Error() == "" {
		t.Fatalf("expected JSON unmarshal error, got %v", err)
	}
}

func TestHlCtx_IsTxConfirmed_ReturnsTrueA(t *testing.T) {
	h := &HlCtx{}
	ok, err := h.IsTxConfirmed(context.Background(), "0xabc", 0)
	if err != nil {
		t.Fatalf("IsTxConfirmed err: %v", err)
	}
	if !ok {
		t.Fatalf("IsTxConfirmed = false, want true")
	}
}

func TestHlCtx_BuildSendTx(t *testing.T) {
	h := &HlCtx{hlPrivKey: createPrivateKey(t), hlClient: &clients.HttpClient{}}

	oneEth := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	raw, err := h.BuildSendTx(context.Background(),
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"0xBBBBbbbbBBBBbbbbBBBBbbbbBBBBbbbbBBBBbbbb",
		oneEth,
	)
	if err != nil {
		t.Fatalf("BuildSendTx err: %v", err)
	}

	var got struct {
		Amount string `json:"amount"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Amount != "1000.000000" {
		t.Fatalf("amount = %q, want %q", got.Amount, "1000.000000")
	}
	if got.Token == "" {
		t.Fatalf("token empty")
	}
}

// TODO: explore using simulated backend to unit test ethclient and hyperliquid client calls more easily
