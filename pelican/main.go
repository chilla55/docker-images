package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	registryclient "github.com/chilla55/registry-client/v2"
)

type config struct {
	registryAddr        string
	serviceName         string
	domains             []string
	routePath           string
	backendPort         string
	priority            int
	proxyHealthPath     string
	proxyHealthInterval string
	proxyHealthTimeout  string
	localHealthURL      string
	localCheckInterval  time.Duration
	localCheckTimeout   time.Duration
	debug               bool
}

type agent struct {
	cfg        config
	mu         sync.Mutex
	client     *registryclient.RegistryClientV2
	routeID    string
	registered bool
}

func main() {
	cfg := loadConfig()
	agent := &agent{cfg: cfg}
	defer agent.shutdown("process exiting")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log("INFO", "Starting Pelican registry agent for %s", strings.Join(cfg.domains, ","))
	agent.reconcile()

	ticker := time.NewTicker(cfg.localCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case sig := <-sigChan:
			log("INFO", "Received signal %v", sig)
			return
		case <-ticker.C:
			agent.reconcile()
		}
	}
}

func loadConfig() config {
	return config{
		registryAddr:        fmt.Sprintf("%s:%s", getEnv("REGISTRY_HOST", "proxy"), getEnv("REGISTRY_PORT", "81")),
		serviceName:         getEnv("SERVICE_NAME", "pelican"),
		domains:             splitDomains(getEnv("DOMAINS", "panel.chilla55.de")),
		routePath:           getEnv("ROUTE_PATH", "/"),
		backendPort:         getEnv("PORT", "80"),
		priority:            getEnvInt("REGISTRY_ROUTE_PRIORITY", 10),
		proxyHealthPath:     getEnv("REGISTRY_PROXY_HEALTH_PATH", "/"),
		proxyHealthInterval: getEnv("REGISTRY_PROXY_HEALTH_INTERVAL", "30s"),
		proxyHealthTimeout:  getEnv("REGISTRY_PROXY_HEALTH_TIMEOUT", "5s"),
		localHealthURL:      getEnv("REGISTRY_LOCAL_HEALTH_URL", "http://127.0.0.1:80/"),
		localCheckInterval:  getEnvDuration("REGISTRY_LOCAL_CHECK_INTERVAL", 10*time.Second),
		localCheckTimeout:   getEnvDuration("REGISTRY_LOCAL_CHECK_TIMEOUT", 3*time.Second),
		debug:               getEnvBool("REGISTRY_DEBUG", true),
	}
}

func (a *agent) reconcile() {
	if !localEndpointHealthy(a.cfg.localHealthURL, a.cfg.localCheckTimeout) {
		if a.isRegistered() {
			log("WARN", "Local Caddy endpoint is unhealthy, removing route")
		}
		a.shutdown("local caddy endpoint unhealthy")
		return
	}

	a.ensureRegistered()
}

func (a *agent) ensureRegistered() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.registered {
		return
	}

	metadata := map[string]interface{}{
		"service": "pelican-panel",
		"version": getEnv("IMAGE_VERSION", "overlay"),
	}

	client := registryclient.NewRegistryClient(a.cfg.registryAddr, a.cfg.serviceName, "", 0, metadata, a.cfg.debug)
	registerEventHandlers(client)

	if err := client.Init(); err != nil {
		log("ERROR", "Registry init failed: %v", err)
		return
	}

	backendURL := client.BuildBackendURL(a.cfg.backendPort)
	routeID, err := client.AddRoute(a.cfg.domains, a.cfg.routePath, backendURL, a.cfg.priority)
	if err != nil {
		log("ERROR", "Failed to add route: %v", err)
		client.Close()
		return
	}

	if err := client.SetHealthCheck(routeID, a.cfg.proxyHealthPath, a.cfg.proxyHealthInterval, a.cfg.proxyHealthTimeout); err != nil {
		log("WARN", "Failed to set proxy health check: %v", err)
	}

	if err := client.SetOptions("compression", "true"); err != nil {
		log("WARN", "Failed to enable compression: %v", err)
	}

	if err := client.ApplyConfig(); err != nil {
		log("ERROR", "Failed to apply registry config: %v", err)
		client.Close()
		return
	}

	a.client = client
	a.routeID = routeID
	a.registered = true

	log("INFO", "Registered route %s for %s -> %s", routeID, strings.Join(a.cfg.domains, ","), backendURL)
	go client.StartKeepalive()
}

func (a *agent) shutdown(reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client == nil {
		return
	}

	log("INFO", "Shutting down registry client: %s", reason)

	if a.routeID != "" {
		if err := a.client.RemoveRoute(a.routeID); err != nil {
			log("WARN", "Failed to remove route %s: %v", a.routeID, err)
		} else if err := a.client.ApplyConfig(); err != nil {
			log("WARN", "Failed to apply route removal for %s: %v", a.routeID, err)
		} else {
			log("INFO", "Removed route %s", a.routeID)
		}
	}

	if err := a.client.Shutdown(); err != nil {
		log("WARN", "Registry shutdown returned error: %v", err)
	}
	a.client.Close()
	if a.registered {
		log("INFO", "Registry session closed")
	}

	a.client = nil
	a.routeID = ""
	a.registered = false
}

func (a *agent) isRegistered() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.registered
}

func registerEventHandlers(client *registryclient.RegistryClientV2) {
	client.On(registryclient.EventLog, func(event registryclient.Event) {
		level := strings.ToUpper(fmt.Sprintf("%v", event.Data["level"]))
		log(level, "[Registry] %v", event.Data["message"])
	})

	client.On(registryclient.EventError, func(event registryclient.Event) {
		log("ERROR", "[Registry] %v", event.Data["message"])
	})

	client.On(registryclient.EventIPChanged, func(event registryclient.Event) {
		log("WARN", "[Registry] IP changed: %v -> %v", event.Data["old_ip"], event.Data["new_ip"])
	})
}

func localEndpointHealthy(url string, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode < 500
}

func splitDomains(raw string) []string {
	parts := strings.Split(raw, ",")
	domains := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			domains = append(domains, trimmed)
		}
	}
	if len(domains) == 0 {
		return []string{"panel.chilla55.de"}
	}
	return domains
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func log(level, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	fmt.Printf("[%s] [%s] %s\n", timestamp, level, message)
}
