test:
	go test $$(go list ./... | grep -v /constants$$ | grep -v /mocks$$)

init:
	go run ./cmd/init/main.go

cleanup:
	go run ./cmd/cleanup/main.go

start:
	go run ./cmd/agent/main.go
