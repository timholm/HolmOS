package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
)

// Catppuccin Mocha colors
const (
	ColorBase     = "#1e1e2e"
	ColorMantle   = "#181825"
	ColorCrust    = "#11111b"
	ColorText     = "#cdd6f4"
	ColorSubtext0 = "#a6adc8"
	ColorGreen    = "#a6e3a1"
	ColorRed      = "#f38ba8"
	ColorBlue     = "#89b4fa"
	ColorMauve    = "#cba6f7"
	ColorMaroon   = "#eba0ac"
)

type Logger struct {
	mu sync.Mutex
}

func (l *Logger) Log(level, msg string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     level,
		"service":   "event-dlq",
		"message":   msg,
	}
	for k, v := range fields {
		entry[k] = v
	}
	json.NewEncoder(os.Stdout).Encode(entry)
}

var (
	logger          = &Logger{}
	db              *sql.DB
	nc              *nats.Conn
	requestCounter  uint64
	dlqReceived     uint64
	dlqProcessed    uint64
)

type DLQEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Data      map[string]interface{} `json:"data"`
	Error     string                 `json:"error"`
	Timestamp time.Time              `json:"timestamp"`
}

func initDB() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@postgres:5432/holm?sslmode=disable"
	}

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Create DLQ tables
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS dead_letter_queue (
			id SERIAL PRIMARY KEY,
			event_id VARCHAR(255),
			event_type VARCHAR(255),
			source VARCHAR(255),
			data JSONB,
			error_message TEXT,
			original_timestamp TIMESTAMPTZ,
			received_at TIMESTAMPTZ DEFAULT NOW(),
			processed BOOLEAN DEFAULT FALSE,
			processed_at TIMESTAMPTZ
		);
		CREATE INDEX IF NOT EXISTS idx_dlq_processed ON dead_letter_queue(processed);
		CREATE INDEX IF NOT EXISTS idx_dlq_event_type ON dead_letter_queue(event_type);

		CREATE TABLE IF NOT EXISTS failed_events (
			id VARCHAR(255) PRIMARY KEY,
			type VARCHAR(255) NOT NULL,
			source VARCHAR(255) NOT NULL,
			data JSONB,
			error TEXT,
			retries INT DEFAULT 0,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			last_retry TIMESTAMPTZ
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	logger.Log("info", "Database initialized", nil)
	return nil
}

func initNATS() error {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://event-broker:4222"
	}

	var err error
	nc, err = nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Subscribe to DLQ
	_, err = nc.Subscribe("events.dlq", func(msg *nats.Msg) {
		atomic.AddUint64(&dlqReceived, 1)

		var event DLQEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			logger.Log("error", "Failed to unmarshal DLQ event", map[string]interface{}{"error": err.Error()})
			return
		}

		if err := storeDLQEvent(&event); err != nil {
			logger.Log("error", "Failed to store DLQ event", map[string]interface{}{"error": err.Error()})
			return
		}

		atomic.AddUint64(&dlqProcessed, 1)
		logger.Log("info", "DLQ event stored", map[string]interface{}{"event_id": event.ID, "type": event.Type})
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to DLQ: %w", err)
	}

	logger.Log("info", "NATS connected and subscribed to DLQ", map[string]interface{}{"url": natsURL})
	return nil
}

func storeDLQEvent(event *DLQEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dataJSON, err := json.Marshal(event.Data)
	if err != nil {
		dataJSON = []byte("{}")
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO dead_letter_queue (event_id, event_type, source, data, error_message, original_timestamp)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		event.ID, event.Type, event.Source, dataJSON, event.Error, event.Timestamp)

	if err != nil {
		return err
	}

	// Also add to failed_events for replay
	_, err = db.ExecContext(ctx,
		`INSERT INTO failed_events (id, type, source, data, error)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE SET retries = failed_events.retries + 1, last_retry = NOW()`,
		event.ID, event.Type, event.Source, dataJSON, event.Error)

	return err
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCounter, 1)

	dbHealthy := false
	natsHealthy := false

	if db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err == nil {
			dbHealthy = true
		}
	}

	if nc != nil && nc.IsConnected() {
		natsHealthy = true
	}

	status := map[string]interface{}{
		"service":   "event-dlq",
		"status":    "healthy",
		"database":  dbHealthy,
		"nats":      natsHealthy,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if !dbHealthy || !natsHealthy {
		status["status"] = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCounter, 1)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	dbUp := 0
	if db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err == nil {
			dbUp = 1
		}
	}

	natsUp := 0
	if nc != nil && nc.IsConnected() {
		natsUp = 1
	}

	dlqCount := 0
	unprocessedCount := 0
	if db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dead_letter_queue`).Scan(&dlqCount)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dead_letter_queue WHERE processed = FALSE`).Scan(&unprocessedCount)
	}

	fmt.Fprintf(w, "# HELP event_dlq_up Whether the service is up\n")
	fmt.Fprintf(w, "# TYPE event_dlq_up gauge\n")
	fmt.Fprintf(w, "event_dlq_up 1\n")

	fmt.Fprintf(w, "# HELP event_dlq_database_up Whether database is connected\n")
	fmt.Fprintf(w, "# TYPE event_dlq_database_up gauge\n")
	fmt.Fprintf(w, "event_dlq_database_up %d\n", dbUp)

	fmt.Fprintf(w, "# HELP event_dlq_nats_up Whether NATS is connected\n")
	fmt.Fprintf(w, "# TYPE event_dlq_nats_up gauge\n")
	fmt.Fprintf(w, "event_dlq_nats_up %d\n", natsUp)

	fmt.Fprintf(w, "# HELP event_dlq_received_total Total DLQ events received\n")
	fmt.Fprintf(w, "# TYPE event_dlq_received_total counter\n")
	fmt.Fprintf(w, "event_dlq_received_total %d\n", atomic.LoadUint64(&dlqReceived))

	fmt.Fprintf(w, "# HELP event_dlq_processed_total Total DLQ events processed\n")
	fmt.Fprintf(w, "# TYPE event_dlq_processed_total counter\n")
	fmt.Fprintf(w, "event_dlq_processed_total %d\n", atomic.LoadUint64(&dlqProcessed))

	fmt.Fprintf(w, "# HELP event_dlq_queue_size Total events in DLQ\n")
	fmt.Fprintf(w, "# TYPE event_dlq_queue_size gauge\n")
	fmt.Fprintf(w, "event_dlq_queue_size %d\n", dlqCount)

	fmt.Fprintf(w, "# HELP event_dlq_unprocessed Unprocessed events in DLQ\n")
	fmt.Fprintf(w, "# TYPE event_dlq_unprocessed gauge\n")
	fmt.Fprintf(w, "event_dlq_unprocessed %d\n", unprocessedCount)

	fmt.Fprintf(w, "# HELP event_dlq_requests_total Total HTTP requests\n")
	fmt.Fprintf(w, "# TYPE event_dlq_requests_total counter\n")
	fmt.Fprintf(w, "event_dlq_requests_total %d\n", atomic.LoadUint64(&requestCounter))
}

func retryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if db == nil || nc == nil {
		http.Error(w, "Service not ready", http.StatusServiceUnavailable)
		return
	}

	// Get event ID from query parameter
	eventID := r.URL.Query().Get("id")
	if eventID == "" {
		http.Error(w, "Missing event id parameter", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetch the event from DLQ
	var dbID int
	var eventType, source sql.NullString
	var data []byte
	err := db.QueryRowContext(ctx,
		`SELECT id, event_type, source, data FROM dead_letter_queue WHERE event_id = $1 AND processed = FALSE`,
		eventID).Scan(&dbID, &eventType, &source, &data)

	if err == sql.ErrNoRows {
		http.Error(w, "Event not found or already processed", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Republish to the original topic
	subject := fmt.Sprintf("events.%s", eventType.String)
	eventPayload := map[string]interface{}{
		"id":        eventID,
		"type":      eventType.String,
		"source":    source.String,
		"data":      json.RawMessage(data),
		"timestamp": time.Now().UTC(),
		"retry":     true,
	}

	payload, _ := json.Marshal(eventPayload)
	if err := nc.Publish(subject, payload); err != nil {
		http.Error(w, fmt.Sprintf("Failed to republish: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Mark as processed
	_, err = db.ExecContext(ctx,
		`UPDATE dead_letter_queue SET processed = TRUE, processed_at = NOW() WHERE id = $1`,
		dbID)
	if err != nil {
		logger.Log("warn", "Failed to mark DLQ event as processed", map[string]interface{}{"error": err.Error()})
	}

	logger.Log("info", "DLQ event retried", map[string]interface{}{"event_id": eventID, "type": eventType.String})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "retried",
		"event_id": eventID,
		"subject":  subject,
	})
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		http.Error(w, "Database not connected", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT id, event_id, event_type, source, error_message, received_at, processed
		FROM dead_letter_queue ORDER BY received_at DESC LIMIT 100
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var id int
		var eventID, eventType, source, errorMsg sql.NullString
		var receivedAt time.Time
		var processed bool

		if err := rows.Scan(&id, &eventID, &eventType, &source, &errorMsg, &receivedAt, &processed); err != nil {
			continue
		}

		events = append(events, map[string]interface{}{
			"id":         id,
			"event_id":   eventID.String,
			"event_type": eventType.String,
			"source":     source.String,
			"error":      errorMsg.String,
			"received":   receivedAt,
			"processed":  processed,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func uiHandler(w http.ResponseWriter, r *http.Request) {
	dbStatus := "Disconnected"
	dbColor := ColorRed
	if db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err == nil {
			dbStatus = "Connected"
			dbColor = ColorGreen
		}
	}

	natsStatus := "Disconnected"
	natsColor := ColorRed
	if nc != nil && nc.IsConnected() {
		natsStatus = "Connected"
		natsColor = ColorGreen
	}

	dlqCount := 0
	unprocessedCount := 0
	if db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dead_letter_queue`).Scan(&dlqCount)
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dead_letter_queue WHERE processed = FALSE`).Scan(&unprocessedCount)
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Event DLQ</title>
    <style>
        body { background: %s; color: %s; font-family: 'JetBrains Mono', monospace; padding: 2rem; }
        .container { max-width: 800px; margin: 0 auto; }
        h1 { color: %s; }
        .status { padding: 1rem; background: %s; border-radius: 8px; margin: 1rem 0; display: flex; justify-content: space-between; }
        .status-indicator { display: inline-block; width: 12px; height: 12px; border-radius: 50%%; margin-right: 8px; }
        .metric { display: flex; justify-content: space-between; padding: 0.5rem 0; border-bottom: 1px solid %s; }
        .metric-value { color: %s; }
        .warning { color: %s; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Dead Letter Queue</h1>
        <div class="status">
            <span><span class="status-indicator" style="background: %s"></span>Database: %s</span>
            <span><span class="status-indicator" style="background: %s"></span>NATS: %s</span>
        </div>
        <div class="metrics">
            <div class="metric"><span>DLQ Received</span><span class="metric-value">%d</span></div>
            <div class="metric"><span>DLQ Processed</span><span class="metric-value">%d</span></div>
            <div class="metric"><span>Total in Queue</span><span class="metric-value">%d</span></div>
            <div class="metric"><span>Unprocessed</span><span class="metric-value warning">%d</span></div>
        </div>
        <p style="margin-top: 2rem; color: %s;">View events: <a href="/list" style="color: %s">/list</a></p>
    </div>
</body>
</html>`, ColorBase, ColorText, ColorMaroon, ColorMantle, ColorCrust, ColorBlue, ColorRed,
		dbColor, dbStatus, natsColor, natsStatus,
		atomic.LoadUint64(&dlqReceived), atomic.LoadUint64(&dlqProcessed), dlqCount, unprocessedCount,
		ColorSubtext0, ColorMauve)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	logger.Log("info", "Starting event-dlq service", nil)

	if err := initDB(); err != nil {
		logger.Log("warn", "Database initialization failed, will retry", map[string]interface{}{"error": err.Error()})
	}

	if err := initNATS(); err != nil {
		logger.Log("warn", "NATS initialization failed, will retry", map[string]interface{}{"error": err.Error()})
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)
	http.HandleFunc("/list", listHandler)
	http.HandleFunc("/retry", retryHandler)
	http.HandleFunc("/", uiHandler)

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	logger.Log("info", "HTTP server starting", map[string]interface{}{"port": port})

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		logger.Log("error", "HTTP server failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
