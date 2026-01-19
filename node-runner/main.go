package main

import (
    "archive/zip"
    "fmt"
    "io"
    "net"
    "os"
    "os/exec"
    "os/signal"
    "path/filepath"
    "strconv"
    "strings"
    "syscall"
    "time"

    registryclient "github.com/chilla55/registry-client/v2"
)

type config struct {
    AppDir         string
    EntryCommand   string
    NodeEnv        string
    EntryInstall   bool
    EntryBuild     bool
    AppPort        string
    ServiceName    string
    RegistryHost   string
    RegistryPort   string
    Domains        []string
    RoutePath      string
    HealthPath     string
    EnableRegistry bool
    WaitForPort    bool
    PortWaitTime   time.Duration
    ZipPath        string
    ZipStrip       int
    ZipClean       bool
}

func main() {
    cfg := loadConfig()

    if cfg.EntryCommand == "" {
        fatal("ENTRY_COMMAND env variable is required")
    }

    log("app dir: %s", cfg.AppDir)
    if err := os.MkdirAll(cfg.AppDir, 0755); err != nil {
        fatal("cannot create APP_DIR: %v", err)
    }

    if cfg.ZipPath != "" {
        if err := fetchAndExtractZip(cfg); err != nil {
            fatal("zip fetch/extract failed: %v", err)
        }
    }

    if cfg.EntryInstall && fileExists(filepathJoin(cfg.AppDir, "package.json")) {
        if err := installDeps(cfg); err != nil {
            fatal("npm install failed: %v", err)
        }
    }

    if cfg.EntryBuild && fileExists(filepathJoin(cfg.AppDir, "package.json")) {
        if err := buildApp(cfg); err != nil {
            fatal("npm run build failed: %v", err)
        }
    }

    cmd, procDone := startProcess(cfg)

    regClient, regRoute := startRegistry(cfg)
    defer shutdownRegistry(regClient)

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    select {
    case err := <-procDone:
        if err != nil {
            log("process exited with error: %v", err)
            os.Exit(1)
        }
        log("process exited cleanly")
    case sig := <-sigCh:
        log("received signal %v, stopping child...", sig)
        stopProcess(cmd)
    }

    if regRoute != "" {
        log("registry route %s removed on shutdown", regRoute)
    }
}

func loadConfig() config {
    return config{
        AppDir:         getEnv("APP_DIR", "/workspace"),
        EntryCommand:   os.Getenv("ENTRY_COMMAND"),
        NodeEnv:        getEnv("NODE_ENV", "production"),
        EntryInstall:   getBool("ENTRY_INSTALL", true),
        EntryBuild:     getBool("ENTRY_BUILD", false),
        AppPort:        getEnv("APP_PORT", "3000"),
        ServiceName:    getEnv("SERVICE_NAME", "nodeapp"),
        RegistryHost:   getEnv("REGISTRY_HOST", "proxy"),
        RegistryPort:   getEnv("REGISTRY_PORT", "81"),
        Domains:        splitAndTrim(getEnv("DOMAINS", "example.com")),
        RoutePath:      getEnv("ROUTE_PATH", "/"),
        HealthPath:     getEnv("HEALTH_PATH", "/health"),
        EnableRegistry: getBool("ENABLE_REGISTRY", true),
        WaitForPort:    getBool("WAIT_FOR_PORT", true),
        PortWaitTime:   getDuration("PORT_WAIT_TIMEOUT", 30*time.Second),
        ZipPath:        getEnv("ZIP_PATH", ""),
        ZipStrip:       getInt("ZIP_STRIP_COMPONENTS", 1),
        ZipClean:       getBool("ZIP_CLEAN", true),
    }
}

func startProcess(cfg config) (*exec.Cmd, <-chan error) {
    cmd := exec.Command("sh", "-c", cfg.EntryCommand)
    cmd.Dir = cfg.AppDir
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Env = append(os.Environ(), fmt.Sprintf("NODE_ENV=%s", cfg.NodeEnv))

    if err := cmd.Start(); err != nil {
        fatal("failed to start process: %v", err)
    }

    log("started process pid=%d", cmd.Process.Pid)

    done := make(chan error, 1)
    go func() {
        done <- cmd.Wait()
    }()

    return cmd, done
}

func stopProcess(cmd *exec.Cmd) {
    if cmd == nil || cmd.Process == nil {
        return
    }

    _ = cmd.Process.Signal(syscall.SIGTERM)
    timer := time.NewTimer(10 * time.Second)
    done := make(chan struct{})

    go func() {
        _, _ = cmd.Process.Wait()
        close(done)
    }()

    select {
    case <-done:
        log("process terminated")
    case <-timer.C:
        log("process did not stop in time, killing")
        _ = cmd.Process.Kill()
    }
}

func installDeps(cfg config) error {
    if fileExists(filepathJoin(cfg.AppDir, "package-lock.json")) {
        log("running npm ci")
        return runCmd(cfg.AppDir, "npm", "ci")
    }
    log("running npm install")
    return runCmd(cfg.AppDir, "npm", "install")
}

func buildApp(cfg config) error {
    log("running npm run build")
    return runCmd(cfg.AppDir, "npm", "run", "build")
}

func startRegistry(cfg config) (*registryclient.RegistryClientV2, string) {
    if !cfg.EnableRegistry {
        log("registry disabled; skipping registration")
        return nil, ""
    }

    if cfg.WaitForPort {
        if err := waitForPort("127.0.0.1", cfg.AppPort, cfg.PortWaitTime); err != nil {
            log("warning: app port not ready before registration: %v", err)
        }
    }

    addr := fmt.Sprintf("%s:%s", cfg.RegistryHost, cfg.RegistryPort)
    metadata := map[string]interface{}{
        "service": "node-runner",
        "entry_command": cfg.EntryCommand,
    }

    client := registryclient.NewRegistryClient(addr, cfg.ServiceName, "", 0, metadata, false)

    client.On(registryclient.EventLog, func(event registryclient.Event) {
        level := strings.ToUpper(fmt.Sprintf("%v", event.Data["level"]))
        msg := event.Data["message"]
        log("[registry/%s] %v", level, msg)
    })

    client.On(registryclient.EventError, func(event registryclient.Event) {
        log("[registry/ERROR] %v", event.Data["message"])
    })

    log("connecting to registry at %s", addr)
    if err := client.Init(); err != nil {
        log("registry init failed: %v", err)
        return nil, ""
    }

    backendURL := client.BuildBackendURL(cfg.AppPort)
    routeID, err := client.AddRoute(cfg.Domains, cfg.RoutePath, backendURL, 10)
    if err != nil {
        log("failed to add registry route: %v", err)
        return client, ""
    }

    if err := client.SetHealthCheck(routeID, cfg.HealthPath, "30s", "5s"); err != nil {
        log("warning: failed to set health check: %v", err)
    }

    _ = client.SetOptions("compression", "true")
    _ = client.SetOptions("http2", "true")

    if err := client.ApplyConfig(); err != nil {
        log("failed to apply registry config: %v", err)
        return client, routeID
    }

    log("registered route %s -> %s (%v)", routeID, backendURL, strings.Join(cfg.Domains, ","))

    go client.StartKeepalive()
    return client, routeID
}

func shutdownRegistry(client *registryclient.RegistryClientV2) {
    if client == nil {
        return
    }
    defer func() { _ = recover() }()
    client.Shutdown()
}

func waitForPort(host, port string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    target := net.JoinHostPort(host, port)
    for time.Now().Before(deadline) {
        conn, err := net.DialTimeout("tcp", target, 2*time.Second)
        if err == nil {
            _ = conn.Close()
            return nil
        }
        time.Sleep(2 * time.Second)
    }
    return fmt.Errorf("port %s not reachable within %s", target, timeout)
}

func runCmd(dir, name string, args ...string) error {
    cmd := exec.Command(name, args...)
    cmd.Dir = dir
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Env = os.Environ()
    return cmd.Run()
}

func getEnv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func getBool(key string, def bool) bool {
    if v := strings.ToLower(os.Getenv(key)); v != "" {
        switch v {
        case "1", "true", "yes", "on":
            return true
        case "0", "false", "no", "off":
            return false
        }
    }
    return def
}

func getDuration(key string, def time.Duration) time.Duration {
    if v := os.Getenv(key); v != "" {
        if d, err := time.ParseDuration(v); err == nil {
            return d
        }
    }
    return def
}

func getInt(key string, def int) int {
    if v := os.Getenv(key); v != "" {
        if i, err := strconv.Atoi(v); err == nil {
            return i
        }
    }
    return def
}

func fetchAndExtractZip(cfg config) error {
    zipFile := cfg.ZipPath
    log("extracting zip from %s", zipFile)

    info, err := os.Stat(zipFile)
    if err != nil {
        return fmt.Errorf("stat zip failed: %w", err)
    }
    if info.IsDir() {
        return fmt.Errorf("zip path is a directory: %s", zipFile)
    }

    appDirClean := filepath.Clean(cfg.AppDir)

    if cfg.ZipClean {
        if err := cleanDir(appDirClean); err != nil {
            return fmt.Errorf("clean app dir failed: %w", err)
        }
    }

    f, err := os.Open(zipFile)
    if err != nil {
        return fmt.Errorf("open zip failed: %w", err)
    }
    defer f.Close()

    zr, err := zip.NewReader(f, info.Size())
    if err != nil {
        return fmt.Errorf("read zip failed: %w", err)
    }

    for _, zf := range zr.File {
        rel := stripComponents(zf.Name, cfg.ZipStrip)
        if rel == "" {
            continue
        }

        destPath := filepath.Join(appDirClean, rel)
        destPath = filepath.Clean(destPath)

        if !strings.HasPrefix(destPath, appDirClean+string(os.PathSeparator)) && destPath != appDirClean {
            return fmt.Errorf("zip entry escapes app dir: %s", zf.Name)
        }

        if zf.FileInfo().IsDir() {
            if err := os.MkdirAll(destPath, 0755); err != nil {
                return fmt.Errorf("mkdir failed: %w", err)
            }
            continue
        }

        if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
            return fmt.Errorf("mkdir parents failed: %w", err)
        }

        rc, err := zf.Open()
        if err != nil {
            return fmt.Errorf("open entry failed: %w", err)
        }

        out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, zf.Mode())
        if err != nil {
            rc.Close()
            return fmt.Errorf("create file failed: %w", err)
        }

        if _, err := io.Copy(out, rc); err != nil {
            rc.Close()
            out.Close()
            return fmt.Errorf("copy failed: %w", err)
        }

        rc.Close()
        out.Close()
    }

    log("zip extracted into %s", cfg.AppDir)
    return nil
}

func stripComponents(path string, n int) string {
    if n < 0 {
        n = 0
    }
    parts := strings.Split(path, "/")
    if n >= len(parts) {
        return ""
    }
    out := parts[n:]
    if len(out) == 0 {
        return ""
    }
    cleaned := filepath.Join(out...)
    if cleaned == "." {
        return ""
    }
    return strings.TrimSpace(cleaned)
}

func cleanDir(dir string) error {
    entries, err := os.ReadDir(dir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return err
    }
    for _, e := range entries {
        p := filepath.Join(dir, e.Name())
        if err := os.RemoveAll(p); err != nil {
            return err
        }
    }
    return nil
}

func splitAndTrim(val string) []string {
    parts := strings.Split(val, ",")
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p != "" {
            out = append(out, p)
        }
    }
    return out
}

func fileExists(path string) bool {
    _, err := os.Stat(path)
    return err == nil
}

func filepathJoin(parts ...string) string {
    return filepath.Join(parts...)
}

func log(format string, args ...interface{}) {
    fmt.Printf("[node-runner] "+format+"\n", args...)
}

func fatal(format string, args ...interface{}) {
    log(format, args...)
    os.Exit(1)
}
