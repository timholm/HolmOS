package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Service represents a backend service
type Service struct {
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	HealthURL   string    `json:"health_url"`
	Healthy     bool      `json:"healthy"`
	LastCheck   time.Time `json:"last_check"`
	Latency     int64     `json:"latency_ms"`
	RequestRate int64     `json:"request_rate"`
	ErrorRate   int64     `json:"error_rate"`
	Weight      int       `json:"weight"`
	index       int       // for round-robin
}

// Route represents a routing rule
type Route struct {
	Path        string   `json:"path"`
	Service     string   `json:"service"`
	Methods     []string `json:"methods,omitempty"`
	StripPrefix bool     `json:"strip_prefix"`
	RateLimit   int      `json:"rate_limit,omitempty"` // requests per minute
	Timeout     int      `json:"timeout_seconds,omitempty"`
}

// RateLimiter for per-client rate limiting
type RateLimiter struct {
	requests map[string]*clientRate
	mu       sync.RWMutex
	limit    int
	window   time.Duration
}

type clientRate struct {
	count     int64
	windowEnd time.Time
}

// GatewayMetrics tracks gateway performance
type GatewayMetrics struct {
	TotalRequests   int64            `json:"total_requests"`
	SuccessRequests int64            `json:"success_requests"`
	ErrorRequests   int64            `json:"error_requests"`
	ActiveConns     int64            `json:"active_connections"`
	AvgLatency      float64          `json:"avg_latency_ms"`
	ServiceMetrics  map[string]int64 `json:"service_requests"`
	mu              sync.RWMutex
	latencySum      int64
	latencyCount    int64
}

var (
	services      = make(map[string]*Service)
	routes        = []Route{}
	serviceMu     sync.RWMutex
	routeMu       sync.RWMutex
	metrics       = &GatewayMetrics{ServiceMetrics: make(map[string]int64)}
	globalLimiter *RateLimiter
	startTime     = time.Now()
)

func main() {
	log.Println("HolmOS Gateway Agent v1.0 - All roads lead through me")

	// Initialize rate limiter (default 1000 requests per minute per client)
	globalLimiter = newRateLimiter(getEnvInt("RATE_LIMIT", 1000), time.Minute)

	// Initialize services from environment or use defaults
	initServices()
	initRoutes()

	// Start health checker
	go healthChecker()

	// Start metrics aggregator
	go metricsAggregator()

	// HTTP server
	mux := http.NewServeMux()

	// Admin API
	mux.HandleFunc("/admin", handleAdmin)
	mux.HandleFunc("/admin/services", handleAdminServices)
	mux.HandleFunc("/admin/routes", handleAdminRoutes)
	mux.HandleFunc("/admin/metrics", handleAdminMetrics)

	// Health endpoints
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/ready", handleReady)

	// API endpoints
	mux.HandleFunc("/api/services", handleAPIServices)
	mux.HandleFunc("/api/services/", handleAPIServiceByName)
	mux.HandleFunc("/api/routes", handleAPIRoutes)
	mux.HandleFunc("/api/metrics", handleAPIMetrics)
	mux.HandleFunc("/api/health", handleAPIHealth)

	// Gateway proxy - catch all
	mux.HandleFunc("/", handleProxy)

	port := getEnv("PORT", "8080")
	log.Printf("Gateway starting on port %s", port)
	log.Printf("Registered %d services, %d routes", len(services), len(routes))
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(loggingMiddleware(mux))))
}

func initServices() {
	// Default HolmOS services
	defaultServices := []Service{
		{Name: "auth-gateway", URL: "http://auth-gateway.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
		{Name: "metrics-dashboard", URL: "http://metrics-dashboard.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
		{Name: "deploy-controller", URL: "http://deploy-controller.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
		{Name: "notification-hub", URL: "http://notification-hub.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
		{Name: "backup-dashboard", URL: "http://backup-dashboard.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
		{Name: "file-web", URL: "http://file-web.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
		{Name: "terminal-web", URL: "http://terminal-web.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
		{Name: "settings-web", URL: "http://settings-web.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
		{Name: "calculator-app", URL: "http://calculator-app.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
		{Name: "test-dashboard", URL: "http://test-dashboard.holm.svc.cluster.local", HealthURL: "/health", Weight: 1},
	}

	// Load custom services from environment
	customServices := os.Getenv("GATEWAY_SERVICES")
	if customServices != "" {
		var custom []Service
		if err := json.Unmarshal([]byte(customServices), &custom); err == nil {
			defaultServices = append(defaultServices, custom...)
		}
	}

	for _, svc := range defaultServices {
		svc.Healthy = false
		svc.LastCheck = time.Time{}
		services[svc.Name] = &svc
	}
}

func initRoutes() {
	// Default routes
	defaultRoutes := []Route{
		{Path: "/auth/", Service: "auth-gateway", StripPrefix: true, RateLimit: 100},
		{Path: "/metrics/", Service: "metrics-dashboard", StripPrefix: true},
		{Path: "/deploy/", Service: "deploy-controller", StripPrefix: true},
		{Path: "/notify/", Service: "notification-hub", StripPrefix: true},
		{Path: "/backup/", Service: "backup-dashboard", StripPrefix: true},
		{Path: "/files/", Service: "file-web", StripPrefix: true},
		{Path: "/terminal/", Service: "terminal-web", StripPrefix: true},
		{Path: "/settings/", Service: "settings-web", StripPrefix: true},
		{Path: "/calculator/", Service: "calculator-app", StripPrefix: true},
		{Path: "/test/", Service: "test-dashboard", StripPrefix: true},
	}

	// Load custom routes from environment
	customRoutes := os.Getenv("GATEWAY_ROUTES")
	if customRoutes != "" {
		var custom []Route
		if err := json.Unmarshal([]byte(customRoutes), &custom); err == nil {
			defaultRoutes = append(defaultRoutes, custom...)
		}
	}

	// Sort routes by path length (longest first for most specific match)
	sort.Slice(defaultRoutes, func(i, j int) bool {
		return len(defaultRoutes[i].Path) > len(defaultRoutes[j].Path)
	})

	routes = defaultRoutes
}

func newRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string]*clientRate),
		limit:    limit,
		window:   window,
	}
	// Cleanup old entries periodically
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			rl.cleanup()
		}
	}()
	return rl
}

func (rl *RateLimiter) Allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rate, exists := rl.requests[clientIP]

	if !exists || now.After(rate.windowEnd) {
		rl.requests[clientIP] = &clientRate{
			count:     1,
			windowEnd: now.Add(rl.window),
		}
		return true
	}

	if rate.count >= int64(rl.limit) {
		return false
	}

	rate.count++
	return true
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, rate := range rl.requests {
		if now.After(rate.windowEnd) {
			delete(rl.requests, ip)
		}
	}
}

func healthChecker() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Initial check
	checkAllServices()

	for range ticker.C {
		checkAllServices()
	}
}

func checkAllServices() {
	serviceMu.RLock()
	svcs := make([]*Service, 0, len(services))
	for _, svc := range services {
		svcs = append(svcs, svc)
	}
	serviceMu.RUnlock()

	var wg sync.WaitGroup
	for _, svc := range svcs {
		wg.Add(1)
		go func(s *Service) {
			defer wg.Done()
			checkServiceHealth(s)
		}(svc)
	}
	wg.Wait()
}

func checkServiceHealth(svc *Service) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	healthURL := svc.URL + svc.HealthURL
	start := time.Now()

	resp, err := client.Get(healthURL)
	latency := time.Since(start).Milliseconds()

	serviceMu.Lock()
	defer serviceMu.Unlock()

	svc.LastCheck = time.Now()
	svc.Latency = latency

	if err != nil {
		svc.Healthy = false
		return
	}
	defer resp.Body.Close()

	svc.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 400
}

func metricsAggregator() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// Reset request rates for services
		serviceMu.Lock()
		for _, svc := range services {
			atomic.StoreInt64(&svc.RequestRate, 0)
			atomic.StoreInt64(&svc.ErrorRate, 0)
		}
		serviceMu.Unlock()

		// Calculate average latency
		metrics.mu.Lock()
		if metrics.latencyCount > 0 {
			metrics.AvgLatency = float64(metrics.latencySum) / float64(metrics.latencyCount)
		}
		metrics.latencySum = 0
		metrics.latencyCount = 0
		metrics.mu.Unlock()
	}
}

func handleProxy(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&metrics.TotalRequests, 1)
	atomic.AddInt64(&metrics.ActiveConns, 1)
	defer atomic.AddInt64(&metrics.ActiveConns, -1)

	// Rate limiting
	clientIP := getClientIP(r)
	if !globalLimiter.Allow(clientIP) {
		atomic.AddInt64(&metrics.ErrorRequests, 1)
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Find matching route
	route := findRoute(r.URL.Path, r.Method)
	if route == nil {
		// Default response for unmatched routes
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "HolmOS Gateway",
			"version": "1.0",
			"message": "All roads lead through me",
			"routes":  len(routes),
		})
		return
	}

	// Get service
	serviceMu.RLock()
	svc, exists := services[route.Service]
	serviceMu.RUnlock()

	if !exists || !svc.Healthy {
		atomic.AddInt64(&metrics.ErrorRequests, 1)
		http.Error(w, fmt.Sprintf("Service %s unavailable", route.Service), http.StatusServiceUnavailable)
		return
	}

	// Increment service metrics
	atomic.AddInt64(&svc.RequestRate, 1)
	metrics.mu.Lock()
	metrics.ServiceMetrics[route.Service]++
	metrics.mu.Unlock()

	// Proxy request
	target, _ := url.Parse(svc.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Custom director to modify request
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		if route.StripPrefix {
			req.URL.Path = strings.TrimPrefix(req.URL.Path, strings.TrimSuffix(route.Path, "/"))
			if req.URL.Path == "" {
				req.URL.Path = "/"
			}
		}
		req.Header.Set("X-Forwarded-For", clientIP)
		req.Header.Set("X-Gateway-Service", route.Service)
		req.Header.Set("X-Request-ID", fmt.Sprintf("%d", time.Now().UnixNano()))
	}

	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		atomic.AddInt64(&metrics.ErrorRequests, 1)
		atomic.AddInt64(&svc.ErrorRate, 1)
		http.Error(w, fmt.Sprintf("Gateway error: %v", err), http.StatusBadGateway)
	}

	// Modify response
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			atomic.AddInt64(&metrics.SuccessRequests, 1)
		} else {
			atomic.AddInt64(&metrics.ErrorRequests, 1)
		}
		return nil
	}

	start := time.Now()
	proxy.ServeHTTP(w, r)
	latency := time.Since(start).Milliseconds()

	metrics.mu.Lock()
	metrics.latencySum += latency
	metrics.latencyCount++
	metrics.mu.Unlock()
}

func findRoute(path, method string) *Route {
	routeMu.RLock()
	defer routeMu.RUnlock()

	for _, route := range routes {
		if strings.HasPrefix(path, route.Path) {
			if len(route.Methods) > 0 {
				matched := false
				for _, m := range route.Methods {
					if m == method {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			return &route
		}
	}
	return nil
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	// Check if at least one service is healthy
	serviceMu.RLock()
	healthy := false
	for _, svc := range services {
		if svc.Healthy {
			healthy = true
			break
		}
	}
	serviceMu.RUnlock()

	if healthy {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("No healthy services"))
	}
}

func handleAPIServices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		serviceMu.RLock()
		svcs := make([]Service, 0, len(services))
		for _, svc := range services {
			svcs = append(svcs, *svc)
		}
		serviceMu.RUnlock()

		sort.Slice(svcs, func(i, j int) bool {
			return svcs[i].Name < svcs[j].Name
		})

		json.NewEncoder(w).Encode(svcs)

	case "POST":
		var svc Service
		if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if svc.Name == "" || svc.URL == "" {
			http.Error(w, "Name and URL are required", http.StatusBadRequest)
			return
		}

		if svc.HealthURL == "" {
			svc.HealthURL = "/health"
		}
		if svc.Weight == 0 {
			svc.Weight = 1
		}

		serviceMu.Lock()
		services[svc.Name] = &svc
		serviceMu.Unlock()

		// Check health immediately
		go checkServiceHealth(&svc)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(svc)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPIServiceByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/services/")
	if name == "" {
		http.Error(w, "Service name required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		serviceMu.RLock()
		svc, exists := services[name]
		serviceMu.RUnlock()

		if !exists {
			http.Error(w, "Service not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(svc)

	case "DELETE":
		serviceMu.Lock()
		delete(services, name)
		serviceMu.Unlock()

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPIRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		routeMu.RLock()
		json.NewEncoder(w).Encode(routes)
		routeMu.RUnlock()

	case "POST":
		var route Route
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if route.Path == "" || route.Service == "" {
			http.Error(w, "Path and Service are required", http.StatusBadRequest)
			return
		}

		routeMu.Lock()
		routes = append(routes, route)
		// Re-sort routes
		sort.Slice(routes, func(i, j int) bool {
			return len(routes[i].Path) > len(routes[j].Path)
		})
		routeMu.Unlock()

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(route)

	case "DELETE":
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "Path query parameter required", http.StatusBadRequest)
			return
		}

		routeMu.Lock()
		for i, route := range routes {
			if route.Path == path {
				routes = append(routes[:i], routes[i+1:]...)
				break
			}
		}
		routeMu.Unlock()

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	metrics.mu.RLock()
	result := map[string]interface{}{
		"total_requests":      atomic.LoadInt64(&metrics.TotalRequests),
		"success_requests":    atomic.LoadInt64(&metrics.SuccessRequests),
		"error_requests":      atomic.LoadInt64(&metrics.ErrorRequests),
		"active_connections":  atomic.LoadInt64(&metrics.ActiveConns),
		"avg_latency_ms":      metrics.AvgLatency,
		"service_requests":    metrics.ServiceMetrics,
		"uptime_seconds":      int64(time.Since(startTime).Seconds()),
		"registered_services": len(services),
		"registered_routes":   len(routes),
	}
	metrics.mu.RUnlock()

	json.NewEncoder(w).Encode(result)
}

func handleAPIHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	serviceMu.RLock()
	healthyCount := 0
	totalCount := len(services)
	serviceHealth := make(map[string]bool)

	for name, svc := range services {
		serviceHealth[name] = svc.Healthy
		if svc.Healthy {
			healthyCount++
		}
	}
	serviceMu.RUnlock()

	status := "healthy"
	if healthyCount == 0 {
		status = "unhealthy"
	} else if healthyCount < totalCount {
		status = "degraded"
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          status,
		"healthy_count":   healthyCount,
		"total_count":     totalCount,
		"services":        serviceHealth,
		"uptime_seconds":  int64(time.Since(startTime).Seconds()),
	})
}

// Admin handlers
func handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" && r.URL.Path != "/admin/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(adminHTML))
}

func handleAdminServices(w http.ResponseWriter, r *http.Request) {
	handleAPIServices(w, r)
}

func handleAdminRoutes(w http.ResponseWriter, r *http.Request) {
	handleAPIRoutes(w, r)
}

func handleAdminMetrics(w http.ResponseWriter, r *http.Request) {
	handleAPIMetrics(w, r)
}

// Middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create response wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		// Log request (skip health checks to reduce noise)
		if r.URL.Path != "/health" && r.URL.Path != "/ready" {
			log.Printf("%s %s %d %v %s",
				r.Method,
				r.URL.Path,
				wrapped.statusCode,
				time.Since(start),
				getClientIP(r),
			)
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Utility functions
func getClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return strings.Split(forwarded, ",")[0]
	}
	real := r.Header.Get("X-Real-IP")
	if real != "" {
		return real
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	}
	return defaultVal
}

// Catppuccin Mocha Theme Admin UI
const adminHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>HolmOS Gateway - Admin</title>
    <style>
        :root {
            /* Catppuccin Mocha */
            --ctp-rosewater: #f5e0dc;
            --ctp-flamingo: #f2cdcd;
            --ctp-pink: #f5c2e7;
            --ctp-mauve: #cba6f7;
            --ctp-red: #f38ba8;
            --ctp-maroon: #eba0ac;
            --ctp-peach: #fab387;
            --ctp-yellow: #f9e2af;
            --ctp-green: #a6e3a1;
            --ctp-teal: #94e2d5;
            --ctp-sky: #89dceb;
            --ctp-sapphire: #74c7ec;
            --ctp-blue: #89b4fa;
            --ctp-lavender: #b4befe;
            --ctp-text: #cdd6f4;
            --ctp-subtext1: #bac2de;
            --ctp-subtext0: #a6adc8;
            --ctp-overlay2: #9399b2;
            --ctp-overlay1: #7f849c;
            --ctp-overlay0: #6c7086;
            --ctp-surface2: #585b70;
            --ctp-surface1: #45475a;
            --ctp-surface0: #313244;
            --ctp-base: #1e1e2e;
            --ctp-mantle: #181825;
            --ctp-crust: #11111b;
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'JetBrains Mono', 'Fira Code', monospace, -apple-system, sans-serif;
            background: var(--ctp-base);
            color: var(--ctp-text);
            min-height: 100vh;
            line-height: 1.6;
        }
        .container { max-width: 1400px; margin: 0 auto; padding: 20px; }
        .header {
            background: var(--ctp-mantle);
            border-bottom: 1px solid var(--ctp-surface0);
            padding: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .header h1 {
            font-size: 1.5rem;
            color: var(--ctp-mauve);
            display: flex;
            align-items: center;
            gap: 12px;
        }
        .header h1 svg { width: 32px; height: 32px; }
        .header .tagline {
            font-size: 0.85rem;
            color: var(--ctp-subtext0);
            font-style: italic;
        }
        .nav {
            display: flex;
            gap: 8px;
            background: var(--ctp-surface0);
            padding: 8px;
            border-radius: 8px;
            margin: 20px 0;
        }
        .nav-btn {
            padding: 10px 20px;
            border: none;
            background: transparent;
            color: var(--ctp-subtext1);
            border-radius: 6px;
            cursor: pointer;
            font-family: inherit;
            font-size: 0.9rem;
            transition: all 0.2s;
        }
        .nav-btn:hover { background: var(--ctp-surface1); color: var(--ctp-text); }
        .nav-btn.active { background: var(--ctp-mauve); color: var(--ctp-base); }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 16px;
            margin-bottom: 24px;
        }
        .stat-card {
            background: var(--ctp-surface0);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid var(--ctp-surface1);
        }
        .stat-card .label {
            font-size: 0.8rem;
            color: var(--ctp-subtext0);
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 8px;
        }
        .stat-card .value {
            font-size: 2rem;
            font-weight: 700;
        }
        .stat-card.requests .value { color: var(--ctp-blue); }
        .stat-card.success .value { color: var(--ctp-green); }
        .stat-card.errors .value { color: var(--ctp-red); }
        .stat-card.latency .value { color: var(--ctp-peach); }
        .stat-card.services .value { color: var(--ctp-mauve); }
        .stat-card.routes .value { color: var(--ctp-teal); }
        .panel {
            background: var(--ctp-surface0);
            border-radius: 12px;
            border: 1px solid var(--ctp-surface1);
            margin-bottom: 24px;
            overflow: hidden;
        }
        .panel-header {
            padding: 16px 20px;
            border-bottom: 1px solid var(--ctp-surface1);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .panel-header h2 {
            font-size: 1rem;
            color: var(--ctp-lavender);
        }
        .btn-add {
            background: var(--ctp-mauve);
            color: var(--ctp-base);
            border: none;
            padding: 8px 16px;
            border-radius: 6px;
            cursor: pointer;
            font-family: inherit;
            font-size: 0.85rem;
            display: flex;
            align-items: center;
            gap: 6px;
            transition: all 0.2s;
        }
        .btn-add:hover { background: var(--ctp-pink); }
        table {
            width: 100%;
            border-collapse: collapse;
        }
        th, td {
            padding: 14px 20px;
            text-align: left;
            border-bottom: 1px solid var(--ctp-surface1);
        }
        th {
            background: var(--ctp-mantle);
            color: var(--ctp-subtext0);
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            font-weight: 600;
        }
        tr:hover { background: var(--ctp-surface1); }
        .status-badge {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            padding: 4px 10px;
            border-radius: 12px;
            font-size: 0.75rem;
            font-weight: 600;
        }
        .status-badge.healthy {
            background: rgba(166, 227, 161, 0.2);
            color: var(--ctp-green);
        }
        .status-badge.unhealthy {
            background: rgba(243, 139, 168, 0.2);
            color: var(--ctp-red);
        }
        .status-badge .dot {
            width: 6px;
            height: 6px;
            border-radius: 50%;
            background: currentColor;
        }
        .service-url {
            color: var(--ctp-subtext0);
            font-size: 0.85rem;
            font-family: monospace;
        }
        .latency {
            color: var(--ctp-peach);
            font-size: 0.85rem;
        }
        .btn-delete {
            background: transparent;
            border: 1px solid var(--ctp-red);
            color: var(--ctp-red);
            padding: 4px 10px;
            border-radius: 4px;
            cursor: pointer;
            font-family: inherit;
            font-size: 0.8rem;
            transition: all 0.2s;
        }
        .btn-delete:hover {
            background: var(--ctp-red);
            color: var(--ctp-base);
        }
        .view { display: none; }
        .view.active { display: block; }
        .empty-state {
            padding: 40px;
            text-align: center;
            color: var(--ctp-subtext0);
        }
        .modal-overlay {
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: rgba(17, 17, 27, 0.8);
            display: none;
            align-items: center;
            justify-content: center;
            z-index: 1000;
        }
        .modal-overlay.active { display: flex; }
        .modal {
            background: var(--ctp-surface0);
            border-radius: 12px;
            width: 450px;
            max-width: 90%;
            border: 1px solid var(--ctp-surface1);
        }
        .modal-header {
            padding: 20px;
            border-bottom: 1px solid var(--ctp-surface1);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .modal-header h3 {
            color: var(--ctp-lavender);
        }
        .modal-close {
            background: transparent;
            border: none;
            color: var(--ctp-subtext0);
            cursor: pointer;
            font-size: 1.5rem;
            line-height: 1;
        }
        .modal-close:hover { color: var(--ctp-text); }
        .modal-body { padding: 20px; }
        .form-group { margin-bottom: 16px; }
        .form-group label {
            display: block;
            margin-bottom: 6px;
            color: var(--ctp-subtext1);
            font-size: 0.85rem;
        }
        .form-group input, .form-group select {
            width: 100%;
            padding: 10px 12px;
            background: var(--ctp-base);
            border: 1px solid var(--ctp-surface1);
            border-radius: 6px;
            color: var(--ctp-text);
            font-family: inherit;
            font-size: 0.9rem;
        }
        .form-group input:focus, .form-group select:focus {
            outline: none;
            border-color: var(--ctp-mauve);
        }
        .form-group input::placeholder { color: var(--ctp-overlay0); }
        .modal-footer {
            padding: 16px 20px;
            border-top: 1px solid var(--ctp-surface1);
            display: flex;
            justify-content: flex-end;
            gap: 12px;
        }
        .btn-cancel {
            background: transparent;
            border: 1px solid var(--ctp-surface2);
            color: var(--ctp-subtext1);
            padding: 8px 16px;
            border-radius: 6px;
            cursor: pointer;
            font-family: inherit;
        }
        .btn-cancel:hover { background: var(--ctp-surface1); }
        .btn-save {
            background: var(--ctp-green);
            color: var(--ctp-base);
            border: none;
            padding: 8px 16px;
            border-radius: 6px;
            cursor: pointer;
            font-family: inherit;
        }
        .btn-save:hover { background: var(--ctp-teal); }
        .route-path {
            color: var(--ctp-blue);
            font-family: monospace;
        }
        .route-service { color: var(--ctp-mauve); }
        .checkbox-group {
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .checkbox-group input[type="checkbox"] {
            width: auto;
            accent-color: var(--ctp-mauve);
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        .loading { animation: pulse 1.5s infinite; }
    </style>
</head>
<body>
    <div class="header">
        <div>
            <h1>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/>
                </svg>
                HolmOS Gateway
            </h1>
            <div class="tagline">All roads lead through me</div>
        </div>
        <div id="uptime" style="color: var(--ctp-subtext0); font-size: 0.85rem;"></div>
    </div>
    <div class="container">
        <div class="nav">
            <button class="nav-btn active" data-view="overview">Overview</button>
            <button class="nav-btn" data-view="services">Services</button>
            <button class="nav-btn" data-view="routes">Routes</button>
        </div>

        <div id="overview" class="view active">
            <div class="stats-grid">
                <div class="stat-card requests">
                    <div class="label">Total Requests</div>
                    <div class="value" id="total-requests">-</div>
                </div>
                <div class="stat-card success">
                    <div class="label">Successful</div>
                    <div class="value" id="success-requests">-</div>
                </div>
                <div class="stat-card errors">
                    <div class="label">Errors</div>
                    <div class="value" id="error-requests">-</div>
                </div>
                <div class="stat-card latency">
                    <div class="label">Avg Latency</div>
                    <div class="value" id="avg-latency">-</div>
                </div>
                <div class="stat-card services">
                    <div class="label">Services</div>
                    <div class="value" id="total-services">-</div>
                </div>
                <div class="stat-card routes">
                    <div class="label">Routes</div>
                    <div class="value" id="total-routes">-</div>
                </div>
            </div>
            <div class="panel">
                <div class="panel-header">
                    <h2>Service Health Status</h2>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>Service</th>
                            <th>Status</th>
                            <th>Latency</th>
                            <th>Last Check</th>
                        </tr>
                    </thead>
                    <tbody id="health-table">
                        <tr><td colspan="4" class="empty-state loading">Loading...</td></tr>
                    </tbody>
                </table>
            </div>
        </div>

        <div id="services" class="view">
            <div class="panel">
                <div class="panel-header">
                    <h2>Registered Services</h2>
                    <button class="btn-add" onclick="openServiceModal()">+ Add Service</button>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>Name</th>
                            <th>URL</th>
                            <th>Health Endpoint</th>
                            <th>Status</th>
                            <th>Weight</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody id="services-table">
                        <tr><td colspan="6" class="empty-state loading">Loading...</td></tr>
                    </tbody>
                </table>
            </div>
        </div>

        <div id="routes" class="view">
            <div class="panel">
                <div class="panel-header">
                    <h2>Routing Rules</h2>
                    <button class="btn-add" onclick="openRouteModal()">+ Add Route</button>
                </div>
                <table>
                    <thead>
                        <tr>
                            <th>Path</th>
                            <th>Service</th>
                            <th>Strip Prefix</th>
                            <th>Rate Limit</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody id="routes-table">
                        <tr><td colspan="5" class="empty-state loading">Loading...</td></tr>
                    </tbody>
                </table>
            </div>
        </div>
    </div>

    <!-- Service Modal -->
    <div class="modal-overlay" id="service-modal">
        <div class="modal">
            <div class="modal-header">
                <h3>Add Service</h3>
                <button class="modal-close" onclick="closeServiceModal()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>Service Name</label>
                    <input type="text" id="svc-name" placeholder="my-service">
                </div>
                <div class="form-group">
                    <label>URL</label>
                    <input type="text" id="svc-url" placeholder="http://my-service.holm.svc.cluster.local">
                </div>
                <div class="form-group">
                    <label>Health Endpoint</label>
                    <input type="text" id="svc-health" placeholder="/health" value="/health">
                </div>
                <div class="form-group">
                    <label>Weight (for load balancing)</label>
                    <input type="number" id="svc-weight" placeholder="1" value="1" min="1">
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn-cancel" onclick="closeServiceModal()">Cancel</button>
                <button class="btn-save" onclick="saveService()">Add Service</button>
            </div>
        </div>
    </div>

    <!-- Route Modal -->
    <div class="modal-overlay" id="route-modal">
        <div class="modal">
            <div class="modal-header">
                <h3>Add Route</h3>
                <button class="modal-close" onclick="closeRouteModal()">&times;</button>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>Path Pattern</label>
                    <input type="text" id="route-path" placeholder="/api/v1/">
                </div>
                <div class="form-group">
                    <label>Target Service</label>
                    <select id="route-service"></select>
                </div>
                <div class="form-group checkbox-group">
                    <input type="checkbox" id="route-strip" checked>
                    <label for="route-strip">Strip prefix from forwarded request</label>
                </div>
                <div class="form-group">
                    <label>Rate Limit (requests/min, 0 = unlimited)</label>
                    <input type="number" id="route-limit" placeholder="0" value="0" min="0">
                </div>
            </div>
            <div class="modal-footer">
                <button class="btn-cancel" onclick="closeRouteModal()">Cancel</button>
                <button class="btn-save" onclick="saveRoute()">Add Route</button>
            </div>
        </div>
    </div>

    <script>
        let services = [];
        let routes = [];
        let metrics = {};

        async function fetchMetrics() {
            try {
                const r = await fetch('/api/metrics');
                metrics = await r.json();
                updateMetricsUI();
            } catch (e) {
                console.error('Failed to fetch metrics:', e);
            }
        }

        async function fetchServices() {
            try {
                const r = await fetch('/api/services');
                services = await r.json();
                updateServicesUI();
                updateServiceSelector();
            } catch (e) {
                console.error('Failed to fetch services:', e);
            }
        }

        async function fetchRoutes() {
            try {
                const r = await fetch('/api/routes');
                routes = await r.json();
                updateRoutesUI();
            } catch (e) {
                console.error('Failed to fetch routes:', e);
            }
        }

        function updateMetricsUI() {
            document.getElementById('total-requests').textContent = metrics.total_requests?.toLocaleString() || '0';
            document.getElementById('success-requests').textContent = metrics.success_requests?.toLocaleString() || '0';
            document.getElementById('error-requests').textContent = metrics.error_requests?.toLocaleString() || '0';
            document.getElementById('avg-latency').textContent = (metrics.avg_latency_ms?.toFixed(1) || '0') + 'ms';
            document.getElementById('total-services').textContent = metrics.registered_services || '0';
            document.getElementById('total-routes').textContent = metrics.registered_routes || '0';

            const uptime = metrics.uptime_seconds || 0;
            const hours = Math.floor(uptime / 3600);
            const mins = Math.floor((uptime % 3600) / 60);
            document.getElementById('uptime').textContent = 'Uptime: ' + hours + 'h ' + mins + 'm';
        }

        function updateServicesUI() {
            const tbody = document.getElementById('services-table');
            const healthTbody = document.getElementById('health-table');

            if (!services || services.length === 0) {
                tbody.innerHTML = '<tr><td colspan="6" class="empty-state">No services registered</td></tr>';
                healthTbody.innerHTML = '<tr><td colspan="4" class="empty-state">No services registered</td></tr>';
                return;
            }

            tbody.innerHTML = services.map(s => '<tr>' +
                '<td><strong>' + s.name + '</strong></td>' +
                '<td class="service-url">' + s.url + '</td>' +
                '<td>' + s.health_url + '</td>' +
                '<td><span class="status-badge ' + (s.healthy ? 'healthy' : 'unhealthy') + '">' +
                    '<span class="dot"></span>' + (s.healthy ? 'Healthy' : 'Unhealthy') + '</span></td>' +
                '<td>' + s.weight + '</td>' +
                '<td><button class="btn-delete" onclick="deleteService(\'' + s.name + '\')">Delete</button></td>' +
            '</tr>').join('');

            healthTbody.innerHTML = services.map(s => '<tr>' +
                '<td><strong>' + s.name + '</strong></td>' +
                '<td><span class="status-badge ' + (s.healthy ? 'healthy' : 'unhealthy') + '">' +
                    '<span class="dot"></span>' + (s.healthy ? 'Healthy' : 'Unhealthy') + '</span></td>' +
                '<td class="latency">' + (s.latency_ms || 0) + 'ms</td>' +
                '<td>' + (s.last_check ? new Date(s.last_check).toLocaleTimeString() : 'Never') + '</td>' +
            '</tr>').join('');
        }

        function updateRoutesUI() {
            const tbody = document.getElementById('routes-table');

            if (!routes || routes.length === 0) {
                tbody.innerHTML = '<tr><td colspan="5" class="empty-state">No routes configured</td></tr>';
                return;
            }

            tbody.innerHTML = routes.map(r => '<tr>' +
                '<td class="route-path">' + r.path + '</td>' +
                '<td class="route-service">' + r.service + '</td>' +
                '<td>' + (r.strip_prefix ? 'Yes' : 'No') + '</td>' +
                '<td>' + (r.rate_limit || 'Unlimited') + '</td>' +
                '<td><button class="btn-delete" onclick="deleteRoute(\'' + r.path + '\')">Delete</button></td>' +
            '</tr>').join('');
        }

        function updateServiceSelector() {
            const select = document.getElementById('route-service');
            select.innerHTML = services.map(s =>
                '<option value="' + s.name + '">' + s.name + '</option>'
            ).join('');
        }

        // Modal functions
        function openServiceModal() {
            document.getElementById('service-modal').classList.add('active');
        }

        function closeServiceModal() {
            document.getElementById('service-modal').classList.remove('active');
            document.getElementById('svc-name').value = '';
            document.getElementById('svc-url').value = '';
            document.getElementById('svc-health').value = '/health';
            document.getElementById('svc-weight').value = '1';
        }

        function openRouteModal() {
            document.getElementById('route-modal').classList.add('active');
        }

        function closeRouteModal() {
            document.getElementById('route-modal').classList.remove('active');
            document.getElementById('route-path').value = '';
            document.getElementById('route-strip').checked = true;
            document.getElementById('route-limit').value = '0';
        }

        async function saveService() {
            const svc = {
                name: document.getElementById('svc-name').value,
                url: document.getElementById('svc-url').value,
                health_url: document.getElementById('svc-health').value,
                weight: parseInt(document.getElementById('svc-weight').value) || 1
            };

            if (!svc.name || !svc.url) {
                alert('Name and URL are required');
                return;
            }

            try {
                await fetch('/api/services', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(svc)
                });
                closeServiceModal();
                fetchServices();
            } catch (e) {
                alert('Failed to add service: ' + e);
            }
        }

        async function deleteService(name) {
            if (!confirm('Delete service ' + name + '?')) return;

            try {
                await fetch('/api/services/' + name, { method: 'DELETE' });
                fetchServices();
            } catch (e) {
                alert('Failed to delete service: ' + e);
            }
        }

        async function saveRoute() {
            const route = {
                path: document.getElementById('route-path').value,
                service: document.getElementById('route-service').value,
                strip_prefix: document.getElementById('route-strip').checked,
                rate_limit: parseInt(document.getElementById('route-limit').value) || 0
            };

            if (!route.path || !route.service) {
                alert('Path and Service are required');
                return;
            }

            try {
                await fetch('/api/routes', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(route)
                });
                closeRouteModal();
                fetchRoutes();
            } catch (e) {
                alert('Failed to add route: ' + e);
            }
        }

        async function deleteRoute(path) {
            if (!confirm('Delete route ' + path + '?')) return;

            try {
                await fetch('/api/routes?path=' + encodeURIComponent(path), { method: 'DELETE' });
                fetchRoutes();
            } catch (e) {
                alert('Failed to delete route: ' + e);
            }
        }

        // Navigation
        document.querySelectorAll('.nav-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');

                document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
                document.getElementById(btn.dataset.view).classList.add('active');
            });
        });

        // Initialize
        function init() {
            fetchMetrics();
            fetchServices();
            fetchRoutes();

            setInterval(fetchMetrics, 5000);
            setInterval(fetchServices, 10000);
        }

        init();
    </script>
</body>
</html>`
