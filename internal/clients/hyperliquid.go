package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type HyperliquidClient interface {
	CreditDeposit(ctx context.Context, dstAddr string, asset string, amount string) error
	RequestWithdraw(ctx context.Context, srcAddr string, asset string, amount string, dstAddr string) (requestID string, err error)
}

type HTTPClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewHTTPClient(baseURL, apiKey string) *HTTPClient {
	return &HTTPClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *HTTPClient) CreditDeposit(ctx context.Context, dstAddr, asset, amount string) error {
	// NOTE: Hyperliquid's real API will have an authenticated endpoint.
	payload := map[string]interface{}{
		"destination": dstAddr,
		"asset":       asset,
		"amount":      amount,
		"reason":      "unit-deposit",
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", h.baseURL+"/v1/internal/credit", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("hyperliquid credit failed: %s", resp.Status)
	}
	return nil
}

func (h *HTTPClient) RequestWithdraw(ctx context.Context, srcAddr, asset, amount, dstAddr string) (string, error) {
	payload := map[string]string{
		"source_account": srcAddr,
		"asset":          asset,
		"amount":         amount,
		"destination":    dstAddr,
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", h.baseURL+"/v1/withdraws", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("hyperliquid withdraw failed: %s", resp.Status)
	}
	// parse ID from response if any; for demo return a static value
	return "req-123", nil
}
