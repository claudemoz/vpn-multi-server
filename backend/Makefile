APP_NAME=wg-vpn-platform
MAIN=./cmd/api
BIN=bin/$(APP_NAME)
AGENT_MAIN=./cmd/agent
AGENT_BIN=bin/wg-agent

DB_HOST?=localhost
DB_PORT?=5432
DB_USER?=postgres
DB_PASSWORD?=postgres
DB_NAME?=wg-vpn-platform
DB_SSLMODE?=disable

# DB_URL?=postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)
DB_URL?=postgresql://ujcnhcb4dml17onzefuv:YYAU4aYHKll0wB8kLosd@b7ibhmkkxeupqxodsndh-postgresql.services.clever-cloud.com:9292/b7ibhmkkxeupqxodsndh
SEED_DIR=seed
SEED_DB_URL=$(DB_URL)&x-migrations-table=seed_schema_migrations

.PHONY: migration migrate-up migrate-down migrate-version \
	seed-migration seed-up seed-down seed-version \
	run dev build test fmt tidy seed setup-db \
	proto run-agent build-agent

run:
	go run $(MAIN)

dev:
	air -c .air.toml

build:
	go build -o $(BIN) $(MAIN)

# Agent (runs on each WireGuard server). AGENT_BACKEND=memory lets you run it
# locally without root or a wg0 interface.
run-agent:
	AGENT_BACKEND=$${AGENT_BACKEND:-memory} go run $(AGENT_MAIN)

build-agent:
	CGO_ENABLED=0 GOOS=$${GOOS:-linux} GOARCH=$${GOARCH:-amd64} go build -o $(AGENT_BIN) $(AGENT_MAIN)

# Regenerate gRPC code from api/proto (see scripts/gen-proto.sh for tooling).
proto:
	./scripts/gen-proto.sh
	
test:
	go test ./... -cover

fmt:
	go fmt ./...

tidy:
	go mod tidy
	
# 	
migration:
	migrate create -ext sql -dir migrations -seq $(name)

seed-migration:
	migrate create -ext sql -dir $(SEED_DIR) -seq $(name)

migrate-up:
	migrate -path migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path migrations -database "$(DB_URL)" down 1

seed-up:
	migrate -path $(SEED_DIR) -database "$(SEED_DB_URL)" up

seed-down:
	migrate -path $(SEED_DIR) -database "$(SEED_DB_URL)" down 1

setup-db: migrate-up seed-up