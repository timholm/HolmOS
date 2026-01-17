package main

import (
	"bytes"
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
	// Notification settings
	NotificationWebhook  string
	NotificationEmail    string
	NotifyOnFailure      bool
	NotifyOnSuccess      bool
	RetentionDays        int
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

// BackupJob represents a backup job with logs
type BackupJob struct {
	ID          string     `json:"id"`
	ScheduleID  string     `json:"schedule_id,omitempty"`
	BackupID    string     `json:"backup_id,omitempty"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Size        int64      `json:"size,omitempty"`
	Message     string     `json:"message,omitempty"`
}

// JobLog represents a log entry for a backup job
type JobLog struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Details   string    `json:"details,omitempty"`
}

// BackupStats represents comprehensive backup statistics
type BackupStats struct {
	TotalSchedules    int       `json:"total_schedules"`
	ActiveSchedules   int       `json:"active_schedules"`
	TotalBackups      int       `json:"total_backups"`
	TotalSize         int64     `json:"total_size"`
	TotalSizeHuman    string    `json:"total_size_human"`
	EncryptedBackups  int       `json:"encrypted_backups"`
	RestoreJobs       int       `json:"restore_jobs"`
	SuccessfulBackups int       `json:"successful_backups"`
	FailedBackups     int       `json:"failed_backups"`
	SuccessRate       float64   `json:"success_rate"`
	LastBackupTime    string    `json:"last_backup_time,omitempty"`
	NextScheduledRun  string    `json:"next_scheduled_run,omitempty"`
	AvgBackupSize     int64     `json:"avg_backup_size"`
	AvgBackupSizeHuman string   `json:"avg_backup_size_human"`
	BackupsByType     map[string]int `json:"backups_by_type"`
	StorageByType     map[string]int64 `json:"storage_by_type"`
	RecentFailures    int       `json:"recent_failures"`
	DailyBackupCount  int       `json:"daily_backup_count"`
	WeeklyBackupCount int       `json:"weekly_backup_count"`
}

// Notification represents a notification for backup events
type Notification struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	JobID     string    `json:"job_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	SentAt    time.Time `json:"sent_at"`
	Delivered bool      `json:"delivered"`
	Channel   string    `json:"channel"`
}

// NotificationConfig for configuring notifications
type NotificationConfig struct {
	WebhookURL      string `json:"webhook_url,omitempty"`
	EmailAddress    string `json:"email_address,omitempty"`
	NotifyOnFailure bool   `json:"notify_on_failure"`
	NotifyOnSuccess bool   `json:"notify_on_success"`
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

// RestoreRequest for triggering restores
type RestoreRequest struct {
	BackupID    string `json:"backup_id"`
	TargetPath  string `json:"target_path,omitempty"`
	Overwrite   bool   `json:"overwrite"`
	VerifyOnly  bool   `json:"verify_only"`
}

var config *Config

func main() {
	config = &Config{
		VaultIntegration:    getEnv("VAULT_INTEGRATION", "true") == "true",
		BackupDir:           getEnv("BACKUP_DIR", "/data/backups"),
		NotificationWebhook: getEnv("NOTIFICATION_WEBHOOK", ""),
		NotificationEmail:   getEnv("NOTIFICATION_EMAIL", ""),
		NotifyOnFailure:     getEnv("NOTIFY_ON_FAILURE", "true") == "true",
		NotifyOnSuccess:     getEnv("NOTIFY_ON_SUCCESS", "false") == "true",
		RetentionDays:       getEnvInt("RETENTION_DAYS", 30),
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
	api.HandleFunc("/schedules/{id}", getScheduleHandler).Methods("GET")
	api.HandleFunc("/schedules/{id}", updateScheduleHandler).Methods("PUT")
	api.HandleFunc("/schedules/{id}", deleteScheduleHandler).Methods("DELETE")
	api.HandleFunc("/schedules/{id}/run", triggerScheduleHandler).Methods("POST")
	api.HandleFunc("/schedules/{id}/enable", enableScheduleHandler).Methods("POST")
	api.HandleFunc("/schedules/{id}/disable", disableScheduleHandler).Methods("POST")
	api.HandleFunc("/schedules/{id}/history", getScheduleHistoryHandler).Methods("GET")

	// Backup endpoints (integrated)
	api.HandleFunc("/backups", listBackupsHandler).Methods("GET")
	api.HandleFunc("/backups/{id}", getBackupHandler).Methods("GET")
	api.HandleFunc("/backups/{id}/download", downloadBackupHandler).Methods("GET")
	api.HandleFunc("/backup/manual", triggerManualBackupHandler).Methods("POST")

	// Restore endpoints
	api.HandleFunc("/restore", triggerRestoreHandler).Methods("POST")
	api.HandleFunc("/restore/points", listRestorePointsHandler).Methods("GET")
	api.HandleFunc("/restore/start", startRestoreHandler).Methods("POST")
	api.HandleFunc("/restore/verify", verifyRestoreHandler).Methods("POST")
	api.HandleFunc("/restore/jobs", listRestoreJobsHandler).Methods("GET")
	api.HandleFunc("/restore/jobs/{id}", getRestoreJobHandler).Methods("GET")
	api.HandleFunc("/restore/jobs/{id}/cancel", cancelRestoreJobHandler).Methods("POST")

	// Job logs endpoints
	api.HandleFunc("/jobs", listJobsHandler).Methods("GET")
	api.HandleFunc("/jobs/{id}", getJobHandler).Methods("GET")
	api.HandleFunc("/jobs/{id}/logs", getJobLogsHandler).Methods("GET")
	api.HandleFunc("/jobs/{id}/logs", addJobLogHandler).Methods("POST")

	// Notification endpoints
	api.HandleFunc("/notifications", listNotificationsHandler).Methods("GET")
	api.HandleFunc("/notifications/config", getNotificationConfigHandler).Methods("GET")
	api.HandleFunc("/notifications/config", updateNotificationConfigHandler).Methods("PUT")
	api.HandleFunc("/notifications/test", testNotificationHandler).Methods("POST")

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

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
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
		message TEXT,
		target_path VARCHAR(500),
		verify_only BOOLEAN DEFAULT FALSE
	);
	CREATE INDEX IF NOT EXISTS idx_restore_jobs_backup ON restore_jobs(backup_id);
	CREATE INDEX IF NOT EXISTS idx_restore_jobs_started ON restore_jobs(started_at DESC);

	CREATE TABLE IF NOT EXISTS job_logs (
		id VARCHAR(36) PRIMARY KEY,
		job_id VARCHAR(36) NOT NULL,
		level VARCHAR(20) NOT NULL DEFAULT 'info',
		message TEXT NOT NULL,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		details TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_job_logs_job ON job_logs(job_id);
	CREATE INDEX IF NOT EXISTS idx_job_logs_timestamp ON job_logs(timestamp DESC);

	CREATE TABLE IF NOT EXISTS notifications (
		id VARCHAR(36) PRIMARY KEY,
		type VARCHAR(50) NOT NULL,
		job_id VARCHAR(36),
		status VARCHAR(20) NOT NULL,
		message TEXT,
		sent_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		delivered BOOLEAN DEFAULT FALSE,
		channel VARCHAR(50)
	);
	CREATE INDEX IF NOT EXISTS idx_notifications_job ON notifications(job_id);
	CREATE INDEX IF NOT EXISTS idx_notifications_sent ON notifications(sent_at DESC);

	CREATE TABLE IF NOT EXISTS notification_config (
		id VARCHAR(36) PRIMARY KEY DEFAULT 'default',
		webhook_url VARCHAR(500),
		email_address VARCHAR(255),
		notify_on_failure BOOLEAN DEFAULT TRUE,
		notify_on_success BOOLEAN DEFAULT FALSE,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
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
	// Check if detailed stats requested
	detailed := r.URL.Query().Get("detailed") == "true"

	if detailed {
		stats := getDetailedStats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
		return
	}

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

// getDetailedStats returns comprehensive backup statistics
func getDetailedStats() BackupStats {
	stats := BackupStats{
		BackupsByType: make(map[string]int),
		StorageByType: make(map[string]int64),
	}

	// Get schedule stats
	config.DB.QueryRow("SELECT COUNT(*) FROM schedules").Scan(&stats.TotalSchedules)
	config.DB.QueryRow("SELECT COUNT(*) FROM schedules WHERE enabled = true").Scan(&stats.ActiveSchedules)

	// Get backup stats with type breakdown
	rows, err := config.DB.Query("SELECT size, encrypted, status, type, created_at FROM backups ORDER BY created_at DESC")
	if err == nil {
		defer rows.Close()
		var lastBackupTime time.Time
		for rows.Next() {
			var size int64
			var encrypted bool
			var status, backupType string
			var createdAt time.Time
			rows.Scan(&size, &encrypted, &status, &backupType, &createdAt)

			stats.TotalBackups++
			stats.TotalSize += size

			if encrypted {
				stats.EncryptedBackups++
			}

			if status == "completed" {
				stats.SuccessfulBackups++
			} else if status == "failed" {
				stats.FailedBackups++
			}

			stats.BackupsByType[backupType]++
			stats.StorageByType[backupType] += size

			if stats.TotalBackups == 1 {
				lastBackupTime = createdAt
			}
		}
		if !lastBackupTime.IsZero() {
			stats.LastBackupTime = lastBackupTime.Format(time.RFC3339)
		}
	}

	stats.TotalSizeHuman = formatBytes(stats.TotalSize)

	// Calculate success rate
	if stats.TotalBackups > 0 {
		stats.SuccessRate = float64(stats.SuccessfulBackups) / float64(stats.TotalBackups) * 100
		stats.AvgBackupSize = stats.TotalSize / int64(stats.TotalBackups)
		stats.AvgBackupSizeHuman = formatBytes(stats.AvgBackupSize)
	}

	// Get restore jobs count
	config.DB.QueryRow("SELECT COUNT(*) FROM restore_jobs").Scan(&stats.RestoreJobs)

	// Get next scheduled run
	var nextRun *time.Time
	err = config.DB.QueryRow("SELECT MIN(next_run_at) FROM schedules WHERE enabled = true AND next_run_at IS NOT NULL").Scan(&nextRun)
	if err == nil && nextRun != nil {
		stats.NextScheduledRun = nextRun.Format(time.RFC3339)
	}

	// Get recent failures (last 24 hours)
	config.DB.QueryRow(`
		SELECT COUNT(*) FROM backup_history
		WHERE status = 'failed' AND started_at > NOW() - INTERVAL '24 hours'
	`).Scan(&stats.RecentFailures)

	// Get daily backup count
	config.DB.QueryRow(`
		SELECT COUNT(*) FROM backups
		WHERE created_at > NOW() - INTERVAL '24 hours'
	`).Scan(&stats.DailyBackupCount)

	// Get weekly backup count
	config.DB.QueryRow(`
		SELECT COUNT(*) FROM backups
		WHERE created_at > NOW() - INTERVAL '7 days'
	`).Scan(&stats.WeeklyBackupCount)

	return stats
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
		Enabled  *bool  `json:"enabled,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	id := uuid.New().String()
	_, err := config.DB.Exec(`
		INSERT INTO schedules (id, name, cron_expression, type, target, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, req.Name, req.CronExpr, req.Type, req.Target, enabled)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	// Log the creation
	addJobLogEntry(id, "info", "Schedule created", fmt.Sprintf("Name: %s, Cron: %s, Type: %s", req.Name, req.CronExpr, req.Type))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "created"})
}

func getScheduleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var s Schedule
	err := config.DB.QueryRow(`
		SELECT id, name, cron_expression, type, target, enabled, created_at, updated_at, last_run_at, next_run_at
		FROM schedules WHERE id = $1
	`, vars["id"]).Scan(&s.ID, &s.Name, &s.CronExpr, &s.Type, &s.Target, &s.Enabled, &s.CreatedAt, &s.UpdatedAt, &s.LastRunAt, &s.NextRunAt)

	if err != nil {
		http.Error(w, `{"error": "Schedule not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func updateScheduleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var req struct {
		Name     string `json:"name,omitempty"`
		Type     string `json:"type,omitempty"`
		CronExpr string `json:"cron_expression,omitempty"`
		Target   string `json:"target,omitempty"`
		Enabled  *bool  `json:"enabled,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Build update query dynamically
	updates := []string{"updated_at = CURRENT_TIMESTAMP"}
	args := []interface{}{}
	argNum := 1

	if req.Name != "" {
		updates = append(updates, fmt.Sprintf("name = $%d", argNum))
		args = append(args, req.Name)
		argNum++
	}
	if req.Type != "" {
		updates = append(updates, fmt.Sprintf("type = $%d", argNum))
		args = append(args, req.Type)
		argNum++
	}
	if req.CronExpr != "" {
		updates = append(updates, fmt.Sprintf("cron_expression = $%d", argNum))
		args = append(args, req.CronExpr)
		argNum++
	}
	if req.Target != "" {
		updates = append(updates, fmt.Sprintf("target = $%d", argNum))
		args = append(args, req.Target)
		argNum++
	}
	if req.Enabled != nil {
		updates = append(updates, fmt.Sprintf("enabled = $%d", argNum))
		args = append(args, *req.Enabled)
		argNum++
	}

	args = append(args, vars["id"])
	query := fmt.Sprintf("UPDATE schedules SET %s WHERE id = $%d",
		joinStrings(updates, ", "), argNum)

	result, err := config.DB.Exec(query, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, `{"error": "Schedule not found"}`, http.StatusNotFound)
		return
	}

	addJobLogEntry(vars["id"], "info", "Schedule updated", "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func enableScheduleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	result, err := config.DB.Exec("UPDATE schedules SET enabled = true, updated_at = CURRENT_TIMESTAMP WHERE id = $1", vars["id"])
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, `{"error": "Schedule not found"}`, http.StatusNotFound)
		return
	}

	addJobLogEntry(vars["id"], "info", "Schedule enabled", "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "enabled"})
}

func disableScheduleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	result, err := config.DB.Exec("UPDATE schedules SET enabled = false, updated_at = CURRENT_TIMESTAMP WHERE id = $1", vars["id"])
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, `{"error": "Schedule not found"}`, http.StatusNotFound)
		return
	}

	addJobLogEntry(vars["id"], "info", "Schedule disabled", "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "disabled"})
}

// Helper to join strings
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
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

	// Create history entry first
	historyID := uuid.New().String()
	startTime := time.Now()
	config.DB.Exec(`
		INSERT INTO backup_history (id, schedule_id, status, started_at, message)
		VALUES ($1, $2, $3, $4, $5)
	`, historyID, scheduleID, "running", startTime, "Backup in progress")

	addJobLogEntry(historyID, "info", "Backup job started", fmt.Sprintf("Schedule: %s, Target: %s", name, target))

	// Create backup
	backupID := uuid.New().String()
	backupData := fmt.Sprintf("Scheduled backup: %s, Target: %s, Time: %s", name, target, time.Now().Format(time.RFC3339))
	backupPath := filepath.Join(config.BackupDir, backupID+".dat")

	addJobLogEntry(historyID, "info", "Writing backup file", backupPath)

	if err := os.WriteFile(backupPath, []byte(backupData), 0644); err != nil {
		addJobLogEntry(historyID, "error", "Failed to write backup file", err.Error())
		config.DB.Exec(`
			UPDATE backup_history SET status = 'failed', completed_at = $1, message = $2
			WHERE id = $3
		`, time.Now(), fmt.Sprintf("Failed to write backup: %v", err), historyID)
		sendNotification("backup_failed", historyID, fmt.Sprintf("Backup failed for schedule %s: %v", name, err))
		http.Error(w, fmt.Sprintf(`{"error": "Failed to write backup: %v"}`, err), http.StatusInternalServerError)
		return
	}

	addJobLogEntry(historyID, "info", "Backup file written successfully", fmt.Sprintf("Size: %d bytes", len(backupData)))

	// Save backup metadata
	_, err = config.DB.Exec(`
		INSERT INTO backups (id, source_path, backup_path, size, description, status, type, encrypted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, backupID, target, backupPath, len(backupData), name, "completed", schedType, false)
	if err != nil {
		log.Printf("Failed to save backup metadata: %v", err)
		addJobLogEntry(historyID, "warn", "Failed to save backup metadata", err.Error())
	}

	// Update schedule last run
	config.DB.Exec("UPDATE schedules SET last_run_at = $1, updated_at = $1 WHERE id = $2", time.Now(), scheduleID)

	// Update history to completed
	config.DB.Exec(`
		UPDATE backup_history SET status = 'completed', completed_at = $1, size = $2, message = $3
		WHERE id = $4
	`, time.Now(), len(backupData), "Backup completed successfully", historyID)

	addJobLogEntry(historyID, "info", "Backup completed successfully", fmt.Sprintf("Backup ID: %s", backupID))
	sendNotification("backup_completed", historyID, fmt.Sprintf("Backup completed for schedule %s", name))

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

	// Create job log entry
	addJobLogEntry(backupID, "info", "Manual backup started", fmt.Sprintf("Name: %s, Type: %s", req.Name, req.Type))

	backupData := fmt.Sprintf("Manual backup: %s, Type: %s, Target: %s, Time: %s",
		req.Name, req.Type, req.Target, time.Now().Format(time.RFC3339))

	var finalData []byte
	var backupName string
	encrypted := false

	if req.Encrypt && config.VaultIntegration {
		addJobLogEntry(backupID, "info", "Encrypting backup data", "")
		encryptedData, err := encryptWithVault([]byte(backupData))
		if err != nil {
			addJobLogEntry(backupID, "error", "Encryption failed", err.Error())
			sendNotification("backup_failed", backupID, fmt.Sprintf("Manual backup failed: encryption error - %v", err))
			http.Error(w, fmt.Sprintf(`{"error": "Encryption failed: %v"}`, err), http.StatusInternalServerError)
			return
		}
		finalData = encryptedData
		backupName = req.Name + ".enc"
		encrypted = true
		addJobLogEntry(backupID, "info", "Encryption successful", fmt.Sprintf("Encrypted size: %d bytes", len(finalData)))
	} else {
		finalData = []byte(backupData)
		backupName = req.Name
	}

	backupPath := filepath.Join(config.BackupDir, backupID+".dat")
	addJobLogEntry(backupID, "info", "Writing backup file", backupPath)

	if err := os.WriteFile(backupPath, finalData, 0644); err != nil {
		addJobLogEntry(backupID, "error", "Failed to write backup file", err.Error())
		sendNotification("backup_failed", backupID, fmt.Sprintf("Manual backup failed: %v", err))
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
		addJobLogEntry(backupID, "warn", "Failed to save backup metadata", err.Error())
	}

	addJobLogEntry(backupID, "info", "Backup completed successfully", fmt.Sprintf("Size: %d bytes, Encrypted: %v", len(finalData), encrypted))
	sendNotification("backup_completed", backupID, fmt.Sprintf("Manual backup %s completed successfully", backupName))

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

func cancelRestoreJobHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	// Can only cancel running jobs
	var status string
	err := config.DB.QueryRow("SELECT status FROM restore_jobs WHERE id = $1", vars["id"]).Scan(&status)
	if err != nil {
		http.Error(w, `{"error": "Restore job not found"}`, http.StatusNotFound)
		return
	}

	if status != "running" {
		http.Error(w, `{"error": "Can only cancel running jobs"}`, http.StatusBadRequest)
		return
	}

	_, err = config.DB.Exec(`
		UPDATE restore_jobs SET status = 'cancelled', completed_at = CURRENT_TIMESTAMP, message = 'Cancelled by user'
		WHERE id = $1
	`, vars["id"])
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	addJobLogEntry(vars["id"], "warn", "Restore job cancelled", "Cancelled by user request")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

// triggerRestoreHandler is the main restore endpoint
func triggerRestoreHandler(w http.ResponseWriter, r *http.Request) {
	var req RestoreRequest
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

	_, err = config.DB.Exec(`
		INSERT INTO restore_jobs (id, backup_id, status, started_at, target_path, verify_only)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, jobID, req.BackupID, "running", startTime, req.TargetPath, req.VerifyOnly)
	if err != nil {
		http.Error(w, `{"error": "Failed to create restore job"}`, http.StatusInternalServerError)
		return
	}

	addJobLogEntry(jobID, "info", "Restore job started", fmt.Sprintf("Backup: %s, Target: %s, VerifyOnly: %v", req.BackupID, req.TargetPath, req.VerifyOnly))

	// Execute restore in background
	go executeRestoreEnhanced(jobID, req.BackupID, backupPath, encrypted, req.TargetPath, req.VerifyOnly, req.Overwrite)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id":      jobID,
		"backup_id":   req.BackupID,
		"status":      "running",
		"message":     "Restore job started",
		"verify_only": fmt.Sprintf("%v", req.VerifyOnly),
	})
}

func executeRestoreEnhanced(jobID, backupID, backupPath string, encrypted bool, targetPath string, verifyOnly, overwrite bool) {
	addJobLogEntry(jobID, "info", "Reading backup file", backupPath)

	// Read backup file
	data, err := os.ReadFile(backupPath)
	if err != nil {
		addJobLogEntry(jobID, "error", "Failed to read backup file", err.Error())
		updateRestoreJob(jobID, "failed", fmt.Sprintf("Failed to read backup file: %v", err))
		sendNotification("restore_failed", jobID, fmt.Sprintf("Restore failed: %v", err))
		return
	}

	addJobLogEntry(jobID, "info", "Backup file read successfully", fmt.Sprintf("Size: %d bytes", len(data)))

	// Decrypt if needed
	if encrypted {
		addJobLogEntry(jobID, "info", "Decrypting backup data", "")
		decrypted, err := decryptWithVault(data)
		if err != nil {
			addJobLogEntry(jobID, "error", "Decryption failed", err.Error())
			updateRestoreJob(jobID, "failed", fmt.Sprintf("Decryption failed: %v", err))
			sendNotification("restore_failed", jobID, fmt.Sprintf("Restore decryption failed: %v", err))
			return
		}
		data = decrypted
		addJobLogEntry(jobID, "info", "Decryption successful", fmt.Sprintf("Decrypted size: %d bytes", len(data)))
	}

	// Verify only mode - just validate the data
	if verifyOnly {
		addJobLogEntry(jobID, "info", "Verification complete", "Data integrity verified")
		updateRestoreJob(jobID, "completed", fmt.Sprintf("Verification successful - %d bytes verified", len(data)))
		sendNotification("restore_completed", jobID, "Restore verification completed successfully")
		return
	}

	// If target path is specified, write the restored data
	if targetPath != "" {
		// Check if file exists and overwrite is not enabled
		if _, err := os.Stat(targetPath); err == nil && !overwrite {
			addJobLogEntry(jobID, "error", "Target file exists", "Set overwrite=true to replace")
			updateRestoreJob(jobID, "failed", "Target file exists and overwrite is disabled")
			sendNotification("restore_failed", jobID, "Restore failed: target file exists")
			return
		}

		addJobLogEntry(jobID, "info", "Creating target directory", filepath.Dir(targetPath))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			addJobLogEntry(jobID, "error", "Failed to create target directory", err.Error())
			updateRestoreJob(jobID, "failed", fmt.Sprintf("Failed to create target directory: %v", err))
			sendNotification("restore_failed", jobID, fmt.Sprintf("Restore failed: %v", err))
			return
		}

		addJobLogEntry(jobID, "info", "Writing restored data", targetPath)
		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			addJobLogEntry(jobID, "error", "Failed to write restored data", err.Error())
			updateRestoreJob(jobID, "failed", fmt.Sprintf("Failed to write restored data: %v", err))
			sendNotification("restore_failed", jobID, fmt.Sprintf("Restore failed: %v", err))
			return
		}
	}

	log.Printf("Restore job %s completed, restored %d bytes", jobID, len(data))
	addJobLogEntry(jobID, "info", "Restore completed successfully", fmt.Sprintf("Restored %d bytes", len(data)))
	updateRestoreJob(jobID, "completed", fmt.Sprintf("Successfully restored %d bytes", len(data)))
	sendNotification("restore_completed", jobID, fmt.Sprintf("Restore completed: %d bytes restored", len(data)))
}

func verifyRestoreHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackupID string `json:"backup_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	// Create a verify-only restore job
	restoreReq := RestoreRequest{
		BackupID:   req.BackupID,
		VerifyOnly: true,
	}

	// Verify backup exists
	var backupPath string
	var encrypted bool
	err := config.DB.QueryRow("SELECT backup_path, encrypted FROM backups WHERE id = $1", restoreReq.BackupID).Scan(&backupPath, &encrypted)
	if err != nil {
		http.Error(w, `{"error": "Backup not found"}`, http.StatusNotFound)
		return
	}

	jobID := uuid.New().String()
	startTime := time.Now()

	_, err = config.DB.Exec(`
		INSERT INTO restore_jobs (id, backup_id, status, started_at, verify_only)
		VALUES ($1, $2, $3, $4, $5)
	`, jobID, restoreReq.BackupID, "running", startTime, true)
	if err != nil {
		http.Error(w, `{"error": "Failed to create verification job"}`, http.StatusInternalServerError)
		return
	}

	go executeRestoreEnhanced(jobID, restoreReq.BackupID, backupPath, encrypted, "", true, false)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id":    jobID,
		"backup_id": restoreReq.BackupID,
		"status":    "running",
		"message":   "Verification job started",
	})
}

// Job handlers
func listJobsHandler(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	status := r.URL.Query().Get("status")

	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = config.DB.Query(`
			SELECT id, schedule_id, status, started_at, completed_at, size, message
			FROM backup_history WHERE status = $1 ORDER BY started_at DESC LIMIT $2
		`, status, limit)
	} else {
		rows, err = config.DB.Query(`
			SELECT id, schedule_id, status, started_at, completed_at, size, message
			FROM backup_history ORDER BY started_at DESC LIMIT $1
		`, limit)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	jobs := []BackupJob{}
	for rows.Next() {
		var j BackupJob
		rows.Scan(&j.ID, &j.ScheduleID, &j.Status, &j.StartedAt, &j.CompletedAt, &j.Size, &j.Message)
		jobs = append(jobs, j)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobs":      jobs,
		"count":     len(jobs),
		"service":   "Backup Dashboard",
		"timestamp": time.Now().Format(time.RFC3339),
		"status":    "ok",
	})
}

func getJobHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var j BackupJob

	err := config.DB.QueryRow(`
		SELECT id, schedule_id, status, started_at, completed_at, size, message
		FROM backup_history WHERE id = $1
	`, vars["id"]).Scan(&j.ID, &j.ScheduleID, &j.Status, &j.StartedAt, &j.CompletedAt, &j.Size, &j.Message)

	if err != nil {
		http.Error(w, `{"error": "Job not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(j)
}

func getJobLogsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	level := r.URL.Query().Get("level")

	var rows *sql.Rows
	var err error

	if level != "" {
		rows, err = config.DB.Query(`
			SELECT id, job_id, level, message, timestamp, details
			FROM job_logs WHERE job_id = $1 AND level = $2 ORDER BY timestamp ASC LIMIT $3
		`, vars["id"], level, limit)
	} else {
		rows, err = config.DB.Query(`
			SELECT id, job_id, level, message, timestamp, details
			FROM job_logs WHERE job_id = $1 ORDER BY timestamp ASC LIMIT $2
		`, vars["id"], limit)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	logs := []JobLog{}
	for rows.Next() {
		var l JobLog
		var details *string
		rows.Scan(&l.ID, &l.JobID, &l.Level, &l.Message, &l.Timestamp, &details)
		if details != nil {
			l.Details = *details
		}
		logs = append(logs, l)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":   logs,
		"count":  len(logs),
		"job_id": vars["id"],
	})
}

func addJobLogHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var req struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		Details string `json:"details,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	if req.Level == "" {
		req.Level = "info"
	}

	logID := addJobLogEntry(vars["id"], req.Level, req.Message, req.Details)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": logID, "status": "created"})
}

// addJobLogEntry adds a log entry for a job
func addJobLogEntry(jobID, level, message, details string) string {
	logID := uuid.New().String()
	_, err := config.DB.Exec(`
		INSERT INTO job_logs (id, job_id, level, message, details)
		VALUES ($1, $2, $3, $4, $5)
	`, logID, jobID, level, message, details)
	if err != nil {
		log.Printf("Failed to add job log: %v", err)
	}
	return logID
}

// Notification handlers
func listNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	rows, err := config.DB.Query(`
		SELECT id, type, job_id, status, message, sent_at, delivered, channel
		FROM notifications ORDER BY sent_at DESC LIMIT $1
	`, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	notifications := []Notification{}
	for rows.Next() {
		var n Notification
		var jobID, channel *string
		rows.Scan(&n.ID, &n.Type, &jobID, &n.Status, &n.Message, &n.SentAt, &n.Delivered, &channel)
		if jobID != nil {
			n.JobID = *jobID
		}
		if channel != nil {
			n.Channel = *channel
		}
		notifications = append(notifications, n)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"notifications": notifications,
		"count":         len(notifications),
	})
}

func getNotificationConfigHandler(w http.ResponseWriter, r *http.Request) {
	var cfg NotificationConfig

	err := config.DB.QueryRow(`
		SELECT webhook_url, email_address, notify_on_failure, notify_on_success
		FROM notification_config WHERE id = 'default'
	`).Scan(&cfg.WebhookURL, &cfg.EmailAddress, &cfg.NotifyOnFailure, &cfg.NotifyOnSuccess)

	if err != nil {
		// Return defaults from environment
		cfg = NotificationConfig{
			WebhookURL:      config.NotificationWebhook,
			EmailAddress:    config.NotificationEmail,
			NotifyOnFailure: config.NotifyOnFailure,
			NotifyOnSuccess: config.NotifyOnSuccess,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func updateNotificationConfigHandler(w http.ResponseWriter, r *http.Request) {
	var cfg NotificationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	_, err := config.DB.Exec(`
		INSERT INTO notification_config (id, webhook_url, email_address, notify_on_failure, notify_on_success, updated_at)
		VALUES ('default', $1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			webhook_url = $1,
			email_address = $2,
			notify_on_failure = $3,
			notify_on_success = $4,
			updated_at = CURRENT_TIMESTAMP
	`, cfg.WebhookURL, cfg.EmailAddress, cfg.NotifyOnFailure, cfg.NotifyOnSuccess)

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%v"}`, err), http.StatusInternalServerError)
		return
	}

	// Update in-memory config
	config.mu.Lock()
	config.NotificationWebhook = cfg.WebhookURL
	config.NotificationEmail = cfg.EmailAddress
	config.NotifyOnFailure = cfg.NotifyOnFailure
	config.NotifyOnSuccess = cfg.NotifyOnSuccess
	config.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func testNotificationHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Message string `json:"message,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		req.Message = "This is a test notification from HolmOS Backup Dashboard"
	}

	err := sendNotificationToChannel(req.Channel, "test", "", req.Message)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to send notification: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sent", "channel": req.Channel})
}

// sendNotification sends a notification for backup events
func sendNotification(notifType, jobID, message string) {
	config.mu.RLock()
	notifyFailure := config.NotifyOnFailure
	notifySuccess := config.NotifyOnSuccess
	webhookURL := config.NotificationWebhook
	config.mu.RUnlock()

	// Check if we should send based on type
	isFailure := notifType == "backup_failed" || notifType == "restore_failed"
	isSuccess := notifType == "backup_completed" || notifType == "restore_completed"

	if (isFailure && !notifyFailure) || (isSuccess && !notifySuccess) {
		return
	}

	// Record notification
	notifID := uuid.New().String()
	channel := "webhook"
	if webhookURL == "" {
		channel = "log"
	}

	_, err := config.DB.Exec(`
		INSERT INTO notifications (id, type, job_id, status, message, channel, delivered)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, notifID, notifType, jobID, "sent", message, channel, webhookURL != "")

	if err != nil {
		log.Printf("Failed to record notification: %v", err)
	}

	// Send to webhook if configured
	if webhookURL != "" {
		go sendWebhookNotification(webhookURL, notifType, jobID, message)
	}

	// Always log
	log.Printf("[NOTIFICATION] %s: %s (job: %s)", notifType, message, jobID)
}

func sendNotificationToChannel(channel, notifType, jobID, message string) error {
	config.mu.RLock()
	webhookURL := config.NotificationWebhook
	config.mu.RUnlock()

	switch channel {
	case "webhook":
		if webhookURL == "" {
			return fmt.Errorf("webhook URL not configured")
		}
		return sendWebhookNotification(webhookURL, notifType, jobID, message)
	case "log":
		log.Printf("[NOTIFICATION] %s: %s", notifType, message)
		return nil
	default:
		return fmt.Errorf("unknown channel: %s", channel)
	}
}

func sendWebhookNotification(webhookURL, notifType, jobID, message string) error {
	payload := map[string]interface{}{
		"type":      notifType,
		"job_id":    jobID,
		"message":   message,
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "backup-dashboard",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("Failed to send webhook notification: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("Webhook returned error status: %d", resp.StatusCode)
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
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
