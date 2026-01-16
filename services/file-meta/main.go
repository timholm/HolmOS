package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileMeta struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	IsDir      bool      `json:"is_dir"`
	Mode       string    `json:"mode"`
	ModTime    time.Time `json:"mod_time"`
	AccessTime time.Time `json:"access_time,omitempty"`
	Checksum   string    `json:"checksum,omitempty"`
	Mime       string    `json:"mime,omitempty"`
}

type MetaResponse struct {
	Success bool      `json:"success"`
	Meta    *FileMeta `json:"meta,omitempty"`
	Error   string    `json:"error,omitempty"`
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
	http.HandleFunc("/api/v1/meta/", metaHandler)

	log.Printf("file-meta starting on :%s (root: %s)", port, storageRoot)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func metaHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reqPath := strings.TrimPrefix(r.URL.Path, "/api/v1/meta/")
	if p := r.URL.Query().Get("path"); p != "" {
		reqPath = p
	}

	if reqPath == "" {
		respondJSON(w, http.StatusBadRequest, MetaResponse{
			Success: false,
			Error:   "path required",
		})
		return
	}

	fullPath := filepath.Join(storageRoot, reqPath)

	// Security: prevent path traversal
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, filepath.Clean(storageRoot)) {
		respondJSON(w, http.StatusForbidden, MetaResponse{
			Success: false,
			Error:   "forbidden path",
		})
		return
	}

	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		respondJSON(w, http.StatusNotFound, MetaResponse{
			Success: false,
			Error:   "not found",
		})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, MetaResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	meta := &FileMeta{
		Path:    reqPath,
		Name:    info.Name(),
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime(),
	}

	// Calculate checksum for files (if requested and file is small enough)
	if !info.IsDir() && r.URL.Query().Get("checksum") == "true" && info.Size() < 100*1024*1024 {
		if hash, err := calculateChecksum(fullPath); err == nil {
			meta.Checksum = hash
		}
	}

	// Get mime type
	if !info.IsDir() {
		meta.Mime = getMimeType(info.Name())
	}

	respondJSON(w, http.StatusOK, MetaResponse{
		Success: true,
		Meta:    meta,
	})
}

func calculateChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func getMimeType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	mimes := map[string]string{
		".txt": "text/plain", ".html": "text/html", ".css": "text/css",
		".js": "application/javascript", ".json": "application/json",
		".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".gif": "image/gif", ".svg": "image/svg+xml", ".webp": "image/webp",
		".mp4": "video/mp4", ".webm": "video/webm", ".mkv": "video/x-matroska",
		".mp3": "audio/mpeg", ".wav": "audio/wav", ".flac": "audio/flac",
		".pdf": "application/pdf", ".zip": "application/zip",
		".go": "text/x-go", ".py": "text/x-python", ".rs": "text/x-rust",
	}
	if m, ok := mimes[ext]; ok {
		return m
	}
	return "application/octet-stream"
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
