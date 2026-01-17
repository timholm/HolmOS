package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// GetNotificationByID returns a notification by ID
func (h *NotificationHub) GetNotificationByID(id string) *UnifiedNotification {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for i := range h.notifications {
		if h.notifications[i].ID == id {
			return &h.notifications[i]
		}
	}
	return nil
}

// DeleteNotification removes a notification by ID
func (h *NotificationHub) DeleteNotification(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i, n := range h.notifications {
		if n.ID == id {
			h.notifications = append(h.notifications[:i], h.notifications[i+1:]...)
			return true
		}
	}
	return false
}

// UpdateNotificationStatus updates the status of a notification
func (h *NotificationHub) UpdateNotificationStatus(id, status string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i := range h.notifications {
		if h.notifications[i].ID == id {
			h.notifications[i].Status = status
			now := time.Now()
			h.notifications[i].ProcessedAt = &now
			return true
		}
	}
	return false
}

// ClearNotifications removes all notifications matching the filter
func (h *NotificationHub) ClearNotifications(source, priority, status string) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	if source == "" && priority == "" && status == "" {
		count := len(h.notifications)
		h.notifications = make([]UnifiedNotification, 0)
		return count
	}

	count := 0
	filtered := make([]UnifiedNotification, 0)
	for _, n := range h.notifications {
		keep := true
		if source != "" && source != "all" && n.Source == source {
			keep = false
		}
		if priority != "" && priority != "all" && n.Priority == priority {
			keep = false
		}
		if status != "" && status != "all" && n.Status == status {
			keep = false
		}
		if keep {
			filtered = append(filtered, n)
		} else {
			count++
		}
	}
	h.notifications = filtered
	return count
}

// GetWebhookByID returns a webhook by ID
func (h *NotificationHub) GetWebhookByID(id string) *WebhookConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if wh, exists := h.webhooks[id]; exists {
		copy := *wh
		copy.Secret = "" // Don't expose secret
		return &copy
	}
	return nil
}

// UpdateWebhook updates an existing webhook
func (h *NotificationHub) UpdateWebhook(id string, update WebhookConfig) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if wh, exists := h.webhooks[id]; exists {
		if update.Name != "" {
			wh.Name = update.Name
		}
		if update.URL != "" {
			wh.URL = update.URL
		}
		if len(update.Events) > 0 {
			wh.Events = update.Events
		}
		if update.Secret != "" {
			wh.Secret = update.Secret
		}
		wh.Active = update.Active
		return true
	}
	return false
}

// TestWebhook sends a test payload to a webhook
func (h *NotificationHub) TestWebhook(id string) error {
	h.mu.RLock()
	wh, exists := h.webhooks[id]
	h.mu.RUnlock()

	if !exists {
		return nil
	}

	payload := map[string]interface{}{
		"event": "test",
		"payload": map[string]interface{}{
			"message":   "This is a test notification from Notification Hub",
			"timestamp": time.Now().UTC(),
			"webhook":   wh.Name,
		},
	}

	jsonData, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("POST", wh.URL, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", "test")
	if wh.Secret != "" {
		req.Header.Set("X-Webhook-Secret", wh.Secret)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetEndpoints returns the service endpoints configuration
func (h *NotificationHub) GetEndpoints() ServiceEndpoints {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.endpoints
}

// UpdateEndpoints updates the service endpoints configuration
func (h *NotificationHub) UpdateEndpoints(endpoints ServiceEndpoints) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if endpoints.NotificationQueue != "" {
		h.endpoints.NotificationQueue = endpoints.NotificationQueue
	}
	if endpoints.NotificationEmail != "" {
		h.endpoints.NotificationEmail = endpoints.NotificationEmail
	}
	if endpoints.NotificationWebhook != "" {
		h.endpoints.NotificationWebhook = endpoints.NotificationWebhook
	}
	if endpoints.Echo != "" {
		h.endpoints.Echo = endpoints.Echo
	}
}

// ResendNotification resends a notification to all configured channels
func (h *NotificationHub) ResendNotification(id string) (*UnifiedNotification, error) {
	n := h.GetNotificationByID(id)
	if n == nil {
		return nil, nil
	}

	// Create a copy for resending
	resend := *n
	resend.Status = "pending"
	resend.ProcessedAt = nil

	// Send through all channels
	err := h.SendNotification(resend)
	return &resend, err
}

// HTTP Handlers for new endpoints

// getNotificationByIDHandler handles GET /api/notifications/{id}
func getNotificationByIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	notification := hub.GetNotificationByID(id)
	if notification == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "notification not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notification)
}

// deleteNotificationHandler handles DELETE /api/notifications/{id}
func deleteNotificationHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if !hub.DeleteNotification(id) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "notification not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "notification deleted", "id": id})
}

// updateNotificationStatusHandler handles PUT /api/notifications/{id}/status
func updateNotificationStatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	validStatuses := map[string]bool{
		"pending":   true,
		"sent":      true,
		"delivered": true,
		"failed":    true,
		"read":      true,
	}
	if !validStatuses[body.Status] {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid status value"})
		return
	}

	if !hub.UpdateNotificationStatus(id, body.Status) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "notification not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "status updated", "id": id, "status": body.Status})
}

// clearNotificationsHandler handles DELETE /api/notifications
func clearNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	priority := r.URL.Query().Get("priority")
	status := r.URL.Query().Get("status")

	count := hub.ClearNotifications(source, priority, status)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "notifications cleared",
		"count":   count,
	})
}

// getWebhookByIDHandler handles GET /api/webhooks/{id}
func getWebhookByIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	webhook := hub.GetWebhookByID(id)
	if webhook == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "webhook not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhook)
}

// updateWebhookHandler handles PUT /api/webhooks/{id}
func updateWebhookHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var update WebhookConfig
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if !hub.UpdateWebhook(id, update) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "webhook not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "webhook updated", "id": id})
}

// testWebhookHandler handles POST /api/webhooks/{id}/test
func testWebhookHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	webhook := hub.GetWebhookByID(id)
	if webhook == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "webhook not found"})
		return
	}

	if err := hub.TestWebhook(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "webhook test failed", "details": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "webhook test sent", "id": id})
}

// getEndpointsHandler handles GET /api/endpoints
func getEndpointsHandler(w http.ResponseWriter, r *http.Request) {
	endpoints := hub.GetEndpoints()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(endpoints)
}

// updateEndpointsHandler handles PUT /api/endpoints
func updateEndpointsHandler(w http.ResponseWriter, r *http.Request) {
	var endpoints ServiceEndpoints
	if err := json.NewDecoder(r.Body).Decode(&endpoints); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	hub.UpdateEndpoints(endpoints)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "endpoints updated"})
}

// resendNotificationHandler handles POST /api/notifications/{id}/resend
func resendNotificationHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	notification, err := hub.ResendNotification(id)
	if notification == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "notification not found"})
		return
	}

	response := map[string]interface{}{
		"message":      "notification resent",
		"notification": notification,
	}
	if err != nil {
		response["warning"] = "some channels may have failed: " + err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RegisterAPIRoutes registers all the new API endpoints
func RegisterAPIRoutes(r *mux.Router) {
	// Single notification operations
	r.HandleFunc("/api/notifications/{id}", getNotificationByIDHandler).Methods("GET")
	r.HandleFunc("/api/notifications/{id}", deleteNotificationHandler).Methods("DELETE")
	r.HandleFunc("/api/notifications/{id}/status", updateNotificationStatusHandler).Methods("PUT")
	r.HandleFunc("/api/notifications/{id}/resend", resendNotificationHandler).Methods("POST")

	// Bulk notification operations
	r.HandleFunc("/api/notifications/clear", clearNotificationsHandler).Methods("DELETE", "POST")

	// Single webhook operations
	r.HandleFunc("/api/webhooks/{id}", getWebhookByIDHandler).Methods("GET")
	r.HandleFunc("/api/webhooks/{id}", updateWebhookHandler).Methods("PUT")
	r.HandleFunc("/api/webhooks/{id}/test", testWebhookHandler).Methods("POST")

	// Service endpoints configuration
	r.HandleFunc("/api/endpoints", getEndpointsHandler).Methods("GET")
	r.HandleFunc("/api/endpoints", updateEndpointsHandler).Methods("PUT", "POST")
}
