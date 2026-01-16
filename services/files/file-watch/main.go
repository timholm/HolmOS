package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const dataPath = "/data"

type WatchRequest struct {
	Path        string `json:"path"`
	CallbackURL string `json:"callback_url"`
}

type WatchInfo struct {
	Path        string    `json:"path"`
	CallbackURL string    `json:"callback_url"`
	CreatedAt   time.Time `json:"created_at"`
}

type WebhookPayload struct {
	Path      string `json:"path"`
	Event     string `json:"event"`
	FileName  string `json:"file_name"`
	Timestamp string `json:"timestamp"`
}

type WatchRegistry struct {
	mu      sync.RWMutex
	watches map[string]*WatchEntry
	watcher *fsnotify.Watcher
}

type WatchEntry struct {
	Info WatchInfo
}

var registry *WatchRegistry

func NewWatchRegistry() (*WatchRegistry, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	r := &WatchRegistry{
		watches: make(map[string]*WatchEntry),
		watcher: watcher,
	}

	go r.eventLoop()
	return r, nil
}

func (r *WatchRegistry) eventLoop() {
	for {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}
			r.handleEvent(event)
		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func (r *WatchRegistry) handleEvent(event fsnotify.Event) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Find matching watch entries
	for watchPath, entry := range r.watches {
		// Check if the event path is under the watched path
		if strings.HasPrefix(event.Name, watchPath) || event.Name == watchPath {
			go sendWebhook(entry.Info.CallbackURL, event, watchPath)
		}
	}
}

func eventTypeString(op fsnotify.Op) string {
	switch {
	case op&fsnotify.Create == fsnotify.Create:
		return "create"
	case op&fsnotify.Write == fsnotify.Write:
		return "write"
	case op&fsnotify.Remove == fsnotify.Remove:
		return "remove"
	case op&fsnotify.Rename == fsnotify.Rename:
		return "rename"
	case op&fsnotify.Chmod == fsnotify.Chmod:
		return "chmod"
	default:
		return "unknown"
	}
}

func sendWebhook(callbackURL string, event fsnotify.Event, watchPath string) {
	relPath, _ := filepath.Rel(dataPath, event.Name)

	payload := WebhookPayload{
		Path:      relPath,
		Event:     eventTypeString(event.Op),
		FileName:  filepath.Base(event.Name),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal webhook payload: %v", err)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(callbackURL, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("Failed to send webhook to %s: %v", callbackURL, err)
		return
	}
	defer resp.Body.Close()

	log.Printf("Webhook sent to %s for event %s on %s, status: %d",
		callbackURL, payload.Event, payload.Path, resp.StatusCode)
}

func (r *WatchRegistry) AddWatch(path, callbackURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Sanitize and build full path
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return os.ErrInvalid
	}
	fullPath := filepath.Join(dataPath, cleanPath)

	if _, exists := r.watches[fullPath]; exists {
		// Update callback URL if already watching
		r.watches[fullPath].Info.CallbackURL = callbackURL
		return nil
	}

	// Verify path exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return err
	}

	if err := r.watcher.Add(fullPath); err != nil {
		return err
	}

	r.watches[fullPath] = &WatchEntry{
		Info: WatchInfo{
			Path:        path,
			CallbackURL: callbackURL,
			CreatedAt:   time.Now(),
		},
	}

	log.Printf("Started watching: %s -> %s", path, callbackURL)
	return nil
}

func (r *WatchRegistry) RemoveWatch(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cleanPath := filepath.Clean(path)
	fullPath := filepath.Join(dataPath, cleanPath)

	if _, exists := r.watches[fullPath]; exists {
		r.watcher.Remove(fullPath)
		delete(r.watches, fullPath)
		log.Printf("Stopped watching: %s", path)
	}
	return nil
}

func (r *WatchRegistry) ListWatches() []WatchInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	watches := make([]WatchInfo, 0, len(r.watches))
	for _, entry := range r.watches {
		watches = append(watches, entry.Info)
	}
	return watches
}

func handleWatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleAddWatch(w, r)
	case http.MethodDelete:
		handleDeleteWatch(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAddWatch(w http.ResponseWriter, r *http.Request) {
	var req WatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" || req.CallbackURL == "" {
		http.Error(w, "path and callback_url are required", http.StatusBadRequest)
		return
	}

	if err := registry.AddWatch(req.Path, req.CallbackURL); err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Path not found: "+req.Path, http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to add watch: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "watching",
		"path":    req.Path,
		"message": "Watch started successfully",
	})
}

func handleDeleteWatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	if err := registry.RemoveWatch(req.Path); err != nil {
		http.Error(w, "Failed to remove watch: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "removed",
		"path":    req.Path,
		"message": "Watch removed successfully",
	})
}

func handleWatches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	watches := registry.ListWatches()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"watches": watches,
		"count":   len(watches),
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"service":   "file-watch",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func main() {
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Printf("Warning: Could not create data directory: %v", err)
	}

	var err error
	registry, err = NewWatchRegistry()
	if err != nil {
		log.Fatalf("Failed to create watch registry: %v", err)
	}

	http.HandleFunc("/watch", handleWatch)
	http.HandleFunc("/watches", handleWatches)
	http.HandleFunc("/health", handleHealth)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("File watch service starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
