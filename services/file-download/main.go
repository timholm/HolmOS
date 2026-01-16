package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
	http.HandleFunc("/api/v1/download/", downloadHandler)

	log.Printf("file-download starting on :%s (root: %s)", port, storageRoot)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get file path from URL
	reqPath := strings.TrimPrefix(r.URL.Path, "/api/v1/download/")
	if p := r.URL.Query().Get("path"); p != "" {
		reqPath = p
	}

	if reqPath == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(storageRoot, reqPath)

	// Security: prevent path traversal
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, filepath.Clean(storageRoot)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Check if file exists
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if info.IsDir() {
		http.Error(w, "cannot download directory", http.StatusBadRequest)
		return
	}

	// Set content disposition for download
	filename := filepath.Base(reqPath)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")

	// Serve file
	http.ServeFile(w, r, fullPath)
}
