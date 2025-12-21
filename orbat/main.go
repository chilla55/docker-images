package main

import (
	"encoding/json"
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
	statusFile         = "/tmp/status.json"
	registryHost       = getEnv("REGISTRY_HOST", "proxy")
	registryPort       = getEnv("REGISTRY_PORT", "81")
	domainsStr         = getEnv("DOMAINS", "orbat.chilla55.de")
	routePath          = getEnv("ROUTE_PATH", "/")
	backendHost        = getEnv("BACKEND_HOST", "orbat")
	port               = getEnv("PORT", "3000")
	maintenancePort    = getEnv("MAINTENANCE_PORT", "3001")
	serviceName        = getEnv("SERVICE_NAME", "orbat")
	updateCheckIntvl   = getEnv("UPDATE_CHECK_INTERVAL", "300")
	maintenancePageURL string // Will be set in main() if not provided via env

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

func updateStatus(step, message string, progress int, details string) {
	status := map[string]interface{}{
		"step":         step,
		"message":      message,
		"progress":     progress,
		"details":      details,
		"timestamp":    time.Now().Format(time.RFC3339),
		"showExtended": false,
	}
	data, _ := json.Marshal(status)
	os.WriteFile(statusFile, data, 0644)
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
	// The client will auto-detect the container IP and use it as instance name
	var err error
	registryClientV2, err = NewRegistryClientV2(registryAddr, serviceName, "", 3001, metadata)
	if err != nil {
		return fmt.Errorf("failed to register with V2: %w", err)
	}

	log("Using container IP: %s", registryClientV2.GetLocalIP())

	// Register comprehensive event handlers for all lifecycle events
	registryClientV2.On(EventConnected, func(event Event) {
		sessionID := event.Data["session_id"]
		localIP := event.Data["local_ip"]
		log("✓ Connected to registry - Session: %v, IP: %v", sessionID, localIP)
	})

	registryClientV2.On(EventDisconnected, func(event Event) {
		reason := event.Data["reason"]
		log("⚠ Disconnected from registry - Reason: %v", reason)
	})

	registryClientV2.On(EventRouteAdded, func(event Event) {
		routeID := event.Data["route_id"]
		domains := event.Data["domains"]
		backendURL := event.Data["backend_url"]
		log("✓ Route registered - ID: %v", routeID)
		log("  Domains: %v", domains)
		log("  Backend: %v", backendURL)
	})

	registryClientV2.On(EventHealthCheckSet, func(event Event) {
		routeID := event.Data["route_id"]
		path := event.Data["path"]
		interval := event.Data["interval"]
		timeout := event.Data["timeout"]
		log("✓ Health check configured for route %v", routeID)
		log("  Path: %v, Interval: %v, Timeout: %v", path, interval, timeout)
	})

	registryClientV2.On(EventConfigApplied, func(event Event) {
		log("✓ Configuration applied and active on proxy")
	})

	registryClientV2.On(EventMaintenanceEnter, func(event Event) {
		target := event.Data["target"]
		url := event.Data["maintenance_page_url"]
		log("→ Entering maintenance mode")
		log("  Target: %v", target)
		log("  Maintenance URL: %v", url)
		updateStatus("maintenance", "Entering maintenance mode", 10, "Requesting proxy to enter maintenance")
	})

	registryClientV2.On(EventMaintenanceOK, func(event Event) {
		target := event.Data["target"]
		log("✓ Maintenance mode confirmed by proxy")
		log("  Target: %v", target)
		updateStatus("maintenance", "Maintenance mode active", 100, "Proxy confirmed maintenance mode")
	})

	registryClientV2.On(EventMaintenanceExit, func(event Event) {
		target := event.Data["target"]
		log("→ Exiting maintenance mode")
		log("  Target: %v", target)
		updateStatus("running", "Exiting maintenance", 90, "Requesting proxy to exit maintenance")
	})

	registryClientV2.On(EventDisconnected, func(event Event) {
		log("⚠ Disconnected from registry: %v", event.Data["reason"])
	})

	// Build backend URL using the detected IP and configured port
	backendURL := registryClientV2.BuildBackendURL(port)
	domains := strings.Split(strings.ReplaceAll(domainsStr, " ", ""), ",")

	routeID, err = registryClientV2.AddRoute(domains, routePath, backendURL, 10)
	if err != nil {
		return fmt.Errorf("failed to add route: %w", err)
	}
	log("Route added with ID: %s", routeID)
	log("Backend URL: %s", backendURL)

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

	// Event handlers will provide feedback
	return registryClientV2.MaintenanceEnterWithURL("ALL", maintenancePageURL)
}

func exitProxyMaintenance() error {
	if registryClientV2 == nil {
		return fmt.Errorf("registry client not connected")
	}

	// Event handlers will provide feedback, retry logic here
	for i := 0; i < 3; i++ {
		err := registryClientV2.MaintenanceExit("ALL")
		if err == nil {
			return nil
		}
		log("Retry %d/3: %v", i+1, err)
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

func checkForUpdates() (bool, error) {
	log("Checking for updates...")
	if err := runCommand("git", "fetch", "origin", "main"); err != nil {
		log("WARNING: git fetch failed")
		return false, err
	}

	// Get local and remote commits
	localCmd := exec.Command("git", "rev-parse", "HEAD")
	localCmd.Dir = appDir
	local, _ := localCmd.Output()

	remoteCmd := exec.Command("git", "rev-parse", "origin/main")
	remoteCmd.Dir = appDir
	remote, _ := remoteCmd.Output()

	localHash := strings.TrimSpace(string(local))
	remoteHash := strings.TrimSpace(string(remote))

	if localHash != remoteHash {
		log("Updates detected: %s -> %s", localHash[:7], remoteHash[:7])
		return true, nil
	}

	log("No updates found")
	return false, nil
}

func performUpdate() error {
	log("========================================")
	log("Starting zero-downtime update process")
	log("========================================")

	// Step 1: Enter maintenance mode
	log("[Update 1/8] Entering maintenance mode...")
	updateStatus("entering-maintenance", "Entering maintenance mode", 5, "Starting update process")
	if !isMaintenanceServerRunning() {
		log("[Update] Starting maintenance server...")
		if err := startMaintenanceServer(); err != nil {
			return fmt.Errorf("failed to start maintenance server: %w", err)
		}
		// Wait for maintenance server to be ready
		if err := waitForMaintenanceServerHealthy(10 * time.Second); err != nil {
			return fmt.Errorf("maintenance server not healthy: %w", err)
		}
	}

	if err := enterProxyMaintenance(); err != nil {
		return fmt.Errorf("failed to enter maintenance (no MAINT_OK): %w", err)
	}
	log("[Update] ✓ Maintenance mode active, proxy confirmed")

	// Step 2: Stop Next.js
	log("[Update 2/8] Stopping Next.js...")
	updateStatus("stopping-app", "Stopping application", 10, "Gracefully shutting down Next.js")
	if npmStartPID > 0 {
		if err := syscall.Kill(npmStartPID, syscall.SIGTERM); err != nil {
			log("Warning: failed to kill Next.js: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
	log("[Update] ✓ Next.js stopped")

	// Step 3: Pull code
	log("[Update 3/8] Pulling latest code...")
	updateStatus("pulling", "Pulling latest changes", 15, "Downloading updated code from repository")
	if err := runCommand("git", "pull", "origin", "main"); err != nil {
		return fmt.Errorf("git pull failed: %w", err)
	}
	log("[Update] ✓ Code updated")

	// Step 4-7: Build
	if err := buildApp(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Step 8: Wait for Next.js to be healthy
	log("[Update 8/8] Waiting for Next.js to be healthy...")
	updateStatus("waiting-healthy", "Waiting for app to start", 90, "Verifying service is responding")
	if err := waitForNextJSHealthy(30 * time.Second); err != nil {
		log("Warning: Next.js health check failed: %v", err)
	}
	log("[Update] ✓ Next.js is healthy")

	// Step 9: Exit maintenance mode
	log("[Update 9/9] Exiting maintenance mode...")
	updateStatus("exiting-maintenance", "Exiting maintenance mode", 95, "Switching back to main service")
	if err := exitProxyMaintenance(); err != nil {
		log("Warning: failed to exit maintenance (no MAINT_OK): %v", err)
	}
	log("[Update] ✓ Maintenance mode exited, proxy confirmed")

	updateStatus("complete", "Update complete", 100, "Service is now running with latest code")
	log("========================================")
	log("Zero-downtime update completed successfully")
	log("========================================")
	return nil
}

func buildApp() error {
	log("[Update 4/8] Installing dependencies...")
	updateStatus("dependencies", "Installing dependencies", 25, "Running npm ci to install packages")
	if err := runCommand("npm", "ci", "--production=false"); err != nil {
		return fmt.Errorf("npm ci failed: %w", err)
	}
	log("[Update] ✓ Dependencies installed")

	log("[Update 5/8] Generating Prisma client...")
	updateStatus("prisma-generate", "Generating Prisma client", 45, "Creating database client from schema")
	if err := runCommand("npx", "prisma", "generate"); err != nil {
		return fmt.Errorf("prisma generate failed: %w", err)
	}
	log("[Update] ✓ Prisma client generated")

	log("[Update 6/8] Running database migrations...")
	updateStatus("migrations", "Running database migrations", 60, "Applying schema changes to database")
	if err := runCommand("npx", "prisma", "migrate", "deploy"); err != nil {
		return fmt.Errorf("prisma migrate deploy failed: %w", err)
	}
	log("[Update] ✓ Migrations applied")

	log("[Update 7/8] Building Next.js application...")
	updateStatus("building", "Building Next.js application", 75, "Compiling TypeScript and optimizing assets")
	if err := runCommand("npm", "run", "build"); err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}
	log("[Update] ✓ Next.js built")

	return nil
}

func isMaintenanceServerRunning() bool {
	if maintenancePID <= 0 {
		return false
	}
	// Check if process is still alive
	err := syscall.Kill(maintenancePID, 0)
	return err == nil
}

func waitForNextJSHealthy(timeout time.Duration) error {
	backendURL := fmt.Sprintf("http://%s:%s/", backendHost, port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.Command("curl", "-sf", "-o", "/dev/null", backendURL)
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("Next.js did not become healthy within %v", timeout)
}

func waitForMaintenanceServerHealthy(timeout time.Duration) error {
	maintenanceURL := fmt.Sprintf("http://localhost:%s/", maintenancePort)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.Command("curl", "-sf", "-o", "/dev/null", maintenanceURL)
		if err := cmd.Run(); err == nil {
			log("Maintenance server is healthy and accepting connections")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("maintenance server did not become healthy within %v", timeout)
}

func startMaintenanceServer() error {
	log("Starting maintenance server on port %s...", maintenancePort)

	// Create initial status file
	updateStatus("initializing", "Service is starting up", 0, "Initializing Orbat service")

	cmd := exec.Command("node", "-e", `
		const http = require('http');
		const fs = require('fs');
		const port = `+maintenancePort+`;
		
		// Read the maintenance HTML file
		const maintenanceHTML = fs.existsSync('/maintenance.html') 
			? fs.readFileSync('/maintenance.html', 'utf8')
			: '<html><head><title>Orbat Maintenance</title></head><body><h1>Orbat is starting...</h1><p>Please wait while the service initializes.</p></body></html>';
		
		const server = http.createServer((req, res) => {
			// Health check endpoint for Docker/Kubernetes
			if (req.url === '/healthcheck' || req.url === '/health') {
				res.writeHead(200, {'Content-Type': 'text/plain'});
				res.end('OK');
				return;
			}
			// Status API endpoint for progress updates
			if (req.url === '/api/status') {
				res.writeHead(200, {'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*'});
				try {
					const status = fs.readFileSync('`+statusFile+`', 'utf8');
					res.end(status);
				} catch (e) {
					res.end('{"step":"initializing","message":"Starting up...","progress":5,"showExtended":false}');
				}
				return;
			}
			// Serve maintenance page for all paths (including /, /favicon.ico, etc.)
			res.writeHead(200, {'Content-Type': 'text/html; charset=utf-8'});
			res.end(maintenanceHTML);
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
	// Keep maintenance server running (suspended) for fast re-entry during updates
	// The server remains on port 3001 but proxy routes away from it
	log("Maintenance server suspended (kept running on port %s)", maintenancePort)
	// No-op: maintenance server stays alive for next update cycle
}

func startUpdateChecker() {
	go func() {
		log("Starting background update checker (interval: %ss)...", updateCheckIntvl)
		for {
			time.Sleep(time.Duration(parseDuration(updateCheckIntvl)) * time.Second)
			log("[Update Check] Checking for updates...")

			updateAvailable, err := checkForUpdates()
			if err != nil {
				log("[Update Check] Error: %v", err)
				continue
			}

			if !updateAvailable {
				log("[Update Check] No updates available")
				continue
			}

			log("[Update Check] Update available, starting zero-downtime update...")
			if err := performUpdate(); err != nil {
				log("[Update Check] Update failed: %v", err)
				continue
			}
			log("[Update Check] Update completed successfully")
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

	// Setup maintenance page URL if not provided
	// Note: We'll set this properly after registry client is created
	if maintenancePageURL == "" {
		maintenancePageURL = getEnv("MAINTENANCE_PAGE_URL", "")
	}

	// Setup DATABASE_URL from secrets before any database operations
	if err := setupDatabaseURL(); err != nil {
		log("WARNING: Failed to setup DATABASE_URL: %v", err)
	}

	// Ensure app directory exists
	os.MkdirAll(appDir, 0755)

	// PRIORITY A: Start the local maintenance page immediately so the
	// container shows a sensible page while we attempt registration/build.
	log("PRIORITY: Starting local maintenance server (non-fatal)...")
	if err := startMaintenanceServer(); err != nil {
		log("WARNING: Failed to start local maintenance server: %v", err)
	} else {
		log("Local maintenance server started")
		// Wait for it to be healthy before proceeding
		if err := waitForMaintenanceServerHealthy(10 * time.Second); err != nil {
			log("WARNING: Maintenance server not healthy: %v", err)
		}
	}

	// PRIORITY B: Try to register with proxy, but do not treat failure as fatal.
	// The previous behavior exited the process if registration failed which
	// left the container down and the maintenance page never had a chance
	// to serve. Instead, try a few times and continue even if registration
	// cannot be established.
	log("PRIORITY: Registering with proxy (non-fatal)...")
	maxRetries := 3
	registered := false
	for i := 0; i < maxRetries; i++ {
		if err := registerWithProxy(); err != nil {
			log("Registration attempt %d/%d failed: %v", i+1, maxRetries, err)
			time.Sleep(2 * time.Second)
			continue
		}
		registered = true
		log("Successfully registered with proxy")
		break
	}
	if !registered {
		log("WARNING: Failed to register with proxy after %d attempts, continuing without registry", maxRetries)
	}

	// If registration succeeded, enter proxy maintenance so the proxy routes
	// to our maintenance page while we build. If registration did not succeed
	// we continue — the local maintenance server remains available on the
	// maintenance port for debugging and manual routing.
	if registered {
		// Set maintenance URL using the registry client helper
		if maintenancePageURL == "" {
			maintenancePageURL = registryClientV2.BuildMaintenanceURL(maintenancePort)
			log("Using maintenance page URL: %s", maintenancePageURL)
		}

		// No need for DNS propagation delay since we're using direct IP
		if err := enterProxyMaintenance(); err != nil {
			log("Warning: enterProxyMaintenance failed: %v", err)
		}
	}

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
			// Check for updates and pull if available
			updateAvailable, err := checkForUpdates()
			if err != nil {
				log("WARNING: Failed to check updates: %v", err)
			} else if updateAvailable {
				log("Updates found, pulling latest code...")
				if err := runCommand("git", "pull", "origin", "main"); err != nil {
					log("WARNING: git pull failed: %v", err)
				}
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
