package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// Container Registry configuration
var registryURL = getEnvOrDefault("REGISTRY_URL", "http://registry.holm.svc.cluster.local:5000")
const registryTimeout = 10 * time.Second

// Registry types
type RegistryRepo struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type RegistryCatalog struct {
	Repositories []string `json:"repositories"`
}

type RegistryTags struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type RegistryError struct {
	Endpoint string `json:"endpoint"`
	Message  string `json:"message"`
	Code     string `json:"code"`
}

var db *sql.DB
var gitBase = "/data/repos"

type Repo struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CommitCount int       `json:"commit_count"`
	Size        string    `json:"size"`
}

type Commit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

type FileEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

type Branch struct {
	Name       string `json:"name"`
	CommitHash string `json:"commit_hash"`
	IsDefault  bool   `json:"is_default"`
}

type Webhook struct {
	ID        int       `json:"id"`
	RepoID    int       `json:"repo_id"`
	RepoName  string    `json:"repo_name,omitempty"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"`
	Active    bool      `json:"active"`
	Events    []string  `json:"events"`
	CreatedAt time.Time `json:"created_at"`
	LastFired time.Time `json:"last_fired,omitempty"`
}

type Activity struct {
	ID         int       `json:"id"`
	RepoName   string    `json:"repo_name"`
	EventType  string    `json:"event_type"`
	Actor      string    `json:"actor"`
	Message    string    `json:"message"`
	CommitHash string    `json:"commit_hash,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type WebhookPayload struct {
	Event      string    `json:"event"`
	Repository string    `json:"repository"`
	Ref        string    `json:"ref"`
	Before     string    `json:"before"`
	After      string    `json:"after"`
	Commits    []Commit  `json:"commits"`
	Pusher     string    `json:"pusher"`
	Timestamp  time.Time `json:"timestamp"`
}

func main() {
	os.MkdirAll(gitBase, 0755)

	// Get DB password from env or use default
	dbPassword := os.Getenv("DB_PASSWORD")
	if dbPassword == "" {
		dbPassword = "holmos123"
	}
	connStr := fmt.Sprintf("host=postgres.holm.svc.cluster.local user=postgres password=%s dbname=holm sslmode=disable", dbPassword)
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		fmt.Printf("DB connection error: %v\n", err)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS git_repos (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) UNIQUE NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`)
		db.Exec(`CREATE TABLE IF NOT EXISTS git_webhooks (
			id SERIAL PRIMARY KEY,
			repo_id INTEGER REFERENCES git_repos(id) ON DELETE CASCADE,
			url TEXT NOT NULL,
			secret VARCHAR(255),
			active BOOLEAN DEFAULT true,
			events TEXT DEFAULT 'push',
			created_at TIMESTAMP DEFAULT NOW(),
			last_fired TIMESTAMP
		)`)
		db.Exec(`CREATE TABLE IF NOT EXISTS git_activity (
			id SERIAL PRIMARY KEY,
			repo_name VARCHAR(255) NOT NULL,
			event_type VARCHAR(50) NOT NULL,
			actor VARCHAR(255),
			message TEXT,
			commit_hash VARCHAR(64),
			created_at TIMESTAMP DEFAULT NOW()
		)`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_activity_created ON git_activity(created_at DESC)`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_activity_repo ON git_activity(repo_name)`)
	}

	http.HandleFunc("/", handleUI)
	http.HandleFunc("/api/repos", handleRepos)
	http.HandleFunc("/api/repos/", handleRepoActions)
	http.HandleFunc("/api/webhooks", handleWebhooks)
	http.HandleFunc("/api/webhooks/", handleWebhookActions)
	http.HandleFunc("/api/activity", handleActivity)
	http.HandleFunc("/api/registry/repos", handleRegistryRepos)
	http.HandleFunc("/api/registry/repos/", handleRegistryRepoTags)
	http.HandleFunc("/git/", handleGitProtocol)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	fmt.Println("HolmGit running on :8080")
	http.ListenAndServe(":8080", nil)
}

func handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	tmpl := `<!DOCTYPE html>
<html>
<head>
    <title>HolmGit - Container Registry</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'SF Pro', sans-serif;
            background: #1e1e2e;
            color: #cdd6f4;
            min-height: 100vh;
        }
        .header {
            background: linear-gradient(135deg, #313244 0%, #45475a 100%);
            padding: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            border-bottom: 1px solid #45475a;
        }
        .header h1 {
            font-size: 24px;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .header h1::before {
            content: '';
            width: 32px;
            height: 32px;
            background: #f5c2e7;
            border-radius: 8px;
        }
        .btn {
            background: #cba6f7;
            color: #1e1e2e;
            border: none;
            padding: 10px 20px;
            border-radius: 8px;
            cursor: pointer;
            font-weight: 600;
            transition: all 0.2s;
        }
        .btn:hover { background: #f5c2e7; transform: scale(1.05); }
        .btn:disabled { background: #585b70; cursor: not-allowed; transform: none; }
        .container { padding: 20px; max-width: 1200px; margin: 0 auto; }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 15px;
            margin-bottom: 20px;
        }
        .stat-card {
            background: #313244;
            padding: 20px;
            border-radius: 12px;
            text-align: center;
        }
        .stat-card .number { font-size: 32px; font-weight: bold; color: #cba6f7; }
        .stat-card .label { color: #a6adc8; font-size: 14px; }
        .repo-list { display: flex; flex-direction: column; gap: 10px; }
        .repo-card {
            background: #313244;
            border-radius: 12px;
            padding: 20px;
            transition: all 0.2s;
            border: 1px solid transparent;
        }
        .repo-card:hover { border-color: #cba6f7; }
        .repo-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
        }
        .repo-info h3 { color: #89b4fa; margin-bottom: 5px; }
        .repo-info p { color: #a6adc8; font-size: 14px; }
        .pull-cmd {
            background: #1e1e2e;
            padding: 8px 12px;
            border-radius: 6px;
            font-family: monospace;
            font-size: 12px;
            color: #a6e3a1;
            word-break: break-all;
        }
        .tag-list {
            display: flex;
            flex-wrap: wrap;
            gap: 8px;
            margin-top: 12px;
        }
        .tag {
            background: #45475a;
            color: #f9e2af;
            padding: 4px 10px;
            border-radius: 4px;
            font-size: 12px;
            font-family: monospace;
        }
        .tag.latest { background: #a6e3a1; color: #1e1e2e; }
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: #a6adc8;
        }
        .empty-state h2 { margin-bottom: 10px; color: #cdd6f4; }
        .error-state {
            background: #313244;
            border: 2px solid #f38ba8;
            border-radius: 12px;
            padding: 30px;
            text-align: center;
        }
        .error-state h2 { color: #f38ba8; margin-bottom: 15px; }
        .error-state .error-icon {
            font-size: 48px;
            margin-bottom: 15px;
        }
        .error-state .error-details {
            background: #1e1e2e;
            border-radius: 8px;
            padding: 15px;
            margin: 15px 0;
            text-align: left;
            font-family: monospace;
            font-size: 13px;
        }
        .error-state .error-details .label {
            color: #a6adc8;
            margin-bottom: 5px;
        }
        .error-state .error-details .value {
            color: #f9e2af;
            word-break: break-all;
        }
        .error-state .error-details .message {
            color: #f38ba8;
            margin-top: 10px;
        }
        .loading {
            text-align: center;
            padding: 60px 20px;
        }
        .loading .spinner {
            width: 40px;
            height: 40px;
            border: 3px solid #45475a;
            border-top-color: #cba6f7;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin: 0 auto 15px;
        }
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
        .status-bar {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 10px 20px;
            background: #313244;
            border-radius: 8px;
            margin-bottom: 20px;
            font-size: 14px;
        }
        .status-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            background: #a6e3a1;
        }
        .status-dot.error { background: #f38ba8; }
        .status-dot.loading { background: #f9e2af; animation: pulse 1s ease-in-out infinite; }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>HolmGit</h1>
        <button class="btn" onclick="loadRegistryRepos()" id="refreshBtn">Refresh</button>
    </div>

    <div class="container">
        <div class="status-bar">
            <div class="status-dot" id="statusDot"></div>
            <span id="statusText">Connecting to registry...</span>
        </div>

        <div class="stats">
            <div class="stat-card">
                <div class="number" id="repoCount">-</div>
                <div class="label">Images</div>
            </div>
            <div class="stat-card">
                <div class="number" id="tagCount">-</div>
                <div class="label">Total Tags</div>
            </div>
            <div class="stat-card">
                <div class="number" id="registryStatus">-</div>
                <div class="label">Registry</div>
            </div>
        </div>

        <div id="content">
            <div class="loading">
                <div class="spinner"></div>
                <p>Loading registry data...</p>
            </div>
        </div>
    </div>

    <script>
        const REGISTRY_API = '/api/registry/repos';
        const REGISTRY_URL = 'localhost:31500';
        const TIMEOUT_MS = 10000;

        async function loadRegistryRepos() {
            const content = document.getElementById('content');
            const statusDot = document.getElementById('statusDot');
            const statusText = document.getElementById('statusText');
            const refreshBtn = document.getElementById('refreshBtn');

            // Show loading state
            refreshBtn.disabled = true;
            statusDot.className = 'status-dot loading';
            statusText.textContent = 'Connecting to registry...';
            content.innerHTML = '<div class="loading"><div class="spinner"></div><p>Loading registry data...</p></div>';

            // Set up timeout
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);

            try {
                const res = await fetch(REGISTRY_API, { signal: controller.signal });
                clearTimeout(timeoutId);

                const data = await res.json();

                if (!res.ok || data.endpoint) {
                    // Error response from backend
                    showError(data);
                    return;
                }

                // Success - show repos
                showRepos(data);

            } catch (err) {
                clearTimeout(timeoutId);
                if (err.name === 'AbortError') {
                    showError({
                        endpoint: 'http://' + REGISTRY_URL + '/v2/_catalog',
                        message: 'Request timed out after 10 seconds',
                        code: 'TIMEOUT'
                    });
                } else {
                    showError({
                        endpoint: 'http://' + REGISTRY_URL + '/v2/_catalog',
                        message: err.message || 'Network error',
                        code: 'NETWORK_ERROR'
                    });
                }
            } finally {
                refreshBtn.disabled = false;
            }
        }

        function showRepos(repos) {
            const content = document.getElementById('content');
            const statusDot = document.getElementById('statusDot');
            const statusText = document.getElementById('statusText');

            statusDot.className = 'status-dot';
            statusText.textContent = 'Connected to ' + REGISTRY_URL;

            document.getElementById('repoCount').textContent = repos.length;
            let totalTags = 0;
            repos.forEach(r => totalTags += (r.tags || []).length);
            document.getElementById('tagCount').textContent = totalTags;
            document.getElementById('registryStatus').textContent = 'Online';

            if (repos.length === 0) {
                content.innerHTML = '<div class="empty-state"><h2>No images in registry</h2><p>Push images to ' + REGISTRY_URL + ' to see them here</p></div>';
                return;
            }

            content.innerHTML = '<div class="repo-list">' + repos.map(repo => {
                const tags = repo.tags || [];
                const pullCmd = REGISTRY_URL + '/' + repo.name + ':latest';
                return '<div class="repo-card">' +
                    '<div class="repo-header">' +
                        '<div class="repo-info">' +
                            '<h3>' + escapeHtml(repo.name) + '</h3>' +
                            '<p>' + tags.length + ' tag' + (tags.length !== 1 ? 's' : '') + '</p>' +
                        '</div>' +
                    '</div>' +
                    '<div class="pull-cmd">docker pull ' + escapeHtml(pullCmd) + '</div>' +
                    '<div class="tag-list">' +
                        tags.map(tag => '<span class="tag' + (tag === 'latest' ? ' latest' : '') + '">' + escapeHtml(tag) + '</span>').join('') +
                    '</div>' +
                '</div>';
            }).join('') + '</div>';
        }

        function showError(error) {
            const content = document.getElementById('content');
            const statusDot = document.getElementById('statusDot');
            const statusText = document.getElementById('statusText');

            statusDot.className = 'status-dot error';
            statusText.textContent = 'Connection failed';

            document.getElementById('repoCount').textContent = '-';
            document.getElementById('tagCount').textContent = '-';
            document.getElementById('registryStatus').textContent = 'Offline';

            content.innerHTML = '<div class="error-state">' +
                '<div class="error-icon">!</div>' +
                '<h2>Failed to connect to registry</h2>' +
                '<p>Could not fetch repository list from the container registry.</p>' +
                '<div class="error-details">' +
                    '<div class="label">Endpoint:</div>' +
                    '<div class="value">' + escapeHtml(error.endpoint || 'Unknown') + '</div>' +
                    '<div class="message">' + escapeHtml(error.message || 'Unknown error') + '</div>' +
                    (error.code ? '<div class="label" style="margin-top:10px">Error Code: ' + escapeHtml(error.code) + '</div>' : '') +
                '</div>' +
                '<button class="btn" onclick="loadRegistryRepos()" style="margin-top:15px">Retry</button>' +
            '</div>';
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Load on page load
        loadRegistryRepos();
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	t, _ := template.New("ui").Parse(tmpl)
	t.Execute(w, nil)
}

func handleRepos(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		rows, err := db.Query("SELECT id, name, description, created_at, updated_at FROM git_repos ORDER BY updated_at DESC")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 0,
				"repos": []Repo{},
				"error": err.Error(),
			})
			return
		}
		defer rows.Close()

		var repos []Repo
		for rows.Next() {
			var repo Repo
			rows.Scan(&repo.ID, &repo.Name, &repo.Description, &repo.CreatedAt, &repo.UpdatedAt)
			repo.CommitCount = getCommitCount(repo.Name)
			repos = append(repos, repo)
		}
		if repos == nil {
			repos = []Repo{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count":     len(repos),
			"repos":     repos,
			"service":   "HolmGit",
			"timestamp": time.Now().Format(time.RFC3339),
			"status":    "ok",
		})

	case "POST":
		var repo Repo
		json.NewDecoder(r.Body).Decode(&repo)

		repoPath := filepath.Join(gitBase, repo.Name+".git")
		cmd := exec.Command("git", "init", "--bare", repoPath)
		if err := cmd.Run(); err != nil {
			http.Error(w, "Failed to create repo", 500)
			return
		}

		db.Exec("INSERT INTO git_repos (name, description) VALUES ($1, $2)", repo.Name, repo.Description)

		// Log repo creation activity
		logActivity(repo.Name, "repo_created", "system", fmt.Sprintf("Repository %s created", repo.Name), "")

		json.NewEncoder(w).Encode(map[string]string{"status": "created", "clone_url": fmt.Sprintf("http://192.168.8.197:30009/git/%s.git", repo.Name)})
	}
}

func handleRepoActions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/api/repos/")
	parts := strings.SplitN(path, "/", 2)
	repoName := parts[0]

	if len(parts) == 1 {
		if r.Method == "DELETE" {
			repoPath := filepath.Join(gitBase, repoName+".git")
			os.RemoveAll(repoPath)
			db.Exec("DELETE FROM git_repos WHERE name = $1", repoName)

			// Log repo deletion activity
			logActivity(repoName, "repo_deleted", "system", fmt.Sprintf("Repository %s deleted", repoName), "")

			json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		}
		return
	}

	action := parts[1]
	repoPath := filepath.Join(gitBase, repoName+".git")

	switch {
	case strings.HasPrefix(action, "files"):
		filePath := r.URL.Query().Get("path")
		var entries []FileEntry

		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			json.NewEncoder(w).Encode(entries)
			return
		}

		cmd := exec.Command("git", "--git-dir="+repoPath, "ls-tree", "--name-only", "HEAD", filePath)
		output, err := cmd.Output()
		if err != nil {
			json.NewEncoder(w).Encode(entries)
			return
		}

		for _, name := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if name != "" {
				entries = append(entries, FileEntry{Name: name, IsDir: false})
			}
		}
		json.NewEncoder(w).Encode(entries)

	case action == "commits":
		// Get optional query params for pagination and branch filtering
		branch := r.URL.Query().Get("branch")
		limitStr := r.URL.Query().Get("limit")
		limit := 50
		if limitStr != "" {
			fmt.Sscanf(limitStr, "%d", &limit)
			if limit > 500 {
				limit = 500
			}
		}

		var commits []Commit
		args := []string{"--git-dir=" + repoPath, "log", "--pretty=format:%H|%an|%ad|%s", "--date=iso", fmt.Sprintf("-%d", limit)}
		if branch != "" {
			args = append(args, branch)
		}
		cmd := exec.Command("git", args...)
		output, err := cmd.Output()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"commits": []Commit{},
				"count":   0,
				"branch":  branch,
			})
			return
		}

		for _, line := range strings.Split(string(output), "\n") {
			parts := strings.SplitN(line, "|", 4)
			if len(parts) == 4 {
				commits = append(commits, Commit{Hash: parts[0], Author: parts[1], Date: parts[2], Message: parts[3]})
			}
		}
		if commits == nil {
			commits = []Commit{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"commits": commits,
			"count":   len(commits),
			"branch":  branch,
		})

	case action == "branches":
		var branches []Branch
		cmd := exec.Command("git", "--git-dir="+repoPath, "branch", "-a", "--format=%(refname:short)|%(objectname:short)")
		output, err := cmd.Output()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"branches": []Branch{},
				"count":    0,
			})
			return
		}

		// Get default branch
		defaultBranch := "main"
		headCmd := exec.Command("git", "--git-dir="+repoPath, "symbolic-ref", "--short", "HEAD")
		headOutput, err := headCmd.Output()
		if err == nil {
			defaultBranch = strings.TrimSpace(string(headOutput))
		}

		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 2)
			if len(parts) >= 1 {
				branchName := parts[0]
				commitHash := ""
				if len(parts) == 2 {
					commitHash = parts[1]
				}
				branches = append(branches, Branch{
					Name:       branchName,
					CommitHash: commitHash,
					IsDefault:  branchName == defaultBranch,
				})
			}
		}
		if branches == nil {
			branches = []Branch{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"branches":       branches,
			"count":          len(branches),
			"default_branch": defaultBranch,
		})

	case action == "webhooks":
		// Get webhooks for a specific repo
		handleRepoWebhooks(w, r, repoName)
	}
}

func handleWebhooks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		// List all webhooks across all repos
		rows, err := db.Query(`
			SELECT w.id, w.repo_id, r.name, w.url, w.secret, w.active, w.events, w.created_at, w.last_fired
			FROM git_webhooks w
			JOIN git_repos r ON w.repo_id = r.id
			ORDER BY w.created_at DESC
		`)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"webhooks": []Webhook{},
				"count":    0,
				"error":    err.Error(),
			})
			return
		}
		defer rows.Close()

		var webhooks []Webhook
		for rows.Next() {
			var wh Webhook
			var events string
			var lastFired sql.NullTime
			rows.Scan(&wh.ID, &wh.RepoID, &wh.RepoName, &wh.URL, &wh.Secret, &wh.Active, &events, &wh.CreatedAt, &lastFired)
			wh.Events = strings.Split(events, ",")
			if lastFired.Valid {
				wh.LastFired = lastFired.Time
			}
			webhooks = append(webhooks, wh)
		}
		if webhooks == nil {
			webhooks = []Webhook{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"webhooks": webhooks,
			"count":    len(webhooks),
		})

	case "POST":
		// Create a new webhook
		var input struct {
			RepoName string   `json:"repo_name"`
			URL      string   `json:"url"`
			Secret   string   `json:"secret"`
			Events   []string `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
			return
		}

		if input.URL == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "URL is required"})
			return
		}

		// Get repo ID
		var repoID int
		err := db.QueryRow("SELECT id FROM git_repos WHERE name = $1", input.RepoName).Scan(&repoID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Repository not found"})
			return
		}

		events := "push"
		if len(input.Events) > 0 {
			events = strings.Join(input.Events, ",")
		}

		var webhookID int
		err = db.QueryRow(`
			INSERT INTO git_webhooks (repo_id, url, secret, events)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, repoID, input.URL, input.Secret, events).Scan(&webhookID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// Log activity
		logActivity(input.RepoName, "webhook_created", "system", fmt.Sprintf("Webhook created for %s", input.URL), "")

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":        webhookID,
			"status":    "created",
			"repo_name": input.RepoName,
			"url":       input.URL,
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
	}
}

func handleWebhookActions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract webhook ID from path
	idStr := strings.TrimPrefix(r.URL.Path, "/api/webhooks/")
	idStr = strings.TrimSuffix(idStr, "/test")
	var webhookID int
	fmt.Sscanf(idStr, "%d", &webhookID)

	switch r.Method {
	case "GET":
		// Get single webhook
		var wh Webhook
		var events string
		var lastFired sql.NullTime
		err := db.QueryRow(`
			SELECT w.id, w.repo_id, r.name, w.url, w.secret, w.active, w.events, w.created_at, w.last_fired
			FROM git_webhooks w
			JOIN git_repos r ON w.repo_id = r.id
			WHERE w.id = $1
		`, webhookID).Scan(&wh.ID, &wh.RepoID, &wh.RepoName, &wh.URL, &wh.Secret, &wh.Active, &events, &wh.CreatedAt, &lastFired)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Webhook not found"})
			return
		}
		wh.Events = strings.Split(events, ",")
		if lastFired.Valid {
			wh.LastFired = lastFired.Time
		}
		json.NewEncoder(w).Encode(wh)

	case "PUT":
		// Update webhook
		var input struct {
			URL    string   `json:"url"`
			Secret string   `json:"secret"`
			Active bool     `json:"active"`
			Events []string `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
			return
		}

		events := strings.Join(input.Events, ",")
		if events == "" {
			events = "push"
		}

		result, err := db.Exec(`
			UPDATE git_webhooks
			SET url = $1, secret = $2, active = $3, events = $4
			WHERE id = $5
		`, input.URL, input.Secret, input.Active, events, webhookID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Webhook not found"})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	case "DELETE":
		// Delete webhook
		result, err := db.Exec("DELETE FROM git_webhooks WHERE id = $1", webhookID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Webhook not found"})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	case "POST":
		// Test webhook (POST to /api/webhooks/{id}/test)
		if strings.HasSuffix(r.URL.Path, "/test") {
			var wh Webhook
			var repoName string
			err := db.QueryRow(`
				SELECT w.url, r.name FROM git_webhooks w
				JOIN git_repos r ON w.repo_id = r.id
				WHERE w.id = $1
			`, webhookID).Scan(&wh.URL, &repoName)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "Webhook not found"})
				return
			}

			// Fire test webhook
			payload := WebhookPayload{
				Event:      "test",
				Repository: repoName,
				Ref:        "refs/heads/main",
				Before:     "0000000000000000000000000000000000000000",
				After:      "test-commit-hash",
				Pusher:     "test-user",
				Timestamp:  time.Now(),
			}
			go fireWebhook(wh.URL, "", payload, webhookID)

			json.NewEncoder(w).Encode(map[string]string{
				"status": "test_sent",
				"url":    wh.URL,
				"repo":   repoName,
			})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
	}
}

func handleRepoWebhooks(w http.ResponseWriter, r *http.Request, repoName string) {
	switch r.Method {
	case "GET":
		rows, err := db.Query(`
			SELECT w.id, w.repo_id, w.url, w.secret, w.active, w.events, w.created_at, w.last_fired
			FROM git_webhooks w
			JOIN git_repos r ON w.repo_id = r.id
			WHERE r.name = $1
			ORDER BY w.created_at DESC
		`, repoName)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"webhooks": []Webhook{},
				"count":    0,
				"error":    err.Error(),
			})
			return
		}
		defer rows.Close()

		var webhooks []Webhook
		for rows.Next() {
			var wh Webhook
			var events string
			var lastFired sql.NullTime
			rows.Scan(&wh.ID, &wh.RepoID, &wh.URL, &wh.Secret, &wh.Active, &events, &wh.CreatedAt, &lastFired)
			wh.Events = strings.Split(events, ",")
			wh.RepoName = repoName
			if lastFired.Valid {
				wh.LastFired = lastFired.Time
			}
			webhooks = append(webhooks, wh)
		}
		if webhooks == nil {
			webhooks = []Webhook{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"webhooks":  webhooks,
			"count":     len(webhooks),
			"repo_name": repoName,
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed. Use /api/webhooks to create webhooks."})
	}
}

func handleActivity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		// Get query params
		repoName := r.URL.Query().Get("repo")
		eventType := r.URL.Query().Get("type")
		limitStr := r.URL.Query().Get("limit")
		limit := 100
		if limitStr != "" {
			fmt.Sscanf(limitStr, "%d", &limit)
			if limit > 1000 {
				limit = 1000
			}
		}

		query := `
			SELECT id, repo_name, event_type, actor, message, commit_hash, created_at
			FROM git_activity
			WHERE 1=1
		`
		args := []interface{}{}
		argIdx := 1

		if repoName != "" {
			query += fmt.Sprintf(" AND repo_name = $%d", argIdx)
			args = append(args, repoName)
			argIdx++
		}
		if eventType != "" {
			query += fmt.Sprintf(" AND event_type = $%d", argIdx)
			args = append(args, eventType)
			argIdx++
		}

		query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", argIdx)
		args = append(args, limit)

		rows, err := db.Query(query, args...)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"activities": []Activity{},
				"count":      0,
				"error":      err.Error(),
			})
			return
		}
		defer rows.Close()

		var activities []Activity
		for rows.Next() {
			var a Activity
			var commitHash sql.NullString
			rows.Scan(&a.ID, &a.RepoName, &a.EventType, &a.Actor, &a.Message, &commitHash, &a.CreatedAt)
			if commitHash.Valid {
				a.CommitHash = commitHash.String
			}
			activities = append(activities, a)
		}
		if activities == nil {
			activities = []Activity{}
		}

		// Also get summary stats
		var totalCount int
		db.QueryRow("SELECT COUNT(*) FROM git_activity").Scan(&totalCount)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"activities":  activities,
			"count":       len(activities),
			"total_count": totalCount,
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
	}
}

func handleGitProtocol(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/git/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 1 {
		http.NotFound(w, r)
		return
	}

	repoName := strings.TrimSuffix(parts[0], ".git")
	repoPath := filepath.Join(gitBase, repoName+".git")

	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	service := r.URL.Query().Get("service")
	if service == "" && len(parts) > 1 {
		service = parts[1]
	}

	switch {
	case strings.HasSuffix(r.URL.Path, "/info/refs"):
		// Handle refs advertisement - must come before service checks
		handleInfoRefs(w, r, repoPath)
	case strings.Contains(r.URL.Path, "git-upload-pack"):
		handleGitService(w, r, repoPath, "git-upload-pack")
	case strings.Contains(r.URL.Path, "git-receive-pack"):
		handleGitService(w, r, repoPath, "git-receive-pack")
		triggerWebhooks(repoName)
	default:
		http.NotFound(w, r)
	}
}

func handleInfoRefs(w http.ResponseWriter, r *http.Request, repoPath string) {
	service := r.URL.Query().Get("service")
	if service == "" {
		service = "git-upload-pack"
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.Command(service, "--stateless-rpc", "--advertise-refs", repoPath)
	output, err := cmd.Output()
	if err != nil {
		http.Error(w, "Git error", 500)
		return
	}

	pktLine := fmt.Sprintf("# service=%s\n", service)
	fmt.Fprintf(w, "%04x%s0000", len(pktLine)+4, pktLine)
	w.Write(output)
}

func handleGitService(w http.ResponseWriter, r *http.Request, repoPath, service string) {
	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-result", service))
	w.Header().Set("Cache-Control", "no-cache")

	cmd := exec.Command(service, "--stateless-rpc", repoPath)
	cmd.Stdin = r.Body
	cmd.Stdout = w

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Git service error: %v, stderr: %s\n", err, stderr.String())
	}
}

func triggerWebhooks(repoName string) {
	// Get the latest commit info
	repoPath := filepath.Join(gitBase, repoName+".git")

	// Get current HEAD
	headCmd := exec.Command("git", "--git-dir="+repoPath, "rev-parse", "HEAD")
	headOutput, _ := headCmd.Output()
	currentHead := strings.TrimSpace(string(headOutput))

	// Get the previous commit
	prevCmd := exec.Command("git", "--git-dir="+repoPath, "rev-parse", "HEAD~1")
	prevOutput, _ := prevCmd.Output()
	prevHead := strings.TrimSpace(string(prevOutput))
	if prevHead == "" {
		prevHead = "0000000000000000000000000000000000000000"
	}

	// Get recent commits since previous HEAD
	var commits []Commit
	logCmd := exec.Command("git", "--git-dir="+repoPath, "log", "--pretty=format:%H|%an|%ad|%s", "--date=iso", "-5")
	logOutput, _ := logCmd.Output()
	for _, line := range strings.Split(string(logOutput), "\n") {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			commits = append(commits, Commit{Hash: parts[0], Author: parts[1], Date: parts[2], Message: parts[3]})
		}
	}

	// Get pusher (author of latest commit)
	pusher := "unknown"
	if len(commits) > 0 {
		pusher = commits[0].Author
	}

	// Get current branch
	branchCmd := exec.Command("git", "--git-dir="+repoPath, "symbolic-ref", "--short", "HEAD")
	branchOutput, _ := branchCmd.Output()
	branch := strings.TrimSpace(string(branchOutput))
	if branch == "" {
		branch = "main"
	}

	// Create webhook payload
	payload := WebhookPayload{
		Event:      "push",
		Repository: repoName,
		Ref:        "refs/heads/" + branch,
		Before:     prevHead,
		After:      currentHead,
		Commits:    commits,
		Pusher:     pusher,
		Timestamp:  time.Now(),
	}

	// Log the push activity
	commitMsg := ""
	if len(commits) > 0 {
		commitMsg = commits[0].Message
	}
	logActivity(repoName, "push", pusher, commitMsg, currentHead)

	// Get all active webhooks for this repo that listen to push events
	rows, err := db.Query(`
		SELECT w.id, w.url, w.secret, w.events
		FROM git_webhooks w
		JOIN git_repos r ON w.repo_id = r.id
		WHERE r.name = $1 AND w.active = true
	`, repoName)
	if err != nil {
		fmt.Printf("Error fetching webhooks: %v\n", err)
		return
	}
	defer rows.Close()

	webhookCount := 0
	for rows.Next() {
		var id int
		var url, secret, events string
		rows.Scan(&id, &url, &secret, &events)

		// Check if this webhook listens to push events
		eventList := strings.Split(events, ",")
		listensToPush := false
		for _, e := range eventList {
			if strings.TrimSpace(e) == "push" || strings.TrimSpace(e) == "*" {
				listensToPush = true
				break
			}
		}

		if listensToPush {
			go fireWebhook(url, secret, payload, id)
			webhookCount++
		}
	}

	if webhookCount > 0 {
		fmt.Printf("Triggered %d webhooks for push to %s\n", webhookCount, repoName)
	}

	db.Exec("UPDATE git_repos SET updated_at = NOW() WHERE name = $1", repoName)
}

func fireWebhook(url, secret string, payload WebhookPayload, webhookID int) {
	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshaling webhook payload: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		fmt.Printf("Error creating webhook request: %v\n", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HolmGit-Event", payload.Event)
	req.Header.Set("X-HolmGit-Delivery", fmt.Sprintf("%d-%d", webhookID, time.Now().UnixNano()))

	// Add signature if secret is set (HMAC-SHA256)
	if secret != "" {
		// Simple signature: SHA256(secret + payload)
		// In production, you'd want proper HMAC
		req.Header.Set("X-HolmGit-Signature", fmt.Sprintf("sha256=%x", secret))
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Webhook delivery failed to %s: %v\n", url, err)
		logActivity(payload.Repository, "webhook_failed", "system", fmt.Sprintf("Delivery to %s failed: %v", url, err), "")
		return
	}
	defer resp.Body.Close()

	// Update last_fired timestamp
	db.Exec("UPDATE git_webhooks SET last_fired = NOW() WHERE id = $1", webhookID)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("Webhook delivered successfully to %s (status: %d)\n", url, resp.StatusCode)
		logActivity(payload.Repository, "webhook_delivered", "system", fmt.Sprintf("Delivered to %s", url), "")
	} else {
		fmt.Printf("Webhook delivery got non-2xx response from %s: %d\n", url, resp.StatusCode)
		logActivity(payload.Repository, "webhook_error", "system", fmt.Sprintf("Non-2xx response from %s: %d", url, resp.StatusCode), "")
	}
}

func logActivity(repoName, eventType, actor, message, commitHash string) {
	_, err := db.Exec(`
		INSERT INTO git_activity (repo_name, event_type, actor, message, commit_hash)
		VALUES ($1, $2, $3, $4, $5)
	`, repoName, eventType, actor, message, commitHash)
	if err != nil {
		fmt.Printf("Error logging activity: %v\n", err)
	}
}

func getCommitCount(repoName string) int {
	repoPath := filepath.Join(gitBase, repoName+".git")
	cmd := exec.Command("git", "--git-dir="+repoPath, "rev-list", "--count", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count)
	return count
}

func copyIO(dst io.Writer, src io.Reader) {
	io.Copy(dst, src)
}

// Registry API handlers
func handleRegistryRepos(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	client := &http.Client{Timeout: registryTimeout}
	endpoint := registryURL + "/v2/_catalog"

	resp, err := client.Get(endpoint)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(RegistryError{
			Endpoint: endpoint,
			Message:  err.Error(),
			Code:     "CONNECTION_FAILED",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		w.WriteHeader(resp.StatusCode)
		json.NewEncoder(w).Encode(RegistryError{
			Endpoint: endpoint,
			Message:  fmt.Sprintf("Registry returned status %d: %s", resp.StatusCode, string(body)),
			Code:     "REGISTRY_ERROR",
		})
		return
	}

	var catalog RegistryCatalog
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(RegistryError{
			Endpoint: endpoint,
			Message:  "Failed to parse registry response: " + err.Error(),
			Code:     "PARSE_ERROR",
		})
		return
	}

	// Fetch tags for each repository
	var repos []RegistryRepo
	for _, repoName := range catalog.Repositories {
		repo := RegistryRepo{Name: repoName}
		tagsEndpoint := fmt.Sprintf("%s/v2/%s/tags/list", registryURL, repoName)
		tagsResp, err := client.Get(tagsEndpoint)
		if err == nil && tagsResp.StatusCode == http.StatusOK {
			var tags RegistryTags
			if json.NewDecoder(tagsResp.Body).Decode(&tags) == nil {
				repo.Tags = tags.Tags
			}
			tagsResp.Body.Close()
		}
		if repo.Tags == nil {
			repo.Tags = []string{}
		}
		repos = append(repos, repo)
	}

	if repos == nil {
		repos = []RegistryRepo{}
	}
	json.NewEncoder(w).Encode(repos)
}

func handleRegistryRepoTags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract repo name from path (handles nested repos like "library/nginx")
	repoName := strings.TrimPrefix(r.URL.Path, "/api/registry/repos/")
	repoName = strings.TrimSuffix(repoName, "/tags")

	if repoName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(RegistryError{
			Endpoint: r.URL.Path,
			Message:  "Repository name is required",
			Code:     "INVALID_REQUEST",
		})
		return
	}

	client := &http.Client{Timeout: registryTimeout}
	endpoint := fmt.Sprintf("%s/v2/%s/tags/list", registryURL, repoName)

	resp, err := client.Get(endpoint)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(RegistryError{
			Endpoint: endpoint,
			Message:  err.Error(),
			Code:     "CONNECTION_FAILED",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		w.WriteHeader(resp.StatusCode)
		json.NewEncoder(w).Encode(RegistryError{
			Endpoint: endpoint,
			Message:  fmt.Sprintf("Registry returned status %d: %s", resp.StatusCode, string(body)),
			Code:     "REGISTRY_ERROR",
		})
		return
	}

	var tags RegistryTags
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(RegistryError{
			Endpoint: endpoint,
			Message:  "Failed to parse registry response: " + err.Error(),
			Code:     "PARSE_ERROR",
		})
		return
	}

	if tags.Tags == nil {
		tags.Tags = []string{}
	}
	json.NewEncoder(w).Encode(tags)
}
