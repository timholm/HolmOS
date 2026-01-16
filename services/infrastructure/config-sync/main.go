package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Catppuccin Mocha colors
const (
	ColorBase     = "#1e1e2e"
	ColorMantle   = "#181825"
	ColorCrust    = "#11111b"
	ColorText     = "#cdd6f4"
	ColorSubtext0 = "#a6adc8"
	ColorGreen    = "#a6e3a1"
	ColorRed      = "#f38ba8"
	ColorBlue     = "#89b4fa"
	ColorMauve    = "#cba6f7"
	ColorTeal     = "#94e2d5"
)

type Logger struct {
	mu sync.Mutex
}

func (l *Logger) Log(level, msg string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     level,
		"service":   "config-sync",
		"message":   msg,
	}
	for k, v := range fields {
		entry[k] = v
	}
	json.NewEncoder(os.Stdout).Encode(entry)
}

var (
	logger          = &Logger{}
	requestCounter  uint64
	configsSynced   uint64
	lastSyncTime    time.Time
	watchedConfigs  = make(map[string]time.Time)
	configMu        sync.RWMutex
	watcher         *fsnotify.Watcher
)

func initWatcher() error {
	var err error
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/etc/holm/config"
	}

	// Check if path exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logger.Log("warn", "Config path does not exist, will create watcher when available", map[string]interface{}{"path": configPath})
		return nil
	}

	if err := watcher.Add(configPath); err != nil {
		return fmt.Errorf("failed to watch path: %w", err)
	}

	logger.Log("info", "Watching config path", map[string]interface{}{"path": configPath})
	return nil
}

func syncConfigs() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/etc/holm/config"
	}

	files, err := filepath.Glob(filepath.Join(configPath, "*"))
	if err != nil {
		logger.Log("error", "Failed to glob config files", map[string]interface{}{"error": err.Error()})
		return
	}

	configMu.Lock()
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		lastMod, exists := watchedConfigs[file]
		if !exists || info.ModTime().After(lastMod) {
			watchedConfigs[file] = info.ModTime()
			atomic.AddUint64(&configsSynced, 1)
			logger.Log("info", "Config synced", map[string]interface{}{"file": filepath.Base(file)})
		}
	}
	lastSyncTime = time.Now()
	configMu.Unlock()
}

func startWatcher(ctx context.Context) {
	if watcher == nil {
		return
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					logger.Log("info", "Config file changed", map[string]interface{}{"file": event.Name, "op": event.Op.String()})
					syncConfigs()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Log("error", "Watcher error", map[string]interface{}{"error": err.Error()})
			case <-ctx.Done():
				return
			}
		}
	}()

	// Periodic sync
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				syncConfigs()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCounter, 1)

	configMu.RLock()
	configCount := len(watchedConfigs)
	lastSync := lastSyncTime
	configMu.RUnlock()

	status := map[string]interface{}{
		"service":       "config-sync",
		"status":        "healthy",
		"configs":       configCount,
		"last_sync":     lastSync.Format(time.RFC3339),
		"watcher_active": watcher != nil,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCounter, 1)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	configMu.RLock()
	configCount := len(watchedConfigs)
	lastSync := lastSyncTime
	configMu.RUnlock()

	watcherActive := 0
	if watcher != nil {
		watcherActive = 1
	}

	fmt.Fprintf(w, "# HELP config_sync_up Whether the service is up\n")
	fmt.Fprintf(w, "# TYPE config_sync_up gauge\n")
	fmt.Fprintf(w, "config_sync_up 1\n")

	fmt.Fprintf(w, "# HELP config_sync_watcher_active Whether file watcher is active\n")
	fmt.Fprintf(w, "# TYPE config_sync_watcher_active gauge\n")
	fmt.Fprintf(w, "config_sync_watcher_active %d\n", watcherActive)

	fmt.Fprintf(w, "# HELP config_sync_configs_total Number of watched configs\n")
	fmt.Fprintf(w, "# TYPE config_sync_configs_total gauge\n")
	fmt.Fprintf(w, "config_sync_configs_total %d\n", configCount)

	fmt.Fprintf(w, "# HELP config_sync_synced_total Total configs synced\n")
	fmt.Fprintf(w, "# TYPE config_sync_synced_total counter\n")
	fmt.Fprintf(w, "config_sync_synced_total %d\n", atomic.LoadUint64(&configsSynced))

	fmt.Fprintf(w, "# HELP config_sync_last_sync_timestamp Last sync timestamp\n")
	fmt.Fprintf(w, "# TYPE config_sync_last_sync_timestamp gauge\n")
	fmt.Fprintf(w, "config_sync_last_sync_timestamp %d\n", lastSync.Unix())

	fmt.Fprintf(w, "# HELP config_sync_requests_total Total HTTP requests\n")
	fmt.Fprintf(w, "# TYPE config_sync_requests_total counter\n")
	fmt.Fprintf(w, "config_sync_requests_total %d\n", atomic.LoadUint64(&requestCounter))
}

func configsHandler(w http.ResponseWriter, r *http.Request) {
	configMu.RLock()
	defer configMu.RUnlock()

	configs := make([]map[string]interface{}, 0)
	for file, lastMod := range watchedConfigs {
		configs = append(configs, map[string]interface{}{
			"file":     filepath.Base(file),
			"path":     file,
			"last_mod": lastMod.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configs)
}

func syncHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	syncConfigs()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sync triggered"})
}

func uiHandler(w http.ResponseWriter, r *http.Request) {
	watcherStatus := "Inactive"
	watcherColor := ColorRed
	if watcher != nil {
		watcherStatus = "Active"
		watcherColor = ColorGreen
	}

	configMu.RLock()
	configCount := len(watchedConfigs)
	lastSync := lastSyncTime
	configMu.RUnlock()

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Config Sync</title>
    <style>
        body { background: %s; color: %s; font-family: 'JetBrains Mono', monospace; padding: 2rem; }
        .container { max-width: 800px; margin: 0 auto; }
        h1 { color: %s; }
        .status { padding: 1rem; background: %s; border-radius: 8px; margin: 1rem 0; }
        .status-indicator { display: inline-block; width: 12px; height: 12px; border-radius: 50%%; margin-right: 8px; }
        .metric { display: flex; justify-content: space-between; padding: 0.5rem 0; border-bottom: 1px solid %s; }
        .metric-value { color: %s; }
        .btn { background: %s; color: %s; border: none; padding: 0.75rem 1.5rem; border-radius: 4px; cursor: pointer; margin-top: 1rem; }
        .btn:hover { opacity: 0.8; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Config Sync</h1>
        <div class="status">
            <span class="status-indicator" style="background: %s"></span>
            File Watcher: %s
        </div>
        <div class="metrics">
            <div class="metric"><span>Watched Configs</span><span class="metric-value">%d</span></div>
            <div class="metric"><span>Total Synced</span><span class="metric-value">%d</span></div>
            <div class="metric"><span>Last Sync</span><span class="metric-value">%s</span></div>
        </div>
        <form action="/sync" method="POST">
            <button type="submit" class="btn">Trigger Sync</button>
        </form>
        <p style="margin-top: 2rem; color: %s;">View configs: <a href="/configs" style="color: %s">/configs</a></p>
    </div>
</body>
</html>`, ColorBase, ColorText, ColorTeal, ColorMantle, ColorCrust, ColorBlue,
		ColorMauve, ColorBase, watcherColor, watcherStatus, configCount,
		atomic.LoadUint64(&configsSynced), lastSync.Format("15:04:05"),
		ColorSubtext0, ColorMauve)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	logger.Log("info", "Starting config-sync service", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := initWatcher(); err != nil {
		logger.Log("warn", "Watcher initialization failed", map[string]interface{}{"error": err.Error()})
	}

	// Initial sync
	syncConfigs()

	// Start watching
	startWatcher(ctx)

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)
	http.HandleFunc("/configs", configsHandler)
	http.HandleFunc("/sync", syncHandler)
	http.HandleFunc("/", uiHandler)

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	logger.Log("info", "HTTP server starting", map[string]interface{}{"port": port})

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Log("error", "HTTP server failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
