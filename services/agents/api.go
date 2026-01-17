package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// API Response types

// AgentsListResponse represents the response for listing all agents
type AgentsListResponse struct {
	Count   int      `json:"count"`
	Agents  []*Agent `json:"agents"`
	Message string   `json:"message,omitempty"`
}

// AgentResponse represents a single agent response
type AgentResponse struct {
	Agent   *Agent `json:"agent"`
	Message string `json:"message,omitempty"`
}

// CapabilitiesResponse represents the response for capabilities listing
type CapabilitiesResponse struct {
	Count        int               `json:"count"`
	Capabilities []AgentCapability `json:"capabilities"`
	ByAgent      map[string]int    `json:"by_agent"`
	Message      string            `json:"message,omitempty"`
}

// RouteResponse represents the response for message routing
type RouteResponse struct {
	Routing RoutingResult `json:"routing"`
	Message string        `json:"message,omitempty"`
}

// HealthCheckResponse represents the response for health checks
type HealthCheckResponse struct {
	Timestamp    time.Time       `json:"timestamp"`
	TotalAgents  int             `json:"total_agents"`
	HealthyCount int             `json:"healthy_count"`
	Status       string          `json:"status"`
	Results      map[string]bool `json:"results"`
	Message      string          `json:"message,omitempty"`
}

// StatusResponse represents the overall service status
type StatusResponse struct {
	Service       string          `json:"service"`
	Version       string          `json:"version"`
	Tagline       string          `json:"tagline"`
	Uptime        int64           `json:"uptime_seconds"`
	TotalAgents   int             `json:"total_agents"`
	HealthyAgents int             `json:"healthy_agents"`
	AgentStatus   map[string]bool `json:"agent_status"`
	Message       string          `json:"message,omitempty"`
}

// handleAPIAgents handles GET /api/agents and POST /api/agents
func handleAPIAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		// GET /api/agents - List all agents
		agents := ListAgents()

		response := AgentsListResponse{
			Count:   len(agents),
			Agents:  agents,
			Message: "Successfully retrieved all registered agents",
		}

		if len(agents) == 0 {
			response.Message = "No agents currently registered"
		}

		json.NewEncoder(w).Encode(response)

	case http.MethodPost:
		// POST /api/agents - Register a new agent
		var agent Agent
		if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
			http.Error(w, `{"error": "Invalid JSON body: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		if err := RegisterAgent(&agent); err != nil {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(AgentResponse{
			Agent:   &agent,
			Message: "Agent successfully registered",
		})

	default:
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleAPIAgentByName handles GET /api/agents/{name} and DELETE /api/agents/{name}
func handleAPIAgentByName(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract agent name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	name := strings.TrimSuffix(path, "/")

	if name == "" {
		http.Error(w, `{"error": "Agent name is required"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// GET /api/agents/{name} - Get a specific agent
		agent, exists := GetAgent(name)
		if !exists {
			http.Error(w, `{"error": "Agent not found: `+name+`"}`, http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(AgentResponse{
			Agent:   agent,
			Message: "Agent retrieved successfully",
		})

	case http.MethodDelete:
		// DELETE /api/agents/{name} - Unregister an agent
		if removed := UnregisterAgent(name); !removed {
			http.Error(w, `{"error": "Agent not found: `+name+`"}`, http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"status":  "deleted",
			"agent":   name,
			"message": "Agent successfully unregistered",
		})

	default:
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleAPICapabilities handles GET /api/capabilities
func handleAPICapabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	capabilities := GetCapabilities()

	// Optional filter by agent
	agentFilter := r.URL.Query().Get("agent")
	if agentFilter != "" {
		filtered := make([]AgentCapability, 0)
		for _, cap := range capabilities {
			if cap.AgentName == agentFilter {
				filtered = append(filtered, cap)
			}
		}
		capabilities = filtered
	}

	// Count capabilities by agent
	byAgent := make(map[string]int)
	for _, cap := range capabilities {
		byAgent[cap.AgentName]++
	}

	response := CapabilitiesResponse{
		Count:        len(capabilities),
		Capabilities: capabilities,
		ByAgent:      byAgent,
		Message:      "Successfully retrieved agent capabilities",
	}

	if agentFilter != "" {
		response.Message = "Filtered capabilities for agent: " + agentFilter
	}

	json.NewEncoder(w).Encode(response)
}

// handleAPIRoute handles POST /api/route
func handleAPIRoute(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "Method not allowed. Use POST."}`, http.StatusMethodNotAllowed)
		return
	}

	var msg ChatMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, `{"error": "Invalid JSON body: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	if msg.Message == "" {
		http.Error(w, `{"error": "Message is required"}`, http.StatusBadRequest)
		return
	}

	routing := RouteMessage(msg)

	response := RouteResponse{
		Routing: routing,
		Message: "Message routed to " + routing.SelectedAgent,
	}

	json.NewEncoder(w).Encode(response)
}

// handleAPIHealthCheck handles GET /api/health-check
func handleAPIHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	results := HealthCheck()

	healthyCount := 0
	for _, healthy := range results {
		if healthy {
			healthyCount++
		}
	}

	status := "healthy"
	if healthyCount == 0 {
		status = "unhealthy"
	} else if healthyCount < len(results) {
		status = "degraded"
	}

	message := "All agents are healthy"
	if status == "unhealthy" {
		message = "No agents are responding"
	} else if status == "degraded" {
		message = "Some agents are unhealthy"
	}

	response := HealthCheckResponse{
		Timestamp:    time.Now(),
		TotalAgents:  len(results),
		HealthyCount: healthyCount,
		Status:       status,
		Results:      results,
		Message:      message,
	}

	json.NewEncoder(w).Encode(response)
}

// handleAPIStatus handles GET /api/status
func handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	agents := ListAgents()
	agentStatus := make(map[string]bool)
	healthyCount := 0

	for _, agent := range agents {
		agentStatus[agent.Name] = agent.Healthy
		if agent.Healthy {
			healthyCount++
		}
	}

	response := StatusResponse{
		Service:       serviceName,
		Version:       serviceVersion,
		Tagline:       serviceTagline,
		Uptime:        int64(time.Since(startTime).Seconds()),
		TotalAgents:   len(agents),
		HealthyAgents: healthyCount,
		AgentStatus:   agentStatus,
		Message:       serviceTagline,
	}

	json.NewEncoder(w).Encode(response)
}

// RegisterAPIRoutes registers all API HTTP handlers
// This function is called from main() to register the endpoints
func RegisterAPIRoutes() {
	// Agent management endpoints
	http.HandleFunc("/api/agents", handleAPIAgents)
	http.HandleFunc("/api/agents/", handleAPIAgentByName)

	// Capabilities endpoint
	http.HandleFunc("/api/capabilities", handleAPICapabilities)

	// Message routing endpoint
	http.HandleFunc("/api/route", handleAPIRoute)

	// Health check endpoint (checks all agents)
	http.HandleFunc("/api/health-check", handleAPIHealthCheck)

	// Status endpoint
	http.HandleFunc("/api/status", handleAPIStatus)
}
