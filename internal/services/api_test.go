package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"unit/agent/internal/mocks"
	"unit/agent/internal/models"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/common"
)

func newAPIForTest(ks stores.IKeyStore, as stores.IAccountStore) *Api {
	src := []string{"ethereum"}
	dst := []string{"hyperliquid"}
	assets := []string{"usdc"}
	return NewApi(ks, as, src, dst, assets)
}

func TestHandleGenerate_ExistingAccount(t *testing.T) {
	existing := &models.Account{
		DepositAddr: common.HexToAddress("0x1111111111111111111111111111111111111111"),
	}
	as := &mocks.MockAccountStore{
		GetFn: func(ctx context.Context, id string) (*models.Account, error) {
			return existing, nil
		},
	}
	api := newAPIForTest(&mocks.MockKeyStore{}, as)

	req := httptest.NewRequest(http.MethodGet, "/gen/ethereum/hyperliquid/usdc/0x960b650301e941c095aef35f57ae1b2d73fc4df1", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body generateResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Address != existing.DepositAddr.Hex() {
		t.Fatalf("address = %s, want %s", body.Address, existing.DepositAddr.Hex())
	}
	if body.Status != "ok" {
		t.Fatalf("status = %s, want ok", body.Status)
	}
}

func TestHandleGenerate_NewAccount(t *testing.T) {
	ks := &mocks.MockKeyStore{Addr: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	as := &mocks.MockAccountStore{
		GetFn: func(ctx context.Context, id string) (*models.Account, error) {
			return nil, stores.ErrAccountNotFound
		},
	}
	api := newAPIForTest(ks, as)

	req := httptest.NewRequest(http.MethodGet, "/gen/ethereum/hyperliquid/usdc/0x960b650301e941c095aef35f57ae1b2d73fc4df1", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body generateResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestHandleGenerate_MethodNotAllowed(t *testing.T) {
	api := newAPIForTest(&mocks.MockKeyStore{}, &mocks.MockAccountStore{})
	req := httptest.NewRequest(http.MethodPost, "/gen/ethereum/hyperliquid/usdc/0x960b650301e941c095aef35f57ae1b2d73fc4df1", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	if w.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Result().StatusCode)
	}
}

func TestHandleGenerate_BadPath(t *testing.T) {
	api := newAPIForTest(&mocks.MockKeyStore{}, &mocks.MockAccountStore{})
	req := httptest.NewRequest(http.MethodGet, "/gen/ethereum/hyperliquid/usdc", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
}

func TestHandleGenerate_UnsupportedChain(t *testing.T) {
	api := newAPIForTest(&mocks.MockKeyStore{}, &mocks.MockAccountStore{})
	req := httptest.NewRequest(http.MethodGet, "/gen/unknown/hyperliquid/usdc/0x960b650301e941c095aef35f57ae1b2d73fc4df1", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
}

func TestHandleGenerate_UnsupportedDstChain(t *testing.T) {
	api := newAPIForTest(&mocks.MockKeyStore{}, &mocks.MockAccountStore{})
	req := httptest.NewRequest(http.MethodGet, "/gen/ethereum/unknown/usdc/0x960b650301e941c095aef35f57ae1b2d73fc4df1", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
}

func TestHandleGenerate_UnsupportedAsset(t *testing.T) {
	api := newAPIForTest(&mocks.MockKeyStore{}, &mocks.MockAccountStore{})
	req := httptest.NewRequest(http.MethodGet, "/gen/ethereum/hyperliquid/eth/0x960b650301e941c095aef35f57ae1b2d73fc4df1", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
}

func TestHandleGenerate_InvalidDestinationAddress(t *testing.T) {
	api := newAPIForTest(&mocks.MockKeyStore{}, &mocks.MockAccountStore{})
	req := httptest.NewRequest(http.MethodGet, "/gen/ethereum/hyperliquid/usdc/not-an-address", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	if w.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Result().StatusCode)
	}
}

func TestHandleGenerate_InternalErrorOnGet(t *testing.T) {
	as := &mocks.MockAccountStore{
		GetFn: func(ctx context.Context, id string) (*models.Account, error) {
			return nil, errors.New("boom")
		},
	}
	api := newAPIForTest(&mocks.MockKeyStore{}, as)

	req := httptest.NewRequest(http.MethodGet, "/gen/ethereum/hyperliquid/usdc/0x960b650301e941c095aef35f57ae1b2d73fc4df1", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Result().StatusCode)
	}
}

func TestHandleGenerate_InternalErrorOnCreateKey(t *testing.T) {
	ks := &mocks.MockKeyStore{Err: errors.New("keygen fail")}
	as := &mocks.MockAccountStore{
		GetFn: func(ctx context.Context, id string) (*models.Account, error) {
			return nil, stores.ErrAccountNotFound
		},
	}
	api := newAPIForTest(ks, as)

	req := httptest.NewRequest(http.MethodGet, "/gen/ethereum/hyperliquid/usdc/0x960b650301e941c095aef35f57ae1b2d73fc4df1", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Result().StatusCode)
	}
}

func TestHandleGenerate_InternalErrorOnInsert(t *testing.T) {
	ks := &mocks.MockKeyStore{Addr: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	as := &mocks.MockAccountStore{
		GetFn: func(ctx context.Context, id string) (*models.Account, error) {
			return nil, stores.ErrAccountNotFound
		},
		InsertFn: func(ctx context.Context, a models.Account) error {
			return errors.New("insert fail")
		},
	}
	api := newAPIForTest(ks, as)

	req := httptest.NewRequest(http.MethodGet, "/gen/ethereum/hyperliquid/usdc/0x960b650301e941c095aef35f57ae1b2d73fc4df1", nil)
	w := httptest.NewRecorder()

	api.HandleGenerate(w, req)
	if w.Result().StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Result().StatusCode)
	}
}
