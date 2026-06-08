# ============================================================================
# Makefile for Pelican Panel Docker Management
# ============================================================================
# Usage: make [target]
# ============================================================================

.PHONY: help build push deploy secrets logs health scale clean upgrade rollback

# Variables
PANEL_VERSION ?= $(shell cat VERSION 2>/dev/null || echo "v1.0.0")
IMAGE_NAME ?= ghcr.io/chilla55/pelican-panel
IMAGE_TAG ?= $(PANEL_VERSION)
STACK_NAME ?= pelican
STACK_FILE ?= docker-compose.swarm.yml

# Colors for output
GREEN  := \033[0;32m
YELLOW := \033[1;33m
RED    := \033[0;31m
NC     := \033[0m # No Color

help: ## Show this help message
	@echo "$(GREEN)Pelican Panel Docker Management$(NC)"
	@echo ""
	@echo "$(YELLOW)Available targets:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-15s$(NC) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(YELLOW)Variables:$(NC)"
	@echo "  PANEL_VERSION  = $(PANEL_VERSION)"
	@echo "  IMAGE_NAME     = $(IMAGE_NAME)"
	@echo "  STACK_NAME     = $(STACK_NAME)"

build: ## Build Docker image locally
	@echo "$(GREEN)Building image $(IMAGE_NAME):$(IMAGE_TAG)...$(NC)"
	docker build \
		--build-arg PANEL_VERSION=$(PANEL_VERSION) \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):latest \
		.
	@echo "$(GREEN)✓ Build complete$(NC)"

build-multi: ## Build multi-platform image (amd64, arm64)
	@echo "$(GREEN)Building multi-platform image...$(NC)"
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg PANEL_VERSION=$(PANEL_VERSION) \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):latest \
		.
	@echo "$(GREEN)✓ Multi-platform build complete$(NC)"

push: ## Push image to registry
	@echo "$(GREEN)Pushing $(IMAGE_NAME):$(IMAGE_TAG)...$(NC)"
	docker push $(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(IMAGE_NAME):latest
	@echo "$(GREEN)✓ Push complete$(NC)"

secrets-create: ## Create Docker secrets (interactive)
	@echo "$(YELLOW)Creating Docker secrets...$(NC)"
	@echo "$(RED)WARNING: This will prompt for sensitive data$(NC)"
	@read -p "Enter APP_KEY (or press Enter to generate): " APP_KEY; \
	if [ -z "$$APP_KEY" ]; then \
		APP_KEY=$$(docker run --rm $(IMAGE_NAME):$(IMAGE_TAG) php artisan key:generate --show); \
		echo "Generated: $$APP_KEY"; \
	fi; \
	echo "$$APP_KEY" | docker secret create pelican_app_key - || true
	@read -sp "Enter DB password: " DB_PASS; echo ""; \
	echo "$$DB_PASS" | docker secret create pelican_db_password - || true
	@read -sp "Enter Mail password (optional): " MAIL_PASS; echo ""; \
	[ -n "$$MAIL_PASS" ] && echo "$$MAIL_PASS" | docker secret create vaultwarden_smtp_password - || true
	@echo "$(GREEN)✓ Secrets created$(NC)"

secrets-list: ## List all Pelican secrets
	@echo "$(GREEN)Pelican Secrets:$(NC)"
	@docker secret ls | grep pelican || echo "No secrets found"

deploy: ## Deploy stack to Swarm
	@echo "$(GREEN)Deploying stack $(STACK_NAME)...$(NC)"
	docker stack deploy -c $(STACK_FILE) $(STACK_NAME)
	@echo "$(GREEN)✓ Stack deployed (migrations will run automatically)$(NC)"
	@echo "$(YELLOW)Monitor with: make logs$(NC)"

logs: ## Follow all service logs
	docker service logs -f $(STACK_NAME)_php-fpm

logs-caddy: ## Follow caddy logs
	docker service logs -f $(STACK_NAME)_caddy

logs-queue: ## Follow queue logs
	docker service logs -f $(STACK_NAME)_queue

logs-tail: ## Show last 100 log lines
	docker service logs --tail 100 $(STACK_NAME)_php-fpm

health: ## Check service health
	@echo "$(GREEN)Service Status:$(NC)"
	@docker service ls | grep $(STACK_NAME) || echo "Stack not running"
	@echo ""
	@echo "$(GREEN)Container Health:$(NC)"
	@docker ps --filter "name=$(STACK_NAME)" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

scale: ## Scale php-fpm service (usage: make scale REPLICAS=2)
	@echo "$(GREEN)Scaling $(STACK_NAME)_php-fpm to $(REPLICAS) replicas...$(NC)"
	docker service scale $(STACK_NAME)_php-fpm=$(REPLICAS)

ps: ## Show service tasks
	docker service ps $(STACK_NAME)_php-fpm

inspect: ## Inspect php-fpm service details
	docker service inspect $(STACK_NAME)_php-fpm --pretty

remove: ## Remove stack
	@echo "$(RED)Removing stack $(STACK_NAME)...$(NC)"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo ""; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker stack rm $(STACK_NAME); \
		echo "$(GREEN)✓ Stack removed$(NC)"; \
	else \
		echo "$(YELLOW)Cancelled$(NC)"; \
	fi

clean: ## Clean up stopped containers and unused images
	@echo "$(GREEN)Cleaning up...$(NC)"
	docker system prune -f
	@echo "$(GREEN)✓ Cleanup complete$(NC)"

exec: ## Execute command in php-fpm container (usage: make exec CMD="php artisan migrate:status")
	@CONTAINER_ID=$$(docker ps -q -f name=$(STACK_NAME)_php-fpm | head -1); \
	if [ -z "$$CONTAINER_ID" ]; then \
		echo "$(RED)No running container found$(NC)"; \
		exit 1; \
	fi; \
	docker exec -it $$CONTAINER_ID $(CMD)

shell: ## Open shell in php-fpm container
	@CONTAINER_ID=$$(docker ps -q -f name=$(STACK_NAME)_php-fpm | head -1); \
	if [ -z "$$CONTAINER_ID" ]; then \
		echo "$(RED)No running container found$(NC)"; \
		exit 1; \
	fi; \
	docker exec -it $$CONTAINER_ID sh

upgrade: ## Upgrade to new version (usage: make upgrade NEW_VERSION=v1.0.1)
	@if [ -z "$(NEW_VERSION)" ]; then \
		echo "$(RED)Error: NEW_VERSION not specified$(NC)"; \
		echo "Usage: make upgrade NEW_VERSION=v1.0.1"; \
		exit 1; \
	fi
	@echo "$(YELLOW)Upgrading from $(PANEL_VERSION) to $(NEW_VERSION)...$(NC)"
	@echo "$(RED)⚠️  Make sure you have a database backup!$(NC)"
	@read -p "Continue? [y/N] " -n 1 -r; \
	echo ""; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "$(NEW_VERSION)" > VERSION; \
		$(MAKE) deploy PANEL_VERSION=$(NEW_VERSION); \
		echo "$(GREEN)✓ Upgrade complete (migrations ran automatically)$(NC)"; \
	else \
		echo "$(YELLOW)Cancelled$(NC)"; \
	fi

rollback: ## Rollback php-fpm to previous version
	@echo "$(YELLOW)Rolling back service...$(NC)"
	docker service rollback $(STACK_NAME)_php-fpm
	@echo "$(GREEN)✓ Rollback initiated$(NC)"

backup-db: ## Backup database (requires DB_PASS)
	@echo "$(GREEN)Creating database backup...$(NC)"
	@TIMESTAMP=$$(date +%Y%m%d_%H%M%S); \
	docker exec mariadb mysqldump -u pelican -p$(DB_PASS) pelican > pelican_backup_$$TIMESTAMP.sql && \
	echo "$(GREEN)✓ Backup saved: pelican_backup_$$TIMESTAMP.sql$(NC)" || \
	echo "$(RED)✗ Backup failed$(NC)"

test: ## Run healthcheck manually in php-fpm container
	@CONTAINER_ID=$$(docker ps -q -f name=$(STACK_NAME)_php-fpm | head -1); \
	if [ -z "$$CONTAINER_ID" ]; then \
		echo "$(RED)No running container found$(NC)"; \
		exit 1; \
	fi; \
	echo "$(GREEN)Running healthcheck...$(NC)"; \
	docker exec $$CONTAINER_ID /healthcheck.sh && \
	echo "$(GREEN)✓ Healthcheck passed$(NC)" || \
	echo "$(RED)✗ Healthcheck failed$(NC)"

validate: ## Validate deployment prerequisites
	@bash validate-env.sh

networks: ## Create required networks
	@echo "$(GREEN)Creating networks...$(NC)"
	docker network create --driver overlay web-net || true
	docker network create --driver overlay mariadb-net || true
	docker network create --driver overlay redis-net || true
	@echo "$(GREEN)✓ Networks ready$(NC)"

.DEFAULT_GOAL := help
