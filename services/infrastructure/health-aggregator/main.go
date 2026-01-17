package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
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
	ColorYellow   = "#f9e2af"
	ColorLavender = "#b4befe"
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
		"service":   "health-aggregator",
		"message":   msg,
	}
	for k, v := range fields {
		entry[k] = v
	}
	json.NewEncoder(os.Stdout).Encode(entry)
}

type ServiceHealth struct {
	Name      string                 `json:"name"`
	URL       string                 `json:"url"`
	Status    string                 `json:"status"`
	Healthy   bool                   `json:"healthy"`
	Details   map[string]interface{} `json:"details,omitempty"`
	LastCheck time.Time              `json:"last_check"`
	Latency   time.Duration          `json:"latency_ms"`
}

var (
	logger         = &Logger{}
	requestCounter uint64
	healthChecks   uint64
	services       = []struct {
		Name string
		URL  string
	}{
		{"event-broker", "http://event-broker:8080/health"},
		{"event-persist", "http://event-persist:8080/health"},
		{"event-replay", "http://event-replay:8080/health"},
		{"event-dlq", "http://event-dlq:8080/health"},
		{"config-sync", "http://config-sync:8080/health"},
	}
	healthCache   = make(map[string]*ServiceHealth)
	healthCacheMu sync.RWMutex
	httpClient    = &http.Client{Timeout: 5 * time.Second}
)

func checkService(name, url string) *ServiceHealth {
	health := &ServiceHealth{
		Name:      name,
		URL:       url,
		Status:    "unknown",
		Healthy:   false,
		LastCheck: time.Now(),
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		health.Status = "error"
		health.Details = map[string]interface{}{"error": err.Error()}
		return health
	}

	resp, err := httpClient.Do(req)
	health.Latency = time.Since(start)

	if err != nil {
		health.Status = "unreachable"
		health.Details = map[string]interface{}{"error": err.Error()}
		return health
	}
	defer resp.Body.Close()

	atomic.AddUint64(&healthChecks, 1)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		health.Status = "healthy"
		health.Healthy = true
	} else if resp.StatusCode >= 500 {
		health.Status = "unhealthy"
	} else {
		health.Status = "degraded"
	}

	var details map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&details); err == nil {
		health.Details = details
	}

	return health
}

func checkAllServices() map[string]*ServiceHealth {
	results := make(map[string]*ServiceHealth)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, svc := range services {
		wg.Add(1)
		go func(name, url string) {
			defer wg.Done()
			health := checkService(name, url)
			mu.Lock()
			results[name] = health
			mu.Unlock()
		}(svc.Name, svc.URL)
	}

	wg.Wait()
	return results
}

func startHealthChecker() {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		// Initial check
		results := checkAllServices()
		healthCacheMu.Lock()
		healthCache = results
		healthCacheMu.Unlock()

		for range ticker.C {
			results := checkAllServices()
			healthCacheMu.Lock()
			healthCache = results
			healthCacheMu.Unlock()

			unhealthyCount := 0
			for _, h := range results {
				if !h.Healthy {
					unhealthyCount++
				}
			}

			if unhealthyCount > 0 {
				logger.Log("warn", "Unhealthy services detected", map[string]interface{}{"count": unhealthyCount})
			}
		}
	}()
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCounter, 1)

	healthCacheMu.RLock()
	cache := healthCache
	healthCacheMu.RUnlock()

	allHealthy := true
	for _, h := range cache {
		if !h.Healthy {
			allHealthy = false
			break
		}
	}

	status := map[string]interface{}{
		"service":        "health-aggregator",
		"status":         "healthy",
		"all_healthy":    allHealthy,
		"services_count": len(cache),
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	}

	if !allHealthy {
		status["status"] = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCounter, 1)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	healthCacheMu.RLock()
	cache := healthCache
	healthCacheMu.RUnlock()

	healthyCount := 0
	unhealthyCount := 0
	for _, h := range cache {
		if h.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}

	fmt.Fprintf(w, "# HELP health_aggregator_up Whether the service is up\n")
	fmt.Fprintf(w, "# TYPE health_aggregator_up gauge\n")
	fmt.Fprintf(w, "health_aggregator_up 1\n")

	fmt.Fprintf(w, "# HELP health_aggregator_services_total Total monitored services\n")
	fmt.Fprintf(w, "# TYPE health_aggregator_services_total gauge\n")
	fmt.Fprintf(w, "health_aggregator_services_total %d\n", len(cache))

	fmt.Fprintf(w, "# HELP health_aggregator_services_healthy Healthy services count\n")
	fmt.Fprintf(w, "# TYPE health_aggregator_services_healthy gauge\n")
	fmt.Fprintf(w, "health_aggregator_services_healthy %d\n", healthyCount)

	fmt.Fprintf(w, "# HELP health_aggregator_services_unhealthy Unhealthy services count\n")
	fmt.Fprintf(w, "# TYPE health_aggregator_services_unhealthy gauge\n")
	fmt.Fprintf(w, "health_aggregator_services_unhealthy %d\n", unhealthyCount)

	fmt.Fprintf(w, "# HELP health_aggregator_checks_total Total health checks performed\n")
	fmt.Fprintf(w, "# TYPE health_aggregator_checks_total counter\n")
	fmt.Fprintf(w, "health_aggregator_checks_total %d\n", atomic.LoadUint64(&healthChecks))

	fmt.Fprintf(w, "# HELP health_aggregator_requests_total Total HTTP requests\n")
	fmt.Fprintf(w, "# TYPE health_aggregator_requests_total counter\n")
	fmt.Fprintf(w, "health_aggregator_requests_total %d\n", atomic.LoadUint64(&requestCounter))

	// Per-service metrics
	fmt.Fprintf(w, "# HELP health_aggregator_service_healthy Service health status\n")
	fmt.Fprintf(w, "# TYPE health_aggregator_service_healthy gauge\n")
	for name, h := range cache {
		healthy := 0
		if h.Healthy {
			healthy = 1
		}
		fmt.Fprintf(w, "health_aggregator_service_healthy{service=\"%s\"} %d\n", name, healthy)
	}

	fmt.Fprintf(w, "# HELP health_aggregator_service_latency_ms Service check latency in ms\n")
	fmt.Fprintf(w, "# TYPE health_aggregator_service_latency_ms gauge\n")
	for name, h := range cache {
		fmt.Fprintf(w, "health_aggregator_service_latency_ms{service=\"%s\"} %d\n", name, h.Latency.Milliseconds())
	}
}

func allHealthHandler(w http.ResponseWriter, r *http.Request) {
	healthCacheMu.RLock()
	cache := healthCache
	healthCacheMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cache)
}

func checkHandler(w http.ResponseWriter, r *http.Request) {
	serviceName := r.URL.Query().Get("service")
	if serviceName == "" {
		http.Error(w, "Missing service parameter", http.StatusBadRequest)
		return
	}

	// Find the service URL
	var serviceURL string
	for _, svc := range services {
		if svc.Name == serviceName {
			serviceURL = svc.URL
			break
		}
	}

	if serviceURL == "" {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	// Perform the health check
	health := checkService(serviceName, serviceURL)

	// Update the cache
	healthCacheMu.Lock()
	healthCache[serviceName] = health
	healthCacheMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	results := checkAllServices()
	healthCacheMu.Lock()
	healthCache = results
	healthCacheMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func uiHandler(w http.ResponseWriter, r *http.Request) {
	healthCacheMu.RLock()
	cache := healthCache
	healthCacheMu.RUnlock()

	healthyCount := 0
	unhealthyCount := 0
	for _, h := range cache {
		if h.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}

	overallStatus := "All Systems Operational"
	overallColor := ColorGreen
	if unhealthyCount > 0 {
		overallStatus = fmt.Sprintf("%d Service(s) Degraded", unhealthyCount)
		overallColor = ColorYellow
	}
	if unhealthyCount == len(cache) {
		overallStatus = "All Systems Down"
		overallColor = ColorRed
	}

	servicesHTML := ""
	for _, svc := range services {
		h, exists := cache[svc.Name]
		statusColor := ColorSubtext0
		statusText := "Unknown"
		latency := int64(0)

		if exists {
			latency = h.Latency.Milliseconds()
			switch h.Status {
			case "healthy":
				statusColor = ColorGreen
				statusText = "Healthy"
			case "unhealthy":
				statusColor = ColorRed
				statusText = "Unhealthy"
			case "degraded":
				statusColor = ColorYellow
				statusText = "Degraded"
			case "unreachable":
				statusColor = ColorRed
				statusText = "Unreachable"
			default:
				statusText = h.Status
			}
		}

		servicesHTML += fmt.Sprintf(`
			<div class="service">
				<div class="service-name">%s</div>
				<div class="service-status">
					<span class="status-indicator" style="background: %s"></span>
					%s
					<span class="latency">%dms</span>
				</div>
			</div>`, svc.Name, statusColor, statusText, latency)
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Health Aggregator</title>
    <meta http-equiv="refresh" content="30">
    <style>
        body { background: %s; color: %s; font-family: 'JetBrains Mono', monospace; padding: 2rem; }
        .container { max-width: 900px; margin: 0 auto; }
        h1 { color: %s; }
        .overall { padding: 1.5rem; background: %s; border-radius: 8px; margin: 1rem 0; text-align: center; }
        .overall-status { font-size: 1.2rem; color: %s; font-weight: bold; }
        .services { margin-top: 2rem; }
        .service { display: flex; justify-content: space-between; align-items: center; padding: 1rem; background: %s; border-radius: 4px; margin: 0.5rem 0; }
        .service-name { color: %s; }
        .service-status { display: flex; align-items: center; gap: 0.5rem; }
        .status-indicator { display: inline-block; width: 10px; height: 10px; border-radius: 50%%; }
        .latency { color: %s; font-size: 0.8rem; margin-left: 1rem; }
        .summary { display: flex; justify-content: space-around; margin-top: 2rem; }
        .summary-item { text-align: center; padding: 1rem; background: %s; border-radius: 8px; min-width: 120px; }
        .summary-value { font-size: 2rem; color: %s; }
        .summary-label { color: %s; font-size: 0.8rem; }
        .btn { background: %s; color: %s; border: none; padding: 0.75rem 1.5rem; border-radius: 4px; cursor: pointer; margin-top: 1rem; }
        .btn:hover { opacity: 0.8; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Health Aggregator</h1>
        <div class="overall">
            <div class="overall-status">%s</div>
        </div>
        <div class="summary">
            <div class="summary-item">
                <div class="summary-value" style="color: %s">%d</div>
                <div class="summary-label">Healthy</div>
            </div>
            <div class="summary-item">
                <div class="summary-value" style="color: %s">%d</div>
                <div class="summary-label">Unhealthy</div>
            </div>
            <div class="summary-item">
                <div class="summary-value">%d</div>
                <div class="summary-label">Total</div>
            </div>
        </div>
        <div class="services">
            <h2 style="color: %s">Services</h2>
            %s
        </div>
        <form action="/refresh" method="POST">
            <button type="submit" class="btn">Refresh Now</button>
        </form>
    </div>
</body>
</html>`, ColorBase, ColorText, ColorLavender, ColorMantle, overallColor, ColorCrust, ColorBlue, ColorSubtext0,
		ColorMantle, ColorMauve, ColorSubtext0, ColorMauve, ColorBase, overallStatus,
		ColorGreen, healthyCount, ColorRed, unhealthyCount, len(cache), ColorSubtext0, servicesHTML)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	logger.Log("info", "Starting health-aggregator service", nil)

	startHealthChecker()

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)
	http.HandleFunc("/all", allHealthHandler)
	http.HandleFunc("/check", checkHandler)
	http.HandleFunc("/refresh", refreshHandler)
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
