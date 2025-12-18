package nginx

import (
	"log"
	"os/exec"
	"sync"
	"time"
)

// Controller manages nginx reload operations and batches multiple reload requests
type Controller struct {
	mu              sync.Mutex
	pendingReload   bool
	reloadTimer     *time.Timer
	batchWindow     time.Duration
	debug           bool
}

func NewController(debug bool) *Controller {
	return &Controller{
		batchWindow: 5 * time.Second,
		debug:       debug,
	}
}

// ScheduleReload schedules a nginx reload, batching requests within the batch window
func (c *Controller) ScheduleReload(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.debug {
		log.Printf("[nginx] Reload scheduled: %s", reason)
	}

	if c.pendingReload {
		if c.debug {
			log.Printf("[nginx] Reload already pending, batching this request")
		}
		return
	}

	c.pendingReload = true
	c.reloadTimer = time.AfterFunc(c.batchWindow, func() {
		c.executeReload(reason)
		c.mu.Lock()
		c.pendingReload = false
		c.mu.Unlock()
	})
}

// ReloadNow performs an immediate reload without batching
func (c *Controller) ReloadNow(reason string) error {
	c.mu.Lock()
	if c.reloadTimer != nil {
		c.reloadTimer.Stop()
	}
	c.pendingReload = false
	c.mu.Unlock()

	return c.executeReload(reason)
}

func (c *Controller) executeReload(reason string) error {
	log.Printf("[nginx] Reloading nginx: %s", reason)

	// Test configuration first
	cmd := exec.Command("nginx", "-t")
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[nginx] Configuration test failed: %s\n%s", err, output)
		return err
	}

	// Reload nginx
	cmd = exec.Command("nginx", "-s", "reload")
	if err := cmd.Run(); err != nil {
		// Try HUP signal as fallback
		cmd = exec.Command("kill", "-HUP", "1")
		if err := cmd.Run(); err != nil {
			log.Printf("[nginx] Reload failed: %s", err)
			return err
		}
	}

	log.Printf("[nginx] Reload successful: %s", reason)
	return nil
}

// TestConfig tests the nginx configuration without reloading
func (c *Controller) TestConfig() error {
	cmd := exec.Command("nginx", "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if c.debug {
			log.Printf("[nginx] Config test failed: %s\n%s", err, output)
		}
		return err
	}
	return nil
}
