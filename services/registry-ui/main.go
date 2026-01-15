package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

var registryURL string
var templates *template.Template

type Repository struct {
	Name       string   `json:"name"`
	Tags       []string `json:"tags"`
	TagDetails []TagDetail
}

type TagDetail struct {
	Tag        string
	Digest     string
	Size       int64
	Created    string
	Layers     int
	SizeHuman  string
}

type CatalogResponse struct {
	Repositories []string `json:"repositories"`
}

type TagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type ManifestResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func getRepositories() ([]string, error) {
	resp, err := http.Get(registryURL + "/v2/_catalog")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to registry: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var catalog CatalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	sort.Strings(catalog.Repositories)
	return catalog.Repositories, nil
}

func getTags(repo string) ([]string, error) {
	resp, err := http.Get(fmt.Sprintf("%s/v2/%s/tags/list", registryURL, repo))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var tags TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}

	return tags.Tags, nil
}

func getManifest(repo, tag string) (*ManifestResponse, string, error) {
	// Try OCI manifest first (buildah/podman), then Docker v2
	acceptHeaders := []string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for _, accept := range acceptHeaders {
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, repo, tag), nil)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Accept", accept)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		digest := resp.Header.Get("Docker-Content-Digest")

		var manifest ManifestResponse
		if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
			return nil, digest, err
		}

		return &manifest, digest, nil
	}

	return nil, "", fmt.Errorf("no supported manifest format found")
}

func deleteManifest(repo, digest string) error {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, repo, digest), nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	repos, err := getRepositories()
	if err != nil {
		log.Printf("Error getting repositories: %v", err)
		http.Error(w, "Failed to fetch repositories: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		RegistryURL  string
		Repositories []string
		Error        string
	}{
		RegistryURL:  registryURL,
		Repositories: repos,
	}

	if err := templates.ExecuteTemplate(w, "index", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func repoHandler(w http.ResponseWriter, r *http.Request) {
	repo := strings.TrimPrefix(r.URL.Path, "/repo/")
	if repo == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	tags, err := getTags(repo)
	if err != nil {
		http.Error(w, "Failed to fetch tags: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var tagDetails []TagDetail
	for _, tag := range tags {
		manifest, digest, err := getManifest(repo, tag)
		td := TagDetail{Tag: tag}
		if err == nil && manifest != nil {
			var totalSize int64
			for _, layer := range manifest.Layers {
				totalSize += layer.Size
			}
			td.Digest = digest
			td.Size = totalSize
			td.SizeHuman = humanSize(totalSize)
			td.Layers = len(manifest.Layers)
		}
		tagDetails = append(tagDetails, td)
	}

	data := struct {
		Repo       string
		TagDetails []TagDetail
	}{
		Repo:       repo,
		TagDetails: tagDetails,
	}

	if err := templates.ExecuteTemplate(w, "repo", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	repo := r.FormValue("repo")
	digest := r.FormValue("digest")

	if repo == "" || digest == "" {
		http.Error(w, "Missing repo or digest", http.StatusBadRequest)
		return
	}

	if err := deleteManifest(repo, digest); err != nil {
		log.Printf("Delete error: %v", err)
		http.Error(w, "Failed to delete: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Deleted %s@%s", repo, digest)
	http.Redirect(w, r, "/repo/"+repo, http.StatusFound)
}

func apiReposHandler(w http.ResponseWriter, r *http.Request) {
	repos, err := getRepositories()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"repositories": repos})
}

func apiTagsHandler(w http.ResponseWriter, r *http.Request) {
	repo := strings.TrimPrefix(r.URL.Path, "/api/tags/")
	tags, err := getTags(repo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"repo": repo, "tags": tags})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	registryURL = os.Getenv("REGISTRY_URL")
	if registryURL == "" {
		registryURL = "http://registry.holm.svc.cluster.local:5000"
	}
	registryURL = strings.TrimSuffix(registryURL, "/")

	// Parse templates
	var err error
	templates, err = template.New("").Parse(templateHTML)
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	// Routes
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/repo/", repoHandler)
	http.HandleFunc("/delete", deleteHandler)
	http.HandleFunc("/api/repos", apiReposHandler)
	http.HandleFunc("/api/tags/", apiTagsHandler)

	log.Printf("Registry UI starting on port %s", port)
	log.Printf("Registry URL: %s", registryURL)

	server := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

const templateHTML = `
{{define "index"}}
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Registry UI</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0a0a0a;
            color: #e0e0e0;
            min-height: 100vh;
        }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        h1 {
            font-size: 24px;
            font-weight: 600;
            margin-bottom: 8px;
            color: #fff;
        }
        .subtitle {
            color: #888;
            font-size: 14px;
            margin-bottom: 24px;
        }
        .repo-list {
            display: grid;
            gap: 12px;
        }
        .repo-card {
            background: #161616;
            border: 1px solid #2a2a2a;
            border-radius: 8px;
            padding: 16px 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            transition: all 0.2s;
        }
        .repo-card:hover {
            border-color: #3a3a3a;
            background: #1a1a1a;
        }
        .repo-name {
            font-weight: 500;
            color: #fff;
        }
        .repo-link {
            color: #58a6ff;
            text-decoration: none;
            font-size: 14px;
        }
        .repo-link:hover { text-decoration: underline; }
        .empty {
            text-align: center;
            padding: 60px 20px;
            color: #666;
        }
        .empty-icon { font-size: 48px; margin-bottom: 16px; }
        .stats {
            background: #161616;
            border: 1px solid #2a2a2a;
            border-radius: 8px;
            padding: 16px 20px;
            margin-bottom: 24px;
            display: flex;
            gap: 32px;
        }
        .stat-item { }
        .stat-value { font-size: 24px; font-weight: 600; color: #fff; }
        .stat-label { font-size: 12px; color: #888; text-transform: uppercase; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Container Registry</h1>
        <p class="subtitle">{{.RegistryURL}}</p>

        <div class="stats">
            <div class="stat-item">
                <div class="stat-value">{{len .Repositories}}</div>
                <div class="stat-label">Repositories</div>
            </div>
        </div>

        {{if .Repositories}}
        <div class="repo-list">
            {{range .Repositories}}
            <div class="repo-card">
                <span class="repo-name">{{.}}</span>
                <a href="/repo/{{.}}" class="repo-link">View Tags</a>
            </div>
            {{end}}
        </div>
        {{else}}
        <div class="empty">
            <div class="empty-icon">📦</div>
            <p>No images in registry</p>
            <p style="margin-top: 8px; font-size: 14px;">Push an image to get started</p>
        </div>
        {{end}}
    </div>
</body>
</html>
{{end}}

{{define "repo"}}
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Repo}} - Registry UI</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #0a0a0a;
            color: #e0e0e0;
            min-height: 100vh;
        }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        .breadcrumb {
            margin-bottom: 16px;
        }
        .breadcrumb a {
            color: #58a6ff;
            text-decoration: none;
        }
        .breadcrumb a:hover { text-decoration: underline; }
        h1 {
            font-size: 24px;
            font-weight: 600;
            margin-bottom: 24px;
            color: #fff;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            background: #161616;
            border: 1px solid #2a2a2a;
            border-radius: 8px;
            overflow: hidden;
        }
        th, td {
            padding: 12px 16px;
            text-align: left;
            border-bottom: 1px solid #2a2a2a;
        }
        th {
            background: #1a1a1a;
            font-weight: 500;
            font-size: 12px;
            text-transform: uppercase;
            color: #888;
        }
        tr:last-child td { border-bottom: none; }
        tr:hover td { background: #1a1a1a; }
        .tag { font-family: monospace; color: #58a6ff; }
        .digest {
            font-family: monospace;
            font-size: 12px;
            color: #666;
            max-width: 200px;
            overflow: hidden;
            text-overflow: ellipsis;
        }
        .size { color: #888; }
        .delete-btn {
            background: #d73a49;
            color: white;
            border: none;
            padding: 6px 12px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 12px;
        }
        .delete-btn:hover { background: #cb2431; }
        .empty {
            text-align: center;
            padding: 40px;
            color: #666;
        }
        .pull-cmd {
            background: #1a1a1a;
            border: 1px solid #2a2a2a;
            border-radius: 4px;
            padding: 8px 12px;
            font-family: monospace;
            font-size: 12px;
            color: #888;
            margin-bottom: 24px;
        }
        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.8);
            z-index: 1000;
            align-items: center;
            justify-content: center;
        }
        .modal.active { display: flex; }
        .modal-content {
            background: #161616;
            border: 1px solid #2a2a2a;
            border-radius: 8px;
            padding: 24px;
            max-width: 400px;
            width: 90%;
        }
        .modal h2 { font-size: 18px; margin-bottom: 16px; }
        .modal p { color: #888; margin-bottom: 20px; font-size: 14px; }
        .modal-btns { display: flex; gap: 12px; justify-content: flex-end; }
        .btn-cancel {
            background: #2a2a2a;
            color: #e0e0e0;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
        }
        .btn-confirm {
            background: #d73a49;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 4px;
            cursor: pointer;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="breadcrumb">
            <a href="/">Registry</a> / {{.Repo}}
        </div>
        <h1>{{.Repo}}</h1>

        {{if .TagDetails}}
        <table>
            <thead>
                <tr>
                    <th>Tag</th>
                    <th>Digest</th>
                    <th>Size</th>
                    <th>Layers</th>
                    <th>Actions</th>
                </tr>
            </thead>
            <tbody>
                {{range .TagDetails}}
                <tr>
                    <td><span class="tag">{{.Tag}}</span></td>
                    <td><span class="digest" title="{{.Digest}}">{{.Digest}}</span></td>
                    <td class="size">{{.SizeHuman}}</td>
                    <td>{{.Layers}}</td>
                    <td>
                        {{if .Digest}}
                        <button class="delete-btn" onclick="confirmDelete('{{$.Repo}}', '{{.Digest}}', '{{.Tag}}')">Delete</button>
                        {{end}}
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{else}}
        <div class="empty">No tags found for this repository</div>
        {{end}}
    </div>

    <div class="modal" id="deleteModal">
        <div class="modal-content">
            <h2>Delete Image Tag</h2>
            <p>Are you sure you want to delete <strong id="deleteTag"></strong>? This action cannot be undone.</p>
            <form id="deleteForm" method="POST" action="/delete">
                <input type="hidden" name="repo" id="deleteRepo">
                <input type="hidden" name="digest" id="deleteDigest">
                <div class="modal-btns">
                    <button type="button" class="btn-cancel" onclick="closeModal()">Cancel</button>
                    <button type="submit" class="btn-confirm">Delete</button>
                </div>
            </form>
        </div>
    </div>

    <script>
        function confirmDelete(repo, digest, tag) {
            document.getElementById('deleteRepo').value = repo;
            document.getElementById('deleteDigest').value = digest;
            document.getElementById('deleteTag').textContent = tag;
            document.getElementById('deleteModal').classList.add('active');
        }
        function closeModal() {
            document.getElementById('deleteModal').classList.remove('active');
        }
        document.getElementById('deleteModal').addEventListener('click', function(e) {
            if (e.target === this) closeModal();
        });
    </script>
</body>
</html>
{{end}}
`
