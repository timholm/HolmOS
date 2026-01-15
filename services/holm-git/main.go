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
    <title>HolmGit - Code Repository</title>
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
            display: flex;
            justify-content: space-between;
            align-items: center;
            cursor: pointer;
            transition: all 0.2s;
            border: 1px solid transparent;
        }
        .repo-card:hover { border-color: #cba6f7; transform: translateX(5px); }
        .repo-info h3 { color: #89b4fa; margin-bottom: 5px; }
        .repo-info p { color: #a6adc8; font-size: 14px; }
        .repo-meta { display: flex; gap: 20px; color: #a6adc8; font-size: 14px; }
        .repo-actions { display: flex; gap: 10px; }
        .btn-small {
            background: #45475a;
            color: #cdd6f4;
            border: none;
            padding: 8px 16px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 12px;
        }
        .btn-small:hover { background: #585b70; }
        .btn-danger { background: #f38ba8; color: #1e1e2e; }
        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.8);
            justify-content: center;
            align-items: center;
            z-index: 1000;
        }
        .modal.active { display: flex; }
        .modal-content {
            background: #313244;
            padding: 30px;
            border-radius: 16px;
            width: 90%;
            max-width: 500px;
        }
        .modal-content h2 { margin-bottom: 20px; }
        .form-group { margin-bottom: 15px; }
        .form-group label { display: block; margin-bottom: 5px; color: #a6adc8; }
        .form-group input, .form-group textarea {
            width: 100%;
            padding: 12px;
            border: 1px solid #45475a;
            border-radius: 8px;
            background: #1e1e2e;
            color: #cdd6f4;
            font-size: 16px;
        }
        .clone-url {
            background: #1e1e2e;
            padding: 10px;
            border-radius: 6px;
            font-family: monospace;
            font-size: 12px;
            margin-top: 10px;
            word-break: break-all;
        }
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: #a6adc8;
        }
        .empty-state h2 { margin-bottom: 10px; color: #cdd6f4; }
        /* Repo detail view */
        .repo-detail { display: none; }
        .repo-detail.active { display: block; }
        .breadcrumb {
            display: flex;
            align-items: center;
            gap: 10px;
            margin-bottom: 20px;
            color: #a6adc8;
        }
        .breadcrumb a { color: #89b4fa; text-decoration: none; }
        .file-list {
            background: #313244;
            border-radius: 12px;
            overflow: hidden;
        }
        .file-item {
            display: flex;
            align-items: center;
            padding: 12px 20px;
            border-bottom: 1px solid #45475a;
            cursor: pointer;
        }
        .file-item:hover { background: #45475a; }
        .file-item:last-child { border-bottom: none; }
        .file-icon { width: 20px; margin-right: 15px; }
        .commit-list { margin-top: 20px; }
        .commit-item {
            background: #313244;
            padding: 15px 20px;
            border-radius: 8px;
            margin-bottom: 10px;
        }
        .commit-hash { color: #f9e2af; font-family: monospace; }
        .commit-msg { margin: 5px 0; }
        .commit-meta { color: #a6adc8; font-size: 12px; }
        .tabs {
            display: flex;
            gap: 10px;
            margin-bottom: 20px;
        }
        .tab {
            padding: 10px 20px;
            background: #45475a;
            border-radius: 8px;
            cursor: pointer;
        }
        .tab.active { background: #cba6f7; color: #1e1e2e; }
    </style>
</head>
<body>
    <div class="header">
        <h1>HolmGit</h1>
        <button class="btn" onclick="showCreateModal()">+ New Repository</button>
    </div>

    <div class="container">
        <div id="repoList">
            <div class="stats">
                <div class="stat-card">
                    <div class="number" id="repoCount">0</div>
                    <div class="label">Repositories</div>
                </div>
                <div class="stat-card">
                    <div class="number" id="commitCount">0</div>
                    <div class="label">Total Commits</div>
                </div>
                <div class="stat-card">
                    <div class="number" id="totalSize">0</div>
                    <div class="label">Storage Used</div>
                </div>
            </div>
            <div class="repo-list" id="repos"></div>
        </div>

        <div class="repo-detail" id="repoDetail">
            <div class="breadcrumb">
                <a href="#" onclick="showRepoList()">← Back</a>
                <span>/</span>
                <span id="currentRepo"></span>
            </div>
            <div class="clone-url" id="cloneUrl"></div>
            <div class="tabs">
                <div class="tab active" onclick="showTab('files')">Files</div>
                <div class="tab" onclick="showTab('commits')">Commits</div>
            </div>
            <div id="filesTab" class="file-list"></div>
            <div id="commitsTab" class="commit-list" style="display:none"></div>
        </div>
    </div>

    <div class="modal" id="createModal">
        <div class="modal-content">
            <h2>Create Repository</h2>
            <div class="form-group">
                <label>Repository Name</label>
                <input type="text" id="repoName" placeholder="my-awesome-project">
            </div>
            <div class="form-group">
                <label>Description</label>
                <textarea id="repoDesc" rows="3" placeholder="What's this repo about?"></textarea>
            </div>
            <div style="display:flex;gap:10px;justify-content:flex-end">
                <button class="btn-small" onclick="hideModal()">Cancel</button>
                <button class="btn" onclick="createRepo()">Create</button>
            </div>
        </div>
    </div>

    <script>
        const API = '/api/repos';
        let currentRepoName = '';

        async function loadRepos() {
            const res = await fetch(API);
            const repos = await res.json();
            const container = document.getElementById('repos');
            document.getElementById('repoCount').textContent = repos.length;

            let totalCommits = 0;
            repos.forEach(r => totalCommits += r.commit_count || 0);
            document.getElementById('commitCount').textContent = totalCommits;

            if (repos.length === 0) {
                container.innerHTML = '<div class="empty-state"><h2>No repositories yet</h2><p>Create your first repository to get started</p></div>';
                return;
            }

            container.innerHTML = repos.map(repo => ` + "`" + `
                <div class="repo-card" onclick="viewRepo('${repo.name}')">
                    <div class="repo-info">
                        <h3>${repo.name}</h3>
                        <p>${repo.description || 'No description'}</p>
                    </div>
                    <div class="repo-actions">
                        <span class="repo-meta">${repo.commit_count || 0} commits</span>
                        <button class="btn-small btn-danger" onclick="event.stopPropagation();deleteRepo('${repo.name}')">Delete</button>
                    </div>
                </div>
            ` + "`" + `).join('');
        }

        async function createRepo() {
            const name = document.getElementById('repoName').value.trim();
            const desc = document.getElementById('repoDesc').value.trim();
            if (!name) return alert('Name required');

            await fetch(API, {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({name, description: desc})
            });
            hideModal();
            loadRepos();
        }

        async function deleteRepo(name) {
            if (!confirm('Delete ' + name + '?')) return;
            await fetch(API + '/' + name, {method: 'DELETE'});
            loadRepos();
        }

        async function viewRepo(name) {
            currentRepoName = name;
            document.getElementById('repoList').style.display = 'none';
            document.getElementById('repoDetail').classList.add('active');
            document.getElementById('currentRepo').textContent = name;
            document.getElementById('cloneUrl').textContent = 'git clone http://192.168.8.197:30009/git/' + name + '.git';
            loadFiles(name, '');
            loadCommits(name);
        }

        function showRepoList() {
            document.getElementById('repoList').style.display = 'block';
            document.getElementById('repoDetail').classList.remove('active');
        }

        async function loadFiles(repo, path) {
            const res = await fetch(API + '/' + repo + '/files?path=' + encodeURIComponent(path));
            const files = await res.json();
            const container = document.getElementById('filesTab');

            if (!files || files.length === 0) {
                container.innerHTML = '<div class="empty-state"><p>Empty repository. Push some code!</p></div>';
                return;
            }

            container.innerHTML = files.map(f => ` + "`" + `
                <div class="file-item">
                    <span class="file-icon">${f.is_dir ? '📁' : '📄'}</span>
                    <span>${f.name}</span>
                </div>
            ` + "`" + `).join('');
        }

        async function loadCommits(repo) {
            const res = await fetch(API + '/' + repo + '/commits');
            const commits = await res.json();
            const container = document.getElementById('commitsTab');

            if (!commits || commits.length === 0) {
                container.innerHTML = '<div class="empty-state"><p>No commits yet</p></div>';
                return;
            }

            container.innerHTML = commits.map(c => ` + "`" + `
                <div class="commit-item">
                    <span class="commit-hash">${c.hash.substring(0,7)}</span>
                    <div class="commit-msg">${c.message}</div>
                    <div class="commit-meta">${c.author} · ${c.date}</div>
                </div>
            ` + "`" + `).join('');
        }

        function showTab(tab) {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            event.target.classList.add('active');
            document.getElementById('filesTab').style.display = tab === 'files' ? 'block' : 'none';
            document.getElementById('commitsTab').style.display = tab === 'commits' ? 'block' : 'none';
        }

        function showCreateModal() {
            document.getElementById('createModal').classList.add('active');
        }
        function hideModal() {
            document.getElementById('createModal').classList.remove('active');
            document.getElementById('repoName').value = '';
            document.getElementById('repoDesc').value = '';
        }

        loadRepos();
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
