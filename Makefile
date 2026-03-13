.PHONY: build test test-race lint sqlc migrate-up migrate-down dev clean openapi dashboard

# Binary
BIN := steerlane
CMD := ./cmd/steerlane

# Tools
GOLANGCI_LINT := golangci-lint
SQLC := sqlc
MIGRATE := migrate

# Database
POSTGRES_DSN ?= postgres://steerlane:steerlane@localhost:5432/steerlane?sslmode=disable

## build: Build the server binary.
build:
	go build -o $(BIN) $(CMD)

## test: Run all tests.
test:
	go test ./...

## test-race: Run all tests with race detector.
test-race:
	go test -race -count=1 ./...

## lint: Run golangci-lint.
lint:
	$(GOLANGCI_LINT) run

## sqlc: Generate type-safe Go from SQL queries.
sqlc:
	$(SQLC) generate

## sqlc-check: Verify sqlc-generated code is up to date.
sqlc-check:
	$(SQLC) generate
	git diff --exit-code internal/store/postgres/sqlc/

## migrate-up: Run all database migrations.
migrate-up:
	$(MIGRATE) -path migrations -database "$(POSTGRES_DSN)" up

## migrate-down: Roll back the last migration.
migrate-down:
	$(MIGRATE) -path migrations -database "$(POSTGRES_DSN)" down 1

## migrate-create: Create a new migration pair. Usage: make migrate-create NAME=create_foo
migrate-create:
	$(MIGRATE) create -ext sql -dir migrations -seq $(NAME)

## dev: Run the server with hot-reload (requires air or similar).
dev:
	go run $(CMD)

## openapi: Fetch the OpenAPI spec from the running server.
openapi:
	curl -s http://localhost:8080/openapi.json | jq . > openapi.json

## dashboard: Build the SvelteKit dashboard assets.
dashboard:
	npm --prefix web run build

## clean: Remove build artifacts.
clean:
	rm -f $(BIN) openapi.json

## help: Show this help.
help:
	@grep -E '^##' Makefile | sed 's/## //'
