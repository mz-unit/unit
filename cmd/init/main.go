package main

import (
	"log"
	"os"
	"strings"
	"unit/agent/internal/constants"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// import Hyperliquid hot wallet private key to keystore
	hotWalletKey := os.Getenv("HOT_WALLET_PRIVATE_KEY")
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(hotWalletKey, "0x"))
	if err != nil {
		log.Fatalf("failed to parse private key: %v", err)
	}
	keyStore, _ := stores.NewLocalKeyStore(constants.KeyStorePassword, constants.KeyStorePath)

	addr, err := keyStore.ImportECDSA(privateKey, constants.KeyStorePassword)
	if err != nil {
		log.Fatalf("import failed: %v", err)
	}

	log.Printf("imported private key, address %s", addr)
}
