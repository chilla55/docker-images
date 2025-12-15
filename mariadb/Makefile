.PHONY: build push deploy clean help

VERSION ?= latest
REGISTRY := ghcr.io
IMAGE_NAME := chilla55/mariadb-single
FULL_IMAGE := $(REGISTRY)/$(IMAGE_NAME):$(VERSION)

help:
	@echo "MariaDB Docker Image - Makefile Commands"
	@echo "========================================="
	@echo "make build          - Build the Docker image"
	@echo "make push           - Push the image to registry"
	@echo "make deploy         - Deploy to Docker Swarm"
	@echo "make clean          - Remove local images"
	@echo "make logs           - View service logs"
	@echo "make backup-full    - Trigger immediate full backup"
	@echo "make backup-incr    - Trigger immediate incremental backup"
	@echo "make backup-list    - List available backups"
	@echo "make backup-restore - Restore from latest backup"
	@echo ""

build:
	@echo "Building MariaDB image $(FULL_IMAGE)..."
	docker build -t $(FULL_IMAGE) .
	docker tag $(FULL_IMAGE) $(REGISTRY)/$(IMAGE_NAME):latest

push:
	@echo "Pushing $(FULL_IMAGE)..."
	docker push $(FULL_IMAGE)
	docker push $(REGISTRY)/$(IMAGE_NAME):latest

deploy:
	@echo "Deploying MariaDB service to Swarm..."
	docker stack deploy -c docker-compose.swarm.yml mariadb

update:
	@echo "Updating MariaDB service..."
	docker service update --image $(FULL_IMAGE) mariadb_mariadb

remove:
	@echo "Removing MariaDB stack..."
	docker stack rm mariadb

logs:
	@echo "Fetching logs for MariaDB service..."
	docker service logs -f mariadb_mariadb

ps:
	@echo "MariaDB service status:"
	docker service ps mariadb_mariadb

clean:
	@echo "Removing local MariaDB images..."
	docker rmi $(FULL_IMAGE) || true
	docker rmi $(REGISTRY)/$(IMAGE_NAME):latest || true

backup-full:
	@echo "Triggering immediate full backup..."
	@CONTAINER=$$(docker ps -q -f name=mariadb_mariadb); \
	if [ -z "$$CONTAINER" ]; then \
		echo "ERROR: MariaDB container not found"; \
		exit 1; \
	fi; \
	docker exec $$CONTAINER /usr/local/bin/backup-full.sh

backup-incr:
	@echo "Triggering immediate incremental backup..."
	@CONTAINER=$$(docker ps -q -f name=mariadb_mariadb); \
	if [ -z "$$CONTAINER" ]; then \
		echo "ERROR: MariaDB container not found"; \
		exit 1; \
	fi; \
	docker exec $$CONTAINER /usr/local/bin/backup-incremental.sh

backup-list:
	@echo "Listing available backups..."
	@CONTAINER=$$(docker ps -q -f name=mariadb_mariadb); \
	if [ -z "$$CONTAINER" ]; then \
		echo "ERROR: MariaDB container not found"; \
		exit 1; \
	fi; \
	docker exec $$CONTAINER /usr/local/bin/backup-restore.sh list

backup-restore:
	@echo "Restoring from latest backup..."
	@echo "WARNING: This will overwrite existing data!"
	@read -p "Are you sure? (y/N): " confirm; \
	if [ "$$confirm" = "y" ] || [ "$$confirm" = "Y" ]; then \
		CONTAINER=$$(docker ps -q -f name=mariadb_mariadb); \
		if [ -z "$$CONTAINER" ]; then \
			echo "ERROR: MariaDB container not found"; \
			exit 1; \
		fi; \
		docker exec $$CONTAINER /usr/local/bin/backup-restore.sh restore-latest; \
	else \
		echo "Restore cancelled"; \
	fi

shell:
	@CONTAINER=$$(docker ps -q -f name=mariadb_mariadb); \
	if [ -z "$$CONTAINER" ]; then \
		echo "ERROR: MariaDB container not found"; \
		exit 1; \
	fi; \
	docker exec -it $$CONTAINER bash
