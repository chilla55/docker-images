# PostgreSQL Single-Node Makefile
VERSION ?= latest
STACK_NAME ?= postgresql-single

.PHONY: help build push deploy down logs backup-full backup-incremental restore clean

help:
	@echo "PostgreSQL Single-Node Management"
	@echo "=================================="
	@echo ""
	@echo "Available commands:"
	@echo "  make build              - Build PostgreSQL single-node image"
	@echo "  make push               - Push image to registry"
	@echo "  make deploy             - Deploy PostgreSQL stack"
	@echo "  make down               - Remove PostgreSQL stack"
	@echo "  make logs               - Show PostgreSQL logs"
	@echo "  make backup-full        - Trigger immediate full backup"
	@echo "  make backup-incremental - Trigger immediate incremental backup"
	@echo "  make restore            - Restore from backup"
	@echo "  make clean              - Clean up old images"
	@echo ""

build:
	@echo "Building PostgreSQL single-node image..."
	docker build -t ghcr.io/chilla55/postgresql-single:$(VERSION) .
	@echo "✓ Build complete"

push: build
	@echo "Pushing image to registry..."
	docker push ghcr.io/chilla55/postgresql-single:$(VERSION)
	@echo "✓ Push complete"

deploy:
	@echo "Deploying PostgreSQL single-node stack..."
	docker stack deploy -c docker-compose.yml $(STACK_NAME)
	@echo "✓ Stack deployed"
	@echo ""
	@echo "To check status: docker service ls | grep postgresql"

down:
	@echo "Removing PostgreSQL stack..."
	docker stack rm $(STACK_NAME)
	@echo "✓ Stack removed"

logs:
	@docker service logs -f $(STACK_NAME)_postgresql

backup-full:
	@echo "Triggering full backup..."
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_postgresql) /usr/local/bin/backup-full.sh

backup-incremental:
	@echo "Triggering incremental backup..."
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_postgresql) /usr/local/bin/backup-incremental.sh

restore:
	@echo "Available backups:"
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_postgresql) /usr/local/bin/backup-restore.sh

clean:
	@echo "Cleaning up old images..."
	docker image prune -f
	@echo "✓ Cleanup complete"
