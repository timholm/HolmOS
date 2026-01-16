package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Name      string    `json:"name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type WebhookRequest struct {
	URL  string `json:"url"`
	Name string `json:"name,omitempty"`
}

type SendRequest struct {
	Event   string                 `json:"event"`
	Payload map[string]interface{} `json:"payload"`
}

type DeliveryHistory struct {
	ID         string    `json:"id"`
	WebhookID  string    `json:"webhook_id"`
	Event      string    `json:"event"`
	StatusCode int       `json:"status_code"`
	Success    bool      `json:"success"`
	Attempts   int       `json:"attempts"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type SendResult struct {
	WebhookID  string `json:"webhook_id"`
	URL        string `json:"url"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code,omitempty"`
	Attempts   int    `json:"attempts"`
	Error      string `json:"error,omitempty"`
}

var db *sql.DB

func main() {
	var err error

	dbHost := getEnv("DB_HOST", "postgres.holm.svc.cluster.local")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "notifications")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

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

	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	r := mux.NewRouter()

	r.HandleFunc("/health", healthHandler).Methods("GET")
	r.HandleFunc("/webhooks", createWebhookHandler).Methods("POST")
	r.HandleFunc("/webhooks", listWebhooksHandler).Methods("GET")
	r.HandleFunc("/webhooks/{id}", deleteWebhookHandler).Methods("DELETE")
	r.HandleFunc("/webhooks/{id}/history", getHistoryHandler).Methods("GET")
	r.HandleFunc("/send", sendNotificationHandler).Methods("POST")

	port := getEnv("PORT", "8080")
	log.Printf("Starting notification-webhook service on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func initDB() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS webhooks (
			id VARCHAR(36) PRIMARY KEY,
			url TEXT NOT NULL,
			name VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create webhooks table: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS delivery_history (
			id VARCHAR(36) PRIMARY KEY,
			webhook_id VARCHAR(36) REFERENCES webhooks(id) ON DELETE CASCADE,
			event VARCHAR(255),
			status_code INT,
			success BOOLEAN,
			attempts INT,
			error TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create delivery_history table: %v", err)
	}

	log.Println("Database tables initialized")
	return nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if err := db.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func createWebhookHandler(w http.ResponseWriter, r *http.Request) {
	var req WebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, `{"error": "url is required"}`, http.StatusBadRequest)
		return
	}

	id := generateID()
	webhook := Webhook{
		ID:        id,
		URL:       req.URL,
		Name:      req.Name,
		CreatedAt: time.Now(),
	}

	_, err := db.Exec("INSERT INTO webhooks (id, url, name, created_at) VALUES ($1, $2, $3, $4)",
		webhook.ID, webhook.URL, webhook.Name, webhook.CreatedAt)
	if err != nil {
		log.Printf("Failed to insert webhook: %v", err)
		http.Error(w, `{"error": "failed to create webhook"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(webhook)
}

func listWebhooksHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, url, name, created_at FROM webhooks ORDER BY created_at DESC")
	if err != nil {
		log.Printf("Failed to query webhooks: %v", err)
		http.Error(w, `{"error": "failed to list webhooks"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	webhooks := []Webhook{}
	for rows.Next() {
		var webhook Webhook
		var name sql.NullString
		if err := rows.Scan(&webhook.ID, &webhook.URL, &name, &webhook.CreatedAt); err != nil {
			log.Printf("Failed to scan webhook: %v", err)
			continue
		}
		if name.Valid {
			webhook.Name = name.String
		}
		webhooks = append(webhooks, webhook)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhooks)
}

func deleteWebhookHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	result, err := db.Exec("DELETE FROM webhooks WHERE id = $1", id)
	if err != nil {
		log.Printf("Failed to delete webhook: %v", err)
		http.Error(w, `{"error": "failed to delete webhook"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error": "webhook not found"}`, http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func getHistoryHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	webhookID := vars["id"]

	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM webhooks WHERE id = $1)", webhookID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, `{"error": "webhook not found"}`, http.StatusNotFound)
		return
	}

	rows, err := db.Query(`
		SELECT id, webhook_id, event, status_code, success, attempts, error, created_at
		FROM delivery_history
		WHERE webhook_id = $1
		ORDER BY created_at DESC
		LIMIT 100
	`, webhookID)
	if err != nil {
		log.Printf("Failed to query history: %v", err)
		http.Error(w, `{"error": "failed to get history"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	history := []DeliveryHistory{}
	for rows.Next() {
		var h DeliveryHistory
		var errMsg sql.NullString
		if err := rows.Scan(&h.ID, &h.WebhookID, &h.Event, &h.StatusCode, &h.Success, &h.Attempts, &errMsg, &h.CreatedAt); err != nil {
			log.Printf("Failed to scan history: %v", err)
			continue
		}
		if errMsg.Valid {
			h.Error = errMsg.String
		}
		history = append(history, h)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func sendNotificationHandler(w http.ResponseWriter, r *http.Request) {
	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Event == "" {
		http.Error(w, `{"error": "event is required"}`, http.StatusBadRequest)
		return
	}

	rows, err := db.Query("SELECT id, url, name FROM webhooks")
	if err != nil {
		log.Printf("Failed to query webhooks: %v", err)
		http.Error(w, `{"error": "failed to get webhooks"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var webhooks []Webhook
	for rows.Next() {
		var webhook Webhook
		var name sql.NullString
		if err := rows.Scan(&webhook.ID, &webhook.URL, &name); err != nil {
			continue
		}
		if name.Valid {
			webhook.Name = name.String
		}
		webhooks = append(webhooks, webhook)
	}

	if len(webhooks) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "no webhooks registered",
			"results": []SendResult{},
		})
		return
	}

	payload := map[string]interface{}{
		"event":     req.Event,
		"payload":   req.Payload,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	payloadBytes, _ := json.Marshal(payload)

	results := []SendResult{}
	for _, webhook := range webhooks {
		result := sendWithRetry(webhook, payloadBytes, req.Event)
		results = append(results, result)

		recordDelivery(webhook.ID, req.Event, result)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": fmt.Sprintf("sent to %d webhooks", len(webhooks)),
		"results": results,
	})
}

func sendWithRetry(webhook Webhook, payload []byte, event string) SendResult {
	maxAttempts := 3
	result := SendResult{
		WebhookID: webhook.ID,
		URL:       webhook.URL,
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result.Attempts = attempt

		req, err := http.NewRequest("POST", webhook.URL, bytes.NewReader(payload))
		if err != nil {
			result.Error = fmt.Sprintf("failed to create request: %v", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Event", event)
		req.Header.Set("X-Webhook-Attempt", fmt.Sprintf("%d", attempt))

		resp, err := client.Do(req)
		if err != nil {
			result.Error = fmt.Sprintf("request failed: %v", err)
			if attempt < maxAttempts {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
			continue
		}
		resp.Body.Close()

		result.StatusCode = resp.StatusCode

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result.Success = true
			result.Error = ""
			return result
		}

		result.Error = fmt.Sprintf("received status code %d", resp.StatusCode)
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return result
}

func recordDelivery(webhookID, event string, result SendResult) {
	_, err := db.Exec(`
		INSERT INTO delivery_history (id, webhook_id, event, status_code, success, attempts, error, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, generateID(), webhookID, event, result.StatusCode, result.Success, result.Attempts, result.Error, time.Now())

	if err != nil {
		log.Printf("Failed to record delivery: %v", err)
	}
}

func generateID() string {
	return fmt.Sprintf("%d%d", time.Now().UnixNano(), time.Now().UnixNano()%10000)
}
