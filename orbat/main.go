package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	repoURL            = "https://github.com/6th-Maroon-Division/Homepage.git"
	appDir             = "/app/repo"
	registryHost       = getEnv("REGISTRY_HOST", "proxy")
	registryPort       = getEnv("REGISTRY_PORT", "81")
	domainsStr         = getEnv("DOMAINS", "orbat.chilla55.de")
	routePath          = getEnv("ROUTE_PATH", "/")
	backendHost        = getEnv("BACKEND_HOST", "orbat")
	port               = getEnv("PORT", "3000")
	maintenancePort    = getEnv("MAINTENANCE_PORT", "3001")
	serviceName        = getEnv("SERVICE_NAME", "orbat")
	updateCheckIntvl   = getEnv("UPDATE_CHECK_INTERVAL", "300")
	maintenancePageURL = getEnv("MAINTENANCE_PAGE_URL", "") // Custom maintenance page URL

	registryClientV2 *RegistryClientV2
	routeID          string
	maintenancePID   int
	updateChkPID     int
	npmStartPID      int

	done = make(chan os.Signal, 1)
)

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func readSecret(secretName string) string {
	data, err := os.ReadFile("/run/secrets/" + secretName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func setupDatabaseURL() error {
	// If DATABASE_URL is already set, skip
	if os.Getenv("DATABASE_URL") != "" {
		return nil
	}

	// Read individual components
	dbHost := getEnv("DATABASE_HOST", "postgresql")
	dbPort := getEnv("DATABASE_PORT", "5432")
	dbName := getEnv("DATABASE_NAME", "orbat")
	dbUser := getEnv("DATABASE_USER", "orbat")
	dbSchema := getEnv("DATABASE_SCHEMA", "public")
	dbPassword := readSecret("database_password")

	if dbPassword == "" {
		return fmt.Errorf("database_password secret not found")
	}

	// Construct DATABASE_URL
	databaseURL := fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?schema=%s",
		dbUser,
		dbPassword,
		dbHost,
		dbPort,
		dbName,
		dbSchema,
	)

	os.Setenv("DATABASE_URL", databaseURL)
	log("DATABASE_URL configured from secrets and environment")
	return nil
}

func log(format string, args ...interface{}) {
	fmt.Printf("[Orbat] "+format+"\n", args...)
}

func cleanup() {
	log("Shutting down, closing persistent connection...")
	if registryClientV2 != nil {
		log("Shutting down registry connection...")
		registryClientV2.Shutdown()
		registryClientV2 = nil
	}
	if maintenancePID > 0 {
		syscall.Kill(maintenancePID, syscall.SIGTERM)
	}
	if updateChkPID > 0 {
		syscall.Kill(updateChkPID, syscall.SIGTERM)
	}
	if npmStartPID > 0 {
		syscall.Kill(npmStartPID, syscall.SIGTERM)
	}
}

// keepAliveLoop sends periodic pings to keep the V2 connection alive
func keepAliveLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if registryClientV2 != nil {
				err := registryClientV2.Ping()
				if err != nil {
					log("Keepalive ping failed: %v", err)
					// Try to reconnect
					go func() {
						time.Sleep(5 * time.Second)
						if err := registerWithProxy(); err != nil {
							log("Reconnection failed: %v", err)
						}
					}()
				}
			}
		case <-done:
			return
		}
	}
}

func registerWithProxy() error {
	log("Registering service with go-proxy registry (V2 protocol)...")

	registryAddr := fmt.Sprintf("%s:%s", registryHost, registryPort)

	// Create metadata
	metadata := map[string]interface{}{
		"version":     "1.0.0",
		"git_repo":    repoURL,
		"auto_update": true,
	}

	// Connect and register with V2 protocol
	instanceName := backendHost
	var err error
	registryClientV2, err = NewRegistryClientV2(registryAddr, serviceName, instanceName, 3001, metadata)
	if err != nil {
		return fmt.Errorf("failed to register with V2: %w", err)
	}

	// Add route
	backendURL := fmt.Sprintf("http://%s:%s", backendHost, port)
	domains := strings.Split(strings.ReplaceAll(domainsStr, " ", ""), ",")

	routeID, err = registryClientV2.AddRoute(domains, routePath, backendURL, 10)
	if err != nil {
		return fmt.Errorf("failed to add route: %w", err)
	}
	log("Route added with ID: %s", routeID)

	// Configure health check
	err = registryClientV2.SetHealthCheck(routeID, "/", "30s", "5s")
	if err != nil {
		log("Warning: failed to set health check: %v", err)
	}

	// Configure options
	err = registryClientV2.SetOptions("compression", "true")
	if err != nil {
		log("Warning: failed to set compression: %v", err)
	}

	// Apply all configuration
	err = registryClientV2.ApplyConfig()
	if err != nil {
		return fmt.Errorf("failed to apply config: %w", err)
	}

	log("Service successfully registered with V2 protocol")

	// Start keepalive pinger
	go keepAliveLoop()

	return nil
}

func enterProxyMaintenance() error {
	if registryClientV2 == nil {
		return fmt.Errorf("registry client not connected")
	}

	err := registryClientV2.MaintenanceEnterWithURL("ALL", maintenancePageURL)
	if err != nil {
		return fmt.Errorf("failed to enter maintenance: %w", err)
	}

	log("Proxy maintenance mode entered")
	if maintenancePageURL != "" {
		log("Using custom maintenance page: %s", maintenancePageURL)
	}
	return nil
}

func exitProxyMaintenance() error {
	if registryClientV2 == nil {
		return fmt.Errorf("registry client not connected")
	}

	for i := 0; i < 3; i++ {
		err := registryClientV2.MaintenanceExit("ALL")
		if err == nil {
			log("Proxy maintenance mode exited")
			return nil
		}
		log("Proxy maintenance exit retry %d: %v", i+1, err)
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("failed to exit maintenance after 3 retries")
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = appDir
	return cmd.Run()
}

func cloneRepo() error {
	log("First run - cloning repository...")
	return runCommand("git", "clone", repoURL, appDir)
}

func checkUpdates() error {
	log("Checking for updates...")
	if err := runCommand("git", "fetch", "origin", "main"); err != nil {
		log("WARNING: git fetch failed")
		return nil
	}

	// Get local and remote commits
	localCmd := exec.Command("git", "rev-parse", "HEAD")
	localCmd.Dir = appDir
	local, _ := localCmd.Output()

	remoteCmd := exec.Command("git", "rev-parse", "origin/main")
	remoteCmd.Dir = appDir
	remote, _ := remoteCmd.Output()

	if strings.TrimSpace(string(local)) != strings.TrimSpace(string(remote)) {
		log("Updates detected - pulling changes...")
		enterProxyMaintenance()
		if err := runCommand("git", "pull", "origin", "main"); err != nil {
			log("WARNING: git pull failed")
			return nil
		}
	} else {
		log("No updates found")
	}
	return nil
}

func buildApp() error {
	log("Installing dependencies...")
	if err := runCommand("npm", "ci", "--production=false"); err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}

	log("Generating Prisma client...")
	if err := runCommand("npx", "prisma", "generate"); err != nil {
		return fmt.Errorf("prisma generate failed: %w", err)
	}

	log("Running database migrations...")
	if err := runCommand("npx", "prisma", "migrate", "deploy"); err != nil {
		return fmt.Errorf("prisma migrate deploy failed: %w", err)
	}

	log("Building Next.js application...")
	if err := runCommand("npm", "run", "build"); err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}

	return nil
}

func startMaintenanceServer() error {
	log("Starting maintenance server on port %s...", maintenancePort)
	cmd := exec.Command("node", "-e", `
		const http = require('http');
		const fs = require('fs');
		const port = `+maintenancePort+`;
		
		const server = http.createServer((req, res) => {
			if (req.url === '/api/status') {
				res.writeHead(200, {'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*'});
				res.end('{"step":"startup","message":"Starting up...","progress":50}');
				return;
			}
			res.writeHead(503, {'Content-Type': 'text/html'});
			res.end('<html><body><h1>Orbat is starting...</h1></body></html>');
		});
		
		server.listen(port, () => {
			console.log('Maintenance server running on port ' + port);
		});
	`)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}
	maintenancePID = cmd.Process.Pid
	log("Maintenance server ready on port %s (PID: %d)", maintenancePort, maintenancePID)
	return nil
}

func stopMaintenanceServer() {
	log("Stopping maintenance server...")
	if maintenancePID > 0 {
		syscall.Kill(maintenancePID, syscall.SIGTERM)
		maintenancePID = 0
	}
}

func startUpdateChecker() {
	go func() {
		log("Starting background update checker...")
		for {
			time.Sleep(time.Duration(parseDuration(updateCheckIntvl)) * time.Second)
			log("Checking for updates (periodic check)...")
			if err := checkUpdates(); err != nil {
				log("Update check error: %v", err)
			}
		}
	}()
}

func parseDuration(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}

func startNPMServer() {
	go func() {
		for {
			log("Launching Next.js...")
			cmd := exec.Command("npm", "start")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Dir = appDir

			if err := cmd.Start(); err != nil {
				log("Failed to start Next.js: %v (retrying in 5s)", err)
				time.Sleep(5 * time.Second)
				continue
			}

			npmStartPID = cmd.Process.Pid
			if err := cmd.Wait(); err != nil {
				log("Next.js exited (code: %v), restarting in 5s...", err)
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

func main() {
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	defer cleanup()

	log("Starting entrypoint...")
	log("Service: %s, Port: %s, Maintenance: %s", serviceName, port, maintenancePort)

	// Setup DATABASE_URL from secrets before any database operations
	if err := setupDatabaseURL(); err != nil {
		log("WARNING: Failed to setup DATABASE_URL: %v", err)
	}

	// Ensure app directory exists
	os.MkdirAll(appDir, 0755)

	// PRIORITY 1: Register with proxy immediately
	log("PRIORITY 1: Registering with proxy...")
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		if err := registerWithProxy(); err != nil {
			log("Registration attempt %d/%d failed: %v", i+1, maxRetries, err)
			if i < maxRetries-1 {
				time.Sleep(2 * time.Second)
				continue
			}
			log("ERROR: Failed to register after %d attempts", maxRetries)
			time.Sleep(30 * time.Second)
			os.Exit(1)
		}
		log("Successfully registered with proxy")
		break
	}

	// PRIORITY 2: Start maintenance page immediately
	log("PRIORITY 2: Starting maintenance server...")
	if err := startMaintenanceServer(); err != nil {
		log("ERROR: Failed to start maintenance server: %v", err)
		time.Sleep(30 * time.Second)
		os.Exit(1)
	}

	// Enter proxy maintenance mode to show loading page
	enterProxyMaintenance()

	// BACKGROUND: Clone/build app while maintenance page is visible
	go func() {
		log("BACKGROUND: Building application...")

		// Clone or check updates
		if _, err := os.Stat(filepath.Join(appDir, ".git")); os.IsNotExist(err) {
			if err := cloneRepo(); err != nil {
				log("ERROR: Failed to clone repository: %v", err)
				return
			}
		} else {
			if err := checkUpdates(); err != nil {
				log("ERROR: Failed to check updates: %v", err)
				return
			}
		}

		// Build app
		if err := buildApp(); err != nil {
			log("ERROR: Build failed: %v", err)
			return
		}

		log("Build complete, exiting maintenance mode...")
		exitProxyMaintenance()
		stopMaintenanceServer()

		// Start update checker
		startUpdateChecker()

		// Start npm server
		log("Starting application supervisor...")
		startNPMServer()
	}()

	// Keep connection alive and wait for signals
	for {
		select {
		case <-done:
			return
		case <-time.After(10 * time.Second):
			// Keep-alive interval - handled by keepAliveLoop
		}
	}
}
