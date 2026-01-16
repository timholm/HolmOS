package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats-server/v2/server"
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
		"service":   "event-broker",
		"message":   msg,
	}
	for k, v := range fields {
		entry[k] = v
	}
	json.NewEncoder(os.Stdout).Encode(entry)
}

var (
	logger         = &Logger{}
	natsServer     *server.Server
	requestCounter uint64
	healthy        = true
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCounter, 1)

	status := map[string]interface{}{
		"service": "event-broker",
		"status":  "healthy",
		"nats": map[string]interface{}{
			"running":     natsServer != nil && natsServer.Running(),
			"connections": 0,
			"subscriptions": 0,
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if natsServer != nil && natsServer.Running() {
		status["nats"].(map[string]interface{})["connections"] = natsServer.NumClients()
		status["nats"].(map[string]interface{})["subscriptions"] = natsServer.NumSubscriptions()
	} else {
		status["status"] = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	atomic.AddUint64(&requestCounter, 1)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	running := 0
	clients := 0
	subs := 0
	if natsServer != nil && natsServer.Running() {
		running = 1
		clients = natsServer.NumClients()
		subs = natsServer.NumSubscriptions()
	}

	fmt.Fprintf(w, "# HELP event_broker_up Whether the event broker is up\n")
	fmt.Fprintf(w, "# TYPE event_broker_up gauge\n")
	fmt.Fprintf(w, "event_broker_up %d\n", running)

	fmt.Fprintf(w, "# HELP event_broker_nats_connections Number of NATS connections\n")
	fmt.Fprintf(w, "# TYPE event_broker_nats_connections gauge\n")
	fmt.Fprintf(w, "event_broker_nats_connections %d\n", clients)

	fmt.Fprintf(w, "# HELP event_broker_nats_subscriptions Number of NATS subscriptions\n")
	fmt.Fprintf(w, "# TYPE event_broker_nats_subscriptions gauge\n")
	fmt.Fprintf(w, "event_broker_nats_subscriptions %d\n", subs)

	fmt.Fprintf(w, "# HELP event_broker_requests_total Total HTTP requests\n")
	fmt.Fprintf(w, "# TYPE event_broker_requests_total counter\n")
	fmt.Fprintf(w, "event_broker_requests_total %d\n", atomic.LoadUint64(&requestCounter))
}

func uiHandler(w http.ResponseWriter, r *http.Request) {
	running := natsServer != nil && natsServer.Running()
	statusColor := ColorGreen
	statusText := "Running"
	if !running {
		statusColor = ColorRed
		statusText = "Stopped"
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Event Broker - NATS</title>
    <style>
        body { background: %s; color: %s; font-family: 'JetBrains Mono', monospace; padding: 2rem; }
        .container { max-width: 800px; margin: 0 auto; }
        h1 { color: %s; }
        .status { padding: 1rem; background: %s; border-radius: 8px; margin: 1rem 0; }
        .status-indicator { display: inline-block; width: 12px; height: 12px; border-radius: 50%%; background: %s; margin-right: 8px; }
        .metric { display: flex; justify-content: space-between; padding: 0.5rem 0; border-bottom: 1px solid %s; }
        .metric-value { color: %s; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Event Broker</h1>
        <div class="status">
            <span class="status-indicator"></span>
            NATS Server: %s
        </div>
        <div class="metrics">
            <div class="metric"><span>Connections</span><span class="metric-value">%d</span></div>
            <div class="metric"><span>Subscriptions</span><span class="metric-value">%d</span></div>
        </div>
    </div>
</body>
</html>`, ColorBase, ColorText, ColorMauve, ColorMantle, statusColor, ColorCrust, ColorBlue, statusText,
		func() int { if natsServer != nil { return natsServer.NumClients() }; return 0 }(),
		func() int { if natsServer != nil { return natsServer.NumSubscriptions() }; return 0 }())

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func startNATSServer() error {
	opts := &server.Options{
		Host:           "0.0.0.0",
		Port:           4222,
		NoLog:          false,
		NoSigs:         true,
		MaxControlLine: 4096,
	}

	var err error
	natsServer, err = server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("failed to create NATS server: %w", err)
	}

	go natsServer.Start()

	if !natsServer.ReadyForConnections(10 * time.Second) {
		return fmt.Errorf("NATS server not ready for connections")
	}

	logger.Log("info", "NATS server started", map[string]interface{}{
		"host": opts.Host,
		"port": opts.Port,
	})

	return nil
}

func main() {
	logger.Log("info", "Starting event-broker service", nil)

	if err := startNATSServer(); err != nil {
		logger.Log("error", "Failed to start NATS server", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/metrics", metricsHandler)
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
