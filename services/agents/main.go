package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// HolmOS Agents Service
// Central service for managing and querying AI agents in the HolmOS ecosystem

const (
	serviceName    = "agents"
	serviceVersion = "1.0.0"
	serviceTagline = "Orchestrating intelligence across the realm"
)

// Agent represents an AI agent in the HolmOS ecosystem
type Agent struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Endpoint    string            `json:"endpoint"`
	HealthURL   string            `json:"health_url"`
	Healthy     bool              `json:"healthy"`
	LastCheck   time.Time         `json:"last_check"`
	Latency     int64             `json:"latency_ms"`
	Personality map[string]string `json:"personality,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

// AgentCapability represents a capability offered by an agent
type AgentCapability struct {
	AgentName   string   `json:"agent_name"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}

// AgentStore manages registered agents
type AgentStore struct {
	mu     sync.RWMutex
	agents map[string]*Agent
}

// ChatMessage represents a message for agent routing
type ChatMessage struct {
	Message   string            `json:"message"`
	Context   map[string]string `json:"context,omitempty"`
	AgentHint string            `json:"agent_hint,omitempty"`
}

// RoutingResult represents the result of routing a message to an agent
type RoutingResult struct {
	SelectedAgent string   `json:"selected_agent"`
	Confidence    float64  `json:"confidence"`
	Reasoning     string   `json:"reasoning"`
	Alternatives  []string `json:"alternatives,omitempty"`
}

var (
	store     *AgentStore
	startTime = time.Now()
)

// NewAgentStore creates a new agent store with default agents
func NewAgentStore() *AgentStore {
	s := &AgentStore{
		agents: make(map[string]*Agent),
	}
	s.initDefaultAgents()
	return s
}

// initDefaultAgents registers the default HolmOS agents
func (s *AgentStore) initDefaultAgents() {
	defaultAgents := []*Agent{
		{
			Name:        "atlas",
			Description: "File management AI agent. Handles file operations, search, compression, and organization.",
			Endpoint:    "http://atlas.holm.svc.cluster.local",
			HealthURL:   "/health",
			Personality: map[string]string{
				"name":       "Atlas",
				"domain":     "File system operations",
				"catchphrase": "Everything in its place.",
			},
			Tags: []string{"files", "storage", "organization"},
		},
		{
			Name:        "scribe",
			Description: "Log aggregation and analysis agent. Records and searches through system logs.",
			Endpoint:    "http://scribe.holm.svc.cluster.local",
			HealthURL:   "/health",
			Personality: map[string]string{
				"name":       "Scribe",
				"domain":     "Log management",
				"catchphrase": "It's all in the records.",
			},
			Tags: []string{"logs", "monitoring", "search"},
		},
		{
			Name:        "gateway",
			Description: "API gateway and routing agent. Routes requests and manages service discovery.",
			Endpoint:    "http://gateway.holm.svc.cluster.local",
			HealthURL:   "/health",
			Personality: map[string]string{
				"name":       "Gateway",
				"domain":     "Request routing",
				"catchphrase": "All roads lead through me.",
			},
			Tags: []string{"routing", "api", "services"},
		},
		{
			Name:        "nova",
			Description: "Deployment and CI/CD agent. Manages application deployments and pipelines.",
			Endpoint:    "http://nova.holm.svc.cluster.local",
			HealthURL:   "/health",
			Personality: map[string]string{
				"name":       "Nova",
				"domain":     "Deployments",
				"catchphrase": "Ship it.",
			},
			Tags: []string{"deploy", "cicd", "kubernetes"},
		},
		{
			Name:        "vault",
			Description: "Secrets and configuration management agent. Securely stores and retrieves sensitive data.",
			Endpoint:    "http://vault.holm.svc.cluster.local",
			HealthURL:   "/health",
			Personality: map[string]string{
				"name":       "Vault",
				"domain":     "Secrets management",
				"catchphrase": "Your secrets are safe with me.",
			},
			Tags: []string{"secrets", "security", "config"},
		},
		{
			Name:        "merchant",
			Description: "E-commerce and transaction agent. Handles shopping, inventory, and payments.",
			Endpoint:    "http://merchant.holm.svc.cluster.local",
			HealthURL:   "/health",
			Personality: map[string]string{
				"name":       "Merchant",
				"domain":     "E-commerce",
				"catchphrase": "The best deals in the realm.",
			},
			Tags: []string{"commerce", "inventory", "payments"},
		},
	}

	for _, agent := range defaultAgents {
		s.agents[agent.Name] = agent
	}
}

// ListAgents returns all registered agents
func ListAgents() []*Agent {
	store.mu.RLock()
	defer store.mu.RUnlock()

	agents := make([]*Agent, 0, len(store.agents))
	for _, agent := range store.agents {
		agents = append(agents, agent)
	}

	// Sort by name for consistent ordering
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	return agents
}

// GetAgent retrieves a specific agent by name
func GetAgent(name string) (*Agent, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()

	agent, exists := store.agents[name]
	return agent, exists
}

// RegisterAgent adds a new agent to the registry
func RegisterAgent(agent *Agent) error {
	if agent.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if agent.Endpoint == "" {
		return fmt.Errorf("agent endpoint is required")
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if agent.HealthURL == "" {
		agent.HealthURL = "/health"
	}

	store.agents[agent.Name] = agent
	return nil
}

// UnregisterAgent removes an agent from the registry
func UnregisterAgent(name string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	if _, exists := store.agents[name]; exists {
		delete(store.agents, name)
		return true
	}
	return false
}

// GetCapabilities returns all capabilities across all agents
func GetCapabilities() []AgentCapability {
	store.mu.RLock()
	defer store.mu.RUnlock()

	capabilities := []AgentCapability{
		// Atlas capabilities
		{AgentName: "atlas", Name: "list", Description: "List files in a directory", Examples: []string{"Show me recent downloads", "List files in documents"}},
		{AgentName: "atlas", Name: "search", Description: "Search for files by name or type", Examples: []string{"Find all PDFs", "Search for report"}},
		{AgentName: "atlas", Name: "compress", Description: "Create zip/tar archives", Examples: []string{"Compress the downloads folder", "Zip the documents"}},
		{AgentName: "atlas", Name: "decompress", Description: "Extract archives", Examples: []string{"Unzip archive.zip", "Extract backup.tar.gz"}},

		// Scribe capabilities
		{AgentName: "scribe", Name: "logs", Description: "View and search logs", Examples: []string{"Show me error logs", "Search logs for deploy"}},
		{AgentName: "scribe", Name: "tail", Description: "Stream live logs", Examples: []string{"Tail the gateway logs", "Watch for errors"}},
		{AgentName: "scribe", Name: "export", Description: "Export logs to file", Examples: []string{"Export last hour's logs", "Download error logs"}},

		// Gateway capabilities
		{AgentName: "gateway", Name: "routes", Description: "Manage API routes", Examples: []string{"List all routes", "Add a route to my-service"}},
		{AgentName: "gateway", Name: "services", Description: "View registered services", Examples: []string{"Show healthy services", "Check service status"}},

		// Nova capabilities
		{AgentName: "nova", Name: "deploy", Description: "Deploy applications", Examples: []string{"Deploy my-app", "Rollback to previous version"}},
		{AgentName: "nova", Name: "status", Description: "Check deployment status", Examples: []string{"What's the status of my deployment?", "Show running pods"}},

		// Vault capabilities
		{AgentName: "vault", Name: "secrets", Description: "Manage secrets", Examples: []string{"List my secrets", "Get database password"}},
		{AgentName: "vault", Name: "config", Description: "Manage configuration", Examples: []string{"Show app config", "Update redis host"}},

		// Merchant capabilities
		{AgentName: "merchant", Name: "inventory", Description: "Check inventory", Examples: []string{"How many widgets in stock?", "List low stock items"}},
		{AgentName: "merchant", Name: "orders", Description: "Manage orders", Examples: []string{"Show recent orders", "Order status for #12345"}},
	}

	return capabilities
}

// RouteMessage determines which agent should handle a message
func RouteMessage(msg ChatMessage) RoutingResult {
	// Simple keyword-based routing for demonstration
	// In production, this could use ML/NLP for better routing

	keywords := map[string][]string{
		"atlas":    {"file", "folder", "directory", "compress", "zip", "unzip", "extract", "move", "copy", "delete", "search file", "find file"},
		"scribe":   {"log", "logs", "error", "warning", "debug", "tail", "stream", "chronicle", "record"},
		"gateway":  {"route", "routing", "service", "api", "proxy", "endpoint", "health"},
		"nova":     {"deploy", "deployment", "rollback", "pod", "kubernetes", "k8s", "release", "build"},
		"vault":    {"secret", "password", "credential", "config", "configuration", "env", "environment"},
		"merchant": {"order", "inventory", "stock", "purchase", "payment", "product", "cart"},
	}

	msgLower := msg.Message

	// Check for explicit agent hint
	if msg.AgentHint != "" {
		if _, exists := store.agents[msg.AgentHint]; exists {
			return RoutingResult{
				SelectedAgent: msg.AgentHint,
				Confidence:    1.0,
				Reasoning:     "Explicit agent hint provided",
			}
		}
	}

	// Score each agent based on keyword matches
	scores := make(map[string]int)
	for agent, kws := range keywords {
		for _, kw := range kws {
			if contains(msgLower, kw) {
				scores[agent]++
			}
		}
	}

	// Find the agent with the highest score
	var bestAgent string
	var bestScore int
	var alternatives []string

	for agent, score := range scores {
		if score > bestScore {
			if bestAgent != "" {
				alternatives = append(alternatives, bestAgent)
			}
			bestAgent = agent
			bestScore = score
		} else if score > 0 {
			alternatives = append(alternatives, agent)
		}
	}

	if bestAgent == "" {
		// Default to gateway as the general-purpose entry point
		return RoutingResult{
			SelectedAgent: "gateway",
			Confidence:    0.3,
			Reasoning:     "No specific keywords matched, defaulting to gateway",
			Alternatives:  []string{"atlas", "scribe"},
		}
	}

	confidence := float64(bestScore) / 5.0 // Max 5 keyword matches for 100% confidence
	if confidence > 1.0 {
		confidence = 1.0
	}

	return RoutingResult{
		SelectedAgent: bestAgent,
		Confidence:    confidence,
		Reasoning:     fmt.Sprintf("Matched %d keywords for %s", bestScore, bestAgent),
		Alternatives:  alternatives,
	}
}

// HealthCheck performs health checks on all agents
func HealthCheck() map[string]bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	results := make(map[string]bool)
	var wg sync.WaitGroup

	for name, agent := range store.agents {
		wg.Add(1)
		go func(n string, a *Agent) {
			defer wg.Done()

			client := &http.Client{Timeout: 5 * time.Second}
			start := time.Now()

			resp, err := client.Get(a.Endpoint + a.HealthURL)
			latency := time.Since(start).Milliseconds()

			store.mu.Lock()
			a.LastCheck = time.Now()
			a.Latency = latency
			if err != nil {
				a.Healthy = false
			} else {
				a.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 400
				resp.Body.Close()
			}
			results[n] = a.Healthy
			store.mu.Unlock()
		}(name, agent)
	}

	wg.Wait()
	return results
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if matchLower(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func matchLower(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// HTTP Handlers

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": serviceName,
		"tagline": serviceTagline,
	})
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service":     serviceName,
		"version":     serviceVersion,
		"tagline":     serviceTagline,
		"description": "Central service for managing AI agents in HolmOS",
		"endpoints": []string{
			"/health",
			"/api/agents",
			"/api/agents/{name}",
			"/api/capabilities",
			"/api/route",
			"/api/health-check",
			"/api/status",
		},
		"uptime_seconds": int64(time.Since(startTime).Seconds()),
	})
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("HolmOS Agents Service v%s starting - %s", serviceVersion, serviceTagline)

	// Initialize the store
	store = NewAgentStore()

	// Start periodic health checks
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Initial health check
		HealthCheck()

		for range ticker.C {
			HealthCheck()
		}
	}()

	// Setup HTTP routes
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)

	// Register API routes
	RegisterAPIRoutes()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Agents service listening on port %s", port)
	log.Printf("Registered %d default agents", len(store.agents))

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
