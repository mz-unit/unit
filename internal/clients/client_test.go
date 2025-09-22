package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHttpClient_Post_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %s, want application/json", ct)
		}
		var data map[string]any
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if data["foo"] != "bar" {
			t.Fatalf("payload foo = %v, want bar", data["foo"])
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewHttpClient(srv.URL)
	body, err := c.Post(context.Background(), "/test", map[string]string{"foo": "bar"})
	if err != nil {
		t.Fatalf("Post error: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %s, want %s", string(body), `{"ok":true}`)
	}
}

func TestHttpClient_Post_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewHttpClient(srv.URL)
	_, err := c.Post(context.Background(), "/fail", map[string]string{"foo": "bar"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got == "" || got[:6] != "status" {
		t.Fatalf("unexpected error string: %q", got)
	}
}

func TestHttpClient_Post_MarshalError(t *testing.T) {
	c := NewHttpClient("http://localhost")
	ch := make(chan int)
	_, err := c.Post(context.Background(), "/marshal", ch)
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
}

func TestHttpClient_Post_RequestError(t *testing.T) {
	c := &HttpClient{
		BaseURL: "http://127.0.0.1:0", // invalid port
		HttpClient: &http.Client{
			Timeout: 10 * time.Millisecond,
		},
	}
	_, err := c.Post(context.Background(), "/path", map[string]string{"foo": "bar"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
