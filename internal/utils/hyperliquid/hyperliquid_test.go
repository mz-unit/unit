package hyperliquid

import (
	"crypto/ecdsa"
	"encoding/hex"
	"strings"
	"testing"

	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
)

func createKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}

func buildDigest(t *testing.T, td TypedData) (digest [32]byte) {
	t.Helper()
	api := toAPITypedData(td)
	ds, err := api.HashStruct("EIP712Domain", api.Domain.Map())
	if err != nil {
		t.Fatalf("domain hash: %v", err)
	}
	mh, err := api.HashStruct(td.PrimaryType, api.Message)
	if err != nil {
		t.Fatalf("message hash: %v", err)
	}
	raw := append([]byte{0x19, 0x01}, append(ds[:], mh[:]...)...)
	return crypto.Keccak256Hash(raw)
}

func TestHexToBig(t *testing.T) {
	n, err := hexToBig("0x66eee")
	if err != nil {
		t.Fatalf("hexToBig odd len: %v", err)
	}
	if n.Cmp(big.NewInt(0)) <= 0 {
		t.Fatalf("expected positive value, got %s", n)
	}

	n2, err := hexToBig("0x01")
	if err != nil {
		t.Fatalf("hexToBig even len: %v", err)
	}
	if n2.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("expected 1, got %s", n2)
	}

	if _, err := hexToBig("0xZZZ"); err == nil {
		t.Fatalf("expected error for invalid hex")
	}
}

func TestUserSignedPayload_FiltersFieldsAndErrorsOnMissing(t *testing.T) {
	primary := "HyperliquidTransaction:SpotSend"
	types := []TypeProperty{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "destination", Type: "string"},
		{Name: "token", Type: "string"},
		{Name: "amount", Type: "string"},
		{Name: "time", Type: "uint256"},
	}

	action := map[string]interface{}{
		"type":             "spotSend",
		"signatureChainId": "0x66eee",
		"hyperliquidChain": "Testnet",
		"destination":      strings.ToLower("0x1111111111111111111111111111111111111111"),
		"token":            strings.ToLower(USDCTestnet),
		"amount":           "10",
		"time":             new(big.Int).SetUint64(1716531066415),
	}

	td, err := UserSignedPayload(primary, types, action)
	if err != nil {
		t.Fatalf("UserSignedPayload: %v", err)
	}

	if len(td.Message) != 5 {
		t.Fatalf("expected 5 message fields, got %d: %#v", len(td.Message), td.Message)
	}
	for _, k := range []string{"hyperliquidChain", "destination", "token", "amount", "time"} {
		if _, ok := td.Message[k]; !ok {
			t.Fatalf("missing required field %q", k)
		}
	}
	if _, ok := td.Message["type"]; ok {
		t.Fatalf("unexpected field 'type' present in message")
	}
	if _, ok := td.Message["signatureChainId"]; ok {
		t.Fatalf("unexpected field 'signatureChainId' present in message")
	}

	if td.Domain.ChainID == nil || td.Domain.ChainID.Sign() == 0 {
		t.Fatalf("expected non-zero domain chain id")
	}

	bad := map[string]interface{}{
		"signatureChainId": "0x66eee",
		"hyperliquidChain": "Testnet",
		"destination":      strings.ToLower("0xaaa"),
		"token":            "usdc:0xbb",
		"time":             new(big.Int).SetUint64(1),
	}
	if _, err := UserSignedPayload(primary, types, bad); err == nil {
		t.Fatalf("expected error when a required field is missing")
	}
}

func TestSignInner_RecoversCorrectAddress(t *testing.T) {
	priv := createKey(t)
	addr := crypto.PubkeyToAddress(priv.PublicKey)

	primary := "HyperliquidTransaction:SpotSend"
	types := []TypeProperty{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "destination", Type: "string"},
		{Name: "token", Type: "string"},
		{Name: "amount", Type: "string"},
		{Name: "time", Type: "uint256"},
	}

	action := map[string]interface{}{
		"signatureChainId": "0x66eee",
		"hyperliquidChain": "Testnet",
		"destination":      "0x1111111111111111111111111111111111111111",
		"token":            USDCTestnet,
		"amount":           "10",
		"time":             new(big.Int).SetUint64(1716531066415),
	}

	td, err := UserSignedPayload(primary, types, action)
	if err != nil {
		t.Fatalf("UserSignedPayload: %v", err)
	}

	sig, err := SignInner(priv, td)
	if err != nil {
		t.Fatalf("SignInner: %v", err)
	}

	digest := buildDigest(t, td)

	sigBytes := make([]byte, 65)
	rb, _ := hex.DecodeString(strings.TrimPrefix(sig.R, "0x"))
	sb, _ := hex.DecodeString(strings.TrimPrefix(sig.S, "0x"))
	copy(sigBytes[0:32], rb)
	copy(sigBytes[32:64], sb)
	if sig.V != 27 && sig.V != 28 {
		t.Fatalf("unexpected V: %d", sig.V)
	}
	sigBytes[64] = sig.V - 27

	pub, err := crypto.SigToPub(digest[:], sigBytes)
	if err != nil {
		t.Fatalf("SigToPub: %v", err)
	}
	recovered := crypto.PubkeyToAddress(*pub)
	if recovered != addr {
		t.Fatalf("recovered %s != expected %s", recovered.Hex(), addr.Hex())
	}
}

func TestSignUserSignedAction_SetsFieldsAndSigns(t *testing.T) {
	priv := createKey(t)

	payloadTypes := []TypeProperty{
		{Name: "hyperliquidChain", Type: "string"},
		{Name: "destination", Type: "string"},
		{Name: "token", Type: "string"},
		{Name: "amount", Type: "string"},
		{Name: "time", Type: "uint64"},
	}

	action := map[string]interface{}{
		"type":        "spotSend",
		"destination": strings.ToLower("0x1111111111111111111111111111111111111111"),
		"token":       USDCTestnet,
		"amount":      "1.5",
		"time":        new(big.Int).SetUint64(1716531066415),
	}

	sig, err := SignUserSignedAction(priv, action, payloadTypes, "HyperliquidTransaction:SpotSend", false)
	if err != nil {
		t.Fatalf("SignUserSignedAction: %v", err)
	}
	if sig.R == "" || sig.S == "" || (sig.V != 27 && sig.V != 28) {
		t.Fatalf("bad signature: %#v", sig)
	}
	if action["hyperliquidChain"] != "Testnet" {
		t.Fatalf("expected hyperliquidChain=Testnet, got %v", action["hyperliquidChain"])
	}
	if action["signatureChainId"] != "0x66eee" {
		t.Fatalf("expected signatureChainId=0x66eee, got %v", action["signatureChainId"])
	}
}

func TestToAPITypedData_Marshals(t *testing.T) {
	td := TypedData{
		Types: map[string][]TypeProperty{
			"X": {
				{Name: "a", Type: "string"},
			},
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
		},
		PrimaryType: "X",
		Domain: EIP712Domain{
			Name:              "HyperliquidSignTransaction",
			Version:           "1",
			ChainID:           big.NewInt(1),
			VerifyingContract: "0x0000000000000000000000000000000000000000",
		},
		Message: map[string]interface{}{"a": "b"},
	}
	api := toAPITypedData(td)
	if _, err := api.HashStruct("EIP712Domain", api.Domain.Map()); err != nil {
		t.Fatalf("domain hash: %v", err)
	}
	if _, err := api.HashStruct(td.PrimaryType, api.Message); err != nil {
		t.Fatalf("message hash: %v", err)
	}
}
