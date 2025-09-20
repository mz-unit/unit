package models

import (
	"github.com/ethereum/go-ethereum/core/types"
)

type Block struct {
	Number   uint64
	Hash     string
	Block    *types.Block
	Receipts []*types.Receipt
}
