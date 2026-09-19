package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func testRuntime(t *testing.T) *appRuntime {
	t.Helper()
	dir := t.TempDir()
	oldDir := appDir
	appDir = dir
	t.Cleanup(func() { appDir = oldDir })
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("SCHEDULER_ENABLED", "true")
	t.Setenv("DATABASE_URL", "test-inherited-database")
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"scheduler":"node scheduler.js"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s:%s\\n' \"$*\" \"$DATABASE_URL\" >> invocations\nexec sleep 60\n"
	if err := os.WriteFile(filepath.Join(dir, "npm"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	r := newAppRuntime()
	r.stateFile = filepath.Join(dir, "state.json")
	r.stopTimeout = time.Second
	r.webAddress = "127.0.0.1:0"
	t.Cleanup(r.close)
	return r
}
func processIDs(r *appRuntime) []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []int
	for _, p := range r.processes {
		if p.cmd != nil {
			result = append(result, p.cmd.Process.Pid)
		}
	}
	return result
}
func await(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(9 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for process transition")
}
func TestRuntimeLifecycle(t *testing.T) {
	r := testRuntime(t)
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	original := processIDs(r)
	if len(original) != 2 {
		t.Fatalf("wanted web and scheduler: %v", original)
	}
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	if ids := processIDs(r); ids[0] != original[0] || ids[1] != original[1] {
		t.Fatal("duplicate start replaced processes")
	}
	await(t, func() bool {
		data, _ := os.ReadFile(filepath.Join(appDir, "invocations"))
		return strings.Contains(string(data), "start:test-inherited-database") && strings.Contains(string(data), "run scheduler:test-inherited-database")
	})
	r.pause()
	for _, pid := range original {
		if syscall.Kill(pid, 0) == nil {
			t.Fatalf("process %d survived update pause", pid)
		}
	}
	time.Sleep(1100 * time.Millisecond)
	if len(processIDs(r)) != 0 {
		t.Fatal("process restarted while build was in progress")
	}
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	if len(processIDs(r)) != 2 {
		t.Fatal("both processes must restart after build")
	}
	r.fail()
	var state struct{ Mode string }
	data, err := os.ReadFile(r.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state.Mode != "failed" || len(processIDs(r)) != 0 {
		t.Fatal("failed build must stop workers and fail health")
	}
	r.close()
	if err := r.start(); err == nil {
		t.Fatal("start allowed after shutdown")
	}
}
func TestSchedulerCrashRestartsOnlyScheduler(t *testing.T) {
	r := testRuntime(t)
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	ids := processIDs(r)
	if err := syscall.Kill(ids[1], syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { next := processIDs(r); return len(next) == 2 && next[1] != ids[1] })
	if processIDs(r)[0] != ids[0] {
		t.Fatal("scheduler crash restarted healthy web server")
	}
}
func TestSchedulerScriptRequiredAndOptional(t *testing.T) {
	r := testRuntime(t)
	if err := os.WriteFile(filepath.Join(appDir, "package.json"), []byte(`{"scripts":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := r.start(); err == nil {
			t.Fatal("missing scheduler accepted")
		}
	}
	t.Setenv("SCHEDULER_ENABLED", "false")
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	if len(processIDs(r)) != 1 {
		t.Fatal("disabled scheduler was launched")
	}
}
func TestHealthcheckModes(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	script, err := os.ReadFile("healthcheck.sh")
	if err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "healthcheck.sh")
	if err := os.WriteFile(scriptPath, []byte(strings.ReplaceAll(string(script), runtimeStateFile, stateFile)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "curl"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("SCHEDULER_ENABLED", "true")
	for _, tc := range []struct {
		name, mode string
		scheduler  int
		healthy    bool
	}{
		{"build", "building", 0, true}, {"failure", "failed", 0, false},
		{"missing scheduler", "running", 0, false}, {"running", "running", os.Getpid(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(map[string]interface{}{"mode": tc.mode, "processes": map[string]int{"web": os.Getpid(), "scheduler": tc.scheduler}})
			if err := os.WriteFile(stateFile, data, 0644); err != nil {
				t.Fatal(err)
			}
			err := exec.Command("bash", scriptPath).Run()
			if (err == nil) != tc.healthy {
				t.Fatalf("healthcheck returned %v; expected healthy=%v", err, tc.healthy)
			}
		})
	}
}
func TestUpdateInterval(t *testing.T) {
	for input, want := range map[string]int{"0": 0, "300": 300, "-1": 300, "invalid": 300, "5garbage": 300} {
		if got := parseDuration(input); got != want {
			t.Fatalf("%q: got %d want %d", input, got, want)
		}
	}
}

// npm can exit before its Node child has flushed work and disconnected from DB.
func TestStopAllowsDescendantsToFinish(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	r := testRuntime(t)
	script := `#!/bin/sh
node -e '
const fs = require("fs");
process.on("SIGTERM", () => setTimeout(() => {
  fs.writeFileSync("stopped-" + process.pid, "graceful");
  process.exit(0);
}, 150));
fs.writeFileSync("ready-" + process.pid, "ready");
setInterval(() => {}, 1000);
' &
wait
`
	if err := os.WriteFile(filepath.Join(appDir, "npm"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { files, _ := filepath.Glob(filepath.Join(appDir, "ready-*")); return len(files) == 2 })
	r.pause()
	files, _ := filepath.Glob(filepath.Join(appDir, "stopped-*"))
	if len(files) != 2 {
		t.Fatalf("only %d descendants completed graceful shutdown", len(files))
	}
}

func TestUpdateReleasesWebPortBeforeRestart(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	r := testRuntime(t)
	t.Setenv("SCHEDULER_ENABLED", "false")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r.webAddress = listener.Addr().String()
	t.Setenv("TEST_WEB_PORT", fmt.Sprint(listener.Addr().(*net.TCPAddr).Port))
	listener.Close()
	// Model npm -> shell -> Node, with a web child that refuses graceful shutdown.
	script := "#!/bin/sh\nnode server.js &\nwait\n"
	server := `const fs = require('fs');
process.on('SIGTERM', () => {});
require('http').createServer((req, res) => res.end('OK')).listen(
 Number(process.env.TEST_WEB_PORT), '127.0.0.1', () => fs.writeFileSync('listening', String(process.pid)));
`
	if err := os.WriteFile(filepath.Join(appDir, "npm"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "server.js"), []byte(server), 0644); err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 2; cycle++ {
		if err := r.start(); err != nil {
			t.Fatal(err)
		}
		await(t, func() bool { _, err := os.Stat(filepath.Join(appDir, "listening")); return err == nil })
		conn, err := net.DialTimeout("tcp", r.webAddress, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		conn.Close()
		if err := r.pause(); err != nil {
			t.Fatal(err)
		}
		rebound, err := net.Listen("tcp", r.webAddress)
		if err != nil {
			t.Fatalf("cycle %d: orphaned child still holds web port: %v", cycle, err)
		}
		rebound.Close()
		if err := os.Remove(filepath.Join(appDir, "listening")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPortReleaseReportsOccupiedAddress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := waitForPortRelease(listener.Addr().String(), 100*time.Millisecond); err == nil {
		t.Fatal("occupied web address accepted")
	}
}
