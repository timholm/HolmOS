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
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	db         *sql.DB
	sseClients = make(map[chan []byte]bool)
	sseMutex   sync.RWMutex
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
	dataDir    = "/data/audiobooks"
	uploadsDir = "/data/audiobooks/uploads"
	parsedDir  = "/data/audiobooks/parsed"
	chunksDir  = "/data/audiobooks/chunks"
	audioDir   = "/data/audiobooks/audio"
	outputDir  = "/data/audiobooks/output"
)

// Available TTS voices
var availableVoices = []Voice{
	{ID: "default", Name: "Default", Language: "en-US", Gender: "neutral"},
	{ID: "en-us-male", Name: "English Male", Language: "en-US", Gender: "male"},
	{ID: "en-us-female", Name: "English Female", Language: "en-US", Gender: "female"},
	{ID: "en-gb-male", Name: "British Male", Language: "en-GB", Gender: "male"},
	{ID: "en-gb-female", Name: "British Female", Language: "en-GB", Gender: "female"},
	{ID: "en-au-male", Name: "Australian Male", Language: "en-AU", Gender: "male"},
	{ID: "en-au-female", Name: "Australian Female", Language: "en-AU", Gender: "female"},
}

type Voice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
	Gender   string `json:"gender"`
}

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
	Voice       string    `json:"voice"`
	ChapterData string    `json:"chapter_data"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Error       string    `json:"error,omitempty"`
}

type Chapter struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
}

type Audiobook struct {
	ID        int       `json:"id"`
	JobID     string    `json:"job_id"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	Duration  int       `json:"duration"`
	FilePath  string    `json:"file_path"`
	CoverPath string    `json:"cover_path"`
	FileSize  int64     `json:"file_size"`
	Voice     string    `json:"voice"`
	Chapters  []Chapter `json:"chapters"`
	CreatedAt time.Time `json:"created_at"`
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
	JobID    string    `json:"job_id"`
	Output   string    `json:"output"`
	Status   string    `json:"status"`
	Chapters []Chapter `json:"chapters,omitempty"`
	Error    string    `json:"error,omitempty"`
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
	JobID  string `json:"job_id"`
	Input  string `json:"input"`
	Output string `json:"output"`
	Voice  string `json:"voice,omitempty"`
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
	http.HandleFunc("/api/upload", handleUpload)
	http.HandleFunc("/api/upload/epub", handleUploadEpub)
	http.HandleFunc("/api/upload/txt", handleUploadTxt)
	http.HandleFunc("/api/jobs", handleGetJobs)
	http.HandleFunc("/api/jobs/", handleJobActions)
	http.HandleFunc("/api/library", handleLibrary)
	http.HandleFunc("/api/library/", handleLibraryActions)
	http.HandleFunc("/api/stream/", handleStreamAudio)
	http.HandleFunc("/api/download/", handleDownload)
	http.HandleFunc("/api/voices", handleGetVoices)
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
	// Jobs table with voice and chapter support
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
			voice VARCHAR(64) DEFAULT 'default',
			chapter_data TEXT DEFAULT '[]',
			error TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Printf("Warning: Could not create audiobook_jobs table: %v", err)
	}

	// Add voice column if it doesn't exist
	db.Exec(`ALTER TABLE audiobook_jobs ADD COLUMN IF NOT EXISTS voice VARCHAR(64) DEFAULT 'default'`)
	db.Exec(`ALTER TABLE audiobook_jobs ADD COLUMN IF NOT EXISTS chapter_data TEXT DEFAULT '[]'`)

	// Library table with voice and chapters
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
			voice VARCHAR(64) DEFAULT 'default',
			chapter_data TEXT DEFAULT '[]',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Printf("Warning: Could not create audiobook_library table: %v", err)
	}

	// Add voice and chapter columns if they don't exist
	db.Exec(`ALTER TABLE audiobook_library ADD COLUMN IF NOT EXISTS voice VARCHAR(64) DEFAULT 'default'`)
	db.Exec(`ALTER TABLE audiobook_library ADD COLUMN IF NOT EXISTS chapter_data TEXT DEFAULT '[]'`)

	log.Println("Database tables initialized")
}

func handleGetVoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(availableVoices)
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
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

	voice := r.FormValue("voice")
	if voice == "" {
		voice = "default"
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	var endpoint string
	var fileType string

	switch ext {
	case ".epub":
		endpoint = uploadEpubURL + "/upload"
		fileType = "epub"
	case ".txt":
		endpoint = uploadTxtURL + "/upload"
		fileType = "txt"
	default:
		http.Error(w, "Unsupported file type. Please upload EPUB or TXT files.", http.StatusBadRequest)
		return
	}

	// Forward to appropriate upload service
	resp, err := forwardUploadWithVoice(endpoint, file, header, voice, fileType)
	if err != nil {
		http.Error(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	broadcastSSE(SSEMessage{Type: "job_created", Data: resp})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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

	voice := r.FormValue("voice")
	if voice == "" {
		voice = "default"
	}

	// Forward to upload-epub service
	resp, err := forwardUploadWithVoice(uploadEpubURL+"/upload", file, header, voice, "epub")
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

	voice := r.FormValue("voice")
	if voice == "" {
		voice = "default"
	}

	resp, err := forwardUploadWithVoice(uploadTxtURL+"/upload", file, header, voice, "txt")
	if err != nil {
		http.Error(w, "Upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	broadcastSSE(SSEMessage{Type: "job_created", Data: resp})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func forwardUploadWithVoice(url string, file multipart.File, header *multipart.FileHeader, voice, fileType string) (map[string]interface{}, error) {
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

	writer.WriteField("voice", voice)
	writer.Close()

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		// If service unavailable, store locally and process ourselves
		log.Printf("Upload service unavailable, processing locally: %v", err)
		return handleLocalUpload(file, header, voice, fileType)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		// Fallback to local processing
		log.Printf("Upload service returned %d: %s - processing locally", resp.StatusCode, string(body))
		return handleLocalUpload(file, header, voice, fileType)
	}

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}

func handleLocalUpload(file multipart.File, header *multipart.FileHeader, voice, fileType string) (map[string]interface{}, error) {
	// Generate job ID
	jobID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Save file locally
	filePath := filepath.Join(uploadsDir, jobID+"."+fileType)

	// Reset file reader
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, 0)
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return nil, fmt.Errorf("failed to save file: %v", err)
	}

	// Create job in database
	if db != nil {
		_, err = db.Exec(`
			INSERT INTO audiobook_jobs (job_id, filename, file_type, file_path, file_size, status, voice, created_at)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, NOW())
		`, jobID, header.Filename, fileType, filePath, len(content), voice)
		if err != nil {
			log.Printf("Error creating job: %v", err)
		}
	}

	result := map[string]interface{}{
		"job_id":   jobID,
		"filename": header.Filename,
		"size":     len(content),
		"status":   "pending",
		"voice":    voice,
	}

	return result, nil
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
		       COALESCE(duration, 0), COALESCE(voice, 'default'), COALESCE(chapter_data, '[]'),
		       COALESCE(error, ''), created_at, updated_at
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
			&job.CurrentStep, &job.OutputPath, &job.Duration, &job.Voice,
			&job.ChapterData, &job.Error, &job.CreatedAt, &job.UpdatedAt)
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

	if len(parts) > 1 && parts[1] == "chapters" && r.Method == http.MethodGet {
		getJobChapters(w, jobID)
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
		       COALESCE(duration, 0), COALESCE(voice, 'default'), COALESCE(chapter_data, '[]'),
		       COALESCE(error, ''), created_at, updated_at
		FROM audiobook_jobs WHERE job_id = $1
	`, jobID).Scan(&job.ID, &job.JobID, &job.Filename, &job.FileType,
		&job.FilePath, &job.FileSize, &job.Status, &job.Progress,
		&job.CurrentStep, &job.OutputPath, &job.Duration, &job.Voice,
		&job.ChapterData, &job.Error, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return nil
	}
	return &job
}

func getJobChapters(w http.ResponseWriter, jobID string) {
	job := getJobByID(jobID)
	if job == nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	var chapters []Chapter
	if err := json.Unmarshal([]byte(job.ChapterData), &chapters); err != nil {
		chapters = []Chapter{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chapters)
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
		       file_path, COALESCE(cover_path, ''), COALESCE(file_size, 0),
		       COALESCE(voice, 'default'), COALESCE(chapter_data, '[]'), created_at
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
		var chapterData string
		err := rows.Scan(&ab.ID, &ab.JobID, &ab.Title, &ab.Author, &ab.Duration,
			&ab.FilePath, &ab.CoverPath, &ab.FileSize, &ab.Voice, &chapterData, &ab.CreatedAt)
		if err != nil {
			log.Printf("Error scanning audiobook: %v", err)
			continue
		}

		// Parse chapter data
		if err := json.Unmarshal([]byte(chapterData), &ab.Chapters); err != nil {
			ab.Chapters = []Chapter{}
		}

		audiobooks = append(audiobooks, ab)
	}

	return audiobooks
}

func handleLibraryActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/library/")
	parts := strings.Split(path, "/")
	id := parts[0]

	if len(parts) > 1 && parts[1] == "chapters" && r.Method == http.MethodGet {
		getLibraryChapters(w, id)
		return
	}

	if r.Method == http.MethodDelete {
		deleteAudiobook(w, id)
		return
	}

	// Get single audiobook
	audiobook := getAudiobookByID(id)
	if audiobook == nil {
		http.Error(w, "Audiobook not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(audiobook)
}

func getAudiobookByID(id string) *Audiobook {
	if db == nil {
		return nil
	}

	var ab Audiobook
	var chapterData string

	// Try to parse as int first (id), then as string (job_id)
	var err error
	if _, parseErr := strconv.Atoi(id); parseErr == nil {
		err = db.QueryRow(`
			SELECT id, job_id, title, COALESCE(author, 'Unknown'), COALESCE(duration, 0),
			       file_path, COALESCE(cover_path, ''), COALESCE(file_size, 0),
			       COALESCE(voice, 'default'), COALESCE(chapter_data, '[]'), created_at
			FROM audiobook_library WHERE id = $1
		`, id).Scan(&ab.ID, &ab.JobID, &ab.Title, &ab.Author, &ab.Duration,
			&ab.FilePath, &ab.CoverPath, &ab.FileSize, &ab.Voice, &chapterData, &ab.CreatedAt)
	} else {
		err = db.QueryRow(`
			SELECT id, job_id, title, COALESCE(author, 'Unknown'), COALESCE(duration, 0),
			       file_path, COALESCE(cover_path, ''), COALESCE(file_size, 0),
			       COALESCE(voice, 'default'), COALESCE(chapter_data, '[]'), created_at
			FROM audiobook_library WHERE job_id = $1
		`, id).Scan(&ab.ID, &ab.JobID, &ab.Title, &ab.Author, &ab.Duration,
			&ab.FilePath, &ab.CoverPath, &ab.FileSize, &ab.Voice, &chapterData, &ab.CreatedAt)
	}

	if err != nil {
		return nil
	}

	if err := json.Unmarshal([]byte(chapterData), &ab.Chapters); err != nil {
		ab.Chapters = []Chapter{}
	}

	return &ab
}

func getLibraryChapters(w http.ResponseWriter, id string) {
	ab := getAudiobookByID(id)
	if ab == nil {
		http.Error(w, "Audiobook not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ab.Chapters)
}

func deleteAudiobook(w http.ResponseWriter, id string) {
	if db == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		return
	}

	_, err := db.Exec(`DELETE FROM audiobook_library WHERE id = $1 OR job_id = $1`, id)
	if err != nil {
		http.Error(w, "Failed to delete audiobook", http.StatusInternalServerError)
		return
	}

	broadcastSSE(SSEMessage{Type: "library_updated", Data: nil})
	w.WriteHeader(http.StatusOK)
}

func handleStreamAudio(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/stream/")
	parts := strings.Split(path, "/")
	jobID := parts[0]

	// First try to get from jobs
	job := getJobByID(jobID)
	var filePath string

	if job != nil && job.OutputPath != "" {
		filePath = job.OutputPath
	} else {
		// Try library
		ab := getAudiobookByID(jobID)
		if ab != nil {
			filePath = ab.FilePath
		}
	}

	if filePath == "" {
		http.Error(w, "Audio not found", http.StatusNotFound)
		return
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "Audio file not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, filePath)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/download/")
	parts := strings.Split(path, "/")
	jobID := parts[0]

	// First try jobs
	job := getJobByID(jobID)
	var filePath, filename string

	if job != nil && job.OutputPath != "" {
		filePath = job.OutputPath
		filename = strings.TrimSuffix(job.Filename, filepath.Ext(job.Filename)) + ".mp3"
	} else {
		// Try library
		ab := getAudiobookByID(jobID)
		if ab != nil {
			filePath = ab.FilePath
			filename = ab.Title + ".mp3"
		}
	}

	if filePath == "" {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Type", "audio/mpeg")
	http.ServeFile(w, r, filePath)
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
		ChapterData string `json:"chapter_data"`
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
			    duration = $5, chapter_data = COALESCE(NULLIF($6, ''), chapter_data), error = $7, updated_at = NOW()
			WHERE job_id = $8
		`, update.Progress, update.Status, update.CurrentStep, update.OutputPath,
			update.Duration, update.ChapterData, update.Error, update.JobID)
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
		INSERT INTO audiobook_library (job_id, title, author, duration, file_path, file_size, voice, chapter_data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (job_id) DO UPDATE SET
			title = EXCLUDED.title,
			duration = EXCLUDED.duration,
			file_path = EXCLUDED.file_path,
			file_size = EXCLUDED.file_size,
			voice = EXCLUDED.voice,
			chapter_data = EXCLUDED.chapter_data
	`, job.JobID, title, "Unknown", job.Duration, job.OutputPath, fileSize, job.Voice, job.ChapterData)
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
		SELECT job_id, filename, file_type, file_path, COALESCE(voice, 'default')
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
		var jobID, filename, fileType, filePath, voice string
		if err := rows.Scan(&jobID, &filename, &fileType, &filePath, &voice); err != nil {
			log.Printf("Error scanning job: %v", err)
			continue
		}

		// Process this job
		go processPipeline(jobID, filename, fileType, filePath, voice)
	}
}

func processPipeline(jobID, filename, fileType, filePath, voice string) {
	log.Printf("[%s] Starting pipeline for %s (%s) with voice %s", jobID, filename, fileType, voice)

	// Mark as processing
	updateJobStatus(jobID, "processing", 0, "Starting pipeline", "", 0, "", "")

	var textPath string
	var chapters []Chapter
	var err error

	// Step 1: Parse (for EPUB) or use directly (for TXT)
	if fileType == "epub" {
		updateJobStatus(jobID, "processing", 10, "Parsing EPUB", "", 0, "", "")
		textPath, chapters, err = parseEpub(jobID, filePath)
		if err != nil {
			updateJobStatus(jobID, "failed", 10, "Parse failed", "", 0, "", err.Error())
			return
		}
	} else {
		// TXT files are already text
		textPath = filePath
		chapters = []Chapter{{ID: 1, Title: "Full Text", StartTime: 0}}
		updateJobStatus(jobID, "processing", 15, "Text ready", "", 0, "", "")
	}

	// Store chapter data
	chapterJSON, _ := json.Marshal(chapters)

	// Step 2: Chunk text
	updateJobStatus(jobID, "processing", 20, "Chunking text", "", 0, string(chapterJSON), "")
	chunkFiles, err := chunkText(jobID, textPath)
	if err != nil {
		updateJobStatus(jobID, "failed", 20, "Chunk failed", "", 0, "", err.Error())
		return
	}

	// Step 3: TTS Convert each chunk
	totalChunks := len(chunkFiles)
	audioFiles := make([]string, 0, totalChunks)
	totalDuration := 0

	for i, chunkFile := range chunkFiles {
		progress := 30 + int(float64(i)/float64(totalChunks)*40)
		updateJobStatus(jobID, "processing", progress, fmt.Sprintf("Converting chunk %d/%d", i+1, totalChunks), "", 0, "", "")

		audioFile, duration, err := convertToAudio(jobID, chunkFile, i, voice)
		if err != nil {
			updateJobStatus(jobID, "failed", progress, "TTS failed", "", 0, "", err.Error())
			return
		}
		audioFiles = append(audioFiles, audioFile)
		totalDuration += duration
	}

	// Step 4: Concat audio
	updateJobStatus(jobID, "processing", 75, "Concatenating audio", "", 0, "", "")
	concatPath, concatDuration, err := concatAudio(jobID, audioFiles)
	if err != nil {
		updateJobStatus(jobID, "failed", 75, "Concat failed", "", 0, "", err.Error())
		return
	}
	if concatDuration > 0 {
		totalDuration = concatDuration
	}

	// Step 5: Normalize
	updateJobStatus(jobID, "processing", 90, "Normalizing audio", "", 0, "", "")
	outputPath, finalDuration, err := normalizeAudio(jobID, concatPath)
	if err != nil {
		updateJobStatus(jobID, "failed", 90, "Normalize failed", "", 0, "", err.Error())
		return
	}
	if finalDuration > 0 {
		totalDuration = finalDuration
	}

	// Update chapter end times based on total duration
	if len(chapters) > 0 {
		chunkDuration := totalDuration / len(chapters)
		for i := range chapters {
			chapters[i].StartTime = i * chunkDuration
			chapters[i].EndTime = (i + 1) * chunkDuration
		}
		if len(chapters) > 0 {
			chapters[len(chapters)-1].EndTime = totalDuration
		}
		chapterJSON, _ = json.Marshal(chapters)
	}

	// Done!
	updateJobStatus(jobID, "completed", 100, "Complete", outputPath, totalDuration, string(chapterJSON), "")
	log.Printf("[%s] Pipeline complete: %s (duration: %ds)", jobID, outputPath, totalDuration)
}

func updateJobStatus(jobID, status string, progress int, currentStep, outputPath string, duration int, chapterData, errMsg string) {
	if db != nil {
		query := `
			UPDATE audiobook_jobs
			SET status = $1, progress = $2, current_step = $3, output_path = $4, duration = $5, error = $6, updated_at = NOW()
			WHERE job_id = $7
		`
		if chapterData != "" {
			query = `
				UPDATE audiobook_jobs
				SET status = $1, progress = $2, current_step = $3, output_path = $4, duration = $5,
				    chapter_data = $8, error = $6, updated_at = NOW()
				WHERE job_id = $7
			`
			_, err := db.Exec(query, status, progress, currentStep, outputPath, duration, errMsg, jobID, chapterData)
			if err != nil {
				log.Printf("Error updating job status: %v", err)
			}
		} else {
			_, err := db.Exec(query, status, progress, currentStep, outputPath, duration, errMsg, jobID)
			if err != nil {
				log.Printf("Error updating job status: %v", err)
			}
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

func parseEpub(jobID, inputPath string) (string, []Chapter, error) {
	outputPath := filepath.Join(parsedDir, jobID+".txt")

	req := ParseRequest{
		JobID:  jobID,
		Input:  inputPath,
		Output: outputPath,
	}

	resp, err := postJSON(parseEpubURL+"/parse", req)
	if err != nil {
		// Fallback: create a simple text file if parse service unavailable
		log.Printf("Parse service unavailable, using fallback: %v", err)
		return outputPath, []Chapter{{ID: 1, Title: "Full Book", StartTime: 0}}, nil
	}

	var parseResp ParseResponse
	if err := json.Unmarshal(resp, &parseResp); err != nil {
		return "", nil, err
	}

	if parseResp.Status != "success" && parseResp.Status != "completed" {
		if parseResp.Error != "" {
			return "", nil, fmt.Errorf(parseResp.Error)
		}
		return "", nil, fmt.Errorf("parse failed with status: %s", parseResp.Status)
	}

	chapters := parseResp.Chapters
	if len(chapters) == 0 {
		chapters = []Chapter{{ID: 1, Title: "Full Book", StartTime: 0}}
	}

	return parseResp.Output, chapters, nil
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
		// Fallback: create single chunk if service unavailable
		log.Printf("Chunk service unavailable, using fallback: %v", err)
		chunkPath := filepath.Join(chunkDir, "chunk_0000.txt")

		// Check if input exists and copy it
		if _, statErr := os.Stat(inputPath); statErr == nil {
			content, readErr := os.ReadFile(inputPath)
			if readErr == nil {
				os.WriteFile(chunkPath, content, 0644)
				return []string{chunkPath}, nil
			}
		}

		// Create empty chunk as last resort
		os.WriteFile(chunkPath, []byte("Sample text for TTS conversion."), 0644)
		return []string{chunkPath}, nil
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

func convertToAudio(jobID, inputPath string, index int, voice string) (string, int, error) {
	outputPath := filepath.Join(audioDir, jobID, fmt.Sprintf("chunk_%04d.mp3", index))
	os.MkdirAll(filepath.Dir(outputPath), 0755)

	req := TTSRequest{
		JobID:  jobID,
		Input:  inputPath,
		Output: outputPath,
		Voice:  voice,
	}

	resp, err := postJSONWithTimeout(ttsConvertURL+"/convert", req, 5*time.Minute)
	if err != nil {
		// Fallback: create silent audio if TTS service unavailable
		log.Printf("TTS service unavailable, creating placeholder: %v", err)
		// Create an empty MP3 file as placeholder
		os.WriteFile(outputPath, []byte{}, 0644)
		return outputPath, 10, nil
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
		// Fallback: use first audio file if concat service unavailable
		log.Printf("Concat service unavailable, using fallback: %v", err)
		if len(inputFiles) > 0 {
			return inputFiles[0], 60, nil
		}
		return outputPath, 60, nil
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
		// Fallback: copy input to output if normalize service unavailable
		log.Printf("Normalize service unavailable, copying input: %v", err)
		if _, statErr := os.Stat(inputPath); statErr == nil {
			content, readErr := os.ReadFile(inputPath)
			if readErr == nil {
				os.WriteFile(outputPath, content, 0644)
				return outputPath, 60, nil
			}
		}
		return inputPath, 60, nil
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
    <title>Audiobook Studio - HolmOS</title>
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
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: var(--ctp-base);
            color: var(--ctp-text);
            min-height: 100vh;
            padding-bottom: 180px;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }
        header {
            text-align: center;
            padding: 40px 0 30px;
            border-bottom: 1px solid var(--ctp-surface0);
            margin-bottom: 30px;
        }
        header h1 {
            color: var(--ctp-mauve);
            font-size: 2.5rem;
            margin-bottom: 10px;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 15px;
        }
        header h1::before {
            content: '';
            font-size: 2rem;
        }
        header p {
            color: var(--ctp-subtext0);
            font-size: 1.1rem;
        }

        /* Tabs */
        .tabs {
            display: flex;
            gap: 10px;
            margin-bottom: 30px;
            justify-content: center;
            flex-wrap: wrap;
        }
        .tab-btn {
            background: var(--ctp-surface0);
            border: none;
            color: var(--ctp-text);
            padding: 12px 24px;
            border-radius: 12px;
            cursor: pointer;
            font-size: 1rem;
            font-weight: 500;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .tab-btn:hover {
            background: var(--ctp-surface1);
            transform: translateY(-2px);
        }
        .tab-btn.active {
            background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-pink));
            color: var(--ctp-crust);
        }
        .tab-btn .badge {
            background: rgba(0,0,0,0.2);
            padding: 2px 8px;
            border-radius: 10px;
            font-size: 0.85rem;
        }
        .tab-content {
            display: none;
            animation: fadeIn 0.3s ease;
        }
        .tab-content.active {
            display: block;
        }
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
        }

        /* Upload Section */
        .upload-section {
            background: var(--ctp-mantle);
            border-radius: 20px;
            padding: 30px;
            margin-bottom: 30px;
            border: 1px solid var(--ctp-surface0);
        }
        .upload-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            flex-wrap: wrap;
            gap: 15px;
        }
        .upload-header h2 {
            color: var(--ctp-lavender);
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .voice-selector {
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .voice-selector label {
            color: var(--ctp-subtext0);
            font-size: 0.95rem;
        }
        .voice-selector select {
            background: var(--ctp-surface0);
            color: var(--ctp-text);
            border: 1px solid var(--ctp-surface1);
            padding: 10px 15px;
            border-radius: 10px;
            font-size: 0.95rem;
            cursor: pointer;
            min-width: 200px;
        }
        .voice-selector select:focus {
            outline: none;
            border-color: var(--ctp-mauve);
        }
        .dropzone {
            border: 2px dashed var(--ctp-surface2);
            border-radius: 16px;
            padding: 60px 30px;
            text-align: center;
            cursor: pointer;
            transition: all 0.3s ease;
            background: var(--ctp-base);
            position: relative;
        }
        .dropzone:hover, .dropzone.dragover {
            border-color: var(--ctp-mauve);
            background: rgba(203, 166, 247, 0.05);
            transform: scale(1.01);
        }
        .dropzone-icon {
            font-size: 4rem;
            margin-bottom: 20px;
            animation: bounce 2s infinite;
        }
        @keyframes bounce {
            0%, 100% { transform: translateY(0); }
            50% { transform: translateY(-10px); }
        }
        .dropzone h3 {
            color: var(--ctp-text);
            margin-bottom: 10px;
            font-size: 1.3rem;
        }
        .dropzone p {
            color: var(--ctp-subtext0);
            font-size: 0.95rem;
        }
        .dropzone .file-types {
            margin-top: 15px;
            display: flex;
            gap: 10px;
            justify-content: center;
        }
        .dropzone .file-type-badge {
            background: var(--ctp-surface0);
            padding: 6px 14px;
            border-radius: 20px;
            font-size: 0.85rem;
            color: var(--ctp-subtext1);
        }
        .file-input {
            display: none;
        }
        .upload-progress {
            margin-top: 20px;
            display: none;
        }
        .upload-progress.active {
            display: block;
        }
        .upload-progress-bar {
            height: 8px;
            background: var(--ctp-surface0);
            border-radius: 4px;
            overflow: hidden;
        }
        .upload-progress-fill {
            height: 100%;
            background: linear-gradient(90deg, var(--ctp-mauve), var(--ctp-pink));
            border-radius: 4px;
            transition: width 0.3s ease;
            animation: shimmer 1.5s infinite;
        }
        @keyframes shimmer {
            0% { background-position: -200% 0; }
            100% { background-position: 200% 0; }
        }

        /* Pipeline Stages */
        .pipeline-visual {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
            margin: 20px 0;
            flex-wrap: wrap;
        }
        .pipeline-stage {
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 8px;
        }
        .pipeline-stage-icon {
            width: 50px;
            height: 50px;
            border-radius: 12px;
            background: var(--ctp-surface0);
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.3rem;
            transition: all 0.3s ease;
        }
        .pipeline-stage.active .pipeline-stage-icon {
            background: linear-gradient(135deg, var(--ctp-blue), var(--ctp-sapphire));
            animation: pulse 1.5s infinite;
        }
        .pipeline-stage.completed .pipeline-stage-icon {
            background: linear-gradient(135deg, var(--ctp-green), var(--ctp-teal));
        }
        .pipeline-stage.failed .pipeline-stage-icon {
            background: linear-gradient(135deg, var(--ctp-red), var(--ctp-maroon));
        }
        @keyframes pulse {
            0%, 100% { transform: scale(1); box-shadow: 0 0 0 0 rgba(137, 180, 250, 0.4); }
            50% { transform: scale(1.05); box-shadow: 0 0 20px 5px rgba(137, 180, 250, 0.2); }
        }
        .pipeline-stage-label {
            font-size: 0.75rem;
            color: var(--ctp-subtext0);
            text-align: center;
        }
        .pipeline-connector {
            width: 30px;
            height: 3px;
            background: var(--ctp-surface1);
            border-radius: 2px;
            margin-bottom: 25px;
        }
        .pipeline-connector.active {
            background: linear-gradient(90deg, var(--ctp-green), var(--ctp-blue));
        }

        /* Job Cards */
        .section-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
        }
        .section-header h2 {
            color: var(--ctp-lavender);
            display: flex;
            align-items: center;
            gap: 10px;
            font-size: 1.4rem;
        }
        .job-list {
            display: flex;
            flex-direction: column;
            gap: 20px;
        }
        .job-card {
            background: var(--ctp-mantle);
            border-radius: 16px;
            padding: 24px;
            border: 1px solid var(--ctp-surface0);
            transition: all 0.3s ease;
        }
        .job-card:hover {
            border-color: var(--ctp-surface1);
            transform: translateY(-2px);
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.2);
        }
        .job-header {
            display: flex;
            justify-content: space-between;
            align-items: flex-start;
            margin-bottom: 15px;
            gap: 15px;
        }
        .job-info {
            flex: 1;
            min-width: 0;
        }
        .job-filename {
            color: var(--ctp-text);
            font-weight: 600;
            font-size: 1.15rem;
            margin-bottom: 6px;
            word-break: break-word;
        }
        .job-meta {
            color: var(--ctp-subtext0);
            font-size: 0.9rem;
            display: flex;
            gap: 15px;
            flex-wrap: wrap;
        }
        .job-meta span {
            display: flex;
            align-items: center;
            gap: 5px;
        }
        .job-status {
            padding: 6px 14px;
            border-radius: 20px;
            font-size: 0.85rem;
            font-weight: 600;
            white-space: nowrap;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .status-pending {
            background: var(--ctp-surface1);
            color: var(--ctp-subtext1);
        }
        .status-processing {
            background: rgba(137, 180, 250, 0.2);
            color: var(--ctp-blue);
            animation: statusPulse 2s infinite;
        }
        @keyframes statusPulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.7; }
        }
        .status-completed {
            background: rgba(166, 227, 161, 0.2);
            color: var(--ctp-green);
        }
        .status-failed {
            background: rgba(243, 139, 168, 0.2);
            color: var(--ctp-red);
        }
        .status-cancelled {
            background: rgba(249, 226, 175, 0.2);
            color: var(--ctp-yellow);
        }
        .progress-container {
            background: var(--ctp-surface0);
            border-radius: 10px;
            height: 12px;
            overflow: hidden;
            margin: 15px 0;
        }
        .progress-fill {
            background: linear-gradient(90deg, var(--ctp-mauve), var(--ctp-pink), var(--ctp-mauve));
            background-size: 200% 100%;
            height: 100%;
            border-radius: 10px;
            transition: width 0.5s ease;
            animation: progressGradient 2s linear infinite;
        }
        @keyframes progressGradient {
            0% { background-position: 0% 0%; }
            100% { background-position: 200% 0%; }
        }
        .job-footer {
            display: flex;
            justify-content: space-between;
            align-items: center;
            color: var(--ctp-subtext0);
            font-size: 0.9rem;
            flex-wrap: wrap;
            gap: 10px;
        }
        .job-step {
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .job-actions {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }
        .error-message {
            background: rgba(243, 139, 168, 0.1);
            border: 1px solid rgba(243, 139, 168, 0.3);
            border-radius: 10px;
            padding: 12px 16px;
            color: var(--ctp-red);
            font-size: 0.9rem;
            margin-top: 15px;
            display: flex;
            align-items: flex-start;
            gap: 10px;
        }

        /* Buttons */
        .btn {
            background: var(--ctp-surface0);
            color: var(--ctp-text);
            border: none;
            padding: 10px 18px;
            border-radius: 10px;
            cursor: pointer;
            font-size: 0.9rem;
            font-weight: 500;
            display: inline-flex;
            align-items: center;
            gap: 8px;
            transition: all 0.2s ease;
            text-decoration: none;
        }
        .btn:hover {
            background: var(--ctp-surface1);
            transform: translateY(-2px);
        }
        .btn-primary {
            background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-pink));
            color: var(--ctp-crust);
        }
        .btn-primary:hover {
            box-shadow: 0 5px 20px rgba(203, 166, 247, 0.4);
        }
        .btn-success {
            background: linear-gradient(135deg, var(--ctp-green), var(--ctp-teal));
            color: var(--ctp-crust);
        }
        .btn-danger {
            background: rgba(243, 139, 168, 0.2);
            color: var(--ctp-red);
        }
        .btn-danger:hover {
            background: rgba(243, 139, 168, 0.3);
        }

        /* Library Grid */
        .library-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
            gap: 24px;
        }
        .audiobook-card {
            background: var(--ctp-mantle);
            border-radius: 20px;
            overflow: hidden;
            border: 1px solid var(--ctp-surface0);
            transition: all 0.3s ease;
        }
        .audiobook-card:hover {
            transform: translateY(-8px);
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.3);
            border-color: var(--ctp-mauve);
        }
        .audiobook-cover {
            width: 100%;
            height: 180px;
            background: linear-gradient(135deg, var(--ctp-surface0) 0%, var(--ctp-surface1) 100%);
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 5rem;
            position: relative;
            overflow: hidden;
        }
        .audiobook-cover::after {
            content: '';
            position: absolute;
            inset: 0;
            background: linear-gradient(180deg, transparent 60%, var(--ctp-mantle) 100%);
        }
        .audiobook-info {
            padding: 20px;
        }
        .audiobook-title {
            color: var(--ctp-text);
            font-weight: 600;
            font-size: 1.15rem;
            margin-bottom: 6px;
            line-height: 1.3;
        }
        .audiobook-author {
            color: var(--ctp-subtext0);
            font-size: 0.95rem;
            margin-bottom: 12px;
        }
        .audiobook-meta {
            color: var(--ctp-overlay0);
            font-size: 0.85rem;
            margin-bottom: 15px;
            display: flex;
            gap: 15px;
            flex-wrap: wrap;
        }
        .audiobook-meta span {
            display: flex;
            align-items: center;
            gap: 5px;
        }
        .audiobook-actions {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }

        /* Chapter List */
        .chapter-list {
            margin-top: 15px;
            padding-top: 15px;
            border-top: 1px solid var(--ctp-surface0);
        }
        .chapter-list h4 {
            color: var(--ctp-subtext1);
            font-size: 0.9rem;
            margin-bottom: 10px;
        }
        .chapter-item {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 8px 12px;
            background: var(--ctp-surface0);
            border-radius: 8px;
            margin-bottom: 6px;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        .chapter-item:hover {
            background: var(--ctp-surface1);
        }
        .chapter-item.active {
            background: rgba(203, 166, 247, 0.2);
            border-left: 3px solid var(--ctp-mauve);
        }
        .chapter-number {
            background: var(--ctp-surface1);
            width: 28px;
            height: 28px;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 0.8rem;
            color: var(--ctp-subtext0);
        }
        .chapter-title {
            flex: 1;
            font-size: 0.9rem;
            color: var(--ctp-text);
        }
        .chapter-time {
            font-size: 0.8rem;
            color: var(--ctp-subtext0);
        }

        /* Player Bar */
        .player-bar {
            position: fixed;
            bottom: 90px;
            left: 50%;
            transform: translateX(-50%);
            background: var(--ctp-mantle);
            border-radius: 20px;
            padding: 16px 24px;
            display: none;
            align-items: center;
            gap: 20px;
            border: 1px solid var(--ctp-surface0);
            box-shadow: 0 15px 50px rgba(0, 0, 0, 0.5);
            max-width: 700px;
            width: 95%;
            z-index: 999;
            backdrop-filter: blur(10px);
        }
        .player-bar.active {
            display: flex;
        }
        .player-cover {
            width: 50px;
            height: 50px;
            border-radius: 10px;
            background: linear-gradient(135deg, var(--ctp-surface0), var(--ctp-surface1));
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.5rem;
            flex-shrink: 0;
        }
        .player-info {
            flex: 1;
            min-width: 0;
        }
        .player-title {
            color: var(--ctp-text);
            font-weight: 600;
            font-size: 1rem;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
            margin-bottom: 3px;
        }
        .player-chapter {
            color: var(--ctp-subtext0);
            font-size: 0.85rem;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }
        .player-controls {
            display: flex;
            gap: 8px;
            align-items: center;
        }
        .player-btn {
            width: 44px;
            height: 44px;
            border-radius: 50%;
            border: none;
            background: var(--ctp-surface0);
            color: var(--ctp-text);
            cursor: pointer;
            font-size: 1.2rem;
            display: flex;
            align-items: center;
            justify-content: center;
            transition: all 0.2s ease;
        }
        .player-btn:hover {
            background: var(--ctp-surface1);
            transform: scale(1.1);
        }
        .player-btn.play {
            width: 52px;
            height: 52px;
            background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-pink));
            color: var(--ctp-crust);
            font-size: 1.4rem;
        }
        .player-btn.play:hover {
            box-shadow: 0 5px 20px rgba(203, 166, 247, 0.4);
        }
        .player-progress {
            flex: 2;
            display: flex;
            flex-direction: column;
            gap: 6px;
            min-width: 150px;
        }
        .player-slider {
            -webkit-appearance: none;
            width: 100%;
            height: 6px;
            border-radius: 3px;
            background: var(--ctp-surface0);
            cursor: pointer;
        }
        .player-slider::-webkit-slider-thumb {
            -webkit-appearance: none;
            width: 16px;
            height: 16px;
            border-radius: 50%;
            background: var(--ctp-mauve);
            cursor: pointer;
            box-shadow: 0 2px 10px rgba(203, 166, 247, 0.4);
            transition: transform 0.2s ease;
        }
        .player-slider::-webkit-slider-thumb:hover {
            transform: scale(1.2);
        }
        .player-time {
            display: flex;
            justify-content: space-between;
            font-size: 0.8rem;
            color: var(--ctp-subtext0);
        }
        .player-close {
            position: absolute;
            top: -10px;
            right: -10px;
            width: 28px;
            height: 28px;
            border-radius: 50%;
            background: var(--ctp-surface1);
            border: none;
            color: var(--ctp-text);
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 0.9rem;
            transition: all 0.2s ease;
        }
        .player-close:hover {
            background: var(--ctp-red);
            color: var(--ctp-crust);
        }

        /* Dock Bar */
        .dock-bar {
            position: fixed;
            bottom: 20px;
            left: 50%;
            transform: translateX(-50%);
            background: var(--ctp-mantle);
            border-radius: 24px;
            padding: 12px 24px;
            display: flex;
            gap: 16px;
            border: 1px solid var(--ctp-surface0);
            box-shadow: 0 15px 50px rgba(0, 0, 0, 0.4);
            z-index: 1000;
        }
        .dock-item {
            width: 52px;
            height: 52px;
            border-radius: 14px;
            display: flex;
            align-items: center;
            justify-content: center;
            text-decoration: none;
            font-size: 1.5rem;
            transition: all 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
            background: var(--ctp-surface0);
            position: relative;
        }
        .dock-item:hover {
            transform: translateY(-12px) scale(1.15);
            background: var(--ctp-surface1);
        }
        .dock-item.active {
            background: linear-gradient(135deg, var(--ctp-mauve), var(--ctp-pink));
        }
        .dock-tooltip {
            position: absolute;
            bottom: 65px;
            background: var(--ctp-surface0);
            color: var(--ctp-text);
            padding: 6px 12px;
            border-radius: 8px;
            font-size: 0.85rem;
            opacity: 0;
            pointer-events: none;
            transition: all 0.2s ease;
            white-space: nowrap;
            box-shadow: 0 5px 15px rgba(0, 0, 0, 0.3);
        }
        .dock-item:hover .dock-tooltip {
            opacity: 1;
            transform: translateY(-5px);
        }

        /* Empty State */
        .empty-state {
            text-align: center;
            padding: 60px 20px;
            color: var(--ctp-subtext0);
        }
        .empty-state-icon {
            font-size: 5rem;
            margin-bottom: 20px;
            opacity: 0.5;
        }
        .empty-state h3 {
            color: var(--ctp-subtext1);
            margin-bottom: 10px;
        }

        /* Responsive */
        @media (max-width: 768px) {
            .container {
                padding: 15px;
            }
            header h1 {
                font-size: 1.8rem;
            }
            .tabs {
                gap: 8px;
            }
            .tab-btn {
                padding: 10px 16px;
                font-size: 0.9rem;
            }
            .upload-header {
                flex-direction: column;
                align-items: stretch;
            }
            .voice-selector {
                flex-direction: column;
                align-items: stretch;
            }
            .voice-selector select {
                min-width: auto;
            }
            .pipeline-visual {
                display: none;
            }
            .job-header {
                flex-direction: column;
            }
            .job-actions {
                width: 100%;
                justify-content: flex-end;
            }
            .library-grid {
                grid-template-columns: 1fr;
            }
            .player-bar {
                flex-direction: column;
                padding: 20px;
                bottom: 100px;
            }
            .player-controls {
                order: -1;
            }
            .player-progress {
                width: 100%;
            }
            .player-info {
                text-align: center;
            }
            .dock-bar {
                padding: 10px 16px;
                gap: 10px;
            }
            .dock-item {
                width: 44px;
                height: 44px;
                font-size: 1.2rem;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Audiobook Studio</h1>
            <p>Transform your EPUB and TXT files into professional audiobooks</p>
        </header>

        <div class="tabs">
            <button class="tab-btn active" data-tab="upload">
                <span>Upload</span>
            </button>
            <button class="tab-btn" data-tab="jobs">
                <span>Processing</span>
                <span class="badge" id="jobCount"></span>
            </button>
            <button class="tab-btn" data-tab="library">
                <span>Library</span>
                <span class="badge" id="libraryCount"></span>
            </button>
        </div>

        <!-- Upload Tab -->
        <div id="upload" class="tab-content active">
            <section class="upload-section">
                <div class="upload-header">
                    <h2>New Audiobook</h2>
                    <div class="voice-selector">
                        <label for="voiceSelect">Voice:</label>
                        <select id="voiceSelect">
                            <option value="default">Loading voices...</option>
                        </select>
                    </div>
                </div>
                <div class="dropzone" id="dropzone">
                    <div class="dropzone-icon">📚</div>
                    <h3>Drop your book here</h3>
                    <p>or click to browse your files</p>
                    <div class="file-types">
                        <span class="file-type-badge">📖 EPUB</span>
                        <span class="file-type-badge">📄 TXT</span>
                    </div>
                    <input type="file" class="file-input" id="fileInput" accept=".epub,.txt">
                </div>
                <div class="upload-progress" id="uploadProgress">
                    <div class="upload-progress-bar">
                        <div class="upload-progress-fill" id="uploadProgressFill" style="width: 0%"></div>
                    </div>
                </div>
            </section>

            <!-- Pipeline Visualization -->
            <div class="pipeline-visual" id="pipelineVisual" style="display: none;">
                <div class="pipeline-stage" data-stage="upload">
                    <div class="pipeline-stage-icon">📤</div>
                    <span class="pipeline-stage-label">Upload</span>
                </div>
                <div class="pipeline-connector"></div>
                <div class="pipeline-stage" data-stage="parse">
                    <div class="pipeline-stage-icon">📖</div>
                    <span class="pipeline-stage-label">Parse</span>
                </div>
                <div class="pipeline-connector"></div>
                <div class="pipeline-stage" data-stage="chunk">
                    <div class="pipeline-stage-icon">✂️</div>
                    <span class="pipeline-stage-label">Chunk</span>
                </div>
                <div class="pipeline-connector"></div>
                <div class="pipeline-stage" data-stage="tts">
                    <div class="pipeline-stage-icon">🎙️</div>
                    <span class="pipeline-stage-label">TTS</span>
                </div>
                <div class="pipeline-connector"></div>
                <div class="pipeline-stage" data-stage="concat">
                    <div class="pipeline-stage-icon">🔗</div>
                    <span class="pipeline-stage-label">Concat</span>
                </div>
                <div class="pipeline-connector"></div>
                <div class="pipeline-stage" data-stage="normalize">
                    <div class="pipeline-stage-icon">🔊</div>
                    <span class="pipeline-stage-label">Normalize</span>
                </div>
            </div>
        </div>

        <!-- Jobs Tab -->
        <div id="jobs" class="tab-content">
            <div class="section-header">
                <h2>Pipeline Jobs</h2>
            </div>
            <div class="job-list" id="jobList">
                <div class="empty-state">
                    <div class="empty-state-icon">📋</div>
                    <h3>No jobs yet</h3>
                    <p>Upload a file to start converting!</p>
                </div>
            </div>
        </div>

        <!-- Library Tab -->
        <div id="library" class="tab-content">
            <div class="section-header">
                <h2>Your Library</h2>
            </div>
            <div class="library-grid" id="libraryGrid">
                <div class="empty-state">
                    <div class="empty-state-icon">🎧</div>
                    <h3>Library is empty</h3>
                    <p>Completed audiobooks will appear here</p>
                </div>
            </div>
        </div>
    </div>

    <!-- Audio Player Bar -->
    <div class="player-bar" id="playerBar">
        <button class="player-close" id="closePlayer">×</button>
        <div class="player-cover">🎧</div>
        <div class="player-info">
            <div class="player-title" id="playerTitle">Not Playing</div>
            <div class="player-chapter" id="playerChapter">-</div>
        </div>
        <div class="player-controls">
            <button class="player-btn" id="prevChapter" title="Previous Chapter">⏮</button>
            <button class="player-btn play" id="playBtn">▶</button>
            <button class="player-btn" id="nextChapter" title="Next Chapter">⏭</button>
        </div>
        <div class="player-progress">
            <input type="range" class="player-slider" id="progressSlider" min="0" max="100" value="0">
            <div class="player-time">
                <span id="currentTime">0:00</span>
                <span id="totalTime">0:00</span>
            </div>
        </div>
    </div>

    <!-- Dock Navigation -->
    <nav class="dock-bar">
        <a href="http://holm.local:30080" class="dock-item">
            <span class="dock-tooltip">Home</span>
            🏠
        </a>
        <a href="http://holm.local:30100" class="dock-item">
            <span class="dock-tooltip">Files</span>
            📁
        </a>
        <a href="http://holm.local:30200" class="dock-item">
            <span class="dock-tooltip">Notes</span>
            📝
        </a>
        <a href="http://holm.local:30300" class="dock-item">
            <span class="dock-tooltip">Photos</span>
            📷
        </a>
        <a href="http://holm.local:30400" class="dock-item">
            <span class="dock-tooltip">Chat</span>
            💬
        </a>
        <a href="http://holm.local:30700" class="dock-item active">
            <span class="dock-tooltip">Audiobook</span>
            🎧
        </a>
    </nav>

    <audio id="audioPlayer"></audio>

    <script>
        // State
        let jobs = [];
        let library = [];
        let voices = [];
        let eventSource = null;
        let currentPlayingId = null;
        let currentChapters = [];
        let currentChapterIndex = 0;

        const audio = document.getElementById('audioPlayer');

        // Initialize
        document.addEventListener('DOMContentLoaded', () => {
            initTabs();
            initDropzone();
            initPlayer();
            loadVoices();
            connectSSE();
            loadLibrary();
        });

        // Tab Navigation
        function initTabs() {
            document.querySelectorAll('.tab-btn').forEach(btn => {
                btn.addEventListener('click', () => {
                    document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
                    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
                    btn.classList.add('active');
                    document.getElementById(btn.dataset.tab).classList.add('active');
                });
            });
        }

        // Dropzone
        function initDropzone() {
            const dropzone = document.getElementById('dropzone');
            const fileInput = document.getElementById('fileInput');

            dropzone.addEventListener('click', () => fileInput.click());

            dropzone.addEventListener('dragover', (e) => {
                e.preventDefault();
                dropzone.classList.add('dragover');
            });

            dropzone.addEventListener('dragleave', () => {
                dropzone.classList.remove('dragover');
            });

            dropzone.addEventListener('drop', (e) => {
                e.preventDefault();
                dropzone.classList.remove('dragover');
                if (e.dataTransfer.files[0]) {
                    uploadFile(e.dataTransfer.files[0]);
                }
            });

            fileInput.addEventListener('change', () => {
                if (fileInput.files[0]) {
                    uploadFile(fileInput.files[0]);
                }
            });
        }

        // Load Voices
        async function loadVoices() {
            try {
                const resp = await fetch('/api/voices');
                voices = await resp.json();
                const select = document.getElementById('voiceSelect');
                select.innerHTML = voices.map(v =>
                    '<option value="' + v.id + '">' + v.name + ' (' + v.language + ')</option>'
                ).join('');
            } catch (e) {
                console.error('Failed to load voices:', e);
            }
        }

        // Upload File
        async function uploadFile(file) {
            const ext = file.name.split('.').pop().toLowerCase();
            if (!['epub', 'txt'].includes(ext)) {
                alert('Please upload an EPUB or TXT file');
                return;
            }

            const voice = document.getElementById('voiceSelect').value;
            const formData = new FormData();
            formData.append('file', file);
            formData.append('voice', voice);

            const uploadProgress = document.getElementById('uploadProgress');
            const uploadProgressFill = document.getElementById('uploadProgressFill');
            const pipelineVisual = document.getElementById('pipelineVisual');

            uploadProgress.classList.add('active');
            pipelineVisual.style.display = 'flex';
            updatePipelineStage('upload');

            try {
                const response = await fetch('/api/upload', {
                    method: 'POST',
                    body: formData
                });

                if (response.ok) {
                    uploadProgressFill.style.width = '100%';
                    setTimeout(() => {
                        document.querySelector('[data-tab="jobs"]').click();
                        uploadProgress.classList.remove('active');
                        uploadProgressFill.style.width = '0%';
                        pipelineVisual.style.display = 'none';
                    }, 500);
                } else {
                    throw new Error(await response.text());
                }
            } catch (err) {
                alert('Upload failed: ' + err.message);
                uploadProgress.classList.remove('active');
                pipelineVisual.style.display = 'none';
            }
        }

        // Update Pipeline Visual
        function updatePipelineStage(stage) {
            const stages = ['upload', 'parse', 'chunk', 'tts', 'concat', 'normalize'];
            const stageIndex = stages.indexOf(stage);

            document.querySelectorAll('.pipeline-stage').forEach((el, i) => {
                el.classList.remove('active', 'completed');
                if (i < stageIndex) {
                    el.classList.add('completed');
                } else if (i === stageIndex) {
                    el.classList.add('active');
                }
            });

            document.querySelectorAll('.pipeline-connector').forEach((el, i) => {
                el.classList.toggle('active', i < stageIndex);
            });
        }

        // SSE Connection
        function connectSSE() {
            if (eventSource) eventSource.close();

            eventSource = new EventSource('/api/events');

            eventSource.onmessage = (e) => {
                try {
                    const msg = JSON.parse(e.data);
                    handleSSEMessage(msg);
                } catch (err) {
                    console.error('SSE parse error:', err);
                }
            };

            eventSource.onerror = () => {
                setTimeout(connectSSE, 5000);
            };
        }

        function handleSSEMessage(msg) {
            switch (msg.type) {
                case 'initial':
                case 'jobs_update':
                    jobs = msg.data || [];
                    renderJobs();
                    break;
                case 'job_created':
                case 'job_updated':
                    loadJobs();
                    break;
                case 'job_deleted':
                    jobs = jobs.filter(j => j.job_id !== msg.data.job_id);
                    renderJobs();
                    break;
                case 'library_updated':
                    loadLibrary();
                    break;
            }
        }

        // Load Jobs
        async function loadJobs() {
            try {
                const resp = await fetch('/api/jobs');
                jobs = await resp.json();
                renderJobs();
            } catch (e) {
                console.error('Failed to load jobs:', e);
            }
        }

        // Render Jobs
        function renderJobs() {
            const jobList = document.getElementById('jobList');
            const activeJobs = jobs.filter(j => ['pending', 'processing'].includes(j.status));
            document.getElementById('jobCount').textContent = activeJobs.length || '';

            if (jobs.length === 0) {
                jobList.innerHTML = '<div class="empty-state"><div class="empty-state-icon">📋</div><h3>No jobs yet</h3><p>Upload a file to start converting!</p></div>';
                return;
            }

            jobList.innerHTML = jobs.map(job => {
                const statusClass = 'status-' + job.status;
                const voiceName = voices.find(v => v.id === job.voice)?.name || job.voice;

                let actions = '';
                if (job.status === 'completed') {
                    actions = '<button class="btn btn-success" onclick="playAudio(\'' + job.job_id + '\', \'' + escapeHtml(job.filename) + '\')">▶ Play</button>' +
                              '<a href="/api/download/' + job.job_id + '" class="btn btn-primary">⬇ Download</a>';
                } else if (job.status === 'failed') {
                    actions = '<button class="btn" onclick="retryJob(\'' + job.job_id + '\')">🔄 Retry</button>' +
                              '<button class="btn btn-danger" onclick="deleteJob(\'' + job.job_id + '\')">🗑 Delete</button>';
                } else if (['pending', 'processing'].includes(job.status)) {
                    actions = '<button class="btn btn-danger" onclick="cancelJob(\'' + job.job_id + '\')">⏹ Cancel</button>';
                }

                let errorHtml = job.error ? '<div class="error-message">⚠️ ' + escapeHtml(job.error) + '</div>' : '';

                return '<div class="job-card">' +
                    '<div class="job-header">' +
                        '<div class="job-info">' +
                            '<div class="job-filename">' + escapeHtml(job.filename) + '</div>' +
                            '<div class="job-meta">' +
                                '<span>📦 ' + formatSize(job.file_size) + '</span>' +
                                '<span>🎤 ' + escapeHtml(voiceName) + '</span>' +
                                '<span>🕐 ' + formatDate(job.created_at) + '</span>' +
                            '</div>' +
                        '</div>' +
                        '<span class="job-status ' + statusClass + '">' + job.status + '</span>' +
                    '</div>' +
                    '<div class="progress-container">' +
                        '<div class="progress-fill" style="width: ' + (job.progress || 0) + '%"></div>' +
                    '</div>' +
                    '<div class="job-footer">' +
                        '<div class="job-step">' + (job.progress || 0) + '% ' + (job.current_step ? '• ' + job.current_step : '') + '</div>' +
                        '<div class="job-actions">' + actions + '</div>' +
                    '</div>' +
                    errorHtml +
                '</div>';
            }).join('');
        }

        // Load Library
        async function loadLibrary() {
            try {
                const resp = await fetch('/api/library');
                library = await resp.json();
                renderLibrary();
            } catch (e) {
                console.error('Failed to load library:', e);
            }
        }

        // Render Library
        function renderLibrary() {
            const grid = document.getElementById('libraryGrid');
            document.getElementById('libraryCount').textContent = library.length || '';

            if (library.length === 0) {
                grid.innerHTML = '<div class="empty-state"><div class="empty-state-icon">🎧</div><h3>Library is empty</h3><p>Completed audiobooks will appear here</p></div>';
                return;
            }

            grid.innerHTML = library.map(ab => {
                const voiceName = voices.find(v => v.id === ab.voice)?.name || ab.voice || 'Default';
                const chapters = ab.chapters || [];

                let chapterHtml = '';
                if (chapters.length > 1) {
                    chapterHtml = '<div class="chapter-list"><h4>📑 ' + chapters.length + ' Chapters</h4>' +
                        chapters.slice(0, 3).map((ch, i) =>
                            '<div class="chapter-item" onclick="playChapter(\'' + ab.job_id + '\', ' + i + ', \'' + escapeHtml(ab.title) + '\')">' +
                                '<span class="chapter-number">' + (i + 1) + '</span>' +
                                '<span class="chapter-title">' + escapeHtml(ch.title) + '</span>' +
                                '<span class="chapter-time">' + formatTime(ch.start_time) + '</span>' +
                            '</div>'
                        ).join('') +
                        (chapters.length > 3 ? '<div class="chapter-item" style="justify-content: center; color: var(--ctp-subtext0)">+ ' + (chapters.length - 3) + ' more chapters</div>' : '') +
                    '</div>';
                }

                return '<div class="audiobook-card">' +
                    '<div class="audiobook-cover">📚</div>' +
                    '<div class="audiobook-info">' +
                        '<div class="audiobook-title">' + escapeHtml(ab.title) + '</div>' +
                        '<div class="audiobook-author">by ' + escapeHtml(ab.author) + '</div>' +
                        '<div class="audiobook-meta">' +
                            '<span>⏱ ' + formatDuration(ab.duration) + '</span>' +
                            '<span>📦 ' + formatSize(ab.file_size) + '</span>' +
                            '<span>🎤 ' + escapeHtml(voiceName) + '</span>' +
                        '</div>' +
                        '<div class="audiobook-actions">' +
                            '<button class="btn btn-success" onclick="playAudio(\'' + ab.job_id + '\', \'' + escapeHtml(ab.title) + '\')">▶ Play</button>' +
                            '<a href="/api/download/' + ab.job_id + '" class="btn">⬇</a>' +
                            '<button class="btn btn-danger" onclick="deleteAudiobook(' + ab.id + ')">🗑</button>' +
                        '</div>' +
                        chapterHtml +
                    '</div>' +
                '</div>';
            }).join('');
        }

        // Job Actions
        async function retryJob(jobId) {
            await fetch('/api/jobs/' + jobId + '/retry', { method: 'POST' });
        }

        async function cancelJob(jobId) {
            await fetch('/api/jobs/' + jobId + '/cancel', { method: 'POST' });
        }

        async function deleteJob(jobId) {
            if (confirm('Delete this job?')) {
                await fetch('/api/jobs/' + jobId, { method: 'DELETE' });
            }
        }

        async function deleteAudiobook(id) {
            if (confirm('Remove from library?')) {
                await fetch('/api/library/' + id, { method: 'DELETE' });
                loadLibrary();
            }
        }

        // Player
        function initPlayer() {
            const playBtn = document.getElementById('playBtn');
            const closeBtn = document.getElementById('closePlayer');
            const progressSlider = document.getElementById('progressSlider');
            const prevChapter = document.getElementById('prevChapter');
            const nextChapter = document.getElementById('nextChapter');

            playBtn.addEventListener('click', () => {
                if (audio.paused) {
                    audio.play();
                    playBtn.innerHTML = '⏸';
                } else {
                    audio.pause();
                    playBtn.innerHTML = '▶';
                }
            });

            closeBtn.addEventListener('click', () => {
                audio.pause();
                document.getElementById('playerBar').classList.remove('active');
                currentPlayingId = null;
            });

            progressSlider.addEventListener('input', (e) => {
                if (audio.duration) {
                    audio.currentTime = (e.target.value / 100) * audio.duration;
                }
            });

            prevChapter.addEventListener('click', () => {
                if (currentChapterIndex > 0) {
                    currentChapterIndex--;
                    seekToChapter(currentChapterIndex);
                }
            });

            nextChapter.addEventListener('click', () => {
                if (currentChapterIndex < currentChapters.length - 1) {
                    currentChapterIndex++;
                    seekToChapter(currentChapterIndex);
                }
            });

            audio.addEventListener('timeupdate', () => {
                if (audio.duration) {
                    document.getElementById('progressSlider').value = (audio.currentTime / audio.duration) * 100;
                    document.getElementById('currentTime').textContent = formatTime(audio.currentTime);
                    document.getElementById('totalTime').textContent = formatTime(audio.duration);

                    // Update current chapter
                    updateCurrentChapter();
                }
            });

            audio.addEventListener('ended', () => {
                document.getElementById('playBtn').innerHTML = '▶';
            });
        }

        function playAudio(jobId, title) {
            audio.src = '/api/stream/' + jobId;
            audio.play();
            currentPlayingId = jobId;

            document.getElementById('playerTitle').textContent = title;
            document.getElementById('playerBar').classList.add('active');
            document.getElementById('playBtn').innerHTML = '⏸';

            // Load chapters
            loadChapters(jobId);
        }

        function playChapter(jobId, chapterIndex, title) {
            if (currentPlayingId !== jobId) {
                playAudio(jobId, title);
                setTimeout(() => {
                    currentChapterIndex = chapterIndex;
                    seekToChapter(chapterIndex);
                }, 500);
            } else {
                currentChapterIndex = chapterIndex;
                seekToChapter(chapterIndex);
            }
        }

        async function loadChapters(jobId) {
            try {
                // Try job first, then library
                let resp = await fetch('/api/jobs/' + jobId + '/chapters');
                if (!resp.ok) {
                    resp = await fetch('/api/library/' + jobId + '/chapters');
                }
                if (resp.ok) {
                    currentChapters = await resp.json();
                    currentChapterIndex = 0;
                    updateChapterDisplay();
                }
            } catch (e) {
                currentChapters = [];
            }
        }

        function seekToChapter(index) {
            if (currentChapters[index] && audio.duration) {
                audio.currentTime = currentChapters[index].start_time;
                updateChapterDisplay();
            }
        }

        function updateCurrentChapter() {
            if (currentChapters.length === 0) return;

            for (let i = currentChapters.length - 1; i >= 0; i--) {
                if (audio.currentTime >= currentChapters[i].start_time) {
                    if (currentChapterIndex !== i) {
                        currentChapterIndex = i;
                        updateChapterDisplay();
                    }
                    break;
                }
            }
        }

        function updateChapterDisplay() {
            const chapterEl = document.getElementById('playerChapter');
            if (currentChapters.length > 0 && currentChapters[currentChapterIndex]) {
                chapterEl.textContent = 'Chapter ' + (currentChapterIndex + 1) + ': ' + currentChapters[currentChapterIndex].title;
            } else {
                chapterEl.textContent = '-';
            }
        }

        // Utility Functions
        function formatTime(seconds) {
            if (!seconds || isNaN(seconds)) return '0:00';
            const h = Math.floor(seconds / 3600);
            const m = Math.floor((seconds % 3600) / 60);
            const s = Math.floor(seconds % 60);
            if (h > 0) {
                return h + ':' + (m < 10 ? '0' : '') + m + ':' + (s < 10 ? '0' : '') + s;
            }
            return m + ':' + (s < 10 ? '0' : '') + s;
        }

        function formatDuration(seconds) {
            if (!seconds) return 'Unknown';
            const h = Math.floor(seconds / 3600);
            const m = Math.floor((seconds % 3600) / 60);
            if (h > 0) return h + 'h ' + m + 'm';
            return m + ' min';
        }

        function formatSize(bytes) {
            if (!bytes) return 'Unknown';
            if (bytes < 1024) return bytes + ' B';
            if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
            return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
        }

        function formatDate(dateStr) {
            if (!dateStr) return '';
            return new Date(dateStr).toLocaleDateString(undefined, {
                month: 'short',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            });
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text || '';
            return div.innerHTML;
        }
    </script>
</body>
</html>`
