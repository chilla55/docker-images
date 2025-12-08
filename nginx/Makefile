.PHONY: help build push deploy clean version

NGINX_VERSION := $(shell cat VERSION 2>/dev/null || echo "1.29.0")
IMAGE_NAME := ghcr.io/chilla55/nginx
IMAGE_TAG := $(NGINX_VERSION)

help:
	@echo "Nginx Docker Image - Makefile Commands"
	@echo ""
	@echo "  make build          Build Docker image (version: $(NGINX_VERSION))"
	@echo "  make push           Push image to registry"
	@echo "  make deploy         Deploy to Docker Swarm"
	@echo "  make clean          Remove local image"
	@echo "  make version        Show current version"
	@echo ""

version:
	@echo "Nginx version: $(NGINX_VERSION)"

build:
	@echo "Building Nginx $(NGINX_VERSION)..."
	docker build \
		--build-arg NGINX_VERSION=$(NGINX_VERSION) \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):latest \
		.

push:
	@echo "Pushing $(IMAGE_NAME):$(IMAGE_TAG)..."
	docker push $(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(IMAGE_NAME):latest

deploy:
	@echo "Deploying Nginx stack to Swarm..."
	docker stack deploy -c docker-compose.swarm.yml nginx

clean:
	@echo "Removing local images..."
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) $(IMAGE_NAME):latest || true
