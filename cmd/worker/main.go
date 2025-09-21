package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"unit/agent/internal/models"
	"unit/agent/internal/services"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/ethclient"
)

func main() {
	sepoliaUrl := os.Getenv("SEPOLIA_RPC_URL")
	sepoliaAddr := os.Getenv("SEPOLIA_HOT_WALLET")
	// hlUrl := os.Getenv("HYPERLIQUID_RPC_URL")
	// hlAddr := os.Getenv("HYPERLIQUID_HOT_WALLET")

	primaryClient, err := ethclient.Dial(sepoliaUrl)
	if err != nil {
		log.Fatalf("failed to connect to sepolia eth client: %v", err)
	}

	publisher := services.NewBlockPublisher(primaryClient)

	ks, err := stores.NewLocalKeyStore("password", "./tmp/keys")
	if err != nil {
		log.Fatalf("failed to initialize key store: %v", err)
	}

	as, err := stores.NewLocalAccountStore("./tmp/accounts.db")
	if err != nil {
		log.Fatalf("failed to initialize account store: %v", err)
	}

	st, err := stores.NewLocalStateStore("./tmp/state.db")
	if err != nil {
		log.Fatalf("failed to initialize state store: %v", err)
	}

	clients := map[models.Chain]*ethclient.Client{
		models.Ethereum: primaryClient,
	}
	hotWallets := map[models.Chain]string{
		models.Ethereum: sepoliaAddr,
	}
	wm := services.NewWalletManager(ks, clients)
	sm, err := services.NewStateMachine(primaryClient, wm, as, st, hotWallets)
	if err != nil {
		log.Fatalf("failed to initialize state machine: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigch
		fmt.Println("stopping")
		cancel()
	}()

	go func() {
		if err := publisher.Start(ctx); err != nil {
			log.Fatalf("block publisher stopped: %v", err)
		}
	}()

	go func() {
		if err := sm.Start(ctx); err != nil {
			log.Fatalf("state machine stopped: %v", err)
		}
	}()

	for {
		select {
		case block, ok := <-publisher.Out():
			if !ok {
				fmt.Println("block channel closed")
				return
			}
			fmt.Printf("finalized block %d, hash=%s\n", block.NumberU64(), block.Hash().Hex())

			if err := sm.ProcessBlock(ctx, block); err != nil {
				log.Fatalf("error processing block %d: %v", block.NumberU64(), err)
			}

		case err, ok := <-publisher.Err():
			if !ok {
				fmt.Println("error channel closed")
				return
			}
			log.Printf("publisher error: %v", err)

		case <-ctx.Done():
			fmt.Println("stopping")
			return
		}
	}
}
