package hyperliquid

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	apitypes "github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const (
	USDCTestnet = "USDC:0xeb62eee3685fc4c43992febcd9e75443"
)

type TypeProperty struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type EIP712Domain struct {
	Name              string
	Version           string
	ChainID           *big.Int
	VerifyingContract string
}

type TypedData struct {
	Types       map[string][]TypeProperty
	PrimaryType string
	Domain      EIP712Domain
	Message     map[string]interface{}
}

type Signature struct {
	R string `json:"r"`
	S string `json:"s"`
	V byte   `json:"v"`
}

type SpotSendAction struct {
	PrimaryType string `json:"primary_type"`
	Type        string `json:"type"`
	Destination string `json:"destination"`
	Amount      string `json:"amount"`
	Token       string `json:"token"`
}

func SignUserSignedAction(priv *ecdsa.PrivateKey, action map[string]interface{}, payloadTypes []TypeProperty, primaryType string, isMainnet bool) (*Signature, error) {
	action["signatureChainId"] = "0x66eee"
	if isMainnet {
		action["hyperliquidChain"] = "Mainnet"
	} else {
		action["hyperliquidChain"] = "Testnet"
	}
	td, err := UserSignedPayload(primaryType, payloadTypes, action)
	if err != nil {
		return nil, err
	}
	return SignInner(priv, td)
}

func UserSignedPayload(primaryType string, payloadTypes []TypeProperty, action map[string]interface{}) (TypedData, error) {
	hexStr, _ := action["signatureChainId"].(string)
	chainID, err := hexToBig(hexStr)
	if err != nil {
		return TypedData{}, err
	}

	msg := make(map[string]interface{}, len(payloadTypes))
	for _, p := range payloadTypes {
		v, ok := action[p.Name]
		if !ok {
			return TypedData{}, fmt.Errorf("missing required field %q in action", p.Name)
		}
		msg[p.Name] = v
	}

	types := map[string][]TypeProperty{
		primaryType: payloadTypes,
		"EIP712Domain": {
			{Name: "name", Type: "string"},
			{Name: "version", Type: "string"},
			{Name: "chainId", Type: "uint256"},
			{Name: "verifyingContract", Type: "address"},
		},
	}

	return TypedData{
		Types:       types,
		PrimaryType: primaryType,
		Domain: EIP712Domain{
			Name:              "HyperliquidSignTransaction",
			Version:           "1",
			ChainID:           chainID,
			VerifyingContract: "0x0000000000000000000000000000000000000000",
		},
		Message: msg,
	}, nil
}

func SignInner(priv *ecdsa.PrivateKey, td TypedData) (*Signature, error) {
	api := toAPITypedData(td)

	domainSep, err := api.HashStruct("EIP712Domain", api.Domain.Map())
	if err != nil {
		return nil, err
	}
	msgHash, err := api.HashStruct(td.PrimaryType, api.Message)
	if err != nil {
		return nil, err
	}

	raw := make([]byte, 0, 66)
	raw = append(raw, 0x19, 0x01)
	raw = append(raw, domainSep[:]...)
	raw = append(raw, msgHash[:]...)

	digest := crypto.Keccak256Hash(raw)
	sig, err := crypto.Sign(digest.Bytes(), priv)
	if err != nil {
		return nil, err
	}

	return &Signature{
		R: "0x" + hex.EncodeToString(sig[:32]),
		S: "0x" + hex.EncodeToString(sig[32:64]),
		V: sig[64] + 27,
	}, nil
}

func toAPITypedData(t TypedData) apitypes.TypedData {
	apiTypes := make(map[string][]apitypes.Type, len(t.Types))
	for name, props := range t.Types {
		arr := make([]apitypes.Type, 0, len(props))
		for _, p := range props {
			arr = append(arr, apitypes.Type{Name: p.Name, Type: p.Type})
		}
		apiTypes[name] = arr
	}
	return apitypes.TypedData{
		Types:       apiTypes,
		PrimaryType: t.PrimaryType,
		Domain: apitypes.TypedDataDomain{
			Name:              t.Domain.Name,
			Version:           t.Domain.Version,
			ChainId:           (*math.HexOrDecimal256)(t.Domain.ChainID),
			VerifyingContract: t.Domain.VerifyingContract,
		},
		Message: t.Message,
	}
}

func hexToBig(hexStr string) (*big.Int, error) {
	s := strings.TrimPrefix(strings.ToLower(hexStr), "0x")
	if s == "" {
		return big.NewInt(0), nil
	}
	n := new(big.Int)
	if _, ok := n.SetString(s, 16); !ok {
		return nil, fmt.Errorf("invalid hex: %q", hexStr)
	}
	return n, nil
}
