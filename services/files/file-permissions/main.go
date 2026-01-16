package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	requestCount uint64
	startTime    = time.Now()
)

const dataPath = "/data"

type GetPermissionsRequest struct {
	Path string `json:"path"`
}

type SetPermissionsRequest struct {
	Path string `json:"path"`
	Mode string `json:"mode"` // Octal string like "0755" or "644"
}

type PermissionsResponse struct {
	Path       string `json:"path,omitempty"`
	Mode       string `json:"mode,omitempty"`
	ModeOctal  string `json:"mode_octal,omitempty"`
	Owner      uint32 `json:"owner,omitempty"`
	Group      uint32 `json:"group,omitempty"`
	Readable   bool   `json:"readable,omitempty"`
	Writable   bool   `json:"writable,omitempty"`
	Executable bool   `json:"executable,omitempty"`
	Error      string `json:"error,omitempty"`
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

func formatMode(mode os.FileMode) string {
	var result strings.Builder

	// File type
	if mode.IsDir() {
		result.WriteString("d")
	} else if mode&os.ModeSymlink != 0 {
		result.WriteString("l")
	} else {
		result.WriteString("-")
	}

	// Owner permissions
	if mode&0400 != 0 {
		result.WriteString("r")
	} else {
		result.WriteString("-")
	}
	if mode&0200 != 0 {
		result.WriteString("w")
	} else {
		result.WriteString("-")
	}
	if mode&0100 != 0 {
		result.WriteString("x")
	} else {
		result.WriteString("-")
	}

	// Group permissions
	if mode&0040 != 0 {
		result.WriteString("r")
	} else {
		result.WriteString("-")
	}
	if mode&0020 != 0 {
		result.WriteString("w")
	} else {
		result.WriteString("-")
	}
	if mode&0010 != 0 {
		result.WriteString("x")
	} else {
		result.WriteString("-")
	}

	// Other permissions
	if mode&0004 != 0 {
		result.WriteString("r")
	} else {
		result.WriteString("-")
	}
	if mode&0002 != 0 {
		result.WriteString("w")
	} else {
		result.WriteString("-")
	}
	if mode&0001 != 0 {
		result.WriteString("x")
	} else {
		result.WriteString("-")
	}

	return result.String()
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Method not allowed"})
		return
	}

	var req GetPermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.Path == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Path is required"})
		return
	}

	cleanPath := filepath.Clean(req.Path)
	if strings.Contains(cleanPath, "..") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Invalid path"})
		return
	}

	fullPath := filepath.Join(dataPath, cleanPath)

	info, err := os.Stat(fullPath)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "File not found: " + err.Error()})
		return
	}

	mode := info.Mode()
	perm := mode.Perm()

	var uid, gid uint32
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid = stat.Uid
		gid = stat.Gid
	}

	// Check current process permissions
	readable := true
	writable := true
	executable := true

	if _, err := os.Open(fullPath); err != nil {
		readable = false
	}
	if f, err := os.OpenFile(fullPath, os.O_WRONLY, 0); err != nil {
		writable = false
	} else {
		f.Close()
	}
	if !info.IsDir() && perm&0111 == 0 {
		executable = false
	}

	json.NewEncoder(w).Encode(PermissionsResponse{
		Path:       req.Path,
		Mode:       formatMode(mode),
		ModeOctal:  fmt.Sprintf("%04o", perm),
		Owner:      uid,
		Group:      gid,
		Readable:   readable,
		Writable:   writable,
		Executable: executable,
	})
}

func setHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCount, 1)
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Method not allowed"})
		return
	}

	var req SetPermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Invalid JSON: " + err.Error()})
		return
	}

	if req.Path == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Path is required"})
		return
	}

	if req.Mode == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Mode is required"})
		return
	}

	cleanPath := filepath.Clean(req.Path)
	if strings.Contains(cleanPath, "..") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Invalid path"})
		return
	}

	fullPath := filepath.Join(dataPath, cleanPath)

	// Parse mode
	modeStr := req.Mode
	if !strings.HasPrefix(modeStr, "0") {
		modeStr = "0" + modeStr
	}

	mode, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Invalid mode format: " + err.Error()})
		return
	}

	if err := os.Chmod(fullPath, os.FileMode(mode)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(PermissionsResponse{Error: "Failed to set permissions: " + err.Error()})
		return
	}

	// Return updated permissions
	info, _ := os.Stat(fullPath)
	perm := info.Mode().Perm()

	json.NewEncoder(w).Encode(PermissionsResponse{
		Path:      req.Path,
		Mode:      formatMode(info.Mode()),
		ModeOctal: fmt.Sprintf("%04o", perm),
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(HealthResponse{
		Status:    "healthy",
		Service:   "file-permissions",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MetricsResponse{
		Uptime:   time.Since(startTime).String(),
		Requests: atomic.LoadUint64(&requestCount),
		Service:  "file-permissions",
	})
}

func main() {
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Printf("Warning: Could not create data directory: %v", err)
	}

	http.HandleFunc("/get", getHandler)
	http.HandleFunc("/set", setHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)

	fmt.Println("file-permissions service starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
