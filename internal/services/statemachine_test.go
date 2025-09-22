package services

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"unit/agent/internal/mocks"
	"unit/agent/internal/models"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type mockChainCtx struct {
	buildSendTxFn   func(ctx context.Context, from, to string, amount *big.Int) (string, error)
	buildSweepTxFn  func(ctx context.Context, from, to string) (string, error)
	broadcastTxFn   func(ctx context.Context, rawTx, fromAddr string) (string, error)
	isTxConfirmedFn func(ctx context.Context, txHash string, minConf uint64) (bool, error)
}

func (m *mockChainCtx) BroadcastTx(ctx context.Context, rawTx string, fromAddr string) (string, error) {
	return m.broadcastTxFn(ctx, rawTx, fromAddr)
}
func (m *mockChainCtx) BuildSendTx(ctx context.Context, fromAddr string, toAddr string, amount *big.Int) (string, error) {
	return m.buildSendTxFn(ctx, fromAddr, toAddr, amount)
}
func (m *mockChainCtx) BuildSweepTx(ctx context.Context, fromAddr string, toAddr string) (string, error) {
	return m.buildSweepTxFn(ctx, fromAddr, toAddr)
}
func (m *mockChainCtx) IsTxConfirmed(ctx context.Context, txHash string, minConfirmations uint64) (bool, error) {
	return m.isTxConfirmedFn(ctx, txHash, minConfirmations)
}

type mockChainProvider struct {
	byChain map[models.Chain]*mockChainCtx
}

func (m *mockChainProvider) WithChain(chain models.Chain) ChainCtx {
	return m.byChain[chain]
}

type mockStateStore struct {
	items   map[string]*models.DepositState
	putCh   chan *models.DepositState
	putIfCh chan *models.DepositState
}

func newMockStateStore() *mockStateStore {
	return &mockStateStore{
		items:   make(map[string]*models.DepositState),
		putCh:   make(chan *models.DepositState, 10),
		putIfCh: make(chan *models.DepositState, 10),
	}
}

func (f *mockStateStore) PutIfAbsent(ctx context.Context, state *models.DepositState) error {
	if _, ok := f.items[state.ID]; !ok {
		cp := *state
		f.items[state.ID] = &cp
	}
	select {
	case f.putIfCh <- state:
	default:
	}
	return nil
}

func (f *mockStateStore) Put(ctx context.Context, state *models.DepositState) error {
	cp := *state
	f.items[state.ID] = &cp
	select {
	case f.putCh <- &cp:
	default:
	}
	return nil
}

func (f *mockStateStore) Get(ctx context.Context, id string) (*models.DepositState, error) {
	if v, ok := f.items[id]; ok {
		cp := *v
		return &cp, nil
	}
	return nil, stores.ErrExecutionNotFound
}

func (f *mockStateStore) Scan(ctx context.Context, visit func(*models.DepositState) error) error {
	for _, v := range f.items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		cp := *v
		if err := visit(&cp); err != nil {
			return err
		}
	}
	return nil
}

func (f *mockStateStore) Close() error { return nil }

type mockTrieHasher struct{}

func (*mockTrieHasher) Hash() common.Hash           { return common.Hash{} }
func (*mockTrieHasher) Reset()                      { return }
func (*mockTrieHasher) Update([]byte, []byte) error { return nil }

func newStateMachineForTest(t *testing.T, provider *mockChainProvider) *StateMachine {
	t.Helper()
	hot := map[models.Chain]string{
		models.Ethereum:    "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		models.Hyperliquid: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	sm, err := NewStateMachine((*ChainProvider)(nil), nil, nil, hot)
	if err != nil {
		t.Fatalf("NewStateMachine: %v", err)
	}
	sm.provider = provider
	sm.interval = 1 * time.Millisecond
	sm.minConfirmations = 1
	return sm
}

func TestTransitionDeposit_Success(t *testing.T) {
	srcCtx := &mockChainCtx{
		isTxConfirmedFn: func(ctx context.Context, txHash string, min uint64) (bool, error) { return true, nil },
		buildSweepTxFn:  func(ctx context.Context, from, to string) (string, error) { return "raw_sweep", nil },
		broadcastTxFn:   func(ctx context.Context, raw, from string) (string, error) { return "0xsweephash", nil },
	}
	dstCtx := &mockChainCtx{
		isTxConfirmedFn: func(ctx context.Context, txHash string, min uint64) (bool, error) { return true, nil },
		buildSendTxFn:   func(ctx context.Context, from, to string, amount *big.Int) (string, error) { return "raw_dst", nil },
		broadcastTxFn:   func(ctx context.Context, raw, from string) (string, error) { return "0xdsthash", nil },
	}
	wm := &mockChainProvider{byChain: map[models.Chain]*mockChainCtx{
		models.Ethereum:    srcCtx,
		models.Hyperliquid: dstCtx,
	}}
	sm := newStateMachineForTest(t, wm)

	st := &models.DepositState{
		State:       models.StateSrcTxDiscovered,
		SrcChain:    models.Ethereum,
		DstChain:    models.Hyperliquid,
		TxHash:      "0xsrc",
		DepositAddr: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		DstAddr:     common.HexToAddress("0x2222222222222222222222222222222222222222"),
		AmountWei:   big.NewInt(12345),
	}

	ctx := context.Background()

	// SrcTxDiscovered -> SrcTxConfirmed
	next, changed, err := sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateSrcTxConfirmed {
		t.Fatalf("step1 got next=%s changed=%v err=%v", next, changed, err)
	}
	st.State = next

	// SrcTxConfirmed -> DstTxBuilt
	next, changed, err = sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateDstTxBuilt || st.UnsignedDstTx != "raw_dst" {
		t.Fatalf("step2 got next=%s raw=%s err=%v", next, st.UnsignedDstTx, err)
	}
	st.State = next

	// DstTxBuilt -> DstTxSent
	next, changed, err = sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateDstTxSent || st.SentDstTxHash != "0xdsthash" {
		t.Fatalf("step3 got next=%s hash=%s err=%v", next, st.SentDstTxHash, err)
	}
	st.State = next

	// DstTxSent -> DstTxConfirmed
	next, changed, err = sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateDstTxConfirmed {
		t.Fatalf("step4 got next=%s err=%v", next, err)
	}
	st.State = next

	// DstTxConfirmed -> SweepTxBuilt
	next, changed, err = sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateSweepTxBuilt || st.UnsignedSweepTx != "raw_sweep" {
		t.Fatalf("step5 got next=%s raw=%s err=%v", next, st.UnsignedSweepTx, err)
	}
	st.State = next

	// SweepTxBuilt -> SweepTxSent
	next, changed, err = sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateSweepTxSent || st.SentSweepTxHash != "0xsweephash" {
		t.Fatalf("step6 got next=%s hash=%s err=%v", next, st.SentSweepTxHash, err)
	}
	st.State = next

	// SweepTxSent -> SweepTxConfirmed
	next, changed, err = sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateSweepTxConfirmed {
		t.Fatalf("step7 got next=%s err=%v", next, err)
	}
	st.State = next

	// SweepTxConfirmed -> Done
	next, changed, err = sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateDone {
		t.Fatalf("step8 got next=%s err=%v", next, err)
	}
}

func TestTransitionDeposit_DstRejected_RetryPath(t *testing.T) {
	srcCtx := &mockChainCtx{
		isTxConfirmedFn: func(ctx context.Context, txHash string, min uint64) (bool, error) { return true, nil },
		buildSweepTxFn:  func(ctx context.Context, from, to string) (string, error) { return "raw_sweep", nil },
		broadcastTxFn:   func(ctx context.Context, raw, from string) (string, error) { return "0xsweephash", nil },
	}
	dstConfirmedOnce := false
	dstCtx := &mockChainCtx{
		isTxConfirmedFn: func(ctx context.Context, txHash string, min uint64) (bool, error) {
			if !dstConfirmedOnce {
				dstConfirmedOnce = true
				return false, ErrorRejectedTransaction
			}
			return true, nil
		},
		buildSendTxFn: func(ctx context.Context, from, to string, amount *big.Int) (string, error) { return "raw_dst", nil },
		broadcastTxFn: func(ctx context.Context, raw, from string) (string, error) { return "0xdsthash", nil },
	}
	wm := &mockChainProvider{byChain: map[models.Chain]*mockChainCtx{
		models.Ethereum:    srcCtx,
		models.Hyperliquid: dstCtx,
	}}
	sm := newStateMachineForTest(t, wm)

	st := &models.DepositState{
		State:         models.StateDstTxSent,
		SrcChain:      models.Ethereum,
		DstChain:      models.Hyperliquid,
		SentDstTxHash: "0xdsthash",
		DepositAddr:   common.HexToAddress("0x1111111111111111111111111111111111111111"),
		DstAddr:       common.HexToAddress("0x2222222222222222222222222222222222222222"),
		AmountWei:     big.NewInt(1),
	}

	ctx := context.Background()

	next, changed, err := sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateDstTxRejected {
		t.Fatalf("reject step got next=%s changed=%v err=%v", next, changed, err)
	}
	st.State = next

	next, changed, err = sm.TransitionDeposit(ctx, st)
	if err != nil || !changed || next != models.StateDstTxResend {
		t.Fatalf("resend step got next=%s changed=%v err=%v", next, changed, err)
	}
}

func TestTransitionDeposit_Errors_BuildSendTx(t *testing.T) {
	dstCtx := &mockChainCtx{
		buildSendTxFn: func(ctx context.Context, from, to string, amount *big.Int) (string, error) {
			return "", errors.New("build error")
		},
	}
	wm := &mockChainProvider{byChain: map[models.Chain]*mockChainCtx{
		models.Hyperliquid: dstCtx,
	}}
	sm := newStateMachineForTest(t, wm)

	st := &models.DepositState{
		State:       models.StateSrcTxConfirmed,
		SrcChain:    models.Ethereum,
		DstChain:    models.Hyperliquid,
		DepositAddr: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		DstAddr:     common.HexToAddress("0x2222222222222222222222222222222222222222"),
		AmountWei:   big.NewInt(5),
	}

	_, changed, err := sm.TransitionDeposit(context.Background(), st)
	if err == nil || changed {
		t.Fatalf("expected error and no change, got changed=%v err=%v", changed, err)
	}
}

func TestTransitionDeposit_Waiting_NotChanged(t *testing.T) {
	srcCtx := &mockChainCtx{
		isTxConfirmedFn: func(ctx context.Context, txHash string, min uint64) (bool, error) { return false, nil },
	}
	wm := &mockChainProvider{byChain: map[models.Chain]*mockChainCtx{
		models.Ethereum: srcCtx,
	}}
	sm := newStateMachineForTest(t, wm)

	st := &models.DepositState{
		State:    models.StateSrcTxDiscovered,
		SrcChain: models.Ethereum,
		TxHash:   "0xsrc",
	}

	next, changed, err := sm.TransitionDeposit(context.Background(), st)
	if err != nil || changed || next != models.StateSrcTxDiscovered {
		t.Fatalf("got next=%s changed=%v err=%v", next, changed, err)
	}
}

func TestStateMachine_ProcessBlock_InsertsDepositStateForMatchingTx(t *testing.T) {
	depAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	dstAddr := common.HexToAddress("0x2222222222222222222222222222222222222222")

	account := &models.Account{
		ID:          "acct-1",
		DepositAddr: depAddr,
		DstAddr:     dstAddr,
		SrcChain:    "Ethereum",
		DstChain:    "Arbitrum",
	}

	maccounts := &mocks.MockAccountStore{
		ByAddr: map[string]*models.Account{
			depAddr.Hex(): account,
		},
	}

	mstates := newMockStateStore()

	sm := &StateMachine{
		states:   mstates,
		accounts: maccounts,
	}

	to := depAddr
	tx := types.NewTransaction(0, to, big.NewInt(100), 21000, big.NewInt(1), nil)
	block := types.NewBlock(&types.Header{Number: big.NewInt(1)}, &types.Body{Transactions: types.Transactions{tx}}, nil, new(mockTrieHasher))
	if err := sm.ProcessBlock(context.Background(), block); err != nil {
		t.Fatalf("ProcessBlock error: %v", err)
	}

	select {
	case got := <-mstates.putIfCh:
		if got.DepositAddr != depAddr {
			t.Fatalf("DepositAddr = %s, want %s", got.DepositAddr.Hex(), depAddr.Hex())
		}
		if got.DstAddr != dstAddr {
			t.Fatalf("DstAddr = %s, want %s", got.DstAddr.Hex(), dstAddr.Hex())
		}
		if got.AmountWei == nil || got.AmountWei.Cmp(big.NewInt(100)) != 0 {
			t.Fatalf("AmountWei = %v, want 100", got.AmountWei)
		}
		if got.State != models.StateSrcTxDiscovered {
			t.Fatalf("State = %v, want StateSrcTxDiscovered", got.State)
		}
		if got.TxHash == "" {
			t.Fatalf("TxHash empty")
		}
	default:
		t.Fatal("expected PutIfAbsent call")
	}
}

func TestStateMachine_ProcessBlock_SkipsUnknownDepositAddress(t *testing.T) {
	maccounts := &mocks.MockAccountStore{ByAddr: map[string]*models.Account{}}
	mstates := newMockStateStore()
	sm := &StateMachine{states: mstates, accounts: maccounts}

	to := common.HexToAddress("0x3333333333333333333333333333333333333333")
	tx := types.NewTransaction(0, to, big.NewInt(1), 21000, big.NewInt(1), nil)
	block := types.NewBlock(&types.Header{Number: big.NewInt(3)}, &types.Body{Transactions: types.Transactions{tx}}, nil, new(mockTrieHasher))

	if err := sm.ProcessBlock(context.Background(), block); err != nil {
		t.Fatalf("ProcessBlock error: %v", err)
	}

	select {
	case <-mstates.putIfCh:
		t.Fatal("PutIfAbsent should not be called for unknown address")
	default:
	}
}
