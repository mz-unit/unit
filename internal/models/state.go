package models

import (
	"math/big"
	"time"
)

type State string

const (
	StateDiscovered       State = "DISCOVERED"
	StateDstTxPrepared    State = "DST_TX_PREPARED"
	StateDstTxSent        State = "DST_TX_SENT"
	StateDstTxConfirmed   State = "DST_TX_CONFIRMED"
	StateDstTxRejected    State = "DST_TX_REJECTED"
	StateSweepTxPrepared  State = "SWEEP_TX_PREPARED"
	StateSweepTxSent      State = "SWEEP_TX_SENT"
	StateSweepTxConfirmed State = "SWEEP_TX_CONFIRMED"
	StateSweepTxRejected  State = "SWEEP_TX_REJECTED"
	StateDone             State = "DONE"
	StateFailed           State = "FAILED"
)

type DepositState struct {
	ID              string    `json:"id"` // depositAddr:txHash
	TxHash          string    `json:"tx_hash"`
	DepositAddr     string    `json:"deposit_addr"`
	DstAddr         string    `json:"dst_addr"`
	DstChain        string    `json:"dst_chain"`
	AmountWei       *big.Int  `json:"amount_wei"`
	State           State     `json:"state"`
	UnsignedDstTx   string    `json:"unsigned_dst_tx,omitempty"`
	SentDstTxHash   string    `json:"sent_dst_tx_hash,omitempty"`
	UnsignedSweepTx string    `json:"unsigned_sweep_tx,omitempty"`
	SentSweepTxHash string    `json:"sent_sweep_tx_hash,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
	CreatedAt       time.Time `json:"created_at"`
	Attempts        int       `json:"attempts"`
	Error           string    `json:"error,omitempty"`
}
