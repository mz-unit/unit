package models

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type State string

const (
	StateSrcTxDiscovered  State = "SRC_TX_DISCOVERED"
	StateSrcTxConfirmed   State = "SRC_TX_CONFIRMED"
	StateDstTxBuilt       State = "DST_TX_BUILT"
	StateDstTxSent        State = "DST_TX_SENT"
	StateDstTxConfirmed   State = "DST_TX_CONFIRMED"
	StateDstTxRejected    State = "DST_TX_REJECTED"
	StateSweepTxBuilt     State = "SWEEP_TX_BUILT"
	StateSweepTxSent      State = "SWEEP_TX_SENT"
	StateSweepTxConfirmed State = "SWEEP_TX_CONFIRMED"
	StateSweepTxRejected  State = "SWEEP_TX_REJECTED"
	StateDone             State = "DONE"
	StateFailed           State = "FAILED"
	StateDstTxResend      State = "DST_TX_RESEND"
	StateSweepTxResend    State = "SWEEP_TX_RESEND"
)

type DepositState struct {
	ID              string         `json:"id"` // depositAddr:txHash
	TxHash          string         `json:"tx_hash"`
	DepositAddr     common.Address `json:"deposit_addr"`
	DstAddr         common.Address `json:"dst_addr"`
	DstChain        Chain          `json:"dst_chain"`
	SrcChain        Chain          `json:"src_chain"`
	Asset           string         `json:"asset"`
	AmountWei       *big.Int       `json:"amount_wei"`
	State           State          `json:"state"`
	UnsignedDstTx   string         `json:"unsigned_dst_tx"`
	SentDstTxHash   string         `json:"sent_dst_tx_hash"`
	UnsignedSweepTx string         `json:"unsigned_sweep_tx"`
	SentSweepTxHash string         `json:"sent_sweep_tx_hash"`
	UpdatedAt       time.Time      `json:"updated_at"`
	CreatedAt       time.Time      `json:"created_at"`
	Attempts        int            `json:"attempts"`
	Error           string         `json:"error"`
}
