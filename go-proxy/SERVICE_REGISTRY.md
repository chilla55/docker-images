# Service Registry Guide

Complete guide for integrating applications with the go-proxy service registry for dynamic route management.

## Overview

The service registry allows backend applications to dynamically register and deregister routes without editing configuration files or restarting the proxy. This is ideal for:

- **Microservices** - Services register themselves on startup
- **Auto-scaling** - New instances automatically get routed
- **Blue-Green Deployments** - Switch traffic programmatically
- **Canary Releases** - Gradually roll out new versions
- **Maintenance Mode** - Temporarily disable routes

**Registry Port:** `81` (TCP protocol)

---

## TCP Protocol

The service registry uses a **TCP-based protocol** on port 81 with persistent connections and session management.

**Why TCP?**
- Persistent connections enable automatic cleanup when services die
- Session-based connection tracking for reconnection support
- Real-time maintenance mode coordination
- Lower overhead than HTTP for high-frequency updates
- Connection monitoring detects crashed services automatically

**Port:** `81`

### Protocol Format

Commands are newline-terminated strings with pipe-separated fields:

```
COMMAND|field1|field2|...\n
```

All responses are also newline-terminated.

---

## Commands Reference

### REGISTER

Establish a persistent connection and register a service.

**Format:**
```
REGISTER|service_name|hostname|service_port|maintenance_port\n
```

**Parameters:**
- `service_name` - Logical name for the service (e.g., "api-service")
- `hostname` - Service hostname/container name (e.g., "api-v2")
- `service_port` - Port where the service listens (e.g., 9000)
- `maintenance_port` - Port for maintenance/health checks (e.g., 9001)

**Response:**
```
ACK|session_id\n
```

The `session_id` is used for all subsequent commands and reconnection.

**Example:**
```
REGISTER|api-service|api-v2|9000|9001\n
→ ACK|api-v2-9000-1734532800\n
```

**Notes:**
- Connection remains open after registration
- Session expires if connection is lost for >60 seconds
- Use `RECONNECT` command to restore session after disconnection

---

### ROUTE

Add a route to the proxy (requires active session).

**Format:**
```
ROUTE|session_id|domains|path|backend\n
```

**Parameters:**
- `session_id` - Session ID from REGISTER response
- `domains` - Comma-separated list of domains (e.g., "api.example.com,www.api.example.com")
- `path` - URL path to match (e.g., "/v2" or "/api/users")
- `backend` - Full backend URL (e.g., "http://api-v2:9000")

**Response:**
```
ROUTE_OK\n
```

**Example:**
```
ROUTE|api-v2-9000-1734532800|api.example.com,www.api.example.com|/v2|http://api-v2:9000\n
→ ROUTE_OK\n
```

**Multiple Routes:**
You can register multiple routes for the same service:
```
ROUTE|api-v2-9000-1734532800|api.example.com|/v2|http://api-v2:9000\n
ROUTE|api-v2-9000-1734532800|api.example.com|/v2/admin|http://api-v2:9001\n
```

---

### HEADER

Add a custom header to all routes for this service.

**Format:**
```
HEADER|session_id|header_name|header_value\n
```

**Parameters:**
- `session_id` - Session ID from REGISTER
- `header_name` - HTTP header name (e.g., "X-API-Version")
- `header_value` - Header value (e.g., "2.0")

**Response:**
```
HEADER_OK\n
```

**Example:**
```
HEADER|api-v2-9000-1734532800|X-API-Version|2.0\n
→ HEADER_OK\n
HEADER|api-v2-9000-1734532800|X-Service-Name|api-service\n
→ HEADER_OK\n
```

**Notes:**
- Headers apply to ALL routes registered by this service
- Headers are re-applied when routes are updated
- Use for versioning, debugging, or custom application logic

---

### OPTIONS

Set configuration options for the service.

**Format:**
```
OPTIONS|session_id|key|value\n
```

**Parameters:**
- `session_id` - Session ID from REGISTER
- `key` - Option name
- `value` - Option value (string, parsed based on key)

**Response:**
```
OPTIONS_OK\n
```

**Supported Options:**

| Key | Type | Example | Description |
|-----|------|---------|-------------|
| `timeout` | duration | `60s` | Request timeout |
| `health_check_interval` | duration | `30s` | Health check frequency |
| `health_check_timeout` | duration | `10s` | Health check timeout |
| `websocket` | boolean | `true` | Enable WebSocket support |
| `compression` | boolean | `true` | Enable compression |
| `http2` | boolean | `true` | Enable HTTP/2 |
| `http3` | boolean | `true` | Enable HTTP/3 (QUIC) |

**Examples:**
```
OPTIONS|api-v2-9000-1734532800|timeout|60s\n
→ OPTIONS_OK\n

OPTIONS|api-v2-9000-1734532800|websocket|true\n
→ OPTIONS_OK\n

OPTIONS|api-v2-9000-1734532800|http2|true\n
→ OPTIONS_OK\n
```

---

### VALIDATE

Validate the current configuration without applying it.

**Format:**
```
VALIDATE|session_id\n
```

**Response:**
```
OK\n
```
or
```
ERROR|error_message\n
```

**Example:**
```
VALIDATE|api-v2-9000-1734532800\n
→ OK\n
```

**Use Case:**
Check configuration before committing changes in a multi-step registration process.

---

### MAINT_ENTER

Enter maintenance mode (routes return 503 from maintenance page).

**Format:**
```
MAINT_ENTER|session_id\n
```

**Response:**
```
MAINT_OK\n
```

**Example:**
```
MAINT_ENTER|api-v2-9000-1734532800\n
→ MAINT_OK\n
```

**Behavior:**
- All routes for this service return HTTP 503
- Proxy serves maintenance page from `maintenance_port`
- Other services continue operating normally
- Use for zero-downtime deployments

---

### MAINT_EXIT

Exit maintenance mode (restore normal routing).

**Format:**
```
MAINT_EXIT|session_id\n
```

**Response:**
```
MAINT_OK\n
```

**Example:**
```
MAINT_EXIT|api-v2-9000-1734532800\n
→ MAINT_OK\n
```

---

### SHUTDOWN

Gracefully deregister and close connection.

**Format:**
```
SHUTDOWN|session_id\n
```

**Response:**
```
GOODBYE\n
```

**Example:**
```
SHUTDOWN|api-v2-9000-1734532800\n
→ GOODBYE\n
```

**Behavior:**
- All routes for this service are removed immediately
- Session is invalidated
- Connection is closed
- Use when shutting down a service

---

### RECONNECT

Reconnect with existing session after connection loss.

**Format:**
```
RECONNECT|session_id\n
```

**Response:**
```
OK\n
```
or
```
REREGISTER\n
```

**Examples:**

**Success (session still valid):**
```
RECONNECT|api-v2-9000-1734532800\n
→ OK\n
```

**Failure (session expired):**
```
RECONNECT|api-v2-9000-1734532800\n
→ REREGISTER\n
```

**Use Case:**
- Network interruption
- Proxy restart
- Service reconnecting after transient failure

**Session Expiry:**
Sessions expire after 60 seconds of disconnection. If you receive `REREGISTER`, start over with `REGISTER` command.

---

## Connection Monitoring

The proxy automatically monitors all TCP connections:

- **Connection Loss Detection** - Detects when services crash or disconnect
- **Automatic Cleanup** - Routes are removed if connection is lost for >60 seconds
- **Heartbeat** - Connection idle time is tracked
- **Reconnection Support** - Use `RECONNECT` to restore session

**Best Practices:**
- Keep TCP connection open for the lifetime of your service
- Implement reconnection logic for network interruptions
- Use `SHUTDOWN` for graceful termination
- Monitor connection status in application logs

---

## Client Examples

### Python TCP Client

Complete Python client with reconnection logic:

```python
import socket
import time
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

class ProxyRegistryClient:
    def __init__(self, host='proxy', port=81):
        self.host = host
        self.port = port
        self.socket = None
        self.session_id = None
    
    def connect(self):
        """Establish TCP connection."""
        self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.socket.connect((self.host, self.port))
        logger.info(f"Connected to {self.host}:{self.port}")
    
    def send_command(self, command):
        """Send command and read response."""
        self.socket.sendall((command + '\n').encode())
        response = self.socket.recv(1024).decode().strip()
        logger.debug(f"Command: {command} → Response: {response}")
        return response
    
    def register(self, service_name, hostname, service_port, maint_port):
        """Register service and get session ID."""
        cmd = f"REGISTER|{service_name}|{hostname}|{service_port}|{maint_port}"
        response = self.send_command(cmd)
        
        if response.startswith('ACK|'):
            self.session_id = response.split('|')[1]
            logger.info(f"✓ Registered with session: {self.session_id}")
            return True
        else:
            logger.error(f"✗ Registration failed: {response}")
            return False
    
    def add_route(self, domains, path, backend):
        """Add a route to the proxy."""
        if not self.session_id:
            raise Exception("Not registered")
        
        domains_str = ','.join(domains)
        cmd = f"ROUTE|{self.session_id}|{domains_str}|{path}|{backend}"
        response = self.send_command(cmd)
        
        if response == 'ROUTE_OK':
            logger.info(f"✓ Route added: {domains} {path} → {backend}")
            return True
        else:
            logger.error(f"✗ Route failed: {response}")
            return False
    
    def add_header(self, name, value):
        """Add custom header."""
        if not self.session_id:
            raise Exception("Not registered")
        
        cmd = f"HEADER|{self.session_id}|{name}|{value}"
        response = self.send_command(cmd)
        
        if response == 'HEADER_OK':
            logger.info(f"✓ Header added: {name}: {value}")
            return True
        else:
            logger.error(f"✗ Header failed: {response}")
            return False
    
    def set_option(self, key, value):
        """Set configuration option."""
        if not self.session_id:
            raise Exception("Not registered")
        
        cmd = f"OPTIONS|{self.session_id}|{key}|{value}"
        response = self.send_command(cmd)
        
        if response == 'OPTIONS_OK':
            logger.info(f"✓ Option set: {key}={value}")
            return True
        else:
            logger.error(f"✗ Option failed: {response}")
            return False
    
    def enter_maintenance(self):
        """Enter maintenance mode."""
        if not self.session_id:
            raise Exception("Not registered")
        
        cmd = f"MAINT_ENTER|{self.session_id}"
        response = self.send_command(cmd)
        
        if response == 'MAINT_OK':
            logger.info("✓ Entered maintenance mode")
            return True
        else:
            logger.error(f"✗ Maintenance failed: {response}")
            return False
    
    def exit_maintenance(self):
        """Exit maintenance mode."""
        if not self.session_id:
            raise Exception("Not registered")
        
        cmd = f"MAINT_EXIT|{self.session_id}"
        response = self.send_command(cmd)
        
        if response == 'MAINT_OK':
            logger.info("✓ Exited maintenance mode")
            return True
        else:
            logger.error(f"✗ Exit maintenance failed: {response}")
            return False
    
    def shutdown(self):
        """Gracefully disconnect."""
        if not self.session_id:
            return
        
        cmd = f"SHUTDOWN|{self.session_id}"
        response = self.send_command(cmd)
        logger.info(f"Shutting down: {response}")
        self.socket.close()
    
    def close(self):
        """Close connection without cleanup."""
        if self.socket:
            self.socket.close()

# Example usage
if __name__ == "__main__":
    client = ProxyRegistryClient(host='proxy', port=81)
    
    try:
        # Connect and register
        client.connect()
        client.register('api-service', 'api-v2', 9000, 9001)
        
        # Add routes
        client.add_route(['api.example.com'], '/v2', 'http://api-v2:9000')
        client.add_route(['api.example.com'], '/v2/admin', 'http://api-v2:9001')
        
        # Set headers and options
        client.add_header('X-API-Version', '2.0')
        client.add_header('X-Service-Name', 'api-service')
        client.set_option('timeout', '60s')
        client.set_option('websocket', 'true')
        client.set_option('http2', 'true')
        
        # Keep connection alive
        logger.info("Service running... (Ctrl+C to stop)")
        while True:
            time.sleep(10)
    
    except KeyboardInterrupt:
        logger.info("Shutting down...")
        client.shutdown()
    except Exception as e:
        logger.error(f"Error: {e}")
        client.close()
```

---

### Node.js TCP Client

Complete Node.js client:

```javascript
const net = require('net');

class ProxyRegistryClient {
  constructor(host = 'proxy', port = 81) {
    this.host = host;
    this.port = port;
    this.socket = null;
    this.sessionId = null;
  }

  connect() {
    return new Promise((resolve, reject) => {
      this.socket = net.createConnection({ host: this.host, port: this.port }, () => {
        console.log(`Connected to ${this.host}:${this.port}`);
        resolve();
      });

      this.socket.on('error', (err) => {
        reject(err);
      });
    });
  }

  sendCommand(command) {
    return new Promise((resolve, reject) => {
      this.socket.write(command + '\n');

      const onData = (data) => {
        const response = data.toString().trim();
        this.socket.removeListener('data', onData);
        resolve(response);
      };

      this.socket.once('data', onData);

      setTimeout(() => {
        this.socket.removeListener('data', onData);
        reject(new Error('Command timeout'));
      }, 5000);
    });
  }

  async register(serviceName, hostname, servicePort, maintPort) {
    const cmd = `REGISTER|${serviceName}|${hostname}|${servicePort}|${maintPort}`;
    const response = await this.sendCommand(cmd);

    if (response.startsWith('ACK|')) {
      this.sessionId = response.split('|')[1];
      console.log(`✓ Registered with session: ${this.sessionId}`);
      return true;
    } else {
      console.error(`✗ Registration failed: ${response}`);
      return false;
    }
  }

  async addRoute(domains, path, backend) {
    if (!this.sessionId) throw new Error('Not registered');

    const domainsStr = domains.join(',');
    const cmd = `ROUTE|${this.sessionId}|${domainsStr}|${path}|${backend}`;
    const response = await this.sendCommand(cmd);

    if (response === 'ROUTE_OK') {
      console.log(`✓ Route added: ${domains} ${path} → ${backend}`);
      return true;
    } else {
      console.error(`✗ Route failed: ${response}`);
      return false;
    }
  }

  async addHeader(name, value) {
    if (!this.sessionId) throw new Error('Not registered');

    const cmd = `HEADER|${this.sessionId}|${name}|${value}`;
    const response = await this.sendCommand(cmd);

    if (response === 'HEADER_OK') {
      console.log(`✓ Header added: ${name}: ${value}`);
      return true;
    } else {
      console.error(`✗ Header failed: ${response}`);
      return false;
    }
  }

  async setOption(key, value) {
    if (!this.sessionId) throw new Error('Not registered');

    const cmd = `OPTIONS|${this.sessionId}|${key}|${value}`;
    const response = await this.sendCommand(cmd);

    if (response === 'OPTIONS_OK') {
      console.log(`✓ Option set: ${key}=${value}`);
      return true;
    } else {
      console.error(`✗ Option failed: ${response}`);
      return false;
    }
  }

  async enterMaintenance() {
    if (!this.sessionId) throw new Error('Not registered');

    const cmd = `MAINT_ENTER|${this.sessionId}`;
    const response = await this.sendCommand(cmd);

    if (response === 'MAINT_OK') {
      console.log('✓ Entered maintenance mode');
      return true;
    } else {
      console.error(`✗ Maintenance failed: ${response}`);
      return false;
    }
  }

  async exitMaintenance() {
    if (!this.sessionId) throw new Error('Not registered');

    const cmd = `MAINT_EXIT|${this.sessionId}`;
    const response = await this.sendCommand(cmd);

    if (response === 'MAINT_OK') {
      console.log('✓ Exited maintenance mode');
      return true;
    } else {
      console.error(`✗ Exit maintenance failed: ${response}`);
      return false;
    }
  }

  async shutdown() {
    if (!this.sessionId) return;

    const cmd = `SHUTDOWN|${this.sessionId}`;
    const response = await this.sendCommand(cmd);
    console.log(`Shutting down: ${response}`);
    this.socket.end();
  }
}

// Example usage
(async () => {
  const client = new ProxyRegistryClient('proxy', 81);

  try {
    await client.connect();
    await client.register('api-service', 'api-v2', 9000, 9001);

    await client.addRoute(['api.example.com'], '/v2', 'http://api-v2:9000');
    await client.addRoute(['api.example.com'], '/v2/admin', 'http://api-v2:9001');
    
    await client.addHeader('X-API-Version', '2.0');
    await client.setOption('timeout', '60s');
    await client.setOption('websocket', 'true');

    console.log('Service running... (Ctrl+C to stop)');

    process.on('SIGINT', async () => {
      console.log('\nShutting down...');
      await client.shutdown();
      process.exit(0);
    });

    process.on('SIGTERM', async () => {
      console.log('Shutting down...');
      await client.shutdown();
      process.exit(0);
    });
  } catch (error) {
    console.error('Error:', error.message);
    process.exit(1);
  }
})();
```

---

### Go TCP Client

Complete Go client:

```go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type RegistryClient struct {
	conn      net.Conn
	reader    *bufio.Reader
	sessionID string
}

func NewRegistryClient(host string, port int) (*RegistryClient, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	log.Printf("Connected to %s", addr)

	return &RegistryClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

func (c *RegistryClient) sendCommand(command string) (string, error) {
	_, err := fmt.Fprintf(c.conn, "%s\n", command)
	if err != nil {
		return "", err
	}

	response, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response), nil
}

func (c *RegistryClient) Register(serviceName, hostname string, servicePort, maintPort int) error {
	cmd := fmt.Sprintf("REGISTER|%s|%s|%d|%d", serviceName, hostname, servicePort, maintPort)
	response, err := c.sendCommand(cmd)
	if err != nil {
		return err
	}

	if strings.HasPrefix(response, "ACK|") {
		c.sessionID = strings.Split(response, "|")[1]
		log.Printf("✓ Registered with session: %s", c.sessionID)
		return nil
	}

	return fmt.Errorf("registration failed: %s", response)
}

func (c *RegistryClient) AddRoute(domains []string, path, backend string) error {
	if c.sessionID == "" {
		return fmt.Errorf("not registered")
	}

	domainsStr := strings.Join(domains, ",")
	cmd := fmt.Sprintf("ROUTE|%s|%s|%s|%s", c.sessionID, domainsStr, path, backend)
	response, err := c.sendCommand(cmd)
	if err != nil {
		return err
	}

	if response == "ROUTE_OK" {
		log.Printf("✓ Route added: %v %s → %s", domains, path, backend)
		return nil
	}

	return fmt.Errorf("route failed: %s", response)
}

func (c *RegistryClient) AddHeader(name, value string) error {
	if c.sessionID == "" {
		return fmt.Errorf("not registered")
	}

	cmd := fmt.Sprintf("HEADER|%s|%s|%s", c.sessionID, name, value)
	response, err := c.sendCommand(cmd)
	if err != nil {
		return err
	}

	if response == "HEADER_OK" {
		log.Printf("✓ Header added: %s: %s", name, value)
		return nil
	}

	return fmt.Errorf("header failed: %s", response)
}

func (c *RegistryClient) SetOption(key, value string) error {
	if c.sessionID == "" {
		return fmt.Errorf("not registered")
	}

	cmd := fmt.Sprintf("OPTIONS|%s|%s|%s", c.sessionID, key, value)
	response, err := c.sendCommand(cmd)
	if err != nil {
		return err
	}

	if response == "OPTIONS_OK" {
		log.Printf("✓ Option set: %s=%s", key, value)
		return nil
	}

	return fmt.Errorf("option failed: %s", response)
}

func (c *RegistryClient) EnterMaintenance() error {
	if c.sessionID == "" {
		return fmt.Errorf("not registered")
	}

	cmd := fmt.Sprintf("MAINT_ENTER|%s", c.sessionID)
	response, err := c.sendCommand(cmd)
	if err != nil {
		return err
	}

	if response == "MAINT_OK" {
		log.Printf("✓ Entered maintenance mode")
		return nil
	}

	return fmt.Errorf("maintenance failed: %s", response)
}

func (c *RegistryClient) ExitMaintenance() error {
	if c.sessionID == "" {
		return fmt.Errorf("not registered")
	}

	cmd := fmt.Sprintf("MAINT_EXIT|%s", c.sessionID)
	response, err := c.sendCommand(cmd)
	if err != nil {
		return err
	}

	if response == "MAINT_OK" {
		log.Printf("✓ Exited maintenance mode")
		return nil
	}

	return fmt.Errorf("exit maintenance failed: %s", response)
}

func (c *RegistryClient) Shutdown() error {
	if c.sessionID == "" {
		return nil
	}

	cmd := fmt.Sprintf("SHUTDOWN|%s", c.sessionID)
	response, err := c.sendCommand(cmd)
	if err != nil {
		return err
	}

	log.Printf("Shutting down: %s", response)
	return c.conn.Close()
}

func main() {
	client, err := NewRegistryClient("proxy", 81)
	if err != nil {
		log.Fatal(err)
	}

	// Register service
	if err := client.Register("api-service", "api-v2", 9000, 9001); err != nil {
		log.Fatal(err)
	}

	// Add routes
	client.AddRoute([]string{"api.example.com"}, "/v2", "http://api-v2:9000")
	client.AddRoute([]string{"api.example.com"}, "/v2/admin", "http://api-v2:9001")
	
	// Set headers and options
	client.AddHeader("X-API-Version", "2.0")
	client.SetOption("timeout", "60s")
	client.SetOption("websocket", "true")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Service running... (Ctrl+C to stop)")
	<-sigChan

	log.Println("Shutting down...")
	client.Shutdown()
}
```

---

### Bash/Shell Script Client

Simple bash client using netcat:

```bash
#!/bin/bash

# Configuration
PROXY_HOST="${PROXY_HOST:-proxy}"
PROXY_PORT="${PROXY_PORT:-81}"
SERVICE_NAME="${SERVICE_NAME:-api-service}"
HOSTNAME="${HOSTNAME:-api-v2}"
SERVICE_PORT="${SERVICE_PORT:-9000}"
MAINT_PORT="${MAINT_PORT:-9001}"

# Temporary files for TCP communication
FIFO_IN="/tmp/proxy-registry-in-$$"
FIFO_OUT="/tmp/proxy-registry-out-$$"
SESSION_FILE="/tmp/proxy-session-$$"

# Cleanup on exit
cleanup() {
    rm -f "$FIFO_IN" "$FIFO_OUT" "$SESSION_FILE"
    if [ -n "$NC_PID" ]; then
        kill "$NC_PID" 2>/dev/null
    fi
    exit
}

trap cleanup INT TERM EXIT

# Create named pipes
mkfifo "$FIFO_IN" "$FIFO_OUT"

# Start netcat in background
nc "$PROXY_HOST" "$PROXY_PORT" < "$FIFO_IN" > "$FIFO_OUT" &
NC_PID=$!

# Function to send command and read response
send_command() {
    local cmd="$1"
    echo "$cmd" > "$FIFO_IN"
    read -r response < "$FIFO_OUT"
    echo "$response"
}

# Register service
echo "Connecting to $PROXY_HOST:$PROXY_PORT..."
response=$(send_command "REGISTER|$SERVICE_NAME|$HOSTNAME|$SERVICE_PORT|$MAINT_PORT")

if [[ "$response" =~ ^ACK\|(.+)$ ]]; then
    SESSION_ID="${BASH_REMATCH[1]}"
    echo "$SESSION_ID" > "$SESSION_FILE"
    echo "✓ Registered with session: $SESSION_ID"
else
    echo "✗ Registration failed: $response"
    exit 1
fi

# Add routes
echo "Adding routes..."
response=$(send_command "ROUTE|$SESSION_ID|api.example.com|/v2|http://$HOSTNAME:$SERVICE_PORT")
if [ "$response" = "ROUTE_OK" ]; then
    echo "✓ Route added: api.example.com/v2 → http://$HOSTNAME:$SERVICE_PORT"
else
    echo "✗ Route failed: $response"
fi

# Add headers
response=$(send_command "HEADER|$SESSION_ID|X-API-Version|2.0")
if [ "$response" = "HEADER_OK" ]; then
    echo "✓ Header added: X-API-Version: 2.0"
else
    echo "✗ Header failed: $response"
fi

# Set options
response=$(send_command "OPTIONS|$SESSION_ID|timeout|60s")
if [ "$response" = "OPTIONS_OK" ]; then
    echo "✓ Option set: timeout=60s"
else
    echo "✗ Option failed: $response"
fi

response=$(send_command "OPTIONS|$SESSION_ID|websocket|true")
if [ "$response" = "OPTIONS_OK" ]; then
    echo "✓ Option set: websocket=true"
else
    echo "✗ Option failed: $response"
fi

echo ""
echo "Service running... (Press Ctrl+C to stop)"

# Keep connection alive
while kill -0 "$NC_PID" 2>/dev/null; do
    sleep 10
done

echo "Connection lost"
```

**Usage:**

```bash
# Make executable
chmod +x register.sh

# Run with defaults
./register.sh

# Run with custom values
PROXY_HOST=proxy \
SERVICE_NAME=my-api \
HOSTNAME=my-api-v2 \
SERVICE_PORT=8080 \
MAINT_PORT=8081 \
./register.sh
```

**Advanced Bash Client with Functions:**

```bash
#!/bin/bash

set -euo pipefail

# Configuration
PROXY_HOST="${PROXY_HOST:-proxy}"
PROXY_PORT="${PROXY_PORT:-81}"
SESSION_ID=""

# Connect to proxy
exec 3<>/dev/tcp/$PROXY_HOST/$PROXY_PORT
echo "Connected to $PROXY_HOST:$PROXY_PORT on fd 3"

# Function to send command
send_cmd() {
    local cmd="$1"
    echo "$cmd" >&3
    read -r response <&3
    echo "$response"
}

# Register service
register() {
    local service_name="$1"
    local hostname="$2"
    local service_port="$3"
    local maint_port="$4"
    
    local response
    response=$(send_cmd "REGISTER|$service_name|$hostname|$service_port|$maint_port")
    
    if [[ "$response" =~ ^ACK\|(.+)$ ]]; then
        SESSION_ID="${BASH_REMATCH[1]}"
        echo "✓ Registered with session: $SESSION_ID"
        return 0
    else
        echo "✗ Registration failed: $response"
        return 1
    fi
}

# Add route
add_route() {
    local domains="$1"
    local path="$2"
    local backend="$3"
    
    if [ -z "$SESSION_ID" ]; then
        echo "✗ Not registered"
        return 1
    fi
    
    local response
    response=$(send_cmd "ROUTE|$SESSION_ID|$domains|$path|$backend")
    
    if [ "$response" = "ROUTE_OK" ]; then
        echo "✓ Route added: $domains$path → $backend"
        return 0
    else
        echo "✗ Route failed: $response"
        return 1
    fi
}

# Add header
add_header() {
    local name="$1"
    local value="$2"
    
    if [ -z "$SESSION_ID" ]; then
        echo "✗ Not registered"
        return 1
    fi
    
    local response
    response=$(send_cmd "HEADER|$SESSION_ID|$name|$value")
    
    if [ "$response" = "HEADER_OK" ]; then
        echo "✓ Header added: $name: $value"
        return 0
    else
        echo "✗ Header failed: $response"
        return 1
    fi
}

# Set option
set_option() {
    local key="$1"
    local value="$2"
    
    if [ -z "$SESSION_ID" ]; then
        echo "✗ Not registered"
        return 1
    fi
    
    local response
    response=$(send_cmd "OPTIONS|$SESSION_ID|$key|$value")
    
    if [ "$response" = "OPTIONS_OK" ]; then
        echo "✓ Option set: $key=$value"
        return 0
    else
        echo "✗ Option failed: $response"
        return 1
    fi
}

# Enter maintenance mode
enter_maintenance() {
    if [ -z "$SESSION_ID" ]; then
        echo "✗ Not registered"
        return 1
    fi
    
    local response
    response=$(send_cmd "MAINT_ENTER|$SESSION_ID")
    
    if [ "$response" = "MAINT_OK" ]; then
        echo "✓ Entered maintenance mode"
        return 0
    else
        echo "✗ Maintenance failed: $response"
        return 1
    fi
}

# Exit maintenance mode
exit_maintenance() {
    if [ -z "$SESSION_ID" ]; then
        echo "✗ Not registered"
        return 1
    fi
    
    local response
    response=$(send_cmd "MAINT_EXIT|$SESSION_ID")
    
    if [ "$response" = "MAINT_OK" ]; then
        echo "✓ Exited maintenance mode"
        return 0
    else
        echo "✗ Exit maintenance failed: $response"
        return 1
    fi
}

# Shutdown
shutdown() {
    if [ -z "$SESSION_ID" ]; then
        return 0
    fi
    
    local response
    response=$(send_cmd "SHUTDOWN|$SESSION_ID")
    echo "Shutting down: $response"
    exec 3>&-  # Close connection
}

# Cleanup on exit
trap 'shutdown' EXIT INT TERM

# Main
main() {
    # Register
    register "api-service" "api-v2" 9000 9001 || exit 1
    
    # Add routes
    add_route "api.example.com" "/v2" "http://api-v2:9000"
    add_route "api.example.com" "/v2/admin" "http://api-v2:9001"
    
    # Set headers
    add_header "X-API-Version" "2.0"
    add_header "X-Service-Name" "api-service"
    
    # Set options
    set_option "timeout" "60s"
    set_option "websocket" "true"
    set_option "http2" "true"
    
    echo ""
    echo "Service running... (Press Ctrl+C to stop)"
    
    # Keep connection alive
    while true; do
        sleep 10
    done
}

main "$@"
```

**Docker Entrypoint Example:**

```bash
#!/bin/bash
# entrypoint.sh - Register with proxy then start application

set -e

# Start registry client in background
(
    exec 3<>/dev/tcp/proxy/81
    
    echo "REGISTER|${SERVICE_NAME}|${HOSTNAME}|${SERVICE_PORT}|${MAINT_PORT}" >&3
    read -r response <&3
    
    if [[ "$response" =~ ^ACK\|(.+)$ ]]; then
        SESSION_ID="${BASH_REMATCH[1]}"
        echo "Registered with proxy: $SESSION_ID"
        
        # Add route
        echo "ROUTE|$SESSION_ID|${DOMAIN}|${PATH}|http://${HOSTNAME}:${SERVICE_PORT}" >&3
        read -r response <&3
        echo "Route registration: $response"
        
        # Keep connection alive
        while true; do
            sleep 30
        done
    fi
) &

REGISTRY_PID=$!

# Cleanup on exit
trap "kill $REGISTRY_PID 2>/dev/null || true" EXIT

# Start main application
exec "$@"
```

**Dockerfile:**
```dockerfile
FROM myapp:latest

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV SERVICE_NAME=api-service
ENV HOSTNAME=api-v2
ENV SERVICE_PORT=9000
ENV MAINT_PORT=9001
ENV DOMAIN=api.example.com
ENV PATH=/

ENTRYPOINT ["/entrypoint.sh"]
CMD ["./app"]
```

---

## Best Practices

### 1. Keep Connection Alive

The TCP connection must remain open for the lifetime of your service:

```python
# ✓ Good - connection stays open
while True:
    time.sleep(10)

# ✗ Bad - connection closes after registration
register()
sys.exit(0)
```

### 2. Handle Reconnection

Implement reconnection logic for network interruptions:

```python
def register_with_retry(client, max_attempts=5):
    for attempt in range(max_attempts):
        try:
            client.connect()
            client.register('api-service', 'api-v2', 9000, 9001)
            return True
        except Exception as e:
            if attempt < max_attempts - 1:
                time.sleep(2 ** attempt)  # Exponential backoff
            else:
                raise
```

### 3. Graceful Shutdown

Always use `SHUTDOWN` command when terminating:

```python
import signal
import sys

def signal_handler(sig, frame):
    logger.info("Shutting down...")
    client.shutdown()
    sys.exit(0)

signal.signal(signal.SIGINT, signal_handler)
signal.signal(signal.SIGTERM, signal_handler)
```

### 4. Run in Background Thread

Don't block main application thread:

```python
import threading

def registry_thread():
    client = ProxyRegistryClient()
    client.connect()
    client.register('api-service', 'api-v2', 9000, 9001)
    # Keep connection alive
    while True:
        time.sleep(10)

thread = threading.Thread(target=registry_thread, daemon=True)
thread.start()

# Main application continues
app.run()
```

### 5. Monitor Connection Status

Log connection events for debugging:

```python
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

logger.info("Connecting to registry...")
client.connect()
logger.info("Registering service...")
client.register('api-service', 'api-v2', 9000, 9001)
logger.info("Adding routes...")
```

---

## Troubleshooting

### Connection Refused

**Problem:** Cannot connect to proxy on port 81  
**Solution:** Check network connectivity and firewall

```bash
# From service container
nc -zv proxy 81

# Check if port is exposed
docker service inspect proxy_proxy | grep 81
```

### Session Expired (REREGISTER)

**Problem:** Reconnection returns `REREGISTER`  
**Solution:** Session timeout (>60s), must register again

```python
response = client.send_command(f"RECONNECT|{session_id}")
if response == "REREGISTER":
    client.register('api-service', 'api-v2', 9000, 9001)
```

### Routes Not Active After Registration

**Problem:** Routes registered but traffic not flowing  
**Solution:** Check backend connectivity from proxy

```bash
# From proxy container
docker exec -it proxy curl http://api-v2:9000/health
```

### ERROR|Invalid session

**Problem:** Using wrong or expired session ID  
**Solution:** Re-register to get new session

```python
# Store session ID after registration
response = client.send_command("REGISTER|api|api-v2|9000|9001")
session_id = response.split('|')[1]

# Use this session ID for all subsequent commands
```

---

## Security Considerations

### Network Isolation

**Best Practice:** Only allow registry access from internal networks

```bash
# Firewall rule - Docker Swarm overlay network only
# Port 81 is NOT exposed to public internet by default

# If you need external access, restrict by IP:
ufw allow from 10.0.0.0/8 to any port 81
```

### No Authentication

**Current State:** Registry has NO authentication

**Security Measures:**
- Run on internal overlay network only
- Use firewall rules to restrict access
- Monitor registration events
- Consider implementing mutual TLS in future

**Production Recommendations:**
- DO NOT expose port 81 to public internet
- Use Docker Swarm overlay networks
- Implement application-level authentication if needed

---

## Monitoring

### Connection Status

Monitor TCP connections to registry:

```bash
# On proxy host
netstat -an | grep :81

# Count active sessions
docker exec -it proxy netstat -an | grep :81 | wc -l
```

### Log Events

Registry logs all events:

```bash
docker service logs proxy_proxy | grep "\[registry\]"
```

Sample output:
```
[registry] Service registered: api-service at api-v2:9000 (session: api-v2-9000-1734532800)
[registry] Route added: api-service at [api.example.com]/v2 -> http://api-v2:9000
[registry] Header added: X-API-Version = 2.0 for api-service
```

---

## Advanced Patterns

### Blue-Green Deployment

```python
# Deploy green (v2)
green = ProxyRegistryClient()
green.connect()
green.register('api-green', 'api-v2', 9000, 9001)
green.add_route(['api.example.com'], '/v2-beta', 'http://api-v2:9000')

# Test green environment...

# Switch traffic: shutdown blue, promote green
blue.shutdown()  # Removes all v1 routes
green.add_route(['api.example.com'], '/v2', 'http://api-v2:9000')
```

### Canary Release

```python
# Keep v1 running
v1_client.add_route(['api.example.com'], '/api', 'http://api-v1:9000')

# Deploy v2 on different path
v2_client.add_route(['api.example.com'], '/api-canary', 'http://api-v2:9000')

# Route 10% of users to canary path in application logic
# Monitor metrics, gradually increase traffic
```

### Maintenance Mode Workflow

```python
# Enter maintenance mode
client.enter_maintenance()

# Perform updates (database migrations, etc.)
# ...

# Exit maintenance mode
client.exit_maintenance()
```

### Multi-Domain Service

```python
# Single service, multiple domains
client.add_route(['api.example.com', 'api.example.org'], '/v2', 'http://api-v2:9000')

# Or separate routes per domain
client.add_route(['api.example.com'], '/v2', 'http://api-v2:9000')
client.add_route(['api.example.org'], '/v2', 'http://api-v2-eu:9000')
```

---

## Protocol Specification

### Message Format

All messages are ASCII text, newline-terminated:

```
<COMMAND>|<arg1>|<arg2>|...|<argN>\n
```

### Character Encoding

- **Encoding:** UTF-8
- **Line Ending:** `\n` (LF, ASCII 10)
- **Separator:** `|` (pipe, ASCII 124)

### Reserved Characters

The pipe character `|` is reserved. Do not use it in:
- Service names
- Hostnames
- Paths
- Header names/values
- Backend URLs

### Maximum Message Size

- **Command:** 8 KB per line
- **Response:** 1 KB per line

### Timeout

- **Read Timeout:** 30 seconds per command
- **Connection Idle:** 5 minutes (keepalive)
- **Session Expiry:** 60 seconds after disconnect

---

## Related Documentation

- [README.md](README.md) - Overview and quick start
- [CONFIGURATION.md](CONFIGURATION.md) - Configuration reference
- [DEPLOYMENT.md](DEPLOYMENT.md) - Deployment guide
- [MIGRATION.md](MIGRATION.md) - Migration from nginx/Traefik/HAProxy

---

## Support

For integration help:

- Review client examples in this guide
- Check proxy logs: `docker service logs proxy_proxy`
- Test protocol with `nc` or `telnet`:
  ```bash
  nc proxy 81
  REGISTER|test|test-service|9000|9001
  ```
- Verify network connectivity between service and proxy

**Common Issues:**
- Connection refused → Check network/firewall
- REREGISTER → Session expired, register again
- ERROR|Invalid session → Wrong session ID
- Routes not active → Check backend connectivity
