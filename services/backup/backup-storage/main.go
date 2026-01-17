package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Backup struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Size      int64     `json:"size"`
	FilePath  string    `json:"file_path,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateBackupRequest struct {
	Name string `json:"name"`
	Data string `json:"data"` // Base64 encoded data
	Type string `json:"type"`
}

type BackupResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Database  string `json:"database"`
	Storage   string `json:"storage"`
}

var db *sql.DB
var backupDir = "/data/backups"

func main() {
	// Get configuration from environment
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "backups")
	serverPort := getEnv("SERVER_PORT", "8080")
	backupDir = getEnv("BACKUP_DIR", "/data/backups")

	// Create backup directory if it doesn't exist
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		log.Fatalf("Failed to create backup directory: %v", err)
	}

	// Connect to PostgreSQL
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	var err error
	for i := 0; i < 30; i++ {
		db, err = sql.Open("postgres", connStr)
		if err == nil {
			err = db.Ping()
			if err == nil {
				break
			}
		}
		log.Printf("Waiting for database... attempt %d/30", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	log.Println("Connected to database")

	// Initialize database schema
	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Set up router
	r := mux.NewRouter()

	// API routes
	r.HandleFunc("/health", healthHandler).Methods("GET")
	r.HandleFunc("/backups", createBackupHandler).Methods("POST")
	r.HandleFunc("/backups", listBackupsHandler).Methods("GET")

	// Register additional API routes (must be before generic {id} routes)
	registerAPIRoutes(r)

	r.HandleFunc("/backups/{id}", getBackupHandler).Methods("GET")
	r.HandleFunc("/backups/{id}/download", downloadBackupHandler).Methods("GET")
	r.HandleFunc("/backups/{id}", deleteBackupHandler).Methods("DELETE")

	// Start server
	addr := ":" + serverPort
	log.Printf("Starting backup-storage service on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func initDB() error {
	schema := `
	CREATE TABLE IF NOT EXISTS backups (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		type VARCHAR(100) NOT NULL,
		size BIGINT NOT NULL DEFAULT 0,
		file_path VARCHAR(500) NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_backups_name ON backups(name);
	CREATE INDEX IF NOT EXISTS idx_backups_type ON backups(type);
	CREATE INDEX IF NOT EXISTS idx_backups_created_at ON backups(created_at);
	`
	_, err := db.Exec(schema)
	return err
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	health := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Database:  "healthy",
		Storage:   "healthy",
	}

	// Check database connection
	if err := db.Ping(); err != nil {
		health.Status = "unhealthy"
		health.Database = "unhealthy: " + err.Error()
	}

	// Check storage directory
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		health.Status = "unhealthy"
		health.Storage = "unhealthy: backup directory does not exist"
	}

	w.Header().Set("Content-Type", "application/json")
	if health.Status != "healthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(health)
}

func createBackupHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		sendError(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.Data == "" {
		sendError(w, "Data is required", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = "generic"
	}

	// Decode base64 data
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		sendError(w, "Invalid base64 data", http.StatusBadRequest)
		return
	}

	// Generate unique ID and file path
	id := uuid.New().String()
	fileName := fmt.Sprintf("%s_%s", id, sanitizeFileName(req.Name))
	filePath := filepath.Join(backupDir, fileName)

	// Write data to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		log.Printf("Failed to write backup file: %v", err)
		sendError(w, "Failed to store backup", http.StatusInternalServerError)
		return
	}

	// Get file size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Printf("Failed to get file info: %v", err)
		sendError(w, "Failed to store backup", http.StatusInternalServerError)
		return
	}

	// Insert metadata into database
	now := time.Now().UTC()
	_, err = db.Exec(`
		INSERT INTO backups (id, name, type, size, file_path, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, req.Name, req.Type, fileInfo.Size(), filePath, now, now)
	if err != nil {
		// Clean up file on database error
		os.Remove(filePath)
		log.Printf("Failed to insert backup metadata: %v", err)
		sendError(w, "Failed to store backup metadata", http.StatusInternalServerError)
		return
	}

	response := BackupResponse{
		ID:        id,
		Name:      req.Name,
		Type:      req.Type,
		Size:      fileInfo.Size(),
		CreatedAt: now,
		UpdatedAt: now,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

func listBackupsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	typeFilter := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 100
	offset := 0
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	var rows *sql.Rows
	var err error

	if typeFilter != "" {
		rows, err = db.Query(`
			SELECT id, name, type, size, created_at, updated_at
			FROM backups
			WHERE type = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3`, typeFilter, limit, offset)
	} else {
		rows, err = db.Query(`
			SELECT id, name, type, size, created_at, updated_at
			FROM backups
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2`, limit, offset)
	}

	if err != nil {
		log.Printf("Failed to query backups: %v", err)
		sendError(w, "Failed to retrieve backups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	backups := []BackupResponse{}
	for rows.Next() {
		var b BackupResponse
		if err := rows.Scan(&b.ID, &b.Name, &b.Type, &b.Size, &b.CreatedAt, &b.UpdatedAt); err != nil {
			log.Printf("Failed to scan backup row: %v", err)
			continue
		}
		backups = append(backups, b)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(backups)
}

func getBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var b BackupResponse
	err := db.QueryRow(`
		SELECT id, name, type, size, created_at, updated_at
		FROM backups WHERE id = $1`, id).Scan(
		&b.ID, &b.Name, &b.Type, &b.Size, &b.CreatedAt, &b.UpdatedAt)

	if err == sql.ErrNoRows {
		sendError(w, "Backup not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Failed to query backup: %v", err)
		sendError(w, "Failed to retrieve backup", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(b)
}

func downloadBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var backup Backup
	err := db.QueryRow(`
		SELECT id, name, type, size, file_path, created_at, updated_at
		FROM backups WHERE id = $1`, id).Scan(
		&backup.ID, &backup.Name, &backup.Type, &backup.Size, &backup.FilePath,
		&backup.CreatedAt, &backup.UpdatedAt)

	if err == sql.ErrNoRows {
		sendError(w, "Backup not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Failed to query backup: %v", err)
		sendError(w, "Failed to retrieve backup", http.StatusInternalServerError)
		return
	}

	// Open the file
	file, err := os.Open(backup.FilePath)
	if err != nil {
		log.Printf("Failed to open backup file: %v", err)
		sendError(w, "Backup file not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	// Set headers for download
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", backup.Name))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(backup.Size, 10))

	// Stream the file
	io.Copy(w, file)
}

func deleteBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Get file path first
	var filePath string
	err := db.QueryRow(`SELECT file_path FROM backups WHERE id = $1`, id).Scan(&filePath)
	if err == sql.ErrNoRows {
		sendError(w, "Backup not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Failed to query backup: %v", err)
		sendError(w, "Failed to delete backup", http.StatusInternalServerError)
		return
	}

	// Delete from database
	result, err := db.Exec(`DELETE FROM backups WHERE id = $1`, id)
	if err != nil {
		log.Printf("Failed to delete backup from database: %v", err)
		sendError(w, "Failed to delete backup", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		sendError(w, "Backup not found", http.StatusNotFound)
		return
	}

	// Delete file (log error but don't fail the request)
	if err := os.Remove(filePath); err != nil {
		log.Printf("Warning: Failed to delete backup file %s: %v", filePath, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

func sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

func sanitizeFileName(name string) string {
	// Replace unsafe characters
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "..", "_")
	return name
}
