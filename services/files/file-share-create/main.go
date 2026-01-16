package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	dataPath   = "/data"
	sharesFile = "/data/.shares/shares.json"
)

type ShareRequest struct {
	Path       string `json:"path"`
	ExpiresIn  int64  `json:"expires_in,omitempty"`  // Seconds until expiration (0 = no expiry)
	MaxAccess  int    `json:"max_access,omitempty"` // Max number of accesses (0 = unlimited)
	Password   string `json:"password,omitempty"`   // Optional password protection
}

type Share struct {
	Token       string    `json:"token"`
	Path        string    `json:"path"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	MaxAccess   int       `json:"max_access,omitempty"`
	AccessCount int       `json:"access_count"`
	Password    string    `json:"password,omitempty"`
}

type ShareResponse struct {
	Token     string     `json:"token,omitempty"`
	Path      string     `json:"path,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	MaxAccess int        `json:"max_access,omitempty"`
	Error     string     `json:"error,omitempty"`
}

type ListSharesResponse struct {
	Shares []Share `json:"shares,omitempty"`
	Error  string  `json:"error,omitempty"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

type MetricsResponse struct {
	Uptime       string `json:"uptime"`
	Requests     uint64 `json:"requests"`
	ActiveShares int    `json:"active_shares"`
	Service      string `json:"service"`
}

type ShareStore struct {
	Shares map[string]Share `json:"shares"`
	mu     sync.RWMutex
}

var store = &ShareStore{
	Shares: make(map[string]Share),
}

func generateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
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

func (s *ShareStore) Add(share Share) {
	s.mu.Lock()
	s.Shares[share.Token] = share
	s.mu.Unlock()
	s.Save()
}

func (s *ShareStore) Delete(token string) {
	s.mu.Lock()
	delete(s.Shares, token)
	s.mu.Unlock()
	s.Save()
}

func (s *ShareStore) List() []Share {
	s.mu.RLock()
	defer s.mu.RUnlock()

	shares := make([]Share, 0, len(s.Shares))
	now := time.Now()

	for _, share := range s.Shares {
		// Filter out expired shares
		if share.ExpiresAt != nil && share.ExpiresAt.Before(now) {
			continue
		}
		// Don't expose password in list
		shareCopy := share
		if shareCopy.Password != "" {
			shareCopy.Password = "***"
		}
		shares = append(shares, shareCopy)
	}

	return shares
}

func (s *ShareStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Shares)
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(ShareResponse{Error: "Method not allowed"})
		return
	}

	var req ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ShareResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.Path == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ShareResponse{Error: "Path is required"})
		return
	}

	// Sanitize path
	cleanPath := filepath.Clean(req.Path)
	if strings.Contains(cleanPath, "..") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ShareResponse{Error: "Invalid path"})
		return
	}

	fullPath := filepath.Join(dataPath, cleanPath)

	// Verify file exists
	if _, err := os.Stat(fullPath); err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ShareResponse{Error: "File not found: " + err.Error()})
		return
	}

	share := Share{
		Token:     generateToken(),
		Path:      cleanPath,
		CreatedAt: time.Now(),
		MaxAccess: req.MaxAccess,
		Password:  req.Password,
	}

	if req.ExpiresIn > 0 {
		expires := time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
		share.ExpiresAt = &expires
	}

	store.Add(share)

	json.NewEncoder(w).Encode(ShareResponse{
		Token:     share.Token,
		Path:      share.Path,
		ExpiresAt: share.ExpiresAt,
		MaxAccess: share.MaxAccess,
	})
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		json.NewEncoder(w).Encode(ShareResponse{Error: "Method not allowed"})
		return
	}

	var req struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ShareResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.Token == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ShareResponse{Error: "Token is required"})
		return
	}

	store.Delete(req.Token)

	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "application/json")

	shares := store.List()
	json.NewEncoder(w).Encode(ListSharesResponse{Shares: shares})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{
		Status:    "healthy",
		Service:   "file-share-create",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MetricsResponse{
		Uptime:       time.Since(startTime).String(),
		Requests:     atomic.LoadUint64(&requestCount),
		ActiveShares: store.Count(),
		Service:      "file-share-create",
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

	http.HandleFunc("/create", createHandler)
	http.HandleFunc("/delete", deleteHandler)
	http.HandleFunc("/list", listHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)

	fmt.Println("file-share-create service starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
