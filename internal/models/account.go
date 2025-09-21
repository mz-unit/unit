package models

import (
	"fmt"
	"strings"

	"unit/agent/internal/utils/address"
)

type Account struct {
	ID          string  `json:"id"`
	Chain       string  `json:"chain"`
	DstChain    string  `json:"dst_chain"`
	DstAddr     string  `json:"dst_addr"`
	DepositAddr *string `json:"deposit_addr"`
}

func NewAccount(chain string, dstChain string, dstAddr string) (*Account, error) {
	checksummedDst, err := address.Checksummed(dstAddr)
	if err != nil {
		return nil, err
	}

	c := strings.ToLower(chain)
	dc := strings.ToLower(dstChain)

	return &Account{
		ID:       fmt.Sprintf("%s:%s:%s", c, dc, checksummedDst),
		Chain:    c,
		DstChain: dc,
		DstAddr:  checksummedDst,
	}, nil
}

func (a *Account) SetDepositAddress(addr string) error {
	checksummed, err := address.Checksummed(addr)
	if err != nil {
		return err
	}

	a.DepositAddr = &checksummed
	return nil
}
