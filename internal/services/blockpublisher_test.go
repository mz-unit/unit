package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type rpcReq struct {
	ID     any           `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

type rpcResp struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      any         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type fakeChain struct {
	latest  uint64
	blocks  map[uint64]map[string]any
	failHdr bool
	failBlk bool
}

func newFakeChain() *fakeChain {
	return &fakeChain{
		latest: 0,
		blocks: map[uint64]map[string]any{},
	}
}

func (fc *fakeChain) setLatest(n uint64) {
	fc.latest = n
}

func (fc *fakeChain) setFail(headerFail, blockFail bool) {
	fc.failHdr = headerFail
	fc.failBlk = blockFail
}

func (fc *fakeChain) putBlock(n uint64) {
	h := "0xa444b1e4f2e0cc3d93d50c489aca46b04b263f55879688c061cb70daf5b8a0fa"
	parent := fmt.Sprintf("0x%064x", n-1)
	fc.blocks[n] = map[string]any{
		"number":           fmt.Sprintf("0x%x", n),
		"hash":             h,
		"parentHash":       parent,
		"nonce":            "0x0000000000000000",
		"sha3Uncles":       "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
		"logsBloom":        "0x" + strings.Repeat("0", 512),
		"transactionsRoot": h,
		"stateRoot":        h,
		"receiptsRoot":     h,
		"miner":            "0x0000000000000000000000000000000000000000",
		"difficulty":       "0x0",
		"totalDifficulty":  "0x0",
		"extraData":        "0x",
		"size":             "0x0",
		"gasLimit":         "0x0",
		"gasUsed":          "0x0",
		"timestamp":        "0x0",
		"transactions": []any{
			map[string]any{
				"hash":             h,
				"nonce":            "0x0",
				"blockHash":        h,
				"blockNumber":      fmt.Sprintf("0x%x", n),
				"transactionIndex": "0x0",
				"from":             "0x0000000000000000000000000000000000000001",
				"to":               "0x0000000000000000000000000000000000000002",
				"value":            "0x1",
				"gas":              "0x5208",
				"gasPrice":         "0x1",
				"input":            "0x",
				"v":                "0x1b",
				"r":                "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"s":                "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
		},
		"uncles":        []any{},
		"baseFeePerGas": "0x0",
	}
}

func (fc *fakeChain) serveRPC(w http.ResponseWriter, r *http.Request) {
	var req rpcReq
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r.Body)
	_ = json.Unmarshal(buf.Bytes(), &req)

	write := func(res rpcResp) {
		res.JSONRPC = "2.0"
		_ = json.NewEncoder(w).Encode(res)
	}

	switch req.Method {
	case "eth_getHeaderByNumber":
		if fc.failHdr {
			write(rpcResp{ID: req.ID, Error: &rpcError{Code: -32000, Message: "header failure"}})
			return
		}

		var numArg string
		if len(req.Params) == 0 || req.Params[0] == nil {
			numArg = "latest"
		} else {
			numArg, _ = req.Params[0].(string)
		}

		var n uint64
		if numArg == "latest" {
			n = fc.latest
		} else {
			if strings.HasPrefix(numArg, "0x") {
				v, _ := strconv.ParseUint(numArg[2:], 16, 64)
				n = v
			}
		}

		if n == 0 {
		}
		hexNum := fmt.Sprintf("0x%x", n)
		result := map[string]any{
			"number":     hexNum,
			"hash":       "0xa444b1e4f2e0cc3d93d50c489aca46b04b263f55879688c061cb70daf5b8a0fa",
			"parentHash": "0xa444b1e4f2e0cc3d93d50c489aca46b04b263f55879688c061cb70daf5b8a0fa",
			"nonce":      "0x0000000000000000",
		}
		write(rpcResp{ID: req.ID, Result: result})

	case "eth_getBlockByNumber":
		if fc.failBlk {
			write(rpcResp{ID: req.ID, Error: &rpcError{Code: -32000, Message: "block failure"}})
			return
		}

		if len(req.Params) < 1 {
			write(rpcResp{ID: req.ID, Error: &rpcError{Code: -32602, Message: "missing params"}})
			return
		}
		var n uint64
		switch v := req.Params[0].(type) {
		case string:
			if v == "latest" {
				n = fc.latest
			} else if strings.HasPrefix(v, "0x") {
				u, _ := strconv.ParseUint(v[2:], 16, 64)
				n = u
			}
		default:
			write(rpcResp{ID: req.ID, Error: &rpcError{Code: -32602, Message: "bad param"}})
			return
		}

		blk, ok := fc.blocks[n]
		if !ok {
			write(rpcResp{ID: req.ID, Error: &rpcError{Code: -32000, Message: "unknown block"}})
			return
		}
		write(rpcResp{ID: req.ID, Result: blk})

	default:
		write(rpcResp{ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}})
	}
}

func newEthClient(t *testing.T, fc *fakeChain) *ethclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(fc.serveRPC))
	t.Cleanup(srv.Close)

	cli, err := ethclient.Dial(srv.URL)
	if err != nil {
		t.Fatalf("ethclient.Dial: %v", err)
	}
	return cli
}

func readOut(t *testing.T, ch <-chan *types.Block, timeout time.Duration) *types.Block {
	t.Helper()
	select {
	case b := <-ch:
		return b
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for block")
		return nil
	}
}

func readErr(t *testing.T, ch <-chan error, timeout time.Duration) error {
	t.Helper()
	select {
	case e := <-ch:
		return e
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for error")
		return nil
	}
}

func TestBlockPublisher_Start_EmitsErrorOnHeaderFailure(t *testing.T) {
	fc := newFakeChain()
	fc.setFail(true, false)

	client := newEthClient(t, fc)
	bp := NewBlockPublisher(client)
	bp.interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go bp.Start(ctx)

	err := readErr(t, bp.Err(), 800*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "error getting latest header") {
		t.Fatalf("unexpected error: %v", err)
	}
	cancel()
}

func TestBlockPublisher_getLatestBlock_ReturnsBlock(t *testing.T) {
	fc := newFakeChain()
	fc.putBlock(7)
	fc.setLatest(7)

	client := newEthClient(t, fc)
	bp := NewBlockPublisher(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	blk, err := bp.getLatestBlock(ctx)
	if err != nil {
		t.Fatalf("getLatestBlock error: %v", err)
	}
	if blk == nil || blk.NumberU64() != 7 {
		t.Fatalf("got block #%v, want 7", blk.NumberU64())
	}
}

func TestBlockPublisher_publishBlock_PropagatesClientError(t *testing.T) {
	fc := newFakeChain()
	fc.setLatest(9)
	fc.setFail(false, true)

	client := newEthClient(t, fc)
	bp := NewBlockPublisher(client)

	err := bp.publishBlock(context.Background(), 9)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBlockPublisher_Start_CancelStopsLoop(t *testing.T) {
	fc := newFakeChain()
	fc.putBlock(1)
	fc.setLatest(1)

	client := newEthClient(t, fc)
	bp := NewBlockPublisher(client)
	bp.interval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = bp.Start(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("start loop did not stop")
	}
}
