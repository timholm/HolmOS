package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// Job represents a conversion job
type Job struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"` // pending, processing, completed, failed
	InputPath    string    `json:"input_path"`
	OutputPath   string    `json:"output_path"`
	OutputFormat string    `json:"output_format"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// ConvertRequest represents a conversion request
type ConvertRequest struct {
	InputPath    string `json:"input_path"`
	OutputFormat string `json:"output_format"`
	OutputPath   string `json:"output_path,omitempty"`
}

// FormatInfo represents supported format information
type FormatInfo struct {
	Format      string   `json:"format"`
	Extensions  []string `json:"extensions"`
	Description string   `json:"description"`
}

// JobStore manages conversion jobs
type JobStore struct {
	jobs map[string]*Job
	mu   sync.RWMutex
}

var (
	jobStore = &JobStore{
		jobs: make(map[string]*Job),
	}
	supportedFormats = map[string]FormatInfo{
		"png": {
			Format:      "png",
			Extensions:  []string{".png"},
			Description: "Portable Network Graphics",
		},
		"jpg": {
			Format:      "jpg",
			Extensions:  []string{".jpg", ".jpeg"},
			Description: "JPEG Image",
		},
		"webp": {
			Format:      "webp",
			Extensions:  []string{".webp"},
			Description: "WebP Image",
		},
		"gif": {
			Format:      "gif",
			Extensions:  []string{".gif"},
			Description: "Graphics Interchange Format",
		},
	}
)

func main() {
	router := mux.NewRouter()

	router.HandleFunc("/health", healthHandler).Methods("GET")
	router.HandleFunc("/formats", formatsHandler).Methods("GET")
	router.HandleFunc("/convert", convertHandler).Methods("POST")
	router.HandleFunc("/jobs/{id}", jobStatusHandler).Methods("GET")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("file-convert service starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Check if ImageMagick is available
	cmd := exec.Command("convert", "--version")
	err := cmd.Run()

	status := "healthy"
	if err != nil {
		status = "degraded"
	}

	response := map[string]interface{}{
		"status":    status,
		"service":   "file-convert",
		"timestamp": time.Now().UTC(),
		"imagemagick_available": err == nil,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func formatsHandler(w http.ResponseWriter, r *http.Request) {
	formats := make([]FormatInfo, 0, len(supportedFormats))
	for _, f := range supportedFormats {
		formats = append(formats, f)
	}

	response := map[string]interface{}{
		"formats": formats,
		"conversions": map[string][]string{
			"image_to_image": {"png", "jpg", "webp", "gif"},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func convertHandler(w http.ResponseWriter, r *http.Request) {
	var req ConvertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate input
	if req.InputPath == "" {
		http.Error(w, `{"error": "input_path is required"}`, http.StatusBadRequest)
		return
	}

	if req.OutputFormat == "" {
		http.Error(w, `{"error": "output_format is required"}`, http.StatusBadRequest)
		return
	}

	// Normalize output format
	outputFormat := strings.ToLower(strings.TrimPrefix(req.OutputFormat, "."))
	if _, ok := supportedFormats[outputFormat]; !ok {
		http.Error(w, `{"error": "unsupported output format"}`, http.StatusBadRequest)
		return
	}

	// Check if input file exists
	if _, err := os.Stat(req.InputPath); os.IsNotExist(err) {
		http.Error(w, `{"error": "input file not found"}`, http.StatusNotFound)
		return
	}

	// Generate output path if not provided
	outputPath := req.OutputPath
	if outputPath == "" {
		ext := filepath.Ext(req.InputPath)
		baseName := strings.TrimSuffix(filepath.Base(req.InputPath), ext)
		outputDir := filepath.Dir(req.InputPath)
		outputPath = filepath.Join(outputDir, fmt.Sprintf("%s_converted.%s", baseName, outputFormat))
	}

	// Create job
	job := &Job{
		ID:           uuid.New().String(),
		Status:       "pending",
		InputPath:    req.InputPath,
		OutputPath:   outputPath,
		OutputFormat: outputFormat,
		CreatedAt:    time.Now().UTC(),
	}

	jobStore.mu.Lock()
	jobStore.jobs[job.ID] = job
	jobStore.mu.Unlock()

	// Process job asynchronously
	go processJob(job)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(job)
}

func jobStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	jobStore.mu.RLock()
	job, exists := jobStore.jobs[jobID]
	jobStore.mu.RUnlock()

	if !exists {
		http.Error(w, `{"error": "job not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func processJob(job *Job) {
	jobStore.mu.Lock()
	job.Status = "processing"
	jobStore.mu.Unlock()

	// Use ImageMagick convert command
	cmd := exec.Command("convert", job.InputPath, job.OutputPath)
	output, err := cmd.CombinedOutput()

	jobStore.mu.Lock()
	defer jobStore.mu.Unlock()

	now := time.Now().UTC()
	job.CompletedAt = &now

	if err != nil {
		job.Status = "failed"
		job.Error = fmt.Sprintf("conversion failed: %v - %s", err, string(output))
		log.Printf("Job %s failed: %s", job.ID, job.Error)
		return
	}

	job.Status = "completed"
	log.Printf("Job %s completed: %s -> %s", job.ID, job.InputPath, job.OutputPath)
}
