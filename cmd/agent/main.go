package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"unit/agent/internal/api"
	"unit/agent/internal/constants"
	"unit/agent/internal/models"
	"unit/agent/internal/services"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	hyperliquid "github.com/sonirico/go-hyperliquid"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	sepoliaUrl := os.Getenv("SEPOLIA_RPC_URL")
	hotWalletAddr := os.Getenv("HOT_WALLET_ADDRESS")
	hotWalletPrivKey := os.Getenv("HOT_WALLET_PRIVATE_KEY")
	srcChains := []string{"ethereum", "hyperliquid"}
	dstChains := []string{"ethereum", "hyperliquid"}
	assets := []string{"eth"}

	ethClient, err := ethclient.Dial(sepoliaUrl)
	if err != nil {
		log.Fatalf("failed to connect to sepolia eth client: %v", err)
	}
	fmt.Println("connected to eth client")

	publisher := services.NewBlockPublisher(ethClient)

	ks, err := stores.NewLocalKeyStore(constants.KeyStorePassword, constants.KeyStorePath)
	if err != nil {
		log.Fatalf("failed to initialize key store %v", err)
	}
	as, err := stores.NewLocalAccountStore(constants.AccountDbPath)
	if err != nil {
		log.Fatalf("failed to initialize account store %v", err)
	}
	st, err := stores.NewLocalStateStore(constants.StateDbPath)
	if err != nil {
		log.Fatalf("failed to initialize state store %v", err)
	}
	fmt.Println("initialized stores")

	hlInfo := hyperliquid.NewInfo(context.Background(), hyperliquid.TestnetAPIURL, true, nil, nil)
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(hotWalletPrivKey, "0x"))
	if err != nil {
		log.Fatalf("failed to parse private key: %v", err)
	}
	hlHotWalletExg := hyperliquid.NewExchange(
		context.Background(),
		privateKey,
		hyperliquid.TestnetAPIURL,
		nil,
		"",
		hotWalletAddr,
		nil,
	)

	wm := services.NewWalletManager(ks, map[models.Chain]*ethclient.Client{
		models.Ethereum: ethClient,
	}, hlHotWalletExg, hlInfo)
	sm, err := services.NewStateMachine(wm, as, st, hlHotWalletExg, map[models.Chain]string{
		models.Ethereum: hotWalletAddr,
	})
	if err != nil {
		log.Fatalf("failed to initialize state machine: %v", err)
	}
	a := api.NewApi(ks, as, srcChains, dstChains, assets)

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
		log.Println("API listening on :8000")
		if err := a.Start(); err != nil {
			if err.Error() != "http: Server closed" {
				log.Fatalf("server error: %v", err)
			}
		}
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
			fmt.Printf("block %d, hash=%s\n", block.NumberU64(), block.Hash().Hex())

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
