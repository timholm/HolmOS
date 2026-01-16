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

type CopyRequest struct {
	Source string `json:"source"`
	Dest   string `json:"dest"`
}

type CopyResponse struct {
	Success bool   `json:"success"`
	Source  string `json:"source"`
	Dest    string `json:"dest"`
	Size    int64  `json:"size,omitempty"`
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
	http.HandleFunc("/api/v1/copy", copyHandler)

	log.Printf("file-copy starting on :%s (root: %s)", port, storageRoot)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func copyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CopyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, CopyResponse{
			Success: false,
			Error:   "invalid request body",
		})
		return
	}

	if req.Source == "" || req.Dest == "" {
		respondJSON(w, http.StatusBadRequest, CopyResponse{
			Success: false,
			Error:   "source and dest required",
		})
		return
	}

	srcPath := filepath.Join(storageRoot, req.Source)
	dstPath := filepath.Join(storageRoot, req.Dest)

	// Security: prevent path traversal
	cleanSrc := filepath.Clean(srcPath)
	cleanDst := filepath.Clean(dstPath)
	if !strings.HasPrefix(cleanSrc, filepath.Clean(storageRoot)) ||
		!strings.HasPrefix(cleanDst, filepath.Clean(storageRoot)) {
		respondJSON(w, http.StatusForbidden, CopyResponse{
			Success: false,
			Error:   "forbidden path",
		})
		return
	}

	// Check source exists and is a file
	srcInfo, err := os.Stat(srcPath)
	if os.IsNotExist(err) {
		respondJSON(w, http.StatusNotFound, CopyResponse{
			Success: false,
			Error:   "source not found",
		})
		return
	}
	if srcInfo.IsDir() {
		respondJSON(w, http.StatusBadRequest, CopyResponse{
			Success: false,
			Error:   "cannot copy directories (use recursive option)",
		})
		return
	}

	// Create destination parent directory if needed
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		respondJSON(w, http.StatusInternalServerError, CopyResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Copy file
	src, err := os.Open(srcPath)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, CopyResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, CopyResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, CopyResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusCreated, CopyResponse{
		Success: true,
		Source:  req.Source,
		Dest:    req.Dest,
		Size:    written,
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
