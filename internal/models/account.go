package models

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

type Account struct {
	ID          string         `json:"id"`
	SrcChain    Chain          `json:"src_chain"`
	DstChain    Chain          `json:"dst_chain"`
	DstAddr     common.Address `json:"dst_addr"`
	DepositAddr common.Address `json:"deposit_addr"`
}

func NewAccount(srcChain Chain, dstChain Chain, dstAddr string, depositAddr string) (*Account, error) {
	if !common.IsHexAddress(dstAddr) {
		return nil, fmt.Errorf("invalid destination address: %s", dstAddr)
	}

	if !common.IsHexAddress(depositAddr) {
		return nil, fmt.Errorf("invalid deposit address: %s", depositAddr)
	}

	return &Account{
		ID:          fmt.Sprintf("%s:%s:%s", srcChain, dstChain, common.HexToAddress(dstAddr).Hex()),
		SrcChain:    srcChain,
		DstChain:    dstChain,
		DstAddr:     common.HexToAddress(dstAddr),
		DepositAddr: common.HexToAddress(depositAddr),
	}, nil
}

func AccountID(srcChain Chain, dstChain Chain, dstAddr string) string {
	return fmt.Sprintf("%s:%s:%s", srcChain, dstChain, common.HexToAddress(dstAddr).Hex())
}
