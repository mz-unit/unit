package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"unit/agent/internal/models"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/common"
)

type Api struct {
	server    *http.Server
	keys      stores.IKeyStore
	accounts  stores.IAccountStore
	srcChains []string
	dstChains []string
	assets    []string
}

func NewApi(ks stores.IKeyStore, as stores.IAccountStore, srcChains []string, dstChains []string, assets []string) *Api {
	a := &Api{
		keys:      ks,
		accounts:  as,
		srcChains: srcChains,
		dstChains: dstChains,
		assets:    assets,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/gen/", a.HandleGenerate)

	a.server = &http.Server{
		Addr:    ":8000",
		Handler: mux,
	}
	return a
}

func (a *Api) Start() error {
	return a.server.ListenAndServe()
}

func (a *Api) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}

type generateResponse struct {
	Address string `json:"address"`
	Status  string `json:"status"`
}

func (a *Api) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/gen/"), "/")
	if len(parts) != 4 {
		http.Error(w, "invalid request, expected /gen/:chain/:dst_chain/:asset/:dst_addr", http.StatusBadRequest)
		return
	}

	chain := parts[0]
	dstChain := parts[1]
	asset := parts[2]
	dstAddr := parts[3]

	if !slices.Contains(a.srcChains, chain) {
		http.Error(w, "unsupported chain", http.StatusBadRequest)
		return
	}

	if !slices.Contains(a.dstChains, dstChain) {
		http.Error(w, "unsupported destination chain", http.StatusBadRequest)
		return
	}

	if !slices.Contains(a.assets, asset) {
		http.Error(w, "unsupported asset", http.StatusBadRequest)
		return
	}

	if !common.IsHexAddress(dstAddr) {
		http.Error(w, "invalid destination address", http.StatusBadRequest)
		return
	}

	id := models.AccountID(models.Chain(chain), models.Chain(dstChain), dstAddr)

	existing, err := a.accounts.Get(ctx, id)
	if err != nil && !errors.Is(err, stores.ErrAccountNotFound) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if existing != nil {
		resp := generateResponse{
			Address: existing.DepositAddr.Hex(),
			Status:  "ok",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	depositAddr, err := a.keys.CreateKey(ctx)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	account, err := models.NewAccount(models.Chain(chain), models.Chain(dstChain), dstAddr, depositAddr)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	err = a.accounts.Insert(ctx, *account)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	resp := generateResponse{
		Address: depositAddr,
		Status:  "ok",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
