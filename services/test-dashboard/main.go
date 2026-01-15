package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

// Catppuccin Mocha colors for reference
// --base: #1e1e2e, --mantle: #181825, --crust: #11111b
// --text: #cdd6f4, --green: #a6e3a1, --red: #f38ba8
// --yellow: #f9e2af, --blue: #89b4fa, --mauve: #cba6f7

// === Data Structures ===

type ServiceConfig struct {
	Name        string `json:"name" yaml:"name"`
	Port        int    `json:"port" yaml:"port"`
	Category    string `json:"category" yaml:"category"`
	Description string `json:"description" yaml:"description"`
	Health      string `json:"health" yaml:"health"`
}

type TestResult struct {
	ID           string    `json:"id"`
	ServiceName  string    `json:"serviceName"`
	TestType     string    `json:"testType"` // health, unit, integration, e2e
	Status       string    `json:"status"`   // pass, fail, error, running, pending
	Message      string    `json:"message"`
	Duration     int64     `json:"duration"` // milliseconds
	ResponseTime string    `json:"responseTime"`
	Endpoint     string    `json:"endpoint"`
	Category     string    `json:"category"`
	Timestamp    time.Time `json:"timestamp"`
	Details      string    `json:"details,omitempty"`
}

type TestRun struct {
	ID           string       `json:"id"`
	StartTime    time.Time    `json:"startTime"`
	EndTime      time.Time    `json:"endTime,omitempty"`
	Status       string       `json:"status"` // running, completed, failed
	Results      []TestResult `json:"results"`
	Summary      TestSummary  `json:"summary"`
	TriggerType  string       `json:"triggerType"` // manual, scheduled, webhook, github
	TriggerBy    string       `json:"triggerBy"`
}

type TestSummary struct {
	Total      int   `json:"total"`
	Passed     int   `json:"passed"`
	Failed     int   `json:"failed"`
	Errors     int   `json:"errors"`
	Running    int   `json:"running"`
	Pending    int   `json:"pending"`
	Skipped    int   `json:"skipped"`
	AvgLatency int64 `json:"avgLatency"`
	Duration   int64 `json:"duration"`
	PassRate   float64 `json:"passRate"`
}

type TestHistory struct {
	Timestamp   time.Time `json:"timestamp"`
	RunID       string    `json:"runId"`
	TotalCount  int       `json:"totalCount"`
	PassCount   int       `json:"passCount"`
	FailCount   int       `json:"failCount"`
	ErrorCount  int       `json:"errorCount"`
	AvgResponse int64     `json:"avgResponse"`
	Duration    int64     `json:"duration"`
	PassRate    float64   `json:"passRate"`
}

type Alert struct {
	ID        string    `json:"id"`
	Service   string    `json:"service"`
	Message   string    `json:"message"`
	Severity  string    `json:"severity"` // critical, warning, info
	Timestamp time.Time `json:"timestamp"`
	Resolved  bool      `json:"resolved"`
}

type GitHubWorkflowRun struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	HeadBranch string    `json:"head_branch"`
	HeadSHA    string    `json:"head_sha"`
}

type GitHubWorkflowsResponse struct {
	TotalCount   int                 `json:"total_count"`
	WorkflowRuns []GitHubWorkflowRun `json:"workflow_runs"`
}

// === Global State ===

var (
	testRunsMutex   sync.RWMutex
	testRuns        []TestRun
	testHistoryMutex sync.RWMutex
	testHistory     []TestHistory
	alertsMutex     sync.RWMutex
	activeAlerts    []Alert
	currentRunMutex sync.RWMutex
	currentRun      *TestRun

	maxHistory      = 200
	maxRuns         = 50
	httpClient      = &http.Client{Timeout: 10 * time.Second}

	// Config
	githubRepo   = os.Getenv("GITHUB_REPO")
	githubToken  = os.Getenv("GITHUB_TOKEN")
	clusterHost  = getEnvOrDefault("CLUSTER_HOST", "holm.svc.cluster.local")
)

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// === Service Registry (from services.yaml) ===

func getServices() []ServiceConfig {
	return []ServiceConfig{
		// Core Entry Points
		{"holmos-shell", 30000, "core", "iPhone-style home screen", "/health"},
		{"claude-pod", 30001, "core", "AI chat interface", "/health"},
		{"app-store", 30002, "core", "AI-powered app generator", "/health"},
		{"chat-hub", 30003, "core", "Unified agent messaging", "/health"},

		// AI Agents
		{"nova", 30004, "agent", "I see all 13 stars in our constellation", "/health"},
		{"merchant", 30005, "agent", "Describe what you need, I'll make it happen", "/health"},
		{"pulse", 30006, "agent", "Vital signs are looking good", "/health"},
		{"gateway", 30008, "agent", "All roads lead through me", "/health"},
		{"scribe", 30860, "agent", "It's all in the records", "/health"},
		{"vault", 30870, "agent", "Your secrets are safe with me", "/health"},

		// Apps
		{"clock-app", 30007, "app", "World clock, alarms, timer", "/health"},
		{"calculator-app", 30010, "app", "iPhone-style calculator", "/health"},
		{"file-web-nautilus", 30088, "app", "GNOME-style file manager", "/health"},
		{"settings-web", 30600, "app", "Settings hub", "/health"},
		{"audiobook-web", 30700, "app", "Audiobook TTS pipeline", "/health"},
		{"terminal-web", 30800, "app", "Web-based terminal", "/health"},

		// Infrastructure & DevOps
		{"holm-git", 30009, "devops", "Git repository server", "/health"},
		{"cicd-controller", 30020, "devops", "CI/CD pipeline manager", "/health"},
		{"deploy-controller", 30021, "devops", "Auto-deployment controller", "/health"},

		// Admin & Monitoring
		{"cluster-manager", 30502, "admin", "Cluster admin dashboard", "/health"},
		{"backup-dashboard", 30850, "admin", "Backup management", "/health"},
		{"test-dashboard", 30900, "monitoring", "Service health monitoring", "/health"},
		{"metrics-dashboard", 30950, "monitoring", "Cluster metrics", "/health"},
		{"registry-ui", 31750, "devops", "Container registry browser", "/health"},
	}
}

// === Health Check Functions ===

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func checkServiceHealth(svc ServiceConfig) TestResult {
	start := time.Now()
	id := generateID()

	endpoint := fmt.Sprintf("http://%s.%s:%d%s", svc.Name, clusterHost, svc.Port, svc.Health)

	result := TestResult{
		ID:          id,
		ServiceName: svc.Name,
		TestType:    "health",
		Endpoint:    endpoint,
		Category:    svc.Category,
		Timestamp:   time.Now(),
	}

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		result.Status = "error"
		result.Message = "Failed to create request"
		result.Duration = time.Since(start).Milliseconds()
		result.ResponseTime = fmt.Sprintf("%dms", result.Duration)
		return result
	}

	resp, err := httpClient.Do(req)
	elapsed := time.Since(start)
	result.Duration = elapsed.Milliseconds()
	result.ResponseTime = fmt.Sprintf("%dms", result.Duration)

	if err != nil {
		result.Status = "error"
		if strings.Contains(err.Error(), "timeout") {
			result.Message = "Connection timeout"
		} else if strings.Contains(err.Error(), "connection refused") {
			result.Message = "Connection refused"
		} else if strings.Contains(err.Error(), "no such host") {
			result.Message = "Service not found"
		} else {
			result.Message = "Connection failed"
		}
		result.Details = err.Error()
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
		result.Status = "pass"
		result.Message = "Running (no health endpoint)"
	} else {
		result.Status = "fail"
		result.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		result.Details = string(body)
	}

	return result
}

// === Test Run Execution ===

func startTestRun(triggerType, triggerBy string) *TestRun {
	run := &TestRun{
		ID:          generateID(),
		StartTime:   time.Now(),
		Status:      "running",
		TriggerType: triggerType,
		TriggerBy:   triggerBy,
		Results:     []TestResult{},
	}

	currentRunMutex.Lock()
	currentRun = run
	currentRunMutex.Unlock()

	return run
}

func completeTestRun(run *TestRun) {
	run.EndTime = time.Now()
	run.Status = "completed"

	// Calculate summary
	var totalLatency int64
	var latencyCount int

	for _, r := range run.Results {
		switch r.Status {
		case "pass":
			run.Summary.Passed++
		case "fail":
			run.Summary.Failed++
		case "error":
			run.Summary.Errors++
		case "running":
			run.Summary.Running++
		case "pending":
			run.Summary.Pending++
		case "skipped":
			run.Summary.Skipped++
		}
		if r.Duration > 0 {
			totalLatency += r.Duration
			latencyCount++
		}
	}

	run.Summary.Total = len(run.Results)
	run.Summary.Duration = run.EndTime.Sub(run.StartTime).Milliseconds()

	if latencyCount > 0 {
		run.Summary.AvgLatency = totalLatency / int64(latencyCount)
	}

	if run.Summary.Total > 0 {
		run.Summary.PassRate = float64(run.Summary.Passed) / float64(run.Summary.Total) * 100
	}

	// Add to history
	historyEntry := TestHistory{
		Timestamp:   run.EndTime,
		RunID:       run.ID,
		TotalCount:  run.Summary.Total,
		PassCount:   run.Summary.Passed,
		FailCount:   run.Summary.Failed,
		ErrorCount:  run.Summary.Errors,
		AvgResponse: run.Summary.AvgLatency,
		Duration:    run.Summary.Duration,
		PassRate:    run.Summary.PassRate,
	}

	testHistoryMutex.Lock()
	testHistory = append(testHistory, historyEntry)
	if len(testHistory) > maxHistory {
		testHistory = testHistory[1:]
	}
	testHistoryMutex.Unlock()

	// Store run
	testRunsMutex.Lock()
	testRuns = append([]TestRun{*run}, testRuns...)
	if len(testRuns) > maxRuns {
		testRuns = testRuns[:maxRuns]
	}
	testRunsMutex.Unlock()

	// Generate alerts
	generateAlerts(run)

	// Clear current run
	currentRunMutex.Lock()
	currentRun = nil
	currentRunMutex.Unlock()
}

func runAllHealthChecks(triggerType, triggerBy string) *TestRun {
	run := startTestRun(triggerType, triggerBy)
	services := getServices()
	results := make([]TestResult, len(services))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 20) // Limit concurrent checks

	for i, svc := range services {
		wg.Add(1)
		go func(idx int, service ServiceConfig) {
			defer wg.Done()
			semaphore <- struct{}{}
			results[idx] = checkServiceHealth(service)
			<-semaphore
		}(i, svc)
	}

	wg.Wait()

	// Sort by category then name
	sort.Slice(results, func(i, j int) bool {
		if results[i].Category != results[j].Category {
			return results[i].Category < results[j].Category
		}
		return results[i].ServiceName < results[j].ServiceName
	})

	run.Results = results
	completeTestRun(run)

	return run
}

func generateAlerts(run *TestRun) {
	alertsMutex.Lock()
	defer alertsMutex.Unlock()

	// Clear old alerts
	activeAlerts = []Alert{}

	for _, r := range run.Results {
		var alert *Alert

		switch r.Status {
		case "error":
			alert = &Alert{
				ID:        generateID(),
				Service:   r.ServiceName,
				Message:   r.Message,
				Severity:  "critical",
				Timestamp: time.Now(),
			}
		case "fail":
			alert = &Alert{
				ID:        generateID(),
				Service:   r.ServiceName,
				Message:   r.Message,
				Severity:  "warning",
				Timestamp: time.Now(),
			}
		}

		if alert != nil {
			activeAlerts = append(activeAlerts, *alert)
		}
	}
}

// === GitHub Integration ===

func fetchGitHubWorkflows() ([]GitHubWorkflowRun, error) {
	if githubRepo == "" || githubToken == "" {
		// Return mock data if not configured
		return []GitHubWorkflowRun{
			{
				ID:         1,
				Name:       "HolmOS CI",
				Status:     "completed",
				Conclusion: "success",
				HTMLURL:    "https://github.com/example/holmos/actions/runs/1",
				CreatedAt:  time.Now().Add(-2 * time.Hour),
				UpdatedAt:  time.Now().Add(-1 * time.Hour),
				HeadBranch: "main",
				HeadSHA:    "abc123",
			},
		}, nil
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/actions/runs?per_page=10", githubRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var result GitHubWorkflowsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.WorkflowRuns, nil
}

func triggerGitHubWorkflow(workflowFile string) error {
	if githubRepo == "" || githubToken == "" {
		return fmt.Errorf("GitHub integration not configured")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/actions/workflows/%s/dispatches", githubRepo, workflowFile)

	payload := map[string]string{"ref": "main"}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	return nil
}

// === HTTP Handlers ===

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "test-dashboard",
		"version": "2.0.0",
	})
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Check if a run is already in progress
	currentRunMutex.RLock()
	if currentRun != nil {
		currentRunMutex.RUnlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":      "Test run already in progress",
			"currentRun": currentRun,
		})
		return
	}
	currentRunMutex.RUnlock()

	triggerType := r.URL.Query().Get("trigger")
	if triggerType == "" {
		triggerType = "manual"
	}
	triggerBy := r.URL.Query().Get("by")
	if triggerBy == "" {
		triggerBy = "dashboard"
	}

	run := runAllHealthChecks(triggerType, triggerBy)
	json.NewEncoder(w).Encode(run)
}

func handleRunSingle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	serviceName := r.URL.Query().Get("service")
	if serviceName == "" {
		http.Error(w, `{"error": "service parameter required"}`, http.StatusBadRequest)
		return
	}

	services := getServices()
	for _, svc := range services {
		if svc.Name == serviceName {
			result := checkServiceHealth(svc)
			json.NewEncoder(w).Encode(result)
			return
		}
	}

	http.Error(w, `{"error": "service not found"}`, http.StatusNotFound)
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	json.NewEncoder(w).Encode(getServices())
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	testHistoryMutex.RLock()
	defer testHistoryMutex.RUnlock()

	json.NewEncoder(w).Encode(testHistory)
}

func handleRuns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	testRunsMutex.RLock()
	defer testRunsMutex.RUnlock()

	json.NewEncoder(w).Encode(testRuns)
}

func handleRunByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, `{"error": "id parameter required"}`, http.StatusBadRequest)
		return
	}

	testRunsMutex.RLock()
	defer testRunsMutex.RUnlock()

	for _, run := range testRuns {
		if run.ID == id {
			json.NewEncoder(w).Encode(run)
			return
		}
	}

	http.Error(w, `{"error": "run not found"}`, http.StatusNotFound)
}

func handleCurrentRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	currentRunMutex.RLock()
	defer currentRunMutex.RUnlock()

	if currentRun == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"running": false})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"running": true,
		"run":     currentRun,
	})
}

func handleAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	alertsMutex.RLock()
	defer alertsMutex.RUnlock()

	json.NewEncoder(w).Encode(activeAlerts)
}

func handleGitHubWorkflows(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	workflows, err := fetchGitHubWorkflows()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":     err.Error(),
			"workflows": []GitHubWorkflowRun{},
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"workflows": workflows,
		"repo":      githubRepo,
	})
}

func handleTriggerGitHub(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != "POST" {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	workflow := r.URL.Query().Get("workflow")
	if workflow == "" {
		workflow = "ci.yml"
	}

	err := triggerGitHubWorkflow(workflow)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"workflow": workflow,
		"message":  "Workflow triggered successfully",
	})
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	testHistoryMutex.RLock()
	history := make([]TestHistory, len(testHistory))
	copy(history, testHistory)
	testHistoryMutex.RUnlock()

	if len(history) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"totalRuns":       0,
			"avgPassRate":     0,
			"avgLatency":      0,
			"trend":           "stable",
			"lastRunTime":     nil,
			"failingServices": []string{},
		})
		return
	}

	// Calculate stats
	var totalPassRate float64
	var totalLatency int64
	for _, h := range history {
		totalPassRate += h.PassRate
		totalLatency += h.AvgResponse
	}

	avgPassRate := totalPassRate / float64(len(history))
	avgLatency := totalLatency / int64(len(history))

	// Determine trend
	trend := "stable"
	if len(history) >= 5 {
		recent := history[len(history)-5:]
		older := history[:len(history)-5]

		var recentAvg, olderAvg float64
		for _, h := range recent {
			recentAvg += h.PassRate
		}
		recentAvg /= float64(len(recent))

		if len(older) > 0 {
			for _, h := range older {
				olderAvg += h.PassRate
			}
			olderAvg /= float64(len(older))

			if recentAvg > olderAvg+5 {
				trend = "improving"
			} else if recentAvg < olderAvg-5 {
				trend = "degrading"
			}
		}
	}

	// Get failing services from last run
	var failingServices []string
	testRunsMutex.RLock()
	if len(testRuns) > 0 {
		for _, r := range testRuns[0].Results {
			if r.Status == "fail" || r.Status == "error" {
				failingServices = append(failingServices, r.ServiceName)
			}
		}
	}
	testRunsMutex.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"totalRuns":       len(history),
		"avgPassRate":     avgPassRate,
		"avgLatency":      avgLatency,
		"trend":           trend,
		"lastRunTime":     history[len(history)-1].Timestamp,
		"failingServices": failingServices,
	})
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Send initial state
	testHistoryMutex.RLock()
	historyJSON, _ := json.Marshal(testHistory)
	testHistoryMutex.RUnlock()

	fmt.Fprintf(w, "event: history\ndata: %s\n\n", historyJSON)
	flusher.Flush()

	// Keep connection open and send updates
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Send current run status
			currentRunMutex.RLock()
			if currentRun != nil {
				runJSON, _ := json.Marshal(currentRun)
				fmt.Fprintf(w, "event: running\ndata: %s\n\n", runJSON)
			}
			currentRunMutex.RUnlock()

			// Send alerts
			alertsMutex.RLock()
			alertsJSON, _ := json.Marshal(activeAlerts)
			alertsMutex.RUnlock()
			fmt.Fprintf(w, "event: alerts\ndata: %s\n\n", alertsJSON)

			flusher.Flush()
		}
	}
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// API routes
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/api/run", corsMiddleware(handleRun))
	http.HandleFunc("/api/run/single", corsMiddleware(handleRunSingle))
	http.HandleFunc("/api/services", corsMiddleware(handleServices))
	http.HandleFunc("/api/history", corsMiddleware(handleHistory))
	http.HandleFunc("/api/runs", corsMiddleware(handleRuns))
	http.HandleFunc("/api/runs/get", corsMiddleware(handleRunByID))
	http.HandleFunc("/api/runs/current", corsMiddleware(handleCurrentRun))
	http.HandleFunc("/api/alerts", corsMiddleware(handleAlerts))
	http.HandleFunc("/api/stats", corsMiddleware(handleStats))
	http.HandleFunc("/api/github/workflows", corsMiddleware(handleGitHubWorkflows))
	http.HandleFunc("/api/github/trigger", corsMiddleware(handleTriggerGitHub))
	http.HandleFunc("/api/events", handleSSE)

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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Test Dashboard v2.0.0 starting on :%s", port)
	log.Printf("Monitoring %d services from services.yaml", len(getServices()))
	log.Printf("GitHub repo: %s", githubRepo)

	// Run initial health check
	go func() {
		time.Sleep(3 * time.Second)
		log.Println("Running initial health check...")
		runAllHealthChecks("startup", "system")
	}()

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
