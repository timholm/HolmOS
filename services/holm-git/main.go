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

// Container Registry configuration
const registryURL = "http://localhost:31500"
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

func main() {
	os.MkdirAll(gitBase, 0755)

	connStr := "host=postgres.holm.svc.cluster.local user=postgres password=postgres dbname=holm sslmode=disable"
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
			repo_id INTEGER REFERENCES git_repos(id),
			url TEXT NOT NULL,
			secret VARCHAR(255),
			active BOOLEAN DEFAULT true
		)`)
	}

	http.HandleFunc("/", handleUI)
	http.HandleFunc("/api/repos", handleRepos)
	http.HandleFunc("/api/repos/", handleRepoActions)
	http.HandleFunc("/api/webhooks", handleWebhooks)
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
			json.NewEncoder(w).Encode([]Repo{})
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
		json.NewEncoder(w).Encode(repos)

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
		var commits []Commit
		cmd := exec.Command("git", "--git-dir="+repoPath, "log", "--pretty=format:%H|%an|%ad|%s", "--date=short", "-20")
		output, err := cmd.Output()
		if err != nil {
			json.NewEncoder(w).Encode(commits)
			return
		}

		for _, line := range strings.Split(string(output), "\n") {
			parts := strings.SplitN(line, "|", 4)
			if len(parts) == 4 {
				commits = append(commits, Commit{Hash: parts[0], Author: parts[1], Date: parts[2], Message: parts[3]})
			}
		}
		json.NewEncoder(w).Encode(commits)
	}
}

func handleWebhooks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
	case service == "git-upload-pack" || strings.Contains(r.URL.Path, "git-upload-pack"):
		handleGitService(w, r, repoPath, "git-upload-pack")
	case service == "git-receive-pack" || strings.Contains(r.URL.Path, "git-receive-pack"):
		handleGitService(w, r, repoPath, "git-receive-pack")
		triggerWebhooks(repoName)
	case strings.HasSuffix(r.URL.Path, "/info/refs"):
		handleInfoRefs(w, r, repoPath)
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
	rows, err := db.Query("SELECT w.url FROM git_webhooks w JOIN git_repos r ON w.repo_id = r.id WHERE r.name = $1 AND w.active = true", repoName)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var url string
		rows.Scan(&url)
		go func(u string) {
			payload := map[string]string{"repo": repoName, "event": "push"}
			data, _ := json.Marshal(payload)
			http.Post(u, "application/json", bytes.NewReader(data))
		}(url)
	}

	db.Exec("UPDATE git_repos SET updated_at = NOW() WHERE name = $1", repoName)
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
