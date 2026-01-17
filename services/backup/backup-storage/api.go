package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// UpdateBackupRequest represents a request to update backup metadata
type UpdateBackupRequest struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// BackupStats represents backup statistics
type BackupStats struct {
	TotalCount   int64            `json:"total_count"`
	TotalSize    int64            `json:"total_size"`
	CountByType  map[string]int64 `json:"count_by_type"`
	SizeByType   map[string]int64 `json:"size_by_type"`
	OldestBackup *time.Time       `json:"oldest_backup,omitempty"`
	NewestBackup *time.Time       `json:"newest_backup,omitempty"`
}

// RestoreResponse represents the response from a restore operation
type RestoreResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	Data string `json:"data"` // Base64 encoded data
}

// VerifyResponse represents the response from a verify operation
type VerifyResponse struct {
	ID             string `json:"id"`
	Valid          bool   `json:"valid"`
	FileExists     bool   `json:"file_exists"`
	SizeMatches    bool   `json:"size_matches"`
	ExpectedSize   int64  `json:"expected_size"`
	ActualSize     int64  `json:"actual_size"`
	Message        string `json:"message"`
}

// SearchResult represents search results
type SearchResult struct {
	Backups []BackupResponse `json:"backups"`
	Count   int              `json:"count"`
	Query   string           `json:"query"`
}

// updateBackupHandler handles PUT /backups/{id}
func updateBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req UpdateBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Check if backup exists
	var existingName, existingType string
	err := db.QueryRow(`SELECT name, type FROM backups WHERE id = $1`, id).Scan(&existingName, &existingType)
	if err == sql.ErrNoRows {
		sendError(w, "Backup not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Failed to query backup: %v", err)
		sendError(w, "Failed to retrieve backup", http.StatusInternalServerError)
		return
	}

	// Use existing values if not provided
	if req.Name == "" {
		req.Name = existingName
	}
	if req.Type == "" {
		req.Type = existingType
	}

	// Update backup metadata
	now := time.Now().UTC()
	_, err = db.Exec(`
		UPDATE backups
		SET name = $1, type = $2, updated_at = $3
		WHERE id = $4`,
		req.Name, req.Type, now, id)
	if err != nil {
		log.Printf("Failed to update backup: %v", err)
		sendError(w, "Failed to update backup", http.StatusInternalServerError)
		return
	}

	// Fetch updated backup
	var b BackupResponse
	err = db.QueryRow(`
		SELECT id, name, type, size, created_at, updated_at
		FROM backups WHERE id = $1`, id).Scan(
		&b.ID, &b.Name, &b.Type, &b.Size, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		log.Printf("Failed to fetch updated backup: %v", err)
		sendError(w, "Failed to retrieve updated backup", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(b)
}

// statsHandler handles GET /backups/stats
func statsHandler(w http.ResponseWriter, r *http.Request) {
	stats := BackupStats{
		CountByType: make(map[string]int64),
		SizeByType:  make(map[string]int64),
	}

	// Get total count and size
	err := db.QueryRow(`
		SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(size), 0)
		FROM backups`).Scan(&stats.TotalCount, &stats.TotalSize)
	if err != nil {
		log.Printf("Failed to get total stats: %v", err)
		sendError(w, "Failed to retrieve statistics", http.StatusInternalServerError)
		return
	}

	// Get count and size by type
	rows, err := db.Query(`
		SELECT type, COUNT(*), COALESCE(SUM(size), 0)
		FROM backups
		GROUP BY type`)
	if err != nil {
		log.Printf("Failed to get type stats: %v", err)
		sendError(w, "Failed to retrieve statistics", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var backupType string
		var count, size int64
		if err := rows.Scan(&backupType, &count, &size); err != nil {
			log.Printf("Failed to scan type stats: %v", err)
			continue
		}
		stats.CountByType[backupType] = count
		stats.SizeByType[backupType] = size
	}

	// Get oldest and newest backup timestamps
	var oldest, newest sql.NullTime
	err = db.QueryRow(`SELECT MIN(created_at), MAX(created_at) FROM backups`).Scan(&oldest, &newest)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to get timestamp stats: %v", err)
	}
	if oldest.Valid {
		stats.OldestBackup = &oldest.Time
	}
	if newest.Valid {
		stats.NewestBackup = &newest.Time
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// restoreBackupHandler handles POST /backups/{id}/restore
func restoreBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Get backup metadata
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

	// Read the backup file
	file, err := os.Open(backup.FilePath)
	if err != nil {
		log.Printf("Failed to open backup file: %v", err)
		sendError(w, "Backup file not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		log.Printf("Failed to read backup file: %v", err)
		sendError(w, "Failed to read backup file", http.StatusInternalServerError)
		return
	}

	// Encode data as base64
	response := RestoreResponse{
		ID:   backup.ID,
		Name: backup.Name,
		Type: backup.Type,
		Size: backup.Size,
		Data: base64.StdEncoding.EncodeToString(data),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// searchBackupsHandler handles GET /backups/search
func searchBackupsHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		sendError(w, "Search query 'q' is required", http.StatusBadRequest)
		return
	}

	// Sanitize query for LIKE pattern
	searchPattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"

	rows, err := db.Query(`
		SELECT id, name, type, size, created_at, updated_at
		FROM backups
		WHERE name ILIKE $1 OR type ILIKE $1
		ORDER BY created_at DESC
		LIMIT 100`, searchPattern)
	if err != nil {
		log.Printf("Failed to search backups: %v", err)
		sendError(w, "Failed to search backups", http.StatusInternalServerError)
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

	result := SearchResult{
		Backups: backups,
		Count:   len(backups),
		Query:   query,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// verifyBackupHandler handles POST /backups/{id}/verify
func verifyBackupHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Get backup metadata
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

	response := VerifyResponse{
		ID:           backup.ID,
		ExpectedSize: backup.Size,
	}

	// Check if file exists
	fileInfo, err := os.Stat(backup.FilePath)
	if os.IsNotExist(err) {
		response.Valid = false
		response.FileExists = false
		response.SizeMatches = false
		response.ActualSize = 0
		response.Message = "Backup file does not exist"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}
	if err != nil {
		log.Printf("Failed to stat backup file: %v", err)
		sendError(w, "Failed to verify backup file", http.StatusInternalServerError)
		return
	}

	response.FileExists = true
	response.ActualSize = fileInfo.Size()
	response.SizeMatches = fileInfo.Size() == backup.Size

	if response.FileExists && response.SizeMatches {
		response.Valid = true
		response.Message = "Backup is valid"
	} else if response.FileExists && !response.SizeMatches {
		response.Valid = false
		response.Message = "Backup file size mismatch"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// registerAPIRoutes registers the additional API routes
func registerAPIRoutes(r *mux.Router) {
	r.HandleFunc("/backups/stats", statsHandler).Methods("GET")
	r.HandleFunc("/backups/search", searchBackupsHandler).Methods("GET")
	r.HandleFunc("/backups/{id}", updateBackupHandler).Methods("PUT")
	r.HandleFunc("/backups/{id}/restore", restoreBackupHandler).Methods("POST")
	r.HandleFunc("/backups/{id}/verify", verifyBackupHandler).Methods("POST")
}
