package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type FileResult struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"is_dir"`
	ModTime time.Time `json:"mod_time"`
}

type SearchResponse struct {
	Success bool         `json:"success"`
	Query   string       `json:"query"`
	Results []FileResult `json:"results"`
	Count   int          `json:"count"`
	Error   string       `json:"error,omitempty"`
}

var storageRoot string

func main() {
	storageRoot = os.Getenv("STORAGE_ROOT")
	if storageRoot == "" {
		storageRoot = "/storage"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/v1/search", searchHandler)

	log.Printf("file-search starting on :%s (root: %s)", port, storageRoot)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		respondJSON(w, http.StatusBadRequest, SearchResponse{
			Success: false,
			Error:   "query parameter 'q' required",
		})
		return
	}

	// Optional path prefix to search within
	searchPath := r.URL.Query().Get("path")
	basePath := filepath.Join(storageRoot, searchPath)

	// Security check
	cleanBase := filepath.Clean(basePath)
	if !strings.HasPrefix(cleanBase, filepath.Clean(storageRoot)) {
		respondJSON(w, http.StatusForbidden, SearchResponse{
			Success: false,
			Error:   "forbidden path",
		})
		return
	}

	// Build regex pattern (case insensitive)
	pattern, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		respondJSON(w, http.StatusBadRequest, SearchResponse{
			Success: false,
			Error:   "invalid search pattern",
		})
		return
	}

	var results []FileResult
	maxResults := 100

	err = filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if len(results) >= maxResults {
			return filepath.SkipAll
		}

		if pattern.MatchString(info.Name()) {
			relPath, _ := filepath.Rel(storageRoot, path)
			results = append(results, FileResult{
				Path:    relPath,
				Name:    info.Name(),
				Size:    info.Size(),
				IsDir:   info.IsDir(),
				ModTime: info.ModTime(),
			})
		}

		return nil
	})

	if err != nil && !strings.Contains(err.Error(), "SkipAll") {
		respondJSON(w, http.StatusInternalServerError, SearchResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, SearchResponse{
		Success: true,
		Query:   query,
		Results: results,
		Count:   len(results),
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
