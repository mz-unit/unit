package models

import (
	"fmt"

	"unit/agent/internal/utils/address"
)

type Account struct {
	ID          string `json:"id"`
	SrcChain    Chain  `json:"src_chain"`
	DstChain    Chain  `json:"dst_chain"`
	DstAddr     string `json:"dst_addr"`
	DepositAddr string `json:"deposit_addr"`
}

func NewAccount(srcChain Chain, dstChain Chain, dstAddr string, depositAddr string) (*Account, error) {
	checksummedDst, err := address.Checksummed(dstAddr)
	if err != nil {
		return nil, err
	}

	checksummedDpst, err := address.Checksummed(depositAddr)
	if err != nil {
		return nil, err
	}

	return &Account{
		ID:          fmt.Sprintf("%s:%s:%s", srcChain, dstChain, checksummedDst),
		SrcChain:    srcChain,
		DstChain:    dstChain,
		DstAddr:     checksummedDst,
		DepositAddr: checksummedDpst,
	}, nil
}

func AccountID(srcChain string, dstChain string, dstAddr string) (string, error) {
	checksummedDst, err := address.Checksummed(dstAddr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s:%s", srcChain, dstChain, checksummedDst), nil
}
