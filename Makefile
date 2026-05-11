.PHONY: build run-olympus run-agent test verify-llms-context

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

verify-llms-context:
	@echo "Verifying llms.txt is up to date..."
	@cp llms.txt llms.txt.bak
	@./scripts/gen-llms-context.sh
	@diff llms.txt llms.txt.bak || (echo "llms.txt is out of date. Please run ./scripts/gen-llms-context.sh and commit the changes." && rm llms.txt.bak && exit 1)
	@rm llms.txt.bak
	@echo "llms.txt is up to date."
