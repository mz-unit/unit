package models

import (
	"fmt"
	"math/big"
)

type Deposit struct {
	Chain           string
	BlockNumber     uint64
	BlockHash       string
	TransactionHash string
	DstAddress      string
	SrcAddress      string
	AssetAddress    string
	Amount          big.Int
	Decimals        uint64
}

func (d *Deposit) ID() string {
	return fmt.Sprintf("%s:%s", d.TransactionHash, d.DstAddress)
}
