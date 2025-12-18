package watcher

import (
	"os"
	"path/filepath"
	"testing"
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
