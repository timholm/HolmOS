package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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
	ColorYellow   = "#f9e2af"
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
		"service":   "event-persist",
		"message":   msg,
	}
	for k, v := range fields {
		entry[k] = v
	}
	json.NewEncoder(os.Stdout).Encode(entry)
}

var (
	logger           = &Logger{}
	db               *sql.DB
	nc               *nats.Conn
	requestCounter   uint64
	eventsProcessed  uint64
	eventsFailed     uint64
	healthy          = true
)

type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Data      map[string]interface{} `json:"data"`
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

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Create events table if not exists
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS events (
			id VARCHAR(255) PRIMARY KEY,
			type VARCHAR(255) NOT NULL,
			source VARCHAR(255) NOT NULL,
			data JSONB,
			timestamp TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
	`)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
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

	// Subscribe to all events
	_, err = nc.Subscribe("events.>", func(msg *nats.Msg) {
		var event Event
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			logger.Log("error", "Failed to unmarshal event", map[string]interface{}{"error": err.Error()})
			atomic.AddUint64(&eventsFailed, 1)
			return
		}

		if err := persistEvent(&event); err != nil {
			logger.Log("error", "Failed to persist event", map[string]interface{}{"error": err.Error(), "event_id": event.ID})
			atomic.AddUint64(&eventsFailed, 1)
			// Publish to DLQ
			if nc != nil {
				nc.Publish("events.dlq", msg.Data)
			}
			return
		}

		atomic.AddUint64(&eventsProcessed, 1)
		logger.Log("info", "Event persisted", map[string]interface{}{"event_id": event.ID, "type": event.Type})
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	logger.Log("info", "NATS connected and subscribed", map[string]interface{}{"url": natsURL})
	return nil
}

func persistEvent(event *Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dataJSON, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO events (id, type, source, data, timestamp) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE SET data = $4, timestamp = $5`,
		event.ID, event.Type, event.Source, dataJSON, event.Timestamp)

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
		"service":   "event-persist",
		"status":    "healthy",
		"database":  dbHealthy,
		"nats":      natsHealthy,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if !dbHealthy || !natsHealthy {
		status["status"] = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
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

	fmt.Fprintf(w, "# HELP event_persist_up Whether the service is up\n")
	fmt.Fprintf(w, "# TYPE event_persist_up gauge\n")
	fmt.Fprintf(w, "event_persist_up 1\n")

	fmt.Fprintf(w, "# HELP event_persist_database_up Whether database is connected\n")
	fmt.Fprintf(w, "# TYPE event_persist_database_up gauge\n")
	fmt.Fprintf(w, "event_persist_database_up %d\n", dbUp)

	fmt.Fprintf(w, "# HELP event_persist_nats_up Whether NATS is connected\n")
	fmt.Fprintf(w, "# TYPE event_persist_nats_up gauge\n")
	fmt.Fprintf(w, "event_persist_nats_up %d\n", natsUp)

	fmt.Fprintf(w, "# HELP event_persist_events_processed_total Total events processed\n")
	fmt.Fprintf(w, "# TYPE event_persist_events_processed_total counter\n")
	fmt.Fprintf(w, "event_persist_events_processed_total %d\n", atomic.LoadUint64(&eventsProcessed))

	fmt.Fprintf(w, "# HELP event_persist_events_failed_total Total events failed\n")
	fmt.Fprintf(w, "# TYPE event_persist_events_failed_total counter\n")
	fmt.Fprintf(w, "event_persist_events_failed_total %d\n", atomic.LoadUint64(&eventsFailed))

	fmt.Fprintf(w, "# HELP event_persist_requests_total Total HTTP requests\n")
	fmt.Fprintf(w, "# TYPE event_persist_requests_total counter\n")
	fmt.Fprintf(w, "event_persist_requests_total %d\n", atomic.LoadUint64(&requestCounter))
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		http.Error(w, "Database not connected", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse query parameters
	eventType := r.URL.Query().Get("type")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}

	var rows *sql.Rows
	var err error

	if eventType != "" {
		rows, err = db.QueryContext(ctx,
			`SELECT id, type, source, data, timestamp, created_at FROM events WHERE type = $1 ORDER BY timestamp DESC LIMIT $2`,
			eventType, limit)
	} else {
		rows, err = db.QueryContext(ctx,
			`SELECT id, type, source, data, timestamp, created_at FROM events ORDER BY timestamp DESC LIMIT $1`,
			limit)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var id, eventType, source string
		var data []byte
		var timestamp, createdAt time.Time

		if err := rows.Scan(&id, &eventType, &source, &data, &timestamp, &createdAt); err != nil {
			continue
		}

		var dataMap map[string]interface{}
		json.Unmarshal(data, &dataMap)

		events = append(events, map[string]interface{}{
			"id":         id,
			"type":       eventType,
			"source":     source,
			"data":       dataMap,
			"timestamp":  timestamp,
			"created_at": createdAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func eventHandler(w http.ResponseWriter, r *http.Request) {
	if db == nil {
		http.Error(w, "Database not connected", http.StatusServiceUnavailable)
		return
	}

	// Get event ID from query parameter
	eventID := r.URL.Query().Get("id")
	if eventID == "" {
		http.Error(w, "Missing event id parameter", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var id, eventType, source string
	var data []byte
	var timestamp, createdAt time.Time

	err := db.QueryRowContext(ctx,
		`SELECT id, type, source, data, timestamp, created_at FROM events WHERE id = $1`,
		eventID).Scan(&id, &eventType, &source, &data, &timestamp, &createdAt)

	if err == sql.ErrNoRows {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var dataMap map[string]interface{}
	json.Unmarshal(data, &dataMap)

	event := map[string]interface{}{
		"id":         id,
		"type":       eventType,
		"source":     source,
		"data":       dataMap,
		"timestamp":  timestamp,
		"created_at": createdAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
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

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Event Persist</title>
    <style>
        body { background: %s; color: %s; font-family: 'JetBrains Mono', monospace; padding: 2rem; }
        .container { max-width: 800px; margin: 0 auto; }
        h1 { color: %s; }
        .status { padding: 1rem; background: %s; border-radius: 8px; margin: 1rem 0; display: flex; justify-content: space-between; }
        .status-indicator { display: inline-block; width: 12px; height: 12px; border-radius: 50%%; margin-right: 8px; }
        .metric { display: flex; justify-content: space-between; padding: 0.5rem 0; border-bottom: 1px solid %s; }
        .metric-value { color: %s; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Event Persist</h1>
        <div class="status">
            <span><span class="status-indicator" style="background: %s"></span>Database: %s</span>
            <span><span class="status-indicator" style="background: %s"></span>NATS: %s</span>
        </div>
        <div class="metrics">
            <div class="metric"><span>Events Processed</span><span class="metric-value">%d</span></div>
            <div class="metric"><span>Events Failed</span><span class="metric-value">%d</span></div>
        </div>
    </div>
</body>
</html>`, ColorBase, ColorText, ColorYellow, ColorMantle, ColorCrust, ColorBlue,
		dbColor, dbStatus, natsColor, natsStatus,
		atomic.LoadUint64(&eventsProcessed), atomic.LoadUint64(&eventsFailed))

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	logger.Log("info", "Starting event-persist service", nil)

	if err := initDB(); err != nil {
		logger.Log("warn", "Database initialization failed, will retry", map[string]interface{}{"error": err.Error()})
	}

	if err := initNATS(); err != nil {
		logger.Log("warn", "NATS initialization failed, will retry", map[string]interface{}{"error": err.Error()})
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)
	http.HandleFunc("/events", eventsHandler)
	http.HandleFunc("/event", eventHandler)
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
