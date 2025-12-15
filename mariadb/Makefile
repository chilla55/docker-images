# MariaDB Single-Node Makefile
VERSION ?= latest
STACK_NAME ?= mariadb-single

.PHONY: help build push deploy down logs backup-full backup-incremental restore clean

help:
	@echo "MariaDB Single-Node Management"
	@echo "==============================="
	@echo ""
	@echo "Available commands:"
	@echo "  make build              - Build MariaDB single-node image"
	@echo "  make push               - Push image to registry"
	@echo "  make deploy             - Deploy MariaDB stack"
	@echo "  make down               - Remove MariaDB stack"
	@echo "  make logs               - Show MariaDB logs"
	@echo "  make backup-full        - Trigger immediate full backup"
	@echo "  make backup-incremental - Trigger immediate incremental backup"
	@echo "  make restore            - Restore from backup"
	@echo "  make clean              - Clean up old images"
	@echo ""

build:
	@echo "Building MariaDB single-node image..."
	docker build -f Dockerfile.single -t ghcr.io/chilla55/mariadb-single:$(VERSION) .
	@echo "✓ Build complete"

push: build
	@echo "Pushing image to registry..."
	docker push ghcr.io/chilla55/mariadb-single:$(VERSION)
	@echo "✓ Push complete"

deploy:
	@echo "Deploying MariaDB single-node stack..."
	docker stack deploy -c docker-compose.single.yml $(STACK_NAME)
	@echo "✓ Stack deployed"
	@echo ""
	@echo "To check status: docker service ls | grep mariadb"

down:
	@echo "Removing MariaDB stack..."
	docker stack rm $(STACK_NAME)
	@echo "✓ Stack removed"

logs:
	@docker service logs -f $(STACK_NAME)_mariadb

backup-full:
	@echo "Triggering full backup..."
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_mariadb) /usr/local/bin/backup-full.sh

backup-incremental:
	@echo "Triggering incremental backup..."
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_mariadb) /usr/local/bin/backup-incremental.sh

restore:
	@echo "Available backups:"
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_mariadb) /usr/local/bin/backup-restore.sh

clean:
	@echo "Cleaning up old images..."
	docker image prune -f
	@echo "✓ Cleanup complete"
