package main

import (
	"log"
	"os"
	"strings"
	"unit/agent/internal/constants"
	"unit/agent/internal/stores"

	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	// import Hyperliquid hot wallet private key to keystore
	hotWalletKey := os.Getenv("HOT_WALLET_PRIV_KEY")
	privateKey, _ := crypto.HexToECDSA(strings.TrimPrefix(hotWalletKey, "0x"))
	keyStore, _ := stores.NewLocalKeyStore(constants.KeyStorePassword, constants.KeyStorePath)

	addr, err := keyStore.ImportECDSA(privateKey, constants.KeyStorePassword)
	if err != nil {
		log.Fatalf("import failed: %v", err)
	}

	log.Printf("imported private key, address %s", addr)
}
