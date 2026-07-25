.PHONY: help up down restart build test lint deploy status logs health ci snapshot rollback

COMPOSE := compose/docker-compose.yml

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

up: ## Start the full stack
	docker compose -f $(COMPOSE) up -d

down: ## Stop the full stack
	docker compose -f $(COMPOSE) down

restart: ## Restart the full stack
	docker compose -f $(COMPOSE) restart

build: ## Rebuild all local images
	docker compose -f $(COMPOSE) build

test: ## Run Handoff Go tests
	cd handoff/src && go test -v ./...

lint: ## Run linters (Go + Shell)
	cd handoff/src && golangci-lint run
	shellcheck scripts/*.sh

deploy: ## Run the full deploy pipeline
	./scripts/deploy.sh

status: ## Show stack status
	./scripts/status.sh

logs: ## Tail all logs
	./scripts/logs_all.sh

health: ## Run health checks
	./scripts/health_checks.sh

ci: ## Run the CI suite
	./scripts/ci.sh

snapshot: ## Create a config snapshot
	./scripts/snapshot.sh

rollback: ## Rollback to last snapshot
	./scripts/rollback.sh
