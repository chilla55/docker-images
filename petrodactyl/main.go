package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	appDir = "/var/www/pterodactyl"
)

var (
	registryClientV2 *RegistryClientV2
	routeID          string
	done             = make(chan os.Signal, 1)
)

func main() {
	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	defer cleanup()

	// Change to app directory
	if err := os.Chdir(appDir); err != nil {
		log("ERROR", "Failed to change directory to %s: %v", appDir, err)
		os.Exit(1)
	}

	// Load secrets from files
	loadSecret("APP_KEY")
	loadSecret("DB_PASSWORD")
	loadSecret("MAIL_PASSWORD")

	// Determine service type
	serviceType := "php-fpm"
	if len(os.Args) > 1 {
		serviceType = os.Args[1]
	}

	log("INFO", "Starting Pterodactyl Panel service: %s", serviceType)

	// Run migrations (only for php-fpm)
	if serviceType == "php-fpm" || strings.Contains(serviceType, "php-fpm") {
		if os.Getenv("RUN_MIGRATIONS_ON_START") == "true" {
			runMigrations()
		}

		if os.Getenv("RUN_SEED_ON_START") == "true" {
			runSeed()
		}
	}

	// Set proper permissions
	setPermissions()

	// For Caddy service, register with go-proxy before starting
	if serviceType == "caddy" {
		registerWithProxy()
	}

	// Start the appropriate service
	switch serviceType {
	case "php-fpm":
		startPHPFPM()
	case "caddy":
		startCaddy()
	case "queue":
		startQueue()
	case "cron":
		startCron(sigChan)
	default:
		// Pass through custom command
		log("INFO", "Executing custom command: %v", os.Args[1:])
		cmd := exec.Command(os.Args[1], os.Args[2:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			log("ERROR", "Command failed: %v", err)
			os.Exit(1)
		}
	}
}

func log(level, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	fmt.Printf("[%s] [%s] %s\n", timestamp, level, message)
}

func loadSecret(varName string) {
	fileVar := varName + "_FILE"
	filePath := os.Getenv(fileVar)

	if filePath == "" {
		return
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		log("WARN", "Failed to read secret %s: %v", varName, err)
		return
	}

	value := strings.TrimSpace(string(data))
	if value == "" {
		log("WARN", "Secret %s is empty", varName)
		return
	}

	os.Setenv(varName, value)
	log("INFO", "Loaded %s from secret", varName)
}

func runMigrations() {
	log("INFO", "Running database migrations...")

	cmd := exec.Command("php", "artisan", "migrate", "--force", "--isolated")
	cmd.Dir = appDir
	
	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		log("WARN", "Migrations failed or partially completed: %v", err)
		fmt.Println(string(output))
		return
	}

	log("INFO", "Migrations completed successfully")
}

func runSeed() {
	log("INFO", "Seeding database...")

	cmd := exec.Command("php", "artisan", "db:seed", "--force")
	cmd.Dir = appDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log("WARN", "Seeding failed: %v", err)
	}
}

func setPermissions() {
	log("INFO", "Setting permissions...")

	directories := []string{
		filepath.Join(appDir, "storage"),
		filepath.Join(appDir, "bootstrap/cache"),
	}

	for _, dir := range directories {
		// Change ownership to nginx:nginx (UID/GID 1001)
		if err := chownR(dir, 1001, 1001); err != nil {
			log("WARN", "Failed to set ownership for %s: %v", dir, err)
		}

		// Set permissions to 755
		if err := chmodR(dir, 0755); err != nil {
			log("WARN", "Failed to set permissions for %s: %v", dir, err)
		}
	}
}

func chownR(path string, uid, gid int) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(name, uid, gid)
	})
}

func chmodR(path string, mode os.FileMode) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chmod(name, mode)
	})
}

func startPHPFPM() {
	log("INFO", "Starting PHP-FPM...")

	cmd := exec.Command("/usr/sbin/php-fpm83", "-F", "-R")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		log("ERROR", "PHP-FPM failed: %v", err)
		os.Exit(1)
	}
}

func startCaddy() {
	log("INFO", "Starting Caddy web server...")

	cmd := exec.Command("/usr/sbin/caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		log("ERROR", "Caddy failed: %v", err)
		os.Exit(1)
	}
}

func startQueue() {
	log("INFO", "Starting Laravel queue worker...")

	for {
		cmd := exec.Command("php", "artisan", "queue:work",
			"--queue=high,standard,low",
			"--sleep=3",
			"--tries=3",
			"--max-time=3600")
		cmd.Dir = appDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log("WARN", "Queue worker exited: %v, restarting in 5s...", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log("INFO", "Queue worker exited cleanly, restarting in 5s...")
		time.Sleep(5 * time.Second)
	}
}

func startCron(sigChan chan os.Signal) {
	log("INFO", "Starting Laravel scheduler...")

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	runScheduler()

	for {
		select {
		case <-ticker.C:
			runScheduler()
		case sig := <-sigChan:
			log("INFO", "Received signal %v, shutting down scheduler...", sig)
			return
		}
	}
}

func runScheduler() {
	cmd := exec.Command("php", "artisan", "schedule:run")
	cmd.Dir = appDir

	// Discard output (silent execution)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		log("WARN", "Scheduler execution failed: %v", err)
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func cleanup() {
	log("INFO", "Shutting down, cleaning up...")
	if registryClientV2 != nil {
		log("INFO", "Closing registry connection...")
		registryClientV2.Shutdown()
		registryClientV2 = nil
	}
}

func registerWithProxy() {
	log("INFO", "Registering Pterodactyl Panel with go-proxy registry (V2 protocol)...")

	registryHost := getEnv("REGISTRY_HOST", "proxy")
	registryPort := getEnv("REGISTRY_PORT", "81")
	domains := getEnv("DOMAINS", "gpanel.chilla55.de")
	routePath := getEnv("ROUTE_PATH", "/")
	port := getEnv("PORT", "80")
	serviceName := getEnv("SERVICE_NAME", "pterodactyl")

	registryAddr := fmt.Sprintf("%s:%s", registryHost, registryPort)

	// Create metadata
	metadata := map[string]interface{}{
		"version": "1.0.0",
		"service": "pterodactyl-panel",
	}

	// Connect and register with V2 protocol
	var err error
	registryClientV2, err = NewRegistryClientV2(registryAddr, serviceName, "", 0, metadata)
	if err != nil {
		log("ERROR", "Failed to register with V2: %v", err)
		// Don't fail - continue without registry
		return
	}

	log("INFO", "Using container IP: %s", registryClientV2.GetLocalIP())

	// Register event handlers
	registryClientV2.On(EventConnected, func(event Event) {
		sessionID := event.Data["session_id"]
		localIP := event.Data["local_ip"]
		log("INFO", "✓ Connected to registry - Session: %v, IP: %v", sessionID, localIP)
	})

	registryClientV2.On(EventDisconnected, func(event Event) {
		reason := event.Data["reason"]
		log("WARN", "⚠ Disconnected from registry - Reason: %v", reason)
	})

	registryClientV2.On(EventRouteAdded, func(event Event) {
		routeID := event.Data["route_id"]
		domains := event.Data["domains"]
		backendURL := event.Data["backend_url"]
		log("INFO", "✓ Route registered - ID: %v", routeID)
		log("INFO", "  Domains: %v", domains)
		log("INFO", "  Backend: %v", backendURL)
	})

	registryClientV2.On(EventHealthCheckSet, func(event Event) {
		routeID := event.Data["route_id"]
		path := event.Data["path"]
		interval := event.Data["interval"]
		timeout := event.Data["timeout"]
		log("INFO", "✓ Health check configured for route %v", routeID)
		log("INFO", "  Path: %v, Interval: %v, Timeout: %v", path, interval, timeout)
	})

	registryClientV2.On(EventConfigApplied, func(event Event) {
		log("INFO", "✓ Configuration applied and active on proxy")
	})

	// Build backend URL using the detected IP and configured port
	backendURL := registryClientV2.BuildBackendURL(port)
	domainList := strings.Split(strings.ReplaceAll(domains, " ", ""), ",")

	routeID, err = registryClientV2.AddRoute(domainList, routePath, backendURL, 10)
	if err != nil {
		log("ERROR", "Failed to add route: %v", err)
		return
	}
	log("INFO", "Route added with ID: %s", routeID)
	log("INFO", "Backend URL: %s", backendURL)

	// Configure health check
	err = registryClientV2.SetHealthCheck(routeID, "/", "30s", "5s")
	if err != nil {
		log("WARN", "Warning: failed to set health check: %v", err)
	}

	// Configure options
	err = registryClientV2.SetOptions("compression", "true")
	if err != nil {
		log("WARN", "Warning: failed to set compression: %v", err)
	}

	// Apply all configuration
	err = registryClientV2.ApplyConfig()
	if err != nil {
		log("ERROR", "Failed to apply config: %v", err)
		return
	}

	log("INFO", "Pterodactyl Panel successfully registered with V2 protocol")

	// Start keepalive pinger
	go keepAliveLoop()
}

func keepAliveLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if registryClientV2 != nil {
				err := registryClientV2.Ping()
				if err != nil {
					log("WARN", "Keepalive ping failed: %v", err)
					// Try to reconnect
					go func() {
						time.Sleep(5 * time.Second)
						registerWithProxy()
					}()
				}
			}
		case <-done:
			return
		}
	}
}

