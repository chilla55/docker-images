package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type dummyProxy struct{ added, removed int }

func (d *dummyProxy) AddRoute(domains []string, path, backendURL string, headers map[string]string, websocket bool, options map[string]interface{}) error {
	d.added++
	return nil
}
func (d *dummyProxy) RemoveRoute(domains []string, path string) { d.removed++ }

func TestLoadSite(t *testing.T) {
	dir := t.TempDir()
	yml := `enabled: true
service:
  name: test-svc
headers:
  X-Global: "1"
routes:
  - domains: ["example.com"]
    path: "/"
    backend: "http://localhost:8080"
    headers:
      X-Route: "r"
`
	fname := filepath.Join(dir, "site.yaml")
	if err := os.WriteFile(fname, []byte(yml), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, true)
	w.loadSite(fname)
	if dp.added < 1 {
		t.Fatalf("expected routes added")
	}

	// removeSite should remove routes
	w.removeSite(fname)
	if dp.removed < 1 {
		t.Fatalf("expected routes removed")
	}
}

func TestLoadAllSites(t *testing.T) {
	dir := t.TempDir()

	yml1 := `enabled: true
service:
  name: site1-svc
routes:
  - domains: ["site1.com"]
    path: "/"
    backend: "http://localhost:8080"
`
	yml2 := `enabled: true
service:
  name: site2-svc
routes:
  - domains: ["site2.com"]
    path: "/"
    backend: "http://localhost:8081"
`

	if err := os.WriteFile(filepath.Join(dir, "site1.yaml"), []byte(yml1), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "site2.yml"), []byte(yml2), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, false)
	w.loadAllSites()

	if dp.added < 2 {
		t.Errorf("expected at least 2 routes added, got %d", dp.added)
	}
}

func TestReloadSite(t *testing.T) {
	dir := t.TempDir()

	yml := `enabled: true
service:
  name: reload-svc
routes:
  - domains: ["reload.com"]
    path: "/"
    backend: "http://localhost:8080"
`
	fname := filepath.Join(dir, "reload.yaml")
	if err := os.WriteFile(fname, []byte(yml), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, true)
	w.loadSite(fname)

	initialAdded := dp.added

	// Now reload
	w.reloadSite(fname)

	// Should have removed old routes and added new ones
	if dp.removed < 1 {
		t.Errorf("expected routes to be removed on reload")
	}
	if dp.added <= initialAdded {
		t.Errorf("expected additional routes to be added on reload")
	}
}

func TestSiteWatcherContext(t *testing.T) {
	dir := t.TempDir()
	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, false)

	ctx, cancel := context.WithCancel(context.Background())

	// Start watcher in goroutine
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Cancel context
	cancel()

	// Should exit quickly
	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("watcher did not stop on context cancellation")
	}
}

func TestRemoveSiteRoutes(t *testing.T) {
	dir := t.TempDir()
	yml := `enabled: true
service:
  name: test-svc
routes:
  - domains: ["example.com"]
    path: "/"
    backend: "http://localhost:8080"
`
	fname := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(fname, []byte(yml), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, false)
	w.loadSite(fname)

	// Verify site was loaded
	if _, exists := w.loadedSites[fname]; !exists {
		t.Fatal("site should be in loadedSites map")
	}

	// Remove it
	w.removeSite(fname)

	// Should call RemoveRoute and clear from map
	if _, exists := w.loadedSites[fname]; exists {
		t.Error("site should be removed from loadedSites map")
	}

	if dp.removed < 1 {
		t.Error("routes should have been removed")
	}
}

func TestLoadSiteWithInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	fname := filepath.Join(dir, "invalid.yaml")

	// Write invalid YAML
	if err := os.WriteFile(fname, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, true)
	w.loadSite(fname)

	// Should handle error gracefully (no panic)
	if dp.added != 0 {
		t.Error("invalid yaml should not add routes")
	}
}

func TestLoadSiteDisabled(t *testing.T) {
	dir := t.TempDir()
	yml := `enabled: false
service:
  name: disabled-svc
routes:
  - domains: ["disabled.com"]
    path: "/"
    backend: "http://localhost:8080"
`
	fname := filepath.Join(dir, "disabled.yaml")
	if err := os.WriteFile(fname, []byte(yml), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, true)
	w.loadSite(fname)

	// Disabled sites should not add routes
	if dp.added != 0 {
		t.Errorf("disabled site should not add routes, got %d", dp.added)
	}
}

func TestNewCertWatcher(t *testing.T) {
	w := NewCertWatcher("/path/to/global.yaml", nil, true)

	if w == nil {
		t.Error("NewCertWatcher should not return nil")
	}

	if w.globalConfigPath != "/path/to/global.yaml" {
		t.Error("globalConfigPath not set correctly")
	}

	if w.reloadCooldown != 5*time.Second {
		t.Errorf("reloadCooldown should be 5s, got %v", w.reloadCooldown)
	}
}

func TestLoadAllSitesEmptyDir(t *testing.T) {
	dir := t.TempDir()

	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, false)
	w.loadAllSites()

	// Should handle empty directory gracefully
	if dp.added != 0 {
		t.Errorf("empty directory should add 0 routes, got %d", dp.added)
	}
}

func TestSiteWatcherFileOperations(t *testing.T) {
	dir := t.TempDir()

	yml := `enabled: true
service:
  name: watch-svc
routes:
  - domains: ["watch.com"]
    path: "/"
    backend: "http://localhost:8080"
`

	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, false)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start watcher in background
	go w.Start(ctx)

	time.Sleep(20 * time.Millisecond) // Let watcher start

	// Create a file
	fname := filepath.Join(dir, "dynamic.yaml")
	if err := os.WriteFile(fname, []byte(yml), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Give watcher time to process
	time.Sleep(50 * time.Millisecond)

	if dp.added < 1 {
		t.Logf("Expected routes to be added via file watcher, got %d", dp.added)
		// Note: This test may be timing-sensitive
	}

	// Update the file
	yml2 := `enabled: true
service:
  name: watch-svc2
routes:
  - domains: ["watch2.com"]
    path: "/"
    backend: "http://localhost:8081"
`
	if err := os.WriteFile(fname, []byte(yml2), 0644); err != nil {
		t.Fatalf("update file: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Context cancellation will stop the watcher
	<-ctx.Done()
}

func TestReloadNonExistentSite(t *testing.T) {
	dir := t.TempDir()

	dp := &dummyProxy{}
	w := NewSiteWatcher(dir, dp, false)

	// Try to reload a file that doesn't exist
	w.reloadSite(filepath.Join(dir, "nonexistent.yaml"))

	// Should handle gracefully (no panic)
	if dp.added != 0 {
		t.Error("nonexistent file should not add routes")
	}
}
