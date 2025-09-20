package services

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"unit/agent/internal/models"
	"unit/agent/internal/stores"
)

type ApiServer struct {
	server *http.Server
	ks     stores.KeyStore
	as     stores.AccountStore
}

func NewApiServer(ks stores.KeyStore, as stores.AccountStore) *ApiServer {
	a := &ApiServer{
		ks: ks,
		as: as,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/gen", a.handleGenerate)

	a.server = &http.Server{
		Addr:    ":8000",
		Handler: mux,
	}
	return a
}

func (a *ApiServer) Start() error {
	return a.server.ListenAndServe()
}

func (a *ApiServer) Shutdown(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}

type generateResponse struct {
	Address string `json:"address"`
	Status  string `json:"status"`
	// Signature string `json:"signature"`
}

func (a *ApiServer) handleGenerate(w http.ResponseWriter, r *http.Request) {
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

	// additional validation should be in place to ensure destination address is valid for a destination chain if we are to expand beyond EVM
	chain := parts[0]
	dstChain := parts[1]
	asset := parts[2]
	dstAddr := parts[3]

	account, err := models.NewAccount(chain, dstChain, dstAddr)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if account.Chain != "ethereum" {
		http.Error(w, "unsupported chain", http.StatusBadRequest)
		return
	}

	if account.DstChain != "hyperliquid" {
		http.Error(w, "unsupported destination chain", http.StatusBadRequest)
		return
	}

	if asset != "eth" {
		http.Error(w, "unsupported asset", http.StatusBadRequest)
		return
	}

	existing, err := a.as.GetByID(ctx, account.ID())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if existing != nil && existing.DepositAddr != nil {
		resp := generateResponse{
			Address: *existing.DepositAddr,
			Status:  "ok",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	depositAddr, err := a.ks.CreateAccount()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	err = account.SetDepositAddress(depositAddr)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	err = a.as.Insert(ctx, *account)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	resp := generateResponse{
		Address: depositAddr,
		Status:  "ok",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
