.PHONY: build push deploy remove label-nodes setup-replication logs-primary logs-secondary logs-pgpool status failover clean help

VERSION ?= latest
REGISTRY ?= local
IMAGE_NAME = postgresql-replication
STACK_NAME = postgresql-cluster

help:
	@echo "PostgreSQL Master-Slave Replication Makefile"
	@echo "============================================="
	@echo "build                 - Build the PostgreSQL replication Docker image"
	@echo "push                  - Push the image to registry"
	@echo "deploy                - Deploy the stack to Docker Swarm"
	@echo "remove                - Remove the stack from Docker Swarm"
	@echo "label-nodes           - Label swarm nodes for placement"
	@echo "setup-replication     - Setup replication user on Primary"
	@echo "logs-primary          - Show logs for Primary node"
	@echo "logs-secondary        - Show logs for Secondary node"
	@echo "logs-pgpool           - Show logs for PgPool"
	@echo "status                - Show replication status"
	@echo "clean                 - Remove images and volumes"

build:
	docker build -t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) .

push: build
	docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)

label-nodes:
	@echo "Labeling nodes for PostgreSQL placement..."
	@echo "Enter node name for Primary (node1):"
	@read node1; docker node update --label-add postgresql.node=node1 $$node1
	@echo "Enter node name for Secondary (node2):"
	@read node2; docker node update --label-add postgresql.node=node2 $$node2
	@echo "Enter node name for PgPool (node3 - manager):"
	@read node3; docker node update --label-add postgresql.node=node3 $$node3

deploy:
	docker stack deploy -c docker-compose.swarm.yml $(STACK_NAME)

remove:
	docker stack rm $(STACK_NAME)

setup-replication:
	@echo "Creating replication user on Primary..."
	@docker exec -it $$(docker ps -q -f name=$(STACK_NAME)_postgresql-primary) \
		psql -U postgres -c "CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD '${REPLICATION_PASSWORD}';" || true

logs-primary:
	docker service logs -f $(STACK_NAME)_postgresql-primary

logs-secondary:
	docker service logs -f $(STACK_NAME)_postgresql-secondary

logs-pgpool:
	docker service logs -f $(STACK_NAME)_pgpool

status:
	@echo "=== Service Status ==="
	@docker stack ps $(STACK_NAME)
	@echo ""
	@echo "=== Replication Status (Primary) ==="
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_postgresql-primary) \
		psql -U postgres -c "SELECT * FROM pg_stat_replication;" 2>/dev/null || echo "Not available"
	@echo ""
	@echo "=== Replication Status (Secondary) ==="
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_postgresql-secondary) \
		psql -U postgres -c "SELECT pg_is_in_recovery(), pg_last_wal_receive_lsn(), pg_last_wal_replay_lsn();" 2>/dev/null || echo "Not available"

clean:
	docker rmi $(REGISTRY)/$(IMAGE_NAME):$(VERSION) || true
	docker volume prune -f
