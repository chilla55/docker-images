# ============================================================================
# Makefile for Pelican Panel Overlay (Official Image + Registry Agent)
# ============================================================================

.PHONY: help build build-multi push deploy logs logs-tail health ps inspect remove clean exec shell rollback networks

# Variables
PANEL_VERSION ?= $(shell cat VERSION 2>/dev/null || echo "v1.0.0")
IMAGE_NAME ?= ghcr.io/chilla55/pelican-panel
IMAGE_TAG ?= $(PANEL_VERSION)
STACK_NAME ?= pelican
STACK_FILE ?= docker-compose.swarm.yml
SERVICE_NAME ?= panel

# Colors for output
GREEN  := \033[0;32m
YELLOW := \033[1;33m
RED    := \033[0;31m
NC     := \033[0m

help: ## Show this help message
	@echo "$(GREEN)Pelican Panel Overlay Management$(NC)"
	@echo ""
	@echo "$(YELLOW)Available targets:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-15s$(NC) %s\n", $$1, $$2}'

build: ## Build Docker image locally
	@echo "$(GREEN)Building image $(IMAGE_NAME):$(IMAGE_TAG)...$(NC)"
	docker build \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):latest \
		.
	@echo "$(GREEN)✓ Build complete$(NC)"

build-multi: ## Build multi-platform image (amd64, arm64)
	@echo "$(GREEN)Building multi-platform image...$(NC)"
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):latest \
		.
	@echo "$(GREEN)✓ Multi-platform build complete$(NC)"

push: ## Push image to registry
	@echo "$(GREEN)Pushing $(IMAGE_NAME):$(IMAGE_TAG)...$(NC)"
	docker push $(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(IMAGE_NAME):latest
	@echo "$(GREEN)✓ Push complete$(NC)"

deploy: ## Deploy stack to Swarm
	@echo "$(GREEN)Deploying stack $(STACK_NAME)...$(NC)"
	docker stack deploy -c $(STACK_FILE) $(STACK_NAME)
	@echo "$(GREEN)✓ Stack deployed$(NC)"
	@echo "$(YELLOW)Monitor with: make logs$(NC)"

logs: ## Follow service logs
	docker service logs -f $(STACK_NAME)_$(SERVICE_NAME)

logs-tail: ## Show last 100 log lines
	docker service logs --tail 100 $(STACK_NAME)_$(SERVICE_NAME)

health: ## Check service and container health
	@echo "$(GREEN)Service Status:$(NC)"
	@docker service ls | grep $(STACK_NAME) || echo "Stack not running"
	@echo ""
	@echo "$(GREEN)Container Health:$(NC)"
	@docker ps --filter "name=$(STACK_NAME)_$(SERVICE_NAME)" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

ps: ## Show service tasks
	docker service ps $(STACK_NAME)_$(SERVICE_NAME)

inspect: ## Inspect service details
	docker service inspect $(STACK_NAME)_$(SERVICE_NAME) --pretty

remove: ## Remove stack
	@echo "$(RED)Removing stack $(STACK_NAME)...$(NC)"
	docker stack rm $(STACK_NAME)

clean: ## Clean up stopped containers and unused images
	@echo "$(GREEN)Cleaning up...$(NC)"
	docker system prune -f
	@echo "$(GREEN)✓ Cleanup complete$(NC)"

exec: ## Execute command in service container (usage: make exec CMD="php artisan about")
	@CONTAINER_ID=$$(docker ps -q -f name=$(STACK_NAME)_$(SERVICE_NAME) | head -1); \
	if [ -z "$$CONTAINER_ID" ]; then \
		echo "$(RED)No running container found$(NC)"; \
		exit 1; \
	fi; \
	docker exec -it $$CONTAINER_ID $(CMD)

shell: ## Open shell in service container
	@CONTAINER_ID=$$(docker ps -q -f name=$(STACK_NAME)_$(SERVICE_NAME) | head -1); \
	if [ -z "$$CONTAINER_ID" ]; then \
		echo "$(RED)No running container found$(NC)"; \
		exit 1; \
	fi; \
	docker exec -it $$CONTAINER_ID sh

rollback: ## Rollback service to previous version
	docker service rollback $(STACK_NAME)_$(SERVICE_NAME)

networks: ## Create required overlay networks
	@echo "$(GREEN)Creating networks...$(NC)"
	docker network create --driver overlay web-net || true
	docker network create --driver overlay mariadb-net || true
	docker network create --driver overlay redis-net || true
	@echo "$(GREEN)✓ Networks ready$(NC)"

.DEFAULT_GOAL := help
