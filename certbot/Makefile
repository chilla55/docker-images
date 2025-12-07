.PHONY: build push test deploy remove logs help

# ============================================================================
# Configuration
# ============================================================================
IMAGE_NAME := ghcr.io/chilla55/certbot-storagebox
VERSION := $(shell cat VERSION 2>/dev/null || echo "latest")
STACK_NAME := certbot

# ============================================================================
# Build & Push
# ============================================================================

## build: Build the Docker image
build:
	@echo "Building certbot image..."
	docker build -t $(IMAGE_NAME):$(VERSION) .
	docker tag $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):latest
	@echo "✓ Image built: $(IMAGE_NAME):$(VERSION)"

## push: Push the Docker image to registry
push: build
	@echo "Pushing image to registry..."
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest
	@echo "✓ Image pushed: $(IMAGE_NAME):$(VERSION)"

# ============================================================================
# Setup & Testing
# ============================================================================

## setup: Setup Docker secrets for certbot
setup:
	@echo "Running setup script..."
	@bash setup.sh

## test-cifs: Test CIFS connection to Storage Box (requires sudo)
test-cifs:
	@echo "Testing Storage Box CIFS connection..."
	@sudo bash test-storagebox.sh

## test: Run all tests
test: test-cifs
	@echo "✓ All tests passed"

# ============================================================================
# Deployment
# ============================================================================

## deploy: Deploy the certbot stack
deploy:
	@echo "Deploying certbot stack..."
	docker stack deploy -c docker-compose.swarm.yml $(STACK_NAME)
	@echo "✓ Stack deployed: $(STACK_NAME)"
	@echo ""
	@echo "Check status with: docker stack ps $(STACK_NAME)"
	@echo "Check logs with: docker service logs -f $(STACK_NAME)_certbot"

## remove: Remove the certbot stack
remove:
	@echo "Removing certbot stack..."
	docker stack rm $(STACK_NAME)
	@echo "✓ Stack removed: $(STACK_NAME)"

## restart: Restart the certbot service
restart:
	@echo "Restarting certbot service..."
	docker service update --force $(STACK_NAME)_certbot
	@echo "✓ Service restarted"

# ============================================================================
# Monitoring
# ============================================================================

## logs: Show certbot logs
logs:
	docker service logs -f $(STACK_NAME)_certbot

## status: Show stack status
status:
	@echo "Stack services:"
	@docker stack services $(STACK_NAME)
	@echo ""
	@echo "Service tasks:"
	@docker stack ps $(STACK_NAME)

## inspect: Show detailed service information
inspect:
	docker service inspect $(STACK_NAME)_certbot --pretty

# ============================================================================
# Maintenance
# ============================================================================

## manual-renew: Manually trigger certificate renewal
manual-renew:
	@echo "Manually triggering certificate renewal..."
	@CONTAINER_ID=$$(docker ps --filter "name=$(STACK_NAME)_certbot" --format "{{.ID}}" | head -n 1); \
	if [ -n "$$CONTAINER_ID" ]; then \
		docker exec $$CONTAINER_ID certbot renew --force-renewal; \
	else \
		echo "Error: Certbot container not found"; \
		exit 1; \
	fi

## manual-sync: Not applicable with CIFS mount (certs written directly)
manual-sync:
	@echo "Note: With CIFS mount, certificates are written directly to Storage Box"
	@echo "No manual sync needed - changes are immediate"

## shell: Open a shell in the certbot container
shell:
	@CONTAINER_ID=$$(docker ps --filter "name=$(STACK_NAME)_certbot" --format "{{.ID}}" | head -n 1); \
	if [ -n "$$CONTAINER_ID" ]; then \
		docker exec -it $$CONTAINER_ID /bin/bash; \
	else \
		echo "Error: Certbot container not found"; \
		exit 1; \
	fi

# ============================================================================
# Cleanup
# ============================================================================

## clean-secrets: Remove all certbot-related secrets
clean-secrets:
	@echo "Removing certbot secrets..."
	@docker secret rm cloudflare_credentials 2>/dev/null || true
	@docker secret rm rclone_config 2>/dev/null || true
	@echo "✓ Secrets removed"
## clean-secrets: Remove all certbot-related secrets
clean-secrets:
	@echo "Removing certbot secrets..."
	@docker secret rm cloudflare_credentials 2>/dev/null || true
	@docker secret rm storagebox_password 2>/dev/null || true
	@echo "✓ Secrets removed"AME):latest 2>/dev/null || true
	@echo "✓ Cleanup complete"

# ============================================================================
# Help
# ============================================================================

## help: Show this help message
help:
	@echo "Certbot with Hetzner Storage Box - Makefile Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.DEFAULT_GOAL := help
