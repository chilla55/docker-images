package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	repoURL           = "https://github.com/6th-Maroon-Division/Homepage.git"
	appDir            = "/app/repo"
	registryHost      = getEnv("REGISTRY_HOST", "proxy")
	registryPort      = getEnv("REGISTRY_PORT", "81")
	domainsStr        = getEnv("DOMAINS", "orbat.chilla55.de")
	routePath         = getEnv("ROUTE_PATH", "/")
	backendHost       = getEnv("BACKEND_HOST", "orbat")
	port              = getEnv("PORT", "3000")
	maintenancePort   = getEnv("MAINTENANCE_PORT", "3001")
	serviceName       = getEnv("SERVICE_NAME", "orbat")
	updateCheckIntvl  = getEnv("UPDATE_CHECK_INTERVAL", "300")

	registryConn  net.Conn
	registryMutex sync.Mutex
	sessionID     string
	maintenancePID int
	updateChkPID  int
	npmStartPID   int

	done = make(chan os.Signal, 1)
)

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func log(format string, args ...interface{}) {
	fmt.Printf("[Orbat] "+format+"\n", args...)
}

func cleanup() {
	log("Shutting down, closing persistent connection...")
	if registryConn != nil {
		if sessionID != "" {
			sendRegistryCommand("SHUTDOWN|" + sessionID)
		}
		registryConn.Close()
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

func connectRegistry() error {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	if registryConn != nil {
		return nil
	}

	conn, err := net.DialTimeout("tcp", registryHost+":"+registryPort, 10*time.Second)
	if err != nil {
		log("Failed to connect to registry: %v", err)
		return err
	}

	registryConn = conn
	log("Connected to registry at %s:%s", registryHost, registryPort)
	return nil
}

func sendRegistryCommand(cmd string) (string, error) {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	if registryConn == nil {
		return "", fmt.Errorf("registry not connected")
	}

	// Set write deadline
	registryConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := io.WriteString(registryConn, cmd+"\n")
	if err != nil {
		log("Failed to send command: %v", err)
		registryConn.Close()
		registryConn = nil
		return "", err
	}

	// Set read deadline
	registryConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(registryConn)
	response, err := reader.ReadString('\n')
	if err != nil {
		log("Failed to read response: %v", err)
		registryConn.Close()
		registryConn = nil
		return "", err
	}

	return strings.TrimSpace(response), nil
}

func registerWithProxy() error {
	log("Registering service with go-proxy registry...")

	if err := connectRegistry(); err != nil {
		return err
	}

	// Try to reconnect with existing session
	if sessionID != "" {
		resp, err := sendRegistryCommand("RECONNECT|" + sessionID)
		if err == nil && resp == "OK" {
			log("Reconnected to registry with session: %s", sessionID)
			return nil
		}
		log("Reconnect failed, re-registering")
		sessionID = ""
	}

	// Register new service
	backendURL := fmt.Sprintf("http://%s:%s", backendHost, port)
	registerCmd := fmt.Sprintf("REGISTER|%s|%s|%s|%s", serviceName, backendHost, port, maintenancePort)
	response, err := sendRegistryCommand(registerCmd)
	if err != nil {
		return err
	}

	// Parse response: ACK|<session-id>
	if !strings.HasPrefix(response, "ACK|") {
		return fmt.Errorf("registration failed: %s", response)
	}
	sessionID = strings.TrimPrefix(response, "ACK|")
	log("Registered with session: %s", sessionID)

	// Register route
	domains := strings.ReplaceAll(domainsStr, " ", "")
	routeCmd := fmt.Sprintf("ROUTE|%s|%s|%s|%s", sessionID, domains, routePath, backendURL)
	resp, err := sendRegistryCommand(routeCmd)
	if err != nil {
		return err
	}
	log("Route registration: %s", resp)

	// Set options
	sendRegistryCommand("OPTIONS|" + sessionID + "|timeout|60s")
	sendRegistryCommand("OPTIONS|" + sessionID + "|compression|true")
	sendRegistryCommand("OPTIONS|" + sessionID + "|http2|true")

	return nil
}

func enterProxyMaintenance() error {
	if sessionID == "" {
		return nil
	}
	resp, err := sendRegistryCommand("MAINT_ENTER|" + sessionID)
	if err != nil {
		return err
	}
	log("Proxy maintenance enter: %s", resp)
	return nil
}

func exitProxyMaintenance() error {
	if sessionID == "" {
		return nil
	}
	for i := 0; i < 3; i++ {
		resp, err := sendRegistryCommand("MAINT_EXIT|" + sessionID)
		if err == nil && resp == "MAINT_OK" {
			log("Proxy maintenance exit acknowledged")
			return nil
		}
		log("Proxy maintenance exit retry %d", i+1)
		time.Sleep(1 * time.Second)
	}
	return nil
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
			npmStartPID = cmd.Process.Pid
			
			if err := cmd.Run(); err != nil {
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
		log("Successfully registered with proxy (session: %s)", sessionID)
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
			// Periodically check if registry connection is still alive
			if registryConn != nil {
				registryMutex.Lock()
				// Test connection with a simple operation
				registryMutex.Unlock()
			}
		}
	}
}
