package models

import (
	"fmt"
	"strings"

	"unit/agent/internal/utils/address"
)

type Account struct {
	Chain       string
	DstChain    string
	DstAddr     string
	DepositAddr *string
}

func NewAccount(chain string, dstChain string, dstAddr string) (*Account, error) {
	checksummedDst, err := address.Checksummed(dstAddr)
	if err != nil {
		return nil, err
	}

	return &Account{
		Chain:    strings.ToLower(chain),
		DstChain: strings.ToLower(dstChain),
		DstAddr:  checksummedDst,
	}, nil
}

func (a *Account) ID() string {
	return fmt.Sprintf("%s:%s:%s", a.Chain, a.DstChain, a.DstAddr)
}

func (a *Account) SetDepositAddress(addr string) error {
	checksummed, err := address.Checksummed(addr)
	if err != nil {
		return err
	}

	a.DepositAddr = &checksummed
	return nil
}
