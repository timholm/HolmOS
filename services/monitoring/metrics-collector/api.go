package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ServiceStatus represents the health status of a service
type ServiceStatus struct {
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Healthy        bool      `json:"healthy"`
	LastCheck      time.Time `json:"last_check,omitempty"`
	AvgResponseMs  float64   `json:"avg_response_ms"`
	ErrorRate      float64   `json:"error_rate"`
	TotalRequests  int       `json:"total_requests"`
	TotalErrors    int       `json:"total_errors"`
}

// ServiceListResponse represents the response for listing services
type ServiceListResponse struct {
	Services []ServiceEndpoint `json:"services"`
	Count    int               `json:"count"`
}

// ServiceMetricsResponse represents detailed metrics for a service
type ServiceMetricsResponse struct {
	Name         string        `json:"name"`
	URL          string        `json:"url"`
	DataPoints   []MetricPoint `json:"data_points"`
	Summary      MetricSummary `json:"summary"`
}

// MetricSummary provides aggregated metrics
type MetricSummary struct {
	TotalRequests   int     `json:"total_requests"`
	TotalErrors     int     `json:"total_errors"`
	AvgResponseMs   float64 `json:"avg_response_ms"`
	MinResponseMs   float64 `json:"min_response_ms"`
	MaxResponseMs   float64 `json:"max_response_ms"`
	ErrorRate       float64 `json:"error_rate"`
	LastResponseMs  float64 `json:"last_response_ms"`
}

// handleListServices returns all registered services
// GET /services
func (mc *MetricsCollector) handleListServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mc.mu.RLock()
	services := make([]ServiceEndpoint, 0, len(mc.endpoints))
	for _, ep := range mc.endpoints {
		services = append(services, ep)
	}
	mc.mu.RUnlock()

	response := ServiceListResponse{
		Services: services,
		Count:    len(services),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetServiceMetrics returns detailed metrics for a specific service
// GET /services/{name}/metrics
func (mc *MetricsCollector) handleGetServiceMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service name from path: /services/{name}/metrics
	path := strings.TrimPrefix(r.URL.Path, "/services/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "metrics" {
		http.Error(w, "Invalid path. Use /services/{name}/metrics", http.StatusBadRequest)
		return
	}
	serviceName := parts[0]

	mc.mu.RLock()
	svc, exists := mc.services[serviceName]
	ep, epExists := mc.endpoints[serviceName]
	mc.mu.RUnlock()

	if !exists || !epExists {
		http.Error(w, fmt.Sprintf("Service %s not found", serviceName), http.StatusNotFound)
		return
	}

	svc.mu.RLock()
	points := make([]MetricPoint, len(svc.DataPoints))
	copy(points, svc.DataPoints)
	svc.mu.RUnlock()

	// Calculate summary
	summary := calculateSummary(points)

	response := ServiceMetricsResponse{
		Name:       serviceName,
		URL:        ep.URL,
		DataPoints: points,
		Summary:    summary,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCollectService triggers collection for a specific service
// POST /services/{name}/collect
func (mc *MetricsCollector) handleCollectService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service name from path: /services/{name}/collect
	path := strings.TrimPrefix(r.URL.Path, "/services/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "collect" {
		http.Error(w, "Invalid path. Use /services/{name}/collect", http.StatusBadRequest)
		return
	}
	serviceName := parts[0]

	err := mc.CollectFromService(serviceName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	response := struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Service string `json:"service"`
	}{
		Status:  "collected",
		Message: fmt.Sprintf("Metrics collected from service %s", serviceName),
		Service: serviceName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleUnregisterService removes a service from monitoring
// DELETE /services/{name}
func (mc *MetricsCollector) handleUnregisterService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service name from path: /services/{name}
	serviceName := strings.TrimPrefix(r.URL.Path, "/services/")
	serviceName = strings.TrimSuffix(serviceName, "/")

	if serviceName == "" {
		http.Error(w, "Service name is required", http.StatusBadRequest)
		return
	}

	mc.mu.Lock()
	_, exists := mc.endpoints[serviceName]
	if !exists {
		mc.mu.Unlock()
		http.Error(w, fmt.Sprintf("Service %s not found", serviceName), http.StatusNotFound)
		return
	}
	delete(mc.endpoints, serviceName)
	delete(mc.services, serviceName)
	mc.mu.Unlock()

	response := struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Service string `json:"service"`
	}{
		Status:  "unregistered",
		Message: fmt.Sprintf("Service %s has been unregistered", serviceName),
		Service: serviceName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetServiceStatus returns health status for a specific service
// GET /services/{name}/status
func (mc *MetricsCollector) handleGetServiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service name from path: /services/{name}/status
	path := strings.TrimPrefix(r.URL.Path, "/services/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "status" {
		http.Error(w, "Invalid path. Use /services/{name}/status", http.StatusBadRequest)
		return
	}
	serviceName := parts[0]

	mc.mu.RLock()
	svc, exists := mc.services[serviceName]
	ep, epExists := mc.endpoints[serviceName]
	mc.mu.RUnlock()

	if !exists || !epExists {
		http.Error(w, fmt.Sprintf("Service %s not found", serviceName), http.StatusNotFound)
		return
	}

	svc.mu.RLock()
	points := make([]MetricPoint, len(svc.DataPoints))
	copy(points, svc.DataPoints)
	svc.mu.RUnlock()

	status := ServiceStatus{
		Name: serviceName,
		URL:  ep.URL,
	}

	if len(points) > 0 {
		var totalErrors int
		var totalResponseTime float64
		for _, p := range points {
			totalResponseTime += p.ResponseTime
			if p.Error {
				totalErrors++
			}
		}
		status.TotalRequests = len(points)
		status.TotalErrors = totalErrors
		status.AvgResponseMs = totalResponseTime / float64(len(points))
		status.ErrorRate = float64(totalErrors) / float64(len(points))
		status.LastCheck = points[len(points)-1].Timestamp
		// Consider healthy if error rate is below 50%
		status.Healthy = status.ErrorRate < 0.5
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleServicesRouter routes requests to the appropriate service handler
// Routes: /services/{name}, /services/{name}/metrics, /services/{name}/status, /services/{name}/collect
func (mc *MetricsCollector) handleServicesRouter(w http.ResponseWriter, r *http.Request) {
	// Extract path after /services/
	path := strings.TrimPrefix(r.URL.Path, "/services/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Service name is required", http.StatusBadRequest)
		return
	}

	// Route based on path structure
	if len(parts) == 1 {
		// /services/{name} - DELETE to unregister
		mc.handleUnregisterService(w, r)
		return
	}

	// /services/{name}/{action}
	action := parts[1]
	switch action {
	case "metrics":
		mc.handleGetServiceMetrics(w, r)
	case "status":
		mc.handleGetServiceStatus(w, r)
	case "collect":
		mc.handleCollectService(w, r)
	default:
		http.Error(w, fmt.Sprintf("Unknown action: %s", action), http.StatusNotFound)
	}
}

// calculateSummary computes aggregated metrics from data points
func calculateSummary(points []MetricPoint) MetricSummary {
	summary := MetricSummary{}

	if len(points) == 0 {
		return summary
	}

	var totalResponseTime float64
	minResponse := points[0].ResponseTime
	maxResponse := points[0].ResponseTime

	for _, p := range points {
		summary.TotalRequests++
		totalResponseTime += p.ResponseTime

		if p.ResponseTime < minResponse {
			minResponse = p.ResponseTime
		}
		if p.ResponseTime > maxResponse {
			maxResponse = p.ResponseTime
		}

		if p.Error {
			summary.TotalErrors++
		}
	}

	summary.AvgResponseMs = totalResponseTime / float64(summary.TotalRequests)
	summary.MinResponseMs = minResponse
	summary.MaxResponseMs = maxResponse
	summary.ErrorRate = float64(summary.TotalErrors) / float64(summary.TotalRequests)
	summary.LastResponseMs = points[len(points)-1].ResponseTime

	return summary
}
