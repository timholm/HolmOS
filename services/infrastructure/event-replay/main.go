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
	ColorPeach    = "#fab387"
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
		"service":   "event-replay",
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
	eventsReplayed  uint64
	replaysFailed   uint64
)

type FailedEvent struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	Data      json.RawMessage `json:"data"`
	Error     string          `json:"error"`
	Retries   int             `json:"retries"`
	CreatedAt time.Time       `json:"created_at"`
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

	// Create failed_events table
	_, err = db.ExecContext(ctx, `
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
		CREATE INDEX IF NOT EXISTS idx_failed_events_retries ON failed_events(retries);
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

	logger.Log("info", "NATS connected", map[string]interface{}{"url": natsURL})
	return nil
}

func replayEvents() {
	if db == nil || nc == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT id, type, source, data FROM failed_events
		WHERE retries < 5 AND (last_retry IS NULL OR last_retry < NOW() - INTERVAL '5 minutes')
		ORDER BY created_at ASC LIMIT 100
	`)
	if err != nil {
		logger.Log("error", "Failed to query failed events", map[string]interface{}{"error": err.Error()})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var event FailedEvent
		if err := rows.Scan(&event.ID, &event.Type, &event.Source, &event.Data); err != nil {
			continue
		}

		eventData, _ := json.Marshal(map[string]interface{}{
			"id":        event.ID,
			"type":      event.Type,
			"source":    event.Source,
			"data":      event.Data,
			"timestamp": time.Now().UTC(),
		})

		subject := fmt.Sprintf("events.%s", event.Type)
		if err := nc.Publish(subject, eventData); err != nil {
			atomic.AddUint64(&replaysFailed, 1)
			logger.Log("error", "Failed to replay event", map[string]interface{}{"error": err.Error(), "event_id": event.ID})

			db.ExecContext(ctx, `UPDATE failed_events SET retries = retries + 1, last_retry = NOW() WHERE id = $1`, event.ID)
		} else {
			atomic.AddUint64(&eventsReplayed, 1)
			logger.Log("info", "Event replayed", map[string]interface{}{"event_id": event.ID, "type": event.Type})

			db.ExecContext(ctx, `DELETE FROM failed_events WHERE id = $1`, event.ID)
		}
	}
}

func startReplayWorker() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			replayEvents()
		}
	}()
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
		"service":   "event-replay",
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

	pendingCount := 0
	if db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM failed_events WHERE retries < 5`).Scan(&pendingCount)
	}

	fmt.Fprintf(w, "# HELP event_replay_up Whether the service is up\n")
	fmt.Fprintf(w, "# TYPE event_replay_up gauge\n")
	fmt.Fprintf(w, "event_replay_up 1\n")

	fmt.Fprintf(w, "# HELP event_replay_database_up Whether database is connected\n")
	fmt.Fprintf(w, "# TYPE event_replay_database_up gauge\n")
	fmt.Fprintf(w, "event_replay_database_up %d\n", dbUp)

	fmt.Fprintf(w, "# HELP event_replay_nats_up Whether NATS is connected\n")
	fmt.Fprintf(w, "# TYPE event_replay_nats_up gauge\n")
	fmt.Fprintf(w, "event_replay_nats_up %d\n", natsUp)

	fmt.Fprintf(w, "# HELP event_replay_events_replayed_total Total events replayed\n")
	fmt.Fprintf(w, "# TYPE event_replay_events_replayed_total counter\n")
	fmt.Fprintf(w, "event_replay_events_replayed_total %d\n", atomic.LoadUint64(&eventsReplayed))

	fmt.Fprintf(w, "# HELP event_replay_replays_failed_total Total replay failures\n")
	fmt.Fprintf(w, "# TYPE event_replay_replays_failed_total counter\n")
	fmt.Fprintf(w, "event_replay_replays_failed_total %d\n", atomic.LoadUint64(&replaysFailed))

	fmt.Fprintf(w, "# HELP event_replay_pending_events Number of pending events\n")
	fmt.Fprintf(w, "# TYPE event_replay_pending_events gauge\n")
	fmt.Fprintf(w, "event_replay_pending_events %d\n", pendingCount)

	fmt.Fprintf(w, "# HELP event_replay_requests_total Total HTTP requests\n")
	fmt.Fprintf(w, "# TYPE event_replay_requests_total counter\n")
	fmt.Fprintf(w, "event_replay_requests_total %d\n", atomic.LoadUint64(&requestCounter))
}

func replayHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	go replayEvents()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "replay triggered"})
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

	pendingCount := 0
	if db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM failed_events WHERE retries < 5`).Scan(&pendingCount)
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Event Replay</title>
    <style>
        body { background: %s; color: %s; font-family: 'JetBrains Mono', monospace; padding: 2rem; }
        .container { max-width: 800px; margin: 0 auto; }
        h1 { color: %s; }
        .status { padding: 1rem; background: %s; border-radius: 8px; margin: 1rem 0; display: flex; justify-content: space-between; }
        .status-indicator { display: inline-block; width: 12px; height: 12px; border-radius: 50%%; margin-right: 8px; }
        .metric { display: flex; justify-content: space-between; padding: 0.5rem 0; border-bottom: 1px solid %s; }
        .metric-value { color: %s; }
        .btn { background: %s; color: %s; border: none; padding: 0.75rem 1.5rem; border-radius: 4px; cursor: pointer; margin-top: 1rem; }
        .btn:hover { opacity: 0.8; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Event Replay</h1>
        <div class="status">
            <span><span class="status-indicator" style="background: %s"></span>Database: %s</span>
            <span><span class="status-indicator" style="background: %s"></span>NATS: %s</span>
        </div>
        <div class="metrics">
            <div class="metric"><span>Events Replayed</span><span class="metric-value">%d</span></div>
            <div class="metric"><span>Replays Failed</span><span class="metric-value">%d</span></div>
            <div class="metric"><span>Pending Events</span><span class="metric-value">%d</span></div>
        </div>
        <form action="/replay" method="POST">
            <button type="submit" class="btn">Trigger Replay</button>
        </form>
    </div>
</body>
</html>`, ColorBase, ColorText, ColorPeach, ColorMantle, ColorCrust, ColorBlue,
		ColorMauve, ColorBase, dbColor, dbStatus, natsColor, natsStatus,
		atomic.LoadUint64(&eventsReplayed), atomic.LoadUint64(&replaysFailed), pendingCount)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	logger.Log("info", "Starting event-replay service", nil)

	if err := initDB(); err != nil {
		logger.Log("warn", "Database initialization failed, will retry", map[string]interface{}{"error": err.Error()})
	}

	if err := initNATS(); err != nil {
		logger.Log("warn", "NATS initialization failed, will retry", map[string]interface{}{"error": err.Error()})
	}

	startReplayWorker()

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)
	http.HandleFunc("/replay", replayHandler)
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
