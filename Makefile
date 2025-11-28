.PHONY: build run-olympus run-agent test

build:
	go build ./...

run-olympus:
	go run cmd/olympus-api/main.go

run-agent:
	go run cmd/hecatoncheir-agent/main.go

test:
	go test ./...
