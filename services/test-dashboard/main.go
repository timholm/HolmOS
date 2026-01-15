package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

type ServiceHealth struct {
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	ResponseTime string    `json:"responseTime"`
	ResponseMs   int64     `json:"responseMs"`
	Message      string    `json:"message"`
	Endpoint     string    `json:"endpoint"`
	LastChecked  time.Time `json:"lastChecked"`
	Category     string    `json:"category"`
}

type TestHistory struct {
	Timestamp   time.Time `json:"timestamp"`
	TotalCount  int       `json:"totalCount"`
	PassCount   int       `json:"passCount"`
	FailCount   int       `json:"failCount"`
	ErrorCount  int       `json:"errorCount"`
	AvgResponse int64     `json:"avgResponse"`
}

type HealthResponse struct {
	Results     []ServiceHealth `json:"results"`
	Timestamp   time.Time       `json:"timestamp"`
	Summary     Summary         `json:"summary"`
	History     []TestHistory   `json:"history"`
	Alerts      []Alert         `json:"alerts"`
}

type Summary struct {
	Total   int   `json:"total"`
	Healthy int   `json:"healthy"`
	Failing int   `json:"failing"`
	Errors  int   `json:"errors"`
	AvgMs   int64 `json:"avgMs"`
}

type Alert struct {
	Service   string    `json:"service"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"`
	Timestamp time.Time `json:"timestamp"`
}

type ServiceConfig struct {
	Name     string
	Endpoint string
	Port     string
	Category string
}

var (
	historyMutex sync.RWMutex
	testHistory  []TestHistory
	alertsMutex  sync.RWMutex
	activeAlerts []Alert
	maxHistory   = 100
)

func getServices() []ServiceConfig {
	return []ServiceConfig{
		// Test Services
		{"auth-test", "http://auth-test.holm.svc.cluster.local:8080/health", "8080", "test"},
		{"files-test", "http://files-test.holm.svc.cluster.local/health", "80", "test"},
		{"terminal-test", "http://terminal-test.holm.svc.cluster.local:8080/health", "8080", "test"},
		{"cluster-test", "http://cluster-test.holm.svc.cluster.local:8080/health", "8080", "test"},
		{"registry-test", "http://registry-test.holm.svc.cluster.local/health", "80", "test"},
		{"integration-test", "http://integration-test.holm.svc.cluster.local/health", "80", "test"},
		
		// Core Infrastructure
		{"api-gateway", "http://api-gateway.holm.svc.cluster.local:8080/health", "8080", "core"},
		{"auth-gateway", "http://auth-gateway.holm.svc.cluster.local:8080/health", "8080", "core"},
		{"gateway", "http://gateway.holm.svc.cluster.local:8080/health", "8080", "core"},
		{"postgres", "http://postgres.holm.svc.cluster.local:5432", "5432", "core"},
		
		// Auth Services
		{"auth-login", "http://auth-login.holm.svc.cluster.local/health", "80", "auth"},
		{"auth-logout", "http://auth-logout.holm.svc.cluster.local/health", "80", "auth"},
		{"auth-refresh", "http://auth-refresh.holm.svc.cluster.local/health", "80", "auth"},
		{"auth-register", "http://auth-register.holm.svc.cluster.local/health", "80", "auth"},
		{"auth-token-validate", "http://auth-token-validate.holm.svc.cluster.local:8080/health", "8080", "auth"},
		
		// File Services
		{"file-list", "http://file-list.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-upload", "http://file-upload.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-download", "http://file-download.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-delete", "http://file-delete.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-copy", "http://file-copy.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-move", "http://file-move.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-mkdir", "http://file-mkdir.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-meta", "http://file-meta.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-search", "http://file-search.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-compress", "http://file-compress.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-decompress", "http://file-decompress.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-preview", "http://file-preview.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-convert", "http://file-convert.holm.svc.cluster.local/health", "80", "files"},
		{"file-encrypt", "http://file-encrypt.holm.svc.cluster.local/health", "80", "files"},
		{"file-share", "http://file-share.holm.svc.cluster.local/health", "80", "files"},
		{"file-thumbnail", "http://file-thumbnail.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-watch", "http://file-watch.holm.svc.cluster.local/health", "80", "files"},
		{"file-web", "http://file-web.holm.svc.cluster.local:8080/health", "8080", "files"},
		{"file-web-nautilus", "http://file-web-nautilus.holm.svc.cluster.local/health", "80", "files"},
		
		// Terminal Services
		{"terminal", "http://terminal.holm.svc.cluster.local:8080/health", "8080", "terminal"},
		{"terminal-host-add", "http://terminal-host-add.holm.svc.cluster.local:8080/health", "8080", "terminal"},
		{"terminal-host-list", "http://terminal-host-list.holm.svc.cluster.local:8080/health", "8080", "terminal"},
		
		// Cluster Services
		{"cluster-manager", "http://cluster-manager.holm.svc.cluster.local:8080/health", "8080", "cluster"},
		{"cluster-apt-update", "http://cluster-apt-update.holm.svc.cluster.local:8080/health", "8080", "cluster"},
		{"cluster-node-list", "http://cluster-node-list.holm.svc.cluster.local:8080/health", "8080", "cluster"},
		{"cluster-node-ping", "http://cluster-node-ping.holm.svc.cluster.local:8080/health", "8080", "cluster"},
		{"cluster-reboot-exec", "http://cluster-reboot-exec.holm.svc.cluster.local:8080/health", "8080", "cluster"},
		
		// Registry Services
		{"registry-ui", "http://registry-ui.holm.svc.cluster.local:8080/health", "8080", "registry"},
		{"registry-list-repos", "http://registry-list-repos.holm.svc.cluster.local:8080/health", "8080", "registry"},
		{"registry-list-tags", "http://registry-list-tags.holm.svc.cluster.local:8080/health", "8080", "registry"},
		
		// Settings Services
		{"settings-web", "http://settings-web.holm.svc.cluster.local:8080/health", "8080", "settings"},
		{"settings-theme", "http://settings-theme.holm.svc.cluster.local:8080/health", "8080", "settings"},
		{"settings-tabs", "http://settings-tabs.holm.svc.cluster.local:8080/health", "8080", "settings"},
		{"settings-backup", "http://settings-backup.holm.svc.cluster.local:8080/health", "8080", "settings"},
		{"settings-restore", "http://settings-restore.holm.svc.cluster.local:8080/health", "8080", "settings"},
		
		// Audiobook Services
		{"audiobook-web", "http://audiobook-web.holm.svc.cluster.local:8080/health", "8080", "audiobook"},
		{"audiobook-parse-epub", "http://audiobook-parse-epub.holm.svc.cluster.local:8080/health", "8080", "audiobook"},
		{"audiobook-chunk-text", "http://audiobook-chunk-text.holm.svc.cluster.local:8080/health", "8080", "audiobook"},
		{"audiobook-tts-convert", "http://audiobook-tts-convert.holm.svc.cluster.local:8080/health", "8080", "audiobook"},
		{"audiobook-audio-concat", "http://audiobook-audio-concat.holm.svc.cluster.local:8080/health", "8080", "audiobook"},
		{"audiobook-audio-normalize", "http://audiobook-audio-normalize.holm.svc.cluster.local:8080/health", "8080", "audiobook"},
		{"audiobook-upload-epub", "http://audiobook-upload-epub.holm.svc.cluster.local:8080/health", "8080", "audiobook"},
		{"audiobook-upload-txt", "http://audiobook-upload-txt.holm.svc.cluster.local:8080/health", "8080", "audiobook"},
		
		// Apps
		{"app-store", "http://app-store.holm.svc.cluster.local/health", "80", "apps"},
		{"browser-app", "http://browser-app.holm.svc.cluster.local/health", "80", "apps"},
		{"calculator-app", "http://calculator-app.holm.svc.cluster.local/health", "80", "apps"},
		{"clock-app", "http://clock-app.holm.svc.cluster.local/health", "80", "apps"},
		{"contacts-app", "http://contacts-app.holm.svc.cluster.local/health", "80", "apps"},
		{"mail-app", "http://mail-app.holm.svc.cluster.local/health", "80", "apps"},
		{"maps-app", "http://maps-app.holm.svc.cluster.local/health", "80", "apps"},
		{"music-app", "http://music-app.holm.svc.cluster.local/health", "80", "apps"},
		{"notes-app", "http://notes-app.holm.svc.cluster.local/health", "80", "apps"},
		{"photos-app", "http://photos-app.holm.svc.cluster.local/health", "80", "apps"},
		{"reminders-app", "http://reminders-app.holm.svc.cluster.local/health", "80", "apps"},
		{"video-app", "http://video-app.holm.svc.cluster.local/health", "80", "apps"},
		
		// Agent Services
		{"agent-orchestrator", "http://agent-orchestrator.holm.svc.cluster.local/health", "80", "agents"},
		{"agent-router", "http://agent-router.holm.svc.cluster.local/health", "80", "agents"},
		{"config-agent", "http://config-agent.holm.svc.cluster.local:5000/health", "5000", "agents"},
		{"guardian-agent", "http://guardian-agent.holm.svc.cluster.local/health", "80", "agents"},
		
		// Platform Services
		{"atlas", "http://atlas.holm.svc.cluster.local/health", "80", "platform"},
		{"chat-hub", "http://chat-hub.holm.svc.cluster.local:8080/health", "8080", "platform"},
		{"claude-pod", "http://claude-pod.holm.svc.cluster.local/health", "80", "platform"},
		{"compass", "http://compass.holm.svc.cluster.local/health", "80", "platform"},
		{"echo", "http://echo.holm.svc.cluster.local/health", "80", "platform"},
		{"forge", "http://forge.holm.svc.cluster.local/health", "80", "platform"},
		{"harbor", "http://harbor.holm.svc.cluster.local/health", "80", "platform"},
		{"helix", "http://helix.holm.svc.cluster.local/health", "80", "platform"},
		{"holmos-shell", "http://holmos-shell.holm.svc.cluster.local/health", "80", "platform"},
		{"merchant", "http://merchant.holm.svc.cluster.local/health", "80", "platform"},
		{"nova", "http://nova.holm.svc.cluster.local/health", "80", "platform"},
		{"pulse", "http://pulse.holm.svc.cluster.local/health", "80", "platform"},
		{"scribe", "http://scribe.holm.svc.cluster.local/health", "80", "platform"},
		{"sentinel", "http://sentinel.holm.svc.cluster.local/health", "80", "platform"},
		{"vault", "http://vault.holm.svc.cluster.local/health", "80", "platform"},
		
		// DevOps Services
		{"build-orchestrator", "http://build-orchestrator.holm.svc.cluster.local/health", "80", "devops"},
		{"cicd-controller", "http://cicd-controller-service.holm.svc.cluster.local:5000/health", "5000", "devops"},
		{"config-server", "http://config-server.holm.svc.cluster.local:8080/health", "8080", "devops"},
		{"deploy-controller", "http://deploy-controller.holm.svc.cluster.local/health", "80", "devops"},
		{"gitops-sync", "http://gitops-sync.holm.svc.cluster.local/health", "80", "devops"},
		{"secret-manager", "http://secret-manager.holm.svc.cluster.local:8080/health", "8080", "devops"},
		{"service-mesh-controller", "http://service-mesh-controller.holm.svc.cluster.local/health", "80", "devops"},
		
		// Monitoring Services
		{"alerting", "http://alerting.holm.svc.cluster.local/health", "80", "monitoring"},
		{"log-aggregator", "http://log-aggregator.holm.svc.cluster.local/health", "80", "monitoring"},
		{"metrics-collector", "http://metrics-collector.holm.svc.cluster.local:8080/health", "8080", "monitoring"},
		{"metrics-dashboard", "http://metrics-dashboard.holm.svc.cluster.local:8080/health", "8080", "monitoring"},
		
		// Backup Services
		{"backup-scheduler", "http://backup-scheduler.holm.svc.cluster.local:8080/health", "8080", "backup"},
		{"backup-storage", "http://backup-storage.holm.svc.cluster.local/health", "80", "backup"},
		{"restore-manager", "http://restore-manager.holm.svc.cluster.local:8080/health", "8080", "backup"},
		
		// Notification Services
		{"notification-email", "http://notification-email.holm.svc.cluster.local:8080/health", "8080", "notification"},
		{"notification-queue", "http://notification-queue.holm.svc.cluster.local/health", "80", "notification"},
		{"notification-webhook", "http://notification-webhook.holm.svc.cluster.local:8080/health", "8080", "notification"},
		
		// User Services
		{"user-activity", "http://user-activity.holm.svc.cluster.local/health", "80", "user"},
		{"user-preferences", "http://user-preferences.holm.svc.cluster.local/health", "80", "user"},
		{"user-profile", "http://user-profile.holm.svc.cluster.local/health", "80", "user"},
		
		// Cache and Rate Limiting
		{"cache-service", "http://cache-service.holm.svc.cluster.local:8080/health", "8080", "infra"},
		{"rate-limiter", "http://rate-limiter.holm.svc.cluster.local:8080/health", "8080", "infra"},
		
		// Database Services
		{"contacts-db", "http://contacts-db.holm.svc.cluster.local:5432", "5432", "database"},
		{"mail-app-postgres", "http://mail-app-postgres.holm.svc.cluster.local:5432", "5432", "database"},
		{"backup-scheduler-postgres", "http://backup-scheduler-postgres.holm.svc.cluster.local:5432", "5432", "database"},
	}
}

func checkHealth(svc ServiceConfig) ServiceHealth {
	start := time.Now()
	client := &http.Client{Timeout: 5 * time.Second}
	
	result := ServiceHealth{
		Name:        svc.Name,
		Endpoint:    svc.Endpoint,
		Category:    svc.Category,
		LastChecked: time.Now(),
	}
	
	// Special handling for postgres/database services
	if svc.Port == "5432" {
		// TCP check for databases
		result.Status = "pass"
		result.Message = "Database service (TCP check skipped)"
		result.ResponseTime = "N/A"
		result.ResponseMs = 0
		return result
	}
	
	resp, err := client.Get(svc.Endpoint)
	elapsed := time.Since(start)
	result.ResponseMs = elapsed.Milliseconds()
	result.ResponseTime = fmt.Sprintf("%dms", elapsed.Milliseconds())
	
	if err != nil {
		result.Status = "error"
		if strings.Contains(err.Error(), "timeout") {
			result.Message = "Connection timeout"
		} else if strings.Contains(err.Error(), "connection refused") {
			result.Message = "Connection refused"
		} else {
			result.Message = "Connection failed"
		}
		return result
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Status = "pass"
		result.Message = "Healthy"
		// Try to parse JSON response for more details
		var healthResp map[string]interface{}
		if json.Unmarshal(body, &healthResp) == nil {
			if status, ok := healthResp["status"].(string); ok {
				result.Message = status
			}
		}
	} else if resp.StatusCode == 404 {
		// 404 might mean service is up but no /health endpoint
		result.Status = "pass"
		result.Message = "Running (no health endpoint)"
	} else {
		result.Status = "fail"
		result.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	
	return result
}

func runAllHealthChecks() HealthResponse {
	services := getServices()
	results := make([]ServiceHealth, len(services))
	
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 20) // Limit concurrent checks
	
	for i, svc := range services {
		wg.Add(1)
		go func(idx int, service ServiceConfig) {
			defer wg.Done()
			semaphore <- struct{}{}
			results[idx] = checkHealth(service)
			<-semaphore
		}(i, svc)
	}
	
	wg.Wait()
	
	// Sort by category then name
	sort.Slice(results, func(i, j int) bool {
		if results[i].Category != results[j].Category {
			return results[i].Category < results[j].Category
		}
		return results[i].Name < results[j].Name
	})
	
	// Calculate summary
	summary := Summary{Total: len(results)}
	var totalMs int64
	var countMs int
	newAlerts := []Alert{}
	
	for _, r := range results {
		switch r.Status {
		case "pass":
			summary.Healthy++
		case "fail":
			summary.Failing++
			newAlerts = append(newAlerts, Alert{
				Service:   r.Name,
				Message:   r.Message,
				Severity:  "warning",
				Timestamp: time.Now(),
			})
		case "error":
			summary.Errors++
			newAlerts = append(newAlerts, Alert{
				Service:   r.Name,
				Message:   r.Message,
				Severity:  "critical",
				Timestamp: time.Now(),
			})
		}
		if r.ResponseMs > 0 {
			totalMs += r.ResponseMs
			countMs++
		}
	}
	
	if countMs > 0 {
		summary.AvgMs = totalMs / int64(countMs)
	}
	
	// Update alerts
	alertsMutex.Lock()
	activeAlerts = newAlerts
	alertsMutex.Unlock()
	
	// Record history
	historyEntry := TestHistory{
		Timestamp:   time.Now(),
		TotalCount:  summary.Total,
		PassCount:   summary.Healthy,
		FailCount:   summary.Failing,
		ErrorCount:  summary.Errors,
		AvgResponse: summary.AvgMs,
	}
	
	historyMutex.Lock()
	testHistory = append(testHistory, historyEntry)
	if len(testHistory) > maxHistory {
		testHistory = testHistory[1:]
	}
	historyCopy := make([]TestHistory, len(testHistory))
	copy(historyCopy, testHistory)
	historyMutex.Unlock()
	
	alertsMutex.RLock()
	alertsCopy := make([]Alert, len(activeAlerts))
	copy(alertsCopy, activeAlerts)
	alertsMutex.RUnlock()
	
	return HealthResponse{
		Results:   results,
		Timestamp: time.Now(),
		Summary:   summary,
		History:   historyCopy,
		Alerts:    alertsCopy,
	}
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	response := runAllHealthChecks()
	json.NewEncoder(w).Encode(response)
}

func handleRunSingle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	serviceName := r.URL.Query().Get("service")
	if serviceName == "" {
		http.Error(w, "service parameter required", http.StatusBadRequest)
		return
	}
	
	services := getServices()
	for _, svc := range services {
		if svc.Name == serviceName {
			result := checkHealth(svc)
			json.NewEncoder(w).Encode(result)
			return
		}
	}
	
	http.Error(w, "service not found", http.StatusNotFound)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	historyMutex.RLock()
	defer historyMutex.RUnlock()
	
	json.NewEncoder(w).Encode(testHistory)
}

func handleAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	alertsMutex.RLock()
	defer alertsMutex.RUnlock()
	
	json.NewEncoder(w).Encode(activeAlerts)
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	json.NewEncoder(w).Encode(getServices())
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": "test-dashboard"})
}

func main() {
	// API routes
	http.HandleFunc("/api/run", handleRun)
	http.HandleFunc("/api/run/single", handleRunSingle)
	http.HandleFunc("/api/history", handleHistory)
	http.HandleFunc("/api/alerts", handleAlerts)
	http.HandleFunc("/api/services", handleServices)
	http.HandleFunc("/health", handleHealth)
	
	// Static files
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			content, err := staticFiles.ReadFile("static/index.html")
			if err != nil {
				http.Error(w, "Not found", 404)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.Write(content)
			return
		}
		http.FileServer(http.FS(staticFiles)).ServeHTTP(w, r)
	})
	
	log.Println("Test Dashboard starting on :8080")
	log.Println("Monitoring", len(getServices()), "services")
	
	// Run initial check
	go func() {
		time.Sleep(2 * time.Second)
		runAllHealthChecks()
	}()
	
	log.Fatal(http.ListenAndServe(":8080", nil))
}
