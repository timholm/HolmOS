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
	"sync"
	"sync/atomic"
	"time"
)

var (
	requestCount uint64
	startTime    = time.Now()
)

const (
	dataPath      = "/data"
	sharesFile    = "/data/.shares/shares.json"
	maxFileSize   = 50 * 1024 * 1024 // 50MB max for inline content
)

type Share struct {
	Token       string     `json:"token"`
	Path        string     `json:"path"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	MaxAccess   int        `json:"max_access,omitempty"`
	AccessCount int        `json:"access_count"`
	Password    string     `json:"password,omitempty"`
}

type ValidateRequest struct {
	Token    string `json:"token"`
	Password string `json:"password,omitempty"`
}

type ValidateResponse struct {
	Valid       bool   `json:"valid"`
	Path        string `json:"path,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	Size        int64  `json:"size,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
	IsDir       bool   `json:"is_dir,omitempty"`
	AccessCount int    `json:"access_count,omitempty"`
	MaxAccess   int    `json:"max_access,omitempty"`
	Error       string `json:"error,omitempty"`
}

type DownloadResponse struct {
	FileName string `json:"file_name,omitempty"`
	Size     int64  `json:"size,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Content  string `json:"content,omitempty"` // Base64 encoded
	Error    string `json:"error,omitempty"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

type MetricsResponse struct {
	Uptime   string `json:"uptime"`
	Requests uint64 `json:"requests"`
	Service  string `json:"service"`
}

type ShareStore struct {
	Shares map[string]Share `json:"shares"`
	mu     sync.RWMutex
}

var store = &ShareStore{
	Shares: make(map[string]Share),
}

func (s *ShareStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(sharesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.Shares)
}

func (s *ShareStore) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.Shares, "", "  ")
	s.mu.RUnlock()

	if err != nil {
		return err
	}

	dir := filepath.Dir(sharesFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(sharesFile, data, 0600)
}

func (s *ShareStore) Get(token string) (Share, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	share, ok := s.Shares[token]
	return share, ok
}

func (s *ShareStore) IncrementAccess(token string) {
	s.mu.Lock()
	if share, ok := s.Shares[token]; ok {
		share.AccessCount++
		s.Shares[token] = share
	}
	s.mu.Unlock()
	s.Save()
}

func (s *ShareStore) Delete(token string) {
	s.mu.Lock()
	delete(s.Shares, token)
	s.mu.Unlock()
	s.Save()
}

func getMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	mimeTypes := map[string]string{
		".txt":  "text/plain",
		".html": "text/html",
		".css":  "text/css",
		".js":   "application/javascript",
		".json": "application/json",
		".xml":  "application/xml",
		".pdf":  "application/pdf",
		".zip":  "application/zip",
		".gz":   "application/gzip",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".svg":  "image/svg+xml",
		".mp3":  "audio/mpeg",
		".mp4":  "video/mp4",
		".webm": "video/webm",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}

	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

func validateShare(token, password string) (Share, string) {
	// Reload shares to get latest data
	store.Load()

	share, ok := store.Get(token)
	if !ok {
		return Share{}, "Invalid share token"
	}

	// Check expiration
	if share.ExpiresAt != nil && share.ExpiresAt.Before(time.Now()) {
		store.Delete(token)
		return Share{}, "Share has expired"
	}

	// Check max access
	if share.MaxAccess > 0 && share.AccessCount >= share.MaxAccess {
		store.Delete(token)
		return Share{}, "Share access limit reached"
	}

	// Check password
	if share.Password != "" && share.Password != password {
		return Share{}, "Invalid password"
	}

	return share, ""
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(ValidateResponse{Error: "Method not allowed"})
		return
	}

	var req ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ValidateResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.Token == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ValidateResponse{Error: "Token is required"})
		return
	}

	share, errMsg := validateShare(req.Token, req.Password)
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ValidateResponse{Valid: false, Error: errMsg})
		return
	}

	fullPath := filepath.Join(dataPath, share.Path)
	info, err := os.Stat(fullPath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ValidateResponse{Valid: false, Error: "File not found"})
		return
	}

	json.NewEncoder(w).Encode(ValidateResponse{
		Valid:       true,
		Path:        share.Path,
		FileName:    filepath.Base(share.Path),
		Size:        info.Size(),
		MimeType:    getMimeType(share.Path),
		IsDir:       info.IsDir(),
		AccessCount: share.AccessCount,
		MaxAccess:   share.MaxAccess,
	})
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(DownloadResponse{Error: "Method not allowed"})
		return
	}

	var req ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DownloadResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.Token == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DownloadResponse{Error: "Token is required"})
		return
	}

	share, errMsg := validateShare(req.Token, req.Password)
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(DownloadResponse{Error: errMsg})
		return
	}

	fullPath := filepath.Join(dataPath, share.Path)
	info, err := os.Stat(fullPath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(DownloadResponse{Error: "File not found"})
		return
	}

	if info.IsDir() {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DownloadResponse{Error: "Cannot download directory"})
		return
	}

	if info.Size() > maxFileSize {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(DownloadResponse{Error: "File too large for download"})
		return
	}

	file, err := os.Open(fullPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(DownloadResponse{Error: "Failed to open file: " + err.Error()})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(DownloadResponse{Error: "Failed to read file: " + err.Error()})
		return
	}

	// Increment access count after successful download
	store.IncrementAccess(req.Token)

	json.NewEncoder(w).Encode(DownloadResponse{
		FileName: filepath.Base(share.Path),
		Size:     info.Size(),
		MimeType: getMimeType(share.Path),
		Content:  base64.StdEncoding.EncodeToString(data),
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{
		Status:    "healthy",
		Service:   "file-share-validate",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MetricsResponse{
		Uptime:   time.Since(startTime).String(),
		Requests: atomic.LoadUint64(&requestCount),
		Service:  "file-share-validate",
	})
}

func main() {
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Printf("Warning: Could not create data directory: %v", err)
	}

	// Load existing shares
	if err := store.Load(); err != nil {
		log.Printf("Warning: Could not load shares: %v", err)
	}

	http.HandleFunc("/validate", validateHandler)
	http.HandleFunc("/download", downloadHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)

	fmt.Println("file-share-validate service starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
