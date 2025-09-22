test:
	go test $$(go list ./... | grep -v /constants$$ | grep -v /mocks$$) -coverprofile .testCoverage.txt

init:
	go run ./cmd/init/main.go

teardown:
	go run ./cmd/teardown/main.go

start:
	go run ./cmd/agent/main.go
