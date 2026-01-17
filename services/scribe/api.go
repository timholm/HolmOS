package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// TailResponse represents the response for the tail endpoint
type TailResponse struct {
	Count      int        `json:"count"`
	Entries    []LogEntry `json:"entries"`
	ScribeSays string     `json:"scribe_says,omitempty"`
}

// CountResponse represents log counts by various dimensions
type CountResponse struct {
	Total      int            `json:"total"`
	ByLevel    map[string]int `json:"by_level"`
	ByPod      map[string]int `json:"by_pod"`
	ByNs       map[string]int `json:"by_namespace"`
	ScribeSays string         `json:"scribe_says,omitempty"`
}

// LevelsResponse represents available log levels
type LevelsResponse struct {
	Levels     []LevelInfo `json:"levels"`
	ScribeSays string      `json:"scribe_says,omitempty"`
}

// LevelInfo provides details about a log level
type LevelInfo struct {
	Level   string `json:"level"`
	Count   int    `json:"count"`
	Percent float64 `json:"percent"`
}

// ClearResponse represents the response after clearing logs
type ClearResponse struct {
	Status     string `json:"status"`
	Cleared    int    `json:"cleared"`
	ScribeSays string `json:"scribe_says,omitempty"`
}

// BetweenResponse represents logs between two timestamps
type BetweenResponse struct {
	Count      int        `json:"count"`
	Start      time.Time  `json:"start"`
	End        time.Time  `json:"end"`
	Entries    []LogEntry `json:"entries"`
	ScribeSays string     `json:"scribe_says,omitempty"`
}

// handleReady handles the /api/ready endpoint for Kubernetes readiness probes
func handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if the store is initialized and accepting logs
	if store == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not_ready",
			"reason": "store not initialized",
		})
		return
	}

	// Check if Kubernetes client is available
	if clientset == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not_ready",
			"reason": "kubernetes client not initialized",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status": "ready",
		"agent":  "Scribe",
	})
}

// handleLogsTail handles the /api/logs/tail endpoint
// Returns the most recent N log entries (default 50)
func handleLogsTail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse limit parameter (default 50)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	// Optional filters
	namespace := r.URL.Query().Get("namespace")
	level := r.URL.Query().Get("level")
	pod := r.URL.Query().Get("pod")

	// Get entries using existing Search method with empty query
	entries := store.Search("", namespace, level, pod, limit, false)

	scribeSays := ""
	if len(entries) == 0 {
		scribeSays = "The chronicles are empty. No recent events to report."
	} else if len(entries) < limit {
		scribeSays = "I have retrieved all available entries from the chronicles."
	} else {
		scribeSays = "Here are the most recent whispers from the realm."
	}

	json.NewEncoder(w).Encode(TailResponse{
		Count:      len(entries),
		Entries:    entries,
		ScribeSays: scribeSays,
	})
}

// handleLogsCount handles the /api/logs/count endpoint
// Returns log counts grouped by various dimensions
func handleLogsCount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := store.Stats()

	response := CountResponse{
		Total:   stats.TotalEntries,
		ByLevel: stats.Levels,
		ByPod:   stats.Pods,
		ByNs:    stats.Namespaces,
	}

	if stats.TotalEntries == 0 {
		response.ScribeSays = "The chronicles await their first entry."
	} else {
		errorCount := stats.Levels["ERROR"]
		warnCount := stats.Levels["WARN"]
		if errorCount > 0 {
			response.ScribeSays = "The annals reveal troubles in the realm. Errors demand attention."
		} else if warnCount > 0 {
			response.ScribeSays = "Warnings have been recorded. The realm is watchful."
		} else {
			response.ScribeSays = "All is well in the chronicles. The realm is at peace."
		}
	}

	json.NewEncoder(w).Encode(response)
}

// handleLogsLevels handles the /api/logs/levels endpoint
// Returns available log levels with counts and percentages
func handleLogsLevels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := store.Stats()
	total := stats.TotalEntries

	// Define standard levels in order of severity
	standardLevels := []string{"ERROR", "WARN", "INFO", "DEBUG"}
	levels := make([]LevelInfo, 0, len(standardLevels))

	for _, level := range standardLevels {
		count := stats.Levels[level]
		percent := 0.0
		if total > 0 {
			percent = float64(count) / float64(total) * 100
		}
		levels = append(levels, LevelInfo{
			Level:   level,
			Count:   count,
			Percent: percent,
		})
	}

	// Add any non-standard levels that may exist
	for level, count := range stats.Levels {
		found := false
		for _, std := range standardLevels {
			if level == std {
				found = true
				break
			}
		}
		if !found {
			percent := 0.0
			if total > 0 {
				percent = float64(count) / float64(total) * 100
			}
			levels = append(levels, LevelInfo{
				Level:   level,
				Count:   count,
				Percent: percent,
			})
		}
	}

	scribeSays := "These are the voices of the realm, categorized by their urgency."
	if stats.Levels["ERROR"] > 0 {
		scribeSays = "Errors echo through the chronicles. They speak of disturbances that require your wisdom."
	}

	json.NewEncoder(w).Encode(LevelsResponse{
		Levels:     levels,
		ScribeSays: scribeSays,
	})
}

// handleLogsClear handles the /api/logs/clear endpoint
// Clears all stored log entries (POST only)
func handleLogsClear(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed. Use POST or DELETE.", http.StatusMethodNotAllowed)
		return
	}

	// Get current count before clearing
	store.mu.Lock()
	clearedCount := len(store.entries)
	store.entries = make([]LogEntry, 0, defaultMaxLogEntries)
	store.mu.Unlock()

	// Reset volume statistics
	store.statsMu.Lock()
	store.totalBytes = 0
	store.bytesPerPod = make(map[string]int64)
	store.bytesPerNs = make(map[string]int64)
	store.entriesPerHour = make(map[string]int)
	store.statsMu.Unlock()

	scribeSays := "The chronicles have been wiped clean. A fresh chapter begins."
	if clearedCount == 0 {
		scribeSays = "The chronicles were already empty. Nothing to clear."
	}

	json.NewEncoder(w).Encode(ClearResponse{
		Status:     "cleared",
		Cleared:    clearedCount,
		ScribeSays: scribeSays,
	})
}

// handleLogsBetween handles the /api/logs/between endpoint
// Returns logs between two timestamps
func handleLogsBetween(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse start time (required)
	startStr := r.URL.Query().Get("start")
	if startStr == "" {
		http.Error(w, "Missing required 'start' parameter (RFC3339 format)", http.StatusBadRequest)
		return
	}

	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		// Try parsing as Unix timestamp
		if unix, err := strconv.ParseInt(startStr, 10, 64); err == nil {
			startTime = time.Unix(unix, 0)
		} else {
			http.Error(w, "Invalid 'start' parameter. Use RFC3339 format or Unix timestamp.", http.StatusBadRequest)
			return
		}
	}

	// Parse end time (defaults to now)
	endTime := time.Now()
	if endStr := r.URL.Query().Get("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			endTime = t
		} else if unix, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			endTime = time.Unix(unix, 0)
		} else {
			http.Error(w, "Invalid 'end' parameter. Use RFC3339 format or Unix timestamp.", http.StatusBadRequest)
			return
		}
	}

	// Ensure start is before end
	if startTime.After(endTime) {
		startTime, endTime = endTime, startTime
	}

	// Optional filters
	namespace := r.URL.Query().Get("namespace")
	level := r.URL.Query().Get("level")
	pod := r.URL.Query().Get("pod")

	// Parse limit (default 1000)
	limit := 1000
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	// Get all entries and filter by time range
	store.mu.RLock()
	var results []LogEntry
	for i := len(store.entries) - 1; i >= 0 && len(results) < limit; i-- {
		entry := store.entries[i]

		// Time filter
		if entry.Timestamp.Before(startTime) || entry.Timestamp.After(endTime) {
			continue
		}

		// Apply other filters
		if namespace != "" && entry.Namespace != namespace {
			continue
		}
		if level != "" && entry.Level != level {
			continue
		}
		if pod != "" && entry.Pod != pod {
			continue
		}

		results = append(results, entry)
	}
	store.mu.RUnlock()

	duration := endTime.Sub(startTime)
	scribeSays := ""
	if len(results) == 0 {
		scribeSays = "No entries found in the specified time range. The chronicles are silent for this period."
	} else if duration.Hours() > 24 {
		scribeSays = "I have searched the ancient records spanning multiple days."
	} else if duration.Hours() > 1 {
		scribeSays = "The chronicles reveal events from the past hours."
	} else {
		scribeSays = "Here are the recent whispers from the specified moment in time."
	}

	json.NewEncoder(w).Encode(BetweenResponse{
		Count:      len(results),
		Start:      startTime,
		End:        endTime,
		Entries:    results,
		ScribeSays: scribeSays,
	})
}

// RegisterAPIRoutes registers all additional API routes
// This function is called from main() to register the new endpoints
func RegisterAPIRoutes() {
	http.HandleFunc("/api/ready", handleReady)
	http.HandleFunc("/api/logs/tail", handleLogsTail)
	http.HandleFunc("/api/logs/count", handleLogsCount)
	http.HandleFunc("/api/logs/levels", handleLogsLevels)
	http.HandleFunc("/api/logs/clear", handleLogsClear)
	http.HandleFunc("/api/logs/between", handleLogsBetween)
}
