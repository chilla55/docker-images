package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type CloudflareUpdater struct {
	interval        time.Duration
	nginxController NginxController
	debug           bool
	configPath      string
	lastIPs         string
}

type NginxController interface {
	ScheduleReload(reason string)
}

type cloudflareResponse struct {
	Result struct {
		IPv4CIDRs []string `json:"ipv4_cidrs"`
		IPv6CIDRs []string `json:"ipv6_cidrs"`
	} `json:"result"`
}

func NewCloudflareUpdater(interval time.Duration, nginxCtrl NginxController, debug bool) *CloudflareUpdater {
	return &CloudflareUpdater{
		interval:        interval,
		nginxController: nginxCtrl,
		debug:           debug,
		configPath:      "/etc/nginx/cloudflare-realip.conf",
	}
}

func (u *CloudflareUpdater) Start(ctx context.Context) {
	log.Println("[cf-updater] Starting Cloudflare IP updater")

	// Initial update
	u.updateCloudflareIPs()

	ticker := time.NewTicker(u.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[cf-updater] Stopping Cloudflare IP updater")
			return
		case <-ticker.C:
			u.updateCloudflareIPs()
		}
	}
}

func (u *CloudflareUpdater) updateCloudflareIPs() {
	if u.debug {
		log.Println("[cf-updater] Fetching Cloudflare IP ranges")
	}

	// Fetch IP ranges from Cloudflare API
	ipv4Ranges, err := u.fetchIPRanges("https://api.cloudflare.com/client/v4/ips")
	if err != nil {
		log.Printf("[cf-updater] Failed to fetch Cloudflare IPs: %s", err)
		return
	}

	// Generate configuration content
	var configLines []string
	configLines = append(configLines, "# Cloudflare IP ranges - auto-generated")
	configLines = append(configLines, fmt.Sprintf("# Last updated: %s", time.Now().Format(time.RFC3339)))
	configLines = append(configLines, "")

	for _, cidr := range ipv4Ranges {
		configLines = append(configLines, fmt.Sprintf("set_real_ip_from %s;", cidr))
	}

	configLines = append(configLines, "")
	configLines = append(configLines, "real_ip_header CF-Connecting-IP;")

	newContent := strings.Join(configLines, "\n") + "\n"

	// Check if content changed
	if newContent == u.lastIPs {
		if u.debug {
			log.Println("[cf-updater] Cloudflare IPs unchanged")
		}
		return
	}

	// Write configuration file
	if err := os.WriteFile(u.configPath, []byte(newContent), 0644); err != nil {
		log.Printf("[cf-updater] Failed to write config: %s", err)
		return
	}

	log.Printf("[cf-updater] Updated Cloudflare IP ranges (%d ranges)", len(ipv4Ranges))
	u.lastIPs = newContent
	u.nginxController.ScheduleReload("cloudflare IPs updated")
}

func (u *CloudflareUpdater) fetchIPRanges(url string) ([]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var cfResp cloudflareResponse
	if err := json.Unmarshal(body, &cfResp); err != nil {
		return nil, err
	}

	// Combine IPv4 and IPv6 ranges
	allRanges := append(cfResp.Result.IPv4CIDRs, cfResp.Result.IPv6CIDRs...)

	if len(allRanges) == 0 {
		return nil, fmt.Errorf("no IP ranges returned from Cloudflare API")
	}

	return allRanges, nil
}
