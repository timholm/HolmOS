package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

var port = os.Getenv("PORT")

// Service represents a monitored service
type Service struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Critical bool   `json:"critical"`
}

// HealthResult represents the health check result for a service
type HealthResult struct {
	Name         string  `json:"name"`
	URL          string  `json:"url"`
	Status       string  `json:"status"` // healthy, unhealthy, degraded
	ResponseTime float64 `json:"responseTimeMs"`
	StatusCode   int     `json:"statusCode"`
	Error        string  `json:"error,omitempty"`
	Critical     bool    `json:"critical"`
	LastChecked  string  `json:"lastChecked"`
}

// AggregatedHealth represents the overall system health
type AggregatedHealth struct {
	Status           string         `json:"status"` // healthy, degraded, unhealthy
	TotalServices    int            `json:"totalServices"`
	HealthyCount     int            `json:"healthyCount"`
	UnhealthyCount   int            `json:"unhealthyCount"`
	DegradedCount    int            `json:"degradedCount"`
	CriticalFailures int            `json:"criticalFailures"`
	AvgResponseTime  float64        `json:"avgResponseTimeMs"`
	Services         []HealthResult `json:"services"`
	Timestamp        string         `json:"timestamp"`
	Uptime           float64        `json:"uptimePercent"`
}

// Services to monitor - matches Steve's endpoint list
var services = []Service{
	{Name: "Nova Dashboard", URL: "http://192.168.8.197:30004/api/dashboard", Critical: true},
	{Name: "Nova Nodes API", URL: "http://192.168.8.197:30004/api/nodes", Critical: false},
	{Name: "Nova Pods API", URL: "http://192.168.8.197:30004/api/pods", Critical: false},
	{Name: "Cluster Manager", URL: "http://192.168.8.197:30502/api/v1/nodes", Critical: true},
	{Name: "CI/CD Dashboard", URL: "http://192.168.8.197:30020/", Critical: true},
	{Name: "CI/CD Builds API", URL: "http://192.168.8.197:30020/api/builds", Critical: true},
	{Name: "CI/CD Pipelines", URL: "http://192.168.8.197:30020/api/pipelines", Critical: true},
	{Name: "HolmGit UI", URL: "http://192.168.8.197:30009/", Critical: false},
	{Name: "HolmGit Repos", URL: "http://192.168.8.197:30009/api/repos", Critical: true},
	{Name: "Container Registry", URL: "http://192.168.8.197:30009/api/registry/repos", Critical: false},
	{Name: "Deploy Controller", URL: "http://192.168.8.197:30015/", Critical: false},
	{Name: "Deployments API", URL: "http://192.168.8.197:30015/api/deployments", Critical: true},
	{Name: "Scribe Logs", URL: "http://192.168.8.197:30017/api/logs", Critical: false},
	{Name: "Backup Jobs", URL: "http://192.168.8.197:30016/api/jobs", Critical: false},
	{Name: "App Store", URL: "http://192.168.8.197:30002/api/apps", Critical: false},
	{Name: "Metrics API", URL: "http://192.168.8.197:30950/api/metrics", Critical: false},
	{Name: "iOS Shell", URL: "http://192.168.8.197:30001/", Critical: false},
	{Name: "Files API", URL: "http://192.168.8.197:30088/api/list?path=/", Critical: false},
	{Name: "Terminal", URL: "http://192.168.8.197:30800/", Critical: false},
	{Name: "Calculator", URL: "http://192.168.8.197:30010/", Critical: false},
	{Name: "Vault", URL: "http://192.168.8.197:30870/", Critical: false},
	{Name: "Steve Bot", URL: "http://192.168.8.197:30666/health", Critical: false},
}

var (
	cachedHealth     *AggregatedHealth
	cacheMu          sync.RWMutex
	healthHistory    []AggregatedHealth
	historyMu        sync.RWMutex
	uptimeStats      map[string][]bool
	uptimeMu         sync.RWMutex
)

func init() {
	uptimeStats = make(map[string][]bool)
	for _, s := range services {
		uptimeStats[s.Name] = make([]bool, 0)
	}
}

func checkService(svc Service) HealthResult {
	result := HealthResult{
		Name:        svc.Name,
		URL:         svc.URL,
		Critical:    svc.Critical,
		LastChecked: time.Now().Format(time.RFC3339),
	}

	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()

	resp, err := client.Get(svc.URL)
	elapsed := time.Since(start).Seconds() * 1000

	result.ResponseTime = elapsed

	if err != nil {
		result.Status = "unhealthy"
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode >= 500 {
		result.Status = "unhealthy"
		result.Error = fmt.Sprintf("Server error: %d", resp.StatusCode)
	} else if resp.StatusCode >= 400 {
		result.Status = "degraded"
		result.Error = fmt.Sprintf("Client error: %d", resp.StatusCode)
	} else if elapsed > 1000 {
		result.Status = "degraded"
		result.Error = "Slow response time"
	} else {
		result.Status = "healthy"
	}

	return result
}

func checkAllServices() *AggregatedHealth {
	var wg sync.WaitGroup
	results := make([]HealthResult, len(services))

	for i, svc := range services {
		wg.Add(1)
		go func(idx int, s Service) {
			defer wg.Done()
			results[idx] = checkService(s)
		}(i, svc)
	}

	wg.Wait()

	// Calculate aggregates
	health := &AggregatedHealth{
		TotalServices: len(results),
		Services:      results,
		Timestamp:     time.Now().Format(time.RFC3339),
	}

	var totalTime float64
	for _, r := range results {
		totalTime += r.ResponseTime

		switch r.Status {
		case "healthy":
			health.HealthyCount++
		case "unhealthy":
			health.UnhealthyCount++
			if r.Critical {
				health.CriticalFailures++
			}
		case "degraded":
			health.DegradedCount++
		}

		// Update uptime stats
		uptimeMu.Lock()
		stats := uptimeStats[r.Name]
		stats = append(stats, r.Status == "healthy")
		if len(stats) > 100 { // Keep last 100 checks
			stats = stats[1:]
		}
		uptimeStats[r.Name] = stats
		uptimeMu.Unlock()
	}

	health.AvgResponseTime = totalTime / float64(len(results))

	// Calculate overall uptime
	uptimeMu.RLock()
	var totalChecks, successfulChecks int
	for _, stats := range uptimeStats {
		for _, ok := range stats {
			totalChecks++
			if ok {
				successfulChecks++
			}
		}
	}
	uptimeMu.RUnlock()

	if totalChecks > 0 {
		health.Uptime = float64(successfulChecks) / float64(totalChecks) * 100
	}

	// Determine overall status
	if health.CriticalFailures > 0 {
		health.Status = "unhealthy"
	} else if health.UnhealthyCount > 0 || health.DegradedCount > 2 {
		health.Status = "degraded"
	} else {
		health.Status = "healthy"
	}

	return health
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	cacheMu.RLock()
	cached := cachedHealth
	cacheMu.RUnlock()

	if cached == nil {
		// First request, do a fresh check
		cached = checkAllServices()
		cacheMu.Lock()
		cachedHealth = cached
		cacheMu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cached)
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	health := checkAllServices()

	cacheMu.Lock()
	cachedHealth = health
	cacheMu.Unlock()

	// Store in history
	historyMu.Lock()
	healthHistory = append(healthHistory, *health)
	if len(healthHistory) > 1000 {
		healthHistory = healthHistory[1:]
	}
	historyMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func historyHandler(w http.ResponseWriter, r *http.Request) {
	historyMu.RLock()
	defer historyMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(healthHistory),
		"history": healthHistory,
	})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	cacheMu.RLock()
	cached := cachedHealth
	cacheMu.RUnlock()

	status := "unknown"
	if cached != nil {
		status = cached.Status
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    status,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func prometheusHandler(w http.ResponseWriter, r *http.Request) {
	cacheMu.RLock()
	cached := cachedHealth
	cacheMu.RUnlock()

	if cached == nil {
		cached = checkAllServices()
	}

	w.Header().Set("Content-Type", "text/plain")

	// Output Prometheus-style metrics
	fmt.Fprintf(w, "# HELP holmos_service_up Service availability (1=up, 0=down)\n")
	fmt.Fprintf(w, "# TYPE holmos_service_up gauge\n")
	for _, s := range cached.Services {
		up := 0
		if s.Status == "healthy" {
			up = 1
		}
		fmt.Fprintf(w, "holmos_service_up{service=\"%s\",critical=\"%t\"} %d\n", s.Name, s.Critical, up)
	}

	fmt.Fprintf(w, "\n# HELP holmos_service_response_time_ms Service response time in milliseconds\n")
	fmt.Fprintf(w, "# TYPE holmos_service_response_time_ms gauge\n")
	for _, s := range cached.Services {
		fmt.Fprintf(w, "holmos_service_response_time_ms{service=\"%s\"} %.2f\n", s.Name, s.ResponseTime)
	}

	fmt.Fprintf(w, "\n# HELP holmos_health_total Total services by status\n")
	fmt.Fprintf(w, "# TYPE holmos_health_total gauge\n")
	fmt.Fprintf(w, "holmos_health_total{status=\"healthy\"} %d\n", cached.HealthyCount)
	fmt.Fprintf(w, "holmos_health_total{status=\"unhealthy\"} %d\n", cached.UnhealthyCount)
	fmt.Fprintf(w, "holmos_health_total{status=\"degraded\"} %d\n", cached.DegradedCount)

	fmt.Fprintf(w, "\n# HELP holmos_uptime_percent Overall system uptime percentage\n")
	fmt.Fprintf(w, "# TYPE holmos_uptime_percent gauge\n")
	fmt.Fprintf(w, "holmos_uptime_percent %.2f\n", cached.Uptime)
}

func backgroundChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		health := checkAllServices()

		cacheMu.Lock()
		cachedHealth = health
		cacheMu.Unlock()

		historyMu.Lock()
		healthHistory = append(healthHistory, *health)
		if len(healthHistory) > 1000 {
			healthHistory = healthHistory[1:]
		}
		historyMu.Unlock()

		log.Printf("Health check: %s - %d/%d healthy, avg %.0fms, uptime %.1f%%",
			health.Status, health.HealthyCount, health.TotalServices,
			health.AvgResponseTime, health.Uptime)

		<-ticker.C
	}
}

func main() {
	if port == "" {
		port = "8080"
	}

	log.Println("Health Aggregation Service starting...")
	log.Printf("Monitoring %d services", len(services))

	// Start background health checker
	go backgroundChecker()

	http.HandleFunc("/", statusHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/health", healthHandler)
	http.HandleFunc("/api/health/refresh", refreshHandler)
	http.HandleFunc("/api/health/history", historyHandler)
	http.HandleFunc("/metrics", prometheusHandler)

	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
