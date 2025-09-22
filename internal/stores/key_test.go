package stores

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func newTestKeyStore(t *testing.T) *LocalKeyStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "keystore")
	ks, err := NewLocalKeyStore("testpass", path)
	if err != nil {
		t.Fatalf("NewLocalKeyStore error: %v", err)
	}
	return ks
}

func TestCreateKeyAndHasKey(t *testing.T) {
	ks := newTestKeyStore(t)
	ctx := context.Background()

	addrHex, err := ks.CreateKey(ctx)
	if err != nil {
		t.Fatalf("CreateKey error: %v", err)
	}
	if addrHex == "" {
		t.Fatal("CreateKey returned empty address")
	}

	if !ks.HasKey(ctx, addrHex) {
		t.Fatalf("HasKey(%s) = false, want true", addrHex)
	}

	if ks.HasKey(ctx, common.HexToAddress("0x000000000000000000000000000000000000dEaD").Hex()) {
		t.Fatal("HasKey returned true for unknown address")
	}
}

func TestSignHash_VerifySignature(t *testing.T) {
	ks := newTestKeyStore(t)
	ctx := context.Background()

	addrHex, err := ks.CreateKey(ctx)
	if err != nil {
		t.Fatalf("CreateKey error: %v", err)
	}
	addr := common.HexToAddress(addrHex)

	hash := crypto.Keccak256([]byte("hello world"))

	sig, err := ks.SignHash(ctx, addrHex, hash)
	if err != nil {
		t.Fatalf("SignHash error: %v", err)
	}
	if len(sig) != 65 {
		t.Fatalf("signature length = %d, want 65", len(sig))
	}

	pubKey, err := crypto.SigToPub(hash, sig)
	if err != nil {
		t.Fatalf("SigToPub error: %v", err)
	}
	recoveredAddr := crypto.PubkeyToAddress(*pubKey)
	if recoveredAddr != addr {
		t.Fatalf("recovered address %s != signer %s", recoveredAddr.Hex(), addr.Hex())
	}
}

func TestSignTx_SetsFromCorrectly(t *testing.T) {
	ks := newTestKeyStore(t)
	ctx := context.Background()

	addrHex, err := ks.CreateKey(ctx)
	if err != nil {
		t.Fatalf("CreateKey error: %v", err)
	}
	from := common.HexToAddress(addrHex)
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")

	nonce := uint64(0)
	value := big.NewInt(12345)
	gasLimit := uint64(21000)
	gasPrice := big.NewInt(1_000_000_000)
	tx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, nil)

	chainID := big.NewInt(1)

	signedTx, err := ks.SignTx(ctx, addrHex, tx, chainID)
	if err != nil {
		t.Fatalf("SignTx error: %v", err)
	}

	signer := types.LatestSignerForChainID(chainID)
	sender, err := types.Sender(signer, signedTx)
	if err != nil {
		t.Fatalf("types.Sender error: %v", err)
	}
	if sender != from {
		t.Fatalf("sender %s != expected %s", sender.Hex(), from.Hex())
	}
}

func TestSignTx_UnknownAddress(t *testing.T) {
	ks := newTestKeyStore(t)
	ctx := context.Background()

	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tx := types.NewTransaction(0, to, big.NewInt(0), 21000, big.NewInt(1), nil)

	if _, err := ks.SignTx(ctx, "0x000000000000000000000000000000000000dEaD", tx, big.NewInt(1)); err == nil {
		t.Fatal("expected error signing with unknown address, got nil")
	}
}

func TestSignHash_UnknownAddress(t *testing.T) {
	ks := newTestKeyStore(t)
	ctx := context.Background()

	hash := make([]byte, 32)
	if _, err := ks.SignHash(ctx, "0x000000000000000000000000000000000000dEaD", hash); err == nil {
		t.Fatal("expected error signing hash with unknown address, got nil")
	}
}

func TestImportECDSA(t *testing.T) {
	ks := newTestKeyStore(t)

	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey error: %v", err)
	}
	wantAddr := crypto.PubkeyToAddress(priv.PublicKey)

	addrHex, err := ks.ImportECDSA((*ecdsa.PrivateKey)(priv), "anotherpass")
	if err != nil {
		t.Fatalf("ImportECDSA error: %v", err)
	}
	if addrHex != wantAddr.Hex() {
		t.Fatalf("imported address %s != expected %s", addrHex, wantAddr.Hex())
	}

	if !ks.HasKey(context.Background(), addrHex) {
		t.Fatalf("HasKey(%s) = false, want true", addrHex)
	}
}
