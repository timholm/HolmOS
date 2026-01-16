package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type UploadResponse struct {
	Success bool   `json:"success"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
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
	http.HandleFunc("/api/v1/upload", uploadHandler)
	http.HandleFunc("/api/v1/upload/", uploadHandler)

	log.Printf("file-upload starting on :%s (root: %s)", port, storageRoot)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Max 100MB file size
	r.ParseMultipartForm(100 << 20)

	// Get target path from URL or form
	targetPath := strings.TrimPrefix(r.URL.Path, "/api/v1/upload")
	targetPath = strings.TrimPrefix(targetPath, "/")
	if p := r.FormValue("path"); p != "" {
		targetPath = p
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondJSON(w, http.StatusBadRequest, UploadResponse{
			Success: false,
			Error:   "no file provided: " + err.Error(),
		})
		return
	}
	defer file.Close()

	// Determine final path
	filename := header.Filename
	if n := r.FormValue("filename"); n != "" {
		filename = n
	}

	destDir := filepath.Join(storageRoot, targetPath)
	destPath := filepath.Join(destDir, filename)

	// Security: prevent path traversal
	cleanDest := filepath.Clean(destPath)
	if !strings.HasPrefix(cleanDest, filepath.Clean(storageRoot)) {
		respondJSON(w, http.StatusForbidden, UploadResponse{
			Success: false,
			Error:   "forbidden path",
		})
		return
	}

	// Create directory if needed
	if err := os.MkdirAll(destDir, 0755); err != nil {
		respondJSON(w, http.StatusInternalServerError, UploadResponse{
			Success: false,
			Error:   "failed to create directory: " + err.Error(),
		})
		return
	}

	// Create destination file
	dst, err := os.Create(destPath)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, UploadResponse{
			Success: false,
			Error:   "failed to create file: " + err.Error(),
		})
		return
	}
	defer dst.Close()

	// Copy file content
	written, err := io.Copy(dst, file)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, UploadResponse{
			Success: false,
			Error:   "failed to write file: " + err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusCreated, UploadResponse{
		Success: true,
		Path:    filepath.Join(targetPath, filename),
		Size:    written,
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
