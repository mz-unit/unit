package address

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// Validates address and returns checksummed EVM address
// Checksumming is used to normalize EVM addresses for storage (can also lowercase all addresses instead)
// NOTE: Solana addresses are case sensitive and we must store them in their exact original form
func Checksummed(addressStr string) (string, error) {
	if !common.IsHexAddress(addressStr) {
		return "", fmt.Errorf("invalid address: %s", addressStr)
	}
	address := common.HexToAddress(addressStr)
	return address.Hex(), nil
}
