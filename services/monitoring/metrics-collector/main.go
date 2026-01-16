package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ServiceEndpoint represents a registered service to collect metrics from
type ServiceEndpoint struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Interval int    `json:"interval"` // collection interval in seconds
}

// MetricPoint represents a single metric data point
type MetricPoint struct {
	Timestamp    time.Time
	ResponseTime float64 // in milliseconds
	StatusCode   int
	Error        bool
}

// ServiceMetrics holds metrics for a single service
type ServiceMetrics struct {
	Name         string
	URL          string
	DataPoints   []MetricPoint
	mu           sync.RWMutex
}

// MetricsCollector manages all service metrics
type MetricsCollector struct {
	services   map[string]*ServiceMetrics
	endpoints  map[string]ServiceEndpoint
	mu         sync.RWMutex
	retention  time.Duration
	httpClient *http.Client
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(retention time.Duration) *MetricsCollector {
	mc := &MetricsCollector{
		services:  make(map[string]*ServiceMetrics),
		endpoints: make(map[string]ServiceEndpoint),
		retention: retention,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	// Start cleanup goroutine
	go mc.cleanupLoop()
	return mc
}

// cleanupLoop periodically removes old data points
func (mc *MetricsCollector) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		mc.cleanup()
	}
}

// cleanup removes data points older than retention period
func (mc *MetricsCollector) cleanup() {
	mc.mu.RLock()
	services := make([]*ServiceMetrics, 0, len(mc.services))
	for _, svc := range mc.services {
		services = append(services, svc)
	}
	mc.mu.RUnlock()

	cutoff := time.Now().Add(-mc.retention)
	for _, svc := range services {
		svc.mu.Lock()
		newPoints := make([]MetricPoint, 0)
		for _, p := range svc.DataPoints {
			if p.Timestamp.After(cutoff) {
				newPoints = append(newPoints, p)
			}
		}
		svc.DataPoints = newPoints
		svc.mu.Unlock()
	}
}

// RegisterService registers a new service endpoint
func (mc *MetricsCollector) RegisterService(ep ServiceEndpoint) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.endpoints[ep.Name] = ep
	if _, exists := mc.services[ep.Name]; !exists {
		mc.services[ep.Name] = &ServiceMetrics{
			Name:       ep.Name,
			URL:        ep.URL,
			DataPoints: make([]MetricPoint, 0),
		}
	}
	log.Printf("Registered service: %s at %s", ep.Name, ep.URL)
}

// CollectFromService collects metrics from a single service
func (mc *MetricsCollector) CollectFromService(name string) error {
	mc.mu.RLock()
	ep, exists := mc.endpoints[name]
	svc := mc.services[name]
	mc.mu.RUnlock()

	if !exists {
		return fmt.Errorf("service %s not registered", name)
	}

	start := time.Now()
	resp, err := mc.httpClient.Get(ep.URL)
	responseTime := float64(time.Since(start).Milliseconds())

	point := MetricPoint{
		Timestamp:    time.Now(),
		ResponseTime: responseTime,
	}

	if err != nil {
		point.Error = true
		point.StatusCode = 0
	} else {
		point.StatusCode = resp.StatusCode
		point.Error = resp.StatusCode >= 400
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	svc.mu.Lock()
	svc.DataPoints = append(svc.DataPoints, point)
	svc.mu.Unlock()

	return nil
}

// CollectAll collects metrics from all registered services
func (mc *MetricsCollector) CollectAll() map[string]error {
	mc.mu.RLock()
	names := make([]string, 0, len(mc.endpoints))
	for name := range mc.endpoints {
		names = append(names, name)
	}
	mc.mu.RUnlock()

	errors := make(map[string]error)
	var wg sync.WaitGroup

	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if err := mc.CollectFromService(n); err != nil {
				errors[n] = err
			}
		}(name)
	}

	wg.Wait()
	return errors
}

// GetPrometheusMetrics returns metrics in Prometheus format
func (mc *MetricsCollector) GetPrometheusMetrics() string {
	mc.mu.RLock()
	services := make([]*ServiceMetrics, 0, len(mc.services))
	for _, svc := range mc.services {
		services = append(services, svc)
	}
	mc.mu.RUnlock()

	var sb strings.Builder

	// Write metric help and type headers
	sb.WriteString("# HELP http_request_duration_milliseconds HTTP request duration in milliseconds\n")
	sb.WriteString("# TYPE http_request_duration_milliseconds gauge\n")
	sb.WriteString("# HELP http_requests_total Total number of HTTP requests\n")
	sb.WriteString("# TYPE http_requests_total counter\n")
	sb.WriteString("# HELP http_request_errors_total Total number of HTTP request errors\n")
	sb.WriteString("# TYPE http_request_errors_total counter\n")
	sb.WriteString("# HELP http_response_time_avg_milliseconds Average HTTP response time in milliseconds\n")
	sb.WriteString("# TYPE http_response_time_avg_milliseconds gauge\n")
	sb.WriteString("# HELP http_error_rate Error rate (0-1)\n")
	sb.WriteString("# TYPE http_error_rate gauge\n")

	// Sort services for consistent output
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	for _, svc := range services {
		svc.mu.RLock()
		points := make([]MetricPoint, len(svc.DataPoints))
		copy(points, svc.DataPoints)
		name := svc.Name
		svc.mu.RUnlock()

		if len(points) == 0 {
			continue
		}

		// Calculate metrics
		var totalRequests int64
		var totalErrors int64
		var totalResponseTime float64
		var lastResponseTime float64

		for _, p := range points {
			totalRequests++
			totalResponseTime += p.ResponseTime
			lastResponseTime = p.ResponseTime
			if p.Error {
				totalErrors++
			}
		}

		avgResponseTime := totalResponseTime / float64(totalRequests)
		errorRate := float64(totalErrors) / float64(totalRequests)

		// Write metrics
		sb.WriteString(fmt.Sprintf("http_request_duration_milliseconds{service=\"%s\"} %.2f\n", name, lastResponseTime))
		sb.WriteString(fmt.Sprintf("http_requests_total{service=\"%s\"} %d\n", name, totalRequests))
		sb.WriteString(fmt.Sprintf("http_request_errors_total{service=\"%s\"} %d\n", name, totalErrors))
		sb.WriteString(fmt.Sprintf("http_response_time_avg_milliseconds{service=\"%s\"} %.2f\n", name, avgResponseTime))
		sb.WriteString(fmt.Sprintf("http_error_rate{service=\"%s\"} %.4f\n", name, errorRate))
	}

	return sb.String()
}

// HTTP Handlers

func (mc *MetricsCollector) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Write([]byte(mc.GetPrometheusMetrics()))
}

func (mc *MetricsCollector) handleCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	errors := mc.CollectAll()

	response := struct {
		Status    string            `json:"status"`
		Collected int               `json:"collected"`
		Errors    map[string]string `json:"errors,omitempty"`
	}{
		Status:    "ok",
		Collected: len(mc.endpoints) - len(errors),
		Errors:    make(map[string]string),
	}

	for name, err := range errors {
		response.Errors[name] = err.Error()
	}

	if len(errors) > 0 {
		response.Status = "partial"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (mc *MetricsCollector) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := struct {
		Status            string `json:"status"`
		RegisteredServices int    `json:"registered_services"`
		Uptime            string `json:"uptime"`
	}{
		Status:            "healthy",
		RegisteredServices: len(mc.endpoints),
		Uptime:            time.Since(startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (mc *MetricsCollector) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var ep ServiceEndpoint
	if err := json.NewDecoder(r.Body).Decode(&ep); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if ep.Name == "" || ep.URL == "" {
		http.Error(w, "Name and URL are required", http.StatusBadRequest)
		return
	}

	if ep.Interval <= 0 {
		ep.Interval = 30 // default 30 seconds
	}

	mc.RegisterService(ep)

	response := struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}{
		Status:  "registered",
		Message: fmt.Sprintf("Service %s registered successfully", ep.Name),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

var startTime time.Time

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func main() {
	startTime = time.Now()

	// Configurable retention (default 60 minutes)
	retentionMinutes := getEnvInt("RETENTION_MINUTES", 60)
	collector := NewMetricsCollector(time.Duration(retentionMinutes) * time.Minute)

	log.Printf("Retention period: %d minutes", retentionMinutes)

	// Setup routes
	http.HandleFunc("/metrics", collector.handleMetrics)
	http.HandleFunc("/collect", collector.handleCollect)
	http.HandleFunc("/health", collector.handleHealth)
	http.HandleFunc("/register", collector.handleRegister)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("metrics-collector starting on port %s", port)
	log.Printf("Endpoints: GET /metrics, GET /collect, GET /health, POST /register")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
