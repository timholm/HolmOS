package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type MkdirResponse struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
	Error   string `json:"error,omitempty"`
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
	http.HandleFunc("/api/v1/mkdir/", mkdirHandler)

	log.Printf("file-mkdir starting on :%s (root: %s)", port, storageRoot)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func mkdirHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqPath := strings.TrimPrefix(r.URL.Path, "/api/v1/mkdir/")
	if p := r.URL.Query().Get("path"); p != "" {
		reqPath = p
	}

	if reqPath == "" {
		respondJSON(w, http.StatusBadRequest, MkdirResponse{
			Success: false,
			Error:   "path required",
		})
		return
	}

	fullPath := filepath.Join(storageRoot, reqPath)

	// Security: prevent path traversal
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, filepath.Clean(storageRoot)) {
		respondJSON(w, http.StatusForbidden, MkdirResponse{
			Success: false,
			Error:   "forbidden path",
		})
		return
	}

	// Create directory (with parents if needed)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		respondJSON(w, http.StatusInternalServerError, MkdirResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusCreated, MkdirResponse{
		Success: true,
		Path:    reqPath,
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
