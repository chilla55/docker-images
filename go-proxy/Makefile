.PHONY: build run test clean docker-build docker-run help

# Variables
BINARY_NAME=proxy-manager
DOCKER_IMAGE=proxy-manager
VERSION?=latest
GO_FILES=$(shell find nginx-manager -name '*.go')

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary locally
	@echo "Building $(BINARY_NAME)..."
	cd nginx-manager && go build -ldflags="-w -s" -o ../$(BINARY_NAME) .
	@echo "Build complete: $(BINARY_NAME)"

run: build ## Build and run locally
	@echo "Starting $(BINARY_NAME)..."
	./$(BINARY_NAME) \
		--sites-path=./sites-available \
		--global-config=./global.yaml \
		--debug

test: ## Run tests
	cd nginx-manager && go test -v ./...

fmt: ## Format code
	cd nginx-manager && go fmt ./...

vet: ## Run go vet
	cd nginx-manager && go vet ./...

lint: ## Run golangci-lint
	cd nginx-manager && golangci-lint run

docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_IMAGE):$(VERSION)..."
	docker build -t $(DOCKER_IMAGE):$(VERSION) .
	@echo "Docker build complete"

docker-run: ## Run Docker container
	@echo "Running $(DOCKER_IMAGE):$(VERSION)..."
	docker run --rm \
		-p 80:80 \
		-p 443:443/tcp \
		-p 443:443/udp \
		-p 81:81 \
		-p 8080:8080 \
		-v $(PWD)/sites-available:/etc/proxy/sites-available:ro \
		-v $(PWD)/global.yaml:/etc/proxy/global.yaml:ro \
		-v $(PWD)/certs:/etc/proxy/certs:ro \
		-e DEBUG=1 \
		$(DOCKER_IMAGE):$(VERSION)

docker-push: ## Push Docker image to registry
	docker push $(DOCKER_IMAGE):$(VERSION)

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f nginx-manager/$(BINARY_NAME)
	@echo "Clean complete"

deps: ## Download dependencies
	cd nginx-manager && go mod download

tidy: ## Tidy go.mod
	cd nginx-manager && go mod tidy

gen-test-cert: ## Generate self-signed test certificate (usage: make gen-test-cert DOMAIN=test.local)
	@./generate-test-cert.sh $(or $(DOMAIN),test.local)

example-site: ## Create example site config
	@mkdir -p sites-available
	@cp example-simple.yaml sites-available/
	@echo "Created sites-available/example-simple.yaml"

validate-yaml: ## Validate all YAML configs
	@echo "Validating YAML configs..."
	@for file in sites-available/*.yaml global.yaml; do \
		if [ -f "$$file" ]; then \
			echo "Checking $$file..."; \
			yamllint -d relaxed "$$file" || true; \
		fi \
	done

logs: ## Show Docker container logs
	docker logs -f proxy-manager

health: ## Check health endpoint
	@curl -f http://localhost:8080/health && echo "" || echo "Health check failed"

metrics: ## Show metrics
	@curl -s http://localhost:8080/metrics

all: clean deps build ## Clean, download deps, and build

.DEFAULT_GOAL := help
