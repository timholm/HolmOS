package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultMaxLogEntries  = 50000
	logCollectionInterval = 30 * time.Second
	scribeTagline         = "It's all in the records"
	primaryNamespace      = "holm"
)

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Namespace string    `json:"namespace"`
	Pod       string    `json:"pod"`
	Container string    `json:"container"`
	Message   string    `json:"message"`
	Level     string    `json:"level"`
	Size      int       `json:"size"` // bytes
}

type LogStore struct {
	mu          sync.RWMutex
	entries     []LogEntry
	subscribers []chan LogEntry
	subMu       sync.RWMutex

	// Retention settings
	retentionMu    sync.RWMutex
	maxEntries     int
	retentionHours int

	// Volume stats
	statsMu        sync.RWMutex
	totalBytes     int64
	bytesPerPod    map[string]int64
	bytesPerNs     map[string]int64
	entriesPerHour map[string]int // key: "2006-01-02-15"
}

type StatsResponse struct {
	Agent           string            `json:"agent"`
	Tagline         string            `json:"tagline"`
	TotalEntries    int               `json:"total_entries"`
	Namespaces      map[string]int    `json:"namespaces"`
	Pods            map[string]int    `json:"pods"`
	Levels          map[string]int    `json:"levels"`
	VolumeStats     VolumeStats       `json:"volume_stats"`
	RetentionConfig RetentionConfig   `json:"retention_config"`
}

type VolumeStats struct {
	TotalBytes     int64            `json:"total_bytes"`
	TotalBytesHR   string           `json:"total_bytes_human"`
	BytesPerPod    map[string]int64 `json:"bytes_per_pod"`
	BytesPerNs     map[string]int64 `json:"bytes_per_namespace"`
	EntriesPerHour map[string]int   `json:"entries_per_hour"`
	AvgEntrySize   int64            `json:"avg_entry_size"`
}

type RetentionConfig struct {
	MaxEntries     int `json:"max_entries"`
	RetentionHours int `json:"retention_hours"`
}

type LogsResponse struct {
	Count      int        `json:"count"`
	Entries    []LogEntry `json:"entries"`
	ScribeSays string     `json:"scribe_says,omitempty"`
}

type ChatRequest struct {
	Message string `json:"message"`
}

type ChatResponse struct {
	Response string `json:"response"`
}

type RetentionRequest struct {
	MaxEntries     int `json:"max_entries"`
	RetentionHours int `json:"retention_hours"`
}

var (
	store       *LogStore
	clientset   *kubernetes.Clientset
	podLastSeen map[string]time.Time
	podMu       sync.RWMutex
)

func NewLogStore() *LogStore {
	return &LogStore{
		entries:        make([]LogEntry, 0, defaultMaxLogEntries),
		subscribers:    make([]chan LogEntry, 0),
		maxEntries:     defaultMaxLogEntries,
		retentionHours: 24, // Default 24 hour retention
		bytesPerPod:    make(map[string]int64),
		bytesPerNs:     make(map[string]int64),
		entriesPerHour: make(map[string]int),
	}
}

func (ls *LogStore) Add(entry LogEntry) {
	entry.Size = len(entry.Message)

	ls.mu.Lock()
	ls.entries = append(ls.entries, entry)

	// Apply retention by max entries
	ls.retentionMu.RLock()
	maxEntries := ls.maxEntries
	retentionHours := ls.retentionHours
	ls.retentionMu.RUnlock()

	if len(ls.entries) > maxEntries {
		ls.entries = ls.entries[len(ls.entries)-maxEntries:]
	}

	// Apply retention by time
	if retentionHours > 0 {
		cutoff := time.Now().Add(-time.Duration(retentionHours) * time.Hour)
		idx := 0
		for i, e := range ls.entries {
			if e.Timestamp.After(cutoff) {
				idx = i
				break
			}
		}
		if idx > 0 {
			ls.entries = ls.entries[idx:]
		}
	}
	ls.mu.Unlock()

	// Update volume stats
	ls.statsMu.Lock()
	ls.totalBytes += int64(entry.Size)
	ls.bytesPerPod[entry.Pod] += int64(entry.Size)
	ls.bytesPerNs[entry.Namespace] += int64(entry.Size)
	hourKey := entry.Timestamp.Format("2006-01-02-15")
	ls.entriesPerHour[hourKey]++
	ls.statsMu.Unlock()

	// Notify subscribers
	ls.subMu.RLock()
	for _, sub := range ls.subscribers {
		select {
		case sub <- entry:
		default:
		}
	}
	ls.subMu.RUnlock()
}

func (ls *LogStore) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 100)
	ls.subMu.Lock()
	ls.subscribers = append(ls.subscribers, ch)
	ls.subMu.Unlock()
	return ch
}

func (ls *LogStore) Unsubscribe(ch chan LogEntry) {
	ls.subMu.Lock()
	defer ls.subMu.Unlock()
	for i, sub := range ls.subscribers {
		if sub == ch {
			ls.subscribers = append(ls.subscribers[:i], ls.subscribers[i+1:]...)
			close(ch)
			break
		}
	}
}

func (ls *LogStore) GetAll() []LogEntry {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	result := make([]LogEntry, len(ls.entries))
	copy(result, ls.entries)
	return result
}

func (ls *LogStore) Search(query, namespace, level, pod string, limit int, useRegex bool) []LogEntry {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	var results []LogEntry
	var regex *regexp.Regexp
	var err error

	if useRegex && query != "" {
		regex, err = regexp.Compile("(?i)" + query)
		if err != nil {
			// Fall back to literal search if regex is invalid
			regex = nil
		}
	}

	queryLower := strings.ToLower(query)

	for i := len(ls.entries) - 1; i >= 0 && (limit == 0 || len(results) < limit); i-- {
		entry := ls.entries[i]

		if namespace != "" && entry.Namespace != namespace {
			continue
		}
		if level != "" && entry.Level != level {
			continue
		}
		if pod != "" && !strings.Contains(entry.Pod, pod) {
			continue
		}

		if query != "" {
			if regex != nil {
				// Regex search
				if !regex.MatchString(entry.Message) && !regex.MatchString(entry.Pod) {
					continue
				}
			} else {
				// Standard case-insensitive search
				if !strings.Contains(strings.ToLower(entry.Message), queryLower) &&
					!strings.Contains(strings.ToLower(entry.Pod), queryLower) {
					continue
				}
			}
		}

		results = append(results, entry)
	}

	return results
}

func (ls *LogStore) Stats() StatsResponse {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	namespaces := make(map[string]int)
	pods := make(map[string]int)
	levels := make(map[string]int)

	for _, entry := range ls.entries {
		namespaces[entry.Namespace]++
		pods[entry.Pod]++
		levels[entry.Level]++
	}

	ls.statsMu.RLock()
	volumeStats := VolumeStats{
		TotalBytes:     ls.totalBytes,
		TotalBytesHR:   formatBytes(ls.totalBytes),
		BytesPerPod:    copyInt64Map(ls.bytesPerPod),
		BytesPerNs:     copyInt64Map(ls.bytesPerNs),
		EntriesPerHour: copyIntMap(ls.entriesPerHour),
	}
	if len(ls.entries) > 0 {
		volumeStats.AvgEntrySize = ls.totalBytes / int64(len(ls.entries))
	}
	ls.statsMu.RUnlock()

	ls.retentionMu.RLock()
	retentionConfig := RetentionConfig{
		MaxEntries:     ls.maxEntries,
		RetentionHours: ls.retentionHours,
	}
	ls.retentionMu.RUnlock()

	return StatsResponse{
		Agent:           "Scribe",
		Tagline:         scribeTagline,
		TotalEntries:    len(ls.entries),
		Namespaces:      namespaces,
		Pods:            pods,
		Levels:          levels,
		VolumeStats:     volumeStats,
		RetentionConfig: retentionConfig,
	}
}

func (ls *LogStore) Namespaces() []string {
	stats := ls.Stats()
	namespaces := make([]string, 0, len(stats.Namespaces))
	for ns := range stats.Namespaces {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)
	return namespaces
}

func (ls *LogStore) Pods() []string {
	stats := ls.Stats()
	pods := make([]string, 0, len(stats.Pods))
	for pod := range stats.Pods {
		pods = append(pods, pod)
	}
	sort.Strings(pods)
	return pods
}

func (ls *LogStore) SetRetention(maxEntries, retentionHours int) {
	ls.retentionMu.Lock()
	defer ls.retentionMu.Unlock()

	if maxEntries > 0 {
		ls.maxEntries = maxEntries
	}
	if retentionHours >= 0 {
		ls.retentionHours = retentionHours
	}

	// Apply new retention immediately
	go ls.applyRetention()
}

func (ls *LogStore) applyRetention() {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.retentionMu.RLock()
	maxEntries := ls.maxEntries
	retentionHours := ls.retentionHours
	ls.retentionMu.RUnlock()

	// Apply max entries
	if len(ls.entries) > maxEntries {
		ls.entries = ls.entries[len(ls.entries)-maxEntries:]
	}

	// Apply time-based retention
	if retentionHours > 0 {
		cutoff := time.Now().Add(-time.Duration(retentionHours) * time.Hour)
		idx := 0
		for i, e := range ls.entries {
			if e.Timestamp.After(cutoff) {
				idx = i
				break
			}
		}
		if idx > 0 {
			ls.entries = ls.entries[idx:]
		}
	}
}

func copyInt64Map(m map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func copyIntMap(m map[string]int) map[string]int {
	result := make(map[string]int, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func detectLogLevel(message string) string {
	msgUpper := strings.ToUpper(message)

	// Check for explicit level markers
	if strings.Contains(msgUpper, "ERROR") || strings.Contains(msgUpper, "FATAL") ||
		strings.Contains(msgUpper, "PANIC") || strings.Contains(msgUpper, "EXCEPTION") {
		return "ERROR"
	}
	if strings.Contains(msgUpper, "WARN") {
		return "WARN"
	}
	if strings.Contains(msgUpper, "DEBUG") || strings.Contains(msgUpper, "TRACE") {
		return "DEBUG"
	}
	return "INFO"
}

func collectPodLogs(ctx context.Context) {
	ticker := time.NewTicker(logCollectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectAllPodLogs()
		}
	}
}

func collectAllPodLogs() {
	// Primary focus: holm namespace
	collectNamespacePodLogs(primaryNamespace)

	// Also collect from other namespaces for completeness
	namespaces, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("Error listing namespaces: %v", err)
		return
	}

	for _, ns := range namespaces.Items {
		if ns.Name == primaryNamespace {
			continue // Already collected
		}
		// Skip system namespaces unless they have issues
		if ns.Name == "kube-system" || ns.Name == "kube-public" || ns.Name == "kube-node-lease" {
			continue
		}
		collectNamespacePodLogs(ns.Name)
	}
}

func collectNamespacePodLogs(namespace string) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return
	}

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			go collectContainerLogs(namespace, pod.Name, container.Name)
		}
	}
}

func collectContainerLogs(namespace, podName, containerName string) {
	podKey := fmt.Sprintf("%s/%s/%s", namespace, podName, containerName)

	podMu.RLock()
	lastSeen, exists := podLastSeen[podKey]
	podMu.RUnlock()

	sinceSeconds := int64(300) // 5 minutes default
	if exists {
		elapsed := time.Since(lastSeen)
		if elapsed < 5*time.Minute {
			sinceSeconds = int64(elapsed.Seconds()) + 5
		}
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container:    containerName,
		SinceSeconds: &sinceSeconds,
		Timestamps:   true,
		Follow:       true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	stream, err := req.Stream(ctx)
	if err != nil {
		return
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse timestamp from beginning of line
		var timestamp time.Time
		var message string

		if len(line) > 30 && line[10] == 'T' {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				if t, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
					timestamp = t
					message = parts[1]
				}
			}
		}

		if timestamp.IsZero() {
			timestamp = time.Now()
			message = line
		}

		entry := LogEntry{
			Timestamp: timestamp,
			Namespace: namespace,
			Pod:       podName,
			Container: containerName,
			Message:   message,
			Level:     detectLogLevel(message),
		}

		store.Add(entry)

		podMu.Lock()
		podLastSeen[podKey] = timestamp
		podMu.Unlock()
	}
}

// HTTP Handlers

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"agent":  "Scribe",
	})
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(store.Stats())
}

func handleNamespaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(store.Namespaces())
}

func handlePods(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(store.Pods())
}

// CreateLogRequest represents a manual log entry creation request
type CreateLogRequest struct {
	Timestamp time.Time `json:"timestamp,omitempty"`
	Namespace string    `json:"namespace,omitempty"`
	Pod       string    `json:"pod,omitempty"`
	Container string    `json:"container,omitempty"`
	Message   string    `json:"message"`
	Level     string    `json:"level,omitempty"`
	Source    string    `json:"source,omitempty"`
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Handle POST - create a new log entry
	if r.Method == http.MethodPost {
		var req CreateLogRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Message == "" {
			http.Error(w, "Message is required", http.StatusBadRequest)
			return
		}

		// Set defaults
		if req.Timestamp.IsZero() {
			req.Timestamp = time.Now()
		}
		if req.Namespace == "" {
			req.Namespace = "manual"
		}
		if req.Pod == "" {
			req.Pod = "api"
		}
		if req.Container == "" {
			req.Container = "manual-entry"
		}
		if req.Level == "" {
			req.Level = detectLogLevel(req.Message)
		}
		if req.Source != "" {
			req.Pod = req.Source
		}

		entry := LogEntry{
			Timestamp: req.Timestamp,
			Namespace: req.Namespace,
			Pod:       req.Pod,
			Container: req.Container,
			Message:   req.Message,
			Level:     strings.ToUpper(req.Level),
		}

		store.Add(entry)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "created",
			"message": "Log entry recorded in the chronicles",
			"entry":   entry,
		})
		return
	}

	// Handle GET - list logs
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	entries := store.Search("", "", "", "", limit, false)
	json.NewEncoder(w).Encode(LogsResponse{
		Count:   len(entries),
		Entries: entries,
	})
}

func handleLogsSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query().Get("q")
	namespace := r.URL.Query().Get("namespace")
	level := r.URL.Query().Get("level")
	pod := r.URL.Query().Get("pod")
	useRegex := r.URL.Query().Get("regex") == "true"

	limit := 500
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	entries := store.Search(query, namespace, level, pod, limit, useRegex)

	// Generate Scribe commentary
	var scribeSays string
	if len(entries) == 0 {
		scribeSays = "The chronicles hold no such records. Perhaps the knowledge you seek lies beyond my scrolls."
	} else if level == "ERROR" {
		scribeSays = fmt.Sprintf("I have uncovered %d troubled entries in the annals. These errors speak of disturbances in the realm.", len(entries))
	} else if useRegex && query != "" {
		scribeSays = fmt.Sprintf("Your pattern '%s' yields %d records. The regex reveals what simple searches cannot.", query, len(entries))
	} else if query != "" {
		scribeSays = fmt.Sprintf("Your query yields %d records. It's all in the records, and I have found what you seek.", len(entries))
	}

	json.NewEncoder(w).Encode(LogsResponse{
		Count:      len(entries),
		Entries:    entries,
		ScribeSays: scribeSays,
	})
}

func handleLogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Optional filters
	namespace := r.URL.Query().Get("namespace")
	level := r.URL.Query().Get("level")
	pod := r.URL.Query().Get("pod")
	query := r.URL.Query().Get("q")
	useRegex := r.URL.Query().Get("regex") == "true"

	var regex *regexp.Regexp
	if useRegex && query != "" {
		var err error
		regex, err = regexp.Compile("(?i)" + query)
		if err != nil {
			regex = nil
		}
	}

	ch := store.Subscribe()
	defer store.Unsubscribe(ch)

	for {
		select {
		case entry := <-ch:
			if namespace != "" && entry.Namespace != namespace {
				continue
			}
			if level != "" && entry.Level != level {
				continue
			}
			if pod != "" && !strings.Contains(entry.Pod, pod) {
				continue
			}
			if query != "" {
				if regex != nil {
					if !regex.MatchString(entry.Message) && !regex.MatchString(entry.Pod) {
						continue
					}
				} else {
					queryLower := strings.ToLower(query)
					if !strings.Contains(strings.ToLower(entry.Message), queryLower) &&
						!strings.Contains(strings.ToLower(entry.Pod), queryLower) {
						continue
					}
				}
			}

			data, _ := json.Marshal(entry)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

func handleLogsExport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	query := r.URL.Query().Get("q")
	namespace := r.URL.Query().Get("namespace")
	level := r.URL.Query().Get("level")
	pod := r.URL.Query().Get("pod")
	useRegex := r.URL.Query().Get("regex") == "true"

	limit := 10000
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	entries := store.Search(query, namespace, level, pod, limit, useRegex)

	filename := fmt.Sprintf("scribe-logs-%s", time.Now().Format("2006-01-02-150405"))

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", filename))

		w.Write([]byte("timestamp,namespace,pod,container,level,message\n"))
		for _, e := range entries {
			// Escape CSV fields
			msg := strings.ReplaceAll(e.Message, "\"", "\"\"")
			line := fmt.Sprintf("\"%s\",\"%s\",\"%s\",\"%s\",\"%s\",\"%s\"\n",
				e.Timestamp.Format(time.RFC3339),
				e.Namespace, e.Pod, e.Container, e.Level, msg)
			w.Write([]byte(line))
		}

	case "txt":
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.txt", filename))

		for _, e := range entries {
			line := fmt.Sprintf("[%s] [%s] %s/%s [%s] %s\n",
				e.Timestamp.Format("2006-01-02 15:04:05"),
				e.Level, e.Namespace, e.Pod, e.Container, e.Message)
			w.Write([]byte(line))
		}

	default: // json
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", filename))
		json.NewEncoder(w).Encode(map[string]interface{}{
			"exported_at": time.Now(),
			"count":       len(entries),
			"entries":     entries,
		})
	}
}

func handleRetention(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodGet {
		store.retentionMu.RLock()
		config := RetentionConfig{
			MaxEntries:     store.maxEntries,
			RetentionHours: store.retentionHours,
		}
		store.retentionMu.RUnlock()
		json.NewEncoder(w).Encode(config)
		return
	}

	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		var req RetentionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		store.SetRetention(req.MaxEntries, req.RetentionHours)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "updated",
			"message": fmt.Sprintf("Retention set to %d entries, %d hours", req.MaxEntries, req.RetentionHours),
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleVolumeStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	store.statsMu.RLock()
	store.mu.RLock()
	entryCount := len(store.entries)
	store.mu.RUnlock()

	stats := VolumeStats{
		TotalBytes:     store.totalBytes,
		TotalBytesHR:   formatBytes(store.totalBytes),
		BytesPerPod:    copyInt64Map(store.bytesPerPod),
		BytesPerNs:     copyInt64Map(store.bytesPerNs),
		EntriesPerHour: copyIntMap(store.entriesPerHour),
	}
	if entryCount > 0 {
		stats.AvgEntrySize = store.totalBytes / int64(entryCount)
	}
	store.statsMu.RUnlock()

	json.NewEncoder(w).Encode(stats)
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	response := generateScribeResponse(req.Message)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ChatResponse{Response: response})
}

func generateScribeResponse(message string) string {
	msgLower := strings.ToLower(message)
	stats := store.Stats()

	// Pattern matching for common queries
	if strings.Contains(msgLower, "error") || strings.Contains(msgLower, "problem") {
		errorCount := stats.Levels["ERROR"]
		if errorCount > 0 {
			return fmt.Sprintf("The chronicles record %d errors in the realm. These troubles demand your attention. Use the ERROR filter to see them all. It's all in the records.", errorCount)
		}
		return "The scrolls show no errors at this time. The realm appears to be at peace."
	}

	if strings.Contains(msgLower, "how many") || strings.Contains(msgLower, "count") {
		return fmt.Sprintf("I have chronicled %d entries across %d namespaces and %d pods. Every whisper, every event - it's all in the records.",
			stats.TotalEntries, len(stats.Namespaces), len(stats.Pods))
	}

	if strings.Contains(msgLower, "warn") {
		warnCount := stats.Levels["WARN"]
		return fmt.Sprintf("The annals contain %d warnings. These are not yet errors, but signs of potential trouble ahead.", warnCount)
	}

	if strings.Contains(msgLower, "namespace") {
		namespaces := store.Namespaces()
		return fmt.Sprintf("I observe %d realms: %s. Each holds its own chronicles.", len(namespaces), strings.Join(namespaces, ", "))
	}

	if strings.Contains(msgLower, "retention") || strings.Contains(msgLower, "keep") || strings.Contains(msgLower, "storage") {
		return fmt.Sprintf("I currently retain up to %d entries for %d hours. You may adjust these settings in the retention panel.", stats.RetentionConfig.MaxEntries, stats.RetentionConfig.RetentionHours)
	}

	if strings.Contains(msgLower, "volume") || strings.Contains(msgLower, "size") || strings.Contains(msgLower, "bytes") {
		return fmt.Sprintf("The chronicles occupy %s of memory. The average entry consumes %d bytes.", stats.VolumeStats.TotalBytesHR, stats.VolumeStats.AvgEntrySize)
	}

	if strings.Contains(msgLower, "regex") || strings.Contains(msgLower, "pattern") {
		return "Enable the regex toggle to search with patterns. For example, 'error|warn' finds both errors and warnings, while 'pod-[0-9]+' finds numbered pods."
	}

	if strings.Contains(msgLower, "help") || strings.Contains(msgLower, "what can you") {
		return "I am Scribe, keeper of all logs. You may ask me about errors, warnings, namespaces, retention, volume, or the count of records. You may also search the chronicles using the search bar above. Enable regex for pattern matching. Use the filters to narrow your quest. And when you need a permanent record, export the logs to a file. It's all in the records."
	}

	if strings.Contains(msgLower, "export") || strings.Contains(msgLower, "download") {
		return "To preserve the chronicles, click the Export button. You may choose JSON, CSV, or plain text format. The current filters will apply to your export."
	}

	if strings.Contains(msgLower, "stream") || strings.Contains(msgLower, "live") || strings.Contains(msgLower, "real-time") || strings.Contains(msgLower, "tail") {
		return "Click the Live Tail button to witness events as they unfold. The chronicles update in real-time, capturing every moment. Stop the stream when you have seen enough."
	}

	// Default responses
	responses := []string{
		"I record all that transpires in this realm. What specific knowledge do you seek?",
		"The chronicles are vast. Perhaps filter by namespace, level, or search for specific terms?",
		"Every pod's whisper reaches my scrolls. Ask about errors, warnings, or specific services.",
		"It's all in the records. Tell me what you seek, and I shall find it.",
	}

	return responses[time.Now().UnixNano()%int64(len(responses))]
}

func handleUI(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("ui").Parse(uiTemplate))
	tmpl.Execute(w, map[string]string{
		"Tagline": scribeTagline,
	})
}

const uiTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Scribe - Log Aggregator | HolmOS</title>
    <style>
        :root {
            --ctp-rosewater: #f5e0dc;
            --ctp-flamingo: #f2cdcd;
            --ctp-pink: #f5c2e7;
            --ctp-mauve: #cba6f7;
            --ctp-red: #f38ba8;
            --ctp-maroon: #eba0ac;
            --ctp-peach: #fab387;
            --ctp-yellow: #f9e2af;
            --ctp-green: #a6e3a1;
            --ctp-teal: #94e2d5;
            --ctp-sky: #89dceb;
            --ctp-sapphire: #74c7ec;
            --ctp-blue: #89b4fa;
            --ctp-lavender: #b4befe;
            --ctp-text: #cdd6f4;
            --ctp-subtext1: #bac2de;
            --ctp-subtext0: #a6adc8;
            --ctp-overlay2: #9399b2;
            --ctp-overlay1: #7f849c;
            --ctp-overlay0: #6c7086;
            --ctp-surface2: #585b70;
            --ctp-surface1: #45475a;
            --ctp-surface0: #313244;
            --ctp-base: #1e1e2e;
            --ctp-mantle: #181825;
            --ctp-crust: #11111b;
        }

        * { margin: 0; padding: 0; box-sizing: border-box; }

        body {
            font-family: "JetBrains Mono", "Fira Code", monospace;
            background: var(--ctp-base);
            color: var(--ctp-text);
            min-height: 100vh;
        }

        .container { max-width: 1400px; margin: 0 auto; padding: 20px; }

        header {
            background: var(--ctp-mantle);
            border-bottom: 2px solid var(--ctp-surface0);
            padding: 20px 0;
            margin-bottom: 30px;
        }

        .header-content {
            display: flex;
            align-items: center;
            justify-content: space-between;
            max-width: 1400px;
            margin: 0 auto;
            padding: 0 20px;
        }

        .logo { display: flex; align-items: center; gap: 15px; }
        .logo-icon { font-size: 2.5rem; }
        .logo h1 { font-size: 2rem; color: var(--ctp-mauve); }
        .tagline { color: var(--ctp-subtext0); font-style: italic; }

        .stats-badge {
            background: var(--ctp-surface0);
            padding: 10px 20px;
            border-radius: 8px;
            display: flex;
            align-items: center;
            gap: 15px;
            flex-wrap: wrap;
        }

        .stat-item { text-align: center; min-width: 80px; }
        .stat-value { font-size: 1.3rem; font-weight: bold; color: var(--ctp-mauve); }
        .stat-label { font-size: 0.7rem; color: var(--ctp-subtext0); }

        .search-section {
            background: var(--ctp-mantle);
            padding: 20px;
            border-radius: 12px;
            margin-bottom: 20px;
        }

        .search-bar { display: flex; gap: 10px; margin-bottom: 15px; flex-wrap: wrap; }

        .search-input {
            flex: 1;
            min-width: 200px;
            padding: 12px 16px;
            background: var(--ctp-surface0);
            border: 1px solid var(--ctp-surface1);
            border-radius: 8px;
            color: var(--ctp-text);
            font-size: 1rem;
            font-family: inherit;
        }

        .search-input:focus {
            outline: none;
            border-color: var(--ctp-mauve);
        }

        .btn {
            padding: 12px 24px;
            background: var(--ctp-mauve);
            color: var(--ctp-crust);
            border: none;
            border-radius: 8px;
            font-weight: bold;
            cursor: pointer;
            transition: all 0.2s;
            font-family: inherit;
        }

        .btn:hover { background: var(--ctp-pink); }
        .btn-secondary { background: var(--ctp-surface1); color: var(--ctp-text); }
        .btn-secondary:hover { background: var(--ctp-surface2); }
        .btn-export { background: var(--ctp-teal); }
        .btn-export:hover { background: var(--ctp-green); }
        .btn-settings { background: var(--ctp-peach); }
        .btn-settings:hover { background: var(--ctp-yellow); }

        .filters { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; }

        .filter-select {
            padding: 10px 15px;
            background: var(--ctp-surface0);
            border: 1px solid var(--ctp-surface1);
            border-radius: 8px;
            color: var(--ctp-text);
            min-width: 150px;
            font-family: inherit;
        }

        .filter-select:focus { outline: none; border-color: var(--ctp-mauve); }

        .regex-toggle {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 10px 15px;
            background: var(--ctp-surface0);
            border-radius: 8px;
            cursor: pointer;
        }

        .regex-toggle input {
            width: 18px;
            height: 18px;
            accent-color: var(--ctp-mauve);
        }

        .regex-toggle.active {
            background: var(--ctp-mauve);
            color: var(--ctp-crust);
        }

        .agent-section {
            background: var(--ctp-mantle);
            padding: 20px;
            border-radius: 12px;
            margin-bottom: 20px;
            border: 1px solid var(--ctp-surface0);
        }

        .agent-header { display: flex; align-items: center; gap: 15px; margin-bottom: 15px; }
        .agent-avatar { font-size: 3rem; }
        .agent-info h2 { color: var(--ctp-mauve); }
        .agent-info p { color: var(--ctp-subtext0); font-style: italic; }

        .chat-messages {
            background: var(--ctp-base);
            border-radius: 8px;
            padding: 20px;
            max-height: 200px;
            overflow-y: auto;
            margin-bottom: 15px;
        }

        .chat-message {
            padding: 10px 15px;
            margin-bottom: 10px;
            border-radius: 8px;
            line-height: 1.6;
        }

        .chat-message.agent {
            background: var(--ctp-surface0);
            border-left: 3px solid var(--ctp-mauve);
        }

        .chat-message.user {
            background: var(--ctp-surface1);
            border-left: 3px solid var(--ctp-sapphire);
        }

        .chat-input-row { display: flex; gap: 10px; }

        .logs-section {
            background: var(--ctp-mantle);
            border-radius: 12px;
            border: 1px solid var(--ctp-surface0);
            overflow: hidden;
            margin-bottom: 20px;
        }

        .logs-header {
            background: var(--ctp-surface0);
            padding: 15px 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-wrap: wrap;
            gap: 10px;
        }

        .logs-header h3 { color: var(--ctp-lavender); }
        .logs-controls { display: flex; gap: 10px; flex-wrap: wrap; }

        .logs-container { height: 500px; overflow-y: auto; padding: 10px; }

        .log-entry {
            display: grid;
            grid-template-columns: 180px 120px 200px 1fr;
            gap: 15px;
            padding: 12px 15px;
            border-bottom: 1px solid var(--ctp-surface0);
            font-size: 0.9rem;
            transition: background 0.2s;
        }

        .log-entry:hover { background: var(--ctp-surface0); }

        .log-timestamp { color: var(--ctp-subtext0); font-size: 0.85rem; }

        .log-source { display: flex; flex-direction: column; gap: 2px; }
        .log-namespace { color: var(--ctp-sapphire); font-weight: bold; font-size: 0.8rem; }
        .log-pod {
            color: var(--ctp-teal);
            font-size: 0.85rem;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }

        .log-level {
            padding: 3px 10px;
            border-radius: 4px;
            font-size: 0.75rem;
            font-weight: bold;
            text-align: center;
            height: fit-content;
        }

        .log-level.ERROR { background: var(--ctp-red); color: var(--ctp-crust); }
        .log-level.WARN { background: var(--ctp-yellow); color: var(--ctp-crust); }
        .log-level.INFO { background: var(--ctp-green); color: var(--ctp-crust); }
        .log-level.DEBUG { background: var(--ctp-overlay0); color: var(--ctp-crust); }

        .log-message { color: var(--ctp-text); word-break: break-word; line-height: 1.4; }
        .log-message.error { color: var(--ctp-red); }

        .export-dropdown, .settings-dropdown {
            position: relative;
            display: inline-block;
        }

        .export-menu, .settings-menu {
            display: none;
            position: absolute;
            right: 0;
            top: 100%;
            background: var(--ctp-surface0);
            border-radius: 8px;
            min-width: 200px;
            z-index: 100;
            box-shadow: 0 4px 12px rgba(0,0,0,0.3);
        }

        .export-menu.show, .settings-menu.show { display: block; }

        .export-menu a, .settings-menu-item {
            display: block;
            padding: 10px 15px;
            color: var(--ctp-text);
            text-decoration: none;
            transition: background 0.2s;
            cursor: pointer;
        }

        .export-menu a:hover, .settings-menu-item:hover { background: var(--ctp-surface1); }

        .settings-menu-content {
            padding: 15px;
        }

        .settings-field {
            margin-bottom: 15px;
        }

        .settings-field label {
            display: block;
            margin-bottom: 5px;
            color: var(--ctp-subtext1);
            font-size: 0.85rem;
        }

        .settings-field input {
            width: 100%;
            padding: 8px 12px;
            background: var(--ctp-surface1);
            border: 1px solid var(--ctp-surface2);
            border-radius: 6px;
            color: var(--ctp-text);
            font-family: inherit;
        }

        .settings-field input:focus {
            outline: none;
            border-color: var(--ctp-mauve);
        }

        .volume-stats {
            background: var(--ctp-mantle);
            padding: 20px;
            border-radius: 12px;
            border: 1px solid var(--ctp-surface0);
        }

        .volume-stats h3 {
            color: var(--ctp-lavender);
            margin-bottom: 15px;
        }

        .volume-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
        }

        .volume-card {
            background: var(--ctp-surface0);
            padding: 15px;
            border-radius: 8px;
        }

        .volume-card-title {
            color: var(--ctp-subtext0);
            font-size: 0.85rem;
            margin-bottom: 8px;
        }

        .volume-card-value {
            font-size: 1.5rem;
            font-weight: bold;
            color: var(--ctp-mauve);
        }

        .volume-bar {
            height: 8px;
            background: var(--ctp-surface1);
            border-radius: 4px;
            overflow: hidden;
            margin-top: 10px;
        }

        .volume-bar-fill {
            height: 100%;
            background: var(--ctp-mauve);
            border-radius: 4px;
            transition: width 0.3s;
        }

        ::-webkit-scrollbar { width: 8px; }
        ::-webkit-scrollbar-track { background: var(--ctp-surface0); }
        ::-webkit-scrollbar-thumb { background: var(--ctp-surface2); border-radius: 4px; }
        ::-webkit-scrollbar-thumb:hover { background: var(--ctp-overlay0); }

        .empty-state { text-align: center; padding: 60px 20px; color: var(--ctp-subtext0); }
        .empty-state-icon { font-size: 4rem; margin-bottom: 20px; }

        .loading { display: flex; justify-content: center; padding: 40px; }

        .spinner {
            width: 40px;
            height: 40px;
            border: 3px solid var(--ctp-surface1);
            border-top-color: var(--ctp-mauve);
            border-radius: 50%;
            animation: spin 1s linear infinite;
        }

        @keyframes spin { to { transform: rotate(360deg); } }
        @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }

        @media (max-width: 768px) {
            .log-entry { grid-template-columns: 1fr; gap: 8px; }
            .header-content { flex-direction: column; gap: 20px; }
            .search-bar { flex-direction: column; }
            .filters { flex-direction: column; }
            .logs-controls { justify-content: center; }
        }
    </style>
</head>
<body>
    <header>
        <div class="header-content">
            <div class="logo">
                <span class="logo-icon">&#128220;</span>
                <div>
                    <h1>Scribe</h1>
                    <p class="tagline">{{.Tagline}}</p>
                </div>
            </div>
            <div class="stats-badge">
                <div class="stat-item">
                    <div class="stat-value" id="total-logs">-</div>
                    <div class="stat-label">Total Entries</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value" id="error-count">-</div>
                    <div class="stat-label">Errors</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value" id="namespace-count">-</div>
                    <div class="stat-label">Namespaces</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value" id="pod-count">-</div>
                    <div class="stat-label">Pods</div>
                </div>
                <div class="stat-item">
                    <div class="stat-value" id="volume-size">-</div>
                    <div class="stat-label">Volume</div>
                </div>
            </div>
        </div>
    </header>

    <div class="container">
        <section class="search-section">
            <div class="search-bar">
                <input type="text" class="search-input" id="search-query"
                       placeholder="Search the chronicles... (e.g., error, warning, pod name, or regex pattern)"
                       onkeypress="if(event.key==='Enter') searchLogs()">
                <label class="regex-toggle" id="regex-toggle">
                    <input type="checkbox" id="use-regex" onchange="toggleRegex()">
                    <span>Regex</span>
                </label>
                <button class="btn" onclick="searchLogs()">Search</button>
                <button class="btn btn-secondary" onclick="clearSearch()">Clear</button>
            </div>
            <div class="filters">
                <select class="filter-select" id="filter-namespace" onchange="searchLogs()">
                    <option value="">All Namespaces</option>
                </select>
                <select class="filter-select" id="filter-level" onchange="searchLogs()">
                    <option value="">All Levels</option>
                    <option value="ERROR">ERROR</option>
                    <option value="WARN">WARN</option>
                    <option value="INFO">INFO</option>
                    <option value="DEBUG">DEBUG</option>
                </select>
                <select class="filter-select" id="filter-pod" onchange="searchLogs()">
                    <option value="">All Pods</option>
                </select>
            </div>
        </section>

        <section class="agent-section">
            <div class="agent-header">
                <div class="agent-avatar">&#128220;</div>
                <div class="agent-info">
                    <h2>Scribe</h2>
                    <p>{{.Tagline}}</p>
                </div>
            </div>
            <div class="chat-messages" id="chat-messages">
                <div class="chat-message agent">Greetings, seeker of truth. I am Scribe, keeper of the chronicles. Every event, every whisper from your pods - I have recorded them all. What knowledge do you seek from the annals? It's all in the records.</div>
            </div>
            <div class="chat-input-row">
                <input type="text" class="search-input" id="chat-input"
                       placeholder="Ask Scribe about your logs..."
                       onkeypress="if(event.key==='Enter') sendChat()">
                <button class="btn" onclick="sendChat()">Ask</button>
            </div>
        </section>

        <section class="logs-section">
            <div class="logs-header">
                <h3>&#128203; Log Chronicle</h3>
                <div class="logs-controls">
                    <button class="btn btn-secondary" onclick="toggleStream()" id="stream-btn">
                        &#9654; Live Tail
                    </button>
                    <button class="btn btn-secondary" onclick="refreshLogs()">
                        &#8635; Refresh
                    </button>
                    <div class="export-dropdown">
                        <button class="btn btn-export" onclick="toggleExportMenu()">
                            &#128190; Export
                        </button>
                        <div class="export-menu" id="export-menu">
                            <a href="#" onclick="exportLogs('json')">JSON</a>
                            <a href="#" onclick="exportLogs('csv')">CSV</a>
                            <a href="#" onclick="exportLogs('txt')">Plain Text</a>
                        </div>
                    </div>
                    <div class="settings-dropdown">
                        <button class="btn btn-settings" onclick="toggleSettingsMenu()">
                            &#9881; Retention
                        </button>
                        <div class="settings-menu" id="settings-menu">
                            <div class="settings-menu-content">
                                <div class="settings-field">
                                    <label>Max Entries</label>
                                    <input type="number" id="retention-max-entries" placeholder="50000" min="1000" max="500000">
                                </div>
                                <div class="settings-field">
                                    <label>Retention Hours (0 = unlimited)</label>
                                    <input type="number" id="retention-hours" placeholder="24" min="0" max="720">
                                </div>
                                <button class="btn" onclick="saveRetention()" style="width: 100%;">Save</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="logs-container" id="logs-container">
                <div class="loading"><div class="spinner"></div></div>
            </div>
        </section>

        <section class="volume-stats">
            <h3>&#128202; Log Volume Statistics</h3>
            <div class="volume-grid">
                <div class="volume-card">
                    <div class="volume-card-title">Total Volume</div>
                    <div class="volume-card-value" id="vol-total">-</div>
                </div>
                <div class="volume-card">
                    <div class="volume-card-title">Average Entry Size</div>
                    <div class="volume-card-value" id="vol-avg">-</div>
                </div>
                <div class="volume-card">
                    <div class="volume-card-title">Retention Limit</div>
                    <div class="volume-card-value" id="vol-retention">-</div>
                    <div class="volume-bar">
                        <div class="volume-bar-fill" id="vol-bar" style="width: 0%"></div>
                    </div>
                </div>
                <div class="volume-card">
                    <div class="volume-card-title">Hours Retained</div>
                    <div class="volume-card-value" id="vol-hours">-</div>
                </div>
            </div>
        </section>
    </div>

    <script>
        let streaming = false;
        let eventSource = null;
        const API_TIMEOUT = 10000; // 10 second timeout

        // Fetch with timeout wrapper
        async function fetchWithTimeout(url, options = {}) {
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), API_TIMEOUT);

            try {
                const response = await fetch(url, {
                    ...options,
                    signal: controller.signal
                });
                clearTimeout(timeoutId);
                return response;
            } catch (error) {
                clearTimeout(timeoutId);
                if (error.name === 'AbortError') {
                    throw new Error('Request timed out after ' + (API_TIMEOUT/1000) + ' seconds');
                }
                throw error;
            }
        }

        function showError(container, message) {
            container.innerHTML = '<div class="empty-state" style="color: var(--ctp-red);"><div class="empty-state-icon">&#9888;</div><p>' + escapeHtml(message) + '</p><button class="btn btn-secondary" onclick="refreshLogs()" style="margin-top: 15px;">Try Again</button></div>';
        }

        document.addEventListener('DOMContentLoaded', () => {
            loadStats();
            loadNamespaces();
            loadLogs();
            loadRetentionSettings();
        });

        // Close menus when clicking outside
        document.addEventListener('click', (e) => {
            if (!e.target.closest('.export-dropdown')) {
                document.getElementById('export-menu').classList.remove('show');
            }
            if (!e.target.closest('.settings-dropdown')) {
                document.getElementById('settings-menu').classList.remove('show');
            }
        });

        async function loadStats() {
            try {
                const res = await fetchWithTimeout('/api/stats');
                const data = await res.json();
                document.getElementById('total-logs').textContent = formatNumber(data.total_entries || 0);
                document.getElementById('error-count').textContent = formatNumber(data.levels?.ERROR || 0);
                document.getElementById('namespace-count').textContent = Object.keys(data.namespaces || {}).length;
                document.getElementById('pod-count').textContent = Object.keys(data.pods || {}).length;
                document.getElementById('volume-size').textContent = data.volume_stats?.total_bytes_human || '-';

                // Populate pod filter
                const podSelect = document.getElementById('filter-pod');
                const currentPod = podSelect.value;
                podSelect.innerHTML = '<option value="">All Pods</option>';
                Object.keys(data.pods || {}).sort().forEach(pod => {
                    const opt = document.createElement('option');
                    opt.value = pod;
                    opt.textContent = pod;
                    podSelect.appendChild(opt);
                });
                podSelect.value = currentPod;

                // Update volume stats
                updateVolumeStats(data);
            } catch (e) {
                console.error('Stats error:', e);
            }
        }

        function updateVolumeStats(data) {
            document.getElementById('vol-total').textContent = data.volume_stats?.total_bytes_human || '-';
            document.getElementById('vol-avg').textContent = (data.volume_stats?.avg_entry_size || 0) + ' B';
            document.getElementById('vol-retention').textContent = formatNumber(data.retention_config?.max_entries || 50000);
            document.getElementById('vol-hours').textContent = (data.retention_config?.retention_hours || 24) + 'h';

            const used = data.total_entries || 0;
            const max = data.retention_config?.max_entries || 50000;
            const pct = Math.min((used / max) * 100, 100);
            document.getElementById('vol-bar').style.width = pct + '%';
        }

        function formatNumber(n) {
            if (n >= 1000000) return (n/1000000).toFixed(1) + 'M';
            if (n >= 1000) return (n/1000).toFixed(1) + 'K';
            return n.toString();
        }

        async function loadNamespaces() {
            try {
                const res = await fetchWithTimeout('/api/namespaces');
                const namespaces = await res.json();
                const select = document.getElementById('filter-namespace');
                namespaces.forEach(ns => {
                    const opt = document.createElement('option');
                    opt.value = ns;
                    opt.textContent = ns;
                    select.appendChild(opt);
                });
            } catch (e) {
                console.error('Namespaces error:', e);
            }
        }

        async function loadRetentionSettings() {
            try {
                const res = await fetchWithTimeout('/api/retention');
                const data = await res.json();
                document.getElementById('retention-max-entries').value = data.max_entries;
                document.getElementById('retention-hours').value = data.retention_hours;
            } catch (e) {
                console.error('Retention settings error:', e);
            }
        }

        async function saveRetention() {
            const maxEntries = parseInt(document.getElementById('retention-max-entries').value) || 50000;
            const retentionHours = parseInt(document.getElementById('retention-hours').value) || 24;

            try {
                const res = await fetchWithTimeout('/api/retention', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ max_entries: maxEntries, retention_hours: retentionHours })
                });
                const data = await res.json();
                addChatMessage('Retention settings updated: ' + data.message, 'agent');
                document.getElementById('settings-menu').classList.remove('show');
                loadStats();
            } catch (e) {
                addChatMessage('Failed to update retention settings: ' + (e.message || 'timeout'), 'agent');
            }
        }

        async function loadLogs() {
            const container = document.getElementById('logs-container');
            container.innerHTML = '<div class="loading"><div class="spinner"></div></div>';

            try {
                const res = await fetchWithTimeout('/api/logs');
                if (!res.ok) {
                    throw new Error('Server returned ' + res.status);
                }
                const data = await res.json();
                if (data.entries && data.entries.length > 0) {
                    renderLogs(data.entries);
                } else {
                    container.innerHTML = '<div class="empty-state"><div class="empty-state-icon">&#128220;</div><p>The chronicles await their first entry...</p></div>';
                }
            } catch (e) {
                console.error('Load logs error:', e);
                showError(container, e.message || 'Failed to load logs');
            }
        }

        async function searchLogs() {
            const query = document.getElementById('search-query').value;
            const namespace = document.getElementById('filter-namespace').value;
            const level = document.getElementById('filter-level').value;
            const pod = document.getElementById('filter-pod').value;
            const useRegex = document.getElementById('use-regex').checked;

            const params = new URLSearchParams();
            if (query) params.set('q', query);
            if (namespace) params.set('namespace', namespace);
            if (level) params.set('level', level);
            if (pod) params.set('pod', pod);
            if (useRegex) params.set('regex', 'true');

            const container = document.getElementById('logs-container');
            container.innerHTML = '<div class="loading"><div class="spinner"></div></div>';

            try {
                const res = await fetchWithTimeout('/api/logs/search?' + params);
                if (!res.ok) {
                    throw new Error('Server returned ' + res.status);
                }
                const data = await res.json();
                renderLogs(data.entries || []);

                if (data.scribe_says) {
                    addChatMessage(data.scribe_says, 'agent');
                }
            } catch (e) {
                console.error('Search error:', e);
                showError(container, e.message || 'Search failed');
            }
        }

        function toggleRegex() {
            const toggle = document.getElementById('regex-toggle');
            const checkbox = document.getElementById('use-regex');
            if (checkbox.checked) {
                toggle.classList.add('active');
            } else {
                toggle.classList.remove('active');
            }
        }

        function renderLogs(entries) {
            const container = document.getElementById('logs-container');

            if (!entries.length) {
                container.innerHTML = '<div class="empty-state"><div class="empty-state-icon">&#128269;</div><p>No entries found in the chronicles</p></div>';
                return;
            }

            container.innerHTML = entries.map(entry =>
                '<div class="log-entry">' +
                    '<span class="log-timestamp">' + formatTime(entry.timestamp) + '</span>' +
                    '<div class="log-source">' +
                        '<span class="log-namespace">' + escapeHtml(entry.namespace) + '</span>' +
                        '<span class="log-pod" title="' + escapeHtml(entry.pod) + '">' + escapeHtml(entry.pod) + '</span>' +
                    '</div>' +
                    '<span class="log-level ' + entry.level + '">' + entry.level + '</span>' +
                    '<span class="log-message ' + (entry.level === 'ERROR' ? 'error' : '') + '">' + escapeHtml(entry.message) + '</span>' +
                '</div>'
            ).join('');
        }

        function formatTime(timestamp) {
            const date = new Date(timestamp);
            return date.toLocaleString();
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        function clearSearch() {
            document.getElementById('search-query').value = '';
            document.getElementById('filter-namespace').value = '';
            document.getElementById('filter-level').value = '';
            document.getElementById('filter-pod').value = '';
            document.getElementById('use-regex').checked = false;
            document.getElementById('regex-toggle').classList.remove('active');
            loadLogs();
        }

        function refreshLogs() {
            loadStats();
            const query = document.getElementById('search-query').value;
            const namespace = document.getElementById('filter-namespace').value;
            const level = document.getElementById('filter-level').value;

            if (query || namespace || level) {
                searchLogs();
            } else {
                loadLogs();
            }
        }

        function toggleStream() {
            const btn = document.getElementById('stream-btn');

            if (streaming) {
                if (eventSource) {
                    eventSource.close();
                    eventSource = null;
                }
                streaming = false;
                btn.innerHTML = '&#9654; Live Tail';
                btn.style.background = '';
            } else {
                const namespace = document.getElementById('filter-namespace').value;
                const level = document.getElementById('filter-level').value;
                const pod = document.getElementById('filter-pod').value;
                const query = document.getElementById('search-query').value;
                const useRegex = document.getElementById('use-regex').checked;

                let url = '/api/logs/stream';
                const params = new URLSearchParams();
                if (namespace) params.set('namespace', namespace);
                if (level) params.set('level', level);
                if (pod) params.set('pod', pod);
                if (query) params.set('q', query);
                if (useRegex) params.set('regex', 'true');
                if (params.toString()) url += '?' + params;

                eventSource = new EventSource(url);
                eventSource.onmessage = (e) => {
                    const entry = JSON.parse(e.data);
                    prependLog(entry);
                };
                eventSource.onerror = () => {
                    addChatMessage('The stream has been interrupted. Attempting to reconnect...', 'agent');
                };
                streaming = true;
                btn.innerHTML = '&#9632; Stop Tail';
                btn.style.background = 'var(--ctp-green)';
                addChatMessage('Live tail activated. I shall now reveal events as they unfold...', 'agent');
            }
        }

        function prependLog(entry) {
            const container = document.getElementById('logs-container');
            const logHtml =
                '<div class="log-entry" style="animation: fadeIn 0.3s">' +
                    '<span class="log-timestamp">' + formatTime(entry.timestamp) + '</span>' +
                    '<div class="log-source">' +
                        '<span class="log-namespace">' + escapeHtml(entry.namespace) + '</span>' +
                        '<span class="log-pod" title="' + escapeHtml(entry.pod) + '">' + escapeHtml(entry.pod) + '</span>' +
                    '</div>' +
                    '<span class="log-level ' + entry.level + '">' + entry.level + '</span>' +
                    '<span class="log-message ' + (entry.level === 'ERROR' ? 'error' : '') + '">' + escapeHtml(entry.message) + '</span>' +
                '</div>';
            container.insertAdjacentHTML('afterbegin', logHtml);

            // Keep only last 200 entries in view during streaming
            while (container.children.length > 200) {
                container.removeChild(container.lastChild);
            }
        }

        function toggleExportMenu() {
            const menu = document.getElementById('export-menu');
            menu.classList.toggle('show');
            document.getElementById('settings-menu').classList.remove('show');
        }

        function toggleSettingsMenu() {
            const menu = document.getElementById('settings-menu');
            menu.classList.toggle('show');
            document.getElementById('export-menu').classList.remove('show');
        }

        function exportLogs(format) {
            const query = document.getElementById('search-query').value;
            const namespace = document.getElementById('filter-namespace').value;
            const level = document.getElementById('filter-level').value;
            const pod = document.getElementById('filter-pod').value;
            const useRegex = document.getElementById('use-regex').checked;

            const params = new URLSearchParams();
            params.set('format', format);
            if (query) params.set('q', query);
            if (namespace) params.set('namespace', namespace);
            if (level) params.set('level', level);
            if (pod) params.set('pod', pod);
            if (useRegex) params.set('regex', 'true');

            window.location.href = '/api/logs/export?' + params;
            document.getElementById('export-menu').classList.remove('show');

            addChatMessage('The chronicles are being prepared for export. Your ' + format.toUpperCase() + ' file shall appear shortly.', 'agent');
        }

        async function sendChat() {
            const input = document.getElementById('chat-input');
            const message = input.value.trim();
            if (!message) return;

            addChatMessage(message, 'user');
            input.value = '';

            try {
                const res = await fetchWithTimeout('/api/chat', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ message })
                });
                const data = await res.json();
                addChatMessage(data.response, 'agent');
            } catch (e) {
                addChatMessage('The chronicles seem momentarily inaccessible: ' + (e.message || 'timeout'), 'agent');
            }
        }

        function addChatMessage(text, type) {
            const container = document.getElementById('chat-messages');
            const div = document.createElement('div');
            div.className = 'chat-message ' + type;
            div.textContent = text;
            container.appendChild(div);
            container.scrollTop = container.scrollHeight;
        }
    </script>
</body>
</html>`

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Scribe starting - %s", scribeTagline)

	// Initialize Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to get in-cluster config: %v", err)
	}

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Initialize store and tracking
	store = NewLogStore()
	podLastSeen = make(map[string]time.Time)

	// Start log collection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go collectPodLogs(ctx)

	// Also do an initial collection
	go collectAllPodLogs()

	// Setup HTTP routes
	http.HandleFunc("/", handleUI)
	http.HandleFunc("/logs", handleUI)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/api/stats", handleStats)
	http.HandleFunc("/api/namespaces", handleNamespaces)
	http.HandleFunc("/api/pods", handlePods)
	http.HandleFunc("/api/logs", handleLogs)
	http.HandleFunc("/api/logs/search", handleLogsSearch)
	http.HandleFunc("/api/logs/stream", handleLogsStream)
	http.HandleFunc("/api/logs/export", handleLogsExport)
	http.HandleFunc("/api/retention", handleRetention)
	http.HandleFunc("/api/volume", handleVolumeStats)
	http.HandleFunc("/api/chat", handleChat)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Scribe listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
