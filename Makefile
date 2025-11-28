.PHONY: build run-olympus run-agent test

build:
	go build ./...

run-olympus:
	go run cmd/olympus-api/main.go

run-agent:
	go run cmd/hecatoncheir-agent/main.go

test:
	go test ./...

up:
	docker-compose up --build -d

down:
	docker-compose down

cli:
	go build -o bin/tartarus cmd/tartarus/main.go
