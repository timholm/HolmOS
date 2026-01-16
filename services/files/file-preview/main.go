package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var (
	requestCount uint64
	startTime    = time.Now()
)

const (
	dataPath      = "/data"
	maxPreviewSize = 10 * 1024 * 1024 // 10MB max preview
)

type PreviewRequest struct {
	Path string `json:"path"`
	Lines int   `json:"lines,omitempty"` // For text files, limit lines
}

type PreviewResponse struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	Content  string `json:"content,omitempty"`  // For text files
	Base64   string `json:"base64,omitempty"`   // For binary files (images, PDFs)
	MimeType string `json:"mime_type"`
	Error    string `json:"error,omitempty"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

type MetricsResponse struct {
	Uptime       string  `json:"uptime"`
	Requests     uint64  `json:"requests"`
	Service      string  `json:"service"`
}

func getFileType(path string) (string, string) {
	ext := strings.ToLower(filepath.Ext(path))

	textExts := map[string]string{
		".txt": "text/plain", ".md": "text/markdown", ".json": "application/json",
		".xml": "application/xml", ".html": "text/html", ".css": "text/css",
		".js": "application/javascript", ".go": "text/x-go", ".py": "text/x-python",
		".yaml": "text/yaml", ".yml": "text/yaml", ".sh": "text/x-shellscript",
		".log": "text/plain", ".csv": "text/csv", ".ts": "text/typescript",
		".java": "text/x-java", ".c": "text/x-c", ".cpp": "text/x-c++",
		".h": "text/x-c", ".rs": "text/x-rust", ".rb": "text/x-ruby",
	}

	imageExts := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
		".gif": "image/gif", ".bmp": "image/bmp", ".webp": "image/webp",
		".svg": "image/svg+xml", ".ico": "image/x-icon",
	}

	if mime, ok := textExts[ext]; ok {
		return "text", mime
	}
	if mime, ok := imageExts[ext]; ok {
		return "image", mime
	}
	if ext == ".pdf" {
		return "pdf", "application/pdf"
	}

	return "binary", "application/octet-stream"
}

func previewHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(PreviewResponse{Error: "Method not allowed, use GET"})
		return
	}

	// Get path from query parameter: GET /preview?path=/some/file.txt
	path := r.URL.Query().Get("path")
	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PreviewResponse{Error: "Missing 'path' query parameter"})
		return
	}

	// Optional lines parameter
	linesParam := r.URL.Query().Get("lines")
	var lines int
	if linesParam != "" {
		fmt.Sscanf(linesParam, "%d", &lines)
	}

	req := PreviewRequest{Path: path, Lines: lines}

	if req.Path == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PreviewResponse{Error: "Path is required"})
		return
	}

	// Sanitize path
	cleanPath := filepath.Clean(req.Path)
	if strings.Contains(cleanPath, "..") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PreviewResponse{Error: "Invalid path"})
		return
	}

	fullPath := filepath.Join(dataPath, cleanPath)

	info, err := os.Stat(fullPath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(PreviewResponse{Error: "File not found: " + err.Error()})
		return
	}

	if info.IsDir() {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PreviewResponse{Error: "Cannot preview directory"})
		return
	}

	if info.Size() > maxPreviewSize {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PreviewResponse{Error: "File too large for preview"})
		return
	}

	fileType, mimeType := getFileType(fullPath)

	response := PreviewResponse{
		Path:     req.Path,
		Type:     fileType,
		Size:     info.Size(),
		MimeType: mimeType,
	}

	file, err := os.Open(fullPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(PreviewResponse{Error: "Failed to open file: " + err.Error()})
		return
	}
	defer file.Close()

	switch fileType {
	case "text":
		data, err := io.ReadAll(file)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(PreviewResponse{Error: "Failed to read file: " + err.Error()})
			return
		}
		content := string(data)

		// Limit lines if requested
		if req.Lines > 0 {
			lines := strings.Split(content, "\n")
			if len(lines) > req.Lines {
				lines = lines[:req.Lines]
				content = strings.Join(lines, "\n") + "\n... (truncated)"
			}
		}
		response.Content = content

	case "image", "pdf":
		data, err := io.ReadAll(file)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(PreviewResponse{Error: "Failed to read file: " + err.Error()})
			return
		}
		response.Base64 = base64.StdEncoding.EncodeToString(data)

	default:
		// For unknown binary files, return first 1KB as base64
		data := make([]byte, 1024)
		n, _ := file.Read(data)
		response.Base64 = base64.StdEncoding.EncodeToString(data[:n])
	}

	json.NewEncoder(w).Encode(response)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{
		Status:    "healthy",
		Service:   "file-preview",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MetricsResponse{
		Uptime:   time.Since(startTime).String(),
		Requests: atomic.LoadUint64(&requestCount),
		Service:  "file-preview",
	})
}

func main() {
	// Ensure data directory exists
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Printf("Warning: Could not create data directory: %v", err)
	}

	http.HandleFunc("/preview", previewHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)

	fmt.Println("file-preview service starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
