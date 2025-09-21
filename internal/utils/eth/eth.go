package eth

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

func TxToRawHex(tx *types.Transaction) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("nil tx")
	}
	b, err := tx.MarshalBinary() // RLP encoding with signature fields if present
	if err != nil {
		return "", err
	}
	return hexutil.Encode(b), nil // 0x + hex
}

func RawHexToTx(rawHex string) (*types.Transaction, error) {
	b, err := hexutil.Decode(rawHex)
	if err != nil {
		return nil, err
	}
	var tx types.Transaction
	if err := tx.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return &tx, nil
}
