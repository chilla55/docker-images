package watcher

import (
	"context"
	"crypto/tls"
	"log"
	"path/filepath"
	"time"

	"github.com/chilla55/proxy-manager/config"
	"github.com/chilla55/proxy-manager/proxy"
	"github.com/fsnotify/fsnotify"
)

// CertWatcher watches certificate files for changes and reloads them
type CertWatcher struct {
	globalConfigPath string
	proxyServer      *proxy.Server
	debug            bool
	lastReload       time.Time
	reloadCooldown   time.Duration
}

// NewCertWatcher creates a new certificate watcher
func NewCertWatcher(globalConfigPath string, proxyServer *proxy.Server, debug bool) *CertWatcher {
	return &CertWatcher{
		globalConfigPath: globalConfigPath,
		proxyServer:      proxyServer,
		debug:            debug,
		reloadCooldown:   5 * time.Second, // Prevent rapid reloads
	}
}

// Start starts watching certificate files
func (w *CertWatcher) Start(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// Load global config to get certificate paths
	globalCfg, err := config.LoadGlobalConfig(w.globalConfigPath)
	if err != nil {
		return err
	}

	// Watch all certificate directories
	certDirs := make(map[string]bool)
	for _, certCfg := range globalCfg.TLS.Certificates {
		certDir := filepath.Dir(certCfg.CertFile)
		if !certDirs[certDir] {
			if err := watcher.Add(certDir); err != nil {
				log.Printf("[cert-watcher] Warning: Cannot watch %s: %s", certDir, err)
			} else {
				certDirs[certDir] = true
				if w.debug {
					log.Printf("[cert-watcher] Watching certificate directory: %s", certDir)
				}
			}
		}

		keyDir := filepath.Dir(certCfg.KeyFile)
		if !certDirs[keyDir] && keyDir != certDir {
			if err := watcher.Add(keyDir); err != nil {
				log.Printf("[cert-watcher] Warning: Cannot watch %s: %s", keyDir, err)
			} else {
				certDirs[keyDir] = true
				if w.debug {
					log.Printf("[cert-watcher] Watching key directory: %s", keyDir)
				}
			}
		}
	}

	if len(certDirs) == 0 {
		log.Println("[cert-watcher] No certificate directories to watch")
		return nil
	}

	log.Printf("[cert-watcher] Started watching %d certificate directories", len(certDirs))

	// Periodic reload check (every 5 minutes) as backup
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[cert-watcher] Stopping certificate watcher")
			return nil

		case <-ticker.C:
			if w.debug {
				log.Println("[cert-watcher] Periodic certificate check")
			}
			w.reloadCertificates()

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Watch for WRITE and CREATE events (certbot writes new files)
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				// Check if it's a certificate or key file
				if w.isCertFile(event.Name, globalCfg) {
					if w.debug {
						log.Printf("[cert-watcher] Certificate file changed: %s", event.Name)
					}

					// Debounce: wait a bit for certbot to finish writing all files
					time.Sleep(2 * time.Second)
					w.reloadCertificates()
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("[cert-watcher] Error: %s", err)
		}
	}
}

// isCertFile checks if the file is a certificate or key we're watching
func (w *CertWatcher) isCertFile(path string, cfg *config.GlobalConfig) bool {
	filename := filepath.Base(path)

	// Check if it's a fullchain.pem or privkey.pem (Let's Encrypt names)
	if filename == "fullchain.pem" || filename == "privkey.pem" {
		return true
	}

	// Check if it matches any configured cert/key path
	for _, certCfg := range cfg.TLS.Certificates {
		if path == certCfg.CertFile || path == certCfg.KeyFile {
			return true
		}
	}

	return false
}

// reloadCertificates reloads all certificates from disk
func (w *CertWatcher) reloadCertificates() {
	// Cooldown check to prevent rapid reloads
	if time.Since(w.lastReload) < w.reloadCooldown {
		if w.debug {
			log.Println("[cert-watcher] Skipping reload (cooldown active)")
		}
		return
	}

	log.Println("[cert-watcher] Reloading certificates from disk...")
	w.lastReload = time.Now()

	// Load global config
	globalCfg, err := config.LoadGlobalConfig(w.globalConfigPath)
	if err != nil {
		log.Printf("[cert-watcher] Failed to load global config: %s", err)
		return
	}

	// Load certificates
	certificates := make([]proxy.CertMapping, 0, len(globalCfg.TLS.Certificates))
	for i, certCfg := range globalCfg.TLS.Certificates {
		cert, err := tls.LoadX509KeyPair(certCfg.CertFile, certCfg.KeyFile)
		if err != nil {
			log.Printf("[cert-watcher] Failed to load certificate %d (%s): %s", i+1, certCfg.CertFile, err)
			continue
		}

		if len(certCfg.Domains) == 0 {
			log.Printf("[cert-watcher] Certificate %d has no domains defined", i+1)
			continue
		}

		mapping := proxy.CertMapping{
			Domains: certCfg.Domains,
			Cert:    cert,
		}

		certificates = append(certificates, mapping)
		if w.debug {
			log.Printf("[cert-watcher] Loaded certificate for domains: %v", certCfg.Domains)
		}
	}

	if len(certificates) == 0 {
		log.Println("[cert-watcher] Warning: No certificates loaded!")
		return
	}

	// Update certificates in proxy server
	w.proxyServer.UpdateCertificates(certificates)
	log.Printf("[cert-watcher] Successfully reloaded %d certificate(s)", len(certificates))
}
