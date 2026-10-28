.DEFAULT_GOAL := help
SHELL := bash

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

setup: gen ## First-time setup: generate code, resolve Go modules, install web deps
	go mod tidy
	cd web && npm install

gen: ## Generate Go code from proto/ (needs buf)
	buf generate

lint: ## buf lint, go vet, TypeScript check
	buf lint
	go vet ./...
	cd web && npm run typecheck

test: ## All tests: Go (with race detector) and frontend
	go test -race ./...
	cd web && npm test

e2e: ## Browser tests: starts its own services, needs Go and Playwright's Chromium
	cd web && npm run e2e

dev: ## Run all services locally without Docker (in-memory topology) + Vite
	@trap 'kill 0' EXIT; \
	LISTEN_ADDR=:9001 go run ./cmd/topology & \
	LISTEN_ADDR=:9002 TOPOLOGY_ADDR=localhost:9001 go run ./cmd/impact & \
	LISTEN_ADDR=:9003 TOPOLOGY_ADDR=localhost:9001 LIGHTPATH_MODE=$${LIGHTPATH_MODE:-live} go run ./cmd/events & \
	LISTEN_ADDR=:8080 EVENTS_ADDR=localhost:9003 go run ./cmd/gateway & \
	cd web && npm run dev

up: ## Build and start everything with Docker Compose on :8080
	docker compose up -d --build

down: ## Stop the stack
	docker compose down

logs: ## Follow logs from all services
	docker compose logs -f --tail=100

deploy: ## Production: compose + Caddy with HTTPS (needs DOMAIN, POSTGRES_PASSWORD)
	docker compose -f docker-compose.yml -f deploy/compose.prod.yml up -d --build

.PHONY: help setup gen lint test e2e dev up down logs deploy
