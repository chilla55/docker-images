.PHONY: build push deploy remove label-nodes setup-replication create-maxscale-user logs-primary logs-secondary logs-maxscale status failover switchover clean help

VERSION ?= latest
REGISTRY ?= local
IMAGE_NAME = mariadb-replication
STACK_NAME = mariadb-cluster

help:
	@echo "MariaDB Master-Slave Replication Makefile"
	@echo "=========================================="
	@echo "build                 - Build the MariaDB replication Docker image"
	@echo "push                  - Push the image to registry"
	@echo "deploy                - Deploy the stack to Docker Swarm"
	@echo "remove                - Remove the stack from Docker Swarm"
	@echo "label-nodes           - Label swarm nodes for placement"
	@echo "setup-replication     - Setup replication on Secondary"
	@echo "create-maxscale-user  - Create MaxScale monitoring user"
	@echo "logs-primary          - Show logs for Primary node"
	@echo "logs-secondary        - Show logs for Secondary node"
	@echo "logs-maxscale         - Show logs for MaxScale"
	@echo "status                - Show replication status"
	@echo "failover              - Trigger manual failover"
	@echo "switchover            - Trigger graceful switchover"
	@echo "clean                 - Remove images and volumes"

build:
	docker build -t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) .

push: build
	docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)

label-nodes:
	@echo "Labeling nodes for MariaDB placement..."
	@echo "Enter node name for Primary (node1):"
	@read node1; docker node update --label-add mariadb.node=node1 $$node1
	@echo "Enter node name for Secondary (node2):"
	@read node2; docker node update --label-add mariadb.node=node2 $$node2
	@echo "Enter node name for MaxScale (node3 - manager):"
	@read node3; docker node update --label-add mariadb.node=node3 $$node3

deploy:
	docker stack deploy -c docker-compose.swarm.yml $(STACK_NAME)

remove:
	docker stack rm $(STACK_NAME)

setup-replication:
	@echo "Setting up replication..."
	@echo "Step 1: Creating replication user on Primary"
	@docker exec -it $$(docker ps -q -f name=$(STACK_NAME)_mariadb-primary) mysql -u root -p -e \
		"CREATE USER IF NOT EXISTS 'replicator'@'%' IDENTIFIED BY '${REPLICATION_PASSWORD}'; \
		GRANT REPLICATION SLAVE ON *.* TO 'replicator'@'%'; \
		FLUSH PRIVILEGES; \
		SHOW MASTER STATUS;"
	@echo ""
	@echo "Step 2: Copy the File and Position from above, then run:"
	@echo "docker exec -it \$$(docker ps -q -f name=$(STACK_NAME)_mariadb-secondary) mysql -u root -p"
	@echo "Then execute:"
	@echo "CHANGE MASTER TO MASTER_HOST='mariadb-primary', MASTER_USER='replicator', MASTER_PASSWORD='${REPLICATION_PASSWORD}', MASTER_LOG_FILE='<File>', MASTER_LOG_POS=<Position>;"
	@echo "START SLAVE;"
	@echo "SHOW SLAVE STATUS\G"

create-maxscale-user:
	@echo "Creating MaxScale monitoring user..."
	@docker exec -it $$(docker ps -q -f name=$(STACK_NAME)_mariadb-primary) mysql -u root -p -e \
		"CREATE USER IF NOT EXISTS 'maxscale'@'%' IDENTIFIED BY '${MAXSCALE_PASSWORD}'; \
		GRANT SELECT ON mysql.user TO 'maxscale'@'%'; \
		GRANT SELECT ON mysql.db TO 'maxscale'@'%'; \
		GRANT SELECT ON mysql.tables_priv TO 'maxscale'@'%'; \
		GRANT SELECT ON mysql.columns_priv TO 'maxscale'@'%'; \
		GRANT SHOW DATABASES ON *.* TO 'maxscale'@'%'; \
		GRANT REPLICATION CLIENT ON *.* TO 'maxscale'@'%'; \
		GRANT REPLICATION SLAVE ON *.* TO 'maxscale'@'%'; \
		GRANT SUPER ON *.* TO 'maxscale'@'%'; \
		GRANT RELOAD ON *.* TO 'maxscale'@'%'; \
		GRANT PROCESS ON *.* TO 'maxscale'@'%'; \
		GRANT CREATE ON *.* TO 'maxscale'@'%'; \
		GRANT EVENT ON *.* TO 'maxscale'@'%'; \
		FLUSH PRIVILEGES;"

logs-primary:
	docker service logs -f $(STACK_NAME)_mariadb-primary

logs-secondary:
	docker service logs -f $(STACK_NAME)_mariadb-secondary

logs-maxscale:
	docker service logs -f $(STACK_NAME)_maxscale

status:
	@echo "=== Service Status ==="
	@docker stack ps $(STACK_NAME)
	@echo ""
	@echo "=== Replication Status (Secondary) ==="
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_mariadb-secondary) \
		mysql -u root -p${MYSQL_ROOT_PASSWORD} -e "SHOW SLAVE STATUS\G" 2>/dev/null | grep -E "Slave_IO_Running|Slave_SQL_Running|Seconds_Behind_Master|Last_Error" || true
	@echo ""
	@echo "=== MaxScale Status ==="
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_maxscale) maxctrl list servers 2>/dev/null || echo "MaxScale not ready"

failover:
	@echo "Triggering manual failover..."
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_maxscale) maxctrl call command mariadbmon failover MariaDB-Monitor

switchover:
	@echo "Triggering graceful switchover..."
	@docker exec $$(docker ps -q -f name=$(STACK_NAME)_maxscale) maxctrl call command mariadbmon switchover MariaDB-Monitor

clean:
	docker rmi $(REGISTRY)/$(IMAGE_NAME):$(VERSION) || true
	docker volume prune -f
