package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type MoveRequest struct {
	Source string `json:"source"`
	Dest   string `json:"dest"`
}

type MoveResponse struct {
	Success bool   `json:"success"`
	Source  string `json:"source"`
	Dest    string `json:"dest"`
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
	http.HandleFunc("/api/v1/move", moveHandler)

	log.Printf("file-move starting on :%s (root: %s)", port, storageRoot)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func moveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, MoveResponse{
			Success: false,
			Error:   "invalid request body",
		})
		return
	}

	if req.Source == "" || req.Dest == "" {
		respondJSON(w, http.StatusBadRequest, MoveResponse{
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
		respondJSON(w, http.StatusForbidden, MoveResponse{
			Success: false,
			Error:   "forbidden path",
		})
		return
	}

	// Check source exists
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		respondJSON(w, http.StatusNotFound, MoveResponse{
			Success: false,
			Error:   "source not found",
		})
		return
	}

	// Create destination parent directory if needed
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		respondJSON(w, http.StatusInternalServerError, MoveResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Move/rename
	if err := os.Rename(srcPath, dstPath); err != nil {
		respondJSON(w, http.StatusInternalServerError, MoveResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, MoveResponse{
		Success: true,
		Source:  req.Source,
		Dest:    req.Dest,
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
