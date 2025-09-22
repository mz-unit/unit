test:
	go test ./...

init:
	go run ./cmd/init/main.go

cleanup:
	go run ./cmd/cleanup/main.go

start:
	go run ./cmd/agent/main.go
