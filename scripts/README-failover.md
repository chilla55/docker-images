# Web Label Failover Script

Automatically adds `web.node=web` label to `mail` when `srv1` goes offline, allowing web services to failover.

## How It Works

1. **Monitors srv1 status** every 10 seconds
2. **When srv1 goes offline**: Adds `web.node=web` to mail → web services migrate to mail
3. **When srv1 comes back**: Removes `web.node=web` from mail → services migrate back to srv1

## Installation

### Option 1: Systemd Service (Recommended)

Run on a manager node (srv2 or mail):

```bash
# Copy script
sudo cp failover-web-label.sh /usr/local/bin/
sudo chmod +x /usr/local/bin/failover-web-label.sh

# Install systemd service
sudo cp failover-web-label.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable failover-web-label.service
sudo systemctl start failover-web-label.service

# Check status
sudo systemctl status failover-web-label.service
sudo journalctl -u failover-web-label.service -f
```

### Option 2: Docker Service

Deploy as a global service on manager nodes:

```yaml
version: '3.8'

services:
  web-failover:
    image: docker:cli
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./failover-web-label.sh:/failover.sh:ro
    command: /failover.sh
    deploy:
      mode: global
      placement:
        constraints:
          - node.role == manager
      restart_policy:
        condition: any
        delay: 10s
```

Deploy:
```bash
docker stack deploy -c failover-compose.yml failover
```

### Option 3: Manual Execution

Run manually on manager:

```bash
chmod +x failover-web-label.sh
./failover-web-label.sh
```

## Configuration

Edit the script to customize:

```bash
SRV1_HOSTNAME="srv1"      # Primary web node
MAIL_HOSTNAME="mail"      # Failover web node
CHECK_INTERVAL=10         # Check frequency in seconds
```

## Testing

### Test Failover

```bash
# On srv1 - simulate failure
sudo systemctl stop docker

# Check on manager - mail should get web label within 10s
docker node inspect mail --format '{{.Spec.Labels}}'
# Should show: map[mariadb.node:mail postgresql.node:mail web.node:web]

# Check service migration
docker service ps nginx_nginx
# Should show tasks migrating to mail
```

### Test Failback

```bash
# On srv1 - bring back online
sudo systemctl start docker

# Check on manager - web label removed from mail within ~20s
docker node inspect mail --format '{{.Spec.Labels}}'
# Should show: map[mariadb.node:mail postgresql.node:mail]

# Services migrate back to srv1
docker service ps nginx_nginx
```

## Monitoring

```bash
# View logs (systemd)
sudo journalctl -u failover-web-label.service -f

# View logs (Docker service)
docker service logs -f failover_web-failover

# Check current labels
docker node ls --format '{{.Hostname}}: {{.Status}} {{.ManagerStatus}}'
docker node inspect srv1 --format '{{.Spec.Labels}}'
docker node inspect mail --format '{{.Spec.Labels}}'
```

## Troubleshooting

### Script not detecting srv1 offline

Check if hostnames match:
```bash
docker node ls
# Adjust SRV1_HOSTNAME in script if different
```

### Labels not updating

Ensure script runs on a manager node:
```bash
docker node inspect self --format '{{.ManagerStatus.Leader}}'
```

### Services not migrating

Check service placement constraints:
```bash
docker service inspect nginx_nginx --format '{{json .Spec.TaskTemplate.Placement}}' | jq
```

Should have:
```json
{
  "Constraints": ["node.labels.web.node == web"]
}
```

## Notes

- **Automatic failover**: Happens within 10 seconds of srv1 going offline
- **Automatic failback**: Happens within 20 seconds of srv1 coming back online
- **Service migration**: Docker Swarm automatically reschedules services based on label changes
- **Zero downtime**: Services stay running during migration (rolling update)

## Advanced: Multiple Failover Targets

To support multiple failover nodes, modify the script:

```bash
FAILOVER_NODES=("mail" "srv2")

for node in "${FAILOVER_NODES[@]}"; do
    if [ "$SRV1_STATUS" != "ready" ]; then
        docker node update --label-add web.node=web "$node"
    else
        docker node update --label-rm web.node "$node"
    fi
done
```

This adds web label to both mail and srv2 when srv1 fails, distributing load.
