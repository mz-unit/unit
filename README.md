
## Run locally
1. Create .env file following .env.example. This program requires private keys, please do not use a sensitive wallet.
2. Run `go run cmd/init/main.go` to import required private keys into local key store.
3. Start API server by running `go run cmd/api/main.go`.
4. Start state machine by running `go run cmd/worker/main.go`.
