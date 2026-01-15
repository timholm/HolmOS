package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/lib/pq"
)

const uploadDir = "/data/audiobooks/uploads"

var db *sql.DB

type UploadResponse struct {
	JobID    string `json:"job_id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Status   string `json:"status"`
}

func main() {
	os.MkdirAll(uploadDir, 0755)

	// Initialize database connection
	dbHost := getEnv("DB_HOST", "postgres.holm.svc.cluster.local")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPass := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "holm")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName)

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Warning: Could not connect to database: %v", err)
	} else {
		if err = db.Ping(); err != nil {
			log.Printf("Warning: Database ping failed: %v", err)
		} else {
			log.Println("Connected to PostgreSQL database")
			// Ensure audiobook_jobs table exists
			ensureTable()
		}
	}

	http.HandleFunc("/upload", handleUpload)
	http.HandleFunc("/health", handleHealth)

	log.Println("Audiobook Upload EPUB service starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func ensureTable() {
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
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Printf("Warning: Could not ensure audiobook_jobs table: %v", err)
	}
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 100MB)
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

	// Validate file extension
	ext := filepath.Ext(header.Filename)
	if ext != ".epub" {
		http.Error(w, "Only EPUB files are allowed", http.StatusBadRequest)
		return
	}

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	// Generate job ID
	hash := sha256.Sum256(append(content, []byte(time.Now().String())...))
	jobID := hex.EncodeToString(hash[:16])

	// Save file
	filePath := filepath.Join(uploadDir, jobID+".epub")
	err = os.WriteFile(filePath, content, 0644)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	// Create job in PostgreSQL audiobook_jobs table
	status := "pending"
	if db != nil {
		_, err = db.Exec(`
			INSERT INTO audiobook_jobs (job_id, filename, file_type, file_path, file_size, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, jobID, header.Filename, "epub", filePath, len(content), status, time.Now())
		if err != nil {
			log.Printf("Warning: Could not create job in database: %v", err)
		} else {
			log.Printf("Job %s created in database", jobID)
		}
	}

	// Trigger audiobook-parse-epub service asynchronously
	go triggerParseEpub(jobID, filePath)

	response := UploadResponse{
		JobID:    jobID,
		Filename: header.Filename,
		Size:     int64(len(content)),
		Status:   status,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func triggerParseEpub(jobID, filePath string) {
	client := &http.Client{Timeout: 30 * time.Second}

	data := map[string]string{
		"job_id":    jobID,
		"file_path": filePath,
	}
	jsonData, _ := json.Marshal(data)

	parseURL := getEnv("PARSE_EPUB_URL", "http://audiobook-parse-epub.holm.svc.cluster.local:8080/parse")

	resp, err := client.Post(parseURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to trigger parse-epub service: %v", err)
		updateJobStatus(jobID, "failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Parse-epub service returned status %d", resp.StatusCode)
		updateJobStatus(jobID, "failed")
		return
	}

	log.Printf("Successfully triggered parse-epub for job %s", jobID)
	updateJobStatus(jobID, "processing")
}

func updateJobStatus(jobID, status string) {
	if db == nil {
		return
	}
	_, err := db.Exec(`
		UPDATE audiobook_jobs SET status = $1, updated_at = $2 WHERE job_id = $3
	`, status, time.Now(), jobID)
	if err != nil {
		log.Printf("Failed to update job status: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
