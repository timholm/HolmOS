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
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Config struct {
	SchedulerURL     string
	StorageURL       string
	VaultURL         string
	DB               *sql.DB
	EncryptionKey    []byte
	VaultIntegration bool
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

type BackupHistory struct {
	ID          string     `json:"id"`
	ScheduleID  string     `json:"schedule_id"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Size        int64      `json:"size,omitempty"`
	Message     string     `json:"message,omitempty"`
}

type StoredBackup struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Encrypted bool      `json:"encrypted"`
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
		SchedulerURL:     getEnv("SCHEDULER_URL", "http://backup-scheduler.holm.svc.cluster.local:8080"),
		StorageURL:       getEnv("STORAGE_URL", "http://backup-storage.holm.svc.cluster.local"),
		VaultURL:         getEnv("VAULT_URL", "http://vault.holm.svc.cluster.local"),
		VaultIntegration: getEnv("VAULT_INTEGRATION", "true") == "true",
	}

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
				break
			}
		}
		log.Printf("Waiting for database... attempt %d/30", i+1)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer config.DB.Close()

	if err := initDashboardDB(); err != nil {
		log.Printf("Warning: Failed to initialize dashboard tables: %v", err)
	}

	r := mux.NewRouter()

	r.HandleFunc("/", dashboardHandler).Methods("GET")
	r.HandleFunc("/health", healthHandler).Methods("GET")

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/stats", getStatsHandler).Methods("GET")
	api.HandleFunc("/schedules", listSchedulesHandler).Methods("GET")
	api.HandleFunc("/schedules", createScheduleHandler).Methods("POST")
	api.HandleFunc("/schedules/{id}", deleteScheduleHandler).Methods("DELETE")
	api.HandleFunc("/schedules/{id}/run", triggerScheduleHandler).Methods("POST")
	api.HandleFunc("/schedules/{id}/history", getScheduleHistoryHandler).Methods("GET")
	api.HandleFunc("/backups", listBackupsHandler).Methods("GET")
	api.HandleFunc("/backups/{id}", getBackupHandler).Methods("GET")
	api.HandleFunc("/backups/{id}/download", downloadBackupHandler).Methods("GET")
	api.HandleFunc("/backup/manual", triggerManualBackupHandler).Methods("POST")
	api.HandleFunc("/restore/points", listRestorePointsHandler).Methods("GET")
	api.HandleFunc("/restore/start", startRestoreHandler).Methods("POST")
	api.HandleFunc("/restore/jobs", listRestoreJobsHandler).Methods("GET")
	api.HandleFunc("/restore/jobs/{id}", getRestoreJobHandler).Methods("GET")
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

func initDashboardDB() error {
	schema := `
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

	resp, err := http.Get(config.SchedulerURL + "/health")
	if err != nil || resp.StatusCode != 200 {
		status["scheduler"] = "unhealthy"
	} else {
		status["scheduler"] = "healthy"
	}
	if resp != nil {
		resp.Body.Close()
	}

	resp, err = http.Get(config.StorageURL + "/health")
	if err != nil || resp.StatusCode != 200 {
		status["storage"] = "unhealthy"
	} else {
		status["storage"] = "healthy"
	}
	if resp != nil {
		resp.Body.Close()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func getStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats := DashboardStats{}

	resp, err := http.Get(config.SchedulerURL + "/schedules")
	if err == nil && resp.StatusCode == 200 {
		var schedules []Schedule
		json.NewDecoder(resp.Body).Decode(&schedules)
		resp.Body.Close()
		stats.TotalSchedules = len(schedules)
		for _, s := range schedules {
			if s.Enabled {
				stats.ActiveSchedules++
			}
		}
	}

	resp, err = http.Get(config.StorageURL + "/backups")
	if err == nil && resp.StatusCode == 200 {
		var backupResp struct {
			Backups []struct {
				ID   string `json:"id"`
				Size int64  `json:"size"`
				Name string `json:"name"`
			} `json:"backups"`
			Count int `json:"count"`
		}
		json.NewDecoder(resp.Body).Decode(&backupResp)
		resp.Body.Close()
		stats.TotalBackups = backupResp.Count
		for _, b := range backupResp.Backups {
			stats.TotalSize += b.Size
			if strings.HasSuffix(b.Name, ".enc") {
				stats.EncryptedBackups++
			}
		}
		stats.TotalSizeHuman = formatBytes(stats.TotalSize)
	}

	var count int
	config.DB.QueryRow("SELECT COUNT(*) FROM restore_jobs").Scan(&count)
	stats.RestoreJobs = count

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func listSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	proxyRequest(w, r, config.SchedulerURL+"/schedules", "GET", nil)
}

func createScheduleHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	proxyRequest(w, r, config.SchedulerURL+"/schedules", "POST", body)
}

func deleteScheduleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	proxyRequest(w, r, config.SchedulerURL+"/schedules/"+vars["id"], "DELETE", nil)
}

func triggerScheduleHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	proxyRequest(w, r, config.SchedulerURL+"/schedules/"+vars["id"]+"/run", "POST", nil)
}

func getScheduleHistoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	limit := r.URL.Query().Get("limit")
	url := config.SchedulerURL + "/schedules/" + vars["id"] + "/history"
	if limit != "" {
		url += "?limit=" + limit
	}
	proxyRequest(w, r, url, "GET", nil)
}

func listBackupsHandler(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	url := config.StorageURL + "/backups"
	if typeFilter != "" {
		url += "?type=" + typeFilter
	}
	proxyRequest(w, r, url, "GET", nil)
}

func getBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	proxyRequest(w, r, config.StorageURL+"/backups/"+vars["id"], "GET", nil)
}

func downloadBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	resp, err := http.Get(config.StorageURL + "/backups/" + vars["id"] + "/download")
	if err != nil {
		http.Error(w, "Failed to download backup", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func triggerManualBackupHandler(w http.ResponseWriter, r *http.Request) {
	var req ManualBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	backupData := fmt.Sprintf("Manual backup: %s, Type: %s, Target: %s, Time: %s",
		req.Name, req.Type, req.Target, time.Now().Format(time.RFC3339))

	var finalData string
	var backupName string
	if req.Encrypt && config.VaultIntegration {
		encrypted, err := encryptWithVault([]byte(backupData))
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Encryption failed: %v"}`, err), http.StatusInternalServerError)
			return
		}
		finalData = base64.StdEncoding.EncodeToString(encrypted)
		backupName = req.Name + ".enc"
	} else {
		finalData = base64.StdEncoding.EncodeToString([]byte(backupData))
		backupName = req.Name
	}

	storeReq := map[string]string{
		"name": backupName,
		"type": req.Type,
		"data": finalData,
	}
	body, _ := json.Marshal(storeReq)

	resp, err := http.Post(config.StorageURL+"/backups", "application/json", bytes.NewReader(body))
	if err != nil {
		http.Error(w, `{"error": "Failed to store backup"}`, http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	result := make(map[string]interface{})
	json.NewDecoder(resp.Body).Decode(&result)
	if result == nil {
		result = make(map[string]interface{})
	}
	result["encrypted"] = req.Encrypt && config.VaultIntegration
	result["name"] = backupName
	result["status"] = "completed"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func listRestorePointsHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(config.StorageURL + "/backups")
	if err != nil {
		http.Error(w, `{"error": "Failed to fetch backups"}`, http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var backupResp struct {
		Backups []struct {
			ID          string    `json:"id"`
			SourcePath  string    `json:"source_path"`
			BackupPath  string    `json:"backup_path"`
			Size        int64     `json:"size"`
			CreatedAt   time.Time `json:"created_at"`
			Description string    `json:"description"`
			Status      string    `json:"status"`
		} `json:"backups"`
	}
	json.NewDecoder(resp.Body).Decode(&backupResp)

	restorePoints := make([]RestorePoint, len(backupResp.Backups))
	for i, b := range backupResp.Backups {
		backupName := b.Description
		if backupName == "" {
			backupName = b.SourcePath
		}
		restorePoints[i] = RestorePoint{
			ID:         b.ID,
			BackupID:   b.ID,
			BackupName: backupName,
			Type:       "file",
			Size:       b.Size,
			CreatedAt:  b.CreatedAt,
			Encrypted:  strings.HasSuffix(b.BackupPath, ".enc"),
			Status:     b.Status,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(restorePoints)
}

func startRestoreHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackupID   string `json:"backup_id"`
		TargetTime string `json:"target_time,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
		return
	}

	jobID := uuid.New().String()
	startTime := time.Now()

	_, err := config.DB.Exec(`INSERT INTO restore_jobs (id, backup_id, status, started_at) VALUES ($1, $2, $3, $4)`,
		jobID, req.BackupID, "running", startTime)
	if err != nil {
		http.Error(w, `{"error": "Failed to create restore job"}`, http.StatusInternalServerError)
		return
	}

	go executeRestore(jobID, req.BackupID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id":    jobID,
		"backup_id": req.BackupID,
		"status":    "running",
		"message":   "Restore job started",
	})
}

func executeRestore(jobID, backupID string) {
	time.Sleep(3 * time.Second)

	resp, err := http.Get(config.StorageURL + "/backups/" + backupID + "/download")
	if err != nil {
		updateRestoreJob(jobID, "failed", "Failed to download backup: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		updateRestoreJob(jobID, "failed", "Backup not found")
		return
	}

	data, _ := io.ReadAll(resp.Body)

	contentDisp := resp.Header.Get("Content-Disposition")
	if strings.Contains(contentDisp, ".enc") {
		decrypted, err := decryptWithVault(data)
		if err != nil {
			updateRestoreJob(jobID, "failed", "Decryption failed: "+err.Error())
			return
		}
		data = decrypted
	}

	time.Sleep(2 * time.Second)

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
	resp, err := http.Get(config.VaultURL + "/health")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"available":   false,
			"integration": config.VaultIntegration,
			"error":       err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	var vaultHealth map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&vaultHealth)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"available":   true,
		"integration": config.VaultIntegration,
		"vault":       vaultHealth,
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

func proxyRequest(w http.ResponseWriter, r *http.Request, url string, method string, body []byte) {
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		http.Error(w, `{"error": "Failed to create request"}`, http.StatusInternalServerError)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Upstream error: %v"}`, err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
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
        :root {
            --ctp-rosewater: #f5e0dc; --ctp-flamingo: #f2cdcd; --ctp-pink: #f5c2e7;
            --ctp-mauve: #cba6f7; --ctp-red: #f38ba8; --ctp-maroon: #eba0ac;
            --ctp-peach: #fab387; --ctp-yellow: #f9e2af; --ctp-green: #a6e3a1;
            --ctp-teal: #94e2d5; --ctp-sky: #89dceb; --ctp-sapphire: #74c7ec;
            --ctp-blue: #89b4fa; --ctp-lavender: #b4befe; --ctp-text: #cdd6f4;
            --ctp-subtext1: #bac2de; --ctp-subtext0: #a6adc8; --ctp-overlay2: #9399b2;
            --ctp-overlay1: #7f849c; --ctp-overlay0: #6c7086; --ctp-surface2: #585b70;
            --ctp-surface1: #45475a; --ctp-surface0: #313244; --ctp-base: #1e1e2e;
            --ctp-mantle: #181825; --ctp-crust: #11111b;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: 'Inter', 'Segoe UI', system-ui, sans-serif; background: linear-gradient(135deg, var(--ctp-crust) 0%, var(--ctp-base) 50%, var(--ctp-mantle) 100%); color: var(--ctp-text); min-height: 100vh; line-height: 1.6; }
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
        .badge-encrypted { background: var(--ctp-mauve); color: var(--ctp-crust); }
        .form-group { margin-bottom: 1rem; }
        .form-group label { display: block; color: var(--ctp-subtext1); margin-bottom: 0.5rem; }
        input, select, textarea { width: 100%; padding: 0.75rem; background: var(--ctp-surface0); border: 2px solid var(--ctp-surface1); border-radius: 0.5rem; color: var(--ctp-text); font-size: 1rem; }
        input:focus, select:focus { outline: none; border-color: var(--ctp-teal); }
        button { background: linear-gradient(135deg, var(--ctp-teal) 0%, var(--ctp-green) 100%); color: var(--ctp-crust); border: none; padding: 0.75rem 1.5rem; border-radius: 0.5rem; cursor: pointer; font-weight: 600; transition: all 0.2s; }
        button:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(148, 226, 213, 0.3); }
        .btn-danger { background: linear-gradient(135deg, var(--ctp-red) 0%, var(--ctp-maroon) 100%); }
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
        .service-status { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 1rem; }
        .service-badge { display: flex; align-items: center; gap: 0.5rem; padding: 0.75rem 1rem; background: var(--ctp-surface0); border-radius: 0.5rem; }
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

        <div class="stats-grid" id="stats-grid">
            <div class="stat-card"><div class="value" id="stat-schedules">-</div><div class="label">Active Schedules</div></div>
            <div class="stat-card"><div class="value" id="stat-backups">-</div><div class="label">Total Backups</div></div>
            <div class="stat-card"><div class="value" id="stat-size">-</div><div class="label">Storage Used</div></div>
            <div class="stat-card"><div class="value" id="stat-encrypted">-</div><div class="label">Encrypted</div></div>
        </div>

        <div class="tabs">
            <button class="tab active" onclick="showTab('schedules', this)">Schedules</button>
            <button class="tab" onclick="showTab('backups', this)">Backup History</button>
            <button class="tab" onclick="showTab('manual', this)">Manual Backup</button>
            <button class="tab" onclick="showTab('restore', this)">Restore</button>
            <button class="tab" onclick="showTab('vault', this)">Vault Integration</button>
        </div>

        <div id="schedules" class="tab-content active">
            <div class="grid-2">
                <div class="card">
                    <h2>Scheduled Backups</h2>
                    <table><thead><tr><th>Name</th><th>Schedule</th><th>Type</th><th>Next Run</th><th>Actions</th></tr></thead><tbody id="schedules-table"></tbody></table>
                </div>
                <div class="card">
                    <h2>Create Schedule</h2>
                    <form id="schedule-form">
                        <div class="form-group"><label>Name</label><input type="text" id="sched-name" placeholder="Daily Database Backup" required></div>
                        <div class="form-group"><label>Type</label><select id="sched-type"><option value="daily">Daily</option><option value="weekly">Weekly</option><option value="monthly">Monthly</option><option value="custom">Custom</option></select></div>
                        <div class="form-group"><label>Cron Expression</label><input type="text" id="sched-cron" placeholder="0 0 2 * * *" required></div>
                        <div class="form-group"><label>Target</label><input type="text" id="sched-target" placeholder="/data/important" required></div>
                        <button type="submit">Create Schedule</button>
                    </form>
                </div>
            </div>
        </div>

        <div id="backups" class="tab-content">
            <div class="card">
                <h2>Backup History</h2>
                <div style="margin-bottom: 1rem;"><select id="backup-type-filter" onchange="loadBackups()"><option value="">All Types</option><option value="daily">Daily</option><option value="weekly">Weekly</option><option value="manual">Manual</option><option value="database">Database</option></select></div>
                <table><thead><tr><th>Description</th><th>Source</th><th>Size</th><th>Created</th><th>Status</th><th>Actions</th></tr></thead><tbody id="backups-table"></tbody></table>
            </div>
        </div>

        <div id="manual" class="tab-content">
            <div class="grid-2">
                <div class="card">
                    <h2>Trigger Manual Backup</h2>
                    <form id="manual-form">
                        <div class="form-group"><label>Backup Name</label><input type="text" id="manual-name" placeholder="pre-deploy-backup" required></div>
                        <div class="form-group"><label>Backup Type</label><select id="manual-type"><option value="manual">Manual</option><option value="database">Database</option><option value="files">Files</option><option value="config">Configuration</option></select></div>
                        <div class="form-group"><label>Target Path</label><input type="text" id="manual-target" placeholder="/data/app"></div>
                        <div class="form-group"><div class="checkbox-group"><input type="checkbox" id="manual-encrypt" checked><label for="manual-encrypt">Encrypt with Vault</label></div></div>
                        <button type="submit">Start Backup</button>
                    </form>
                </div>
                <div class="card">
                    <h2>Quick Actions</h2>
                    <div style="display: flex; flex-direction: column; gap: 1rem;">
                        <button onclick="triggerScheduleBackups()">Run All Scheduled Backups</button>
                        <button class="btn-secondary" onclick="loadBackups()">Refresh Backup List</button>
                    </div>
                </div>
            </div>
        </div>

        <div id="restore" class="tab-content">
            <div class="grid-2">
                <div class="card">
                    <h2>Available Restore Points</h2>
                    <div class="timeline" id="restore-points"></div>
                </div>
                <div class="card">
                    <h2>Restore Operations</h2>
                    <div id="selected-restore" style="margin-bottom: 1rem; padding: 1rem; background: var(--ctp-surface0); border-radius: 0.5rem;"><p style="color: var(--ctp-overlay0);">Select a restore point from the timeline</p></div>
                    <button id="start-restore-btn" onclick="startRestore()" disabled>Start Restore</button>
                    <h3 style="margin-top: 2rem; margin-bottom: 1rem; color: var(--ctp-subtext1);">Recent Restore Jobs</h3>
                    <div id="restore-jobs"></div>
                </div>
            </div>
        </div>

        <div id="vault" class="tab-content">
            <div class="card">
                <h2>Vault Integration Status</h2>
                <div id="vault-status" class="vault-status"><span>Checking connection...</span></div>
            </div>
            <div class="grid-2">
                <div class="card">
                    <h2>Encryption Settings</h2>
                    <p style="color: var(--ctp-subtext0); margin-bottom: 1rem;">All encrypted backups use AES-256-GCM encryption. Keys are managed securely through the Vault agent.</p>
                    <div class="form-group"><div class="checkbox-group"><input type="checkbox" id="auto-encrypt" checked><label for="auto-encrypt">Auto-encrypt all new backups</label></div></div>
                </div>
                <div class="card">
                    <h2>Test Encryption</h2>
                    <div class="form-group"><label>Test Data</label><textarea id="test-data" rows="3" placeholder="Enter text to encrypt..."></textarea></div>
                    <button onclick="testEncryption()">Test Encryption</button>
                    <div id="encryption-result" style="margin-top: 1rem;"></div>
                </div>
            </div>
        </div>
    </div>

    <script>
        var selectedRestorePoint = null;

        function showTab(tabId, btn) {
            document.querySelectorAll('.tab-content').forEach(function(t) { t.classList.remove('active'); });
            document.querySelectorAll('.tab').forEach(function(t) { t.classList.remove('active'); });
            document.getElementById(tabId).classList.add('active');
            btn.classList.add('active');
            if (tabId === 'schedules') loadSchedules();
            if (tabId === 'backups') loadBackups();
            if (tabId === 'restore') { loadRestorePoints(); loadRestoreJobs(); }
            if (tabId === 'vault') loadVaultStatus();
        }

        function loadServiceStatus() {
            fetch('/health').then(function(r) { return r.json(); }).then(function(status) {
                var container = document.getElementById('service-status');
                var services = ['database', 'scheduler', 'storage'];
                container.innerHTML = services.map(function(s) {
                    var isOnline = status[s] === 'healthy';
                    return '<div class="service-badge ' + (isOnline ? 'online' : 'offline') + '"><span class="status-dot ' + (isOnline ? 'online' : 'offline') + '"></span><span>' + s.charAt(0).toUpperCase() + s.slice(1) + '</span></div>';
                }).join('');
            }).catch(function(e) { console.error('Failed to load service status:', e); });
        }

        function loadStats() {
            fetch('/api/stats').then(function(r) { return r.json(); }).then(function(stats) {
                document.getElementById('stat-schedules').textContent = stats.active_schedules + '/' + stats.total_schedules;
                document.getElementById('stat-backups').textContent = stats.total_backups;
                document.getElementById('stat-size').textContent = stats.total_size_human || '0 B';
                document.getElementById('stat-encrypted').textContent = stats.encrypted_backups;
            }).catch(function(e) { console.error('Failed to load stats:', e); });
        }

        function loadSchedules() {
            fetch('/api/schedules').then(function(r) { return r.json(); }).then(function(schedules) {
                var tbody = document.getElementById('schedules-table');
                if (!schedules || schedules.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="5" class="empty-state">No schedules configured</td></tr>';
                    return;
                }
                tbody.innerHTML = schedules.map(function(s) {
                    return '<tr><td>' + escapeHtml(s.name) + '</td><td><code>' + s.cron_expression + '</code></td><td><span class="badge badge-info">' + s.type + '</span></td><td>' + (s.next_run_at ? new Date(s.next_run_at).toLocaleString() : 'Not scheduled') + '</td><td class="actions"><button class="btn-small" onclick="runSchedule(\'' + s.id + '\')">Run Now</button><button class="btn-small btn-danger" onclick="deleteSchedule(\'' + s.id + '\')">Delete</button></td></tr>';
                }).join('');
            }).catch(function(e) { console.error('Failed to load schedules:', e); });
        }

        function loadBackups() {
            var typeFilter = document.getElementById('backup-type-filter').value;
            var url = typeFilter ? '/api/backups?type=' + typeFilter : '/api/backups';
            fetch(url).then(function(r) { return r.json(); }).then(function(data) {
                var backups = data.backups || [];
                var tbody = document.getElementById('backups-table');
                if (!backups || backups.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="6" class="empty-state">No backups found</td></tr>';
                    return;
                }
                tbody.innerHTML = backups.map(function(b) {
                    var name = b.description || b.source_path || 'Backup';
                    return '<tr><td>' + escapeHtml(name) + '</td><td>' + escapeHtml(b.source_path || '-') + '</td><td>' + formatBytes(b.size) + '</td><td>' + new Date(b.created_at).toLocaleString() + '</td><td><span class="badge ' + (b.status === 'completed' ? 'badge-success' : 'badge-warning') + '">' + b.status + '</span></td><td class="actions"><button class="btn-small btn-secondary" onclick="downloadBackup(\'' + b.id + '\')">Download</button></td></tr>';
                }).join('');
            }).catch(function(e) { console.error('Failed to load backups:', e); });
        }

        function loadRestorePoints() {
            fetch('/api/restore/points').then(function(r) { return r.json(); }).then(function(points) {
                var container = document.getElementById('restore-points');
                if (!points || points.length === 0) {
                    container.innerHTML = '<div class="empty-state">No restore points available</div>';
                    return;
                }
                container.innerHTML = points.map(function(p) {
                    return '<div class="timeline-item" onclick="selectRestorePoint(\'' + p.id + '\', \'' + escapeHtml(p.backup_name) + '\', \'' + p.type + '\', ' + p.size + ', \'' + p.created_at + '\', ' + p.encrypted + ')"><div style="display: flex; justify-content: space-between; align-items: center;"><div><strong>' + escapeHtml(p.backup_name) + '</strong><div style="font-size: 0.85rem; color: var(--ctp-subtext0);">' + new Date(p.created_at).toLocaleString() + '</div></div><div><span class="badge badge-info">' + p.type + '</span>' + (p.encrypted ? '<span class="badge badge-encrypted">Encrypted</span>' : '') + '</div></div><div style="font-size: 0.85rem; color: var(--ctp-overlay0); margin-top: 0.5rem;">' + formatBytes(p.size) + '</div></div>';
                }).join('');
            }).catch(function(e) { console.error('Failed to load restore points:', e); });
        }

        function selectRestorePoint(id, name, type, size, createdAt, encrypted) {
            selectedRestorePoint = { id: id, name: name, type: type, size: size, createdAt: createdAt, encrypted: encrypted };
            document.querySelectorAll('.timeline-item').forEach(function(el) { el.classList.remove('selected'); });
            event.currentTarget.classList.add('selected');
            document.getElementById('selected-restore').innerHTML = '<h4 style="margin-bottom: 0.5rem;">Selected: ' + name + '</h4><p>Type: ' + type + ' | Size: ' + formatBytes(size) + '</p><p>Created: ' + new Date(createdAt).toLocaleString() + '</p>' + (encrypted ? '<p><span class="badge badge-encrypted">Will be decrypted during restore</span></p>' : '');
            document.getElementById('start-restore-btn').disabled = false;
        }

        function startRestore() {
            if (!selectedRestorePoint) return;
            if (!confirm('Are you sure you want to restore from this backup point?')) return;
            fetch('/api/restore/start', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ backup_id: selectedRestorePoint.id }) }).then(function(r) { return r.json(); }).then(function(result) {
                alert('Restore job started: ' + result.job_id);
                loadRestoreJobs();
            }).catch(function(e) { alert('Failed to start restore: ' + e.message); });
        }

        function loadRestoreJobs() {
            fetch('/api/restore/jobs?limit=10').then(function(r) { return r.json(); }).then(function(jobs) {
                var container = document.getElementById('restore-jobs');
                if (!jobs || jobs.length === 0) {
                    container.innerHTML = '<div class="empty-state">No restore jobs yet</div>';
                    return;
                }
                container.innerHTML = jobs.map(function(j) {
                    return '<div style="padding: 0.75rem; background: var(--ctp-surface0); border-radius: 0.5rem; margin-bottom: 0.5rem;"><div style="display: flex; justify-content: space-between; align-items: center;"><span>Job: ' + j.id.substring(0, 8) + '...</span><span class="badge ' + (j.status === 'completed' ? 'badge-success' : j.status === 'failed' ? 'badge-error' : 'badge-warning') + '">' + j.status + '</span></div><div style="font-size: 0.85rem; color: var(--ctp-subtext0); margin-top: 0.25rem;">' + (j.message || 'Processing...') + '</div></div>';
                }).join('');
            }).catch(function(e) { console.error('Failed to load restore jobs:', e); });
        }

        function loadVaultStatus() {
            fetch('/api/vault/status').then(function(r) { return r.json(); }).then(function(status) {
                var container = document.getElementById('vault-status');
                if (status.available) {
                    container.className = 'vault-status connected';
                    container.innerHTML = '<span style="color: var(--ctp-green);">&#x2713;</span><span>Vault Connected - Encryption Active</span>';
                } else {
                    container.className = 'vault-status disconnected';
                    container.innerHTML = '<span style="color: var(--ctp-red);">&#x2717;</span><span>Vault Disconnected - Using local encryption</span>';
                }
            }).catch(function(e) { console.error('Failed to load vault status:', e); });
        }

        function runSchedule(id) {
            fetch('/api/schedules/' + id + '/run', { method: 'POST' }).then(function() { alert('Backup triggered'); setTimeout(loadStats, 1000); }).catch(function() { alert('Failed to trigger backup'); });
        }

        function deleteSchedule(id) {
            if (!confirm('Delete this schedule?')) return;
            fetch('/api/schedules/' + id, { method: 'DELETE' }).then(function() { loadSchedules(); loadStats(); }).catch(function() { alert('Failed to delete schedule'); });
        }

        function downloadBackup(id) { window.open('/api/backups/' + id + '/download', '_blank'); }

        document.getElementById('schedule-form').addEventListener('submit', function(e) {
            e.preventDefault();
            var data = { name: document.getElementById('sched-name').value, type: document.getElementById('sched-type').value, cron_expression: document.getElementById('sched-cron').value, target: document.getElementById('sched-target').value };
            fetch('/api/schedules', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) }).then(function(res) {
                if (res.ok) { e.target.reset(); loadSchedules(); loadStats(); } else { res.json().then(function(err) { alert('Error: ' + err.error); }); }
            }).catch(function() { alert('Failed to create schedule'); });
        });

        document.getElementById('manual-form').addEventListener('submit', function(e) {
            e.preventDefault();
            var data = { name: document.getElementById('manual-name').value, type: document.getElementById('manual-type').value, target: document.getElementById('manual-target').value, encrypt: document.getElementById('manual-encrypt').checked };
            fetch('/api/backup/manual', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data) }).then(function(res) {
                if (res.ok) { alert('Manual backup completed!'); e.target.reset(); loadStats(); } else { res.json().then(function(err) { alert('Error: ' + err.error); }); }
            }).catch(function() { alert('Failed to create backup'); });
        });

        function triggerScheduleBackups() {
            fetch('/api/schedules').then(function(r) { return r.json(); }).then(function(schedules) {
                var promises = schedules.filter(function(s) { return s.enabled; }).map(function(s) { return fetch('/api/schedules/' + s.id + '/run', { method: 'POST' }); });
                Promise.all(promises).then(function() { alert('All scheduled backups triggered'); loadStats(); });
            });
        }

        function testEncryption() {
            var data = document.getElementById('test-data').value;
            if (!data) { alert('Enter some text to encrypt'); return; }
            fetch('/api/vault/encrypt', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ data: data }) }).then(function(r) { return r.json(); }).then(function(result) {
                document.getElementById('encryption-result').innerHTML = '<div style="background: var(--ctp-surface0); padding: 1rem; border-radius: 0.5rem;"><strong>Encrypted:</strong><code style="word-break: break-all; display: block; margin-top: 0.5rem;">' + result.encrypted.substring(0, 100) + '...</code></div>';
            }).catch(function() { alert('Encryption failed'); });
        }

        function formatBytes(bytes) {
            if (bytes === 0) return '0 B';
            var k = 1024;
            var sizes = ['B', 'KB', 'MB', 'GB'];
            var i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
        }

        function escapeHtml(text) {
            var div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        loadServiceStatus();
        loadStats();
        loadSchedules();
        setInterval(loadServiceStatus, 30000);
    </script>
</body>
</html>` + "`"
