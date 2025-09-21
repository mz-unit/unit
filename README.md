## Run locally
1. Create .env file following .env.example. This program requires a funded hot wallet to credit deposits, please do not use a sensitive wallet. Wallet should initially hold USDC on Hyperliquid testnet.
2. Run `go run cmd/init/main.go` to import environment's private key into local key store.
3. Start API server by running `go run cmd/api/main.go`. This will handle deposit address generation requests.
4. Start state machine by running `go run cmd/worker/main.go`. This will listen to new blocks on Sepolia and process deposits.

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
