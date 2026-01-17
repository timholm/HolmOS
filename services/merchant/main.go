package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	merchantTagline = "Describe what you need, I'll make it happen"
)

// Template represents a service template
type Template struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Files       []string `json:"files"`
}

// BuildRequest represents a request to build a service
type BuildRequest struct {
	Template    string            `json:"template"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Options     map[string]string `json:"options"`
}

// BuildResponse represents the result of a build request
type BuildResponse struct {
	Success   bool      `json:"success"`
	BuildID   string    `json:"build_id"`
	Message   string    `json:"message"`
	Template  string    `json:"template"`
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp"`
}

// ChatRequest represents a chat message from the user
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse represents the agent's response
type ChatResponse struct {
	Response string `json:"response"`
	Agent    string `json:"agent"`
}

// CatalogResponse represents the catalog of available templates
type CatalogResponse struct {
	Templates []Template `json:"templates"`
	Count     int        `json:"count"`
	Agent     string     `json:"agent"`
	Tagline   string     `json:"tagline"`
}

var (
	templates   []Template
	templatesMu sync.RWMutex
	buildCount  int
	buildMu     sync.Mutex
)

func init() {
	// Initialize default templates
	templates = []Template{
		{
			ID:          "flask-basic",
			Name:        "Flask Basic",
			Description: "Basic Flask web application with health endpoint",
			Language:    "python",
			Category:    "web",
			Tags:        []string{"flask", "python", "web", "api"},
			Files:       []string{"app.py", "requirements.txt", "Dockerfile", "deployment.yaml"},
		},
		{
			ID:          "flask-api",
			Name:        "Flask REST API",
			Description: "Flask REST API with CORS and JSON endpoints",
			Language:    "python",
			Category:    "api",
			Tags:        []string{"flask", "python", "rest", "api", "cors"},
			Files:       []string{"app.py", "requirements.txt", "Dockerfile", "deployment.yaml"},
		},
		{
			ID:          "go-service",
			Name:        "Go Service",
			Description: "Go HTTP service with standard net/http patterns",
			Language:    "go",
			Category:    "service",
			Tags:        []string{"go", "golang", "http", "service"},
			Files:       []string{"main.go", "go.mod", "Dockerfile", "deployment.yaml"},
		},
		{
			ID:          "go-agent",
			Name:        "Go Agent",
			Description: "Go agent with chat interface and personality",
			Language:    "go",
			Category:    "agent",
			Tags:        []string{"go", "golang", "agent", "chat", "ai"},
			Files:       []string{"main.go", "go.mod", "Dockerfile", "deployment.yaml"},
		},
		{
			ID:          "static-web",
			Name:        "Static Web",
			Description: "Static website with Nginx serving",
			Language:    "html",
			Category:    "web",
			Tags:        []string{"html", "css", "javascript", "nginx", "static"},
			Files:       []string{"index.html", "style.css", "Dockerfile", "deployment.yaml"},
		},
		{
			ID:          "node-express",
			Name:        "Node Express",
			Description: "Node.js Express API server",
			Language:    "javascript",
			Category:    "api",
			Tags:        []string{"node", "javascript", "express", "api"},
			Files:       []string{"server.js", "package.json", "Dockerfile", "deployment.yaml"},
		},
	}
}

func main() {
	log.Printf("Merchant Agent starting - %s", merchantTagline)

	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", handleHealth)

	// Main UI
	mux.HandleFunc("/", handleIndex)

	// API endpoints
	mux.HandleFunc("/catalog", handleCatalog)
	mux.HandleFunc("/templates", handleTemplates)
	mux.HandleFunc("/chat", handleChat)
	mux.HandleFunc("/build", handleBuild)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Merchant listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

// corsMiddleware adds CORS headers to responses
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleHealth returns service health status
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "merchant",
		"agent":   "Merchant",
		"tagline": merchantTagline,
	})
}

// handleIndex serves the main UI
func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(indexHTML))
}

// handleCatalog returns the catalog of available templates
func handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	templatesMu.RLock()
	defer templatesMu.RUnlock()

	response := CatalogResponse{
		Templates: templates,
		Count:     len(templates),
		Agent:     "Merchant",
		Tagline:   merchantTagline,
	}

	json.NewEncoder(w).Encode(response)
}

// handleTemplates returns the list of available templates
func handleTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	templatesMu.RLock()
	defer templatesMu.RUnlock()

	// Filter by category or language if query params provided
	category := r.URL.Query().Get("category")
	language := r.URL.Query().Get("language")

	var filtered []Template
	for _, t := range templates {
		if category != "" && t.Category != category {
			continue
		}
		if language != "" && t.Language != language {
			continue
		}
		filtered = append(filtered, t)
	}

	if filtered == nil {
		filtered = templates
	}

	json.NewEncoder(w).Encode(filtered)
}

// handleChat handles chat messages and returns agent responses
func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	response := generateChatResponse(req.Message)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ChatResponse{
		Response: response,
		Agent:    "Merchant",
	})
}

// handleBuild processes build requests
func handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate template exists
	templatesMu.RLock()
	var foundTemplate *Template
	for _, t := range templates {
		if t.ID == req.Template {
			foundTemplate = &t
			break
		}
	}
	templatesMu.RUnlock()

	if foundTemplate == nil && req.Template != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BuildResponse{
			Success: false,
			Message: "Template not found: " + req.Template,
		})
		return
	}

	// Generate build ID
	buildMu.Lock()
	buildCount++
	buildID := time.Now().Format("20060102-150405") + "-" + string(rune('A'+buildCount%26))
	buildMu.Unlock()

	// Simulate build processing
	message := "Build queued successfully"
	if req.Name != "" {
		message = "Build for '" + req.Name + "' queued successfully"
	}
	if foundTemplate != nil {
		message += " using " + foundTemplate.Name + " template"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BuildResponse{
		Success:   true,
		BuildID:   buildID,
		Message:   message,
		Template:  req.Template,
		Name:      req.Name,
		Timestamp: time.Now(),
	})
}

// generateChatResponse generates a contextual response based on user message
func generateChatResponse(message string) string {
	msgLower := strings.ToLower(message)

	// Template-related queries
	if strings.Contains(msgLower, "template") || strings.Contains(msgLower, "available") {
		templatesMu.RLock()
		count := len(templates)
		templatesMu.RUnlock()
		return "I have " + string(rune('0'+count)) + " templates ready for you. We have Flask, Go, Node.js, and static web options. Describe what you need, and I'll make it happen!"
	}

	// Build-related queries
	if strings.Contains(msgLower, "build") || strings.Contains(msgLower, "create") || strings.Contains(msgLower, "make") {
		return "I can help you build a new service! Tell me what kind of application you need - a web app, API, agent, or something else. I'll set up everything including the Dockerfile and Kubernetes deployment."
	}

	// Flask/Python queries
	if strings.Contains(msgLower, "flask") || strings.Contains(msgLower, "python") {
		return "Flask is a great choice! I have two Flask templates: a basic web app and a REST API with CORS support. Both come with health endpoints, Dockerfiles, and Kubernetes configs. Which would you like?"
	}

	// Go queries
	if strings.Contains(msgLower, "go") || strings.Contains(msgLower, "golang") {
		return "Go services are my specialty! I have a standard HTTP service template and an agent template with chat capabilities. Both follow HolmOS patterns with proper health endpoints. Ready to build?"
	}

	// Node queries
	if strings.Contains(msgLower, "node") || strings.Contains(msgLower, "express") || strings.Contains(msgLower, "javascript") {
		return "Node.js with Express is perfect for quick APIs! The template includes standard middleware, CORS, and the usual HolmOS deployment setup. Shall I create one for you?"
	}

	// Deploy queries
	if strings.Contains(msgLower, "deploy") || strings.Contains(msgLower, "kubernetes") || strings.Contains(msgLower, "k8s") {
		return "Every template I provide comes with a complete deployment.yaml for Kubernetes. It includes the deployment, service, and proper labels for the HolmOS cluster. The service will be ready to deploy with kubectl apply."
	}

	// Help queries
	if strings.Contains(msgLower, "help") || strings.Contains(msgLower, "what can") {
		return "I'm Merchant, your service builder! I can create new HolmOS services from templates. Just describe what you need - a web app, API, agent, or static site - and I'll generate all the files: code, Dockerfile, and Kubernetes deployment. Describe what you need, I'll make it happen!"
	}

	// Greeting
	if strings.Contains(msgLower, "hello") || strings.Contains(msgLower, "hi") || msgLower == "hey" {
		return "Hello! I'm Merchant, your service builder. Describe what you need, and I'll make it happen! Want to see the available templates?"
	}

	// Default response
	return "I'm here to help you build services. Tell me what kind of application you need - web, API, agent, or static site - and I'll set everything up for you. Describe what you need, I'll make it happen!"
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Merchant - Service Builder | HolmOS</title>
    <style>
        :root {
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
            font-family: 'JetBrains Mono', 'Fira Code', monospace;
            background: var(--ctp-base);
            color: var(--ctp-text);
            min-height: 100vh;
        }

        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }

        header {
            background: var(--ctp-mantle);
            border-bottom: 2px solid var(--ctp-surface0);
            padding: 20px 0;
            margin-bottom: 30px;
        }

        .header-content {
            display: flex;
            align-items: center;
            justify-content: space-between;
            max-width: 1200px;
            margin: 0 auto;
            padding: 0 20px;
        }

        .logo { display: flex; align-items: center; gap: 15px; }
        .logo-icon { font-size: 2.5rem; }
        .logo h1 { font-size: 2rem; color: var(--ctp-peach); }
        .tagline { color: var(--ctp-subtext0); font-style: italic; }

        .agent-section {
            background: var(--ctp-mantle);
            padding: 20px;
            border-radius: 12px;
            margin-bottom: 20px;
            border: 1px solid var(--ctp-surface0);
        }

        .agent-header { display: flex; align-items: center; gap: 15px; margin-bottom: 15px; }
        .agent-avatar { font-size: 3rem; }
        .agent-info h2 { color: var(--ctp-peach); }
        .agent-info p { color: var(--ctp-subtext0); font-style: italic; }

        .chat-messages {
            background: var(--ctp-base);
            border-radius: 8px;
            padding: 20px;
            max-height: 200px;
            overflow-y: auto;
            margin-bottom: 15px;
        }

        .chat-message {
            padding: 10px 15px;
            margin-bottom: 10px;
            border-radius: 8px;
            line-height: 1.6;
        }

        .chat-message.agent {
            background: var(--ctp-surface0);
            border-left: 3px solid var(--ctp-peach);
        }

        .chat-message.user {
            background: var(--ctp-surface1);
            border-left: 3px solid var(--ctp-sapphire);
        }

        .chat-input-row { display: flex; gap: 10px; }

        .chat-input {
            flex: 1;
            padding: 12px 16px;
            background: var(--ctp-surface0);
            border: 1px solid var(--ctp-surface1);
            border-radius: 8px;
            color: var(--ctp-text);
            font-family: inherit;
            font-size: 1rem;
        }

        .chat-input:focus {
            outline: none;
            border-color: var(--ctp-peach);
        }

        .btn {
            padding: 12px 24px;
            background: var(--ctp-peach);
            color: var(--ctp-crust);
            border: none;
            border-radius: 8px;
            font-weight: bold;
            cursor: pointer;
            transition: all 0.2s;
            font-family: inherit;
        }

        .btn:hover { background: var(--ctp-yellow); }
        .btn-secondary { background: var(--ctp-surface1); color: var(--ctp-text); }
        .btn-secondary:hover { background: var(--ctp-surface2); }

        .templates-section {
            background: var(--ctp-mantle);
            padding: 20px;
            border-radius: 12px;
            border: 1px solid var(--ctp-surface0);
        }

        .templates-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
        }

        .templates-header h3 { color: var(--ctp-lavender); }

        .templates-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
            gap: 16px;
        }

        .template-card {
            background: var(--ctp-surface0);
            border-radius: 10px;
            padding: 20px;
            border: 1px solid var(--ctp-surface1);
            transition: all 0.2s;
            cursor: pointer;
        }

        .template-card:hover {
            border-color: var(--ctp-peach);
            transform: translateY(-2px);
        }

        .template-card h4 { color: var(--ctp-peach); margin-bottom: 8px; }
        .template-card p { color: var(--ctp-subtext0); font-size: 0.9rem; margin-bottom: 12px; }

        .template-tags { display: flex; flex-wrap: wrap; gap: 6px; }

        .tag {
            background: var(--ctp-surface1);
            padding: 4px 10px;
            border-radius: 12px;
            font-size: 0.75rem;
            color: var(--ctp-subtext1);
        }

        .tag.language {
            background: var(--ctp-blue);
            color: var(--ctp-crust);
        }

        ::-webkit-scrollbar { width: 8px; }
        ::-webkit-scrollbar-track { background: var(--ctp-surface0); }
        ::-webkit-scrollbar-thumb { background: var(--ctp-surface2); border-radius: 4px; }
    </style>
</head>
<body>
    <header>
        <div class="header-content">
            <div class="logo">
                <span class="logo-icon">&#128722;</span>
                <div>
                    <h1>Merchant</h1>
                    <p class="tagline">Describe what you need, I'll make it happen</p>
                </div>
            </div>
        </div>
    </header>

    <div class="container">
        <section class="agent-section">
            <div class="agent-header">
                <div class="agent-avatar">&#128722;</div>
                <div class="agent-info">
                    <h2>Merchant</h2>
                    <p>Describe what you need, I'll make it happen</p>
                </div>
            </div>
            <div class="chat-messages" id="chat-messages">
                <div class="chat-message agent">Welcome! I'm Merchant, your service builder. I can create new HolmOS services from templates - web apps, APIs, agents, and more. Just describe what you need!</div>
            </div>
            <div class="chat-input-row">
                <input type="text" class="chat-input" id="chat-input"
                       placeholder="Describe the service you need..."
                       onkeypress="if(event.key==='Enter') sendChat()">
                <button class="btn" onclick="sendChat()">Send</button>
            </div>
        </section>

        <section class="templates-section">
            <div class="templates-header">
                <h3>&#128230; Available Templates</h3>
                <button class="btn btn-secondary" onclick="loadTemplates()">Refresh</button>
            </div>
            <div class="templates-grid" id="templates-grid">
                <div style="color: var(--ctp-subtext0); padding: 20px;">Loading templates...</div>
            </div>
        </section>
    </div>

    <script>
        document.addEventListener('DOMContentLoaded', loadTemplates);

        async function loadTemplates() {
            try {
                const res = await fetch('/catalog');
                const data = await res.json();
                renderTemplates(data.templates);
            } catch (e) {
                console.error('Error loading templates:', e);
            }
        }

        function renderTemplates(templates) {
            const grid = document.getElementById('templates-grid');
            if (!templates || templates.length === 0) {
                grid.innerHTML = '<div style="color: var(--ctp-subtext0); padding: 20px;">No templates available</div>';
                return;
            }

            grid.innerHTML = templates.map(t =>
                '<div class="template-card" onclick="selectTemplate(\'' + t.id + '\')">' +
                    '<h4>' + escapeHtml(t.name) + '</h4>' +
                    '<p>' + escapeHtml(t.description) + '</p>' +
                    '<div class="template-tags">' +
                        '<span class="tag language">' + escapeHtml(t.language) + '</span>' +
                        '<span class="tag">' + escapeHtml(t.category) + '</span>' +
                    '</div>' +
                '</div>'
            ).join('');
        }

        function selectTemplate(id) {
            addChatMessage('I\'d like to use the ' + id + ' template', 'user');
            fetch('/chat', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ message: 'I want to use ' + id + ' template' })
            })
            .then(res => res.json())
            .then(data => addChatMessage(data.response, 'agent'))
            .catch(e => addChatMessage('Error: ' + e.message, 'agent'));
        }

        async function sendChat() {
            const input = document.getElementById('chat-input');
            const message = input.value.trim();
            if (!message) return;

            addChatMessage(message, 'user');
            input.value = '';

            try {
                const res = await fetch('/chat', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ message })
                });
                const data = await res.json();
                addChatMessage(data.response, 'agent');
            } catch (e) {
                addChatMessage('Error: ' + e.message, 'agent');
            }
        }

        function addChatMessage(text, type) {
            const container = document.getElementById('chat-messages');
            const div = document.createElement('div');
            div.className = 'chat-message ' + type;
            div.textContent = text;
            container.appendChild(div);
            container.scrollTop = container.scrollHeight;
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
    </script>
</body>
</html>`
