package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"unit/agent/internal/api"
	"unit/agent/internal/stores"
)

func main() {
	srcChains := []string{"ethereum", "hyperliquid"}
	dstChains := []string{"ethereum", "hyperliquid"}
	assets := []string{"eth"}

	accountStore, err := stores.NewLocalAccountStore("./tmp/accounts.db")
	if err != nil {
		log.Fatalf("init account store: %v", err)
	}
	keyStore, err := stores.NewLocalKeyStore("password", "./tmp/keys")
	if err != nil {
		log.Fatalf("init keystore: %v", err)
	}

	a := api.NewApi(keyStore, accountStore, srcChains, dstChains, assets)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigc
		log.Println("signal received, shutting down...")
		shutdownCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		if err := a.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
		cancel()
	}()

	log.Println("serving on :8000")
	if err := a.Start(); err != nil {
		if err.Error() != "http: Server closed" {
			log.Fatalf("server error: %v", err)
		}
	}
	log.Println("server stopped")
}
