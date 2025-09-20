package models

import (
	"math/big"
	"github.com/ethereum/go-ethereum/core/types"
)

type Deposit struct {
	Chain       	string
	BlockNumber 	uint64
	BlockHash   	string
	TransactionHash string
	DstAddress  	string
	SrcAddress  	string
	AssetAddress    string
	Amount      	big.Int
	Decimals    	uint64
}
