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

	registryclient "github.com/chilla55/registry-client/v2"
)

const (
	appDir      = "/var/www/pelican"
	envDir      = "/pelican-env"
	panelTarURL = "https://github.com/pelican-dev/panel/releases/latest/download/panel.tar.gz"
)

var (
	registryClientV2 *registryclient.RegistryClientV2
	done             = make(chan os.Signal, 1)
)

func main() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	defer cleanup()

	// Load secrets from files
	loadSecret("APP_KEY")
	loadSecret("DB_PASSWORD")
	loadSecret("MAIL_PASSWORD")

	// Determine service type
	serviceType := "php-fpm"
	if len(os.Args) > 1 {
		serviceType = os.Args[1]
	}

	log("INFO", "Starting Pelican Panel service: %s", serviceType)

	switch serviceType {
	case "php-fpm":
		// php-fpm is responsible for installing/updating the panel into the shared volume
		firstLaunch := !panelInstallComplete()
		ensurePanelInstalled()
		ensureEnvSymlink()
		if os.Getenv("SKIP_ENV_INJECTION") == "true" {
			log("INFO", "SKIP_ENV_INJECTION=true, skipping managed .env sync on all launches")
		} else if firstLaunch && os.Getenv("SKIP_ENV_INJECTION_ON_FIRST_LAUNCH") == "true" {
			log("INFO", "SKIP_ENV_INJECTION_ON_FIRST_LAUNCH=true, skipping managed .env sync on first launch")
		} else {
			syncManagedEnvFile()
		}
		if os.Getenv("RUN_MIGRATIONS_ON_START") == "true" {
			runMigrations()
		}
		if os.Getenv("RUN_SEED_ON_START") == "true" {
			runSeed()
		}
		setPermissions()
		startPHPFPM()

	case "caddy":
		// Wait until php-fpm has finished installing the panel into the shared volume
		waitForPanel()
		ensureEnvSymlink()
		registerWithProxy()
		startCaddy()

	case "queue":
		waitForPanel()
		ensureEnvSymlink()
		startQueue()

	case "cron":
		waitForPanel()
		ensureEnvSymlink()
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

// ─── Panel Install / Update ───────────────────────────────────────────────────

func ensureEnvSymlink() {
	target := filepath.Join(envDir, ".env")
	link := filepath.Join(appDir, ".env")

	if err := os.MkdirAll(envDir, 0755); err != nil {
		log("WARN", "Failed to create env directory %s: %v", envDir, err)
		return
	}

	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if currentTarget, err := os.Readlink(link); err == nil && currentTarget == target {
				return
			}
			if err := os.Remove(link); err != nil {
				log("WARN", "Failed to replace existing .env symlink: %v", err)
				return
			}
		} else {
			if !fileExists(target) {
				if err := os.Rename(link, target); err != nil {
					log("WARN", "Failed to move existing .env into env volume: %v", err)
				} else {
					log("INFO", "Moved existing .env into dedicated env volume")
				}
			}

			if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
				log("WARN", "Failed to remove existing .env before linking: %v", err)
				return
			}
		}
	}

	if err := os.Symlink(target, link); err != nil && !os.IsExist(err) {
		log("WARN", "Failed to create .env symlink: %v", err)
		return
	}
}

func syncManagedEnvFile() {
	envPath := filepath.Join(envDir, ".env")
	managedKeys := []string{
		"APP_ENV",
		"APP_DEBUG",
		"APP_URL",
		"APP_TIMEZONE",
		"APP_LOCALE",
		"APP_KEY",
		"CACHE_DRIVER",
		"SESSION_DRIVER",
		"QUEUE_CONNECTION",
		"DB_CONNECTION",
		"DB_HOST",
		"DB_PORT",
		"DB_DATABASE",
		"DB_USERNAME",
		"DB_PASSWORD",
		"REDIS_HOST",
		"REDIS_PORT",
		"REDIS_PASSWORD",
		"MAIL_MAILER",
		"MAIL_HOST",
		"MAIL_PORT",
		"MAIL_USERNAME",
		"MAIL_PASSWORD",
		"MAIL_ENCRYPTION",
		"MAIL_FROM_ADDRESS",
		"MAIL_FROM_NAME",
		"SESSION_SECURE_COOKIE",
		"TRUSTED_PROXIES",
	}

	content, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		log("WARN", "Failed to read .env for sync: %v", err)
		return
	}

	lines := []string{}
	if len(content) > 0 {
		lines = strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	}

	updatedKeys := make(map[string]bool, len(managedKeys))
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		equalsIndex := strings.Index(line, "=")
		if equalsIndex <= 0 {
			continue
		}

		key := strings.TrimSpace(line[:equalsIndex])
		value, ok := managedEnvValue(key)
		if !ok {
			continue
		}

		lines[index] = key + "=" + quoteEnvValue(value)
		updatedKeys[key] = true
	}

	for _, key := range managedKeys {
		if updatedKeys[key] {
			continue
		}
		value, ok := managedEnvValue(key)
		if !ok {
			continue
		}
		lines = append(lines, key+"="+quoteEnvValue(value))
	}

	output := strings.Join(lines, "\n")
	if output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	if err := os.WriteFile(envPath, []byte(output), 0640); err != nil {
		log("WARN", "Failed to write synced .env: %v", err)
		return
	}

	log("INFO", "Synced managed runtime values into %s", envPath)
}

func managedEnvValue(key string) (string, bool) {
	value := os.Getenv(key)
	if value == "" {
		return "", false
	}
	return value, true
}

func quoteEnvValue(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}

// ensurePanelInstalled checks if the panel is present in the volume.
// If not, it installs it. If UPDATE_ON_START=true and panel already exists, it updates.
func ensurePanelInstalled() {
	if !panelInstallComplete() {
		if fileExists(filepath.Join(appDir, "artisan")) {
			log("WARN", "Panel files are incomplete (missing vendor/autoload.php) — repairing install...")
		} else {
			log("INFO", "Panel not found in volume — running first-time install...")
		}
		if err := installPanel(); err != nil {
			log("ERROR", "Panel installation failed: %v", err)
			os.Exit(1)
		}
		return
	}

	if os.Getenv("UPDATE_ON_START") == "true" {
		log("INFO", "UPDATE_ON_START=true — updating panel to latest release...")
		if err := updatePanel(); err != nil {
			log("WARN", "Panel update failed (continuing with existing install): %v", err)
		}
	} else {
		log("INFO", "Panel already installed — skipping download (set UPDATE_ON_START=true to update)")
	}
}

// installPanel downloads and extracts the panel, then runs composer install.
func installPanel() error {
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return fmt.Errorf("failed to create app directory: %w", err)
	}

	log("INFO", "Downloading panel from %s...", panelTarURL)
	if err := downloadAndExtract(panelTarURL, appDir); err != nil {
		return fmt.Errorf("download/extract failed: %w", err)
	}

	if err := runComposerInstall(); err != nil {
		return fmt.Errorf("composer install failed: %w", err)
	}

	log("INFO", "Panel installed successfully")
	return nil
}

// updatePanel downloads the latest release over the existing install, then re-runs composer.
func updatePanel() error {
	log("INFO", "Downloading latest panel release over existing install...")
	if err := downloadAndExtract(panelTarURL, appDir); err != nil {
		return fmt.Errorf("download/extract failed: %w", err)
	}

	if err := runComposerInstall(); err != nil {
		return fmt.Errorf("composer install failed: %w", err)
	}

	// Clear Laravel caches after update
	runArtisan("config:clear")
	runArtisan("route:clear")
	runArtisan("view:clear")

	log("INFO", "Panel updated successfully")
	return nil
}

// downloadAndExtract pipes curl directly into tar to avoid writing a temp file.
func downloadAndExtract(url, dest string) error {
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf(`curl -fsSL %q | tar -xz -C %q`, url, dest))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runComposerInstall runs composer install --no-dev in appDir.
func runComposerInstall() error {
	log("INFO", "Running composer install...")
	cmd := exec.Command("composer", "install",
		"--no-dev",
		"--optimize-autoloader",
		"--no-interaction",
		"--prefer-dist",
	)
	cmd.Dir = appDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "COMPOSER_ALLOW_SUPERUSER=1")
	return cmd.Run()
}

// runArtisan runs a php artisan command, logging a warning on failure.
func runArtisan(args ...string) {
	cmd := exec.Command("php", append([]string{"artisan"}, args...)...)
	cmd.Dir = appDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log("WARN", "artisan %s failed: %v", strings.Join(args, " "), err)
	}
}

// waitForPanel blocks until appDir/artisan exists (written by the php-fpm service).
// Times out after 10 minutes to avoid an infinite hang.
func waitForPanel() {
	timeout := 10 * time.Minute
	deadline := time.Now().Add(timeout)

	log("INFO", "Waiting for panel to be installed by php-fpm service...")
	for {
		if panelInstallComplete() {
			log("INFO", "Panel ready — proceeding")
			return
		}
		if time.Now().After(deadline) {
			log("ERROR", "Timed out after %v waiting for panel installation", timeout)
			os.Exit(1)
		}
		time.Sleep(5 * time.Second)
	}
}

func panelInstallComplete() bool {
	return fileExists(filepath.Join(appDir, "artisan")) && fileExists(filepath.Join(appDir, "vendor/autoload.php"))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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

	cmd := exec.Command("/usr/sbin/php-fpm85", "-F", "-R")
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
	log("INFO", "Registering Pelican Panel with go-proxy registry (V2 protocol)...")

	registryHost := getEnv("REGISTRY_HOST", "proxy")
	registryPort := getEnv("REGISTRY_PORT", "81")
	domains := getEnv("DOMAINS", "panel.chilla55.de")
	routePath := getEnv("ROUTE_PATH", "/")
	port := getEnv("PORT", "80")
	serviceName := getEnv("SERVICE_NAME", "pelican")

	registryAddr := fmt.Sprintf("%s:%s", registryHost, registryPort)

	// Create metadata
	metadata := map[string]interface{}{
		"version": "1.0.0",
		"service": "pelican-panel",
	}

	// Create client with debug logging enabled
	registryClientV2 = registryclient.NewRegistryClient(registryAddr, serviceName, "", 0, metadata, true)

	// Register event handlers
	registryClientV2.On(registryclient.EventLog, func(event registryclient.Event) {
		level := strings.ToUpper(fmt.Sprintf("%v", event.Data["level"]))
		message := event.Data["message"]
		log(level, "[Registry] %v", message)
	})

	registryClientV2.On(registryclient.EventError, func(event registryclient.Event) {
		message := event.Data["message"]
		log("ERROR", "[Registry] %v", message)
	})

	registryClientV2.On(registryclient.EventIPChanged, func(event registryclient.Event) {
		oldIP := event.Data["old_ip"]
		newIP := event.Data["new_ip"]
		log("WARN", "⚠ IP address changed: %v → %v (cleaning up stale routes)", oldIP, newIP)
	})

	// Connect and register (with automatic cleanup of old routes)
	if err := registryClientV2.Init(); err != nil {
		log("ERROR", "Failed to initialize registry client: %v", err)
		// Don't fail - continue without registry
		return
	}

	log("INFO", "Using container IP: %s", registryClientV2.GetLocalIP())

	// Build backend URL using the detected IP and configured port
	backendURL := registryClientV2.BuildBackendURL(port)
	domainList := strings.Split(strings.ReplaceAll(domains, " ", ""), ",")

	routeID, err := registryClientV2.AddRoute(domainList, routePath, backendURL, 10)
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

	log("INFO", "Successfully registered with V2 protocol")

	// Start automatic keepalive with retry logic
	go registryClientV2.StartKeepalive()
}
