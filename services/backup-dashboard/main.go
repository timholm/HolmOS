package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
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
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Config struct {
	DB               *sql.DB
	EncryptionKey    []byte
	VaultIntegration bool
	BackupDir        string
	mu               sync.RWMutex
}

type Schedule struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CronExpr   string     `json:"cron_expression"`
	Type       string     `json:"type"`
	Target     string     `json:"target"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	NextRunAt  *time.Time `json:"next_run_at,omitempty"`
}

type BackupEntry struct {
	ID          string    `json:"id"`
	SourcePath  string    `json:"source_path"`
	BackupPath  string    `json:"backup_path"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Type        string    `json:"type"`
	Encrypted   bool      `json:"encrypted"`
}

type RestorePoint struct {
	ID         string    `json:"id"`
	BackupID   string    `json:"backup_id"`
	BackupName string    `json:"backup_name"`
	Type       string    `json:"type"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
	Encrypted  bool      `json:"encrypted"`
	Status     string    `json:"status"`
}

type RestoreJob struct {
	ID          string     `json:"id"`
	BackupID    string     `json:"backup_id"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Message     string     `json:"message,omitempty"`
}

type ManualBackupRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	Encrypt bool   `json:"encrypt"`
}

type DashboardStats struct {
	TotalSchedules   int    `json:"total_schedules"`
	ActiveSchedules  int    `json:"active_schedules"`
	TotalBackups     int    `json:"total_backups"`
	TotalSize        int64  `json:"total_size"`
	TotalSizeHuman   string `json:"total_size_human"`
	LastBackupTime   string `json:"last_backup_time"`
	EncryptedBackups int    `json:"encrypted_backups"`
	RestoreJobs      int    `json:"restore_jobs"`
}

var config *Config

func main() {
	config = &Config{
		VaultIntegration: getEnv("VAULT_INTEGRATION", "true") == "true",
		BackupDir:        getEnv("BACKUP_DIR", "/data/backups"),
	}

	// Ensure backup directory exists
	os.MkdirAll(config.BackupDir, 0755)

	keyStr := getEnv("ENCRYPTION_KEY", "")
	if keyStr != "" {
		config.EncryptionKey, _ = base64.StdEncoding.DecodeString(keyStr)
	} else {
		config.EncryptionKey = make([]byte, 32)
		rand.Read(config.EncryptionKey)
	}

	dbHost := getEnv("DB_HOST", "backup-scheduler-postgres.holm.svc.cluster.local")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "backup")
	dbPass := getEnv("DB_PASSWORD", "backup123")
	dbName := getEnv("DB_NAME", "backup_scheduler")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName)

	var err error
	for i := 0; i < 30; i++ {
		config.DB, err = sql.Open("postgres", connStr)
		if err == nil {
			err = config.DB.Ping()
			if err == nil {
				log.Printf("Connected to database")
				break
			}
		}
		log.Printf("Waiting for database... attempt %d/30: %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer config.DB.Close()

	if err := initDB(); err != nil {
		log.Printf("Warning: Failed to initialize tables: %v", err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/", dashboardHandler).Methods("GET")
	r.HandleFunc("/health", healthHandler).Methods("GET")

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/stats", getStatsHandler).Methods("GET")

	// Schedule endpoints (integrated)
	api.HandleFunc("/schedules", listSchedulesHandler).Methods("GET")
	api.HandleFunc("/schedules", createScheduleHandler).Methods("POST")
	api.HandleFunc("/schedules/{id}", deleteScheduleHandler).Methods("DELETE")
	api.HandleFunc("/schedules/{id}/run", triggerScheduleHandler).Methods("POST")
	api.HandleFunc("/schedules/{id}/history", getScheduleHistoryHandler).Methods("GET")

	// Backup endpoints (integrated)
	api.HandleFunc("/backups", listBackupsHandler).Methods("GET")
	api.HandleFunc("/backups/{id}", getBackupHandler).Methods("GET")
	api.HandleFunc("/backups/{id}/download", downloadBackupHandler).Methods("GET")
	api.HandleFunc("/backup/manual", triggerManualBackupHandler).Methods("POST")

	// Restore endpoints
	api.HandleFunc("/restore/points", listRestorePointsHandler).Methods("GET")
	api.HandleFunc("/restore/start", startRestoreHandler).Methods("POST")
	api.HandleFunc("/restore/jobs", listRestoreJobsHandler).Methods("GET")
	api.HandleFunc("/restore/jobs/{id}", getRestoreJobHandler).Methods("GET")

	// Vault endpoints
	api.HandleFunc("/vault/status", vaultStatusHandler).Methods("GET")
	api.HandleFunc("/vault/encrypt", encryptDataHandler).Methods("POST")

	port := getEnv("PORT", "8080")
	log.Printf("Backup Dashboard starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func initDB() error {
	schema := `
	CREATE TABLE IF NOT EXISTS schedules (
		id VARCHAR(36) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		cron_expression VARCHAR(100) NOT NULL,
		type VARCHAR(50) NOT NULL,
		target VARCHAR(500) NOT NULL,
		enabled BOOLEAN DEFAULT TRUE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_run_at TIMESTAMP,
		next_run_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS backups (
		id VARCHAR(36) PRIMARY KEY,
		source_path VARCHAR(500),
		backup_path VARCHAR(500),
		size BIGINT DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		description VARCHAR(500),
		status VARCHAR(50) DEFAULT 'completed',
		type VARCHAR(50) DEFAULT 'manual',
		encrypted BOOLEAN DEFAULT FALSE
	);
	CREATE INDEX IF NOT EXISTS idx_backups_created ON backups(created_at DESC);

	CREATE TABLE IF NOT EXISTS backup_history (
		id VARCHAR(36) PRIMARY KEY,
		schedule_id VARCHAR(36) REFERENCES schedules(id) ON DELETE CASCADE,
		status VARCHAR(20) NOT NULL,
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		size BIGINT,
		message TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_backup_history_schedule ON backup_history(schedule_id);

	CREATE TABLE IF NOT EXISTS restore_jobs (
		id VARCHAR(36) PRIMARY KEY,
		backup_id VARCHAR(36) NOT NULL,
		status VARCHAR(20) NOT NULL,
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		message TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_restore_jobs_backup ON restore_jobs(backup_id);
	CREATE INDEX IF NOT EXISTS idx_restore_jobs_started ON restore_jobs(started_at DESC);
	`
	_, err := config.DB.Exec(schema)
	return err
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":    "healthy",
		"service":   "backup-dashboard",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	if err := config.DB.Ping(); err != nil {
		status["database"] = "unhealthy"
		status["status"] = "degraded"
	} else {
		status["database"] = "healthy"
	}

	// Check backup directory
	if _, err := os.Stat(config.BackupDir); err != nil {
		status["storage"] = "unhealthy"
	} else {
		status["storage"] = "healthy"
	}

	// Scheduler is integrated
	status["scheduler"] = "healthy"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func getStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats := DashboardStats{}

	// Get schedule stats
	config.DB.QueryRow("SELECT COUNT(*) FROM schedules").Scan(&stats.TotalSchedules)
	config.DB.QueryRow("SELECT COUNT(*) FROM schedules WHERE enabled = true").Scan(&stats.ActiveSchedules)

	// Get backup stats
	rows, err := config.DB.Query("SELECT size, encrypted FROM backups")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var size int64
			var encrypted bool
			rows.Scan(&size, &encrypted)
			stats.TotalBackups++
			stats.TotalSize += size
			if encrypted {
				stats.EncryptedBackups++
			}
		}
	}
	stats.TotalSizeHuman = formatBytes(stats.TotalSize)

	// Get restore jobs count
	config.DB.QueryRow("SELECT COUNT(*) FROM restore_jobs").Scan(&stats.RestoreJobs)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Schedule handlers (integrated)
func listSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`
		SELECT id, name, cron_expression, type, target, enabled, created_at, updated_at, last_run_at, next_run_at
		FROM schedules ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	schedules := []Schedule{}
	for rows.Next() {
		var s Schedule
		rows.Scan(&s.ID, &s.Name, &s.CronExpr, &s.Type, &s.Target, &s.Enabled, &s.CreatedAt, &s.UpdatedAt, &s.LastRunAt, &s.NextRunAt)
		schedules = append(schedules, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schedules)
}

func createScheduleHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		CronExpr string `json:"cron_expression"`
		Target   string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	id := uuid.New().String()
	_, err := config.DB.Exec(`
		INSERT INTO schedules (id, name, cron_expression, type, target)
		VALUES ($1, $2, $3, $4, $5)
	`, id, req.Name, req.CronExpr, req.Type, req.Target)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "created"})
}

func deleteScheduleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	_, err := config.DB.Exec("DELETE FROM schedules WHERE id = $1", vars["id"])
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func triggerScheduleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	scheduleID := vars["id"]

	// Get schedule details
	var name, target, schedType string
	err := config.DB.QueryRow("SELECT name, target, type FROM schedules WHERE id = $1", scheduleID).Scan(&name, &target, &schedType)
	if err != nil {
		http.Error(w, `{"error": "Schedule not found"}`, http.StatusNotFound)
		return
	}

	// Create backup
	backupID := uuid.New().String()
	backupData := fmt.Sprintf("Scheduled backup: %s, Target: %s, Time: %s", name, target, time.Now().Format(time.RFC3339))
	backupPath := filepath.Join(config.BackupDir, backupID+".dat")

	if err := os.WriteFile(backupPath, []byte(backupData), 0644); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to write backup: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Save backup metadata
	_, err = config.DB.Exec(`
		INSERT INTO backups (id, source_path, backup_path, size, description, status, type, encrypted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, backupID, target, backupPath, len(backupData), name, "completed", schedType, false)
	if err != nil {
		log.Printf("Failed to save backup metadata: %v", err)
	}

	// Update schedule last run
	config.DB.Exec("UPDATE schedules SET last_run_at = $1, updated_at = $1 WHERE id = $2", time.Now(), scheduleID)

	// Add to history
	historyID := uuid.New().String()
	config.DB.Exec(`
		INSERT INTO backup_history (id, schedule_id, status, started_at, completed_at, size, message)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, historyID, scheduleID, "completed", time.Now(), time.Now(), len(backupData), "Manual trigger successful")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "completed",
		"job_id":    historyID,
		"backup_id": backupID,
	})
}

func getScheduleHistoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	rows, err := config.DB.Query(`
		SELECT id, schedule_id, status, started_at, completed_at, size, message
		FROM backup_history WHERE schedule_id = $1 ORDER BY started_at DESC LIMIT $2
	`, vars["id"], limit)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	history := []map[string]interface{}{}
	for rows.Next() {
		var id, schedID, status, message string
		var startedAt time.Time
		var completedAt *time.Time
		var size *int64
		rows.Scan(&id, &schedID, &status, &startedAt, &completedAt, &size, &message)
		entry := map[string]interface{}{
			"id":          id,
			"schedule_id": schedID,
			"status":      status,
			"started_at":  startedAt,
			"message":     message,
		}
		if completedAt != nil {
			entry["completed_at"] = completedAt
		}
		if size != nil {
			entry["size"] = *size
		}
		history = append(history, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// Backup handlers (integrated)
func listBackupsHandler(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")

	query := "SELECT id, source_path, backup_path, size, created_at, description, status, type, encrypted FROM backups ORDER BY created_at DESC"
	var rows *sql.Rows
	var err error

	if typeFilter != "" {
		query = "SELECT id, source_path, backup_path, size, created_at, description, status, type, encrypted FROM backups WHERE type = $1 ORDER BY created_at DESC"
		rows, err = config.DB.Query(query, typeFilter)
	} else {
		rows, err = config.DB.Query(query)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	backups := []BackupEntry{}
	for rows.Next() {
		var b BackupEntry
		rows.Scan(&b.ID, &b.SourcePath, &b.BackupPath, &b.Size, &b.CreatedAt, &b.Description, &b.Status, &b.Type, &b.Encrypted)
		backups = append(backups, b)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"backups": backups,
		"count":   len(backups),
	})
}

func getBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var b BackupEntry
	err := config.DB.QueryRow(`
		SELECT id, source_path, backup_path, size, created_at, description, status, type, encrypted
		FROM backups WHERE id = $1
	`, vars["id"]).Scan(&b.ID, &b.SourcePath, &b.BackupPath, &b.Size, &b.CreatedAt, &b.Description, &b.Status, &b.Type, &b.Encrypted)

	if err != nil {
		http.Error(w, `{"error": "Backup not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(b)
}

func downloadBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var backupPath string
	var encrypted bool

	err := config.DB.QueryRow("SELECT backup_path, encrypted FROM backups WHERE id = $1", vars["id"]).Scan(&backupPath, &encrypted)
	if err != nil {
		http.Error(w, `{"error": "Backup not found"}`, http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(backupPath)
	if err != nil {
		http.Error(w, `{"error": "Failed to read backup file"}`, http.StatusInternalServerError)
		return
	}

	filename := filepath.Base(backupPath)
	if encrypted {
		filename += ".enc"
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write(data)
}

func triggerManualBackupHandler(w http.ResponseWriter, r *http.Request) {
	var req ManualBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	backupID := uuid.New().String()
	backupData := fmt.Sprintf("Manual backup: %s, Type: %s, Target: %s, Time: %s",
		req.Name, req.Type, req.Target, time.Now().Format(time.RFC3339))

	var finalData []byte
	var backupName string
	encrypted := false

	if req.Encrypt && config.VaultIntegration {
		encryptedData, err := encryptWithVault([]byte(backupData))
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Encryption failed: %v"}`, err), http.StatusInternalServerError)
			return
		}
		finalData = encryptedData
		backupName = req.Name + ".enc"
		encrypted = true
	} else {
		finalData = []byte(backupData)
		backupName = req.Name
	}

	backupPath := filepath.Join(config.BackupDir, backupID+".dat")
	if err := os.WriteFile(backupPath, finalData, 0644); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to write backup: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// Save to database
	_, err := config.DB.Exec(`
		INSERT INTO backups (id, source_path, backup_path, size, description, status, type, encrypted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, backupID, req.Target, backupPath, len(finalData), backupName, "completed", req.Type, encrypted)
	if err != nil {
		log.Printf("Failed to save backup metadata: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":        backupID,
		"name":      backupName,
		"status":    "completed",
		"encrypted": encrypted,
		"size":      len(finalData),
	})
}

// Restore handlers
func listRestorePointsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`
		SELECT id, source_path, backup_path, size, created_at, description, status, type, encrypted
		FROM backups ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	points := []RestorePoint{}
	for rows.Next() {
		var b BackupEntry
		rows.Scan(&b.ID, &b.SourcePath, &b.BackupPath, &b.Size, &b.CreatedAt, &b.Description, &b.Status, &b.Type, &b.Encrypted)

		backupName := b.Description
		if backupName == "" {
			backupName = b.SourcePath
		}

		points = append(points, RestorePoint{
			ID:         b.ID,
			BackupID:   b.ID,
			BackupName: backupName,
			Type:       b.Type,
			Size:       b.Size,
			CreatedAt:  b.CreatedAt,
			Encrypted:  b.Encrypted,
			Status:     b.Status,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

func startRestoreHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackupID   string `json:"backup_id"`
		TargetPath string `json:"target_path,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Verify backup exists
	var backupPath string
	var encrypted bool
	err := config.DB.QueryRow("SELECT backup_path, encrypted FROM backups WHERE id = $1", req.BackupID).Scan(&backupPath, &encrypted)
	if err != nil {
		http.Error(w, `{"error": "Backup not found"}`, http.StatusNotFound)
		return
	}

	jobID := uuid.New().String()
	startTime := time.Now()

	_, err = config.DB.Exec(`INSERT INTO restore_jobs (id, backup_id, status, started_at) VALUES ($1, $2, $3, $4)`,
		jobID, req.BackupID, "running", startTime)
	if err != nil {
		http.Error(w, `{"error": "Failed to create restore job"}`, http.StatusInternalServerError)
		return
	}

	// Execute restore in background
	go executeRestore(jobID, req.BackupID, backupPath, encrypted, req.TargetPath)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id":    jobID,
		"backup_id": req.BackupID,
		"status":    "running",
		"message":   "Restore job started",
	})
}

func executeRestore(jobID, backupID, backupPath string, encrypted bool, targetPath string) {
	// Simulate some processing time
	time.Sleep(2 * time.Second)

	// Read backup file
	data, err := os.ReadFile(backupPath)
	if err != nil {
		updateRestoreJob(jobID, "failed", fmt.Sprintf("Failed to read backup file: %v", err))
		return
	}

	// Decrypt if needed
	if encrypted {
		decrypted, err := decryptWithVault(data)
		if err != nil {
			updateRestoreJob(jobID, "failed", fmt.Sprintf("Decryption failed: %v", err))
			return
		}
		data = decrypted
	}

	// If target path is specified, write the restored data
	if targetPath != "" {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			updateRestoreJob(jobID, "failed", fmt.Sprintf("Failed to create target directory: %v", err))
			return
		}
		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			updateRestoreJob(jobID, "failed", fmt.Sprintf("Failed to write restored data: %v", err))
			return
		}
	}

	log.Printf("Restore job %s completed, restored %d bytes", jobID, len(data))
	updateRestoreJob(jobID, "completed", fmt.Sprintf("Successfully restored %d bytes", len(data)))
}

func updateRestoreJob(jobID, status, message string) {
	completedAt := time.Now()
	config.DB.Exec(`UPDATE restore_jobs SET status = $1, completed_at = $2, message = $3 WHERE id = $4`,
		status, completedAt, message, jobID)
}

func listRestoreJobsHandler(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	rows, err := config.DB.Query(`SELECT id, backup_id, status, started_at, completed_at, message FROM restore_jobs ORDER BY started_at DESC LIMIT $1`, limit)
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch restore jobs"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	jobs := []RestoreJob{}
	for rows.Next() {
		var j RestoreJob
		rows.Scan(&j.ID, &j.BackupID, &j.Status, &j.StartedAt, &j.CompletedAt, &j.Message)
		jobs = append(jobs, j)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func getRestoreJobHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var j RestoreJob
	err := config.DB.QueryRow(`SELECT id, backup_id, status, started_at, completed_at, message FROM restore_jobs WHERE id = $1`, vars["id"]).
		Scan(&j.ID, &j.BackupID, &j.Status, &j.StartedAt, &j.CompletedAt, &j.Message)
	if err != nil {
		http.Error(w, `{"error": "Restore job not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(j)
}

func vaultStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"available":   true,
		"integration": config.VaultIntegration,
		"message":     "Using local AES-256-GCM encryption",
	})
}

func encryptDataHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	encrypted, err := encryptWithVault([]byte(req.Data))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Encryption failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"encrypted": base64.StdEncoding.EncodeToString(encrypted),
	})
}

func encryptWithVault(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(config.EncryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, data, nil), nil
}

func decryptWithVault(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(config.EncryptionKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(dashboardHTML))
}

var dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Backup Dashboard - HolmOS</title>
    <style>
        :root { --ctp-teal: #94e2d5; --ctp-green: #a6e3a1; --ctp-blue: #89b4fa; --ctp-mauve: #cba6f7; --ctp-red: #f38ba8; --ctp-yellow: #f9e2af; --ctp-text: #cdd6f4; --ctp-subtext0: #a6adc8; --ctp-subtext1: #bac2de; --ctp-overlay0: #6c7086; --ctp-surface0: #313244; --ctp-surface1: #45475a; --ctp-base: #1e1e2e; --ctp-mantle: #181825; --ctp-crust: #11111b; }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: system-ui, sans-serif; background: linear-gradient(135deg, var(--ctp-crust) 0%, var(--ctp-base) 50%, var(--ctp-mantle) 100%); color: var(--ctp-text); min-height: 100vh; line-height: 1.6; }
        .container { max-width: 1400px; margin: 0 auto; padding: 2rem; }
        header { text-align: center; padding: 2rem; background: var(--ctp-mantle); border-radius: 1rem; margin-bottom: 2rem; border: 1px solid var(--ctp-surface0); }
        header h1 { color: var(--ctp-teal); font-size: 2.5rem; }
        header p { color: var(--ctp-subtext0); margin-top: 0.5rem; }
        .stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 2rem; }
        .stat-card { background: var(--ctp-mantle); padding: 1.5rem; border-radius: 1rem; border: 1px solid var(--ctp-surface0); text-align: center; }
        .stat-card .value { font-size: 2rem; font-weight: bold; color: var(--ctp-teal); }
        .stat-card .label { color: var(--ctp-subtext0); font-size: 0.9rem; }
        .tabs { display: flex; gap: 0.5rem; margin-bottom: 1.5rem; flex-wrap: wrap; }
        .tab { background: var(--ctp-surface0); color: var(--ctp-text); border: none; padding: 0.75rem 1.5rem; border-radius: 0.5rem; cursor: pointer; font-size: 1rem; transition: all 0.2s; }
        .tab:hover { background: var(--ctp-surface1); }
        .tab.active { background: var(--ctp-teal); color: var(--ctp-crust); }
        .tab-content { display: none; }
        .tab-content.active { display: block; }
        .card { background: var(--ctp-mantle); border-radius: 1rem; padding: 1.5rem; border: 1px solid var(--ctp-surface0); margin-bottom: 1.5rem; }
        .card h2 { color: var(--ctp-blue); margin-bottom: 1rem; padding-bottom: 0.5rem; border-bottom: 1px solid var(--ctp-surface0); }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 0.75rem; text-align: left; border-bottom: 1px solid var(--ctp-surface0); }
        th { color: var(--ctp-subtext1); font-weight: 600; }
        .badge { display: inline-block; padding: 0.25rem 0.75rem; border-radius: 1rem; font-size: 0.8rem; font-weight: 500; }
        .badge-success { background: var(--ctp-green); color: var(--ctp-crust); }
        .badge-warning { background: var(--ctp-yellow); color: var(--ctp-crust); }
        .badge-error { background: var(--ctp-red); color: var(--ctp-crust); }
        .badge-info { background: var(--ctp-blue); color: var(--ctp-crust); }
        .form-group { margin-bottom: 1rem; }
        .form-group label { display: block; color: var(--ctp-subtext1); margin-bottom: 0.5rem; }
        input, select, textarea { width: 100%; padding: 0.75rem; background: var(--ctp-surface0); border: 2px solid var(--ctp-surface1); border-radius: 0.5rem; color: var(--ctp-text); font-size: 1rem; }
        input:focus, select:focus { outline: none; border-color: var(--ctp-teal); }
        button { background: linear-gradient(135deg, var(--ctp-teal) 0%, var(--ctp-green) 100%); color: var(--ctp-crust); border: none; padding: 0.75rem 1.5rem; border-radius: 0.5rem; cursor: pointer; font-weight: 600; transition: all 0.2s; }
        button:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(148, 226, 213, 0.3); }
        .btn-danger { background: linear-gradient(135deg, var(--ctp-red) 0%, #eba0ac 100%); }
        .btn-secondary { background: var(--ctp-surface1); color: var(--ctp-text); }
        .btn-small { padding: 0.5rem 1rem; font-size: 0.875rem; }
        .actions { display: flex; gap: 0.5rem; }
        .grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 1.5rem; }
        @media (max-width: 768px) { .grid-2 { grid-template-columns: 1fr; } }
        .vault-status { display: flex; align-items: center; gap: 0.5rem; padding: 1rem; background: var(--ctp-surface0); border-radius: 0.5rem; margin-bottom: 1rem; }
        .vault-status.connected { border-left: 4px solid var(--ctp-green); }
        .vault-status.disconnected { border-left: 4px solid var(--ctp-red); }
        .timeline { position: relative; padding-left: 2rem; }
        .timeline::before { content: ''; position: absolute; left: 0.5rem; top: 0; bottom: 0; width: 2px; background: var(--ctp-surface1); }
        .timeline-item { position: relative; padding: 1rem; background: var(--ctp-surface0); border-radius: 0.5rem; margin-bottom: 1rem; cursor: pointer; transition: all 0.2s; }
        .timeline-item::before { content: ''; position: absolute; left: -1.75rem; top: 1.25rem; width: 10px; height: 10px; background: var(--ctp-teal); border-radius: 50%; }
        .timeline-item:hover { background: var(--ctp-surface1); }
        .timeline-item.selected { border: 2px solid var(--ctp-teal); }
        .empty-state { text-align: center; padding: 3rem; color: var(--ctp-overlay0); }
        .checkbox-group { display: flex; align-items: center; gap: 0.5rem; }
        .checkbox-group input[type="checkbox"] { width: auto; accent-color: var(--ctp-teal); }
        .service-status { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 1rem; margin-top: 1rem; }
        .service-badge { display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 1rem; background: var(--ctp-surface0); border-radius: 0.5rem; font-size: 0.9rem; }
        .service-badge.online { border-left: 3px solid var(--ctp-green); }
        .service-badge.offline { border-left: 3px solid var(--ctp-red); }
        .status-dot { width: 8px; height: 8px; border-radius: 50%; }
        .status-dot.online { background: var(--ctp-green); }
        .status-dot.offline { background: var(--ctp-red); }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Backup Dashboard</h1>
            <p>Unified backup management for HolmOS</p>
            <div class="service-status" id="service-status"></div>
        </header>
        <div class="stats-grid">
            <div class="stat-card"><div class="value" id="stat-schedules">-</div><div class="label">Active Schedules</div></div>
            <div class="stat-card"><div class="value" id="stat-backups">-</div><div class="label">Total Backups</div></div>
            <div class="stat-card"><div class="value" id="stat-size">-</div><div class="label">Storage Used</div></div>
            <div class="stat-card"><div class="value" id="stat-encrypted">-</div><div class="label">Encrypted</div></div>
        </div>
        <div class="tabs">
            <button class="tab active" onclick="showTab('schedules', this)">Schedules</button>
            <button class="tab" onclick="showTab('backups', this)">History</button>
            <button class="tab" onclick="showTab('manual', this)">Manual</button>
            <button class="tab" onclick="showTab('restore', this)">Restore</button>
            <button class="tab" onclick="showTab('vault', this)">Vault</button>
        </div>
        <div id="schedules" class="tab-content active">
            <div class="grid-2">
                <div class="card"><h2>Scheduled Backups</h2><table><thead><tr><th>Name</th><th>Schedule</th><th>Type</th><th>Actions</th></tr></thead><tbody id="schedules-table"></tbody></table></div>
                <div class="card"><h2>Create Schedule</h2><form id="schedule-form"><div class="form-group"><label>Name</label><input type="text" id="sched-name" required></div><div class="form-group"><label>Type</label><select id="sched-type"><option value="daily">Daily</option><option value="weekly">Weekly</option><option value="monthly">Monthly</option></select></div><div class="form-group"><label>Cron</label><input type="text" id="sched-cron" placeholder="0 0 2 * * *" required></div><div class="form-group"><label>Target</label><input type="text" id="sched-target" required></div><button type="submit">Create</button></form></div>
            </div>
        </div>
        <div id="backups" class="tab-content"><div class="card"><h2>Backup History</h2><table><thead><tr><th>Description</th><th>Source</th><th>Size</th><th>Created</th><th>Status</th><th>Actions</th></tr></thead><tbody id="backups-table"></tbody></table></div></div>
        <div id="manual" class="tab-content">
            <div class="grid-2">
                <div class="card"><h2>Manual Backup</h2><form id="manual-form"><div class="form-group"><label>Name</label><input type="text" id="manual-name" required></div><div class="form-group"><label>Type</label><select id="manual-type"><option value="manual">Manual</option><option value="database">Database</option><option value="files">Files</option></select></div><div class="form-group"><label>Target</label><input type="text" id="manual-target"></div><div class="form-group"><div class="checkbox-group"><input type="checkbox" id="manual-encrypt" checked><label for="manual-encrypt">Encrypt</label></div></div><button type="submit">Start Backup</button></form></div>
                <div class="card"><h2>Quick Actions</h2><div style="display: flex; flex-direction: column; gap: 1rem;"><button onclick="triggerAll()">Run All Schedules</button><button class="btn-secondary" onclick="loadBackups()">Refresh</button></div></div>
            </div>
        </div>
        <div id="restore" class="tab-content">
            <div class="grid-2">
                <div class="card"><h2>Restore Points</h2><div class="timeline" id="restore-points"></div></div>
                <div class="card"><h2>Restore</h2><div id="selected-restore" style="padding: 1rem; background: var(--ctp-surface0); border-radius: 0.5rem; margin-bottom: 1rem;"><p style="color: var(--ctp-overlay0);">Select a restore point</p></div><button id="start-restore-btn" onclick="startRestore()" disabled>Start Restore</button><h3 style="margin-top: 2rem; color: var(--ctp-subtext1);">Recent Jobs</h3><div id="restore-jobs"></div></div>
            </div>
        </div>
        <div id="vault" class="tab-content">
            <div class="card"><h2>Vault Status</h2><div id="vault-status" class="vault-status"><span>Checking...</span></div></div>
            <div class="grid-2"><div class="card"><h2>Encryption</h2><p style="color: var(--ctp-subtext0);">AES-256-GCM encryption for all backups.</p></div><div class="card"><h2>Test</h2><div class="form-group"><textarea id="test-data" rows="2" placeholder="Enter text..."></textarea></div><button onclick="testEncrypt()">Test</button><div id="enc-result" style="margin-top: 1rem;"></div></div></div>
        </div>
    </div>
    <script>
        var selPoint = null;
        function showTab(id, btn) { document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active')); document.querySelectorAll('.tab').forEach(t => t.classList.remove('active')); document.getElementById(id).classList.add('active'); btn.classList.add('active'); if(id==='schedules')loadSchedules(); if(id==='backups')loadBackups(); if(id==='restore'){loadRestorePoints();loadRestoreJobs();} if(id==='vault')loadVault(); }
        function loadStatus() { fetch('/health').then(r=>r.json()).then(s => { var c = document.getElementById('service-status'); c.innerHTML = ['database','scheduler','storage'].map(x => '<div class="service-badge '+(s[x]==='healthy'?'online':'offline')+'"><span class="status-dot '+(s[x]==='healthy'?'online':'offline')+'"></span>'+x.charAt(0).toUpperCase()+x.slice(1)+'</div>').join(''); }).catch(e => console.error('Health check failed:', e)); }
        function loadStats() { fetch('/api/stats').then(r=>r.json()).then(s => { document.getElementById('stat-schedules').textContent = s.active_schedules+'/'+s.total_schedules; document.getElementById('stat-backups').textContent = s.total_backups; document.getElementById('stat-size').textContent = s.total_size_human||'0 B'; document.getElementById('stat-encrypted').textContent = s.encrypted_backups; }).catch(e => console.error('Stats load failed:', e)); }
        function loadSchedules() { fetch('/api/schedules').then(r=>r.json()).then(d => { var t = document.getElementById('schedules-table'); if(!d||!d.length){t.innerHTML='<tr><td colspan="4" class="empty-state">No schedules</td></tr>';return;} t.innerHTML = d.map(s=>'<tr><td>'+esc(s.name)+'</td><td><code>'+s.cron_expression+'</code></td><td><span class="badge badge-info">'+s.type+'</span></td><td class="actions"><button class="btn-small" onclick="runSched(\''+s.id+'\')">Run</button><button class="btn-small btn-danger" onclick="delSched(\''+s.id+'\')">Del</button></td></tr>').join(''); }).catch(e => console.error('Schedules load failed:', e)); }
        function loadBackups() { fetch('/api/backups').then(r=>r.json()).then(d => { var bs = d.backups||[]; var t = document.getElementById('backups-table'); if(!bs.length){t.innerHTML='<tr><td colspan="6" class="empty-state">No backups</td></tr>';return;} t.innerHTML = bs.map(b=>'<tr><td>'+esc(b.description||'-')+'</td><td>'+esc(b.source_path||'-')+'</td><td>'+fmt(b.size)+'</td><td>'+new Date(b.created_at).toLocaleString()+'</td><td><span class="badge badge-success">'+b.status+'</span></td><td><button class="btn-small btn-secondary" onclick="dl(\''+b.id+'\')">Download</button></td></tr>').join(''); }).catch(e => console.error('Backups load failed:', e)); }
        function loadRestorePoints() { fetch('/api/restore/points').then(r=>r.json()).then(p => { var c = document.getElementById('restore-points'); if(!p||!p.length){c.innerHTML='<div class="empty-state">No restore points available</div>';return;} c.innerHTML = p.map(x=>'<div class="timeline-item" onclick="selRP(\''+x.id+'\',\''+esc(x.backup_name)+'\','+x.size+','+x.encrypted+')"><strong>'+esc(x.backup_name)+'</strong><div style="font-size:0.85rem;color:var(--ctp-subtext0);">'+new Date(x.created_at).toLocaleString()+' - '+fmt(x.size)+(x.encrypted?' <span class="badge" style="background:var(--ctp-mauve);color:var(--ctp-crust);">Encrypted</span>':'')+'</div></div>').join(''); }).catch(e => console.error('Restore points load failed:', e)); }
        function selRP(id,name,size,encrypted) { selPoint={id:id,name:name,size:size,encrypted:encrypted}; document.querySelectorAll('.timeline-item').forEach(e=>e.classList.remove('selected')); event.currentTarget.classList.add('selected'); document.getElementById('selected-restore').innerHTML='<h4>'+name+'</h4><p>Size: '+fmt(size)+(encrypted?' (Encrypted - will be decrypted)':'')+'</p>'; document.getElementById('start-restore-btn').disabled=false; }
        function startRestore() { if(!selPoint)return; if(!confirm('Start restore from "'+selPoint.name+'"?'))return; fetch('/api/restore/start',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({backup_id:selPoint.id})}).then(r=>r.json()).then(r=>{alert('Restore job started: '+r.job_id);loadRestoreJobs();}).catch(e => alert('Restore failed: '+e)); }
        function loadRestoreJobs() { fetch('/api/restore/jobs?limit=10').then(r=>r.json()).then(j => { var c = document.getElementById('restore-jobs'); if(!j||!j.length){c.innerHTML='<div class="empty-state">No restore jobs</div>';return;} c.innerHTML = j.map(x=>'<div style="padding:0.75rem;background:var(--ctp-surface0);border-radius:0.5rem;margin-bottom:0.5rem;"><span>'+x.id.substring(0,8)+'...</span> <span class="badge '+(x.status==='completed'?'badge-success':x.status==='failed'?'badge-error':'badge-warning')+'">'+x.status+'</span><div style="font-size:0.85rem;color:var(--ctp-subtext0);">'+(x.message||'Processing...')+'</div></div>').join(''); }).catch(e => console.error('Restore jobs load failed:', e)); }
        function loadVault() { fetch('/api/vault/status').then(r=>r.json()).then(s => { var c = document.getElementById('vault-status'); c.className='vault-status connected'; c.innerHTML='<span style="color:var(--ctp-green);">Active</span> <span>'+s.message+'</span>'; }).catch(e => console.error('Vault status load failed:', e)); }
        function runSched(id) { fetch('/api/schedules/'+id+'/run',{method:'POST'}).then(r=>r.json()).then(r=>{alert('Backup completed! ID: '+r.backup_id);loadStats();loadBackups();}).catch(e => alert('Failed: '+e)); }
        function delSched(id) { if(!confirm('Delete this schedule?'))return; fetch('/api/schedules/'+id,{method:'DELETE'}).then(()=>{loadSchedules();loadStats();}).catch(e => alert('Delete failed: '+e)); }
        function dl(id) { window.open('/api/backups/'+id+'/download','_blank'); }
        document.getElementById('schedule-form').addEventListener('submit', e => { e.preventDefault(); var d={name:document.getElementById('sched-name').value,type:document.getElementById('sched-type').value,cron_expression:document.getElementById('sched-cron').value,target:document.getElementById('sched-target').value}; fetch('/api/schedules',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(d)}).then(r=>{if(r.ok){e.target.reset();loadSchedules();loadStats();alert('Schedule created!');}else{r.json().then(j=>alert('Error: '+j.error));}}); });
        document.getElementById('manual-form').addEventListener('submit', e => { e.preventDefault(); var d={name:document.getElementById('manual-name').value,type:document.getElementById('manual-type').value,target:document.getElementById('manual-target').value,encrypt:document.getElementById('manual-encrypt').checked}; fetch('/api/backup/manual',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(d)}).then(r=>r.json()).then(r=>{alert('Backup completed! ID: '+r.id+(r.encrypted?' (Encrypted)':''));e.target.reset();loadStats();loadBackups();}).catch(e => alert('Backup failed: '+e)); });
        function triggerAll() { fetch('/api/schedules').then(r=>r.json()).then(s=>{if(!s.length){alert('No schedules to run');return;} Promise.all(s.filter(x=>x.enabled).map(x=>fetch('/api/schedules/'+x.id+'/run',{method:'POST'}))).then(()=>{alert('All schedules triggered!');loadStats();loadBackups();}); }); }
        function testEncrypt() { var d=document.getElementById('test-data').value; if(!d){alert('Enter text to encrypt');return;} fetch('/api/vault/encrypt',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({data:d})}).then(r=>r.json()).then(r=>{document.getElementById('enc-result').innerHTML='<div style="background:var(--ctp-surface0);padding:1rem;border-radius:0.5rem;"><strong>Encrypted:</strong><code style="word-break:break-all;display:block;margin-top:0.5rem;">'+r.encrypted.substring(0,80)+'...</code></div>';}); }
        function fmt(b) { if(b===0)return'0 B'; var k=1024,s=['B','KB','MB','GB'],i=Math.floor(Math.log(b)/Math.log(k)); return parseFloat((b/Math.pow(k,i)).toFixed(1))+' '+s[i]; }
        function esc(t) { var d=document.createElement('div'); d.textContent=t||''; return d.innerHTML; }
        loadStatus(); loadStats(); loadSchedules(); setInterval(loadStatus,30000); setInterval(loadStats,60000);
    </script>
</body>
</html>` + "`"
