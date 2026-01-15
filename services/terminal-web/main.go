package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

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
}

type SSHSession struct {
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	mu      sync.Mutex
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
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Printf("Warning: could not create table: %v", err)
	}
}

func main() {
	initDB()
	
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/hosts", handleHosts)
	http.HandleFunc("/api/hosts/add", handleAddHost)
	http.HandleFunc("/api/hosts/delete", handleDeleteHost)
	http.HandleFunc("/api/hosts/init", handleInitHosts)
	http.HandleFunc("/ws/terminal", handleTerminal)
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

func handleHosts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	
	if db == nil {
		json.NewEncoder(w).Encode([]Host{})
		return
	}
	
	rows, err := db.Query("SELECT id, name, hostname, port, username, password, COALESCE(ssh_key, ''), COALESCE(auth_type, 'password'), COALESCE(color, '') FROM terminal_hosts ORDER BY name")
	if err != nil {
		log.Printf("Query error: %v", err)
		json.NewEncoder(w).Encode([]Host{})
		return
	}
	defer rows.Close()
	
	hosts := []Host{}
	for rows.Next() {
		var h Host
		err := rows.Scan(&h.ID, &h.Name, &h.Hostname, &h.Port, &h.Username, &h.Password, &h.SSHKey, &h.AuthType, &h.Color)
		if err != nil {
			log.Printf("Scan error: %v", err)
			continue
		}
		h.Password = "***" // Don't expose password
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
	
	var id int
	err := db.QueryRow(`INSERT INTO terminal_hosts (name, hostname, port, username, password, ssh_key, auth_type, color) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		h.Name, h.Hostname, h.Port, h.Username, h.Password, h.SSHKey, h.AuthType, h.Color).Scan(&id)
	
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
		{Name: "rpi1", Hostname: "192.168.8.197", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#ff6b6b"},
		{Name: "rpi2", Hostname: "192.168.8.198", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#4ecdc4"},
		{Name: "rpi3", Hostname: "192.168.8.199", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#45b7d1"},
		{Name: "rpi4", Hostname: "192.168.8.200", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#96ceb4"},
		{Name: "rpi5", Hostname: "192.168.8.201", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#ffeaa7"},
		{Name: "rpi6", Hostname: "192.168.8.202", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#dfe6e9"},
		{Name: "rpi7", Hostname: "192.168.8.203", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#fd79a8"},
		{Name: "rpi8", Hostname: "192.168.8.204", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#a29bfe"},
		{Name: "rpi9", Hostname: "192.168.8.205", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#74b9ff"},
		{Name: "rpi10", Hostname: "192.168.8.206", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#00b894"},
		{Name: "rpi11", Hostname: "192.168.8.207", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#e17055"},
		{Name: "rpi12", Hostname: "192.168.8.208", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#636e72"},
		{Name: "rpi13", Hostname: "192.168.8.209", Port: 22, Username: "rpi1", Password: "19209746", AuthType: "password", Color: "#b2bec3"},
	}
	
	added := 0
	for _, h := range hosts {
		// Check if exists
		var count int
		db.QueryRow("SELECT COUNT(*) FROM terminal_hosts WHERE name = $1", h.Name).Scan(&count)
		if count > 0 {
			continue
		}
		
		_, err := db.Exec(`INSERT INTO terminal_hosts (name, hostname, port, username, password, auth_type, color) 
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			h.Name, h.Hostname, h.Port, h.Username, h.Password, h.AuthType, h.Color)
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
	
	log.Printf("Terminal connection to %s (%s@%s:%d)", h.Name, h.Username, h.Hostname, h.Port)
	
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
	
	session, err := client.NewSession()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("SSH session error: "+err.Error()))
		return
	}
	defer session.Close()
	
	// Set up PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	
	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("PTY error: "+err.Error()))
		return
	}
	
	stdin, err := session.StdinPipe()
	if err != nil {
		return
	}
	
	stdout, err := session.StdoutPipe()
	if err != nil {
		return
	}
	
	stderr, err := session.StderrPipe()
	if err != nil {
		return
	}
	
	if err := session.Shell(); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Shell error: "+err.Error()))
		return
	}
	
	done := make(chan struct{})
	
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
		
		// Handle resize messages
		if len(msg) > 0 && msg[0] == 1 {
			// Resize: [1, cols_high, cols_low, rows_high, rows_low]
			if len(msg) >= 5 {
				cols := int(msg[1])<<8 | int(msg[2])
				rows := int(msg[3])<<8 | int(msg[4])
				session.WindowChange(rows, cols)
			}
			continue
		}
		
		stdin.Write(msg)
	}
}
