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

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	hyperliquid "github.com/sonirico/go-hyperliquid"
)

func main() {
	sepoliaUrl := os.Getenv("SEPOLIA_RPC_URL")
	sepoliaAddr := os.Getenv("SEPOLIA_HOT_WALLET")
	hlAddr := os.Getenv("HYPERLIQUID_HOT_WALLET")
	hlPrivKey := os.Getenv("HYPERLIQUID_PRIV_KEY")

	hlClient := hyperliquid.NewClient(hyperliquid.TestnetAPIURL)
	privateKey, _ := crypto.HexToECDSA(hlPrivKey)
	exchange := hyperliquid.NewExchange(
		context.Background(),
		privateKey,
		hyperliquid.MainnetAPIURL,
		nil,
		"vault-address",
		hlAddr,
		nil,
	)

	primaryClient, err := ethclient.Dial(sepoliaUrl)
	if err != nil {
		log.Fatalf("failed to connect to sepolia eth client: %v", err)
	}

	publisher := services.NewBlockPublisher(primaryClient)

	ks, _ := stores.NewLocalKeyStore("password", "./tmp/keys")
	as, _ := stores.NewLocalAccountStore("./tmp/accounts.db")
	st, _ := stores.NewLocalStateStore("./tmp/state.db")

	clients := map[models.Chain]*ethclient.Client{
		models.Ethereum: primaryClient,
	}
	hotWallets := map[models.Chain]string{
		models.Ethereum: sepoliaAddr,
	}
	wm := services.NewWalletManager(ks, clients)
	sm, err := services.NewStateMachine(primaryClient, wm, as, st, exchange, hotWallets)
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
