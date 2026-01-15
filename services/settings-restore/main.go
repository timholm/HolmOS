package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

type SettingsImport struct {
	Version    string                 `json:"version"`
	ExportedAt time.Time              `json:"exportedAt"`
	Cluster    map[string]interface{} `json:"cluster"`
	Registry   map[string]interface{} `json:"registry"`
	Theme      map[string]interface{} `json:"theme"`
	TabState   map[string]interface{} `json:"tabState"`
	CustomData map[string]interface{} `json:"customData,omitempty"`
}

type RestoreResult struct {
	Status   string   `json:"status"`
	Restored []string `json:"restored"`
	Errors   []string `json:"errors,omitempty"`
}

func main() {
	// Connect to PostgreSQL
	connStr := "host=postgres.holm.svc.cluster.local port=5432 user=postgres password=postgres dbname=holm sslmode=disable"
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Warning: Could not connect to database: %v", err)
	} else {
		defer db.Close()
		initDB()
	}

	// Routes
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/import", importHandler)
	http.HandleFunc("/validate", validateHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Settings Restore service starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func initDB() {
	// Ensure all required tables exist
	tables := []string{
		`CREATE TABLE IF NOT EXISTS cluster_settings (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) DEFAULT 'holm-cluster',
			resource_limits BOOLEAN DEFAULT true,
			auto_scaling BOOLEAN DEFAULT false,
			network_policy BOOLEAN DEFAULT true,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS registry_settings (
			id SERIAL PRIMARY KEY,
			url VARCHAR(255) DEFAULT '10.110.67.87:5000',
			namespace VARCHAR(255) DEFAULT 'holm',
			insecure BOOLEAN DEFAULT true,
			pull_policy VARCHAR(50) DEFAULT 'IfNotPresent',
			image_gc BOOLEAN DEFAULT true,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS theme_preferences (
			id SERIAL PRIMARY KEY,
			user_id VARCHAR(255) UNIQUE DEFAULT 'default',
			theme VARCHAR(50) DEFAULT 'mocha',
			compact_mode BOOLEAN DEFAULT false,
			animations BOOLEAN DEFAULT true,
			font_size INTEGER DEFAULT 16,
			accent_color VARCHAR(20) DEFAULT 'lavender',
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tab_states (
			id SERIAL PRIMARY KEY,
			user_id VARCHAR(255) UNIQUE DEFAULT 'default',
			active_tab VARCHAR(50) DEFAULT 'cluster',
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, table := range tables {
		_, err := db.Exec(table)
		if err != nil {
			log.Printf("Error creating table: %v", err)
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": "settings-restore"})
}

func importHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var settings SettingsImport
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := RestoreResult{
		Status:   "ok",
		Restored: []string{},
		Errors:   []string{},
	}

	// Restore cluster settings
	if settings.Cluster != nil {
		if err := restoreClusterSettings(settings.Cluster); err != nil {
			result.Errors = append(result.Errors, "cluster: "+err.Error())
		} else {
			result.Restored = append(result.Restored, "cluster")
		}
	}

	// Restore registry settings
	if settings.Registry != nil {
		if err := restoreRegistrySettings(settings.Registry); err != nil {
			result.Errors = append(result.Errors, "registry: "+err.Error())
		} else {
			result.Restored = append(result.Restored, "registry")
		}
	}

	// Restore theme settings
	if settings.Theme != nil {
		if err := restoreThemeSettings(settings.Theme); err != nil {
			result.Errors = append(result.Errors, "theme: "+err.Error())
		} else {
			result.Restored = append(result.Restored, "theme")
		}
	}

	// Restore tab state
	if settings.TabState != nil {
		if err := restoreTabState(settings.TabState); err != nil {
			result.Errors = append(result.Errors, "tabState: "+err.Error())
		} else {
			result.Restored = append(result.Restored, "tabState")
		}
	}

	if len(result.Errors) > 0 {
		result.Status = "partial"
	}

	json.NewEncoder(w).Encode(result)
}

func restoreClusterSettings(data map[string]interface{}) error {
	if db == nil {
		return nil
	}

	name := getStringValue(data, "name", "holm-cluster")
	resourceLimits := getBoolValue(data, "resourceLimits", true)
	autoScaling := getBoolValue(data, "autoScaling", false)
	networkPolicy := getBoolValue(data, "networkPolicy", true)

	// Delete existing and insert new
	db.Exec("DELETE FROM cluster_settings")
	_, err := db.Exec(`
		INSERT INTO cluster_settings (name, resource_limits, auto_scaling, network_policy)
		VALUES ($1, $2, $3, $4)
	`, name, resourceLimits, autoScaling, networkPolicy)

	return err
}

func restoreRegistrySettings(data map[string]interface{}) error {
	if db == nil {
		return nil
	}

	url := getStringValue(data, "url", "10.110.67.87:5000")
	namespace := getStringValue(data, "namespace", "holm")
	insecure := getBoolValue(data, "insecure", true)
	pullPolicy := getStringValue(data, "pullPolicy", "IfNotPresent")
	imageGC := getBoolValue(data, "imageGC", true)

	// Delete existing and insert new
	db.Exec("DELETE FROM registry_settings")
	_, err := db.Exec(`
		INSERT INTO registry_settings (url, namespace, insecure, pull_policy, image_gc)
		VALUES ($1, $2, $3, $4, $5)
	`, url, namespace, insecure, pullPolicy, imageGC)

	return err
}

func restoreThemeSettings(data map[string]interface{}) error {
	if db == nil {
		return nil
	}

	theme := getStringValue(data, "theme", "mocha")
	compactMode := getBoolValue(data, "compactMode", false)
	animations := getBoolValue(data, "animations", true)
	fontSize := getIntValue(data, "fontSize", 16)
	accentColor := getStringValue(data, "accentColor", "lavender")

	// Upsert for default user
	_, err := db.Exec(`
		INSERT INTO theme_preferences (user_id, theme, compact_mode, animations, font_size, accent_color, updated_at)
		VALUES ('default', $1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE SET
			theme = $1,
			compact_mode = $2,
			animations = $3,
			font_size = $4,
			accent_color = $5,
			updated_at = CURRENT_TIMESTAMP
	`, theme, compactMode, animations, fontSize, accentColor)

	return err
}

func restoreTabState(data map[string]interface{}) error {
	if db == nil {
		return nil
	}

	activeTab := getStringValue(data, "activeTab", "cluster")

	// Upsert for default user
	_, err := db.Exec(`
		INSERT INTO tab_states (user_id, active_tab, updated_at)
		VALUES ('default', $1, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE SET
			active_tab = $1,
			updated_at = CURRENT_TIMESTAMP
	`, activeTab)

	return err
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var settings SettingsImport
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":  false,
			"error":  "Invalid JSON format",
			"detail": err.Error(),
		})
		return
	}

	// Validate structure
	validation := map[string]interface{}{
		"valid":    true,
		"sections": []string{},
	}

	sections := []string{}
	if settings.Cluster != nil {
		sections = append(sections, "cluster")
	}
	if settings.Registry != nil {
		sections = append(sections, "registry")
	}
	if settings.Theme != nil {
		sections = append(sections, "theme")
	}
	if settings.TabState != nil {
		sections = append(sections, "tabState")
	}

	validation["sections"] = sections

	if len(sections) == 0 {
		validation["valid"] = false
		validation["error"] = "No valid settings sections found"
	}

	json.NewEncoder(w).Encode(validation)
}

// Helper functions
func getStringValue(data map[string]interface{}, key, defaultValue string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func getBoolValue(data map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := data[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

func getIntValue(data map[string]interface{}, key string, defaultValue int) int {
	if val, ok := data[key]; ok {
		if f, ok := val.(float64); ok {
			return int(f)
		}
		if i, ok := val.(int); ok {
			return i
		}
	}
	return defaultValue
}
