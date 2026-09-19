package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const runtimeStateFile = "/tmp/orbat-runtime.json"

type managedProcess struct {
	name    string
	args    []string
	cmd     *exec.Cmd
	done    chan error
	retryAt time.Time
}

type appRuntime struct {
	mu          sync.Mutex
	once        sync.Once
	processes   []*managedProcess
	mode        string
	closed      bool
	stateFile   string
	stopTimeout time.Duration
	webAddress  string
}

func newAppRuntime() *appRuntime {
	return &appRuntime{mode: "building", stateFile: runtimeStateFile, stopTimeout: 150 * time.Second, webAddress: net.JoinHostPort("", port)}
}

// All process changes and state snapshots are serialized with mu. Wait runs
// independently so stopping a process never waits for the supervisor's lock.
func (r *appRuntime) writeState() {
	pids := map[string]int{}
	for _, p := range r.processes {
		if p.cmd != nil {
			pids[p.name] = p.cmd.Process.Pid
		} else {
			pids[p.name] = 0
		}
	}
	data, _ := json.Marshal(struct {
		Mode      string         `json:"mode"`
		Processes map[string]int `json:"processes"`
	}{r.mode, pids})
	if err := os.WriteFile(r.stateFile+".tmp", data, 0644); err == nil {
		err = os.Rename(r.stateFile+".tmp", r.stateFile)
		if err != nil {
			log("Cannot publish runtime health: %v", err)
		}
	} else {
		log("Cannot write runtime health: %v", err)
	}
}

func validateScheduler() error {
	data, err := os.ReadFile(filepath.Join(appDir, "package.json"))
	if err != nil {
		return err
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return err
	}
	if pkg.Scripts["scheduler"] == "" {
		return fmt.Errorf("package.json has no scheduler script; deploy scheduler code or set SCHEDULER_ENABLED=false")
	}
	return nil
}

func (r *appRuntime) start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("runtime is shutting down")
	}
	if r.mode == "running" {
		return nil
	}
	if getEnv("SCHEDULER_ENABLED", "true") != "false" {
		if err := validateScheduler(); err != nil {
			return err
		}
	}
	if r.processes == nil {
		r.processes = []*managedProcess{{name: "web", args: []string{"start"}}}
		if getEnv("SCHEDULER_ENABLED", "true") != "false" {
			r.processes = append(r.processes, &managedProcess{name: "scheduler", args: []string{"run", "scheduler"}})
		}
	}
	if err := waitForPortRelease(r.webAddress, 5*time.Second); err != nil {
		return err
	}
	r.mode = "running"
	for _, p := range r.processes {
		r.launch(p)
	}
	r.writeState()
	r.once.Do(func() { go r.monitor() })
	return nil
}

func (r *appRuntime) launch(p *managedProcess) {
	cmd := exec.Command("npm", p.args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = appDir, os.Stdout, os.Stderr
	// npm spawns a shell and Node children. Signal the entire process group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		log("Cannot start %s: %v", p.name, err)
		p.retryAt = time.Now().Add(5 * time.Second)
		return
	}
	p.cmd, p.done = cmd, make(chan error, 1)
	exited := p.done
	go func() { exited <- cmd.Wait() }()
	log("Started %s (PID %d)", p.name, cmd.Process.Pid)
}

func (r *appRuntime) monitor() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return
		}
		if r.mode == "running" {
			for _, p := range r.processes {
				if p.cmd != nil {
					select {
					case err := <-p.done:
						log("%s exited (%v); restarting in 5s", p.name, err)
						// A dead npm parent can leave Node children behind.
						_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
						p.cmd = nil
						p.retryAt = time.Now().Add(5 * time.Second)
					default:
					}
				} else if !time.Now().Before(p.retryAt) {
					r.launch(p)
				}
			}
			r.writeState()
		}
		r.mu.Unlock()
	}
}

func (r *appRuntime) stop(mode string, closing bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = closing
	r.mode = mode
	// Publish maintenance before waiting for long-running scheduler transactions.
	r.writeState()
	// Send TERM to both groups before waiting for either one.
	for _, p := range r.processes {
		if p.cmd != nil {
			_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
		}
	}
	deadline := time.Now().Add(r.stopTimeout)
	for _, p := range r.processes {
		if p.cmd == nil {
			continue
		}
		timer := time.NewTimer(maxDuration(time.Until(deadline)))
		select {
		case <-p.done:
		case <-timer.C:
		}
		timer.Stop()
		for time.Now().Before(deadline) && syscall.Kill(-p.cmd.Process.Pid, 0) == nil {
			time.Sleep(50 * time.Millisecond)
		}
		// Also remove descendants that ignored TERM after their npm parent exited.
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		p.cmd = nil
	}
	r.writeState()
}

func maxDuration(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}

// SIGKILL delivery is asynchronous. Confirm the listening socket is gone before
// modifying the application's files or allowing another server to start.
func waitForPortRelease(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		listener, err := net.Listen("tcp", address)
		if err == nil {
			return listener.Close()
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("web address %s is still unavailable after shutdown: %w", address, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (r *appRuntime) pause() error {
	r.stop("building", false)
	return waitForPortRelease(r.webAddress, 5*time.Second)
}
func (r *appRuntime) fail()  { r.stop("failed", false) }
func (r *appRuntime) close() { r.stop("stopped", true) }
