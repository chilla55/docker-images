# Service Registry Guide

Complete guide for integrating applications with the go-proxy service registry for dynamic route management.

## Overview

The service registry allows backend applications to dynamically register and deregister routes without editing configuration files or restarting the proxy. This is ideal for:

- **Microservices** - Services register themselves on startup
- **Auto-scaling** - New instances automatically get routed
- **Blue-Green Deployments** - Switch traffic programmatically
- **Canary Releases** - Gradually roll out new versions
- **Maintenance Mode** - Temporarily disable routes

**Registry Port:** `81` (HTTP API)

---

## API Reference

### Register Route

Add a new route to the proxy.

**Endpoint:** `POST /register`

**Request Body:**
```json
{
  "host": "api.example.com",
  "path": "/v2",
  "backend": "http://api-v2:9000",
  "options": {
    "timeout": "60s",
    "websocket": true,
    "headers": {
      "X-API-Version": "2.0"
    }
  }
}
```

**Response (Success):**
```json
{
  "status": "registered",
  "host": "api.example.com",
  "path": "/v2",
  "backend": "http://api-v2:9000"
}
```

**Response (Error):**
```json
{
  "error": "route already exists"
}
```

### Deregister Route

Remove a route from the proxy.

**Endpoint:** `POST /deregister`

**Request Body:**
```json
{
  "host": "api.example.com",
  "path": "/v2"
}
```

**Response (Success):**
```json
{
  "status": "deregistered",
  "host": "api.example.com",
  "path": "/v2"
}
```

### List Routes

Get all currently registered routes.

**Endpoint:** `GET /routes`

**Response:**
```json
{
  "routes": [
    {
      "host": "api.example.com",
      "path": "/v1",
      "backend": "http://api-v1:8080",
      "registered_at": "2025-12-18T10:30:00Z"
    },
    {
      "host": "api.example.com",
      "path": "/v2",
      "backend": "http://api-v2:9000",
      "registered_at": "2025-12-18T11:45:00Z"
    }
  ],
  "total": 2
}
```

### Health Check

Check registry availability.

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "healthy",
  "registered_routes": 12
}
```

---

## Registration Options

### Basic Options

```json
{
  "timeout": "30s",              // Request timeout
  "websocket": false,            // Enable WebSocket support
  "headers": {}                  // Custom response headers
}
```

### Advanced Options

```json
{
  "timeout": "60s",
  "websocket": true,
  "headers": {
    "X-Service-Version": "2.0",
    "X-Custom-Header": "value"
  },
  "rate_limit": {
    "enabled": true,
    "requests_per_min": 100,
    "per_ip": true
  },
  "circuit_breaker": {
    "enabled": true,
    "failure_threshold": 5,
    "timeout": "30s"
  },
  "retry": {
    "enabled": true,
    "max_attempts": 3,
    "backoff": "exponential"
  }
}
```

---

## Client Examples

### cURL

#### Register Route

```bash
curl -X POST http://proxy:81/register \
  -H "Content-Type: application/json" \
  -d '{
    "host": "api.example.com",
    "path": "/v2",
    "backend": "http://api-v2:9000",
    "options": {
      "timeout": "60s",
      "websocket": true
    }
  }'
```

#### Deregister Route

```bash
curl -X POST http://proxy:81/deregister \
  -H "Content-Type: application/json" \
  -d '{
    "host": "api.example.com",
    "path": "/v2"
  }'
```

#### List Routes

```bash
curl http://proxy:81/routes
```

---

### Python

#### Using requests Library

```python
import requests
import json

REGISTRY_URL = "http://proxy:81"

def register_route(host, path, backend, options=None):
    """Register a route with the proxy."""
    payload = {
        "host": host,
        "path": path,
        "backend": backend
    }
    if options:
        payload["options"] = options
    
    response = requests.post(
        f"{REGISTRY_URL}/register",
        headers={"Content-Type": "application/json"},
        json=payload
    )
    
    if response.status_code == 200:
        print(f"✓ Registered {host}{path} → {backend}")
        return response.json()
    else:
        print(f"✗ Failed to register: {response.text}")
        return None

def deregister_route(host, path):
    """Deregister a route from the proxy."""
    payload = {
        "host": host,
        "path": path
    }
    
    response = requests.post(
        f"{REGISTRY_URL}/deregister",
        headers={"Content-Type": "application/json"},
        json=payload
    )
    
    if response.status_code == 200:
        print(f"✓ Deregistered {host}{path}")
        return response.json()
    else:
        print(f"✗ Failed to deregister: {response.text}")
        return None

def list_routes():
    """List all registered routes."""
    response = requests.get(f"{REGISTRY_URL}/routes")
    
    if response.status_code == 200:
        data = response.json()
        print(f"Total routes: {data['total']}")
        for route in data['routes']:
            print(f"  {route['host']}{route['path']} → {route['backend']}")
        return data['routes']
    else:
        print(f"✗ Failed to list routes: {response.text}")
        return []

# Example usage
if __name__ == "__main__":
    # Register a route
    register_route(
        host="api.example.com",
        path="/v2",
        backend="http://api-v2:9000",
        options={
            "timeout": "60s",
            "websocket": True,
            "rate_limit": {
                "enabled": True,
                "requests_per_min": 100
            }
        }
    )
    
    # List all routes
    list_routes()
    
    # Deregister on shutdown
    deregister_route("api.example.com", "/v2")
```

#### Flask Integration

```python
from flask import Flask
import requests
import atexit
import os

app = Flask(__name__)

PROXY_REGISTRY = os.getenv("PROXY_REGISTRY", "http://proxy:81")
SERVICE_HOST = os.getenv("SERVICE_HOST", "api.example.com")
SERVICE_PATH = os.getenv("SERVICE_PATH", "/v2")
SERVICE_BACKEND = os.getenv("SERVICE_BACKEND", "http://localhost:9000")

def register_with_proxy():
    """Register this service with the proxy on startup."""
    try:
        response = requests.post(
            f"{PROXY_REGISTRY}/register",
            json={
                "host": SERVICE_HOST,
                "path": SERVICE_PATH,
                "backend": SERVICE_BACKEND,
                "options": {
                    "timeout": "60s",
                    "health_check_path": "/health"
                }
            },
            timeout=5
        )
        
        if response.status_code == 200:
            app.logger.info(f"Registered with proxy: {SERVICE_HOST}{SERVICE_PATH}")
        else:
            app.logger.error(f"Failed to register: {response.text}")
    except Exception as e:
        app.logger.error(f"Registry connection failed: {e}")

def deregister_from_proxy():
    """Deregister this service from the proxy on shutdown."""
    try:
        requests.post(
            f"{PROXY_REGISTRY}/deregister",
            json={
                "host": SERVICE_HOST,
                "path": SERVICE_PATH
            },
            timeout=5
        )
        app.logger.info("Deregistered from proxy")
    except Exception as e:
        app.logger.error(f"Deregistration failed: {e}")

# Register on startup
register_with_proxy()

# Deregister on shutdown
atexit.register(deregister_from_proxy)

@app.route("/health")
def health():
    return {"status": "healthy"}, 200

@app.route("/")
def index():
    return {"message": "API v2", "status": "running"}

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=9000)
```

---

### Node.js

#### Using axios

```javascript
const axios = require('axios');

const REGISTRY_URL = process.env.PROXY_REGISTRY || 'http://proxy:81';

async function registerRoute(host, path, backend, options = {}) {
  try {
    const response = await axios.post(`${REGISTRY_URL}/register`, {
      host,
      path,
      backend,
      options
    });
    
    console.log(`✓ Registered ${host}${path} → ${backend}`);
    return response.data;
  } catch (error) {
    console.error(`✗ Registration failed: ${error.response?.data || error.message}`);
    return null;
  }
}

async function deregisterRoute(host, path) {
  try {
    const response = await axios.post(`${REGISTRY_URL}/deregister`, {
      host,
      path
    });
    
    console.log(`✓ Deregistered ${host}${path}`);
    return response.data;
  } catch (error) {
    console.error(`✗ Deregistration failed: ${error.response?.data || error.message}`);
    return null;
  }
}

async function listRoutes() {
  try {
    const response = await axios.get(`${REGISTRY_URL}/routes`);
    console.log(`Total routes: ${response.data.total}`);
    response.data.routes.forEach(route => {
      console.log(`  ${route.host}${route.path} → ${route.backend}`);
    });
    return response.data.routes;
  } catch (error) {
    console.error(`✗ Failed to list routes: ${error.message}`);
    return [];
  }
}

// Example usage
(async () => {
  await registerRoute(
    'api.example.com',
    '/v2',
    'http://api-v2:9000',
    {
      timeout: '60s',
      websocket: true
    }
  );
  
  await listRoutes();
  
  // Deregister on shutdown
  process.on('SIGTERM', async () => {
    await deregisterRoute('api.example.com', '/v2');
    process.exit(0);
  });
})();
```

#### Express.js Integration

```javascript
const express = require('express');
const axios = require('axios');

const app = express();

const PROXY_REGISTRY = process.env.PROXY_REGISTRY || 'http://proxy:81';
const SERVICE_HOST = process.env.SERVICE_HOST || 'api.example.com';
const SERVICE_PATH = process.env.SERVICE_PATH || '/v2';
const SERVICE_BACKEND = process.env.SERVICE_BACKEND || 'http://localhost:9000';

async function registerWithProxy() {
  try {
    await axios.post(`${PROXY_REGISTRY}/register`, {
      host: SERVICE_HOST,
      path: SERVICE_PATH,
      backend: SERVICE_BACKEND,
      options: {
        timeout: '60s',
        health_check_path: '/health'
      }
    });
    console.log(`Registered with proxy: ${SERVICE_HOST}${SERVICE_PATH}`);
  } catch (error) {
    console.error('Failed to register with proxy:', error.message);
  }
}

async function deregisterFromProxy() {
  try {
    await axios.post(`${PROXY_REGISTRY}/deregister`, {
      host: SERVICE_HOST,
      path: SERVICE_PATH
    });
    console.log('Deregistered from proxy');
  } catch (error) {
    console.error('Failed to deregister:', error.message);
  }
}

// Health check endpoint
app.get('/health', (req, res) => {
  res.json({ status: 'healthy' });
});

// API endpoints
app.get('/', (req, res) => {
  res.json({ message: 'API v2', status: 'running' });
});

// Start server
const server = app.listen(9000, () => {
  console.log('Server started on port 9000');
  registerWithProxy();
});

// Graceful shutdown
process.on('SIGTERM', async () => {
  console.log('SIGTERM received, shutting down...');
  await deregisterFromProxy();
  server.close(() => {
    console.log('Server closed');
    process.exit(0);
  });
});
```

---

### Go

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const registryURL = "http://proxy:81"

type RegistrationRequest struct {
	Host    string                 `json:"host"`
	Path    string                 `json:"path"`
	Backend string                 `json:"backend"`
	Options map[string]interface{} `json:"options,omitempty"`
}

type DeregistrationRequest struct {
	Host string `json:"host"`
	Path string `json:"path"`
}

func registerRoute(host, path, backend string, options map[string]interface{}) error {
	req := RegistrationRequest{
		Host:    host,
		Path:    path,
		Backend: backend,
		Options: options,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		registryURL+"/register",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registration failed with status %d", resp.StatusCode)
	}

	log.Printf("✓ Registered %s%s → %s", host, path, backend)
	return nil
}

func deregisterRoute(host, path string) error {
	req := DeregistrationRequest{
		Host: host,
		Path: path,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := http.Post(
		registryURL+"/deregister",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deregistration failed with status %d", resp.StatusCode)
	}

	log.Printf("✓ Deregistered %s%s", host, path)
	return nil
}

func main() {
	// Service configuration
	host := getEnv("SERVICE_HOST", "api.example.com")
	path := getEnv("SERVICE_PATH", "/v2")
	backend := getEnv("SERVICE_BACKEND", "http://localhost:9000")

	// Register on startup
	options := map[string]interface{}{
		"timeout":   "60s",
		"websocket": true,
		"rate_limit": map[string]interface{}{
			"enabled":          true,
			"requests_per_min": 100,
		},
	}

	if err := registerRoute(host, path, backend, options); err != nil {
		log.Printf("Failed to register: %v", err)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"message": "API v2",
			"status":  "running",
		})
	})

	server := &http.Server{Addr: ":9000"}

	go func() {
		log.Println("Server starting on :9000")
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down...")

	// Deregister from proxy
	if err := deregisterRoute(host, path); err != nil {
		log.Printf("Failed to deregister: %v", err)
	}

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(ctx)

	log.Println("Shutdown complete")
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
```

---

### Java (Spring Boot)

```java
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.context.event.EventListener;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.client.RestTemplate;
import org.springframework.beans.factory.annotation.Value;

import javax.annotation.PreDestroy;
import java.util.HashMap;
import java.util.Map;

@SpringBootApplication
@RestController
public class ApiService {

    @Value("${proxy.registry.url:http://proxy:81}")
    private String registryUrl;

    @Value("${service.host:api.example.com}")
    private String serviceHost;

    @Value("${service.path:/v2}")
    private String servicePath;

    @Value("${service.backend:http://localhost:9000}")
    private String serviceBackend;

    private final RestTemplate restTemplate = new RestTemplate();

    public static void main(String[] args) {
        SpringApplication.run(ApiService.class, args);
    }

    @EventListener(ApplicationReadyEvent.class)
    public void registerWithProxy() {
        try {
            Map<String, Object> request = new HashMap<>();
            request.put("host", serviceHost);
            request.put("path", servicePath);
            request.put("backend", serviceBackend);

            Map<String, Object> options = new HashMap<>();
            options.put("timeout", "60s");
            options.put("health_check_path", "/health");
            request.put("options", options);

            restTemplate.postForObject(registryUrl + "/register", request, Map.class);
            System.out.println("✓ Registered with proxy: " + serviceHost + servicePath);
        } catch (Exception e) {
            System.err.println("Failed to register with proxy: " + e.getMessage());
        }
    }

    @PreDestroy
    public void deregisterFromProxy() {
        try {
            Map<String, String> request = new HashMap<>();
            request.put("host", serviceHost);
            request.put("path", servicePath);

            restTemplate.postForObject(registryUrl + "/deregister", request, Map.class);
            System.out.println("✓ Deregistered from proxy");
        } catch (Exception e) {
            System.err.println("Failed to deregister: " + e.getMessage());
        }
    }

    @GetMapping("/health")
    public Map<String, String> health() {
        Map<String, String> response = new HashMap<>();
        response.put("status", "healthy");
        return response;
    }

    @GetMapping("/")
    public Map<String, String> index() {
        Map<String, String> response = new HashMap<>();
        response.put("message", "API v2");
        response.put("status", "running");
        return response;
    }
}
```

---

## Docker Integration

### Docker Compose Sidecar

Run a registration sidecar container:

**docker-compose.yml:**
```yaml
version: '3.8'

services:
  api:
    image: myapi:latest
    environment:
      - SERVICE_NAME=api
    networks:
      - web-net
  
  registrar:
    image: curlimages/curl:latest
    command: >
      sh -c "
        sleep 5 &&
        curl -X POST http://proxy:81/register \
          -H 'Content-Type: application/json' \
          -d '{
            \"host\": \"api.example.com\",
            \"path\": \"/\",
            \"backend\": \"http://api:8080\",
            \"options\": {\"timeout\": \"30s\"}
          }'
      "
    depends_on:
      - api
    networks:
      - web-net
    restart: "no"

networks:
  web-net:
    external: true
```

### Kubernetes Init Container

**deployment.yaml:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-service
spec:
  replicas: 3
  template:
    spec:
      initContainers:
        - name: register
          image: curlimages/curl:latest
          command:
            - sh
            - -c
            - |
              curl -X POST http://proxy:81/register \
                -H "Content-Type: application/json" \
                -d "{
                  \"host\": \"api.example.com\",
                  \"path\": \"/\",
                  \"backend\": \"http://$HOSTNAME:8080\"
                }"
      containers:
        - name: api
          image: myapi:latest
          ports:
            - containerPort: 8080
```

---

## Best Practices

### 1. Register on Startup

Always register routes when your application starts:

```python
# Python example
if __name__ == "__main__":
    register_with_proxy()
    app.run()
```

### 2. Deregister on Shutdown

Use signal handlers to deregister gracefully:

```python
import atexit
atexit.register(deregister_from_proxy)
```

### 3. Handle Registration Failures

Don't crash if registration fails (proxy might be temporarily down):

```python
try:
    register_with_proxy()
except Exception as e:
    app.logger.warning(f"Registration failed: {e}")
    # Continue running - might register later
```

### 4. Use Health Checks

Always include a health check endpoint:

```python
@app.route("/health")
def health():
    return {"status": "healthy"}, 200
```

### 5. Set Reasonable Timeouts

Configure appropriate timeouts for your service:

```json
{
  "options": {
    "timeout": "60s",
    "health_check_interval": "30s"
  }
}
```

### 6. Include Version in Path

Use versioned paths for easier rollouts:

```json
{
  "path": "/v2",
  "backend": "http://api-v2:9000"
}
```

### 7. Retry Registration

Implement retry logic if proxy is unavailable:

```python
def register_with_retry(max_attempts=5):
    for attempt in range(max_attempts):
        try:
            register_with_proxy()
            return
        except Exception as e:
            if attempt < max_attempts - 1:
                time.sleep(2 ** attempt)  # Exponential backoff
            else:
                raise
```

---

## Troubleshooting

### Registration Fails with "route already exists"

**Problem:** Route is already registered (duplicate or orphaned)  
**Solution:** Deregister first, then register again

```bash
curl -X POST http://proxy:81/deregister \
  -H "Content-Type: application/json" \
  -d '{"host": "api.example.com", "path": "/v2"}'

curl -X POST http://proxy:81/register \
  -H "Content-Type: application/json" \
  -d '{"host": "api.example.com", "path": "/v2", "backend": "http://api-v2:9000"}'
```

### Routes Not Showing in Proxy

**Problem:** Backend not reachable or wrong network  
**Solution:** Verify network connectivity

```bash
# From proxy container
docker exec -it proxy_proxy curl http://api-v2:9000/health
```

### Registration Succeeds but Traffic Not Routed

**Problem:** DNS or host header mismatch  
**Solution:** Check domain configuration and test with Host header

```bash
curl -H "Host: api.example.com" http://proxy/v2
```

### Proxy Returns 502 for Registered Route

**Problem:** Backend is down or health check failing  
**Solution:** Check backend logs and health endpoint

```bash
# Check backend health directly
curl http://api-v2:9000/health

# Check proxy logs
docker service logs proxy_proxy | grep api-v2
```

---

## Security Considerations

### Network Isolation

Only allow registry access from trusted networks:

```bash
# Firewall rule - only internal services
ufw allow from 10.0.0.0/8 to any port 81
```

### Authentication (Future)

Currently the registry is unauthenticated. For production:

- Run on internal network only
- Use firewall rules to restrict access
- Consider implementing API key authentication

### Rate Limiting

The registry itself has no rate limiting. Services can spam registrations. Best practice:

- Limit registry access via firewall
- Monitor registration events
- Alert on unusual registration patterns

---

## Monitoring

### Registry Metrics

Check registry health:

```bash
curl http://proxy:81/health
```

### Route List

Audit registered routes:

```bash
curl http://proxy:81/routes | jq '.routes[] | {host, path, backend}'
```

### Proxy Metrics

Check if registered routes are receiving traffic:

```bash
curl http://proxy:8080/metrics | grep proxy_requests_total
```

---

## Advanced Patterns

### Blue-Green Deployment

```python
# Deploy v2 (green)
register_route("api.example.com", "/v2", "http://api-v2:9000")

# Test green
# ...

# Switch traffic: deregister v1 (blue), promote v2
deregister_route("api.example.com", "/v1")
register_route("api.example.com", "/v1", "http://api-v2:9000")  # v2 now serves v1 path
```

### Canary Release

```python
# Keep v1 as main
# Register v2 on different path
register_route("api.example.com", "/v2-canary", "http://api-v2:9000")

# Gradually migrate users to /v2-canary
# Monitor metrics
# Eventually switch /v1 to v2 backend
```

### A/B Testing

```python
# Register both versions
register_route("api.example.com", "/a", "http://api-variant-a:9000")
register_route("api.example.com", "/b", "http://api-variant-b:9000")

# Route users based on cookie/header in application logic
```

---

## Support

For integration help:

- Review examples in this guide
- Check proxy logs: `docker service logs proxy_proxy`
- Test with cURL before implementing client
- Verify network connectivity between service and proxy

Related Documentation:
- [README.md](README.md) - Overview
- [CONFIGURATION.md](CONFIGURATION.md) - Configuration reference
- [DEPLOYMENT.md](DEPLOYMENT.md) - Deployment guide
- [MIGRATION.md](MIGRATION.md) - Migration from other proxies
