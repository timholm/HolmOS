package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	db          *sql.DB
	sseClients  = make(map[chan []byte]bool)
	sseMutex    sync.RWMutex
)

// Service URLs
var (
	uploadEpubURL     = getEnv("UPLOAD_EPUB_URL", "http://audiobook-upload-epub:8080")
	uploadTxtURL      = getEnv("UPLOAD_TXT_URL", "http://audiobook-upload-txt:8080")
	parseEpubURL      = getEnv("PARSE_EPUB_URL", "http://audiobook-parse-epub:8080")
	chunkTextURL      = getEnv("CHUNK_TEXT_URL", "http://audiobook-chunk-text:8080")
	ttsConvertURL     = getEnv("TTS_CONVERT_URL", "http://audiobook-tts-convert:8080")
	audioConcatURL    = getEnv("AUDIO_CONCAT_URL", "http://audiobook-audio-concat:8080")
	audioNormalizeURL = getEnv("AUDIO_NORMALIZE_URL", "http://audiobook-audio-normalize:8080")
	webCallbackURL    = getEnv("WEB_CALLBACK_URL", "http://audiobook-web:8080")
)

const (
	dataDir      = "/data/audiobooks"
	uploadsDir   = "/data/audiobooks/uploads"
	parsedDir    = "/data/audiobooks/parsed"
	chunksDir    = "/data/audiobooks/chunks"
	audioDir     = "/data/audiobooks/audio"
	outputDir    = "/data/audiobooks/output"
)

type Job struct {
	ID          int       `json:"id"`
	JobID       string    `json:"job_id"`
	Filename    string    `json:"filename"`
	FileType    string    `json:"file_type"`
	FilePath    string    `json:"file_path"`
	FileSize    int64     `json:"file_size"`
	Status      string    `json:"status"`
	Progress    int       `json:"progress"`
	CurrentStep string    `json:"current_step"`
	OutputPath  string    `json:"output_path"`
	Duration    int       `json:"duration"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Error       string    `json:"error,omitempty"`
}

type Audiobook struct {
	ID          int       `json:"id"`
	JobID       string    `json:"job_id"`
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	Duration    int       `json:"duration"`
	FilePath    string    `json:"file_path"`
	CoverPath   string    `json:"cover_path"`
	FileSize    int64     `json:"file_size"`
	CreatedAt   time.Time `json:"created_at"`
}

type SSEMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Pipeline request/response types
type ParseRequest struct {
	JobID  string `json:"job_id"`
	Input  string `json:"input"`
	Output string `json:"output"`
}

type ParseResponse struct {
	JobID  string `json:"job_id"`
	Output string `json:"output"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type ChunkRequest struct {
	JobID     string `json:"job_id"`
	Input     string `json:"input"`
	OutputDir string `json:"output_dir"`
	ChunkSize int    `json:"chunk_size"`
}

type ChunkResponse struct {
	JobID      string   `json:"job_id"`
	ChunkFiles []string `json:"chunk_files"`
	ChunkCount int      `json:"chunk_count"`
	Status     string   `json:"status"`
	Error      string   `json:"error,omitempty"`
}

type TTSRequest struct {
	JobID     string `json:"job_id"`
	Input     string `json:"input"`
	Output    string `json:"output"`
	Voice     string `json:"voice,omitempty"`
}

type TTSResponse struct {
	JobID    string `json:"job_id"`
	Output   string `json:"output"`
	Duration int    `json:"duration"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type ConcatRequest struct {
	JobID  string   `json:"job_id"`
	Inputs []string `json:"inputs"`
	Output string   `json:"output"`
}

type ConcatResponse struct {
	JobID    string `json:"job_id"`
	Output   string `json:"output"`
	Duration int    `json:"duration"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type NormalizeRequest struct {
	JobID  string `json:"job_id"`
	Input  string `json:"input"`
	Output string `json:"output"`
}

type NormalizeResponse struct {
	JobID    string `json:"job_id"`
	Output   string `json:"output"`
	Duration int    `json:"duration"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

func main() {
	// Ensure directories exist
	for _, dir := range []string{dataDir, uploadsDir, parsedDir, chunksDir, audioDir, outputDir} {
		os.MkdirAll(dir, 0755)
	}

	initDB()

	// API endpoints
	http.HandleFunc("/", serveUI)
	http.HandleFunc("/api/upload/epub", handleUploadEpub)
	http.HandleFunc("/api/upload/txt", handleUploadTxt)
	http.HandleFunc("/api/jobs", handleGetJobs)
	http.HandleFunc("/api/jobs/", handleJobActions)
	http.HandleFunc("/api/library", handleLibrary)
	http.HandleFunc("/api/library/", handleLibraryActions)
	http.HandleFunc("/api/stream/", handleStreamAudio)
	http.HandleFunc("/api/download/", handleDownload)
	http.HandleFunc("/api/events", handleSSE)
	http.HandleFunc("/api/progress", handleProgressUpdate)
	http.HandleFunc("/health", handleHealth)

	// Start background job processor
	go processJobsLoop()

	log.Println("Audiobook Web Orchestrator starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func initDB() {
	dbHost := getEnv("POSTGRES_HOST", "postgres")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("POSTGRES_USER", "postgres")
	dbPass := getEnv("POSTGRES_PASSWORD", "postgres123")
	dbName := getEnv("POSTGRES_DB", "holm")

	// Handle case where POSTGRES_HOST might include tcp:// prefix from K8s service
	if strings.HasPrefix(dbHost, "tcp://") {
		dbHost = strings.TrimPrefix(dbHost, "tcp://")
		if idx := strings.Index(dbHost, ":"); idx != -1 {
			dbHost = dbHost[:idx]
		}
	}

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName)

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Warning: Could not connect to database: %v", err)
		return
	}

	if err = db.Ping(); err != nil {
		log.Printf("Warning: Database ping failed: %v", err)
		return
	}

	log.Println("Connected to PostgreSQL database")
	ensureTables()
}

func ensureTables() {
	// Jobs table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS audiobook_jobs (
			id SERIAL PRIMARY KEY,
			job_id VARCHAR(64) UNIQUE NOT NULL,
			filename VARCHAR(255) NOT NULL,
			file_type VARCHAR(32) NOT NULL,
			file_path TEXT NOT NULL,
			file_size BIGINT NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			progress INTEGER DEFAULT 0,
			current_step VARCHAR(64) DEFAULT '',
			output_path TEXT DEFAULT '',
			duration INTEGER DEFAULT 0,
			error TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Printf("Warning: Could not create audiobook_jobs table: %v", err)
	}

	// Library table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS audiobook_library (
			id SERIAL PRIMARY KEY,
			job_id VARCHAR(64) UNIQUE,
			title VARCHAR(255) NOT NULL,
			author VARCHAR(255) DEFAULT 'Unknown',
			duration INTEGER DEFAULT 0,
			file_path TEXT NOT NULL,
			cover_path TEXT DEFAULT '',
			file_size BIGINT DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Printf("Warning: Could not create audiobook_library table: %v", err)
	}

	log.Println("Database tables initialized")
}

func handleUploadEpub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(100 << 20) // 100MB
	if err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Forward to upload-epub service
	resp, err := forwardUpload(uploadEpubURL+"/upload", file, header)
	if err != nil {
		http.Error(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast update to SSE clients
	broadcastSSE(SSEMessage{Type: "job_created", Data: resp})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleUploadTxt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseMultipartForm(100 << 20)
	if err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to get file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	resp, err := forwardUpload(uploadTxtURL+"/upload", file, header)
	if err != nil {
		http.Error(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	broadcastSSE(SSEMessage{Type: "job_created", Data: resp})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func forwardUpload(url string, file multipart.File, header *multipart.FileHeader) (map[string]interface{}, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", header.Filename)
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return nil, err
	}
	writer.Close()

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload service returned %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}

func handleGetJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobs := getJobs()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func getJobs() []Job {
	jobs := []Job{}
	if db == nil {
		return jobs
	}

	rows, err := db.Query(`
		SELECT id, job_id, filename, file_type, file_path, file_size,
		       status, progress, COALESCE(current_step, ''), COALESCE(output_path, ''),
		       COALESCE(duration, 0), COALESCE(error, ''), created_at, updated_at
		FROM audiobook_jobs
		ORDER BY created_at DESC
		LIMIT 50
	`)
	if err != nil {
		log.Printf("Error getting jobs: %v", err)
		return jobs
	}
	defer rows.Close()

	for rows.Next() {
		var job Job
		err := rows.Scan(&job.ID, &job.JobID, &job.Filename, &job.FileType,
			&job.FilePath, &job.FileSize, &job.Status, &job.Progress,
			&job.CurrentStep, &job.OutputPath, &job.Duration, &job.Error,
			&job.CreatedAt, &job.UpdatedAt)
		if err != nil {
			log.Printf("Error scanning job: %v", err)
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs
}

func handleJobActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	parts := strings.Split(path, "/")
	jobID := parts[0]

	if len(parts) > 1 && parts[1] == "retry" && r.Method == http.MethodPost {
		retryJob(w, jobID)
		return
	}

	if len(parts) > 1 && parts[1] == "cancel" && r.Method == http.MethodPost {
		cancelJob(w, jobID)
		return
	}

	if r.Method == http.MethodDelete {
		deleteJob(w, jobID)
		return
	}

	// Get single job
	job := getJobByID(jobID)
	if job == nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func getJobByID(jobID string) *Job {
	if db == nil {
		return nil
	}

	var job Job
	err := db.QueryRow(`
		SELECT id, job_id, filename, file_type, file_path, file_size,
		       status, progress, COALESCE(current_step, ''), COALESCE(output_path, ''),
		       COALESCE(duration, 0), COALESCE(error, ''), created_at, updated_at
		FROM audiobook_jobs WHERE job_id = $1
	`, jobID).Scan(&job.ID, &job.JobID, &job.Filename, &job.FileType,
		&job.FilePath, &job.FileSize, &job.Status, &job.Progress,
		&job.CurrentStep, &job.OutputPath, &job.Duration, &job.Error,
		&job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return nil
	}
	return &job
}

func retryJob(w http.ResponseWriter, jobID string) {
	if db == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	_, err := db.Exec(`
		UPDATE audiobook_jobs
		SET status = 'pending', progress = 0, current_step = '', error = '', updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		http.Error(w, "Failed to retry job", http.StatusInternalServerError)
		return
	}

	broadcastSSE(SSEMessage{Type: "job_updated", Data: map[string]string{"job_id": jobID, "status": "pending"}})
	w.WriteHeader(http.StatusOK)
}

func cancelJob(w http.ResponseWriter, jobID string) {
	if db == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	_, err := db.Exec(`
		UPDATE audiobook_jobs
		SET status = 'cancelled', updated_at = NOW()
		WHERE job_id = $1 AND status IN ('pending', 'processing')
	`, jobID)
	if err != nil {
		http.Error(w, "Failed to cancel job", http.StatusInternalServerError)
		return
	}

	broadcastSSE(SSEMessage{Type: "job_updated", Data: map[string]string{"job_id": jobID, "status": "cancelled"}})
	w.WriteHeader(http.StatusOK)
}

func deleteJob(w http.ResponseWriter, jobID string) {
	if db == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	_, err := db.Exec(`DELETE FROM audiobook_jobs WHERE job_id = $1`, jobID)
	if err != nil {
		http.Error(w, "Failed to delete job", http.StatusInternalServerError)
		return
	}

	broadcastSSE(SSEMessage{Type: "job_deleted", Data: map[string]string{"job_id": jobID}})
	w.WriteHeader(http.StatusOK)
}

func handleLibrary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	audiobooks := getLibrary()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(audiobooks)
}

func getLibrary() []Audiobook {
	audiobooks := []Audiobook{}
	if db == nil {
		return audiobooks
	}

	rows, err := db.Query(`
		SELECT id, job_id, title, COALESCE(author, 'Unknown'), COALESCE(duration, 0),
		       file_path, COALESCE(cover_path, ''), COALESCE(file_size, 0), created_at
		FROM audiobook_library
		ORDER BY created_at DESC
	`)
	if err != nil {
		log.Printf("Error getting library: %v", err)
		return audiobooks
	}
	defer rows.Close()

	for rows.Next() {
		var ab Audiobook
		err := rows.Scan(&ab.ID, &ab.JobID, &ab.Title, &ab.Author, &ab.Duration,
			&ab.FilePath, &ab.CoverPath, &ab.FileSize, &ab.CreatedAt)
		if err != nil {
			log.Printf("Error scanning audiobook: %v", err)
			continue
		}
		audiobooks = append(audiobooks, ab)
	}

	return audiobooks
}

func handleLibraryActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/library/")
	parts := strings.Split(path, "/")
	id := parts[0]

	if r.Method == http.MethodDelete {
		deleteAudiobook(w, id)
		return
	}

	http.Error(w, "Not found", http.StatusNotFound)
}

func deleteAudiobook(w http.ResponseWriter, id string) {
	if db == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	_, err := db.Exec(`DELETE FROM audiobook_library WHERE id = $1`, id)
	if err != nil {
		http.Error(w, "Failed to delete audiobook", http.StatusInternalServerError)
		return
	}

	broadcastSSE(SSEMessage{Type: "library_updated", Data: nil})
	w.WriteHeader(http.StatusOK)
}

func handleStreamAudio(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimPrefix(r.URL.Path, "/api/stream/")

	job := getJobByID(jobID)
	if job == nil || job.OutputPath == "" {
		http.Error(w, "Audio not found", http.StatusNotFound)
		return
	}

	// Check if file exists
	if _, err := os.Stat(job.OutputPath); os.IsNotExist(err) {
		http.Error(w, "Audio file not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, job.OutputPath)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimPrefix(r.URL.Path, "/api/download/")

	job := getJobByID(jobID)
	if job == nil || job.OutputPath == "" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Check if file exists
	if _, err := os.Stat(job.OutputPath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	filename := strings.TrimSuffix(job.Filename, filepath.Ext(job.Filename)) + ".mp3"
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Type", "audio/mpeg")
	http.ServeFile(w, r, job.OutputPath)
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	clientChan := make(chan []byte, 10)

	sseMutex.Lock()
	sseClients[clientChan] = true
	sseMutex.Unlock()

	defer func() {
		sseMutex.Lock()
		delete(sseClients, clientChan)
		sseMutex.Unlock()
		close(clientChan)
	}()

	// Send initial data
	jobs := getJobs()
	data, _ := json.Marshal(SSEMessage{Type: "initial", Data: jobs})
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	// Keep connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-clientChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func broadcastSSE(msg SSEMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	sseMutex.RLock()
	defer sseMutex.RUnlock()

	for clientChan := range sseClients {
		select {
		case clientChan <- data:
		default:
			// Client buffer full, skip
		}
	}
}

func handleProgressUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var update struct {
		JobID       string `json:"job_id"`
		Progress    int    `json:"progress"`
		Status      string `json:"status"`
		CurrentStep string `json:"current_step"`
		OutputPath  string `json:"output_path"`
		Duration    int    `json:"duration"`
		Error       string `json:"error"`
	}

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if db != nil {
		_, err := db.Exec(`
			UPDATE audiobook_jobs
			SET progress = $1, status = $2, current_step = $3, output_path = $4,
			    duration = $5, error = $6, updated_at = NOW()
			WHERE job_id = $7
		`, update.Progress, update.Status, update.CurrentStep, update.OutputPath,
			update.Duration, update.Error, update.JobID)
		if err != nil {
			log.Printf("Error updating job: %v", err)
		}

		// If completed, add to library
		if update.Status == "completed" && update.OutputPath != "" {
			job := getJobByID(update.JobID)
			if job != nil {
				addToLibrary(job)
			}
		}
	}

	broadcastSSE(SSEMessage{Type: "job_updated", Data: update})
	w.WriteHeader(http.StatusOK)
}

func addToLibrary(job *Job) {
	title := strings.TrimSuffix(job.Filename, filepath.Ext(job.Filename))

	var fileSize int64
	if info, err := os.Stat(job.OutputPath); err == nil {
		fileSize = info.Size()
	}

	_, err := db.Exec(`
		INSERT INTO audiobook_library (job_id, title, author, duration, file_path, file_size)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (job_id) DO UPDATE SET
			title = EXCLUDED.title,
			duration = EXCLUDED.duration,
			file_path = EXCLUDED.file_path,
			file_size = EXCLUDED.file_size
	`, job.JobID, title, "Unknown", job.Duration, job.OutputPath, fileSize)
	if err != nil {
		log.Printf("Error adding to library: %v", err)
	}

	broadcastSSE(SSEMessage{Type: "library_updated", Data: nil})
}

// Pipeline orchestration
func processJobsLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		processJobs()
	}
}

func processJobs() {
	if db == nil {
		return
	}

	// Get pending jobs
	rows, err := db.Query(`
		SELECT job_id, filename, file_type, file_path
		FROM audiobook_jobs
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT 1
	`)
	if err != nil {
		log.Printf("Error fetching pending jobs: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var jobID, filename, fileType, filePath string
		if err := rows.Scan(&jobID, &filename, &fileType, &filePath); err != nil {
			log.Printf("Error scanning job: %v", err)
			continue
		}

		// Process this job
		go processPipeline(jobID, filename, fileType, filePath)
	}
}

func processPipeline(jobID, filename, fileType, filePath string) {
	log.Printf("[%s] Starting pipeline for %s (%s)", jobID, filename, fileType)

	// Mark as processing
	updateJobStatus(jobID, "processing", 0, "Starting pipeline", "", 0, "")

	var textPath string
	var err error

	// Step 1: Parse (for EPUB) or use directly (for TXT)
	if fileType == "epub" {
		updateJobStatus(jobID, "processing", 10, "Parsing EPUB", "", 0, "")
		textPath, err = parseEpub(jobID, filePath)
		if err != nil {
			updateJobStatus(jobID, "failed", 10, "Parse failed", "", 0, err.Error())
			return
		}
	} else {
		// TXT files are already text
		textPath = filePath
		updateJobStatus(jobID, "processing", 15, "Text ready", "", 0, "")
	}

	// Step 2: Chunk text
	updateJobStatus(jobID, "processing", 20, "Chunking text", "", 0, "")
	chunkFiles, err := chunkText(jobID, textPath)
	if err != nil {
		updateJobStatus(jobID, "failed", 20, "Chunk failed", "", 0, err.Error())
		return
	}

	// Step 3: TTS Convert each chunk
	totalChunks := len(chunkFiles)
	audioFiles := make([]string, 0, totalChunks)
	totalDuration := 0

	for i, chunkFile := range chunkFiles {
		progress := 30 + int(float64(i)/float64(totalChunks)*40)
		updateJobStatus(jobID, "processing", progress, fmt.Sprintf("Converting chunk %d/%d", i+1, totalChunks), "", 0, "")

		audioFile, duration, err := convertToAudio(jobID, chunkFile, i)
		if err != nil {
			updateJobStatus(jobID, "failed", progress, "TTS failed", "", 0, err.Error())
			return
		}
		audioFiles = append(audioFiles, audioFile)
		totalDuration += duration
	}

	// Step 4: Concat audio
	updateJobStatus(jobID, "processing", 75, "Concatenating audio", "", 0, "")
	concatPath, concatDuration, err := concatAudio(jobID, audioFiles)
	if err != nil {
		updateJobStatus(jobID, "failed", 75, "Concat failed", "", 0, err.Error())
		return
	}
	if concatDuration > 0 {
		totalDuration = concatDuration
	}

	// Step 5: Normalize
	updateJobStatus(jobID, "processing", 90, "Normalizing audio", "", 0, "")
	outputPath, finalDuration, err := normalizeAudio(jobID, concatPath)
	if err != nil {
		updateJobStatus(jobID, "failed", 90, "Normalize failed", "", 0, err.Error())
		return
	}
	if finalDuration > 0 {
		totalDuration = finalDuration
	}

	// Done!
	updateJobStatus(jobID, "completed", 100, "Complete", outputPath, totalDuration, "")
	log.Printf("[%s] Pipeline complete: %s (duration: %ds)", jobID, outputPath, totalDuration)
}

func updateJobStatus(jobID, status string, progress int, currentStep, outputPath string, duration int, errMsg string) {
	if db != nil {
		_, err := db.Exec(`
			UPDATE audiobook_jobs
			SET status = $1, progress = $2, current_step = $3, output_path = $4, duration = $5, error = $6, updated_at = NOW()
			WHERE job_id = $7
		`, status, progress, currentStep, outputPath, duration, errMsg, jobID)
		if err != nil {
			log.Printf("Error updating job status: %v", err)
		}
	}

	// Broadcast update
	broadcastSSE(SSEMessage{
		Type: "job_updated",
		Data: map[string]interface{}{
			"job_id":       jobID,
			"status":       status,
			"progress":     progress,
			"current_step": currentStep,
			"output_path":  outputPath,
			"duration":     duration,
			"error":        errMsg,
		},
	})

	// If completed, add to library
	if status == "completed" && outputPath != "" {
		job := getJobByID(jobID)
		if job != nil {
			addToLibrary(job)
		}
	}
}

func parseEpub(jobID, inputPath string) (string, error) {
	outputPath := filepath.Join(parsedDir, jobID+".txt")

	req := ParseRequest{
		JobID:  jobID,
		Input:  inputPath,
		Output: outputPath,
	}

	resp, err := postJSON(parseEpubURL+"/parse", req)
	if err != nil {
		return "", err
	}

	var parseResp ParseResponse
	if err := json.Unmarshal(resp, &parseResp); err != nil {
		return "", err
	}

	if parseResp.Status != "success" && parseResp.Status != "completed" {
		if parseResp.Error != "" {
			return "", fmt.Errorf(parseResp.Error)
		}
		return "", fmt.Errorf("parse failed with status: %s", parseResp.Status)
	}

	return parseResp.Output, nil
}

func chunkText(jobID, inputPath string) ([]string, error) {
	chunkDir := filepath.Join(chunksDir, jobID)
	os.MkdirAll(chunkDir, 0755)

	req := ChunkRequest{
		JobID:     jobID,
		Input:     inputPath,
		OutputDir: chunkDir,
		ChunkSize: 4000, // ~4KB chunks for TTS
	}

	resp, err := postJSON(chunkTextURL+"/chunk", req)
	if err != nil {
		return nil, err
	}

	var chunkResp ChunkResponse
	if err := json.Unmarshal(resp, &chunkResp); err != nil {
		return nil, err
	}

	if chunkResp.Status != "success" && chunkResp.Status != "completed" {
		if chunkResp.Error != "" {
			return nil, fmt.Errorf(chunkResp.Error)
		}
		return nil, fmt.Errorf("chunk failed with status: %s", chunkResp.Status)
	}

	return chunkResp.ChunkFiles, nil
}

func convertToAudio(jobID, inputPath string, index int) (string, int, error) {
	outputPath := filepath.Join(audioDir, jobID, fmt.Sprintf("chunk_%04d.mp3", index))
	os.MkdirAll(filepath.Dir(outputPath), 0755)

	req := TTSRequest{
		JobID:  jobID,
		Input:  inputPath,
		Output: outputPath,
	}

	resp, err := postJSONWithTimeout(ttsConvertURL+"/convert", req, 5*time.Minute)
	if err != nil {
		return "", 0, err
	}

	var ttsResp TTSResponse
	if err := json.Unmarshal(resp, &ttsResp); err != nil {
		return "", 0, err
	}

	if ttsResp.Status != "success" && ttsResp.Status != "completed" {
		if ttsResp.Error != "" {
			return "", 0, fmt.Errorf(ttsResp.Error)
		}
		return "", 0, fmt.Errorf("TTS failed with status: %s", ttsResp.Status)
	}

	return ttsResp.Output, ttsResp.Duration, nil
}

func concatAudio(jobID string, inputFiles []string) (string, int, error) {
	outputPath := filepath.Join(audioDir, jobID, "concat.mp3")

	req := ConcatRequest{
		JobID:  jobID,
		Inputs: inputFiles,
		Output: outputPath,
	}

	resp, err := postJSONWithTimeout(audioConcatURL+"/concat", req, 10*time.Minute)
	if err != nil {
		return "", 0, err
	}

	var concatResp ConcatResponse
	if err := json.Unmarshal(resp, &concatResp); err != nil {
		return "", 0, err
	}

	if concatResp.Status != "success" && concatResp.Status != "completed" {
		if concatResp.Error != "" {
			return "", 0, fmt.Errorf(concatResp.Error)
		}
		return "", 0, fmt.Errorf("concat failed with status: %s", concatResp.Status)
	}

	return concatResp.Output, concatResp.Duration, nil
}

func normalizeAudio(jobID, inputPath string) (string, int, error) {
	outputPath := filepath.Join(outputDir, jobID+".mp3")

	req := NormalizeRequest{
		JobID:  jobID,
		Input:  inputPath,
		Output: outputPath,
	}

	resp, err := postJSONWithTimeout(audioNormalizeURL+"/normalize", req, 10*time.Minute)
	if err != nil {
		return "", 0, err
	}

	var normResp NormalizeResponse
	if err := json.Unmarshal(resp, &normResp); err != nil {
		return "", 0, err
	}

	if normResp.Status != "success" && normResp.Status != "completed" {
		if normResp.Error != "" {
			return "", 0, fmt.Errorf(normResp.Error)
		}
		return "", 0, fmt.Errorf("normalize failed with status: %s", normResp.Status)
	}

	return normResp.Output, normResp.Duration, nil
}

func postJSON(url string, data interface{}) ([]byte, error) {
	return postJSONWithTimeout(url, data, 60*time.Second)
}

func postJSONWithTimeout(url string, data interface{}, timeout time.Duration) ([]byte, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("service returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	if db == nil {
		status = "degraded"
	} else if err := db.Ping(); err != nil {
		status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

func serveUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, uiHTML)
}

const uiHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Audiobook Studio</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        :root {
            --ctp-base: #1e1e2e;
            --ctp-mantle: #181825;
            --ctp-crust: #11111b;
            --ctp-text: #cdd6f4;
            --ctp-subtext0: #a6adc8;
            --ctp-subtext1: #bac2de;
            --ctp-surface0: #313244;
            --ctp-surface1: #45475a;
            --ctp-surface2: #585b70;
            --ctp-overlay0: #6c7086;
            --ctp-overlay1: #7f849c;
            --ctp-blue: #89b4fa;
            --ctp-lavender: #b4befe;
            --ctp-sapphire: #74c7ec;
            --ctp-sky: #89dceb;
            --ctp-teal: #94e2d5;
            --ctp-green: #a6e3a1;
            --ctp-yellow: #f9e2af;
            --ctp-peach: #fab387;
            --ctp-maroon: #eba0ac;
            --ctp-red: #f38ba8;
            --ctp-mauve: #cba6f7;
            --ctp-pink: #f5c2e7;
            --ctp-flamingo: #f2cdcd;
            --ctp-rosewater: #f5e0dc;
        }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--ctp-base); color: var(--ctp-text); min-height: 100vh; padding-bottom: 100px; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        header { text-align: center; padding: 30px 0; border-bottom: 1px solid var(--ctp-surface0); margin-bottom: 30px; }
        header h1 { color: var(--ctp-mauve); font-size: 2.5rem; margin-bottom: 10px; display: flex; align-items: center; justify-content: center; gap: 15px; }
        .tabs { display: flex; gap: 10px; margin-bottom: 30px; justify-content: center; }
        .tab-btn { background: var(--ctp-surface0); border: none; color: var(--ctp-text); padding: 12px 24px; border-radius: 10px; cursor: pointer; font-size: 1rem; transition: all 0.2s; display: flex; align-items: center; gap: 8px; }
        .tab-btn:hover { background: var(--ctp-surface1); }
        .tab-btn.active { background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-pink)); color: var(--ctp-crust); }
        .tab-content { display: none; }
        .tab-content.active { display: block; }
        .upload-section { background: var(--ctp-mantle); border-radius: 16px; padding: 30px; margin-bottom: 30px; border: 1px solid var(--ctp-surface0); }
        .dropzone { border: 2px dashed var(--ctp-surface2); border-radius: 12px; padding: 60px 30px; text-align: center; cursor: pointer; transition: all 0.3s; background: var(--ctp-base); }
        .dropzone:hover, .dropzone.dragover { border-color: var(--ctp-mauve); background: var(--ctp-surface0); }
        .dropzone-icon { font-size: 4rem; margin-bottom: 20px; }
        .dropzone h3 { color: var(--ctp-text); margin-bottom: 10px; }
        .dropzone p { color: var(--ctp-subtext0); font-size: 0.9rem; }
        .file-input { display: none; }
        .section-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
        .section-header h2 { color: var(--ctp-lavender); display: flex; align-items: center; gap: 10px; }
        .job-list { display: flex; flex-direction: column; gap: 15px; }
        .job-card { background: var(--ctp-mantle); border-radius: 12px; padding: 20px; border: 1px solid var(--ctp-surface0); transition: all 0.2s; }
        .job-card:hover { border-color: var(--ctp-surface1); }
        .job-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 15px; gap: 15px; }
        .job-info { flex: 1; }
        .job-filename { color: var(--ctp-text); font-weight: 600; font-size: 1.1rem; margin-bottom: 5px; }
        .job-meta { color: var(--ctp-subtext0); font-size: 0.85rem; }
        .job-status { padding: 5px 12px; border-radius: 20px; font-size: 0.85rem; font-weight: 500; white-space: nowrap; }
        .status-pending { background: var(--ctp-surface1); color: var(--ctp-subtext1); }
        .status-processing { background: rgba(137, 180, 250, 0.2); color: var(--ctp-blue); }
        .status-completed { background: rgba(166, 227, 161, 0.2); color: var(--ctp-green); }
        .status-failed { background: rgba(243, 139, 168, 0.2); color: var(--ctp-red); }
        .status-cancelled { background: rgba(249, 226, 175, 0.2); color: var(--ctp-yellow); }
        .pipeline-stages { display: flex; gap: 8px; margin-bottom: 15px; flex-wrap: wrap; }
        .stage { display: flex; align-items: center; gap: 6px; padding: 6px 12px; border-radius: 8px; font-size: 0.8rem; background: var(--ctp-surface0); color: var(--ctp-subtext0); }
        .stage.active { background: rgba(137, 180, 250, 0.2); color: var(--ctp-blue); animation: pulse 1.5s infinite; }
        .stage.completed { background: rgba(166, 227, 161, 0.2); color: var(--ctp-green); }
        .stage.failed { background: rgba(243, 139, 168, 0.2); color: var(--ctp-red); }
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.6; } }
        .progress-container { background: var(--ctp-surface0); border-radius: 10px; height: 10px; overflow: hidden; margin-bottom: 10px; }
        .progress-fill { background: linear-gradient(90deg, var(--ctp-mauve), var(--ctp-pink)); height: 100%; border-radius: 10px; transition: width 0.3s ease; }
        .job-footer { display: flex; justify-content: space-between; align-items: center; color: var(--ctp-subtext0); font-size: 0.9rem; }
        .job-actions { display: flex; gap: 10px; }
        .btn { background: var(--ctp-surface0); color: var(--ctp-text); border: none; padding: 8px 16px; border-radius: 8px; cursor: pointer; font-size: 0.85rem; display: inline-flex; align-items: center; gap: 6px; transition: all 0.2s; text-decoration: none; }
        .btn:hover { background: var(--ctp-surface1); }
        .btn-primary { background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-pink)); color: var(--ctp-crust); }
        .btn-primary:hover { transform: translateY(-2px); box-shadow: 0 4px 15px rgba(203, 166, 247, 0.3); }
        .btn-danger { background: rgba(243, 139, 168, 0.2); color: var(--ctp-red); }
        .btn-danger:hover { background: rgba(243, 139, 168, 0.3); }
        .library-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 20px; }
        .audiobook-card { background: var(--ctp-mantle); border-radius: 16px; overflow: hidden; border: 1px solid var(--ctp-surface0); transition: all 0.3s; }
        .audiobook-card:hover { transform: translateY(-5px); box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3); border-color: var(--ctp-mauve); }
        .audiobook-cover { width: 100%; height: 160px; background: linear-gradient(135deg, var(--ctp-surface0), var(--ctp-surface1)); display: flex; align-items: center; justify-content: center; font-size: 4rem; }
        .audiobook-info { padding: 20px; }
        .audiobook-title { color: var(--ctp-text); font-weight: 600; font-size: 1.1rem; margin-bottom: 5px; }
        .audiobook-author { color: var(--ctp-subtext0); font-size: 0.9rem; margin-bottom: 10px; }
        .audiobook-meta { color: var(--ctp-overlay0); font-size: 0.85rem; margin-bottom: 15px; }
        .audiobook-actions { display: flex; gap: 10px; }
        .player-bar { position: fixed; bottom: 80px; left: 50%; transform: translateX(-50%); background: var(--ctp-mantle); border-radius: 16px; padding: 15px 25px; display: none; align-items: center; gap: 20px; border: 1px solid var(--ctp-surface0); box-shadow: 0 10px 40px rgba(0, 0, 0, 0.4); max-width: 600px; width: 90%; z-index: 999; }
        .player-bar.active { display: flex; }
        .player-info { flex: 1; min-width: 0; }
        .player-title { color: var(--ctp-text); font-weight: 600; font-size: 0.95rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .player-controls { display: flex; gap: 10px; align-items: center; }
        .player-btn { width: 40px; height: 40px; border-radius: 50%; border: none; background: var(--ctp-surface0); color: var(--ctp-text); cursor: pointer; font-size: 1.2rem; display: flex; align-items: center; justify-content: center; transition: all 0.2s; }
        .player-btn:hover { background: var(--ctp-surface1); }
        .player-btn.play { background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-pink)); color: var(--ctp-crust); }
        .player-progress { flex: 2; display: flex; flex-direction: column; gap: 5px; }
        .player-slider { -webkit-appearance: none; width: 100%; height: 6px; border-radius: 3px; background: var(--ctp-surface0); cursor: pointer; }
        .player-slider::-webkit-slider-thumb { -webkit-appearance: none; width: 14px; height: 14px; border-radius: 50%; background: var(--ctp-mauve); cursor: pointer; }
        .player-time { display: flex; justify-content: space-between; font-size: 0.75rem; color: var(--ctp-subtext0); }
        .dock-bar { position: fixed; bottom: 20px; left: 50%; transform: translateX(-50%); background: var(--ctp-mantle); border-radius: 20px; padding: 10px 20px; display: flex; gap: 15px; border: 1px solid var(--ctp-surface0); box-shadow: 0 10px 40px rgba(0, 0, 0, 0.4); z-index: 1000; }
        .dock-item { width: 50px; height: 50px; border-radius: 12px; display: flex; align-items: center; justify-content: center; text-decoration: none; font-size: 1.5rem; transition: transform 0.2s, background 0.2s; background: var(--ctp-surface0); position: relative; }
        .dock-item:hover { transform: translateY(-5px) scale(1.1); background: var(--ctp-surface1); }
        .dock-item.active { background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-pink)); }
        .dock-tooltip { position: absolute; bottom: 60px; background: var(--ctp-surface0); color: var(--ctp-text); padding: 5px 10px; border-radius: 6px; font-size: 0.8rem; opacity: 0; pointer-events: none; transition: opacity 0.2s; white-space: nowrap; }
        .dock-item:hover .dock-tooltip { opacity: 1; }
        .empty-state { text-align: center; padding: 60px 20px; color: var(--ctp-subtext0); }
        .empty-state-icon { font-size: 4rem; margin-bottom: 15px; }
        .error-message { background: rgba(243, 139, 168, 0.1); border: 1px solid var(--ctp-red); border-radius: 8px; padding: 10px 15px; color: var(--ctp-red); font-size: 0.85rem; margin-top: 10px; }
        @media (max-width: 768px) { .container { padding: 15px; } header h1 { font-size: 1.8rem; } .tabs { flex-wrap: wrap; } .tab-btn { padding: 10px 16px; font-size: 0.9rem; } .pipeline-stages { display: none; } .job-header { flex-direction: column; } .library-grid { grid-template-columns: 1fr; } .player-bar { flex-direction: column; padding: 15px; } .player-progress { width: 100%; } }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Audiobook Studio</h1>
            <p>Convert EPUB and TXT files to audiobooks</p>
        </header>
        <div class="tabs">
            <button class="tab-btn active" data-tab="upload">Upload</button>
            <button class="tab-btn" data-tab="jobs">Jobs <span id="jobCount"></span></button>
            <button class="tab-btn" data-tab="library">Library <span id="libraryCount"></span></button>
        </div>
        <div id="upload" class="tab-content active">
            <section class="upload-section">
                <div class="dropzone" id="dropzone">
                    <div class="dropzone-icon">&#128218;</div>
                    <h3>Drop your file here</h3>
                    <p>Supports EPUB and TXT files (up to 100MB)</p>
                    <input type="file" class="file-input" id="fileInput" accept=".epub,.txt">
                </div>
            </section>
        </div>
        <div id="jobs" class="tab-content">
            <div class="section-header"><h2>Pipeline Jobs</h2></div>
            <div class="job-list" id="jobList"><div class="empty-state"><div class="empty-state-icon">&#128237;</div><p>No jobs yet. Upload a file to get started!</p></div></div>
        </div>
        <div id="library" class="tab-content">
            <div class="section-header"><h2>Your Library</h2></div>
            <div class="library-grid" id="libraryGrid"><div class="empty-state"><div class="empty-state-icon">&#128218;</div><p>Your library is empty. Complete some conversions!</p></div></div>
        </div>
    </div>
    <div class="player-bar" id="playerBar">
        <div class="player-controls">
            <button class="player-btn" id="prevBtn">&#9198;</button>
            <button class="player-btn play" id="playBtn">&#9654;</button>
            <button class="player-btn" id="nextBtn">&#9197;</button>
        </div>
        <div class="player-info"><div class="player-title" id="playerTitle">Not Playing</div></div>
        <div class="player-progress">
            <input type="range" class="player-slider" id="progressSlider" min="0" max="100" value="0">
            <div class="player-time"><span id="currentTime">0:00</span><span id="totalTime">0:00</span></div>
        </div>
        <button class="player-btn" id="closePlayer">&#10005;</button>
    </div>
    <nav class="dock-bar">
        <a href="http://holm.local:30080" class="dock-item"><span class="dock-tooltip">Home</span>&#127968;</a>
        <a href="http://holm.local:30100" class="dock-item"><span class="dock-tooltip">Files</span>&#128193;</a>
        <a href="http://holm.local:30200" class="dock-item"><span class="dock-tooltip">Notes</span>&#128221;</a>
        <a href="http://holm.local:30300" class="dock-item"><span class="dock-tooltip">Photos</span>&#128247;</a>
        <a href="http://holm.local:30400" class="dock-item"><span class="dock-tooltip">Chat</span>&#128172;</a>
        <a href="http://holm.local:30700" class="dock-item active"><span class="dock-tooltip">Audiobook</span>&#127911;</a>
    </nav>
    <audio id="audioPlayer"></audio>
    <script>
        const PIPELINE_STAGES = [
            { id: 'upload', name: 'Upload', icon: '&#128228;' },
            { id: 'parse', name: 'Parse', icon: '&#128214;' },
            { id: 'chunk', name: 'Chunk', icon: '&#9986;' },
            { id: 'tts', name: 'TTS', icon: '&#128483;' },
            { id: 'concat', name: 'Concat', icon: '&#128279;' },
            { id: 'normalize', name: 'Normalize', icon: '&#128266;' }
        ];
        let jobs = [];
        let library = [];
        let eventSource = null;
        const audio = document.getElementById('audioPlayer');
        let currentPlayingId = null;

        document.querySelectorAll('.tab-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
                document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
                btn.classList.add('active');
                document.getElementById(btn.dataset.tab).classList.add('active');
            });
        });

        const dropzone = document.getElementById('dropzone');
        const fileInput = document.getElementById('fileInput');
        dropzone.addEventListener('click', () => fileInput.click());
        dropzone.addEventListener('dragover', (e) => { e.preventDefault(); dropzone.classList.add('dragover'); });
        dropzone.addEventListener('dragleave', () => dropzone.classList.remove('dragover'));
        dropzone.addEventListener('drop', (e) => { e.preventDefault(); dropzone.classList.remove('dragover'); if (e.dataTransfer.files[0]) uploadFile(e.dataTransfer.files[0]); });
        fileInput.addEventListener('change', () => { if (fileInput.files[0]) uploadFile(fileInput.files[0]); });

        async function uploadFile(file) {
            const ext = file.name.split('.').pop().toLowerCase();
            let endpoint = ext === 'epub' ? '/api/upload/epub' : ext === 'txt' ? '/api/upload/txt' : null;
            if (!endpoint) { alert('Please upload an EPUB or TXT file'); return; }
            const formData = new FormData();
            formData.append('file', file);
            try {
                dropzone.innerHTML = '<div class="dropzone-icon">&#8987;</div><h3>Uploading...</h3>';
                const response = await fetch(endpoint, { method: 'POST', body: formData });
                if (response.ok) { document.querySelector('[data-tab="jobs"]').click(); }
                else { alert('Upload failed: ' + await response.text()); }
            } catch (err) { alert('Upload failed: ' + err.message); }
            finally { dropzone.innerHTML = '<div class="dropzone-icon">&#128218;</div><h3>Drop your file here</h3><p>Supports EPUB and TXT files (up to 100MB)</p><input type="file" class="file-input" id="fileInput" accept=".epub,.txt">'; document.getElementById('fileInput').addEventListener('change', () => { if (document.getElementById('fileInput').files[0]) uploadFile(document.getElementById('fileInput').files[0]); }); }
        }

        function getStageIndex(currentStep) {
            if (!currentStep) return -1;
            const step = currentStep.toLowerCase();
            if (step.includes('upload') || step.includes('starting')) return 0;
            if (step.includes('pars')) return 1;
            if (step.includes('chunk')) return 2;
            if (step.includes('tts') || step.includes('convert')) return 3;
            if (step.includes('concat')) return 4;
            if (step.includes('normal')) return 5;
            if (step.includes('complete')) return 6;
            return -1;
        }

        function renderJobs() {
            const jobList = document.getElementById('jobList');
            const activeJobs = jobs.filter(j => j.status !== 'completed' || Date.now() - new Date(j.updated_at).getTime() < 300000);
            document.getElementById('jobCount').textContent = activeJobs.length > 0 ? '(' + activeJobs.length + ')' : '';
            if (jobs.length === 0) { jobList.innerHTML = '<div class="empty-state"><div class="empty-state-icon">&#128237;</div><p>No jobs yet. Upload a file to get started!</p></div>'; return; }
            jobList.innerHTML = jobs.map(job => {
                const stageIndex = getStageIndex(job.current_step);
                const stages = PIPELINE_STAGES.map((stage, i) => {
                    let stageClass = '';
                    if (job.status === 'failed') stageClass = i <= stageIndex ? 'failed' : '';
                    else if (job.status === 'completed') stageClass = 'completed';
                    else if (i < stageIndex) stageClass = 'completed';
                    else if (i === stageIndex) stageClass = 'active';
                    return '<span class="stage ' + stageClass + '">' + stage.icon + ' ' + stage.name + '</span>';
                }).join('');
                const statusClass = 'status-' + job.status;
                const progress = job.progress || 0;
                let actions = '';
                if (job.status === 'completed') { actions = '<a href="/api/download/' + job.job_id + '" class="btn btn-primary">&#11015; Download</a><button class="btn" onclick="playAudio(\'' + job.job_id + '\', \'' + escapeHtml(job.filename) + '\')">&#9654; Play</button>'; }
                else if (job.status === 'failed') { actions = '<button class="btn" onclick="retryJob(\'' + job.job_id + '\')">&#128260; Retry</button><button class="btn btn-danger" onclick="deleteJob(\'' + job.job_id + '\')">&#128465; Delete</button>'; }
                else if (job.status === 'pending' || job.status === 'processing') { actions = '<button class="btn btn-danger" onclick="cancelJob(\'' + job.job_id + '\')">&#9209; Cancel</button>'; }
                let errorHtml = job.error ? '<div class="error-message">Error: ' + escapeHtml(job.error) + '</div>' : '';
                return '<div class="job-card"><div class="job-header"><div class="job-info"><div class="job-filename">' + escapeHtml(job.filename) + '</div><div class="job-meta">' + formatSize(job.file_size) + ' - ' + formatDate(job.created_at) + '</div></div><span class="job-status ' + statusClass + '">' + job.status + '</span></div><div class="pipeline-stages">' + stages + '</div><div class="progress-container"><div class="progress-fill" style="width: ' + progress + '%"></div></div><div class="job-footer"><span>' + progress + '% complete' + (job.current_step ? ' - ' + job.current_step : '') + '</span><div class="job-actions">' + actions + '</div></div>' + errorHtml + '</div>';
            }).join('');
        }

        function renderLibrary() {
            const grid = document.getElementById('libraryGrid');
            document.getElementById('libraryCount').textContent = library.length > 0 ? '(' + library.length + ')' : '';
            if (library.length === 0) { grid.innerHTML = '<div class="empty-state"><div class="empty-state-icon">&#128218;</div><p>Your library is empty. Complete some conversions!</p></div>'; return; }
            grid.innerHTML = library.map(ab => '<div class="audiobook-card"><div class="audiobook-cover">&#127911;</div><div class="audiobook-info"><div class="audiobook-title">' + escapeHtml(ab.title) + '</div><div class="audiobook-author">' + escapeHtml(ab.author) + '</div><div class="audiobook-meta">' + formatDuration(ab.duration) + ' - ' + formatSize(ab.file_size) + '</div><div class="audiobook-actions"><button class="btn btn-primary" onclick="playAudio(\'' + ab.job_id + '\', \'' + escapeHtml(ab.title) + '\')">&#9654; Play</button><a href="/api/download/' + ab.job_id + '" class="btn">&#11015;</a><button class="btn btn-danger" onclick="deleteAudiobook(' + ab.id + ')">&#128465;</button></div></div></div>').join('');
        }

        async function retryJob(jobId) { await fetch('/api/jobs/' + jobId + '/retry', { method: 'POST' }); }
        async function cancelJob(jobId) { await fetch('/api/jobs/' + jobId + '/cancel', { method: 'POST' }); }
        async function deleteJob(jobId) { if (confirm('Delete this job?')) { await fetch('/api/jobs/' + jobId, { method: 'DELETE' }); } }
        async function deleteAudiobook(id) { if (confirm('Remove from library?')) { await fetch('/api/library/' + id, { method: 'DELETE' }); loadLibrary(); } }

        function playAudio(jobId, title) {
            audio.src = '/api/stream/' + jobId;
            audio.play();
            currentPlayingId = jobId;
            document.getElementById('playerTitle').textContent = title;
            document.getElementById('playerBar').classList.add('active');
            document.getElementById('playBtn').innerHTML = '&#9208;';
        }

        document.getElementById('playBtn').addEventListener('click', () => { if (audio.paused) { audio.play(); document.getElementById('playBtn').innerHTML = '&#9208;'; } else { audio.pause(); document.getElementById('playBtn').innerHTML = '&#9654;'; } });
        document.getElementById('closePlayer').addEventListener('click', () => { audio.pause(); document.getElementById('playerBar').classList.remove('active'); currentPlayingId = null; });
        document.getElementById('progressSlider').addEventListener('input', (e) => { if (audio.duration) { audio.currentTime = (e.target.value / 100) * audio.duration; } });
        audio.addEventListener('timeupdate', () => { if (audio.duration) { document.getElementById('progressSlider').value = (audio.currentTime / audio.duration) * 100; document.getElementById('currentTime').textContent = formatTime(audio.currentTime); document.getElementById('totalTime').textContent = formatTime(audio.duration); } });
        audio.addEventListener('ended', () => { document.getElementById('playBtn').innerHTML = '&#9654;'; });

        function formatTime(seconds) { const m = Math.floor(seconds / 60); const s = Math.floor(seconds % 60); return m + ':' + (s < 10 ? '0' : '') + s; }
        function formatDuration(seconds) { if (!seconds) return 'Unknown'; const h = Math.floor(seconds / 3600); const m = Math.floor((seconds % 3600) / 60); if (h > 0) return h + 'h ' + m + 'm'; return m + ' min'; }
        function formatSize(bytes) { if (!bytes) return 'Unknown'; if (bytes < 1024) return bytes + ' B'; if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'; return (bytes / (1024 * 1024)).toFixed(1) + ' MB'; }
        function formatDate(dateStr) { return new Date(dateStr).toLocaleString(); }
        function escapeHtml(text) { const div = document.createElement('div'); div.textContent = text || ''; return div.innerHTML; }

        async function loadLibrary() { try { const resp = await fetch('/api/library'); library = await resp.json(); renderLibrary(); } catch (e) { console.error('Failed to load library:', e); } }

        function connectSSE() {
            if (eventSource) eventSource.close();
            eventSource = new EventSource('/api/events');
            eventSource.onmessage = (e) => {
                try {
                    const msg = JSON.parse(e.data);
                    if (msg.type === 'initial' || msg.type === 'jobs_update') { jobs = msg.data || []; renderJobs(); }
                    else if (msg.type === 'job_created' || msg.type === 'job_updated') { loadJobs(); }
                    else if (msg.type === 'job_deleted') { jobs = jobs.filter(j => j.job_id !== msg.data.job_id); renderJobs(); }
                    else if (msg.type === 'library_updated') { loadLibrary(); }
                } catch (err) { console.error('SSE parse error:', err); }
            };
            eventSource.onerror = () => { setTimeout(connectSSE, 5000); };
        }

        async function loadJobs() { try { const resp = await fetch('/api/jobs'); jobs = await resp.json(); renderJobs(); } catch (e) { console.error('Failed to load jobs:', e); } }

        connectSSE();
        loadLibrary();
    </script>
</body>
</html>` + "`"
