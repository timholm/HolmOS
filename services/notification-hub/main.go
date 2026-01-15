package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// Catppuccin Mocha theme colors
const (
	Base     = "#1e1e2e"
	Mantle   = "#181825"
	Crust    = "#11111b"
	Text     = "#cdd6f4"
	Subtext0 = "#a6adc8"
	Overlay0 = "#6c7086"
	Surface0 = "#313244"
	Surface1 = "#45475a"
	Blue     = "#89b4fa"
	Lavender = "#b4befe"
	Mauve    = "#cba6f7"
	Green    = "#a6e3a1"
	Peach    = "#fab387"
	Yellow   = "#f9e2af"
	Red      = "#f38ba8"
)

// UnifiedNotification represents a notification from any source
type UnifiedNotification struct {
	ID          string                 `json:"id"`
	Source      string                 `json:"source"` // queue, email, webhook, echo
	Type        string                 `json:"type"`   // info, success, warning, error
	Title       string                 `json:"title"`
	Message     string                 `json:"message"`
	Recipient   string                 `json:"recipient,omitempty"`
	Priority    string                 `json:"priority"`
	Status      string                 `json:"status"` // pending, sent, delivered, failed, read
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	ProcessedAt *time.Time             `json:"processed_at,omitempty"`
}

// EmailSettings holds SMTP configuration
type EmailSettings struct {
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUser     string `json:"smtp_user"`
	SMTPPassword string `json:"smtp_password,omitempty"`
	FromAddress  string `json:"from_address"`
	FromName     string `json:"from_name"`
	Enabled      bool   `json:"enabled"`
}

// WebhookConfig represents a webhook endpoint
type WebhookConfig struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Secret    string    `json:"secret,omitempty"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// NotificationPreferences holds user preferences
type NotificationPreferences struct {
	EmailEnabled    bool     `json:"email_enabled"`
	WebhookEnabled  bool     `json:"webhook_enabled"`
	EchoEnabled     bool     `json:"echo_enabled"`
	MutedSources    []string `json:"muted_sources"`
	PriorityFilter  string   `json:"priority_filter"` // all, high, normal
	RetentionDays   int      `json:"retention_days"`
}

// ServiceEndpoints holds the service URLs
type ServiceEndpoints struct {
	NotificationQueue   string
	NotificationEmail   string
	NotificationWebhook string
	Echo                string
}

// NotificationHub manages all notifications
type NotificationHub struct {
	notifications []UnifiedNotification
	webhooks      map[string]*WebhookConfig
	emailSettings EmailSettings
	preferences   NotificationPreferences
	endpoints     ServiceEndpoints
	mu            sync.RWMutex
}

var hub *NotificationHub

func NewNotificationHub() *NotificationHub {
	return &NotificationHub{
		notifications: make([]UnifiedNotification, 0),
		webhooks:      make(map[string]*WebhookConfig),
		emailSettings: EmailSettings{
			SMTPHost:    getEnv("SMTP_HOST", "smtp.gmail.com"),
			SMTPPort:    587,
			FromAddress: getEnv("SMTP_FROM", "notifications@holmos.local"),
			FromName:    "HolmOS Notifications",
			Enabled:     false,
		},
		preferences: NotificationPreferences{
			EmailEnabled:   true,
			WebhookEnabled: true,
			EchoEnabled:    true,
			MutedSources:   []string{},
			PriorityFilter: "all",
			RetentionDays:  7,
		},
		endpoints: ServiceEndpoints{
			NotificationQueue:   getEnv("NOTIFICATION_QUEUE_URL", "http://notification-queue"),
			NotificationEmail:   getEnv("NOTIFICATION_EMAIL_URL", "http://notification-email:8080"),
			NotificationWebhook: getEnv("NOTIFICATION_WEBHOOK_URL", "http://notification-webhook:8080"),
			Echo:                getEnv("ECHO_URL", "http://echo"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// AddNotification adds a new notification
func (h *NotificationHub) AddNotification(n UnifiedNotification) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	if n.Status == "" {
		n.Status = "pending"
	}
	if n.Priority == "" {
		n.Priority = "normal"
	}

	h.notifications = append([]UnifiedNotification{n}, h.notifications...)

	// Keep only recent notifications based on retention
	h.cleanupOldNotifications()
}

func (h *NotificationHub) cleanupOldNotifications() {
	cutoff := time.Now().AddDate(0, 0, -h.preferences.RetentionDays)
	filtered := make([]UnifiedNotification, 0)
	for _, n := range h.notifications {
		if n.CreatedAt.After(cutoff) {
			filtered = append(filtered, n)
		}
	}
	h.notifications = filtered
}

// GetNotifications returns all notifications
func (h *NotificationHub) GetNotifications() []UnifiedNotification {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.notifications
}

// GetFilteredNotifications returns notifications based on filters
func (h *NotificationHub) GetFilteredNotifications(source, priority, status string) []UnifiedNotification {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]UnifiedNotification, 0)
	for _, n := range h.notifications {
		if source != "" && source != "all" && n.Source != source {
			continue
		}
		if priority != "" && priority != "all" && n.Priority != priority {
			continue
		}
		if status != "" && status != "all" && n.Status != status {
			continue
		}
		result = append(result, n)
	}
	return result
}

// FetchFromServices pulls notifications from all connected services
func (h *NotificationHub) FetchFromServices() {
	var wg sync.WaitGroup

	// Fetch from notification-queue
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.fetchFromQueue()
	}()

	// Fetch from Echo
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.fetchFromEcho()
	}()

	wg.Wait()
}

func (h *NotificationHub) fetchFromQueue() {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(h.endpoints.NotificationQueue + "/queue")
	if err != nil {
		log.Printf("Error fetching from queue: %v", err)
		return
	}
	defer resp.Body.Close()

	var items []struct {
		ID        string                 `json:"id"`
		Type      string                 `json:"type"`
		Recipient string                 `json:"recipient"`
		Subject   string                 `json:"subject"`
		Content   string                 `json:"content"`
		Priority  int                    `json:"priority"`
		Metadata  map[string]interface{} `json:"metadata"`
		Status    string                 `json:"status"`
		CreatedAt time.Time              `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		log.Printf("Error decoding queue response: %v", err)
		return
	}

	for _, item := range items {
		priority := "normal"
		if item.Priority > 5 {
			priority = "high"
		} else if item.Priority < 3 {
			priority = "low"
		}

		n := UnifiedNotification{
			ID:        "queue-" + item.ID,
			Source:    "queue",
			Type:      item.Type,
			Title:     item.Subject,
			Message:   item.Content,
			Recipient: item.Recipient,
			Priority:  priority,
			Status:    item.Status,
			Metadata:  item.Metadata,
			CreatedAt: item.CreatedAt,
		}
		h.AddNotification(n)
	}
}

func (h *NotificationHub) fetchFromEcho() {
	client := &http.Client{Timeout: 5 * time.Second}

	// Fetch notifications from Echo
	resp, err := client.Get(h.endpoints.Echo + "/api/notifications")
	if err != nil {
		log.Printf("Error fetching from Echo notifications: %v", err)
		return
	}
	defer resp.Body.Close()

	var notifications []struct {
		ID        string    `json:"id"`
		Type      string    `json:"type"`
		Title     string    `json:"title"`
		Message   string    `json:"message"`
		Source    string    `json:"source"`
		Priority  string    `json:"priority"`
		Read      bool      `json:"read"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&notifications); err != nil {
		log.Printf("Error decoding Echo notifications: %v", err)
		return
	}

	for _, notif := range notifications {
		status := "pending"
		if notif.Read {
			status = "read"
		}

		n := UnifiedNotification{
			ID:        "echo-" + notif.ID,
			Source:    "echo",
			Type:      notif.Type,
			Title:     notif.Title,
			Message:   notif.Message,
			Priority:  notif.Priority,
			Status:    status,
			CreatedAt: notif.CreatedAt,
		}
		h.AddNotification(n)
	}

	// Fetch messages from Echo
	resp2, err := client.Get(h.endpoints.Echo + "/api/messages")
	if err != nil {
		log.Printf("Error fetching from Echo messages: %v", err)
		return
	}
	defer resp2.Body.Close()

	var messages []struct {
		ID        string    `json:"id"`
		From      string    `json:"from"`
		To        string    `json:"to"`
		Subject   string    `json:"subject"`
		Body      string    `json:"body"`
		Priority  string    `json:"priority"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp2.Body).Decode(&messages); err != nil {
		log.Printf("Error decoding Echo messages: %v", err)
		return
	}

	for _, msg := range messages {
		n := UnifiedNotification{
			ID:        "echo-msg-" + msg.ID,
			Source:    "echo",
			Type:      "message",
			Title:     msg.Subject,
			Message:   msg.Body,
			Recipient: msg.To,
			Priority:  msg.Priority,
			Status:    msg.Status,
			Metadata: map[string]interface{}{
				"from": msg.From,
			},
			CreatedAt: msg.CreatedAt,
		}
		h.AddNotification(n)
	}
}

// SendNotification routes a notification to appropriate services
func (h *NotificationHub) SendNotification(n UnifiedNotification) error {
	h.AddNotification(n)

	var lastErr error

	// Send to Echo if enabled
	if h.preferences.EchoEnabled {
		if err := h.sendToEcho(n); err != nil {
			log.Printf("Error sending to Echo: %v", err)
			lastErr = err
		}
	}

	// Send to webhook if enabled
	if h.preferences.WebhookEnabled {
		if err := h.sendToWebhooks(n); err != nil {
			log.Printf("Error sending to webhooks: %v", err)
			lastErr = err
		}
	}

	// Queue for email if enabled and has recipient
	if h.preferences.EmailEnabled && n.Recipient != "" {
		if err := h.queueForEmail(n); err != nil {
			log.Printf("Error queueing for email: %v", err)
			lastErr = err
		}
	}

	return lastErr
}

func (h *NotificationHub) sendToEcho(n UnifiedNotification) error {
	payload := map[string]interface{}{
		"type":     n.Type,
		"title":    n.Title,
		"message":  n.Message,
		"source":   "notification-hub",
		"priority": n.Priority,
	}

	jsonData, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(h.endpoints.Echo+"/api/notify", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (h *NotificationHub) sendToWebhooks(n UnifiedNotification) error {
	h.mu.RLock()
	webhooks := make([]*WebhookConfig, 0)
	for _, wh := range h.webhooks {
		if wh.Active {
			webhooks = append(webhooks, wh)
		}
	}
	h.mu.RUnlock()

	payload := map[string]interface{}{
		"event":   n.Type,
		"payload": n,
	}

	jsonData, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 5 * time.Second}

	// Also send to the notification-webhook service
	resp, err := client.Post(h.endpoints.NotificationWebhook+"/send", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		log.Printf("Error sending to notification-webhook service: %v", err)
	} else {
		resp.Body.Close()
	}

	return nil
}

func (h *NotificationHub) queueForEmail(n UnifiedNotification) error {
	payload := map[string]interface{}{
		"type":      "email",
		"recipient": n.Recipient,
		"subject":   n.Title,
		"content":   n.Message,
		"priority":  5,
	}

	jsonData, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(h.endpoints.NotificationQueue+"/queue", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// Webhook management
func (h *NotificationHub) AddWebhook(wh WebhookConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if wh.ID == "" {
		wh.ID = uuid.New().String()
	}
	wh.CreatedAt = time.Now()
	wh.Active = true
	h.webhooks[wh.ID] = &wh
}

func (h *NotificationHub) GetWebhooks() []*WebhookConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()

	webhooks := make([]*WebhookConfig, 0)
	for _, wh := range h.webhooks {
		// Don't expose secrets
		copy := *wh
		copy.Secret = ""
		webhooks = append(webhooks, &copy)
	}
	return webhooks
}

func (h *NotificationHub) DeleteWebhook(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.webhooks[id]; exists {
		delete(h.webhooks, id)
		return true
	}
	return false
}

// GetStats returns notification statistics
func (h *NotificationHub) GetStats() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	bySource := make(map[string]int)
	byType := make(map[string]int)
	byStatus := make(map[string]int)
	byPriority := make(map[string]int)

	for _, n := range h.notifications {
		bySource[n.Source]++
		byType[n.Type]++
		byStatus[n.Status]++
		byPriority[n.Priority]++
	}

	return map[string]interface{}{
		"total":       len(h.notifications),
		"by_source":   bySource,
		"by_type":     byType,
		"by_status":   byStatus,
		"by_priority": byPriority,
		"webhooks":    len(h.webhooks),
	}
}

// HTTP Handlers
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"service": "notification-hub",
		"time":    time.Now().UTC(),
	})
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	// Refresh from services
	hub.FetchFromServices()

	tmpl := template.Must(template.New("index").Parse(indexHTML))

	notifications := hub.GetNotifications()

	// Sort by created_at descending
	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].CreatedAt.After(notifications[j].CreatedAt)
	})

	// Limit to 50 most recent
	if len(notifications) > 50 {
		notifications = notifications[:50]
	}

	data := struct {
		Notifications []UnifiedNotification
		Stats         map[string]interface{}
		Webhooks      []*WebhookConfig
		EmailSettings EmailSettings
		Preferences   NotificationPreferences
	}{
		Notifications: notifications,
		Stats:         hub.GetStats(),
		Webhooks:      hub.GetWebhooks(),
		EmailSettings: hub.emailSettings,
		Preferences:   hub.preferences,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	priority := r.URL.Query().Get("priority")
	status := r.URL.Query().Get("status")

	notifications := hub.GetFilteredNotifications(source, priority, status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notifications)
}

func sendNotificationHandler(w http.ResponseWriter, r *http.Request) {
	var n UnifiedNotification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	if n.Title == "" || n.Message == "" {
		http.Error(w, `{"error": "title and message are required"}`, http.StatusBadRequest)
		return
	}

	if n.Source == "" {
		n.Source = "hub"
	}

	if err := hub.SendNotification(n); err != nil {
		log.Printf("Error sending notification: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(n)
}

func getStatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hub.GetStats())
}

func refreshHandler(w http.ResponseWriter, r *http.Request) {
	hub.FetchFromServices()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "refreshed"})
}

// Webhook handlers
func listWebhooksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hub.GetWebhooks())
}

func createWebhookHandler(w http.ResponseWriter, r *http.Request) {
	var wh WebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&wh); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	if wh.Name == "" || wh.URL == "" {
		http.Error(w, `{"error": "name and url are required"}`, http.StatusBadRequest)
		return
	}

	hub.AddWebhook(wh)

	// Also register with notification-webhook service
	payload := map[string]interface{}{
		"url":    wh.URL,
		"events": wh.Events,
		"secret": wh.Secret,
	}
	jsonData, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(hub.endpoints.NotificationWebhook+"/webhooks", "application/json", bytes.NewReader(jsonData))
	if err != nil {
		log.Printf("Error registering webhook with service: %v", err)
	} else {
		resp.Body.Close()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(wh)
}

func deleteWebhookHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if !hub.DeleteWebhook(id) {
		http.Error(w, `{"error": "webhook not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "webhook deleted", "id": id})
}

// Email settings handlers
func getEmailSettingsHandler(w http.ResponseWriter, r *http.Request) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	// Don't expose password
	settings := hub.emailSettings
	settings.SMTPPassword = ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

func updateEmailSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var settings EmailSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	hub.mu.Lock()
	// Preserve password if not provided
	if settings.SMTPPassword == "" {
		settings.SMTPPassword = hub.emailSettings.SMTPPassword
	}
	hub.emailSettings = settings
	hub.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

// Preferences handlers
func getPreferencesHandler(w http.ResponseWriter, r *http.Request) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hub.preferences)
}

func updatePreferencesHandler(w http.ResponseWriter, r *http.Request) {
	var prefs NotificationPreferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	hub.mu.Lock()
	hub.preferences = prefs
	hub.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

// Proxy handlers for other services
func proxyQueueHandler(w http.ResponseWriter, r *http.Request) {
	proxyRequest(w, r, hub.endpoints.NotificationQueue)
}

func proxyEmailHandler(w http.ResponseWriter, r *http.Request) {
	proxyRequest(w, r, hub.endpoints.NotificationEmail)
}

func proxyWebhookHandler(w http.ResponseWriter, r *http.Request) {
	proxyRequest(w, r, hub.endpoints.NotificationWebhook)
}

func proxyEchoHandler(w http.ResponseWriter, r *http.Request) {
	proxyRequest(w, r, hub.endpoints.Echo)
}

func proxyRequest(w http.ResponseWriter, r *http.Request, baseURL string) {
	vars := mux.Vars(r)
	path := vars["path"]
	if path == "" {
		path = "/"
	}

	targetURL := baseURL + "/" + path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req.Header = r.Header

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func main() {
	port := getEnv("PORT", "8080")

	hub = NewNotificationHub()

	// Initial fetch from services
	go hub.FetchFromServices()

	// Periodic refresh
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			hub.FetchFromServices()
		}
	}()

	r := mux.NewRouter()

	// Core endpoints
	r.HandleFunc("/", indexHandler).Methods("GET")
	r.HandleFunc("/health", healthHandler).Methods("GET")
	r.HandleFunc("/api/notifications", getNotificationsHandler).Methods("GET")
	r.HandleFunc("/api/notifications", sendNotificationHandler).Methods("POST")
	r.HandleFunc("/api/stats", getStatsHandler).Methods("GET")
	r.HandleFunc("/api/refresh", refreshHandler).Methods("POST")

	// Webhook management
	r.HandleFunc("/api/webhooks", listWebhooksHandler).Methods("GET")
	r.HandleFunc("/api/webhooks", createWebhookHandler).Methods("POST")
	r.HandleFunc("/api/webhooks/{id}", deleteWebhookHandler).Methods("DELETE")

	// Email settings
	r.HandleFunc("/api/email/settings", getEmailSettingsHandler).Methods("GET")
	r.HandleFunc("/api/email/settings", updateEmailSettingsHandler).Methods("POST", "PUT")

	// Preferences
	r.HandleFunc("/api/preferences", getPreferencesHandler).Methods("GET")
	r.HandleFunc("/api/preferences", updatePreferencesHandler).Methods("POST", "PUT")

	// Proxy endpoints to other services
	r.PathPrefix("/proxy/queue/{path:.*}").HandlerFunc(proxyQueueHandler)
	r.PathPrefix("/proxy/email/{path:.*}").HandlerFunc(proxyEmailHandler)
	r.PathPrefix("/proxy/webhook/{path:.*}").HandlerFunc(proxyWebhookHandler)
	r.PathPrefix("/proxy/echo/{path:.*}").HandlerFunc(proxyEchoHandler)

	log.Printf("Notification Hub starting on port %s", port)
	log.Printf("Connected services: Queue=%s, Email=%s, Webhook=%s, Echo=%s",
		hub.endpoints.NotificationQueue,
		hub.endpoints.NotificationEmail,
		hub.endpoints.NotificationWebhook,
		hub.endpoints.Echo)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Notification Hub - HolmOS</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #1e1e2e;
            color: #cdd6f4;
            min-height: 100vh;
            line-height: 1.6;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
            padding: 2rem;
        }

        header {
            text-align: center;
            margin-bottom: 2rem;
            padding: 2rem;
            background: linear-gradient(135deg, #181825 0%, #313244 100%);
            border-radius: 16px;
            border: 1px solid #45475a;
        }

        .logo {
            font-size: 3rem;
            margin-bottom: 0.5rem;
        }

        h1 {
            font-size: 2rem;
            color: #cba6f7;
            margin-bottom: 0.25rem;
        }

        .tagline {
            color: #a6adc8;
            font-size: 1rem;
        }

        .tabs {
            display: flex;
            gap: 0.5rem;
            margin-bottom: 1.5rem;
            flex-wrap: wrap;
        }

        .tab {
            background: #313244;
            border: 1px solid #45475a;
            border-radius: 8px;
            padding: 0.75rem 1.5rem;
            color: #a6adc8;
            cursor: pointer;
            transition: all 0.2s;
        }

        .tab:hover, .tab.active {
            background: #45475a;
            color: #cdd6f4;
            border-color: #cba6f7;
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }

        .stat-card {
            background: #181825;
            border: 1px solid #45475a;
            border-radius: 12px;
            padding: 1.25rem;
            text-align: center;
        }

        .stat-value {
            font-size: 2rem;
            font-weight: bold;
            color: #cba6f7;
        }

        .stat-label {
            color: #a6adc8;
            font-size: 0.85rem;
            text-transform: uppercase;
            letter-spacing: 1px;
        }

        .section {
            background: #181825;
            border: 1px solid #45475a;
            border-radius: 16px;
            padding: 1.5rem;
            margin-bottom: 1.5rem;
        }

        .section-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 1rem;
            padding-bottom: 1rem;
            border-bottom: 1px solid #313244;
        }

        .section-title {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            color: #cdd6f4;
            font-size: 1.1rem;
        }

        .section-icon {
            font-size: 1.25rem;
        }

        .notification-list {
            display: flex;
            flex-direction: column;
            gap: 0.75rem;
            max-height: 500px;
            overflow-y: auto;
        }

        .notification-item {
            background: #313244;
            border-radius: 10px;
            padding: 1rem;
            display: flex;
            gap: 1rem;
            border-left: 4px solid #89b4fa;
        }

        .notification-item.source-queue { border-left-color: #89b4fa; }
        .notification-item.source-email { border-left-color: #a6e3a1; }
        .notification-item.source-webhook { border-left-color: #fab387; }
        .notification-item.source-echo { border-left-color: #b4befe; }
        .notification-item.source-hub { border-left-color: #cba6f7; }

        .notification-item.priority-high { border-left-color: #f38ba8; }
        .notification-item.priority-low { border-left-color: #6c7086; }

        .notif-icon {
            font-size: 1.5rem;
            flex-shrink: 0;
        }

        .notif-content {
            flex: 1;
            min-width: 0;
        }

        .notif-header {
            display: flex;
            justify-content: space-between;
            align-items: flex-start;
            gap: 1rem;
            margin-bottom: 0.25rem;
        }

        .notif-title {
            color: #cdd6f4;
            font-weight: 600;
        }

        .notif-meta {
            display: flex;
            gap: 0.5rem;
            flex-wrap: wrap;
        }

        .badge {
            font-size: 0.7rem;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            text-transform: uppercase;
            font-weight: 600;
        }

        .badge-source { background: #45475a; color: #cdd6f4; }
        .badge-queue { background: #89b4fa20; color: #89b4fa; }
        .badge-email { background: #a6e3a120; color: #a6e3a1; }
        .badge-webhook { background: #fab38720; color: #fab387; }
        .badge-echo { background: #b4befe20; color: #b4befe; }
        .badge-hub { background: #cba6f720; color: #cba6f7; }

        .badge-pending { background: #f9e2af20; color: #f9e2af; }
        .badge-sent { background: #89b4fa20; color: #89b4fa; }
        .badge-delivered { background: #a6e3a120; color: #a6e3a1; }
        .badge-failed { background: #f38ba820; color: #f38ba8; }
        .badge-read { background: #6c708620; color: #6c7086; }

        .notif-message {
            color: #a6adc8;
            font-size: 0.9rem;
            margin-top: 0.25rem;
        }

        .notif-time {
            color: #6c7086;
            font-size: 0.8rem;
            margin-top: 0.5rem;
        }

        .form-grid {
            display: grid;
            gap: 1rem;
        }

        .form-row {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
        }

        .form-group {
            display: flex;
            flex-direction: column;
            gap: 0.4rem;
        }

        .form-group label {
            color: #a6adc8;
            font-size: 0.85rem;
        }

        .form-group input,
        .form-group textarea,
        .form-group select {
            background: #1e1e2e;
            border: 1px solid #45475a;
            border-radius: 8px;
            padding: 0.6rem 0.8rem;
            color: #cdd6f4;
            font-size: 0.95rem;
        }

        .form-group input:focus,
        .form-group textarea:focus,
        .form-group select:focus {
            outline: none;
            border-color: #cba6f7;
        }

        .form-group textarea {
            min-height: 80px;
            resize: vertical;
        }

        .btn {
            background: linear-gradient(135deg, #cba6f7 0%, #b4befe 100%);
            color: #1e1e2e;
            border: none;
            border-radius: 8px;
            padding: 0.6rem 1.2rem;
            font-size: 0.95rem;
            font-weight: 600;
            cursor: pointer;
            transition: transform 0.2s, box-shadow 0.2s;
        }

        .btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(203, 166, 247, 0.3);
        }

        .btn-secondary {
            background: #45475a;
            color: #cdd6f4;
        }

        .btn-secondary:hover {
            background: #585b70;
            box-shadow: none;
        }

        .btn-danger {
            background: #f38ba8;
        }

        .webhook-list, .settings-list {
            display: flex;
            flex-direction: column;
            gap: 0.75rem;
        }

        .webhook-item {
            background: #313244;
            border-radius: 10px;
            padding: 1rem;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .webhook-info {
            flex: 1;
        }

        .webhook-name {
            color: #cdd6f4;
            font-weight: 600;
        }

        .webhook-url {
            color: #6c7086;
            font-size: 0.85rem;
            word-break: break-all;
        }

        .webhook-events {
            display: flex;
            gap: 0.25rem;
            margin-top: 0.5rem;
            flex-wrap: wrap;
        }

        .toggle-group {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 0.75rem 0;
            border-bottom: 1px solid #313244;
        }

        .toggle-label {
            color: #cdd6f4;
        }

        .toggle {
            position: relative;
            width: 50px;
            height: 26px;
        }

        .toggle input {
            opacity: 0;
            width: 0;
            height: 0;
        }

        .toggle-slider {
            position: absolute;
            cursor: pointer;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background-color: #45475a;
            transition: 0.3s;
            border-radius: 26px;
        }

        .toggle-slider:before {
            position: absolute;
            content: "";
            height: 20px;
            width: 20px;
            left: 3px;
            bottom: 3px;
            background-color: #cdd6f4;
            transition: 0.3s;
            border-radius: 50%;
        }

        .toggle input:checked + .toggle-slider {
            background-color: #a6e3a1;
        }

        .toggle input:checked + .toggle-slider:before {
            transform: translateX(24px);
        }

        .empty-state {
            text-align: center;
            padding: 2rem;
            color: #6c7086;
        }

        .filter-bar {
            display: flex;
            gap: 1rem;
            margin-bottom: 1rem;
            flex-wrap: wrap;
        }

        .filter-bar select {
            background: #313244;
            border: 1px solid #45475a;
            border-radius: 6px;
            padding: 0.5rem 1rem;
            color: #cdd6f4;
            font-size: 0.9rem;
        }

        footer {
            text-align: center;
            padding: 2rem;
            color: #6c7086;
            font-size: 0.85rem;
        }

        footer a {
            color: #cba6f7;
            text-decoration: none;
        }

        .panel {
            display: none;
        }

        .panel.active {
            display: block;
        }

        .service-status {
            display: flex;
            gap: 1rem;
            flex-wrap: wrap;
            margin-top: 1rem;
        }

        .service-badge {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            background: #313244;
            padding: 0.5rem 1rem;
            border-radius: 8px;
            font-size: 0.85rem;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: #a6e3a1;
        }

        .status-dot.offline {
            background: #f38ba8;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <div class="logo">&#128276;</div>
            <h1>Notification Hub</h1>
            <p class="tagline">Unified notification center for HolmOS</p>
            <div class="service-status">
                <div class="service-badge">
                    <span class="status-dot"></span>
                    <span>Queue</span>
                </div>
                <div class="service-badge">
                    <span class="status-dot"></span>
                    <span>Email</span>
                </div>
                <div class="service-badge">
                    <span class="status-dot"></span>
                    <span>Webhook</span>
                </div>
                <div class="service-badge">
                    <span class="status-dot"></span>
                    <span>Echo</span>
                </div>
            </div>
        </header>

        <div class="tabs">
            <div class="tab active" data-panel="notifications">Notifications</div>
            <div class="tab" data-panel="send">Send</div>
            <div class="tab" data-panel="webhooks">Webhooks</div>
            <div class="tab" data-panel="email">Email Settings</div>
            <div class="tab" data-panel="preferences">Preferences</div>
        </div>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-value">{{.Stats.total}}</div>
                <div class="stat-label">Total</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{index .Stats.by_source "queue"}}</div>
                <div class="stat-label">Queue</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{index .Stats.by_source "echo"}}</div>
                <div class="stat-label">Echo</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{index .Stats.by_source "webhook"}}</div>
                <div class="stat-label">Webhook</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">{{.Stats.webhooks}}</div>
                <div class="stat-label">Webhooks</div>
            </div>
        </div>

        <!-- Notifications Panel -->
        <div class="panel active" id="notifications">
            <div class="section">
                <div class="section-header">
                    <div class="section-title">
                        <span class="section-icon">&#128276;</span>
                        <span>All Notifications</span>
                    </div>
                    <button class="btn btn-secondary" onclick="refreshNotifications()">Refresh</button>
                </div>

                <div class="filter-bar">
                    <select id="filter-source" onchange="filterNotifications()">
                        <option value="all">All Sources</option>
                        <option value="queue">Queue</option>
                        <option value="echo">Echo</option>
                        <option value="webhook">Webhook</option>
                        <option value="hub">Hub</option>
                    </select>
                    <select id="filter-priority" onchange="filterNotifications()">
                        <option value="all">All Priorities</option>
                        <option value="high">High</option>
                        <option value="normal">Normal</option>
                        <option value="low">Low</option>
                    </select>
                    <select id="filter-status" onchange="filterNotifications()">
                        <option value="all">All Status</option>
                        <option value="pending">Pending</option>
                        <option value="sent">Sent</option>
                        <option value="delivered">Delivered</option>
                        <option value="read">Read</option>
                        <option value="failed">Failed</option>
                    </select>
                </div>

                <div class="notification-list" id="notification-list">
                    {{range .Notifications}}
                    <div class="notification-item source-{{.Source}} priority-{{.Priority}}">
                        <span class="notif-icon">
                            {{if eq .Type "info"}}&#8505;&#65039;{{end}}
                            {{if eq .Type "success"}}&#9989;{{end}}
                            {{if eq .Type "warning"}}&#9888;&#65039;{{end}}
                            {{if eq .Type "error"}}&#10060;{{end}}
                            {{if eq .Type "message"}}&#128172;{{end}}
                            {{if eq .Type "email"}}&#128231;{{end}}
                            {{if not .Type}}&#128276;{{end}}
                        </span>
                        <div class="notif-content">
                            <div class="notif-header">
                                <span class="notif-title">{{.Title}}</span>
                                <div class="notif-meta">
                                    <span class="badge badge-{{.Source}}">{{.Source}}</span>
                                    <span class="badge badge-{{.Status}}">{{.Status}}</span>
                                </div>
                            </div>
                            <div class="notif-message">{{.Message}}</div>
                            {{if .Recipient}}<div class="notif-time">To: {{.Recipient}}</div>{{end}}
                            <div class="notif-time">{{.CreatedAt.Format "Jan 2, 15:04:05"}}</div>
                        </div>
                    </div>
                    {{else}}
                    <div class="empty-state">No notifications yet</div>
                    {{end}}
                </div>
            </div>
        </div>

        <!-- Send Panel -->
        <div class="panel" id="send">
            <div class="section">
                <div class="section-header">
                    <div class="section-title">
                        <span class="section-icon">&#9993;&#65039;</span>
                        <span>Send Notification</span>
                    </div>
                </div>

                <form class="form-grid" id="sendForm">
                    <div class="form-row">
                        <div class="form-group">
                            <label for="notif-type">Type</label>
                            <select id="notif-type" name="type">
                                <option value="info">Info</option>
                                <option value="success">Success</option>
                                <option value="warning">Warning</option>
                                <option value="error">Error</option>
                            </select>
                        </div>
                        <div class="form-group">
                            <label for="notif-priority">Priority</label>
                            <select id="notif-priority" name="priority">
                                <option value="normal">Normal</option>
                                <option value="high">High</option>
                                <option value="low">Low</option>
                            </select>
                        </div>
                        <div class="form-group">
                            <label for="notif-recipient">Recipient (optional)</label>
                            <input type="text" id="notif-recipient" name="recipient" placeholder="email@example.com">
                        </div>
                    </div>
                    <div class="form-group">
                        <label for="notif-title">Title</label>
                        <input type="text" id="notif-title" name="title" placeholder="Notification title" required>
                    </div>
                    <div class="form-group">
                        <label for="notif-message">Message</label>
                        <textarea id="notif-message" name="message" placeholder="Notification message..." required></textarea>
                    </div>
                    <button type="submit" class="btn">Send Notification</button>
                </form>
            </div>
        </div>

        <!-- Webhooks Panel -->
        <div class="panel" id="webhooks">
            <div class="section">
                <div class="section-header">
                    <div class="section-title">
                        <span class="section-icon">&#128279;</span>
                        <span>Webhook Management</span>
                    </div>
                </div>

                <div class="webhook-list" id="webhook-list">
                    {{range .Webhooks}}
                    <div class="webhook-item">
                        <div class="webhook-info">
                            <div class="webhook-name">{{.Name}}</div>
                            <div class="webhook-url">{{.URL}}</div>
                            <div class="webhook-events">
                                {{range .Events}}
                                <span class="badge badge-source">{{.}}</span>
                                {{end}}
                            </div>
                        </div>
                        <button class="btn btn-danger" onclick="deleteWebhook('{{.ID}}')">Delete</button>
                    </div>
                    {{else}}
                    <div class="empty-state">No webhooks configured</div>
                    {{end}}
                </div>

                <div style="margin-top: 1.5rem; padding-top: 1.5rem; border-top: 1px solid #313244;">
                    <h3 style="margin-bottom: 1rem; color: #a6adc8;">Add New Webhook</h3>
                    <form class="form-grid" id="webhookForm">
                        <div class="form-row">
                            <div class="form-group">
                                <label for="wh-name">Name</label>
                                <input type="text" id="wh-name" name="name" placeholder="My Webhook" required>
                            </div>
                            <div class="form-group">
                                <label for="wh-url">URL</label>
                                <input type="url" id="wh-url" name="url" placeholder="https://example.com/webhook" required>
                            </div>
                        </div>
                        <div class="form-row">
                            <div class="form-group">
                                <label for="wh-events">Events (comma-separated)</label>
                                <input type="text" id="wh-events" name="events" placeholder="*, info, error">
                            </div>
                            <div class="form-group">
                                <label for="wh-secret">Secret (optional)</label>
                                <input type="password" id="wh-secret" name="secret" placeholder="Webhook secret">
                            </div>
                        </div>
                        <button type="submit" class="btn">Add Webhook</button>
                    </form>
                </div>
            </div>
        </div>

        <!-- Email Settings Panel -->
        <div class="panel" id="email">
            <div class="section">
                <div class="section-header">
                    <div class="section-title">
                        <span class="section-icon">&#128231;</span>
                        <span>Email Settings</span>
                    </div>
                </div>

                <form class="form-grid" id="emailForm">
                    <div class="toggle-group">
                        <span class="toggle-label">Enable Email Notifications</span>
                        <label class="toggle">
                            <input type="checkbox" id="email-enabled" {{if .EmailSettings.Enabled}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>

                    <div class="form-row">
                        <div class="form-group">
                            <label for="smtp-host">SMTP Host</label>
                            <input type="text" id="smtp-host" name="smtp_host" value="{{.EmailSettings.SMTPHost}}" placeholder="smtp.gmail.com">
                        </div>
                        <div class="form-group">
                            <label for="smtp-port">SMTP Port</label>
                            <input type="number" id="smtp-port" name="smtp_port" value="{{.EmailSettings.SMTPPort}}" placeholder="587">
                        </div>
                    </div>
                    <div class="form-row">
                        <div class="form-group">
                            <label for="smtp-user">SMTP Username</label>
                            <input type="text" id="smtp-user" name="smtp_user" value="{{.EmailSettings.SMTPUser}}" placeholder="user@gmail.com">
                        </div>
                        <div class="form-group">
                            <label for="smtp-password">SMTP Password</label>
                            <input type="password" id="smtp-password" name="smtp_password" placeholder="App password">
                        </div>
                    </div>
                    <div class="form-row">
                        <div class="form-group">
                            <label for="from-address">From Address</label>
                            <input type="email" id="from-address" name="from_address" value="{{.EmailSettings.FromAddress}}" placeholder="notifications@holmos.local">
                        </div>
                        <div class="form-group">
                            <label for="from-name">From Name</label>
                            <input type="text" id="from-name" name="from_name" value="{{.EmailSettings.FromName}}" placeholder="HolmOS Notifications">
                        </div>
                    </div>
                    <button type="submit" class="btn">Save Email Settings</button>
                </form>
            </div>
        </div>

        <!-- Preferences Panel -->
        <div class="panel" id="preferences">
            <div class="section">
                <div class="section-header">
                    <div class="section-title">
                        <span class="section-icon">&#9881;&#65039;</span>
                        <span>Notification Preferences</span>
                    </div>
                </div>

                <form class="form-grid" id="preferencesForm">
                    <div class="toggle-group">
                        <span class="toggle-label">Email Notifications</span>
                        <label class="toggle">
                            <input type="checkbox" id="pref-email" {{if .Preferences.EmailEnabled}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                    <div class="toggle-group">
                        <span class="toggle-label">Webhook Notifications</span>
                        <label class="toggle">
                            <input type="checkbox" id="pref-webhook" {{if .Preferences.WebhookEnabled}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                    <div class="toggle-group">
                        <span class="toggle-label">Echo Messages</span>
                        <label class="toggle">
                            <input type="checkbox" id="pref-echo" {{if .Preferences.EchoEnabled}}checked{{end}}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>

                    <div class="form-row" style="margin-top: 1rem;">
                        <div class="form-group">
                            <label for="pref-priority">Default Priority Filter</label>
                            <select id="pref-priority">
                                <option value="all" {{if eq .Preferences.PriorityFilter "all"}}selected{{end}}>All</option>
                                <option value="high" {{if eq .Preferences.PriorityFilter "high"}}selected{{end}}>High Only</option>
                                <option value="normal" {{if eq .Preferences.PriorityFilter "normal"}}selected{{end}}>Normal & High</option>
                            </select>
                        </div>
                        <div class="form-group">
                            <label for="pref-retention">Retention Days</label>
                            <input type="number" id="pref-retention" value="{{.Preferences.RetentionDays}}" min="1" max="90">
                        </div>
                    </div>
                    <button type="submit" class="btn">Save Preferences</button>
                </form>
            </div>
        </div>

        <footer>
            <p>Notification Hub - Part of <a href="/">HolmOS</a></p>
            <p>Connecting Queue, Email, Webhook, and Echo services</p>
        </footer>
    </div>

    <script>
        // Tab switching
        document.querySelectorAll('.tab').forEach(tab => {
            tab.addEventListener('click', () => {
                document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
                document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
                tab.classList.add('active');
                document.getElementById(tab.dataset.panel).classList.add('active');
            });
        });

        // Refresh notifications
        async function refreshNotifications() {
            try {
                await fetch('/api/refresh', { method: 'POST' });
                window.location.reload();
            } catch (err) {
                alert('Error refreshing: ' + err.message);
            }
        }

        // Filter notifications
        async function filterNotifications() {
            const source = document.getElementById('filter-source').value;
            const priority = document.getElementById('filter-priority').value;
            const status = document.getElementById('filter-status').value;

            try {
                const resp = await fetch('/api/notifications?source=' + source + '&priority=' + priority + '&status=' + status);
                const notifications = await resp.json();
                renderNotifications(notifications);
            } catch (err) {
                console.error('Filter error:', err);
            }
        }

        function renderNotifications(notifications) {
            const list = document.getElementById('notification-list');
            if (!notifications || notifications.length === 0) {
                list.innerHTML = '<div class="empty-state">No notifications found</div>';
                return;
            }

            list.innerHTML = notifications.map(n => {
                const icon = {
                    'info': '&#8505;&#65039;',
                    'success': '&#9989;',
                    'warning': '&#9888;&#65039;',
                    'error': '&#10060;',
                    'message': '&#128172;',
                    'email': '&#128231;'
                }[n.type] || '&#128276;';

                const created = new Date(n.created_at).toLocaleString();

                return '<div class="notification-item source-' + n.source + ' priority-' + n.priority + '">' +
                    '<span class="notif-icon">' + icon + '</span>' +
                    '<div class="notif-content">' +
                        '<div class="notif-header">' +
                            '<span class="notif-title">' + (n.title || 'Notification') + '</span>' +
                            '<div class="notif-meta">' +
                                '<span class="badge badge-' + n.source + '">' + n.source + '</span>' +
                                '<span class="badge badge-' + n.status + '">' + n.status + '</span>' +
                            '</div>' +
                        '</div>' +
                        '<div class="notif-message">' + (n.message || '') + '</div>' +
                        (n.recipient ? '<div class="notif-time">To: ' + n.recipient + '</div>' : '') +
                        '<div class="notif-time">' + created + '</div>' +
                    '</div>' +
                '</div>';
            }).join('');
        }

        // Send notification form
        document.getElementById('sendForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const form = e.target;
            const data = {
                type: form.type.value,
                priority: form.priority.value,
                title: form.title.value,
                message: form.message.value,
                recipient: form.recipient.value || undefined
            };

            try {
                const resp = await fetch('/api/notifications', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(data)
                });

                if (resp.ok) {
                    alert('Notification sent!');
                    form.reset();
                    document.querySelector('[data-panel="notifications"]').click();
                    window.location.reload();
                } else {
                    alert('Failed to send notification');
                }
            } catch (err) {
                alert('Error: ' + err.message);
            }
        });

        // Webhook form
        document.getElementById('webhookForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const form = e.target;
            const events = form.events.value.split(',').map(e => e.trim()).filter(e => e);
            const data = {
                name: form.name.value,
                url: form.url.value,
                events: events.length > 0 ? events : ['*'],
                secret: form.secret.value || undefined
            };

            try {
                const resp = await fetch('/api/webhooks', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(data)
                });

                if (resp.ok) {
                    alert('Webhook added!');
                    window.location.reload();
                } else {
                    alert('Failed to add webhook');
                }
            } catch (err) {
                alert('Error: ' + err.message);
            }
        });

        // Delete webhook
        async function deleteWebhook(id) {
            if (!confirm('Delete this webhook?')) return;

            try {
                const resp = await fetch('/api/webhooks/' + id, { method: 'DELETE' });
                if (resp.ok) {
                    window.location.reload();
                } else {
                    alert('Failed to delete webhook');
                }
            } catch (err) {
                alert('Error: ' + err.message);
            }
        }

        // Email settings form
        document.getElementById('emailForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const data = {
                enabled: document.getElementById('email-enabled').checked,
                smtp_host: document.getElementById('smtp-host').value,
                smtp_port: parseInt(document.getElementById('smtp-port').value) || 587,
                smtp_user: document.getElementById('smtp-user').value,
                smtp_password: document.getElementById('smtp-password').value,
                from_address: document.getElementById('from-address').value,
                from_name: document.getElementById('from-name').value
            };

            try {
                const resp = await fetch('/api/email/settings', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(data)
                });

                if (resp.ok) {
                    alert('Email settings saved!');
                } else {
                    alert('Failed to save settings');
                }
            } catch (err) {
                alert('Error: ' + err.message);
            }
        });

        // Preferences form
        document.getElementById('preferencesForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const data = {
                email_enabled: document.getElementById('pref-email').checked,
                webhook_enabled: document.getElementById('pref-webhook').checked,
                echo_enabled: document.getElementById('pref-echo').checked,
                priority_filter: document.getElementById('pref-priority').value,
                retention_days: parseInt(document.getElementById('pref-retention').value) || 7,
                muted_sources: []
            };

            try {
                const resp = await fetch('/api/preferences', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(data)
                });

                if (resp.ok) {
                    alert('Preferences saved!');
                } else {
                    alert('Failed to save preferences');
                }
            } catch (err) {
                alert('Error: ' + err.message);
            }
        });
    </script>
</body>
</html>
`
