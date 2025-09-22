package models

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestNewAccount_Valid(t *testing.T) {
	src := Chain("ethereum")
	dst := Chain("hyperliquid")

	dstAddrIn := "0x960b650301e941c095aef35f57ae1b2d73fc4df1"
	depAddrIn := "0x6Ae4A873bCD785f28f80285D4B402881649D0f8c"

	acct, err := NewAccount(src, dst, dstAddrIn, depAddrIn)
	if err != nil {
		t.Fatalf("NewAccount error: %v", err)
	}

	wantID := string(src) + ":" + string(dst) + ":" + common.HexToAddress(dstAddrIn).Hex()
	if acct.ID != wantID {
		t.Fatalf("ID = %s, want %s", acct.ID, wantID)
	}
	if acct.SrcChain != src || acct.DstChain != dst {
		t.Fatalf("chains = (%s,%s), want (%s,%s)", acct.SrcChain, acct.DstChain, src, dst)
	}
	if acct.DstAddr != common.HexToAddress(dstAddrIn) {
		t.Fatalf("DstAddr = %s, want %s", acct.DstAddr.Hex(), common.HexToAddress(dstAddrIn).Hex())
	}
	if acct.DepositAddr != common.HexToAddress(depAddrIn) {
		t.Fatalf("DepositAddr = %s, want %s", acct.DepositAddr.Hex(), common.HexToAddress(depAddrIn).Hex())
	}
}

func TestNewAccount_InvalidDstAddr(t *testing.T) {
	_, err := NewAccount(Chain("ethereum"), Chain("hyperliquid"), "invalid", "0x960b650301e941c095aef35f57ae1b2d73fc4df1")
	if err == nil {
		t.Fatal("expected error for invalid destination address")
	}
}

func TestNewAccount_InvalidDepositAddr(t *testing.T) {
	_, err := NewAccount(Chain("ethereum"), Chain("hyperliquid"), "0x960b650301e941c095aef35f57ae1b2d73fc4df1", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid deposit address")
	}
}

func TestAccountID(t *testing.T) {
	src := Chain("ethereum")
	dst := Chain("hyperliquid")
	dstAddr := "0x960b650301e941c095aef35f57ae1b2d73fc4df1"

	got := AccountID(src, dst, dstAddr)
	want := string(src) + ":" + string(dst) + ":" + common.HexToAddress(dstAddr).Hex()

	if got != want {
		t.Fatalf("AccountID = %s, want %s", got, want)
	}
}
