package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Notification struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Recipient   string                 `json:"recipient"`
	Subject     string                 `json:"subject"`
	Content     string                 `json:"content"`
	Priority    int                    `json:"priority"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	ProcessedAt *time.Time             `json:"processed_at,omitempty"`
}

type QueueStats struct {
	TotalQueued   int            `json:"total_queued"`
	Pending       int            `json:"pending"`
	Processed     int            `json:"processed"`
	ByType        map[string]int `json:"by_type"`
	ByPriority    map[int]int    `json:"by_priority"`
	OldestPending *time.Time     `json:"oldest_pending,omitempty"`
}

type NotificationQueue struct {
	notifications map[string]*Notification
	mu            sync.RWMutex
	db            *sql.DB
}

var queue *NotificationQueue

func NewNotificationQueue(db *sql.DB) *NotificationQueue {
	nq := &NotificationQueue{
		notifications: make(map[string]*Notification),
		db:            db,
	}
	if db != nil {
		nq.loadFromDB()
	}
	return nq
}

func (nq *NotificationQueue) loadFromDB() {
	rows, err := nq.db.Query(`
		SELECT id, type, recipient, subject, content, priority, metadata, status, created_at, processed_at
		FROM notifications ORDER BY created_at ASC
	`)
	if err != nil {
		log.Printf("Error loading notifications from DB: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var n Notification
		var metadataJSON []byte
		var processedAt sql.NullTime

		err := rows.Scan(&n.ID, &n.Type, &n.Recipient, &n.Subject, &n.Content,
			&n.Priority, &metadataJSON, &n.Status, &n.CreatedAt, &processedAt)
		if err != nil {
			log.Printf("Error scanning notification: %v", err)
			continue
		}

		if len(metadataJSON) > 0 {
			json.Unmarshal(metadataJSON, &n.Metadata)
		}
		if processedAt.Valid {
			n.ProcessedAt = &processedAt.Time
		}

		nq.notifications[n.ID] = &n
	}
	log.Printf("Loaded %d notifications from database", len(nq.notifications))
}

func (nq *NotificationQueue) Add(n *Notification) error {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	nq.notifications[n.ID] = n

	if nq.db != nil {
		metadataJSON, _ := json.Marshal(n.Metadata)
		_, err := nq.db.Exec(`
			INSERT INTO notifications (id, type, recipient, subject, content, priority, metadata, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				type = EXCLUDED.type,
				recipient = EXCLUDED.recipient,
				subject = EXCLUDED.subject,
				content = EXCLUDED.content,
				priority = EXCLUDED.priority,
				metadata = EXCLUDED.metadata,
				status = EXCLUDED.status
		`, n.ID, n.Type, n.Recipient, n.Subject, n.Content, n.Priority, metadataJSON, n.Status, n.CreatedAt)
		if err != nil {
			log.Printf("Error persisting notification: %v", err)
			return err
		}
	}
	return nil
}

func (nq *NotificationQueue) Get(id string) (*Notification, bool) {
	nq.mu.RLock()
	defer nq.mu.RUnlock()
	n, ok := nq.notifications[id]
	return n, ok
}

func (nq *NotificationQueue) GetPending() []*Notification {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	var pending []*Notification
	for _, n := range nq.notifications {
		if n.Status == "pending" {
			pending = append(pending, n)
		}
	}
	return pending
}

func (nq *NotificationQueue) MarkProcessed(id string) (*Notification, bool) {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	n, ok := nq.notifications[id]
	if !ok {
		return nil, false
	}

	now := time.Now()
	n.Status = "processed"
	n.ProcessedAt = &now

	if nq.db != nil {
		_, err := nq.db.Exec(`
			UPDATE notifications SET status = $1, processed_at = $2 WHERE id = $3
		`, n.Status, n.ProcessedAt, n.ID)
		if err != nil {
			log.Printf("Error updating notification status: %v", err)
		}
	}
	return n, true
}

func (nq *NotificationQueue) Delete(id string) bool {
	nq.mu.Lock()
	defer nq.mu.Unlock()

	if _, ok := nq.notifications[id]; !ok {
		return false
	}

	delete(nq.notifications, id)

	if nq.db != nil {
		_, err := nq.db.Exec(`DELETE FROM notifications WHERE id = $1`, id)
		if err != nil {
			log.Printf("Error deleting notification: %v", err)
		}
	}
	return true
}

func (nq *NotificationQueue) GetStats() QueueStats {
	nq.mu.RLock()
	defer nq.mu.RUnlock()

	stats := QueueStats{
		TotalQueued: len(nq.notifications),
		ByType:      make(map[string]int),
		ByPriority:  make(map[int]int),
	}

	var oldestPending *time.Time

	for _, n := range nq.notifications {
		stats.ByType[n.Type]++
		stats.ByPriority[n.Priority]++

		if n.Status == "pending" {
			stats.Pending++
			if oldestPending == nil || n.CreatedAt.Before(*oldestPending) {
				t := n.CreatedAt
				oldestPending = &t
			}
		} else if n.Status == "processed" {
			stats.Processed++
		}
	}

	stats.OldestPending = oldestPending
	return stats
}

func generateID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "notification-queue",
	})
}

func addToQueueHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Type      string                 `json:"type"`
		Recipient string                 `json:"recipient"`
		Subject   string                 `json:"subject"`
		Content   string                 `json:"content"`
		Priority  int                    `json:"priority"`
		Metadata  map[string]interface{} `json:"metadata"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	if input.Type == "" || input.Recipient == "" {
		http.Error(w, `{"error": "type and recipient are required"}`, http.StatusBadRequest)
		return
	}

	notification := &Notification{
		ID:        generateID(),
		Type:      input.Type,
		Recipient: input.Recipient,
		Subject:   input.Subject,
		Content:   input.Content,
		Priority:  input.Priority,
		Metadata:  input.Metadata,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	if err := queue.Add(notification); err != nil {
		http.Error(w, `{"error": "failed to add notification"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(notification)
}

func getPendingHandler(w http.ResponseWriter, r *http.Request) {
	pending := queue.GetPending()
	if pending == nil {
		pending = []*Notification{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pending)
}

func processNotificationHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	notification, ok := queue.MarkProcessed(id)
	if !ok {
		http.Error(w, `{"error": "notification not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notification)
}

func deleteNotificationHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if !queue.Delete(id) {
		http.Error(w, `{"error": "notification not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "notification deleted",
		"id":      id,
	})
}

func getStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats := queue.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	var db *sql.DB
	var err error

	if dbURL != "" {
		db, err = sql.Open("postgres", dbURL)
		if err != nil {
			log.Printf("Warning: Could not connect to database: %v", err)
		} else {
			if err = db.Ping(); err != nil {
				log.Printf("Warning: Could not ping database: %v", err)
				db = nil
			} else {
				log.Println("Connected to PostgreSQL database")
				// Create table if not exists
				_, err = db.Exec(`
					CREATE TABLE IF NOT EXISTS notifications (
						id VARCHAR(255) PRIMARY KEY,
						type VARCHAR(100) NOT NULL,
						recipient VARCHAR(255) NOT NULL,
						subject VARCHAR(500),
						content TEXT,
						priority INTEGER DEFAULT 0,
						metadata JSONB,
						status VARCHAR(50) DEFAULT 'pending',
						created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
						processed_at TIMESTAMP
					)
				`)
				if err != nil {
					log.Printf("Warning: Could not create table: %v", err)
				}
			}
		}
	}

	queue = NewNotificationQueue(db)

	r := mux.NewRouter()

	r.HandleFunc("/health", healthHandler).Methods("GET")
	r.HandleFunc("/queue", addToQueueHandler).Methods("POST")
	r.HandleFunc("/queue", getPendingHandler).Methods("GET")
	r.HandleFunc("/queue/stats", getStatsHandler).Methods("GET")
	r.HandleFunc("/queue/{id}/process", processNotificationHandler).Methods("POST")
	r.HandleFunc("/queue/{id}", deleteNotificationHandler).Methods("DELETE")

	log.Printf("Notification Queue service starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}
