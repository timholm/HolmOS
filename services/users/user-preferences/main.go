package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

var db *sql.DB

type Preference struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PreferenceUpdate struct {
	Value string `json:"value"`
}

type PreferencesResponse struct {
	UserID      string       `json:"user_id"`
	Preferences []Preference `json:"preferences"`
}

type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

func main() {
	// Get database connection string from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/preferences?sslmode=disable"
	}

	var err error
	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Initialize database schema
	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	log.Println("Connected to database successfully")

	// Set up router
	r := mux.NewRouter()

	// Health check
	r.HandleFunc("/health", healthHandler).Methods("GET")

	// Preferences endpoints
	r.HandleFunc("/preferences/{user_id}", getPreferencesHandler).Methods("GET")
	r.HandleFunc("/preferences/{user_id}", updatePreferencesHandler).Methods("PUT")
	r.HandleFunc("/preferences/{user_id}/{key}", getPreferenceHandler).Methods("GET")
	r.HandleFunc("/preferences/{user_id}/{key}", setPreferenceHandler).Methods("PUT")
	r.HandleFunc("/preferences/{user_id}/{key}", deletePreferenceHandler).Methods("DELETE")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting user-preferences service on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func initDB() error {
	query := `
	CREATE TABLE IF NOT EXISTS user_preferences (
		user_id VARCHAR(255) NOT NULL,
		key VARCHAR(255) NOT NULL,
		value TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, key)
	);
	CREATE INDEX IF NOT EXISTS idx_user_preferences_user_id ON user_preferences(user_id);
	`
	_, err := db.Exec(query)
	return err
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:   "healthy",
		Database: "connected",
	}

	if err := db.Ping(); err != nil {
		response.Status = "unhealthy"
		response.Database = "disconnected"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func getPreferencesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["user_id"]

	rows, err := db.Query("SELECT key, value FROM user_preferences WHERE user_id = $1", userID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("Error querying preferences: %v", err)
		return
	}
	defer rows.Close()

	preferences := []Preference{}
	for rows.Next() {
		var pref Preference
		if err := rows.Scan(&pref.Key, &pref.Value); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			log.Printf("Error scanning preference: %v", err)
			return
		}
		preferences = append(preferences, pref)
	}

	response := PreferencesResponse{
		UserID:      userID,
		Preferences: preferences,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func updatePreferencesHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["user_id"]

	var preferences []Preference
	if err := json.NewDecoder(r.Body).Decode(&preferences); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("Error starting transaction: %v", err)
		return
	}

	for _, pref := range preferences {
		_, err := tx.Exec(`
			INSERT INTO user_preferences (user_id, key, value, updated_at)
			VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
			ON CONFLICT (user_id, key)
			DO UPDATE SET value = $3, updated_at = CURRENT_TIMESTAMP
		`, userID, pref.Key, pref.Value)
		if err != nil {
			tx.Rollback()
			http.Error(w, "Database error", http.StatusInternalServerError)
			log.Printf("Error upserting preference: %v", err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("Error committing transaction: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

func getPreferenceHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["user_id"]
	key := vars["key"]

	var value string
	err := db.QueryRow("SELECT value FROM user_preferences WHERE user_id = $1 AND key = $2", userID, key).Scan(&value)
	if err == sql.ErrNoRows {
		http.Error(w, "Preference not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("Error querying preference: %v", err)
		return
	}

	pref := Preference{
		Key:   key,
		Value: value,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pref)
}

func setPreferenceHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["user_id"]
	key := vars["key"]

	var update PreferenceUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate value is not empty
	if strings.TrimSpace(update.Value) == "" {
		http.Error(w, "Value cannot be empty", http.StatusBadRequest)
		return
	}

	_, err := db.Exec(`
		INSERT INTO user_preferences (user_id, key, value, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, key)
		DO UPDATE SET value = $3, updated_at = CURRENT_TIMESTAMP
	`, userID, key, update.Value)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("Error upserting preference: %v", err)
		return
	}

	pref := Preference{
		Key:   key,
		Value: update.Value,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(pref)
}

func deletePreferenceHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["user_id"]
	key := vars["key"]

	result, err := db.Exec("DELETE FROM user_preferences WHERE user_id = $1 AND key = $2", userID, key)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("Error deleting preference: %v", err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("Error getting rows affected: %v", err)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, "Preference not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
