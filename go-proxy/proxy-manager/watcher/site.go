package watcher

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/chilla55/proxy-manager/config"
	"github.com/fsnotify/fsnotify"
)

type ProxyServer interface {
	AddRoute(domains []string, path, backendURL string, headers map[string]string, websocket bool, options map[string]interface{}) error
	RemoveRoute(domains []string, path string)
}

type SiteWatcher struct {
	sitesPath   string
	proxyServer ProxyServer
	debug       bool
	loadedSites map[string]*config.SiteConfig // filename -> config
}

func NewSiteWatcher(sitesPath string, proxyServer ProxyServer, debug bool) *SiteWatcher {
	return &SiteWatcher{
		sitesPath:   sitesPath,
		proxyServer: proxyServer,
		debug:       debug,
		loadedSites: make(map[string]*config.SiteConfig),
	}
}

func (w *SiteWatcher) Start(ctx context.Context) {
	// Initial load of all site configs
	w.loadAllSites()

	// Watch for changes
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[watcher] Failed to create file watcher: %s", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(w.sitesPath); err != nil {
		log.Printf("[watcher] Failed to watch directory: %s", err)
		return
	}

	log.Printf("[watcher] Watching %s for YAML config changes", w.sitesPath)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if filepath.Ext(event.Name) == ".yaml" || filepath.Ext(event.Name) == ".yml" {
					w.reloadSite(event.Name)
				}
			} else if event.Op&fsnotify.Remove != 0 {
				if filepath.Ext(event.Name) == ".yaml" || filepath.Ext(event.Name) == ".yml" {
					w.removeSite(event.Name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[watcher] Error: %s", err)
		}
	}
}

func (w *SiteWatcher) loadAllSites() {
	files, err := filepath.Glob(filepath.Join(w.sitesPath, "*.yaml"))
	if err != nil {
		log.Printf("[watcher] Failed to list YAML files: %s", err)
		return
	}

	ymlFiles, err := filepath.Glob(filepath.Join(w.sitesPath, "*.yml"))
	if err == nil {
		files = append(files, ymlFiles...)
	}

	for _, file := range files {
		w.loadSite(file)
	}
}

func (w *SiteWatcher) loadSite(filename string) {
	cfg, err := config.LoadSiteConfig(filename)
	if err != nil {
		log.Printf("[watcher] Failed to load %s: %s", filename, err)
		return
	}

	// Check if enabled
	if !cfg.Enabled {
		if w.debug {
			log.Printf("[watcher] Skipping disabled site: %s", filename)
		}
		// If it was previously loaded, remove it
		if oldCfg, exists := w.loadedSites[filename]; exists {
			w.removeSiteRoutes(oldCfg)
			delete(w.loadedSites, filename)
		}
		return
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		log.Printf("[watcher] Invalid config in %s: %s", filename, err)
		return
	}

	// Get parsed options
	options, err := cfg.GetOptions()
	if err != nil {
		log.Printf("[watcher] Invalid options in %s: %s", filename, err)
		return
	}

	// Remove old routes if this site was previously loaded
	if oldCfg, exists := w.loadedSites[filename]; exists {
		w.removeSiteRoutes(oldCfg)
	}

	// Add all routes
	for _, route := range cfg.Routes {
		// Merge global headers with route-specific headers
		headers := make(map[string]string)
		for k, v := range cfg.Headers {
			headers[k] = v
		}
		for k, v := range route.Headers {
			headers[k] = v
		}

		err := w.proxyServer.AddRoute(
			route.Domains,
			route.Path,
			route.Backend,
			headers,
			route.WebSocket,
			options,
		)

		if err != nil {
			log.Printf("[watcher] Failed to add route for %s: %s", filename, err)
			continue
		}

		if w.debug {
			log.Printf("[watcher] Loaded route: %v%s -> %s from %s",
				route.Domains, route.Path, route.Backend, filepath.Base(filename))
		}
	}

	// Store loaded config
	w.loadedSites[filename] = cfg
	log.Printf("[watcher] Loaded site config: %s (%d routes)", filepath.Base(filename), len(cfg.Routes))
}

func (w *SiteWatcher) reloadSite(filename string) {
	// Small delay to ensure file write is complete
	time.Sleep(100 * time.Millisecond)
	
	if w.debug {
		log.Printf("[watcher] Reloading %s", filepath.Base(filename))
	}
	
	w.loadSite(filename)
}

func (w *SiteWatcher) removeSite(filename string) {
	cfg, exists := w.loadedSites[filename]
	if !exists {
		return
	}

	w.removeSiteRoutes(cfg)
	delete(w.loadedSites, filename)
	
	log.Printf("[watcher] Removed site config: %s", filepath.Base(filename))
}

func (w *SiteWatcher) removeSiteRoutes(cfg *config.SiteConfig) {
	for _, route := range cfg.Routes {
		w.proxyServer.RemoveRoute(route.Domains, route.Path)
		
		if w.debug {
			log.Printf("[watcher] Removed route: %v%s", route.Domains, route.Path)
		}
	}
}
