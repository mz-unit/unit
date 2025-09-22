## Run locally
1. Create .env file following .env.example. This program requires a funded hot wallet to credit deposits, please do not use a sensitive wallet. Wallet should be holding USDC on Hyperliquid testnet.
2. Run `make init` to import environment's private key into local key store.
3. Start agent by running `make start`. This will start the API server, block publisher, and state machine.
5. Clean up by running `make teardown`. This will delete all persisted data (deposit addresses, workflow states, keys).

## Deposit flow
1. Call
```
curl --request GET \
  --url http://localhost:8000/gen/ethereum/hyperliquid/eth/{sourceAddress}
```
This will generate a deposit address for a sepolia -> hyperliquid deposit

2. Send ETH on Sepolia to deposit address.
3. Agent detects the deposit and begin waiting for confirmations.
3. Once the transaction has required confirmations (14), agent will credit the deposit on Hyperliquid (0.01 ETH = 10 USDC)
4. Once destination deposit ransaction is confirmed, agent submits transaction to sweep funds out of deposit address. The funds go back to the provided `HOT_WALLET_ADDRESS`.
5. On sweep transaction finalization, deposit workflow is marked as done.



## Limitations
- The block publisher does not persist its last seen block. This means if the service is stopped and restarted, we will only start again from the current head. To address this we can introduce periodic checkpointing to recover gracefully.
- To persist workflow state, I elected to store a simple state object that gets mutated on transitions. In a production environment I would introduce an append-only log instead to ensure auditability and replayability.
- The state machine processes deposits one by one and does so by scanning the entire state DB. This is inefficient and won't scale, instead we should rely on a proper index or introduce a job queue
