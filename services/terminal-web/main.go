package main

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/ssh"
)

//go:embed index.html
var content embed.FS

var db *sql.DB
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Session management
type Session struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // ssh, kubectl, local, pod
	HostName    string    `json:"host_name"`
	HostID      int       `json:"host_id,omitempty"`
	PodName     string    `json:"pod_name,omitempty"`
	Namespace   string    `json:"namespace,omitempty"`
	Container   string    `json:"container,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	LastActive  time.Time `json:"last_active"`
	IsPrivileged bool     `json:"is_privileged"`
}

type CommandHistory struct {
	SessionID string    `json:"session_id"`
	Command   string    `json:"command"`
	Timestamp time.Time `json:"timestamp"`
}

var (
	sessions      = make(map[string]*Session)
	sessionsMutex sync.RWMutex
	commandHistory = make(map[string][]CommandHistory)
	historyMutex   sync.RWMutex
	sessionCounter int
	counterMutex   sync.Mutex
)

func generateSessionID() string {
	counterMutex.Lock()
	defer counterMutex.Unlock()
	sessionCounter++
	return fmt.Sprintf("session-%d-%d", time.Now().Unix(), sessionCounter)
}

func registerSession(s *Session) {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()
	sessions[s.ID] = s
	log.Printf("Session registered: %s (%s)", s.ID, s.Type)
}

func unregisterSession(id string) {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()
	if s, ok := sessions[id]; ok {
		log.Printf("Session unregistered: %s (%s)", s.ID, s.Type)
		delete(sessions, id)
	}
}

func updateSessionActivity(id string) {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()
	if s, ok := sessions[id]; ok {
		s.LastActive = time.Now()
	}
}

func addCommandToHistory(sessionID, command string) {
	historyMutex.Lock()
	defer historyMutex.Unlock()

	entry := CommandHistory{
		SessionID: sessionID,
		Command:   command,
		Timestamp: time.Now(),
	}

	if _, ok := commandHistory[sessionID]; !ok {
		commandHistory[sessionID] = []CommandHistory{}
	}

	// Keep last 1000 commands per session
	if len(commandHistory[sessionID]) >= 1000 {
		commandHistory[sessionID] = commandHistory[sessionID][1:]
	}
	commandHistory[sessionID] = append(commandHistory[sessionID], entry)
}

type Host struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	SSHKey   string `json:"ssh_key,omitempty"`
	AuthType string `json:"auth_type"`
	Color    string `json:"color"`
	Type     string `json:"type"` // ssh, kubectl
}

type Theme struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Background  string `json:"background"`
	Foreground  string `json:"foreground"`
	Cursor      string `json:"cursor"`
	Selection   string `json:"selection"`
	Black       string `json:"black"`
	Red         string `json:"red"`
	Green       string `json:"green"`
	Yellow      string `json:"yellow"`
	Blue        string `json:"blue"`
	Magenta     string `json:"magenta"`
	Cyan        string `json:"cyan"`
	White       string `json:"white"`
	BrightBlack string `json:"brightBlack"`
	BrightRed   string `json:"brightRed"`
	BrightGreen string `json:"brightGreen"`
	BrightYellow string `json:"brightYellow"`
	BrightBlue   string `json:"brightBlue"`
	BrightMagenta string `json:"brightMagenta"`
	BrightCyan    string `json:"brightCyan"`
	BrightWhite   string `json:"brightWhite"`
}

// Catppuccin themes
var themes = []Theme{
	{
		ID: "catppuccin-mocha", Name: "Catppuccin Mocha",
		Background: "#1e1e2e", Foreground: "#cdd6f4", Cursor: "#f5e0dc", Selection: "rgba(88, 91, 112, 0.5)",
		Black: "#45475a", Red: "#f38ba8", Green: "#a6e3a1", Yellow: "#f9e2af",
		Blue: "#89b4fa", Magenta: "#f5c2e7", Cyan: "#94e2d5", White: "#bac2de",
		BrightBlack: "#585b70", BrightRed: "#f38ba8", BrightGreen: "#a6e3a1", BrightYellow: "#f9e2af",
		BrightBlue: "#89b4fa", BrightMagenta: "#f5c2e7", BrightCyan: "#94e2d5", BrightWhite: "#a6adc8",
	},
	{
		ID: "catppuccin-macchiato", Name: "Catppuccin Macchiato",
		Background: "#24273a", Foreground: "#cad3f5", Cursor: "#f4dbd6", Selection: "rgba(91, 96, 120, 0.5)",
		Black: "#494d64", Red: "#ed8796", Green: "#a6da95", Yellow: "#eed49f",
		Blue: "#8aadf4", Magenta: "#f5bde6", Cyan: "#8bd5ca", White: "#b8c0e0",
		BrightBlack: "#5b6078", BrightRed: "#ed8796", BrightGreen: "#a6da95", BrightYellow: "#eed49f",
		BrightBlue: "#8aadf4", BrightMagenta: "#f5bde6", BrightCyan: "#8bd5ca", BrightWhite: "#a5adcb",
	},
	{
		ID: "catppuccin-frappe", Name: "Catppuccin Frappe",
		Background: "#303446", Foreground: "#c6d0f5", Cursor: "#f2d5cf", Selection: "rgba(98, 104, 128, 0.5)",
		Black: "#51576d", Red: "#e78284", Green: "#a6d189", Yellow: "#e5c890",
		Blue: "#8caaee", Magenta: "#f4b8e4", Cyan: "#81c8be", White: "#b5bfe2",
		BrightBlack: "#626880", BrightRed: "#e78284", BrightGreen: "#a6d189", BrightYellow: "#e5c890",
		BrightBlue: "#8caaee", BrightMagenta: "#f4b8e4", BrightCyan: "#81c8be", BrightWhite: "#a5adce",
	},
	{
		ID: "catppuccin-latte", Name: "Catppuccin Latte",
		Background: "#eff1f5", Foreground: "#4c4f69", Cursor: "#dc8a78", Selection: "rgba(172, 176, 190, 0.5)",
		Black: "#5c5f77", Red: "#d20f39", Green: "#40a02b", Yellow: "#df8e1d",
		Blue: "#1e66f5", Magenta: "#ea76cb", Cyan: "#179299", White: "#acb0be",
		BrightBlack: "#6c6f85", BrightRed: "#d20f39", BrightGreen: "#40a02b", BrightYellow: "#df8e1d",
		BrightBlue: "#1e66f5", BrightMagenta: "#ea76cb", BrightCyan: "#179299", BrightWhite: "#bcc0cc",
	},
	{
		ID: "holmos-dark", Name: "HolmOS Dark",
		Background: "#1a1a2e", Foreground: "#eee", Cursor: "#4ecdc4", Selection: "rgba(78, 205, 196, 0.3)",
		Black: "#16213e", Red: "#ff6b6b", Green: "#4ecdc4", Yellow: "#ffeaa7",
		Blue: "#74b9ff", Magenta: "#a29bfe", Cyan: "#45b7d1", White: "#dfe6e9",
		BrightBlack: "#636e72", BrightRed: "#ff7675", BrightGreen: "#55efc4", BrightYellow: "#fdcb6e",
		BrightBlue: "#a29bfe", BrightMagenta: "#fd79a8", BrightCyan: "#81ecec", BrightWhite: "#ffffff",
	},
}

func initDB() {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "host=postgres.holm.svc.cluster.local port=5432 user=holm password=holm-secret-123 dbname=holm sslmode=disable"
	}
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Warning: could not connect to database: %v", err)
		return
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// Create table if not exists
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS terminal_hosts (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		hostname VARCHAR(255) NOT NULL,
		port INTEGER DEFAULT 22,
		username VARCHAR(255) NOT NULL,
		password VARCHAR(255),
		ssh_key TEXT,
		auth_type VARCHAR(50) DEFAULT 'password',
		color VARCHAR(50),
		type VARCHAR(50) DEFAULT 'ssh',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Printf("Warning: could not create table: %v", err)
	}

	// Add type column if not exists
	db.Exec(`ALTER TABLE terminal_hosts ADD COLUMN IF NOT EXISTS type VARCHAR(50) DEFAULT 'ssh'`)
}

func main() {
	initDB()

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/hosts", handleHosts)
	http.HandleFunc("/api/hosts/add", handleAddHost)
	http.HandleFunc("/api/hosts/delete", handleDeleteHost)
	http.HandleFunc("/api/hosts/init", handleInitHosts)
	http.HandleFunc("/api/themes", handleThemes)
	http.HandleFunc("/api/exec", handleExec)
	http.HandleFunc("/api/sessions", handleSessions)
	http.HandleFunc("/api/sessions/history", handleSessionHistory)
	http.HandleFunc("/api/pods", handlePods)
	http.HandleFunc("/api/namespaces", handleNamespaces)
	http.HandleFunc("/ws/terminal", handleTerminal)
	http.HandleFunc("/ws/kubectl", handleKubectl)
	http.HandleFunc("/ws/local", handleLocalShell)
	http.HandleFunc("/ws/pod", handlePodExec)
	http.HandleFunc("/health", handleHealth)

	log.Println("Terminal Web listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, _ := content.ReadFile("index.html")
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ok"))
}

// handleSessions returns all active terminal sessions
func handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	// Handle DELETE to terminate a session
	if r.Method == "DELETE" {
		sessionID := r.URL.Query().Get("id")
		if sessionID == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "session id required",
			})
			return
		}
		unregisterSession(sessionID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "session terminated",
		})
		return
	}

	sessionsMutex.RLock()
	defer sessionsMutex.RUnlock()

	sessionList := make([]*Session, 0, len(sessions))
	for _, s := range sessions {
		sessionList = append(sessionList, s)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"sessions": sessionList,
		"count":    len(sessionList),
	})
}

// handleSessionHistory returns command history for a session
func handleSessionHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	sessionID := r.URL.Query().Get("session_id")

	historyMutex.RLock()
	defer historyMutex.RUnlock()

	if sessionID != "" {
		// Return history for specific session
		history, ok := commandHistory[sessionID]
		if !ok {
			history = []CommandHistory{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"history": history,
		})
		return
	}

	// Return all history
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"history": commandHistory,
	})
}

// handleNamespaces lists all Kubernetes namespaces
func handleNamespaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")
	cmd.Env = getKubeEnv()

	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	namespaces := strings.Fields(string(output))
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"namespaces": namespaces,
	})
}

// handlePods lists pods, optionally filtered by namespace
func handlePods(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get pods with their containers
	cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", namespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name}|{.status.phase}|{range .spec.containers[*]}{.name},{end};{end}")
	cmd.Env = getKubeEnv()

	output, err := cmd.Output()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	type PodInfo struct {
		Name       string   `json:"name"`
		Namespace  string   `json:"namespace"`
		Status     string   `json:"status"`
		Containers []string `json:"containers"`
	}

	var pods []PodInfo
	podEntries := strings.Split(string(output), ";")
	for _, entry := range podEntries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		if len(parts) >= 3 {
			containers := strings.Split(strings.TrimSuffix(parts[2], ","), ",")
			// Filter out empty strings
			var cleanContainers []string
			for _, c := range containers {
				if c != "" {
					cleanContainers = append(cleanContainers, c)
				}
			}
			pods = append(pods, PodInfo{
				Name:       parts[0],
				Namespace:  namespace,
				Status:     parts[1],
				Containers: cleanContainers,
			})
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"pods":    pods,
	})
}

// getKubeEnv returns environment variables needed for kubectl
func getKubeEnv() []string {
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin:/sbin",
		"HOME=/root",
		"TERM=xterm-256color",
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "KUBERNETES_") {
			env = append(env, e)
		}
	}
	return env
}

// handlePodExec opens an interactive shell to a pod
func handlePodExec(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	podName := r.URL.Query().Get("pod")
	container := r.URL.Query().Get("container")

	if namespace == "" {
		namespace = "default"
	}
	if podName == "" {
		http.Error(w, "pod parameter required", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Create session
	sessionID := generateSessionID()
	session := &Session{
		ID:         sessionID,
		Type:       "pod",
		HostName:   fmt.Sprintf("%s/%s", namespace, podName),
		PodName:    podName,
		Namespace:  namespace,
		Container:  container,
		StartedAt:  time.Now(),
		LastActive: time.Now(),
	}
	registerSession(session)
	defer unregisterSession(sessionID)

	log.Printf("Pod exec connection: %s/%s (container: %s)", namespace, podName, container)

	// Build kubectl exec command
	args := []string{"exec", "-it", "-n", namespace, podName}
	if container != "" {
		args = append(args, "-c", container)
	}
	args = append(args, "--", "/bin/sh", "-c", "exec /bin/bash 2>/dev/null || exec /bin/sh")

	cmd := exec.Command("kubectl", args...)
	cmd.Env = getKubeEnv()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to exec into pod: "+err.Error()))
		return
	}
	defer ptmx.Close()

	done := make(chan struct{})
	var inputBuffer bytes.Buffer

	// Read from PTY -> WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
				n, err := ptmx.Read(buf)
				if err != nil {
					return
				}
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
		}
	}()

	// Read from WebSocket -> PTY
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			close(done)
			cmd.Process.Kill()
			return
		}

		updateSessionActivity(sessionID)

		// Handle resize messages
		if len(msg) > 0 && msg[0] == 1 {
			if len(msg) >= 5 {
				cols := int(msg[1])<<8 | int(msg[2])
				rows := int(msg[3])<<8 | int(msg[4])
				pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
			}
			continue
		}

		// Track command history (on Enter key)
		for _, b := range msg {
			if b == '\r' || b == '\n' {
				cmd := strings.TrimSpace(inputBuffer.String())
				if cmd != "" {
					addCommandToHistory(sessionID, cmd)
				}
				inputBuffer.Reset()
			} else if b == 127 || b == 8 { // Backspace
				if inputBuffer.Len() > 0 {
					inputBuffer.Truncate(inputBuffer.Len() - 1)
				}
			} else if b >= 32 && b < 127 { // Printable characters
				inputBuffer.WriteByte(b)
			}
		}

		ptmx.Write(msg)
	}
}

// handleExec executes a command with a 10-second timeout and returns the output
func handleExec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
		return
	}

	var req struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"` // optional, defaults to 10 seconds
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid JSON: " + err.Error(),
		})
		return
	}

	if req.Command == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Command is required",
		})
		return
	}

	// Default timeout of 10 seconds, max 60 seconds
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 10
	}
	if timeout > 60 {
		timeout = 60
	}

	log.Printf("Executing command: %s (timeout: %ds)", req.Command, timeout)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Execute command with timeout using context
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", req.Command)
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin:/sbin",
		"HOME=/root",
		"TERM=xterm-256color",
	}
	// Preserve KUBERNETES_* env vars for in-cluster auth
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "KUBERNETES_") {
			cmd.Env = append(cmd.Env, e)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Check if it was a timeout
	if ctx.Err() == context.DeadlineExceeded {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Command timed out after %d seconds", timeout),
			"stdout":  stdout.String(),
			"stderr":  stderr.String(),
		})
		return
	}

	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
			"stdout":  stdout.String(),
			"stderr":  stderr.String(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"stdout":  stdout.String(),
		"stderr":  stderr.String(),
	})
}

func handleThemes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(themes)
}

func handleHosts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Always include built-in kubectl entry
	builtinHosts := []Host{
		{ID: -1, Name: "kubectl", Hostname: "cluster", Port: 0, Username: "", Type: "kubectl", Color: "#89b4fa"},
		{ID: -2, Name: "local shell", Hostname: "localhost", Port: 0, Username: "", Type: "local", Color: "#a6e3a1"},
	}

	if db == nil {
		json.NewEncoder(w).Encode(builtinHosts)
		return
	}

	rows, err := db.Query("SELECT id, name, hostname, port, username, password, COALESCE(ssh_key, ''), COALESCE(auth_type, 'password'), COALESCE(color, ''), COALESCE(type, 'ssh') FROM terminal_hosts ORDER BY name")
	if err != nil {
		log.Printf("Query error: %v", err)
		json.NewEncoder(w).Encode(builtinHosts)
		return
	}
	defer rows.Close()

	hosts := builtinHosts
	for rows.Next() {
		var h Host
		err := rows.Scan(&h.ID, &h.Name, &h.Hostname, &h.Port, &h.Username, &h.Password, &h.SSHKey, &h.AuthType, &h.Color, &h.Type)
		if err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		h.Password = "***" // Don't expose password
		if h.Type == "" {
			h.Type = "ssh"
		}
		hosts = append(hosts, h)
	}

	json.NewEncoder(w).Encode(hosts)
}

func handleAddHost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		return
	}

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "Method not allowed"})
		return
	}

	var h Host
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "Invalid JSON"})
		return
	}

	if h.Name == "" || h.Hostname == "" || h.Username == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "Name, hostname, and username are required"})
		return
	}

	if h.Port == 0 {
		h.Port = 22
	}
	if h.AuthType == "" {
		h.AuthType = "password"
	}
	if h.Type == "" {
		h.Type = "ssh"
	}

	var id int
	err := db.QueryRow(`INSERT INTO terminal_hosts (name, hostname, port, username, password, ssh_key, auth_type, color, type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		h.Name, h.Hostname, h.Port, h.Username, h.Password, h.SSHKey, h.AuthType, h.Color, h.Type).Scan(&id)

	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "Database error: " + err.Error()})
		return
	}

	log.Printf("Added host: %s (%s@%s:%d)", h.Name, h.Username, h.Hostname, h.Port)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "id": id, "message": "Host added successfully"})
}

func handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "Invalid ID"})
		return
	}

	_, err = db.Exec("DELETE FROM terminal_hosts WHERE id = $1", id)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "Database error"})
		return
	}

	log.Printf("Deleted host: %d", id)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "Host deleted"})
}

func handleInitHosts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Pre-configure all 13 Pi nodes
	hosts := []Host{
		{Name: "rpi1", Hostname: "192.168.8.197", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#f38ba8", Type: "ssh"},
		{Name: "rpi2", Hostname: "192.168.8.198", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#fab387", Type: "ssh"},
		{Name: "rpi3", Hostname: "192.168.8.199", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#f9e2af", Type: "ssh"},
		{Name: "rpi4", Hostname: "192.168.8.200", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#a6e3a1", Type: "ssh"},
		{Name: "rpi5", Hostname: "192.168.8.201", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#94e2d5", Type: "ssh"},
		{Name: "rpi6", Hostname: "192.168.8.202", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#89dceb", Type: "ssh"},
		{Name: "rpi7", Hostname: "192.168.8.203", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#74c7ec", Type: "ssh"},
		{Name: "rpi8", Hostname: "192.168.8.204", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#89b4fa", Type: "ssh"},
		{Name: "rpi9", Hostname: "192.168.8.205", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#cba6f7", Type: "ssh"},
		{Name: "rpi10", Hostname: "192.168.8.206", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#f5c2e7", Type: "ssh"},
		{Name: "rpi11", Hostname: "192.168.8.207", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#eba0ac", Type: "ssh"},
		{Name: "rpi12", Hostname: "192.168.8.208", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#b4befe", Type: "ssh"},
		{Name: "rpi13", Hostname: "192.168.8.209", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#cdd6f4", Type: "ssh"},
	}

	added := 0
	for _, h := range hosts {
		// Check if exists
		var count int
		db.QueryRow("SELECT COUNT(*) FROM terminal_hosts WHERE name = $1", h.Name).Scan(&count)
		if count > 0 {
			continue
		}

		_, err := db.Exec(`INSERT INTO terminal_hosts (name, hostname, port, username, password, auth_type, color, type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			h.Name, h.Hostname, h.Port, h.Username, h.Password, h.AuthType, h.Color, h.Type)
		if err == nil {
			added++
		}
	}

	log.Printf("Initialized %d Pi hosts", added)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "added": added, "message": fmt.Sprintf("Initialized %d Pi hosts", added)})
}

func handleTerminal(w http.ResponseWriter, r *http.Request) {
	hostID := r.URL.Query().Get("host")
	if hostID == "" {
		http.Error(w, "host parameter required", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(hostID)
	if err != nil {
		http.Error(w, "invalid host id", http.StatusBadRequest)
		return
	}

	// Get host from database
	var h Host
	err = db.QueryRow("SELECT id, name, hostname, port, username, password, COALESCE(ssh_key, ''), COALESCE(auth_type, 'password') FROM terminal_hosts WHERE id = $1", id).
		Scan(&h.ID, &h.Name, &h.Hostname, &h.Port, &h.Username, &h.Password, &h.SSHKey, &h.AuthType)
	if err != nil {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Create session
	sessionID := generateSessionID()
	termSession := &Session{
		ID:         sessionID,
		Type:       "ssh",
		HostName:   h.Name,
		HostID:     h.ID,
		StartedAt:  time.Now(),
		LastActive: time.Now(),
	}
	registerSession(termSession)
	defer unregisterSession(sessionID)

	log.Printf("Terminal connection to %s (%s@%s:%d) [session: %s]", h.Name, h.Username, h.Hostname, h.Port, sessionID)

	// Connect to SSH
	config := &ssh.ClientConfig{
		User:            h.Username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	if h.AuthType == "key" && h.SSHKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(h.SSHKey))
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte("Error parsing SSH key: "+err.Error()))
			return
		}
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else {
		config.Auth = []ssh.AuthMethod{ssh.Password(h.Password)}
	}

	addr := fmt.Sprintf("%s:%d", h.Hostname, h.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("SSH connection error: "+err.Error()))
		return
	}
	defer client.Close()

	sshSession, err := client.NewSession()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("SSH session error: "+err.Error()))
		return
	}
	defer sshSession.Close()

	// Set up PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := sshSession.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("PTY error: "+err.Error()))
		return
	}

	stdin, err := sshSession.StdinPipe()
	if err != nil {
		return
	}

	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		return
	}

	stderr, err := sshSession.StderrPipe()
	if err != nil {
		return
	}

	if err := sshSession.Shell(); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Shell error: "+err.Error()))
		return
	}

	done := make(chan struct{})
	var inputBuffer bytes.Buffer

	// Read from SSH stdout -> WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
				n, err := stdout.Read(buf)
				if err != nil {
					return
				}
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
		}
	}()

	// Read from SSH stderr -> WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
				n, err := stderr.Read(buf)
				if err != nil {
					return
				}
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
		}
	}()

	// Read from WebSocket -> SSH stdin
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			close(done)
			return
		}

		updateSessionActivity(sessionID)

		// Handle resize messages
		if len(msg) > 0 && msg[0] == 1 {
			// Resize: [1, cols_high, cols_low, rows_high, rows_low]
			if len(msg) >= 5 {
				cols := int(msg[1])<<8 | int(msg[2])
				rows := int(msg[3])<<8 | int(msg[4])
				sshSession.WindowChange(rows, cols)
			}
			continue
		}

		// Track command history (on Enter key)
		for _, b := range msg {
			if b == '\r' || b == '\n' {
				cmdStr := strings.TrimSpace(inputBuffer.String())
				if cmdStr != "" {
					addCommandToHistory(sessionID, cmdStr)
					// Detect sudo/privilege escalation
					if strings.HasPrefix(cmdStr, "sudo ") || strings.Contains(cmdStr, "| sudo") {
						sessionsMutex.Lock()
						if s, ok := sessions[sessionID]; ok {
							s.IsPrivileged = true
						}
						sessionsMutex.Unlock()
					}
				}
				inputBuffer.Reset()
			} else if b == 127 || b == 8 { // Backspace
				if inputBuffer.Len() > 0 {
					inputBuffer.Truncate(inputBuffer.Len() - 1)
				}
			} else if b >= 32 && b < 127 { // Printable characters
				inputBuffer.WriteByte(b)
			}
		}

		stdin.Write(msg)
	}
}

func handleKubectl(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Create session
	sessionID := generateSessionID()
	session := &Session{
		ID:         sessionID,
		Type:       "kubectl",
		HostName:   "cluster",
		StartedAt:  time.Now(),
		LastActive: time.Now(),
	}
	registerSession(session)
	defer unregisterSession(sessionID)

	log.Printf("kubectl shell connection (session: %s)", sessionID)

	// Start bash shell for kubectl access
	// Use bash for better command-line experience, fall back to sh
	shell := "/bin/bash"
	if _, err := os.Stat(shell); os.IsNotExist(err) {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell)
	// Set up environment for in-cluster kubectl access
	// Remove any KUBECONFIG to force in-cluster config usage
	env := []string{
		"TERM=xterm-256color",
		"HOME=/root",
		"PATH=/usr/local/bin:/usr/bin:/bin:/sbin",
		"PS1=\\[\\033[1;34m\\]kubectl\\[\\033[0m\\]:\\[\\033[1;32m\\]\\w\\[\\033[0m\\]$ ",
	}
	// Preserve KUBERNETES_* env vars for in-cluster auth
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "KUBERNETES_") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to start shell: "+err.Error()))
		return
	}
	defer ptmx.Close()

	done := make(chan struct{})
	var inputBuffer bytes.Buffer

	// Read from PTY -> WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
				n, err := ptmx.Read(buf)
				if err != nil {
					return
				}
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
		}
	}()

	// Read from WebSocket -> PTY
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			close(done)
			cmd.Process.Kill()
			return
		}

		updateSessionActivity(sessionID)

		// Handle resize messages
		if len(msg) > 0 && msg[0] == 1 {
			if len(msg) >= 5 {
				cols := int(msg[1])<<8 | int(msg[2])
				rows := int(msg[3])<<8 | int(msg[4])
				pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
			}
			continue
		}

		// Track command history (on Enter key)
		for _, b := range msg {
			if b == '\r' || b == '\n' {
				cmdStr := strings.TrimSpace(inputBuffer.String())
				if cmdStr != "" {
					addCommandToHistory(sessionID, cmdStr)
					// Detect sudo/privilege escalation
					if strings.HasPrefix(cmdStr, "sudo ") || strings.Contains(cmdStr, "| sudo") {
						sessionsMutex.Lock()
						if s, ok := sessions[sessionID]; ok {
							s.IsPrivileged = true
						}
						sessionsMutex.Unlock()
					}
				}
				inputBuffer.Reset()
			} else if b == 127 || b == 8 { // Backspace
				if inputBuffer.Len() > 0 {
					inputBuffer.Truncate(inputBuffer.Len() - 1)
				}
			} else if b >= 32 && b < 127 { // Printable characters
				inputBuffer.WriteByte(b)
			}
		}

		ptmx.Write(msg)
	}
}

func handleLocalShell(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Create session
	sessionID := generateSessionID()
	session := &Session{
		ID:         sessionID,
		Type:       "local",
		HostName:   "localhost",
		StartedAt:  time.Now(),
		LastActive: time.Now(),
	}
	registerSession(session)
	defer unregisterSession(sessionID)

	log.Printf("local shell connection (session: %s)", sessionID)

	// Start local shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to start shell: "+err.Error()))
		return
	}
	defer ptmx.Close()

	done := make(chan struct{})
	var inputBuffer bytes.Buffer

	// Read from PTY -> WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
				n, err := ptmx.Read(buf)
				if err != nil {
					return
				}
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
		}
	}()

	// Read from WebSocket -> PTY
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			close(done)
			cmd.Process.Kill()
			return
		}

		updateSessionActivity(sessionID)

		// Handle resize messages
		if len(msg) > 0 && msg[0] == 1 {
			if len(msg) >= 5 {
				cols := int(msg[1])<<8 | int(msg[2])
				rows := int(msg[3])<<8 | int(msg[4])
				pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
			}
			continue
		}

		// Track command history (on Enter key)
		for _, b := range msg {
			if b == '\r' || b == '\n' {
				cmdStr := strings.TrimSpace(inputBuffer.String())
				if cmdStr != "" {
					addCommandToHistory(sessionID, cmdStr)
					// Detect sudo/privilege escalation
					if strings.HasPrefix(cmdStr, "sudo ") || strings.Contains(cmdStr, "| sudo") {
						sessionsMutex.Lock()
						if s, ok := sessions[sessionID]; ok {
							s.IsPrivileged = true
						}
						sessionsMutex.Unlock()
					}
				}
				inputBuffer.Reset()
			} else if b == 127 || b == 8 { // Backspace
				if inputBuffer.Len() > 0 {
					inputBuffer.Truncate(inputBuffer.Len() - 1)
				}
			} else if b >= 32 && b < 127 { // Printable characters
				inputBuffer.WriteByte(b)
			}
		}

		ptmx.Write(msg)
	}
}
