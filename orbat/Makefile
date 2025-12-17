# Orbat Next.js Application Makefile
.PHONY: build push deploy clean logs restart update help

# Configuration
IMAGE_NAME = ghcr.io/chilla55/orbat
VERSION = $(shell cat VERSION)
STACK_NAME = orbat

help:
	@echo "Orbat Next.js Application - Available targets:"
	@echo "  build     - Build the Docker image"
	@echo "  push      - Push image to registry"
	@echo "  deploy    - Deploy to Docker Swarm"
	@echo "  update    - Build, push, and deploy (full update)"
	@echo "  logs      - Show service logs"
	@echo "  restart   - Restart the service"
	@echo "  clean     - Remove local images"
	@echo "  version   - Show current version"

build:
	@echo "Building Orbat image $(IMAGE_NAME):$(VERSION)..."
	docker build -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest .

push:
	@echo "Pushing Orbat image $(IMAGE_NAME):$(VERSION)..."
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest

deploy:
	@echo "Deploying Orbat to Docker Swarm..."
	docker stack deploy -c docker-compose.swarm.yml $(STACK_NAME)

update: build push deploy
	@echo "Full update completed for Orbat"

logs:
	@echo "Showing Orbat logs..."
	docker service logs -f $(STACK_NAME)_orbat

restart:
	@echo "Restarting Orbat service..."
	docker service update --force $(STACK_NAME)_orbat

clean:
	@echo "Cleaning local Orbat images..."
	docker rmi $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):latest || true

version:
	@echo "Current version: $(VERSION)"

# Secret management helpers
create-secrets:
	@echo "Creating secrets (you'll be prompted for values)..."
	@echo "Enter NextAuth Secret (generate with: openssl rand -base64 32):"
	@read -s nextauth_secret && echo $$nextauth_secret | docker secret create nextauth_secret -
	@echo "Enter Discord Client ID:"
	@read -s discord_id && echo $$discord_id | docker secret create discord_client_id -
	@echo "Enter Discord Client Secret:"
	@read -s discord_secret && echo $$discord_secret | docker secret create discord_client_secret -
	@echo "Enter Steam API Key:"
	@read -s steam_key && echo $$steam_key | docker secret create steam_api_key -
	@echo "Enter Database Password:"
	@read -s db_password && echo $$db_password | docker secret create database_password -
	@echo "Secrets created successfully"

list-secrets:
	@echo "Listing Orbat-related secrets..."
	@docker secret ls | grep -E "nextauth|discord|steam|database"
