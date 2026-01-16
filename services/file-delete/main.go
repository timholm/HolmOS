package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type DeleteResponse struct {
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
	http.HandleFunc("/api/v1/delete/", deleteHandler)

	log.Printf("file-delete starting on :%s (root: %s)", port, storageRoot)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqPath := strings.TrimPrefix(r.URL.Path, "/api/v1/delete/")
	if p := r.URL.Query().Get("path"); p != "" {
		reqPath = p
	}

	if reqPath == "" {
		respondJSON(w, http.StatusBadRequest, DeleteResponse{
			Success: false,
			Error:   "path required",
		})
		return
	}

	fullPath := filepath.Join(storageRoot, reqPath)

	// Security: prevent path traversal
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, filepath.Clean(storageRoot)) {
		respondJSON(w, http.StatusForbidden, DeleteResponse{
			Success: false,
			Error:   "forbidden path",
		})
		return
	}

	// Prevent deleting storage root
	if cleanPath == filepath.Clean(storageRoot) {
		respondJSON(w, http.StatusForbidden, DeleteResponse{
			Success: false,
			Error:   "cannot delete storage root",
		})
		return
	}

	// Check if exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		respondJSON(w, http.StatusNotFound, DeleteResponse{
			Success: false,
			Error:   "not found",
		})
		return
	}

	// Delete file or directory
	recursive := r.URL.Query().Get("recursive") == "true"
	var err error
	if recursive {
		err = os.RemoveAll(fullPath)
	} else {
		err = os.Remove(fullPath)
	}

	if err != nil {
		respondJSON(w, http.StatusInternalServerError, DeleteResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, DeleteResponse{
		Success: true,
		Path:    reqPath,
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
