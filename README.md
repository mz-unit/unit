## Run locally
1. Create .env file following .env.example. This program requires a funded hot wallet to credit deposits, please do not use a sensitive wallet. Wallet should hold USDC on Hyperliquid testnet.
2. Run `go run cmd/init/main.go` to import environment's private key into local key store.
3. Start agent by running `go run cmd/agent/main.go`. This will start the API server, block publisher, and state machine.
5. Clean up by running `go run cmd/cleanup/main.go`. This will delete all persisted data (deposit addresses, workflow states, keys).

## Deposit flow
1. Call
```
curl --request GET \
  --url http://localhost:8000/gen/ethereum/hyperliquid/eth/{sourceAddress}
```
This will generate a deposit address for a sepolia -> hyperliquid deposit

2. Send ETH on Sepolia to deposit address.
3. Once the transaction is finalized, the worker will pick up the transaction and start executing the deposit state machine.
4. USDC credited on Hyperliquid for the destination address.
