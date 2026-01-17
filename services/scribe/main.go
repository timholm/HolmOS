package main

import (
	"bufio"
	"context"
	"encoding/csv"
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
	Agent           string          `json:"agent"`
	Tagline         string          `json:"tagline"`
	TotalEntries    int             `json:"total_entries"`
	Namespaces      map[string]int  `json:"namespaces"`
	Pods            map[string]int  `json:"pods"`
	Levels          map[string]int  `json:"levels"`
	VolumeStats     VolumeStats     `json:"volume_stats"`
	RetentionConfig RetentionConfig `json:"retention_config"`
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

// ==================== ALERTING TYPES ====================

// Alert represents an alerting rule
type Alert struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Pattern     string    `json:"pattern"`             // Regex pattern to match
	Level       string    `json:"level,omitempty"`     // Optional: only match specific level
	Namespace   string    `json:"namespace,omitempty"` // Optional: only match specific namespace
	Pod         string    `json:"pod,omitempty"`       // Optional: only match specific pod pattern
	Threshold   int       `json:"threshold"`           // Number of matches to trigger
	WindowMins  int       `json:"window_mins"`         // Time window in minutes
	Severity    string    `json:"severity"`            // critical, warning, info
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AlertMatch represents a single match for an alert
type AlertMatch struct {
	AlertID   string    `json:"alert_id"`
	Timestamp time.Time `json:"timestamp"`
	Entry     LogEntry  `json:"entry"`
}

// AlertState tracks the current state of an alert
type AlertState struct {
	Alert       Alert        `json:"alert"`
	Matches     []AlertMatch `json:"matches"`
	Triggered   bool         `json:"triggered"`
	TriggeredAt time.Time    `json:"triggered_at,omitempty"`
	LastChecked time.Time    `json:"last_checked"`
	MatchCount  int          `json:"match_count"`
}

// AlertStore manages alerts
type AlertStore struct {
	mu     sync.RWMutex
	alerts map[string]*Alert
	states map[string]*AlertState
}

// AlertResponse for API responses
type AlertResponse struct {
	Alerts    []Alert      `json:"alerts,omitempty"`
	Alert     *Alert       `json:"alert,omitempty"`
	States    []AlertState `json:"states,omitempty"`
	Triggered []AlertState `json:"triggered,omitempty"`
	Message   string       `json:"message,omitempty"`
}

// ==================== SEARCH TYPES ====================

// SearchRequest for advanced search
type SearchRequest struct {
	Query        string    `json:"query"`
	Regex        bool      `json:"regex"`
	Namespace    string    `json:"namespace,omitempty"`
	Level        string    `json:"level,omitempty"`
	Pod          string    `json:"pod,omitempty"`
	Container    string    `json:"container,omitempty"`
	StartTime    time.Time `json:"start_time,omitempty"`
	EndTime      time.Time `json:"end_time,omitempty"`
	Limit        int       `json:"limit,omitempty"`
	Offset       int       `json:"offset,omitempty"`
	SortOrder    string    `json:"sort_order,omitempty"` // asc or desc
	Highlight    bool      `json:"highlight"`
	ContextLines int       `json:"context_lines,omitempty"` // lines before/after match
}

// SearchResult with metadata
type SearchResult struct {
	Entry         LogEntry   `json:"entry"`
	Score         float64    `json:"score,omitempty"`
	Highlights    []string   `json:"highlights,omitempty"`
	LineNumber    int        `json:"line_number,omitempty"`
	ContextBefore []LogEntry `json:"context_before,omitempty"`
	ContextAfter  []LogEntry `json:"context_after,omitempty"`
}

// SearchResponse for search results
type SearchResponse struct {
	Query         string         `json:"query"`
	TotalMatches  int            `json:"total_matches"`
	ReturnedCount int            `json:"returned_count"`
	Results       []SearchResult `json:"results"`
	Took          string         `json:"took"`
	ScribeSays    string         `json:"scribe_says,omitempty"`
	Facets        *SearchFacets  `json:"facets,omitempty"`
}

// SearchFacets for filtering options
type SearchFacets struct {
	Namespaces map[string]int `json:"namespaces"`
	Levels     map[string]int `json:"levels"`
	Pods       map[string]int `json:"pods"`
	TimeRanges map[string]int `json:"time_ranges"`
}

// ==================== AGGREGATION TYPES ====================

// AggregationRequest for log aggregation
type AggregationRequest struct {
	GroupBy   []string          `json:"group_by"` // level, namespace, pod, container, hour, day
	Metrics   []string          `json:"metrics"`  // count, bytes, avg_size
	Filters   map[string]string `json:"filters,omitempty"`
	StartTime time.Time         `json:"start_time,omitempty"`
	EndTime   time.Time         `json:"end_time,omitempty"`
	TopN      int               `json:"top_n,omitempty"` // Return top N results
}

// AggregationBucket for grouped results
type AggregationBucket struct {
	Key        map[string]string   `json:"key"`
	Count      int                 `json:"count"`
	Bytes      int64               `json:"bytes"`
	AvgSize    float64             `json:"avg_size"`
	FirstSeen  time.Time           `json:"first_seen"`
	LastSeen   time.Time           `json:"last_seen"`
	ErrorRate  float64             `json:"error_rate,omitempty"`
	SubBuckets []AggregationBucket `json:"sub_buckets,omitempty"`
}

// AggregationResponse for aggregation results
type AggregationResponse struct {
	GroupBy    []string            `json:"group_by"`
	TotalCount int                 `json:"total_count"`
	TotalBytes int64               `json:"total_bytes"`
	Buckets    []AggregationBucket `json:"buckets"`
	Took       string              `json:"took"`
	ScribeSays string              `json:"scribe_says,omitempty"`
}

// ==================== RETENTION POLICY TYPES ====================

// RetentionPolicy for advanced retention management
type RetentionPolicy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Namespace   string `json:"namespace,omitempty"` // Apply to specific namespace, empty = all
	Level       string `json:"level,omitempty"`     // Apply to specific level, empty = all
	MaxAge      int    `json:"max_age_hours"`       // Max age in hours
	MaxEntries  int    `json:"max_entries"`         // Max entries to keep
	Priority    int    `json:"priority"`            // Higher priority = applied first
	Enabled     bool   `json:"enabled"`
}

// RetentionPolicyStore manages retention policies
type RetentionPolicyStore struct {
	mu       sync.RWMutex
	policies map[string]*RetentionPolicy
}

// ExportRequest for enhanced export
type ExportRequest struct {
	Format          string    `json:"format"` // json, csv, txt, ndjson
	Query           string    `json:"query,omitempty"`
	Namespace       string    `json:"namespace,omitempty"`
	Level           string    `json:"level,omitempty"`
	Pod             string    `json:"pod,omitempty"`
	StartTime       time.Time `json:"start_time,omitempty"`
	EndTime         time.Time `json:"end_time,omitempty"`
	Limit           int       `json:"limit,omitempty"`
	IncludeMetadata bool      `json:"include_metadata"`
	Compress        bool      `json:"compress"`
}

var (
	store          *LogStore
	alertStore     *AlertStore
	retentionStore *RetentionPolicyStore
	clientset      *kubernetes.Clientset
	podLastSeen    map[string]time.Time
	podMu          sync.RWMutex
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

	// Check alerts for this entry
	if alertStore != nil {
		alertStore.CheckEntry(entry)
	}

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

// AdvancedSearch performs full-text search with advanced options
func (ls *LogStore) AdvancedSearch(req SearchRequest) ([]SearchResult, int, *SearchFacets) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	var results []SearchResult
	var regex *regexp.Regexp
	var err error

	if req.Regex && req.Query != "" {
		regex, err = regexp.Compile("(?i)" + req.Query)
		if err != nil {
			regex = nil
		}
	}

	queryLower := strings.ToLower(req.Query)

	// Build facets
	facets := &SearchFacets{
		Namespaces: make(map[string]int),
		Levels:     make(map[string]int),
		Pods:       make(map[string]int),
		TimeRanges: make(map[string]int),
	}

	totalMatches := 0
	limit := req.Limit
	if limit == 0 {
		limit = 500
	}
	offset := req.Offset

	// Determine iteration order
	var indices []int
	if req.SortOrder == "asc" {
		for i := 0; i < len(ls.entries); i++ {
			indices = append(indices, i)
		}
	} else {
		for i := len(ls.entries) - 1; i >= 0; i-- {
			indices = append(indices, i)
		}
	}

	for _, i := range indices {
		entry := ls.entries[i]

		// Apply filters
		if req.Namespace != "" && entry.Namespace != req.Namespace {
			continue
		}
		if req.Level != "" && entry.Level != req.Level {
			continue
		}
		if req.Pod != "" && !strings.Contains(entry.Pod, req.Pod) {
			continue
		}
		if req.Container != "" && !strings.Contains(entry.Container, req.Container) {
			continue
		}
		if !req.StartTime.IsZero() && entry.Timestamp.Before(req.StartTime) {
			continue
		}
		if !req.EndTime.IsZero() && entry.Timestamp.After(req.EndTime) {
			continue
		}

		// Apply query
		matched := false
		var highlights []string

		if req.Query == "" {
			matched = true
		} else if regex != nil {
			if regex.MatchString(entry.Message) {
				matched = true
				if req.Highlight {
					matches := regex.FindAllString(entry.Message, -1)
					highlights = append(highlights, matches...)
				}
			}
		} else {
			if strings.Contains(strings.ToLower(entry.Message), queryLower) {
				matched = true
				if req.Highlight {
					highlights = append(highlights, req.Query)
				}
			}
		}

		if !matched {
			continue
		}

		totalMatches++

		// Update facets
		facets.Namespaces[entry.Namespace]++
		facets.Levels[entry.Level]++
		facets.Pods[entry.Pod]++
		hourKey := entry.Timestamp.Format("2006-01-02 15:00")
		facets.TimeRanges[hourKey]++

		// Apply pagination
		if totalMatches <= offset {
			continue
		}
		if len(results) >= limit {
			continue
		}

		result := SearchResult{
			Entry:      entry,
			Highlights: highlights,
			LineNumber: i + 1,
		}

		// Add context lines if requested
		if req.ContextLines > 0 {
			// Context before
			start := i - req.ContextLines
			if start < 0 {
				start = 0
			}
			for j := start; j < i; j++ {
				result.ContextBefore = append(result.ContextBefore, ls.entries[j])
			}

			// Context after
			end := i + req.ContextLines + 1
			if end > len(ls.entries) {
				end = len(ls.entries)
			}
			for j := i + 1; j < end; j++ {
				result.ContextAfter = append(result.ContextAfter, ls.entries[j])
			}
		}

		results = append(results, result)
	}

	return results, totalMatches, facets
}

// Aggregate performs log aggregation
func (ls *LogStore) Aggregate(req AggregationRequest) []AggregationBucket {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	buckets := make(map[string]*AggregationBucket)

	for _, entry := range ls.entries {
		// Apply time filters
		if !req.StartTime.IsZero() && entry.Timestamp.Before(req.StartTime) {
			continue
		}
		if !req.EndTime.IsZero() && entry.Timestamp.After(req.EndTime) {
			continue
		}

		// Apply filters
		if ns, ok := req.Filters["namespace"]; ok && entry.Namespace != ns {
			continue
		}
		if lvl, ok := req.Filters["level"]; ok && entry.Level != lvl {
			continue
		}
		if pod, ok := req.Filters["pod"]; ok && !strings.Contains(entry.Pod, pod) {
			continue
		}

		// Build bucket key
		keyParts := make(map[string]string)
		for _, groupBy := range req.GroupBy {
			switch groupBy {
			case "level":
				keyParts["level"] = entry.Level
			case "namespace":
				keyParts["namespace"] = entry.Namespace
			case "pod":
				keyParts["pod"] = entry.Pod
			case "container":
				keyParts["container"] = entry.Container
			case "hour":
				keyParts["hour"] = entry.Timestamp.Format("2006-01-02 15:00")
			case "day":
				keyParts["day"] = entry.Timestamp.Format("2006-01-02")
			}
		}

		// Create bucket key string
		var keyStr string
		for _, k := range req.GroupBy {
			keyStr += keyParts[k] + "|"
		}

		bucket, exists := buckets[keyStr]
		if !exists {
			bucket = &AggregationBucket{
				Key:       keyParts,
				FirstSeen: entry.Timestamp,
				LastSeen:  entry.Timestamp,
			}
			buckets[keyStr] = bucket
		}

		bucket.Count++
		bucket.Bytes += int64(entry.Size)
		if entry.Timestamp.Before(bucket.FirstSeen) {
			bucket.FirstSeen = entry.Timestamp
		}
		if entry.Timestamp.After(bucket.LastSeen) {
			bucket.LastSeen = entry.Timestamp
		}
	}

	// Calculate averages and error rates
	var result []AggregationBucket
	for _, bucket := range buckets {
		if bucket.Count > 0 {
			bucket.AvgSize = float64(bucket.Bytes) / float64(bucket.Count)
		}
		result = append(result, *bucket)
	}

	// Sort by count descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	// Apply TopN if specified
	if req.TopN > 0 && len(result) > req.TopN {
		result = result[:req.TopN]
	}

	return result
}

// ==================== ALERT STORE METHODS ====================

func NewAlertStore() *AlertStore {
	return &AlertStore{
		alerts: make(map[string]*Alert),
		states: make(map[string]*AlertState),
	}
}

func (as *AlertStore) Add(alert Alert) {
	as.mu.Lock()
	defer as.mu.Unlock()

	if alert.ID == "" {
		alert.ID = fmt.Sprintf("alert-%d", time.Now().UnixNano())
	}
	alert.CreatedAt = time.Now()
	alert.UpdatedAt = time.Now()

	as.alerts[alert.ID] = &alert
	as.states[alert.ID] = &AlertState{
		Alert:       alert,
		Matches:     []AlertMatch{},
		LastChecked: time.Now(),
	}
}

func (as *AlertStore) Get(id string) (*Alert, bool) {
	as.mu.RLock()
	defer as.mu.RUnlock()
	alert, ok := as.alerts[id]
	return alert, ok
}

func (as *AlertStore) List() []Alert {
	as.mu.RLock()
	defer as.mu.RUnlock()

	alerts := make([]Alert, 0, len(as.alerts))
	for _, alert := range as.alerts {
		alerts = append(alerts, *alert)
	}
	return alerts
}

func (as *AlertStore) Delete(id string) bool {
	as.mu.Lock()
	defer as.mu.Unlock()

	if _, exists := as.alerts[id]; exists {
		delete(as.alerts, id)
		delete(as.states, id)
		return true
	}
	return false
}

func (as *AlertStore) Update(alert Alert) bool {
	as.mu.Lock()
	defer as.mu.Unlock()

	if _, exists := as.alerts[alert.ID]; exists {
		alert.UpdatedAt = time.Now()
		as.alerts[alert.ID] = &alert
		return true
	}
	return false
}

func (as *AlertStore) GetStates() []AlertState {
	as.mu.RLock()
	defer as.mu.RUnlock()

	states := make([]AlertState, 0, len(as.states))
	for _, state := range as.states {
		states = append(states, *state)
	}
	return states
}

func (as *AlertStore) GetTriggered() []AlertState {
	as.mu.RLock()
	defer as.mu.RUnlock()

	var triggered []AlertState
	for _, state := range as.states {
		if state.Triggered {
			triggered = append(triggered, *state)
		}
	}
	return triggered
}

// CheckEntry checks if a log entry matches any alert patterns
func (as *AlertStore) CheckEntry(entry LogEntry) {
	as.mu.Lock()
	defer as.mu.Unlock()

	for id, alert := range as.alerts {
		if !alert.Enabled {
			continue
		}

		// Check filters
		if alert.Level != "" && entry.Level != alert.Level {
			continue
		}
		if alert.Namespace != "" && entry.Namespace != alert.Namespace {
			continue
		}
		if alert.Pod != "" && !strings.Contains(entry.Pod, alert.Pod) {
			continue
		}

		// Check pattern
		regex, err := regexp.Compile("(?i)" + alert.Pattern)
		if err != nil {
			continue
		}

		if !regex.MatchString(entry.Message) {
			continue
		}

		// Match found
		state := as.states[id]
		match := AlertMatch{
			AlertID:   id,
			Timestamp: entry.Timestamp,
			Entry:     entry,
		}
		state.Matches = append(state.Matches, match)

		// Prune old matches outside window
		windowStart := time.Now().Add(-time.Duration(alert.WindowMins) * time.Minute)
		var recentMatches []AlertMatch
		for _, m := range state.Matches {
			if m.Timestamp.After(windowStart) {
				recentMatches = append(recentMatches, m)
			}
		}
		state.Matches = recentMatches
		state.MatchCount = len(recentMatches)
		state.LastChecked = time.Now()

		// Check if threshold exceeded
		if state.MatchCount >= alert.Threshold && !state.Triggered {
			state.Triggered = true
			state.TriggeredAt = time.Now()
			log.Printf("ALERT TRIGGERED: %s - %d matches in %d minutes",
				alert.Name, state.MatchCount, alert.WindowMins)
		} else if state.MatchCount < alert.Threshold && state.Triggered {
			// Reset if below threshold
			state.Triggered = false
		}
	}
}

// ==================== RETENTION POLICY STORE METHODS ====================

func NewRetentionPolicyStore() *RetentionPolicyStore {
	return &RetentionPolicyStore{
		policies: make(map[string]*RetentionPolicy),
	}
}

func (rps *RetentionPolicyStore) Add(policy RetentionPolicy) {
	rps.mu.Lock()
	defer rps.mu.Unlock()

	if policy.ID == "" {
		policy.ID = fmt.Sprintf("policy-%d", time.Now().UnixNano())
	}
	rps.policies[policy.ID] = &policy
}

func (rps *RetentionPolicyStore) Get(id string) (*RetentionPolicy, bool) {
	rps.mu.RLock()
	defer rps.mu.RUnlock()
	policy, ok := rps.policies[id]
	return policy, ok
}

func (rps *RetentionPolicyStore) List() []RetentionPolicy {
	rps.mu.RLock()
	defer rps.mu.RUnlock()

	policies := make([]RetentionPolicy, 0, len(rps.policies))
	for _, policy := range rps.policies {
		policies = append(policies, *policy)
	}

	// Sort by priority
	sort.Slice(policies, func(i, j int) bool {
		return policies[i].Priority > policies[j].Priority
	})

	return policies
}

func (rps *RetentionPolicyStore) Delete(id string) bool {
	rps.mu.Lock()
	defer rps.mu.Unlock()

	if _, exists := rps.policies[id]; exists {
		delete(rps.policies, id)
		return true
	}
	return false
}

func (rps *RetentionPolicyStore) Update(policy RetentionPolicy) bool {
	rps.mu.Lock()
	defer rps.mu.Unlock()

	if _, exists := rps.policies[policy.ID]; exists {
		rps.policies[policy.ID] = &policy
		return true
	}
	return false
}

// ApplyPolicies applies retention policies to entries
func (rps *RetentionPolicyStore) ApplyPolicies(entries []LogEntry) []LogEntry {
	rps.mu.RLock()
	policies := rps.List()
	rps.mu.RUnlock()

	now := time.Now()
	var retained []LogEntry

	for _, entry := range entries {
		keep := true

		for _, policy := range policies {
			if !policy.Enabled {
				continue
			}

			// Check if policy applies to this entry
			if policy.Namespace != "" && entry.Namespace != policy.Namespace {
				continue
			}
			if policy.Level != "" && entry.Level != policy.Level {
				continue
			}

			// Check age
			age := now.Sub(entry.Timestamp)
			if policy.MaxAge > 0 && age > time.Duration(policy.MaxAge)*time.Hour {
				keep = false
				break
			}
		}

		if keep {
			retained = append(retained, entry)
		}
	}

	return retained
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

// ==================== SEARCH HANDLER ====================

func handleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	startTime := time.Now()

	var req SearchRequest

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		// Parse from query params
		req.Query = r.URL.Query().Get("q")
		req.Regex = r.URL.Query().Get("regex") == "true"
		req.Namespace = r.URL.Query().Get("namespace")
		req.Level = r.URL.Query().Get("level")
		req.Pod = r.URL.Query().Get("pod")
		req.Container = r.URL.Query().Get("container")
		req.Highlight = r.URL.Query().Get("highlight") == "true"
		req.SortOrder = r.URL.Query().Get("sort")

		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				req.Limit = n
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if n, err := strconv.Atoi(o); err == nil {
				req.Offset = n
			}
		}
		if c := r.URL.Query().Get("context"); c != "" {
			if n, err := strconv.Atoi(c); err == nil {
				req.ContextLines = n
			}
		}

		// Parse time range
		if st := r.URL.Query().Get("start_time"); st != "" {
			if t, err := time.Parse(time.RFC3339, st); err == nil {
				req.StartTime = t
			}
		}
		if et := r.URL.Query().Get("end_time"); et != "" {
			if t, err := time.Parse(time.RFC3339, et); err == nil {
				req.EndTime = t
			}
		}
	}

	results, totalMatches, facets := store.AdvancedSearch(req)

	took := time.Since(startTime)

	// Generate Scribe commentary
	var scribeSays string
	if totalMatches == 0 {
		scribeSays = "The chronicles hold no records matching your query. Perhaps refine your search terms?"
	} else if req.Regex {
		scribeSays = fmt.Sprintf("The regex pattern '%s' reveals %d entries in the annals. Searched in %s.", req.Query, totalMatches, took.Round(time.Millisecond))
	} else if totalMatches > 1000 {
		scribeSays = fmt.Sprintf("A vast trove of %d records discovered in %s. Consider narrowing your search.", totalMatches, took.Round(time.Millisecond))
	} else {
		scribeSays = fmt.Sprintf("Found %d records in %s. It's all in the records.", totalMatches, took.Round(time.Millisecond))
	}

	response := SearchResponse{
		Query:         req.Query,
		TotalMatches:  totalMatches,
		ReturnedCount: len(results),
		Results:       results,
		Took:          took.String(),
		ScribeSays:    scribeSays,
		Facets:        facets,
	}

	json.NewEncoder(w).Encode(response)
}

// ==================== ALERTS HANDLER ====================

func handleAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check for specific alert ID in path
	path := strings.TrimPrefix(r.URL.Path, "/api/alerts")
	alertID := strings.TrimPrefix(path, "/")

	switch r.Method {
	case http.MethodGet:
		if alertID == "" {
			// List all alerts
			alerts := alertStore.List()
			json.NewEncoder(w).Encode(AlertResponse{
				Alerts:  alerts,
				Message: fmt.Sprintf("The chronicles track %d alerting rules", len(alerts)),
			})
		} else if alertID == "states" {
			// Get all alert states
			states := alertStore.GetStates()
			json.NewEncoder(w).Encode(AlertResponse{
				States:  states,
				Message: fmt.Sprintf("%d alert states in the realm", len(states)),
			})
		} else if alertID == "triggered" {
			// Get triggered alerts
			triggered := alertStore.GetTriggered()
			json.NewEncoder(w).Encode(AlertResponse{
				Triggered: triggered,
				Message:   fmt.Sprintf("%d alerts currently triggered", len(triggered)),
			})
		} else {
			// Get specific alert
			alert, exists := alertStore.Get(alertID)
			if !exists {
				http.Error(w, "Alert not found", http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(AlertResponse{Alert: alert})
		}

	case http.MethodPost:
		var alert Alert
		if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if alert.Pattern == "" {
			http.Error(w, "Pattern is required", http.StatusBadRequest)
			return
		}
		if alert.Name == "" {
			alert.Name = "Unnamed Alert"
		}
		if alert.Threshold == 0 {
			alert.Threshold = 1
		}
		if alert.WindowMins == 0 {
			alert.WindowMins = 5
		}
		if alert.Severity == "" {
			alert.Severity = "warning"
		}
		alert.Enabled = true

		alertStore.Add(alert)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(AlertResponse{
			Alert:   &alert,
			Message: fmt.Sprintf("Alert '%s' inscribed in the chronicles", alert.Name),
		})

	case http.MethodPut:
		if alertID == "" {
			http.Error(w, "Alert ID required", http.StatusBadRequest)
			return
		}

		var alert Alert
		if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		alert.ID = alertID

		if !alertStore.Update(alert) {
			http.Error(w, "Alert not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(AlertResponse{
			Alert:   &alert,
			Message: "Alert updated in the chronicles",
		})

	case http.MethodDelete:
		if alertID == "" {
			http.Error(w, "Alert ID required", http.StatusBadRequest)
			return
		}

		if !alertStore.Delete(alertID) {
			http.Error(w, "Alert not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(AlertResponse{
			Message: "Alert removed from the chronicles",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ==================== AGGREGATIONS HANDLER ====================

func handleAggregations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	startTime := time.Now()

	var req AggregationRequest

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		// Parse from query params
		if groupBy := r.URL.Query().Get("group_by"); groupBy != "" {
			req.GroupBy = strings.Split(groupBy, ",")
		}
		if topN := r.URL.Query().Get("top_n"); topN != "" {
			if n, err := strconv.Atoi(topN); err == nil {
				req.TopN = n
			}
		}

		// Parse filters
		req.Filters = make(map[string]string)
		if ns := r.URL.Query().Get("namespace"); ns != "" {
			req.Filters["namespace"] = ns
		}
		if lvl := r.URL.Query().Get("level"); lvl != "" {
			req.Filters["level"] = lvl
		}
		if pod := r.URL.Query().Get("pod"); pod != "" {
			req.Filters["pod"] = pod
		}

		// Parse time range
		if st := r.URL.Query().Get("start_time"); st != "" {
			if t, err := time.Parse(time.RFC3339, st); err == nil {
				req.StartTime = t
			}
		}
		if et := r.URL.Query().Get("end_time"); et != "" {
			if t, err := time.Parse(time.RFC3339, et); err == nil {
				req.EndTime = t
			}
		}
	}

	// Default grouping
	if len(req.GroupBy) == 0 {
		req.GroupBy = []string{"level"}
	}

	buckets := store.Aggregate(req)

	took := time.Since(startTime)

	// Calculate totals
	var totalCount int
	var totalBytes int64
	for _, b := range buckets {
		totalCount += b.Count
		totalBytes += b.Bytes
	}

	// Generate Scribe commentary
	var scribeSays string
	if len(buckets) == 0 {
		scribeSays = "No data to aggregate within the specified parameters."
	} else {
		scribeSays = fmt.Sprintf("Aggregated %d entries into %d buckets by %s in %s.",
			totalCount, len(buckets), strings.Join(req.GroupBy, ", "), took.Round(time.Millisecond))
	}

	response := AggregationResponse{
		GroupBy:    req.GroupBy,
		TotalCount: totalCount,
		TotalBytes: totalBytes,
		Buckets:    buckets,
		Took:       took.String(),
		ScribeSays: scribeSays,
	}

	json.NewEncoder(w).Encode(response)
}

// ==================== ENHANCED EXPORT HANDLER ====================

func handleExport(w http.ResponseWriter, r *http.Request) {
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

	// Parse time range
	var startTime, endTime time.Time
	if st := r.URL.Query().Get("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			startTime = t
		}
	}
	if et := r.URL.Query().Get("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			endTime = t
		}
	}

	// Use advanced search for filtering
	searchReq := SearchRequest{
		Query:     query,
		Regex:     useRegex,
		Namespace: namespace,
		Level:     level,
		Pod:       pod,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     limit,
	}

	results, totalMatches, _ := store.AdvancedSearch(searchReq)

	// Extract entries from results
	entries := make([]LogEntry, len(results))
	for i, r := range results {
		entries[i] = r.Entry
	}

	filename := fmt.Sprintf("scribe-logs-%s", time.Now().Format("2006-01-02-150405"))

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", filename))

		writer := csv.NewWriter(w)
		// Write header
		writer.Write([]string{"timestamp", "namespace", "pod", "container", "level", "message", "size"})

		for _, e := range entries {
			writer.Write([]string{
				e.Timestamp.Format(time.RFC3339),
				e.Namespace,
				e.Pod,
				e.Container,
				e.Level,
				e.Message,
				strconv.Itoa(e.Size),
			})
		}
		writer.Flush()

	case "ndjson":
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.ndjson", filename))

		encoder := json.NewEncoder(w)
		for _, e := range entries {
			encoder.Encode(e)
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
			"exported_at":   time.Now(),
			"total_matches": totalMatches,
			"count":         len(entries),
			"filters": map[string]interface{}{
				"query":      query,
				"namespace":  namespace,
				"level":      level,
				"pod":        pod,
				"start_time": startTime,
				"end_time":   endTime,
			},
			"entries": entries,
		})
	}
}

// ==================== RETENTION POLICIES HANDLER ====================

func handleRetentionPolicies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check for specific policy ID in path
	path := strings.TrimPrefix(r.URL.Path, "/api/retention/policies")
	policyID := strings.TrimPrefix(path, "/")

	switch r.Method {
	case http.MethodGet:
		if policyID == "" {
			// List all policies
			policies := retentionStore.List()
			json.NewEncoder(w).Encode(map[string]interface{}{
				"policies": policies,
				"count":    len(policies),
				"message":  fmt.Sprintf("%d retention policies configured", len(policies)),
			})
		} else {
			// Get specific policy
			policy, exists := retentionStore.Get(policyID)
			if !exists {
				http.Error(w, "Policy not found", http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(policy)
		}

	case http.MethodPost:
		var policy RetentionPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if policy.Name == "" {
			policy.Name = "Unnamed Policy"
		}
		if policy.MaxAge == 0 && policy.MaxEntries == 0 {
			http.Error(w, "Either max_age_hours or max_entries is required", http.StatusBadRequest)
			return
		}
		policy.Enabled = true

		retentionStore.Add(policy)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"policy":  policy,
			"message": fmt.Sprintf("Retention policy '%s' created", policy.Name),
		})

	case http.MethodPut:
		if policyID == "" {
			http.Error(w, "Policy ID required", http.StatusBadRequest)
			return
		}

		var policy RetentionPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		policy.ID = policyID

		if !retentionStore.Update(policy) {
			http.Error(w, "Policy not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"policy":  policy,
			"message": "Retention policy updated",
		})

	case http.MethodDelete:
		if policyID == "" {
			http.Error(w, "Policy ID required", http.StatusBadRequest)
			return
		}

		if !retentionStore.Delete(policyID) {
			http.Error(w, "Policy not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Retention policy removed",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
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

	if strings.Contains(msgLower, "alert") || strings.Contains(msgLower, "notify") || strings.Contains(msgLower, "trigger") {
		triggered := alertStore.GetTriggered()
		if len(triggered) > 0 {
			return fmt.Sprintf("Warning! %d alerts are currently triggered. Check /api/alerts/triggered for details. The chronicles demand your attention.", len(triggered))
		}
		alerts := alertStore.List()
		return fmt.Sprintf("I monitor %d alerting rules. Use /api/alerts to manage them. Currently, no alerts are triggered.", len(alerts))
	}

	if strings.Contains(msgLower, "aggregat") || strings.Contains(msgLower, "group") || strings.Contains(msgLower, "summarize") {
		return "Use /api/aggregations to group logs by level, namespace, pod, hour, or day. Example: /api/aggregations?group_by=level,namespace to see counts by level within each namespace."
	}

	if strings.Contains(msgLower, "policy") || strings.Contains(msgLower, "policies") {
		policies := retentionStore.List()
		return fmt.Sprintf("I enforce %d retention policies. Use /api/retention/policies to view and manage them. Policies control how long different types of logs are preserved.", len(policies))
	}

	if strings.Contains(msgLower, "api") || strings.Contains(msgLower, "endpoint") {
		return "Available endpoints: /api/search (advanced search), /api/alerts (alerting), /api/aggregations (log grouping), /api/export (enhanced export), /api/retention/policies (retention management). It's all in the records."
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

        <section class="alerts-section" style="background: var(--ctp-mantle); padding: 20px; border-radius: 12px; margin-bottom: 20px; border: 1px solid var(--ctp-surface0);">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 15px;">
                <h3 style="color: var(--ctp-lavender);">&#128276; Alert Monitor</h3>
                <div style="display: flex; gap: 10px;">
                    <span id="alert-status" style="padding: 6px 12px; border-radius: 6px; font-size: 0.85rem; background: var(--ctp-green); color: var(--ctp-crust);">All Clear</span>
                    <button class="btn btn-secondary" onclick="loadAlerts()" style="padding: 8px 16px;">Refresh</button>
                </div>
            </div>
            <div id="alerts-container" style="max-height: 200px; overflow-y: auto;">
                <div class="loading"><div class="spinner"></div></div>
            </div>
        </section>

        <section class="aggregations-section" style="background: var(--ctp-mantle); padding: 20px; border-radius: 12px; margin-bottom: 20px; border: 1px solid var(--ctp-surface0);">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 15px;">
                <h3 style="color: var(--ctp-lavender);">&#128202; Log Aggregations</h3>
                <div style="display: flex; gap: 10px; align-items: center;">
                    <select id="agg-group-by" class="filter-select" style="min-width: 180px;">
                        <option value="level">By Level</option>
                        <option value="namespace">By Namespace</option>
                        <option value="pod">By Pod</option>
                        <option value="level,namespace">Level + Namespace</option>
                        <option value="hour">By Hour</option>
                        <option value="day">By Day</option>
                    </select>
                    <button class="btn" onclick="loadAggregations()">Aggregate</button>
                </div>
            </div>
            <div id="agg-container" style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 15px;">
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

        // ==================== ALERTS ====================

        async function loadAlerts() {
            const container = document.getElementById('alerts-container');
            const statusBadge = document.getElementById('alert-status');

            try {
                const [alertsRes, triggeredRes] = await Promise.all([
                    fetchWithTimeout('/api/alerts'),
                    fetchWithTimeout('/api/alerts/triggered')
                ]);

                const alertsData = await alertsRes.json();
                const triggeredData = await triggeredRes.json();

                const alerts = alertsData.alerts || [];
                const triggered = triggeredData.triggered || [];

                // Update status badge
                if (triggered.length > 0) {
                    statusBadge.textContent = triggered.length + ' Triggered';
                    statusBadge.style.background = 'var(--ctp-red)';
                } else {
                    statusBadge.textContent = 'All Clear';
                    statusBadge.style.background = 'var(--ctp-green)';
                }

                if (alerts.length === 0) {
                    container.innerHTML = '<p style="color: var(--ctp-subtext0); text-align: center; padding: 20px;">No alerts configured</p>';
                    return;
                }

                container.innerHTML = alerts.map(alert => {
                    const isTriggered = triggered.some(t => t.alert.id === alert.id);
                    const severityColor = alert.severity === 'critical' ? 'var(--ctp-red)' :
                                         alert.severity === 'warning' ? 'var(--ctp-yellow)' : 'var(--ctp-blue)';
                    return '<div style="display: flex; align-items: center; justify-content: space-between; padding: 12px; background: var(--ctp-surface0); border-radius: 8px; margin-bottom: 8px; border-left: 3px solid ' + severityColor + ';">' +
                        '<div>' +
                            '<div style="font-weight: bold; color: ' + (isTriggered ? 'var(--ctp-red)' : 'var(--ctp-text)') + ';">' + escapeHtml(alert.name) + (isTriggered ? ' [TRIGGERED]' : '') + '</div>' +
                            '<div style="font-size: 0.85rem; color: var(--ctp-subtext0);">Pattern: ' + escapeHtml(alert.pattern) + ' | Threshold: ' + alert.threshold + ' in ' + alert.window_mins + 'min</div>' +
                        '</div>' +
                        '<div style="display: flex; gap: 10px; align-items: center;">' +
                            '<span style="padding: 4px 8px; border-radius: 4px; font-size: 0.75rem; background: ' + severityColor + '; color: var(--ctp-crust);">' + alert.severity.toUpperCase() + '</span>' +
                            '<span style="color: ' + (alert.enabled ? 'var(--ctp-green)' : 'var(--ctp-overlay0)') + ';">' + (alert.enabled ? 'ON' : 'OFF') + '</span>' +
                        '</div>' +
                    '</div>';
                }).join('');

            } catch (e) {
                console.error('Alerts error:', e);
                container.innerHTML = '<p style="color: var(--ctp-red); text-align: center; padding: 20px;">Failed to load alerts</p>';
            }
        }

        // ==================== AGGREGATIONS ====================

        async function loadAggregations() {
            const container = document.getElementById('agg-container');
            const groupBy = document.getElementById('agg-group-by').value;

            container.innerHTML = '<div class="loading" style="grid-column: 1 / -1;"><div class="spinner"></div></div>';

            try {
                const res = await fetchWithTimeout('/api/aggregations?group_by=' + groupBy + '&top_n=10');
                const data = await res.json();

                if (!data.buckets || data.buckets.length === 0) {
                    container.innerHTML = '<p style="color: var(--ctp-subtext0); text-align: center; padding: 20px; grid-column: 1 / -1;">No data to aggregate</p>';
                    return;
                }

                const maxCount = Math.max(...data.buckets.map(b => b.count));

                container.innerHTML = data.buckets.map(bucket => {
                    const keyStr = Object.values(bucket.key).join(' / ');
                    const pct = (bucket.count / maxCount) * 100;
                    const levelColor = bucket.key.level === 'ERROR' ? 'var(--ctp-red)' :
                                      bucket.key.level === 'WARN' ? 'var(--ctp-yellow)' :
                                      bucket.key.level === 'DEBUG' ? 'var(--ctp-overlay0)' : 'var(--ctp-green)';
                    return '<div style="background: var(--ctp-surface0); padding: 15px; border-radius: 8px;">' +
                        '<div style="font-size: 0.85rem; color: var(--ctp-subtext0); margin-bottom: 5px;">' + escapeHtml(keyStr) + '</div>' +
                        '<div style="font-size: 1.5rem; font-weight: bold; color: ' + levelColor + ';">' + formatNumber(bucket.count) + '</div>' +
                        '<div style="height: 6px; background: var(--ctp-surface1); border-radius: 3px; margin-top: 8px; overflow: hidden;">' +
                            '<div style="height: 100%; width: ' + pct + '%; background: ' + levelColor + '; border-radius: 3px;"></div>' +
                        '</div>' +
                        '<div style="font-size: 0.75rem; color: var(--ctp-overlay0); margin-top: 5px;">' + formatBytes(bucket.bytes) + '</div>' +
                    '</div>';
                }).join('');

                if (data.scribe_says) {
                    addChatMessage(data.scribe_says, 'agent');
                }

            } catch (e) {
                console.error('Aggregations error:', e);
                container.innerHTML = '<p style="color: var(--ctp-red); text-align: center; padding: 20px; grid-column: 1 / -1;">Failed to load aggregations</p>';
            }
        }

        function formatBytes(bytes) {
            if (bytes < 1024) return bytes + ' B';
            if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
            if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
            return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB';
        }

        // Load alerts and aggregations on page load
        document.addEventListener('DOMContentLoaded', () => {
            // Existing loads are already in the initial DOMContentLoaded
            setTimeout(() => {
                loadAlerts();
                loadAggregations();
            }, 500);
        });
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

	// Initialize stores and tracking
	store = NewLogStore()
	alertStore = NewAlertStore()
	retentionStore = NewRetentionPolicyStore()
	podLastSeen = make(map[string]time.Time)

	// Add some default alerts
	alertStore.Add(Alert{
		Name:        "Error Rate Spike",
		Description: "Triggers when error logs exceed threshold",
		Pattern:     "(?i)(error|exception|fatal|panic)",
		Level:       "ERROR",
		Threshold:   10,
		WindowMins:  5,
		Severity:    "critical",
		Enabled:     true,
	})
	alertStore.Add(Alert{
		Name:        "OOM Detection",
		Description: "Detects out of memory conditions",
		Pattern:     "(?i)(out of memory|oom|memory limit)",
		Threshold:   1,
		WindowMins:  1,
		Severity:    "critical",
		Enabled:     true,
	})
	alertStore.Add(Alert{
		Name:        "Connection Errors",
		Description: "Detects connection failures",
		Pattern:     "(?i)(connection refused|connection reset|timeout|ECONNREFUSED)",
		Threshold:   5,
		WindowMins:  5,
		Severity:    "warning",
		Enabled:     true,
	})

	// Add default retention policies
	retentionStore.Add(RetentionPolicy{
		Name:        "Debug Log Retention",
		Description: "Shorter retention for debug logs",
		Level:       "DEBUG",
		MaxAge:      6,
		Priority:    10,
		Enabled:     true,
	})
	retentionStore.Add(RetentionPolicy{
		Name:        "Error Log Retention",
		Description: "Longer retention for error logs",
		Level:       "ERROR",
		MaxAge:      168, // 7 days
		Priority:    20,
		Enabled:     true,
	})

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
	http.HandleFunc("/api/documents", handleLogs) // Alias for document-style access
	http.HandleFunc("/api/logs/search", handleLogsSearch)
	http.HandleFunc("/api/logs/stream", handleLogsStream)
	http.HandleFunc("/api/logs/export", handleLogsExport)
	http.HandleFunc("/api/retention", handleRetention)
	http.HandleFunc("/api/volume", handleVolumeStats)
	http.HandleFunc("/api/chat", handleChat)

	// New observability endpoints
	http.HandleFunc("/api/search", handleSearch)                         // Advanced full-text search
	http.HandleFunc("/api/alerts", handleAlerts)                         // Alerting management
	http.HandleFunc("/api/alerts/", handleAlerts)                        // Alerting with ID
	http.HandleFunc("/api/aggregations", handleAggregations)             // Log aggregation
	http.HandleFunc("/api/export", handleExport)                         // Enhanced export
	http.HandleFunc("/api/retention/policies", handleRetentionPolicies)  // Retention policies
	http.HandleFunc("/api/retention/policies/", handleRetentionPolicies) // Retention policies with ID

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Scribe listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
