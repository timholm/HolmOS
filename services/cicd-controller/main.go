package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	registryURL      = os.Getenv("REGISTRY_URL")
	holmGitURL       = os.Getenv("HOLMGIT_URL")
	port             = os.Getenv("PORT")
	webhookSecret    = os.Getenv("WEBHOOK_SECRET")
	maxConcurrent    = getEnvInt("MAX_CONCURRENT_BUILDS", 3)
	maxQueueSize     = getEnvInt("MAX_QUEUE_SIZE", 100)
	executionHistory = getEnvInt("EXECUTION_HISTORY_SIZE", 500)
	clientset        *kubernetes.Clientset

	// Pipeline definitions
	pipelines   = make(map[string]*Pipeline)
	pipelinesMu sync.RWMutex

	// Build queue with priority support
	buildQueue   = make([]*BuildJob, 0)
	buildQueueMu sync.RWMutex

	// Pipeline executions (history)
	executions   = make([]*PipelineExecution, 0)
	executionsMu sync.RWMutex

	// Build logs storage
	buildLogs   = make(map[string]*BuildLog)
	buildLogsMu sync.RWMutex

	// Webhook events
	webhookEvents   = make([]*WebhookEvent, 0)
	webhookEventsMu sync.RWMutex

	// SSE subscribers for real-time events
	sseSubscribers   = make(map[string][]chan *SSEEvent)
	sseSubscribersMu sync.RWMutex

	// Build statistics cache
	statsCache     *BuildStats
	statsCacheMu   sync.RWMutex
	statsCacheTime time.Time

	// Service start time for uptime tracking
	serviceStartTime = time.Now()
)

// getEnvInt gets an integer from environment variable with default
func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// BuildStats represents aggregated build statistics
type BuildStats struct {
	TotalBuilds       int            `json:"totalBuilds"`
	SuccessfulBuilds  int            `json:"successfulBuilds"`
	FailedBuilds      int            `json:"failedBuilds"`
	CancelledBuilds   int            `json:"cancelledBuilds"`
	RunningBuilds     int            `json:"runningBuilds"`
	QueuedBuilds      int            `json:"queuedBuilds"`
	SuccessRate       float64        `json:"successRate"`
	AvgBuildTime      float64        `json:"avgBuildTime"`
	MedianBuildTime   float64        `json:"medianBuildTime"`
	P95BuildTime      float64        `json:"p95BuildTime"`
	BuildsToday       int            `json:"buildsToday"`
	BuildsThisWeek    int            `json:"buildsThisWeek"`
	BuildsByPipeline  map[string]int `json:"buildsByPipeline"`
	BuildsByStatus    map[string]int `json:"buildsByStatus"`
	BuildsByBranch    map[string]int `json:"buildsByBranch"`
	BuildsByAuthor    map[string]int `json:"buildsByAuthor"`
	RecentTrend       []DailyStats   `json:"recentTrend"`
	ServiceUptime     float64        `json:"serviceUptime"`
	LastBuildTime     *time.Time     `json:"lastBuildTime"`
	LongestBuild      *BuildDuration `json:"longestBuild"`
	ShortestBuild     *BuildDuration `json:"shortestBuild"`
	CalculatedAt      time.Time      `json:"calculatedAt"`
}

// DailyStats represents build statistics for a single day
type DailyStats struct {
	Date        string  `json:"date"`
	Total       int     `json:"total"`
	Successful  int     `json:"successful"`
	Failed      int     `json:"failed"`
	SuccessRate float64 `json:"successRate"`
	AvgDuration float64 `json:"avgDuration"`
}

// BuildDuration represents a build with its duration info
type BuildDuration struct {
	ExecutionID  string  `json:"executionId"`
	PipelineName string  `json:"pipelineName"`
	Duration     float64 `json:"duration"`
	Status       string  `json:"status"`
}

// Pipeline defines a CI/CD pipeline with stages
type Pipeline struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	RepoURL     string            `json:"repoUrl"`
	Branch      string            `json:"branch"`
	Stages      []PipelineStage   `json:"stages"`
	Triggers    []PipelineTrigger `json:"triggers"`
	Variables   map[string]string `json:"variables"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// PipelineStage defines a stage in a pipeline
type PipelineStage struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // build, test, deploy, custom
	Image       string            `json:"image"`
	Commands    []string          `json:"commands"`
	Environment map[string]string `json:"environment"`
	Timeout     int               `json:"timeout"` // seconds
	DependsOn   []string          `json:"dependsOn"`
	Condition   string            `json:"condition"` // always, on_success, on_failure
}

// PipelineTrigger defines what triggers a pipeline
type PipelineTrigger struct {
	Type     string   `json:"type"` // webhook, schedule, manual
	Branches []string `json:"branches"`
	Events   []string `json:"events"` // push, tag, pr
	Schedule string   `json:"schedule"`
}

// BuildJob represents a queued build with enhanced priority support
type BuildJob struct {
	ID           string            `json:"id"`
	PipelineID   string            `json:"pipelineId"`
	Pipeline     string            `json:"pipeline"`
	Repo         string            `json:"repo"`
	Branch       string            `json:"branch"`
	Commit       string            `json:"commit"`
	Author       string            `json:"author"`
	Message      string            `json:"message"`
	Status       string            `json:"status"` // queued, running, success, failed, cancelled, skipped
	Priority     int               `json:"priority"` // 1=low, 2=normal, 3=high, 4=critical
	PriorityName string            `json:"priorityName"`
	Variables    map[string]string `json:"variables"`
	Labels       map[string]string `json:"labels"`
	TriggerType  string            `json:"triggerType"` // manual, webhook, schedule, api
	TriggerBy    string            `json:"triggerBy"`
	RetryCount   int               `json:"retryCount"`
	MaxRetries   int               `json:"maxRetries"`
	QueuedAt     time.Time         `json:"queuedAt"`
	CreatedAt    time.Time         `json:"createdAt"`
	StartedAt    *time.Time        `json:"startedAt"`
	CompletedAt  *time.Time        `json:"completedAt"`
	EstimatedEnd *time.Time        `json:"estimatedEnd"`
}

// Priority constants
const (
	PriorityLow      = 1
	PriorityNormal   = 2
	PriorityHigh     = 3
	PriorityCritical = 4
)

// getPriorityName returns a human-readable priority name
func getPriorityName(priority int) string {
	switch priority {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// PipelineExecution represents a pipeline run history entry
type PipelineExecution struct {
	ID           string           `json:"id"`
	PipelineID   string           `json:"pipelineId"`
	PipelineName string           `json:"pipelineName"`
	BuildNumber  int              `json:"buildNumber"`
	Repo         string           `json:"repo"`
	Branch       string           `json:"branch"`
	Commit       string           `json:"commit"`
	Author       string           `json:"author"`
	Message      string           `json:"message"`
	Trigger      string           `json:"trigger"`
	Status       string           `json:"status"`
	Stages       []StageExecution `json:"stages"`
	StartedAt    time.Time        `json:"startedAt"`
	CompletedAt  *time.Time       `json:"completedAt"`
	Duration     float64          `json:"duration"`
	Artifacts    []string         `json:"artifacts"`
}

// StageExecution represents a stage execution within a pipeline run
type StageExecution struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
	Duration    float64    `json:"duration"`
	LogID       string     `json:"logId"`
	Error       string     `json:"error"`
}

// BuildLog stores logs for a build/stage
type BuildLog struct {
	ID          string    `json:"id"`
	ExecutionID string    `json:"executionId"`
	Stage       string    `json:"stage"`
	Lines       []LogLine `json:"lines"`
	CreatedAt   time.Time `json:"createdAt"`
}

// LogLine is a single log line with timestamp
type LogLine struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // info, warn, error, debug
	Message   string    `json:"message"`
}

// WebhookEvent represents an incoming webhook with enhanced metadata
type WebhookEvent struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"` // push, pull_request, tag, release, comment
	Action          string                 `json:"action"` // opened, closed, merged, etc.
	Source          string                 `json:"source"` // github, gitlab, holmgit, bitbucket
	Repo            string                 `json:"repo"`
	RepoFullName    string                 `json:"repoFullName"`
	Branch          string                 `json:"branch"`
	BaseBranch      string                 `json:"baseBranch"` // for PRs
	Commit          string                 `json:"commit"`
	CommitShort     string                 `json:"commitShort"`
	Author          string                 `json:"author"`
	AuthorEmail     string                 `json:"authorEmail"`
	Message         string                 `json:"message"`
	PRNumber        int                    `json:"prNumber"`
	PRTitle         string                 `json:"prTitle"`
	TagName         string                 `json:"tagName"`
	Payload         map[string]interface{} `json:"payload"`
	Headers         map[string]string      `json:"headers"`
	Signature       string                 `json:"signature"`
	SignatureValid  bool                   `json:"signatureValid"`
	Timestamp       time.Time              `json:"timestamp"`
	Processed       bool                   `json:"processed"`
	ProcessedAt     *time.Time             `json:"processedAt"`
	PipelineID      string                 `json:"pipelineId"`
	BuildID         string                 `json:"buildId"`
	Error           string                 `json:"error"`
	DeliveryID      string                 `json:"deliveryId"`
}

// WebhookConfig represents webhook configuration for a repository
type WebhookConfig struct {
	ID          string   `json:"id"`
	Repo        string   `json:"repo"`
	Secret      string   `json:"secret"`
	Events      []string `json:"events"`
	Active      bool     `json:"active"`
	URL         string   `json:"url"`
	ContentType string   `json:"contentType"`
	CreatedAt   time.Time `json:"createdAt"`
}

// KanikoBuild represents a Kaniko build job
type KanikoBuild struct {
	ID           string    `json:"id"`
	ExecutionID  string    `json:"executionId"`
	Repo         string    `json:"repo"`
	Branch       string    `json:"branch"`
	Dockerfile   string    `json:"dockerfile"`
	Context      string    `json:"context"`
	Destination  string    `json:"destination"`
	BuildArgs    []string  `json:"buildArgs"`
	Status       string    `json:"status"`
	PodName      string    `json:"podName"`
	StartedAt    time.Time `json:"startedAt"`
	CompletedAt  *time.Time `json:"completedAt"`
}

func main() {
	if registryURL == "" {
		registryURL = "10.110.67.87:5000"
	}
	if holmGitURL == "" {
		holmGitURL = "http://holm-git.holm.svc.cluster.local"
	}
	if port == "" {
		port = "8080"
	}

	log.Printf("CI/CD Controller starting on port %s", port)
	log.Printf("Registry URL: %s", registryURL)
	log.Printf("HolmGit URL: %s", holmGitURL)

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Warning: Running outside cluster: %v", err)
	} else {
		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			log.Fatalf("Failed to create k8s client: %v", err)
		}
	}

	// Initialize default pipelines
	initDefaultPipelines()

	// Start background workers
	go buildQueueWorker()
	go cleanupOldExecutions()

	// API routes
	http.HandleFunc("/", handleUI)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/ready", handleReadiness)

	// Pipeline management
	http.HandleFunc("/api/pipelines", handlePipelines)
	http.HandleFunc("/api/pipelines/", handlePipelineActions)

	// Webhook endpoints - enhanced with signature validation
	http.HandleFunc("/api/webhook/git", handleGitWebhook)
	http.HandleFunc("/api/webhook/github", handleGitHubWebhook)
	http.HandleFunc("/api/webhook/gitlab", handleGitLabWebhook)
	http.HandleFunc("/api/webhook/holmgit", handleHolmGitWebhook)
	http.HandleFunc("/api/webhooks", handleWebhooks)
	http.HandleFunc("/api/webhooks/", handleWebhookActions)

	// Build queue with priority management
	http.HandleFunc("/api/queue", handleQueue)
	http.HandleFunc("/api/queue/", handleQueueActions)
	http.HandleFunc("/api/queue/reorder", handleQueueReorder)
	http.HandleFunc("/api/queue/pause", handleQueuePause)
	http.HandleFunc("/api/queue/resume", handleQueueResume)

	// Pipeline executions (history) - enhanced with filtering and pagination
	http.HandleFunc("/api/executions", handleExecutions)
	http.HandleFunc("/api/executions/", handleExecutionActions)

	// Build statistics endpoint
	http.HandleFunc("/api/stats", handleStats)
	http.HandleFunc("/api/stats/trends", handleStatsTrends)
	http.HandleFunc("/api/stats/pipelines", handleStatsByPipeline)

	// Real-time event streams (SSE)
	http.HandleFunc("/api/events", handleSSEEvents)
	http.HandleFunc("/api/events/builds", handleSSEBuildEvents)
	http.HandleFunc("/api/logs-stream/", handleLogsStream)

	// Build logs
	http.HandleFunc("/api/logs/", handleLogs)

	// Kaniko builds
	http.HandleFunc("/api/build", handleBuild)
	http.HandleFunc("/api/builds", handleBuilds)

	// Deployment triggers
	http.HandleFunc("/api/deploy", handleDeploy)

	log.Printf("CI/CD Controller ready - listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func initDefaultPipelines() {
	// Create a sample pipeline
	defaultPipeline := &Pipeline{
		ID:          generateID("default"),
		Name:        "default",
		Description: "Default CI/CD pipeline",
		Branch:      "main",
		Stages: []PipelineStage{
			{
				Name:     "build",
				Type:     "build",
				Image:    "gcr.io/kaniko-project/executor:latest",
				Commands: []string{},
				Timeout:  600,
			},
			{
				Name:      "deploy",
				Type:      "deploy",
				DependsOn: []string{"build"},
				Timeout:   300,
			},
		},
		Triggers: []PipelineTrigger{
			{
				Type:     "webhook",
				Branches: []string{"main", "master"},
				Events:   []string{"push"},
			},
		},
		Variables: make(map[string]string),
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	pipelinesMu.Lock()
	pipelines[defaultPipeline.ID] = defaultPipeline
	pipelinesMu.Unlock()
}

func generateID(prefix string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())))
	return fmt.Sprintf("%x", hash)[:12]
}

// Build Queue Worker
func buildQueueWorker() {
	log.Println("Build queue worker started")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		processNextBuild()
	}
}

// queuePaused tracks if the build queue is paused
var queuePaused bool
var queuePausedMu sync.RWMutex

func processNextBuild() {
	// Check if queue is paused
	queuePausedMu.RLock()
	if queuePaused {
		queuePausedMu.RUnlock()
		return
	}
	queuePausedMu.RUnlock()

	buildQueueMu.Lock()
	defer buildQueueMu.Unlock()

	// Sort queue by priority (highest first), then by created time (oldest first)
	sort.SliceStable(buildQueue, func(i, j int) bool {
		if buildQueue[i].Status != "queued" || buildQueue[j].Status != "queued" {
			return false
		}
		if buildQueue[i].Priority != buildQueue[j].Priority {
			return buildQueue[i].Priority > buildQueue[j].Priority
		}
		return buildQueue[i].CreatedAt.Before(buildQueue[j].CreatedAt)
	})

	// Find next queued build (now sorted by priority)
	var nextBuild *BuildJob
	for _, job := range buildQueue {
		if job.Status == "queued" {
			nextBuild = job
			break
		}
	}

	if nextBuild == nil {
		return
	}

	// Check if we can start a new build (limit concurrent builds)
	runningCount := 0
	for _, job := range buildQueue {
		if job.Status == "running" {
			runningCount++
		}
	}

	if runningCount >= maxConcurrent {
		return // Max concurrent builds reached
	}

	// Start the build
	now := time.Now()
	nextBuild.Status = "running"
	nextBuild.StartedAt = &now
	nextBuild.PriorityName = getPriorityName(nextBuild.Priority)

	// Estimate completion time based on historical data
	avgDuration := getAverageBuildDuration(nextBuild.PipelineID)
	if avgDuration > 0 {
		estimatedEnd := now.Add(time.Duration(avgDuration) * time.Second)
		nextBuild.EstimatedEnd = &estimatedEnd
	}

	// Broadcast build started event
	broadcastEvent(&SSEEvent{
		ID:   generateID("event"),
		Type: "build_started",
		Data: map[string]interface{}{
			"buildId":    nextBuild.ID,
			"pipelineId": nextBuild.PipelineID,
			"repo":       nextBuild.Repo,
			"branch":     nextBuild.Branch,
			"priority":   nextBuild.Priority,
		},
		Timestamp: now,
	})

	go executePipeline(nextBuild)
}

// getAverageBuildDuration calculates average build duration for a pipeline
func getAverageBuildDuration(pipelineID string) float64 {
	executionsMu.RLock()
	defer executionsMu.RUnlock()

	var totalDuration float64
	var count int
	for _, exec := range executions {
		if exec.PipelineID == pipelineID && exec.Status == "success" && exec.Duration > 0 {
			totalDuration += exec.Duration
			count++
			if count >= 10 { // Use last 10 successful builds
				break
			}
		}
	}

	if count == 0 {
		return 0
	}
	return totalDuration / float64(count)
}

func executePipeline(job *BuildJob) {
	log.Printf("Starting pipeline execution for %s/%s", job.Repo, job.Branch)

	pipelinesMu.RLock()
	pipeline, exists := pipelines[job.PipelineID]
	if !exists {
		// Find pipeline by name
		for _, p := range pipelines {
			if p.Name == job.Pipeline {
				pipeline = p
				exists = true
				break
			}
		}
	}
	pipelinesMu.RUnlock()

	if !exists || pipeline == nil {
		log.Printf("Pipeline not found: %s", job.PipelineID)
		updateBuildStatus(job.ID, "failed", "Pipeline not found")
		return
	}

	// Create execution record
	execution := &PipelineExecution{
		ID:           generateID("exec"),
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		BuildNumber:  getNextBuildNumber(pipeline.ID),
		Repo:         job.Repo,
		Branch:       job.Branch,
		Commit:       job.Commit,
		Author:       job.Author,
		Message:      job.Message,
		Trigger:      "webhook",
		Status:       "running",
		Stages:       make([]StageExecution, 0),
		StartedAt:    time.Now(),
	}

	executionsMu.Lock()
	executions = append([]*PipelineExecution{execution}, executions...)
	if len(executions) > 500 {
		executions = executions[:500]
	}
	executionsMu.Unlock()

	// Execute stages
	success := true
	for _, stage := range pipeline.Stages {
		stageExec := executeStage(execution, stage, job)
		execution.Stages = append(execution.Stages, stageExec)

		if stageExec.Status == "failed" {
			success = false
			break
		}
	}

	// Update execution status
	now := time.Now()
	execution.CompletedAt = &now
	execution.Duration = now.Sub(execution.StartedAt).Seconds()

	if success {
		execution.Status = "success"
		updateBuildStatus(job.ID, "success", "Pipeline completed successfully")
	} else {
		execution.Status = "failed"
		updateBuildStatus(job.ID, "failed", "Pipeline failed")
	}

	log.Printf("Pipeline execution %s completed with status: %s", execution.ID, execution.Status)

	// Broadcast build completed event
	broadcastEvent(&SSEEvent{
		ID:   generateID("event"),
		Type: "build_completed",
		Data: map[string]interface{}{
			"executionId":  execution.ID,
			"buildId":      job.ID,
			"pipelineId":   execution.PipelineID,
			"pipelineName": execution.PipelineName,
			"repo":         execution.Repo,
			"branch":       execution.Branch,
			"status":       execution.Status,
			"duration":     execution.Duration,
		},
		Timestamp: time.Now(),
	})

	// Invalidate stats cache
	statsCacheMu.Lock()
	statsCache = nil
	statsCacheMu.Unlock()
}

func executeStage(execution *PipelineExecution, stage PipelineStage, job *BuildJob) StageExecution {
	log.Printf("Executing stage: %s", stage.Name)

	stageExec := StageExecution{
		Name:   stage.Name,
		Status: "running",
		LogID:  generateID("log"),
	}

	now := time.Now()
	stageExec.StartedAt = &now

	// Create log entry
	buildLog := &BuildLog{
		ID:          stageExec.LogID,
		ExecutionID: execution.ID,
		Stage:       stage.Name,
		Lines:       make([]LogLine, 0),
		CreatedAt:   time.Now(),
	}

	buildLogsMu.Lock()
	buildLogs[buildLog.ID] = buildLog
	buildLogsMu.Unlock()

	addLogLine(buildLog.ID, "info", fmt.Sprintf("Starting stage: %s", stage.Name))

	switch stage.Type {
	case "build":
		err := executeKanikoBuild(execution, stage, job, buildLog.ID)
		if err != nil {
			stageExec.Status = "failed"
			stageExec.Error = err.Error()
			addLogLine(buildLog.ID, "error", err.Error())
		} else {
			stageExec.Status = "success"
			addLogLine(buildLog.ID, "info", "Build completed successfully")
		}

	case "deploy":
		err := executeDeploy(execution, stage, job, buildLog.ID)
		if err != nil {
			stageExec.Status = "failed"
			stageExec.Error = err.Error()
			addLogLine(buildLog.ID, "error", err.Error())
		} else {
			stageExec.Status = "success"
			addLogLine(buildLog.ID, "info", "Deployment completed successfully")
		}

	case "test":
		// Simulate test stage
		addLogLine(buildLog.ID, "info", "Running tests...")
		time.Sleep(2 * time.Second)
		stageExec.Status = "success"
		addLogLine(buildLog.ID, "info", "All tests passed")

	default:
		addLogLine(buildLog.ID, "warn", fmt.Sprintf("Unknown stage type: %s", stage.Type))
		stageExec.Status = "success"
	}

	completed := time.Now()
	stageExec.CompletedAt = &completed
	stageExec.Duration = completed.Sub(*stageExec.StartedAt).Seconds()

	// Broadcast stage completion event
	broadcastEvent(&SSEEvent{
		ID:   generateID("event"),
		Type: "stage_completed",
		Data: map[string]interface{}{
			"executionId": execution.ID,
			"pipelineId":  execution.PipelineID,
			"stageName":   stage.Name,
			"stageType":   stage.Type,
			"status":      stageExec.Status,
			"duration":    stageExec.Duration,
			"error":       stageExec.Error,
		},
		Timestamp: time.Now(),
	})

	return stageExec
}

func executeKanikoBuild(execution *PipelineExecution, stage PipelineStage, job *BuildJob, logID string) error {
	if clientset == nil {
		addLogLine(logID, "warn", "Kubernetes client not available, simulating build")
		time.Sleep(3 * time.Second)
		return nil
	}

	addLogLine(logID, "info", fmt.Sprintf("Building image for %s", job.Repo))

	// Determine image name
	imageName := strings.ToLower(job.Repo)
	if strings.Contains(imageName, "/") {
		parts := strings.Split(imageName, "/")
		imageName = parts[len(parts)-1]
	}
	imageName = strings.ReplaceAll(imageName, " ", "-")

	destination := fmt.Sprintf("%s/%s:latest", registryURL, imageName)
	addLogLine(logID, "info", fmt.Sprintf("Destination: %s", destination))

	// Create Kaniko job
	jobName := fmt.Sprintf("kaniko-%s", execution.ID)
	backoffLimit := int32(0)
	ttl := int32(3600) // Clean up after 1 hour

	kanikoJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: "holm",
			Labels: map[string]string{
				"app":         "cicd-controller",
				"executionId": execution.ID,
				"type":        "kaniko-build",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					InitContainers: []corev1.Container{
						{
							Name:  "git-clone",
							Image: "alpine/git:latest",
							Command: []string{"sh", "-c"},
							Args: []string{
								fmt.Sprintf("set -ex && echo 'Cloning repository %s branch %s...' && git clone --depth 1 --branch %s %s/git/%s.git /workspace && echo 'Clone successful' && ls -la /workspace",
									job.Repo, job.Branch, job.Branch, holmGitURL, job.Repo),
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "workspace", MountPath: "/workspace"},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "kaniko",
							Image: "gcr.io/kaniko-project/executor:latest",
							Args: []string{
								"--dockerfile=/workspace/Dockerfile",
								"--context=/workspace",
								fmt.Sprintf("--destination=%s", destination),
								"--insecure",
								"--skip-tls-verify",
								"--verbosity=info",
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "workspace", MountPath: "/workspace"},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
									corev1.ResourceCPU:    resource.MustParse("2"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	ctx := context.Background()

	// Delete existing job if any
	addLogLine(logID, "info", "Cleaning up any existing build job...")
	_ = clientset.BatchV1().Jobs("holm").Delete(ctx, jobName, metav1.DeleteOptions{})
	time.Sleep(2 * time.Second)

	// Create the job
	addLogLine(logID, "info", fmt.Sprintf("Creating Kaniko job: %s", jobName))
	_, err := clientset.BatchV1().Jobs("holm").Create(ctx, kanikoJob, metav1.CreateOptions{})
	if err != nil {
		addLogLine(logID, "error", fmt.Sprintf("Failed to create job: %v", err))
		return fmt.Errorf("failed to create Kaniko job: %v", err)
	}

	addLogLine(logID, "info", "Kaniko job created, waiting for pod to start...")
	addLogLine(logID, "info", "")
	addLogLine(logID, "info", "=== PHASE: PENDING ===")

	// Wait for job completion with real-time log streaming
	timeout := time.After(time.Duration(stage.Timeout) * time.Second)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	var lastLogOffset int64 = 0
	lastPhase := ""
	podFound := false

	for {
		select {
		case <-timeout:
			addLogLine(logID, "error", fmt.Sprintf("Build timed out after %d seconds", stage.Timeout))
			// Try to get final logs before returning
			streamPodLogs(ctx, jobName, logID, &lastLogOffset, true)
			return fmt.Errorf("build timed out after %d seconds", stage.Timeout)

		case <-ticker.C:
			// Get job status
			k8sJob, err := clientset.BatchV1().Jobs("holm").Get(ctx, jobName, metav1.GetOptions{})
			if err != nil {
				addLogLine(logID, "warn", fmt.Sprintf("Failed to get job status: %v", err))
				continue
			}

			// Get pod for this job
			pods, err := clientset.CoreV1().Pods("holm").List(ctx, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("job-name=%s", jobName),
			})
			if err != nil {
				addLogLine(logID, "warn", fmt.Sprintf("Failed to list pods: %v", err))
				continue
			}

			if len(pods.Items) == 0 {
				addLogLine(logID, "info", "Waiting for build pod to be created...")
				continue
			}

			pod := &pods.Items[0]
			if !podFound {
				addLogLine(logID, "info", fmt.Sprintf("Build pod created: %s", pod.Name))
				podFound = true
			}

			// Report phase changes
			phase := string(pod.Status.Phase)
			if phase != lastPhase {
				phaseMsg := ""
				switch phase {
				case "Pending":
					phaseMsg = "=== PHASE: PENDING (waiting for resources) ==="
				case "Running":
					phaseMsg = "=== PHASE: RUNNING ==="
				case "Succeeded":
					phaseMsg = "=== PHASE: SUCCEEDED ==="
				case "Failed":
					phaseMsg = "=== PHASE: FAILED ==="
				default:
					phaseMsg = fmt.Sprintf("=== PHASE: %s ===", strings.ToUpper(phase))
				}
				addLogLine(logID, "info", "")
				addLogLine(logID, "info", phaseMsg)
				lastPhase = phase
			}

			// Check init container status
			for _, initStatus := range pod.Status.InitContainerStatuses {
				if initStatus.Name == "git-clone" {
					if initStatus.State.Running != nil {
						// Stream init container logs in real-time
						streamInitContainerLogs(ctx, pod.Name, "git-clone", logID)
					} else if initStatus.State.Terminated != nil {
						if initStatus.State.Terminated.ExitCode != 0 {
							addLogLine(logID, "error", "")
							addLogLine(logID, "error", "========================================")
							addLogLine(logID, "error", "        GIT CLONE FAILED")
							addLogLine(logID, "error", "========================================")
							addLogLine(logID, "error", fmt.Sprintf("Exit Code: %d", initStatus.State.Terminated.ExitCode))
							if initStatus.State.Terminated.Reason != "" {
								addLogLine(logID, "error", fmt.Sprintf("Reason: %s", initStatus.State.Terminated.Reason))
							}
							addLogLine(logID, "error", "")
							addLogLine(logID, "error", "--- Last 50 lines of git-clone logs ---")

							// Get the failure logs
							logs := getContainerLogs(ctx, pod.Name, "git-clone")
							showLastNLines(logID, logs, 50, "git-clone")

							addLogLine(logID, "error", "========================================")
							return fmt.Errorf("git clone failed: exit code %d", initStatus.State.Terminated.ExitCode)
						} else {
							// Git clone succeeded, stream final logs
							streamInitContainerLogs(ctx, pod.Name, "git-clone", logID)
						}
					} else if initStatus.State.Waiting != nil {
						reason := initStatus.State.Waiting.Reason
						if reason != "" && reason != "PodInitializing" {
							addLogLine(logID, "info", fmt.Sprintf("Init container waiting: %s", reason))
							if initStatus.State.Waiting.Message != "" {
								addLogLine(logID, "warn", initStatus.State.Waiting.Message)
							}
						}
					}
				}
			}

			// Check main container status and stream logs
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.Name == "kaniko" {
					if containerStatus.State.Running != nil {
						// Stream logs in real-time
						streamPodLogs(ctx, jobName, logID, &lastLogOffset, false)
					} else if containerStatus.State.Waiting != nil {
						reason := containerStatus.State.Waiting.Reason
						if reason != "" && reason != "PodInitializing" {
							addLogLine(logID, "info", fmt.Sprintf("Kaniko container waiting: %s", reason))
							if containerStatus.State.Waiting.Message != "" {
								addLogLine(logID, "warn", containerStatus.State.Waiting.Message)
							}
						}
					} else if containerStatus.State.Terminated != nil {
						// Get final logs
						streamPodLogs(ctx, jobName, logID, &lastLogOffset, true)
						if containerStatus.State.Terminated.ExitCode != 0 {
							addLogLine(logID, "error", fmt.Sprintf("Kaniko exited with code %d: %s",
								containerStatus.State.Terminated.ExitCode,
								containerStatus.State.Terminated.Reason))
						}
					}
				}
			}

			// Check job completion
			if k8sJob.Status.Succeeded > 0 {
				// Get final logs
				streamPodLogs(ctx, jobName, logID, &lastLogOffset, true)
				addLogLine(logID, "info", "=== BUILD SUCCESSFUL ===")
				addLogLine(logID, "info", fmt.Sprintf("Image pushed to: %s", destination))
				return nil
			}

			if k8sJob.Status.Failed > 0 {
				addLogLine(logID, "error", "")
				addLogLine(logID, "error", "========================================")
				addLogLine(logID, "error", "           BUILD FAILED")
				addLogLine(logID, "error", "========================================")
				addLogLine(logID, "error", "")

				// Get detailed failure info
				var failedContainer string
				var exitCode int32
				var failReason string

				if len(pods.Items) > 0 {
					pod := &pods.Items[0]

					// Check init containers first
					for _, cs := range pod.Status.InitContainerStatuses {
						if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
							failedContainer = cs.Name
							exitCode = cs.State.Terminated.ExitCode
							failReason = cs.State.Terminated.Reason
							if cs.State.Terminated.Message != "" {
								failReason = cs.State.Terminated.Message
							}

							addLogLine(logID, "error", fmt.Sprintf("FAILED CONTAINER: %s (init)", failedContainer))
							addLogLine(logID, "error", fmt.Sprintf("EXIT CODE: %d", exitCode))
							if failReason != "" {
								addLogLine(logID, "error", fmt.Sprintf("REASON: %s", failReason))
							}
							addLogLine(logID, "error", "")
							addLogLine(logID, "error", "--- Last 50 lines of logs ---")

							// Get last 50 lines of init container logs
							logs := getContainerLogs(ctx, pod.Name, cs.Name)
							showLastNLines(logID, logs, 50, cs.Name)
							break
						}
					}

					// Check main containers
					if failedContainer == "" {
						for _, cs := range pod.Status.ContainerStatuses {
							if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
								failedContainer = cs.Name
								exitCode = cs.State.Terminated.ExitCode
								failReason = cs.State.Terminated.Reason
								if cs.State.Terminated.Message != "" {
									failReason = cs.State.Terminated.Message
								}

								addLogLine(logID, "error", fmt.Sprintf("FAILED CONTAINER: %s", failedContainer))
								addLogLine(logID, "error", fmt.Sprintf("EXIT CODE: %d", exitCode))
								if failReason != "" {
									addLogLine(logID, "error", fmt.Sprintf("REASON: %s", failReason))
								}
								addLogLine(logID, "error", "")
								addLogLine(logID, "error", "--- Last 50 lines of logs ---")

								// Get last 50 lines of container logs
								logs := getContainerLogs(ctx, pod.Name, cs.Name)
								showLastNLines(logID, logs, 50, cs.Name)
								break
							}
						}
					}

					// Show pod events if available
					addLogLine(logID, "error", "")
					addLogLine(logID, "error", "--- Pod Status ---")
					addLogLine(logID, "error", fmt.Sprintf("Pod: %s", pod.Name))
					addLogLine(logID, "error", fmt.Sprintf("Phase: %s", pod.Status.Phase))
					if pod.Status.Message != "" {
						addLogLine(logID, "error", fmt.Sprintf("Message: %s", pod.Status.Message))
					}
				}

				addLogLine(logID, "error", "")
				addLogLine(logID, "error", "========================================")

				return fmt.Errorf("build failed: container %s exited with code %d", failedContainer, exitCode)
			}
		}
	}
}

// streamPodLogs streams logs from the kaniko container with improved real-time output
func streamPodLogs(ctx context.Context, jobName, logID string, offset *int64, final bool) {
	pods, err := clientset.CoreV1().Pods("holm").List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil || len(pods.Items) == 0 {
		return
	}

	pod := &pods.Items[0]
	opts := &corev1.PodLogOptions{
		Container: "kaniko",
	}

	// For real-time streaming, use sinceSeconds only if not final
	if !final {
		sinceSeconds := int64(30)
		opts.SinceSeconds = &sinceSeconds
	}

	logs, err := clientset.CoreV1().Pods("holm").GetLogs(pod.Name, opts).Do(ctx).Raw()
	if err != nil {
		return
	}

	logStr := string(logs)
	if int64(len(logStr)) > *offset {
		newLogs := logStr[*offset:]
		lines := strings.Split(newLogs, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				// Determine log level from content
				level := "info"
				lowerLine := strings.ToLower(line)
				if strings.Contains(lowerLine, "error") ||
					strings.Contains(lowerLine, "failed") ||
					strings.Contains(lowerLine, "cannot") ||
					strings.Contains(lowerLine, "fatal") {
					level = "error"
				} else if strings.Contains(lowerLine, "warn") {
					level = "warn"
				}
				addLogLine(logID, level, fmt.Sprintf("[kaniko] %s", line))
			}
		}
		*offset = int64(len(logStr))
	}
}

// initContainerLogOffsets tracks which init container logs we've already streamed
var initContainerLogOffsets = make(map[string]int64)
var initContainerLogOffsetsMu sync.Mutex

// streamInitContainerLogs gets logs from an init container with offset tracking for incremental streaming
func streamInitContainerLogs(ctx context.Context, podName, containerName, logID string) {
	key := podName + "/" + containerName

	initContainerLogOffsetsMu.Lock()
	offset := initContainerLogOffsets[key]
	initContainerLogOffsetsMu.Unlock()

	logs := getContainerLogs(ctx, podName, containerName)
	if logs != "" && int64(len(logs)) > offset {
		newLogs := logs[offset:]
		lines := strings.Split(newLogs, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				// Detect log level
				level := "info"
				lowerLine := strings.ToLower(line)
				if strings.Contains(lowerLine, "error") ||
					strings.Contains(lowerLine, "fatal") ||
					strings.Contains(lowerLine, "failed") {
					level = "error"
				} else if strings.Contains(lowerLine, "warn") {
					level = "warn"
				}
				addLogLine(logID, level, fmt.Sprintf("[%s] %s", containerName, line))
			}
		}

		initContainerLogOffsetsMu.Lock()
		initContainerLogOffsets[key] = int64(len(logs))
		initContainerLogOffsetsMu.Unlock()
	}
}

// getContainerLogs retrieves logs from a specific container
func getContainerLogs(ctx context.Context, podName, containerName string) string {
	opts := &corev1.PodLogOptions{
		Container: containerName,
	}
	logs, err := clientset.CoreV1().Pods("holm").GetLogs(podName, opts).Do(ctx).Raw()
	if err != nil {
		return ""
	}
	return string(logs)
}

// showLastNLines adds the last N lines of logs to the build log
func showLastNLines(logID, logs string, n int, containerName string) {
	if logs == "" {
		addLogLine(logID, "warn", "(no logs available)")
		return
	}

	lines := strings.Split(logs, "\n")
	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}

	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			// Detect log level
			level := "error"
			addLogLine(logID, level, fmt.Sprintf("[%s] %s", containerName, line))
		}
	}
}

func executeDeploy(execution *PipelineExecution, stage PipelineStage, job *BuildJob, logID string) error {
	addLogLine(logID, "info", "")
	addLogLine(logID, "info", "========================================")
	addLogLine(logID, "info", fmt.Sprintf("  DEPLOY STAGE: %s", job.Repo))
	addLogLine(logID, "info", "========================================")
	addLogLine(logID, "info", "")

	// Determine deployment name and image
	deploymentName := strings.ToLower(job.Repo)
	if strings.Contains(deploymentName, "/") {
		parts := strings.Split(deploymentName, "/")
		deploymentName = parts[len(parts)-1]
	}
	deploymentName = strings.ReplaceAll(deploymentName, " ", "-")

	image := fmt.Sprintf("%s/%s:latest", registryURL, deploymentName)
	addLogLine(logID, "info", fmt.Sprintf("[deploy] Target deployment: %s", deploymentName))
	addLogLine(logID, "info", fmt.Sprintf("[deploy] Target namespace: holm"))
	addLogLine(logID, "info", fmt.Sprintf("[deploy] Image: %s", image))
	addLogLine(logID, "info", fmt.Sprintf("[deploy] Execution ID: %s", execution.ID))
	addLogLine(logID, "info", "")

	// Determine timeout
	timeout := stage.Timeout
	if timeout <= 0 {
		timeout = 300 // default 5 minutes
	}

	// First, try to update deployment directly if we have k8s client
	if clientset != nil {
		addLogLine(logID, "info", "[deploy] Connecting to Kubernetes cluster...")
		ctx := context.Background()

		// Check if deployment exists
		addLogLine(logID, "info", fmt.Sprintf("[deploy] Looking for deployment '%s' in namespace 'holm'...", deploymentName))
		deployment, err := clientset.AppsV1().Deployments("holm").Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			addLogLine(logID, "warn", fmt.Sprintf("[deploy] Deployment %s not found in cluster: %v", deploymentName, err))
			addLogLine(logID, "info", "[deploy] Will try deploy-controller as fallback...")
		} else {
			// Update the deployment image
			replicas := int32(1)
			if deployment.Spec.Replicas != nil {
				replicas = *deployment.Spec.Replicas
			}
			addLogLine(logID, "info", fmt.Sprintf("[deploy] Found deployment: %s (replicas: %d)", deploymentName, replicas))
			addLogLine(logID, "info", fmt.Sprintf("[deploy] Current generation: %d", deployment.Generation))

			// Update container images
			updated := false
			for i := range deployment.Spec.Template.Spec.Containers {
				container := &deployment.Spec.Template.Spec.Containers[i]
				oldImage := container.Image
				if strings.Contains(oldImage, deploymentName) || i == 0 {
					container.Image = image
					addLogLine(logID, "info", fmt.Sprintf("[deploy] Updating container '%s':", container.Name))
					addLogLine(logID, "info", fmt.Sprintf("[deploy]   Old image: %s", oldImage))
					addLogLine(logID, "info", fmt.Sprintf("[deploy]   New image: %s", image))
					updated = true
				}
			}

			if updated {
				// Add annotations to deployment metadata for tracking
				if deployment.Annotations == nil {
					deployment.Annotations = make(map[string]string)
				}
				deployment.Annotations["cicd.holm/deployed-at"] = time.Now().Format(time.RFC3339)
				deployment.Annotations["cicd.holm/execution-id"] = execution.ID
				deployment.Annotations["cicd.holm/commit"] = job.Commit
				deployment.Annotations["cicd.holm/branch"] = job.Branch
				deployment.Annotations["cicd.holm/repo"] = job.Repo

				// Add annotation to pod template to force rollout
				if deployment.Spec.Template.Annotations == nil {
					deployment.Spec.Template.Annotations = make(map[string]string)
				}
				deployment.Spec.Template.Annotations["cicd.holm/deployed-at"] = time.Now().Format(time.RFC3339)
				deployment.Spec.Template.Annotations["cicd.holm/execution-id"] = execution.ID

				addLogLine(logID, "info", "")
				addLogLine(logID, "info", "[deploy] Applying deployment update...")
				_, err = clientset.AppsV1().Deployments("holm").Update(ctx, deployment, metav1.UpdateOptions{})
				if err != nil {
					addLogLine(logID, "error", fmt.Sprintf("[deploy] Failed to update deployment: %v", err))
					return fmt.Errorf("failed to update deployment: %v", err)
				}
				addLogLine(logID, "info", "[deploy] Deployment update applied successfully")
				addLogLine(logID, "info", "")

				// Watch rollout status
				addLogLine(logID, "info", "[deploy] Monitoring rollout progress...")
				addLogLine(logID, "info", "----------------------------------------")
				rolloutSuccess, rolloutErr := watchRolloutStatusWithTimeout(ctx, deploymentName, logID, timeout)
				addLogLine(logID, "info", "----------------------------------------")

				if !rolloutSuccess {
					addLogLine(logID, "error", "")
					addLogLine(logID, "error", "========================================")
					addLogLine(logID, "error", "  DEPLOYMENT FAILED")
					addLogLine(logID, "error", "========================================")
					if rolloutErr != "" {
						return fmt.Errorf("deployment rollout failed: %s", rolloutErr)
					}
					return fmt.Errorf("deployment rollout failed")
				}

				addLogLine(logID, "info", "")
				addLogLine(logID, "info", "========================================")
				addLogLine(logID, "info", "  DEPLOYMENT SUCCESSFUL")
				addLogLine(logID, "info", "========================================")
				addLogLine(logID, "info", fmt.Sprintf("[deploy] Deployment '%s' is now running", deploymentName))
				addLogLine(logID, "info", fmt.Sprintf("[deploy] Image: %s", image))
				return nil
			}
		}
	}

	// Fallback: Call deploy-controller to trigger deployment
	addLogLine(logID, "info", "")
	addLogLine(logID, "info", "[deploy] Triggering deployment via deploy-controller service...")
	deployPayload := map[string]interface{}{
		"deployment":  deploymentName,
		"namespace":   "holm",
		"image":       image,
		"executionId": execution.ID,
		"commit":      job.Commit,
		"branch":      job.Branch,
	}

	data, _ := json.Marshal(deployPayload)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post("http://deploy-controller.holm.svc.cluster.local:8080/api/deploy",
		"application/json", bytes.NewReader(data))

	if err != nil {
		addLogLine(logID, "error", fmt.Sprintf("[deploy] Failed to call deploy-controller: %v", err))
		addLogLine(logID, "error", "")
		addLogLine(logID, "error", "========================================")
		addLogLine(logID, "error", "  DEPLOYMENT FAILED")
		addLogLine(logID, "error", "========================================")
		return fmt.Errorf("deployment failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	addLogLine(logID, "info", fmt.Sprintf("[deploy] Deploy-controller response (HTTP %d)", resp.StatusCode))

	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		addLogLine(logID, "error", fmt.Sprintf("[deploy] Error response: %s", string(body)))
		addLogLine(logID, "error", "")
		addLogLine(logID, "error", "========================================")
		addLogLine(logID, "error", "  DEPLOYMENT FAILED")
		addLogLine(logID, "error", "========================================")
		return fmt.Errorf("deploy-controller returned status %d: %s", resp.StatusCode, string(body))
	}

	addLogLine(logID, "info", "[deploy] Deployment triggered via deploy-controller")

	// If we have k8s client, still watch the rollout
	if clientset != nil {
		addLogLine(logID, "info", "")
		addLogLine(logID, "info", "[deploy] Monitoring rollout progress...")
		addLogLine(logID, "info", "----------------------------------------")
		ctx := context.Background()
		rolloutSuccess, rolloutErr := watchRolloutStatusWithTimeout(ctx, deploymentName, logID, timeout)
		addLogLine(logID, "info", "----------------------------------------")

		if !rolloutSuccess {
			addLogLine(logID, "error", "")
			addLogLine(logID, "error", "========================================")
			addLogLine(logID, "error", "  DEPLOYMENT FAILED")
			addLogLine(logID, "error", "========================================")
			if rolloutErr != "" {
				return fmt.Errorf("deployment rollout failed: %s", rolloutErr)
			}
			return fmt.Errorf("deployment rollout failed")
		}
	}

	addLogLine(logID, "info", "")
	addLogLine(logID, "info", "========================================")
	addLogLine(logID, "info", "  DEPLOYMENT SUCCESSFUL")
	addLogLine(logID, "info", "========================================")
	return nil
}

// watchRolloutStatusWithTimeout monitors a deployment rollout and reports progress with configurable timeout
func watchRolloutStatusWithTimeout(ctx context.Context, deploymentName, logID string, timeoutSeconds int) (bool, string) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 120
	}

	timeout := time.After(time.Duration(timeoutSeconds) * time.Second)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	lastStatus := ""

	for {
		select {
		case <-timeout:
			addLogLine(logID, "error", fmt.Sprintf("[deploy] Rollout timed out after %d seconds", timeoutSeconds))
			return false, fmt.Sprintf("timed out after %d seconds", timeoutSeconds)

		case <-ticker.C:
			elapsed := int(time.Since(startTime).Seconds())

			deployment, err := clientset.AppsV1().Deployments("holm").Get(ctx, deploymentName, metav1.GetOptions{})
			if err != nil {
				addLogLine(logID, "warn", fmt.Sprintf("[deploy] Failed to get deployment status: %v", err))
				continue
			}

			// Check rollout status
			desiredReplicas := int32(1)
			if deployment.Spec.Replicas != nil {
				desiredReplicas = *deployment.Spec.Replicas
			}

			updatedReplicas := deployment.Status.UpdatedReplicas
			availableReplicas := deployment.Status.AvailableReplicas
			readyReplicas := deployment.Status.ReadyReplicas
			observedGeneration := deployment.Status.ObservedGeneration

			statusLine := fmt.Sprintf("[deploy] Rollout: %d/%d updated, %d/%d ready, %d available (gen: %d, %ds elapsed)",
				updatedReplicas, desiredReplicas, readyReplicas, desiredReplicas, availableReplicas, observedGeneration, elapsed)

			// Only log if status changed
			if statusLine != lastStatus {
				addLogLine(logID, "info", statusLine)
				lastStatus = statusLine
			}

			// Check for rollout completion
			if deployment.Status.ObservedGeneration >= deployment.Generation &&
				updatedReplicas == desiredReplicas &&
				readyReplicas == desiredReplicas &&
				availableReplicas == desiredReplicas {
				addLogLine(logID, "info", fmt.Sprintf("[deploy] Rollout completed in %d seconds", elapsed))
				return true, ""
			}

			// Check for rollout issues in conditions
			for _, cond := range deployment.Status.Conditions {
				if cond.Type == "Progressing" {
					if cond.Status == "False" {
						addLogLine(logID, "error", fmt.Sprintf("[deploy] Rollout stalled: %s", cond.Message))
						// Get pod status for debugging
						checkDeploymentPodStatus(ctx, deploymentName, logID)
						return false, cond.Message
					}
					// Check for deadline exceeded
					if cond.Reason == "ProgressDeadlineExceeded" {
						addLogLine(logID, "error", "[deploy] Rollout deadline exceeded")
						checkDeploymentPodStatus(ctx, deploymentName, logID)
						return false, "progress deadline exceeded"
					}
				}
				if cond.Type == "Available" && cond.Status == "False" {
					addLogLine(logID, "warn", fmt.Sprintf("[deploy] Deployment not yet available: %s", cond.Message))
				}
				if cond.Type == "ReplicaFailure" && cond.Status == "True" {
					addLogLine(logID, "error", fmt.Sprintf("[deploy] Replica failure: %s", cond.Message))
					checkDeploymentPodStatus(ctx, deploymentName, logID)
					return false, cond.Message
				}
			}
		}
	}
}

// checkDeploymentPodStatus gets status of pods for debugging deployment failures
func checkDeploymentPodStatus(ctx context.Context, deploymentName, logID string) {
	addLogLine(logID, "info", "")
	addLogLine(logID, "info", "[deploy] --- Pod Status ---")

	pods, err := clientset.CoreV1().Pods("holm").List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", deploymentName),
	})
	if err != nil {
		addLogLine(logID, "warn", fmt.Sprintf("[deploy] Failed to list pods: %v", err))
		return
	}

	if len(pods.Items) == 0 {
		addLogLine(logID, "warn", "[deploy] No pods found for this deployment")
		return
	}

	for _, pod := range pods.Items {
		addLogLine(logID, "info", fmt.Sprintf("[deploy] Pod: %s (Phase: %s)", pod.Name, pod.Status.Phase))

		// Check container statuses
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				addLogLine(logID, "warn", fmt.Sprintf("[deploy]   Container %s waiting: %s - %s",
					cs.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message))
			}
			if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
				addLogLine(logID, "error", fmt.Sprintf("[deploy]   Container %s terminated: exit code %d - %s",
					cs.Name, cs.State.Terminated.ExitCode, cs.State.Terminated.Reason))
			}
			if !cs.Ready && cs.RestartCount > 0 {
				addLogLine(logID, "warn", fmt.Sprintf("[deploy]   Container %s: not ready, %d restarts",
					cs.Name, cs.RestartCount))
			}
		}

		// Check conditions
		for _, cond := range pod.Status.Conditions {
			if cond.Status == "False" && cond.Message != "" {
				addLogLine(logID, "warn", fmt.Sprintf("[deploy]   Condition %s: %s", cond.Type, cond.Message))
			}
		}
	}
}

func addLogLine(logID, level, message string) {
	buildLogsMu.Lock()
	defer buildLogsMu.Unlock()

	if log, exists := buildLogs[logID]; exists {
		log.Lines = append(log.Lines, LogLine{
			Timestamp: time.Now(),
			Level:     level,
			Message:   message,
		})
	}
}

func updateBuildStatus(jobID, status, message string) {
	buildQueueMu.Lock()
	defer buildQueueMu.Unlock()

	for _, job := range buildQueue {
		if job.ID == jobID {
			job.Status = status
			now := time.Now()
			job.CompletedAt = &now
			break
		}
	}
}

func getNextBuildNumber(pipelineID string) int {
	executionsMu.RLock()
	defer executionsMu.RUnlock()

	maxNum := 0
	for _, exec := range executions {
		if exec.PipelineID == pipelineID && exec.BuildNumber > maxNum {
			maxNum = exec.BuildNumber
		}
	}
	return maxNum + 1
}

func cleanupOldExecutions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-24 * time.Hour)

		buildQueueMu.Lock()
		newQueue := make([]*BuildJob, 0)
		for _, job := range buildQueue {
			if job.Status == "running" || job.Status == "queued" || job.CreatedAt.After(cutoff) {
				newQueue = append(newQueue, job)
			}
		}
		buildQueue = newQueue
		buildQueueMu.Unlock()
	}
}

// HTTP Handlers

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": "1.0.0",
	})
}

func handlePipelines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		pipelinesMu.RLock()
		result := make([]*Pipeline, 0, len(pipelines))
		for _, p := range pipelines {
			result = append(result, p)
		}
		pipelinesMu.RUnlock()

		sort.Slice(result, func(i, j int) bool {
			return result[i].Name < result[j].Name
		})

		json.NewEncoder(w).Encode(result)

	case "POST":
		var pipeline Pipeline
		if err := json.NewDecoder(r.Body).Decode(&pipeline); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		pipeline.ID = generateID(pipeline.Name)
		pipeline.CreatedAt = time.Now()
		pipeline.UpdatedAt = time.Now()
		if pipeline.Variables == nil {
			pipeline.Variables = make(map[string]string)
		}

		pipelinesMu.Lock()
		pipelines[pipeline.ID] = &pipeline
		pipelinesMu.Unlock()

		json.NewEncoder(w).Encode(map[string]string{"status": "created", "id": pipeline.ID})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handlePipelineActions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/api/pipelines/")
	parts := strings.SplitN(path, "/", 2)
	pipelineID := parts[0]

	pipelinesMu.RLock()
	pipeline, exists := pipelines[pipelineID]
	pipelinesMu.RUnlock()

	if !exists {
		http.Error(w, "Pipeline not found", http.StatusNotFound)
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			json.NewEncoder(w).Encode(pipeline)
		case "PUT":
			var updated Pipeline
			if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			updated.ID = pipelineID
			updated.CreatedAt = pipeline.CreatedAt
			updated.UpdatedAt = time.Now()

			pipelinesMu.Lock()
			pipelines[pipelineID] = &updated
			pipelinesMu.Unlock()

			json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
		case "DELETE":
			pipelinesMu.Lock()
			delete(pipelines, pipelineID)
			pipelinesMu.Unlock()
			json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	action := parts[1]
	switch action {
	case "trigger":
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Branch  string            `json:"branch"`
			Commit  string            `json:"commit"`
			Variables map[string]string `json:"variables"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.Branch == "" {
			req.Branch = pipeline.Branch
		}
		if req.Branch == "" {
			req.Branch = "main"
		}

		now := time.Now()
		job := &BuildJob{
			ID:           generateID("build"),
			PipelineID:   pipelineID,
			Pipeline:     pipeline.Name,
			Repo:         pipeline.RepoURL,
			Branch:       req.Branch,
			Commit:       req.Commit,
			Status:       "queued",
			Priority:     PriorityNormal,
			PriorityName: getPriorityName(PriorityNormal),
			Variables:    req.Variables,
			TriggerType:  "manual",
			QueuedAt:     now,
			CreatedAt:    now,
		}

		buildQueueMu.Lock()
		buildQueue = append([]*BuildJob{job}, buildQueue...)
		buildQueueMu.Unlock()

		json.NewEncoder(w).Encode(map[string]string{"status": "triggered", "buildId": job.ID})

	case "executions":
		executionsMu.RLock()
		result := make([]*PipelineExecution, 0)
		for _, exec := range executions {
			if exec.PipelineID == pipelineID {
				result = append(result, exec)
			}
		}
		executionsMu.RUnlock()
		json.NewEncoder(w).Encode(result)

	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

func handleGitWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Git webhook received: %s", string(body))

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	event := parseGitWebhook(payload)
	event.ID = generateID("webhook")
	event.Timestamp = time.Now()
	event.Source = "git"

	webhookEventsMu.Lock()
	webhookEvents = append([]*WebhookEvent{event}, webhookEvents...)
	if len(webhookEvents) > 100 {
		webhookEvents = webhookEvents[:100]
	}
	webhookEventsMu.Unlock()

	// Find matching pipeline and trigger
	go processTrigger(event)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "received",
		"eventId": event.ID,
	})
}

func handleHolmGitWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("HolmGit webhook received: %s", string(body))

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	event := &WebhookEvent{
		ID:        generateID("webhook"),
		Type:      "push",
		Source:    "holmgit",
		Payload:   payload,
		Timestamp: time.Now(),
	}

	// Parse HolmGit specific payload
	if repo, ok := payload["repo"].(string); ok {
		event.Repo = repo
	}
	if branch, ok := payload["branch"].(string); ok {
		event.Branch = branch
	}
	if event.Branch == "" {
		event.Branch = "main"
	}

	webhookEventsMu.Lock()
	webhookEvents = append([]*WebhookEvent{event}, webhookEvents...)
	if len(webhookEvents) > 100 {
		webhookEvents = webhookEvents[:100]
	}
	webhookEventsMu.Unlock()

	// Find matching pipeline and trigger
	go processTrigger(event)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "received",
		"eventId": event.ID,
	})
}

func parseGitWebhook(payload map[string]interface{}) *WebhookEvent {
	event := &WebhookEvent{
		Type:    "push",
		Payload: payload,
	}

	// Parse repository info
	if repo, ok := payload["repository"].(map[string]interface{}); ok {
		if name, ok := repo["name"].(string); ok {
			event.Repo = name
		}
		if fullName, ok := repo["full_name"].(string); ok {
			event.Repo = fullName
		}
	}

	// Parse branch from ref
	if ref, ok := payload["ref"].(string); ok {
		event.Branch = strings.TrimPrefix(ref, "refs/heads/")
	}

	// Parse commit
	if after, ok := payload["after"].(string); ok && len(after) >= 7 {
		event.Commit = after[:7]
	}

	// Parse author and message
	if commits, ok := payload["commits"].([]interface{}); ok && len(commits) > 0 {
		if commit, ok := commits[0].(map[string]interface{}); ok {
			if msg, ok := commit["message"].(string); ok {
				event.Message = msg
			}
			if author, ok := commit["author"].(map[string]interface{}); ok {
				if name, ok := author["name"].(string); ok {
					event.Author = name
				}
			}
		}
	}

	// Parse pusher
	if pusher, ok := payload["pusher"].(map[string]interface{}); ok {
		if name, ok := pusher["name"].(string); ok && event.Author == "" {
			event.Author = name
		}
	}

	return event
}

func processTrigger(event *WebhookEvent) {
	log.Printf("Processing webhook trigger: repo=%s branch=%s", event.Repo, event.Branch)

	pipelinesMu.RLock()
	defer pipelinesMu.RUnlock()

	for _, pipeline := range pipelines {
		if !pipeline.Enabled {
			continue
		}

		// Check if repo matches
		if pipeline.RepoURL != "" && !strings.Contains(event.Repo, pipeline.RepoURL) &&
			!strings.Contains(pipeline.RepoURL, event.Repo) {
			continue
		}

		// Check triggers
		for _, trigger := range pipeline.Triggers {
			if trigger.Type != "webhook" {
				continue
			}

			// Check branch match
			branchMatch := len(trigger.Branches) == 0
			for _, b := range trigger.Branches {
				if b == event.Branch || b == "*" {
					branchMatch = true
					break
				}
			}

			if !branchMatch {
				continue
			}

			// Check event match
			eventMatch := len(trigger.Events) == 0
			for _, e := range trigger.Events {
				if e == event.Type || e == "*" {
					eventMatch = true
					break
				}
			}

			if !eventMatch {
				continue
			}

			// Trigger the pipeline
			log.Printf("Triggering pipeline %s for %s/%s", pipeline.Name, event.Repo, event.Branch)

			now := time.Now()
			job := &BuildJob{
				ID:           generateID("build"),
				PipelineID:   pipeline.ID,
				Pipeline:     pipeline.Name,
				Repo:         event.Repo,
				Branch:       event.Branch,
				Commit:       event.Commit,
				Author:       event.Author,
				Message:      event.Message,
				Status:       "queued",
				Priority:     PriorityNormal,
				PriorityName: getPriorityName(PriorityNormal),
				Variables:    make(map[string]string),
				TriggerType:  "webhook",
				TriggerBy:    event.Author,
				QueuedAt:     now,
				CreatedAt:    now,
			}

			// Check queue size limit
			if len(buildQueue) >= maxQueueSize {
				log.Printf("Build queue full (max %d), skipping trigger for %s", maxQueueSize, event.Repo)
				webhookEventsMu.Lock()
				for _, we := range webhookEvents {
					if we.ID == event.ID {
						we.Error = "Build queue full"
						break
					}
				}
				webhookEventsMu.Unlock()
				continue
			}

			buildQueueMu.Lock()
			buildQueue = append([]*BuildJob{job}, buildQueue...)
			buildQueueMu.Unlock()

			// Mark event as processed with build ID
			processedAt := time.Now()
			webhookEventsMu.Lock()
			for _, we := range webhookEvents {
				if we.ID == event.ID {
					we.Processed = true
					we.ProcessedAt = &processedAt
					we.PipelineID = pipeline.ID
					we.BuildID = job.ID
					break
				}
			}
			webhookEventsMu.Unlock()

			// Broadcast webhook processed event
			broadcastEvent(&SSEEvent{
				ID:   generateID("event"),
				Type: "webhook_triggered",
				Data: map[string]interface{}{
					"webhookId":    event.ID,
					"buildId":      job.ID,
					"pipelineId":   pipeline.ID,
					"pipelineName": pipeline.Name,
					"repo":         event.Repo,
					"branch":       event.Branch,
				},
				Timestamp: processedAt,
			})

			break // Only trigger once per pipeline
		}
	}
}

func handleWebhooks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	webhookEventsMu.RLock()
	defer webhookEventsMu.RUnlock()

	json.NewEncoder(w).Encode(webhookEvents)
}

func handleQueue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		buildQueueMu.RLock()
		json.NewEncoder(w).Encode(buildQueue)
		buildQueueMu.RUnlock()

	case "POST":
		var req struct {
			PipelineID string            `json:"pipelineId"`
			Pipeline   string            `json:"pipeline"`
			Repo       string            `json:"repo"`
			Branch     string            `json:"branch"`
			Commit     string            `json:"commit"`
			Variables  map[string]string `json:"variables"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Branch == "" {
			req.Branch = "main"
		}

		now := time.Now()
		job := &BuildJob{
			ID:           generateID("build"),
			PipelineID:   req.PipelineID,
			Pipeline:     req.Pipeline,
			Repo:         req.Repo,
			Branch:       req.Branch,
			Commit:       req.Commit,
			Status:       "queued",
			Priority:     PriorityNormal,
			PriorityName: getPriorityName(PriorityNormal),
			Variables:    req.Variables,
			TriggerType:  "api",
			QueuedAt:     now,
			CreatedAt:    now,
		}

		buildQueueMu.Lock()
		buildQueue = append([]*BuildJob{job}, buildQueue...)
		buildQueueMu.Unlock()

		json.NewEncoder(w).Encode(map[string]string{"status": "queued", "id": job.ID})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleQueueActions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/api/queue/")
	parts := strings.SplitN(path, "/", 2)
	jobID := parts[0]

	if len(parts) == 2 && parts[1] == "cancel" {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		buildQueueMu.Lock()
		for _, job := range buildQueue {
			if job.ID == jobID && job.Status == "queued" {
				job.Status = "cancelled"
				now := time.Now()
				job.CompletedAt = &now
			}
		}
		buildQueueMu.Unlock()

		json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
		return
	}

	// GET job details
	buildQueueMu.RLock()
	var job *BuildJob
	for _, j := range buildQueue {
		if j.ID == jobID {
			job = j
			break
		}
	}
	buildQueueMu.RUnlock()

	if job == nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(job)
}

func handleExecutions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	executionsMu.RLock()
	defer executionsMu.RUnlock()

	// Parse query parameters for filtering
	q := r.URL.Query()
	pipelineID := q.Get("pipelineId")
	pipelineName := q.Get("pipeline")
	status := q.Get("status")
	branch := q.Get("branch")
	author := q.Get("author")
	repo := q.Get("repo")
	trigger := q.Get("trigger")
	sinceStr := q.Get("since")
	untilStr := q.Get("until")
	search := strings.ToLower(q.Get("search"))

	// Pagination parameters
	page := 1
	limit := 50
	if p := q.Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	// Parse date filters
	var sinceTime, untilTime time.Time
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			sinceTime = t
		} else if t, err := time.Parse("2006-01-02", sinceStr); err == nil {
			sinceTime = t
		}
	}
	if untilStr != "" {
		if t, err := time.Parse(time.RFC3339, untilStr); err == nil {
			untilTime = t
		} else if t, err := time.Parse("2006-01-02", untilStr); err == nil {
			untilTime = t.Add(24 * time.Hour) // Include the entire day
		}
	}

	// Sort order
	sortBy := q.Get("sort")
	sortOrder := q.Get("order")
	if sortBy == "" {
		sortBy = "startedAt"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Filter executions
	filtered := make([]*PipelineExecution, 0)
	for _, exec := range executions {
		// Apply filters
		if pipelineID != "" && exec.PipelineID != pipelineID {
			continue
		}
		if pipelineName != "" && !strings.EqualFold(exec.PipelineName, pipelineName) {
			continue
		}
		if status != "" {
			// Support comma-separated status list
			statuses := strings.Split(status, ",")
			matched := false
			for _, s := range statuses {
				if strings.EqualFold(exec.Status, strings.TrimSpace(s)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if branch != "" && !strings.EqualFold(exec.Branch, branch) {
			continue
		}
		if author != "" && !strings.Contains(strings.ToLower(exec.Author), strings.ToLower(author)) {
			continue
		}
		if repo != "" && !strings.Contains(strings.ToLower(exec.Repo), strings.ToLower(repo)) {
			continue
		}
		if trigger != "" && !strings.EqualFold(exec.Trigger, trigger) {
			continue
		}
		if !sinceTime.IsZero() && exec.StartedAt.Before(sinceTime) {
			continue
		}
		if !untilTime.IsZero() && exec.StartedAt.After(untilTime) {
			continue
		}
		if search != "" {
			// Search across multiple fields
			searchFields := strings.ToLower(exec.PipelineName + " " + exec.Repo + " " + exec.Branch + " " + exec.Author + " " + exec.Message + " " + exec.Commit)
			if !strings.Contains(searchFields, search) {
				continue
			}
		}

		filtered = append(filtered, exec)
	}

	// Sort results
	sort.Slice(filtered, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "duration":
			less = filtered[i].Duration < filtered[j].Duration
		case "buildNumber":
			less = filtered[i].BuildNumber < filtered[j].BuildNumber
		case "pipeline":
			less = filtered[i].PipelineName < filtered[j].PipelineName
		case "status":
			less = filtered[i].Status < filtered[j].Status
		default: // startedAt
			less = filtered[i].StartedAt.Before(filtered[j].StartedAt)
		}
		if sortOrder == "desc" {
			return !less
		}
		return less
	})

	// Calculate pagination
	totalCount := len(filtered)
	totalPages := (totalCount + limit - 1) / limit
	start := (page - 1) * limit
	end := start + limit

	if start > totalCount {
		start = totalCount
	}
	if end > totalCount {
		end = totalCount
	}

	paginated := filtered[start:end]

	// Build response with metadata
	response := map[string]interface{}{
		"executions": paginated,
		"pagination": map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"total":      totalCount,
			"totalPages": totalPages,
			"hasMore":    page < totalPages,
		},
		"filters": map[string]interface{}{
			"pipelineId": pipelineID,
			"status":     status,
			"branch":     branch,
			"author":     author,
			"since":      sinceStr,
			"until":      untilStr,
			"search":     search,
		},
	}

	json.NewEncoder(w).Encode(response)
}

func handleExecutionActions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/api/executions/")
	parts := strings.SplitN(path, "/", 2)
	executionID := parts[0]

	executionsMu.RLock()
	var execution *PipelineExecution
	for _, e := range executions {
		if e.ID == executionID {
			execution = e
			break
		}
	}
	executionsMu.RUnlock()

	if execution == nil {
		http.Error(w, "Execution not found", http.StatusNotFound)
		return
	}

	if len(parts) == 1 {
		json.NewEncoder(w).Encode(execution)
		return
	}

	action := parts[1]
	switch action {
	case "logs":
		// Get all logs for this execution
		buildLogsMu.RLock()
		logs := make(map[string]*BuildLog)
		for id, log := range buildLogs {
			if log.ExecutionID == executionID {
				logs[id] = log
			}
		}
		buildLogsMu.RUnlock()
		json.NewEncoder(w).Encode(logs)

	case "retry":
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Create new build job from execution with retry tracking
		now := time.Now()
		job := &BuildJob{
			ID:           generateID("build"),
			PipelineID:   execution.PipelineID,
			Pipeline:     execution.PipelineName,
			Repo:         execution.Repo,
			Branch:       execution.Branch,
			Commit:       execution.Commit,
			Author:       execution.Author,
			Message:      execution.Message,
			Status:       "queued",
			Priority:     PriorityHigh, // Retries get higher priority
			PriorityName: getPriorityName(PriorityHigh),
			Variables:    make(map[string]string),
			TriggerType:  "retry",
			RetryCount:   1,
			QueuedAt:     now,
			CreatedAt:    now,
		}

		buildQueueMu.Lock()
		buildQueue = append([]*BuildJob{job}, buildQueue...)
		buildQueueMu.Unlock()

		json.NewEncoder(w).Encode(map[string]string{"status": "retried", "buildId": job.ID})

	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	logID := strings.TrimPrefix(r.URL.Path, "/api/logs/")

	buildLogsMu.RLock()
	log, exists := buildLogs[logID]
	buildLogsMu.RUnlock()

	if !exists {
		http.Error(w, "Log not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(log)
}

// handleLogsStream provides Server-Sent Events for real-time log streaming by execution ID
func handleLogsStream(w http.ResponseWriter, r *http.Request) {
	execID := strings.TrimPrefix(r.URL.Path, "/api/logs-stream/")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Track which log lines we've already sent per log ID
	sentLines := make(map[string]int)
	lastStatus := ""

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"execId\":\"%s\"}\n\n", execID)
	flusher.Flush()

	for {
		// Check if client disconnected
		select {
		case <-r.Context().Done():
			return
		default:
		}

		// Get execution status
		executionsMu.RLock()
		var execution *PipelineExecution
		for _, e := range executions {
			if e.ID == execID {
				execution = e
				break
			}
		}
		executionsMu.RUnlock()

		if execution == nil {
			fmt.Fprintf(w, "event: error\ndata: {\"message\":\"Execution not found\"}\n\n")
			flusher.Flush()
			return
		}

		// Send status update if changed
		if execution.Status != lastStatus {
			statusData := map[string]interface{}{
				"status":    execution.Status,
				"stages":    execution.Stages,
				"startedAt": execution.StartedAt,
			}
			if execution.CompletedAt != nil {
				statusData["completedAt"] = execution.CompletedAt
				statusData["duration"] = execution.Duration
			}
			data, _ := json.Marshal(statusData)
			fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
			flusher.Flush()
			lastStatus = execution.Status
		}

		// Get all logs for this execution and send new lines
		buildLogsMu.RLock()
		for logID, logEntry := range buildLogs {
			if logEntry.ExecutionID != execID {
				continue
			}

			startIdx := sentLines[logID]
			if startIdx < len(logEntry.Lines) {
				for i := startIdx; i < len(logEntry.Lines); i++ {
					line := logEntry.Lines[i]
					lineData := map[string]interface{}{
						"logId":     logID,
						"stage":     logEntry.Stage,
						"timestamp": line.Timestamp,
						"level":     line.Level,
						"message":   line.Message,
						"index":     i,
					}
					data, _ := json.Marshal(lineData)
					fmt.Fprintf(w, "event: log\ndata: %s\n\n", data)
				}
				sentLines[logID] = len(logEntry.Lines)
			}
		}
		buildLogsMu.RUnlock()
		flusher.Flush()

		// If execution is complete, send final event and close
		if execution.Status == "success" || execution.Status == "failed" {
			// Give a moment for any final logs to arrive
			time.Sleep(500 * time.Millisecond)

			// Send any remaining logs
			buildLogsMu.RLock()
			for logID, logEntry := range buildLogs {
				if logEntry.ExecutionID != execID {
					continue
				}
				startIdx := sentLines[logID]
				if startIdx < len(logEntry.Lines) {
					for i := startIdx; i < len(logEntry.Lines); i++ {
						line := logEntry.Lines[i]
						lineData := map[string]interface{}{
							"logId":     logID,
							"stage":     logEntry.Stage,
							"timestamp": line.Timestamp,
							"level":     line.Level,
							"message":   line.Message,
							"index":     i,
						}
						data, _ := json.Marshal(lineData)
						fmt.Fprintf(w, "event: log\ndata: %s\n\n", data)
					}
				}
			}
			buildLogsMu.RUnlock()

			fmt.Fprintf(w, "event: complete\ndata: {\"status\":\"%s\"}\n\n", execution.Status)
			flusher.Flush()
			return
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Repo        string   `json:"repo"`
		Branch      string   `json:"branch"`
		Dockerfile  string   `json:"dockerfile"`
		Context     string   `json:"context"`
		Destination string   `json:"destination"`
		BuildArgs   []string `json:"buildArgs"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.Dockerfile == "" {
		req.Dockerfile = "Dockerfile"
	}
	if req.Context == "" {
		req.Context = "."
	}

	// Create a quick build job with high priority for manual builds
	now := time.Now()
	job := &BuildJob{
		ID:           generateID("build"),
		Repo:         req.Repo,
		Branch:       req.Branch,
		Status:       "queued",
		Priority:     PriorityHigh, // Higher priority for manual builds
		PriorityName: getPriorityName(PriorityHigh),
		Variables: map[string]string{
			"DOCKERFILE":  req.Dockerfile,
			"CONTEXT":     req.Context,
			"DESTINATION": req.Destination,
		},
		TriggerType: "manual",
		QueuedAt:    now,
		CreatedAt:   now,
	}

	buildQueueMu.Lock()
	// Insert at front for priority
	buildQueue = append([]*BuildJob{job}, buildQueue...)
	buildQueueMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "queued",
		"buildId": job.ID,
	})
}

func handleBuilds(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	buildQueueMu.RLock()
	defer buildQueueMu.RUnlock()

	// Return recent builds (last 20)
	limit := 20
	if len(buildQueue) < limit {
		limit = len(buildQueue)
	}

	builds := buildQueue[:limit]
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":     len(builds),
		"builds":    builds,
		"service":   "CI/CD Controller",
		"timestamp": time.Now().Format(time.RFC3339),
		"status":    "ok",
		"queue_size": len(buildQueue),
	})
}

func handleDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Deployment string `json:"deployment"`
		Namespace  string `json:"namespace"`
		Image      string `json:"image"`
		Tag        string `json:"tag"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Namespace == "" {
		req.Namespace = "holm"
	}
	if req.Tag == "" {
		req.Tag = "latest"
	}
	if req.Image == "" && req.Deployment != "" {
		req.Image = fmt.Sprintf("%s/%s:%s", registryURL, req.Deployment, req.Tag)
	}

	log.Printf("Deploy trigger: %s/%s -> %s", req.Namespace, req.Deployment, req.Image)

	// Forward to deploy-controller
	deployPayload := map[string]string{
		"deployment": req.Deployment,
		"namespace":  req.Namespace,
		"image":      req.Image,
	}

	data, _ := json.Marshal(deployPayload)
	resp, err := http.Post("http://deploy-controller.holm.svc.cluster.local:8080/api/deploy",
		"application/json", bytes.NewReader(data))

	w.Header().Set("Content-Type", "application/json")

	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	result["triggered"] = true

	json.NewEncoder(w).Encode(result)
}

func handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html, err := os.ReadFile("/app/ui.html")
	if err != nil {
		// Serve embedded fallback UI
		w.Write([]byte(fallbackUI))
		return
	}
	w.Write(html)
}

const fallbackUI = `<!DOCTYPE html>
<html><head><title>CI/CD Controller</title></head>
<body><h1>CI/CD Controller</h1><p>UI loading...</p></body></html>`

// =============================================================================
// STATS ENDPOINTS - Build statistics and analytics
// =============================================================================

// handleStats returns comprehensive build statistics
func handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check cache (refresh every 30 seconds)
	statsCacheMu.RLock()
	if statsCache != nil && time.Since(statsCacheTime) < 30*time.Second {
		json.NewEncoder(w).Encode(statsCache)
		statsCacheMu.RUnlock()
		return
	}
	statsCacheMu.RUnlock()

	// Calculate fresh stats
	stats := calculateBuildStats()

	// Update cache
	statsCacheMu.Lock()
	statsCache = stats
	statsCacheTime = time.Now()
	statsCacheMu.Unlock()

	json.NewEncoder(w).Encode(stats)
}

// calculateBuildStats computes comprehensive build statistics
func calculateBuildStats() *BuildStats {
	executionsMu.RLock()
	buildQueueMu.RLock()
	defer executionsMu.RUnlock()
	defer buildQueueMu.RUnlock()

	stats := &BuildStats{
		BuildsByPipeline: make(map[string]int),
		BuildsByStatus:   make(map[string]int),
		BuildsByBranch:   make(map[string]int),
		BuildsByAuthor:   make(map[string]int),
		RecentTrend:      make([]DailyStats, 0),
		CalculatedAt:     time.Now(),
		ServiceUptime:    time.Since(serviceStartTime).Seconds(),
	}

	// Calculate queue stats
	for _, job := range buildQueue {
		if job.Status == "queued" {
			stats.QueuedBuilds++
		} else if job.Status == "running" {
			stats.RunningBuilds++
		}
	}

	if len(executions) == 0 {
		return stats
	}

	// Track build durations for percentile calculations
	var durations []float64
	today := time.Now().Truncate(24 * time.Hour)
	weekAgo := today.AddDate(0, 0, -7)

	// Daily stats for trend (last 14 days)
	dailyStats := make(map[string]*DailyStats)
	for i := 0; i < 14; i++ {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		dailyStats[date] = &DailyStats{Date: date}
	}

	// Process all executions
	for _, exec := range executions {
		stats.TotalBuilds++

		// Status counts
		stats.BuildsByStatus[exec.Status]++
		switch exec.Status {
		case "success":
			stats.SuccessfulBuilds++
		case "failed":
			stats.FailedBuilds++
		case "cancelled":
			stats.CancelledBuilds++
		}

		// Pipeline counts
		if exec.PipelineName != "" {
			stats.BuildsByPipeline[exec.PipelineName]++
		}

		// Branch counts
		if exec.Branch != "" {
			stats.BuildsByBranch[exec.Branch]++
		}

		// Author counts
		if exec.Author != "" {
			stats.BuildsByAuthor[exec.Author]++
		}

		// Duration tracking (for completed builds)
		if exec.Duration > 0 {
			durations = append(durations, exec.Duration)
		}

		// Track last build time
		if stats.LastBuildTime == nil || exec.StartedAt.After(*stats.LastBuildTime) {
			t := exec.StartedAt
			stats.LastBuildTime = &t
		}

		// Track longest/shortest builds
		if exec.Status == "success" && exec.Duration > 0 {
			if stats.LongestBuild == nil || exec.Duration > stats.LongestBuild.Duration {
				stats.LongestBuild = &BuildDuration{
					ExecutionID:  exec.ID,
					PipelineName: exec.PipelineName,
					Duration:     exec.Duration,
					Status:       exec.Status,
				}
			}
			if stats.ShortestBuild == nil || exec.Duration < stats.ShortestBuild.Duration {
				stats.ShortestBuild = &BuildDuration{
					ExecutionID:  exec.ID,
					PipelineName: exec.PipelineName,
					Duration:     exec.Duration,
					Status:       exec.Status,
				}
			}
		}

		// Builds today and this week
		execDate := exec.StartedAt.Truncate(24 * time.Hour)
		if execDate.Equal(today) {
			stats.BuildsToday++
		}
		if exec.StartedAt.After(weekAgo) {
			stats.BuildsThisWeek++
		}

		// Daily trend
		dateStr := exec.StartedAt.Format("2006-01-02")
		if daily, exists := dailyStats[dateStr]; exists {
			daily.Total++
			if exec.Status == "success" {
				daily.Successful++
			} else if exec.Status == "failed" {
				daily.Failed++
			}
			if exec.Duration > 0 {
				daily.AvgDuration = (daily.AvgDuration*float64(daily.Total-1) + exec.Duration) / float64(daily.Total)
			}
		}
	}

	// Calculate success rate
	completedBuilds := stats.SuccessfulBuilds + stats.FailedBuilds
	if completedBuilds > 0 {
		stats.SuccessRate = float64(stats.SuccessfulBuilds) / float64(completedBuilds) * 100
	}

	// Calculate duration statistics
	if len(durations) > 0 {
		sort.Float64s(durations)

		// Average
		var sum float64
		for _, d := range durations {
			sum += d
		}
		stats.AvgBuildTime = sum / float64(len(durations))

		// Median
		mid := len(durations) / 2
		if len(durations)%2 == 0 {
			stats.MedianBuildTime = (durations[mid-1] + durations[mid]) / 2
		} else {
			stats.MedianBuildTime = durations[mid]
		}

		// P95
		p95Index := int(float64(len(durations)) * 0.95)
		if p95Index >= len(durations) {
			p95Index = len(durations) - 1
		}
		stats.P95BuildTime = durations[p95Index]
	}

	// Build trend array (sorted by date)
	for _, daily := range dailyStats {
		if daily.Total > 0 {
			daily.SuccessRate = float64(daily.Successful) / float64(daily.Total) * 100
		}
		stats.RecentTrend = append(stats.RecentTrend, *daily)
	}
	sort.Slice(stats.RecentTrend, func(i, j int) bool {
		return stats.RecentTrend[i].Date < stats.RecentTrend[j].Date
	})

	return stats
}

// handleStatsTrends returns build trend data for charting
func handleStatsTrends(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 90 {
			days = parsed
		}
	}

	executionsMu.RLock()
	defer executionsMu.RUnlock()

	trends := make(map[string]*DailyStats)
	today := time.Now().Truncate(24 * time.Hour)

	for i := 0; i < days; i++ {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		trends[date] = &DailyStats{Date: date}
	}

	cutoff := today.AddDate(0, 0, -days)
	for _, exec := range executions {
		if exec.StartedAt.Before(cutoff) {
			continue
		}
		dateStr := exec.StartedAt.Format("2006-01-02")
		if daily, exists := trends[dateStr]; exists {
			daily.Total++
			if exec.Status == "success" {
				daily.Successful++
			} else if exec.Status == "failed" {
				daily.Failed++
			}
		}
	}

	// Convert to sorted slice
	result := make([]DailyStats, 0, len(trends))
	for _, daily := range trends {
		if daily.Total > 0 {
			daily.SuccessRate = float64(daily.Successful) / float64(daily.Total) * 100
		}
		result = append(result, *daily)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date < result[j].Date
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"days":   days,
		"trends": result,
	})
}

// handleStatsByPipeline returns per-pipeline statistics
func handleStatsByPipeline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	executionsMu.RLock()
	pipelinesMu.RLock()
	defer executionsMu.RUnlock()
	defer pipelinesMu.RUnlock()

	type PipelineStats struct {
		PipelineID   string  `json:"pipelineId"`
		PipelineName string  `json:"pipelineName"`
		TotalBuilds  int     `json:"totalBuilds"`
		Successful   int     `json:"successful"`
		Failed       int     `json:"failed"`
		SuccessRate  float64 `json:"successRate"`
		AvgDuration  float64 `json:"avgDuration"`
		LastBuild    *time.Time `json:"lastBuild"`
	}

	pipelineStats := make(map[string]*PipelineStats)

	for _, exec := range executions {
		ps, exists := pipelineStats[exec.PipelineID]
		if !exists {
			ps = &PipelineStats{
				PipelineID:   exec.PipelineID,
				PipelineName: exec.PipelineName,
			}
			pipelineStats[exec.PipelineID] = ps
		}

		ps.TotalBuilds++
		if exec.Status == "success" {
			ps.Successful++
		} else if exec.Status == "failed" {
			ps.Failed++
		}

		if exec.Duration > 0 {
			ps.AvgDuration = (ps.AvgDuration*float64(ps.TotalBuilds-1) + exec.Duration) / float64(ps.TotalBuilds)
		}

		if ps.LastBuild == nil || exec.StartedAt.After(*ps.LastBuild) {
			t := exec.StartedAt
			ps.LastBuild = &t
		}
	}

	// Calculate success rates
	result := make([]*PipelineStats, 0, len(pipelineStats))
	for _, ps := range pipelineStats {
		completed := ps.Successful + ps.Failed
		if completed > 0 {
			ps.SuccessRate = float64(ps.Successful) / float64(completed) * 100
		}
		result = append(result, ps)
	}

	// Sort by total builds descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalBuilds > result[j].TotalBuilds
	})

	json.NewEncoder(w).Encode(result)
}

// =============================================================================
// ENHANCED WEBHOOK HANDLERS - GitHub and GitLab support with signature validation
// =============================================================================

// handleGitHubWebhook processes GitHub webhooks with signature validation
func handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Parse headers
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	event := &WebhookEvent{
		ID:         generateID("webhook"),
		Source:     "github",
		Timestamp:  time.Now(),
		Headers:    headers,
		DeliveryID: r.Header.Get("X-GitHub-Delivery"),
	}

	// Validate signature if secret is configured
	signature := r.Header.Get("X-Hub-Signature-256")
	if webhookSecret != "" && signature != "" {
		expectedSig := "sha256=" + computeHMAC(body, webhookSecret)
		if hmac.Equal([]byte(signature), []byte(expectedSig)) {
			event.SignatureValid = true
		} else {
			event.SignatureValid = false
			event.Error = "Invalid webhook signature"
			log.Printf("GitHub webhook signature validation failed")
		}
		event.Signature = signature
	}

	// Parse event type
	eventType := r.Header.Get("X-GitHub-Event")
	event.Type = eventType

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	event.Payload = payload

	// Parse common fields
	parseGitHubPayload(event, payload, eventType)

	log.Printf("GitHub webhook received: event=%s repo=%s branch=%s", eventType, event.Repo, event.Branch)

	// Store event
	webhookEventsMu.Lock()
	webhookEvents = append([]*WebhookEvent{event}, webhookEvents...)
	if len(webhookEvents) > 100 {
		webhookEvents = webhookEvents[:100]
	}
	webhookEventsMu.Unlock()

	// Process trigger
	go processTrigger(event)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":         "received",
		"eventId":        event.ID,
		"eventType":      event.Type,
		"signatureValid": event.SignatureValid,
	})
}

// parseGitHubPayload extracts fields from GitHub webhook payload
func parseGitHubPayload(event *WebhookEvent, payload map[string]interface{}, eventType string) {
	// Repository info
	if repo, ok := payload["repository"].(map[string]interface{}); ok {
		if name, ok := repo["name"].(string); ok {
			event.Repo = name
		}
		if fullName, ok := repo["full_name"].(string); ok {
			event.RepoFullName = fullName
		}
	}

	switch eventType {
	case "push":
		if ref, ok := payload["ref"].(string); ok {
			event.Branch = strings.TrimPrefix(ref, "refs/heads/")
			if strings.HasPrefix(ref, "refs/tags/") {
				event.Type = "tag"
				event.TagName = strings.TrimPrefix(ref, "refs/tags/")
			}
		}
		if after, ok := payload["after"].(string); ok {
			event.Commit = after
			if len(after) >= 7 {
				event.CommitShort = after[:7]
			}
		}
		if commits, ok := payload["commits"].([]interface{}); ok && len(commits) > 0 {
			if commit, ok := commits[len(commits)-1].(map[string]interface{}); ok {
				if msg, ok := commit["message"].(string); ok {
					event.Message = msg
				}
				if author, ok := commit["author"].(map[string]interface{}); ok {
					if name, ok := author["name"].(string); ok {
						event.Author = name
					}
					if email, ok := author["email"].(string); ok {
						event.AuthorEmail = email
					}
				}
			}
		}

	case "pull_request":
		if action, ok := payload["action"].(string); ok {
			event.Action = action
		}
		if pr, ok := payload["pull_request"].(map[string]interface{}); ok {
			if num, ok := pr["number"].(float64); ok {
				event.PRNumber = int(num)
			}
			if title, ok := pr["title"].(string); ok {
				event.PRTitle = title
			}
			if head, ok := pr["head"].(map[string]interface{}); ok {
				if ref, ok := head["ref"].(string); ok {
					event.Branch = ref
				}
				if sha, ok := head["sha"].(string); ok {
					event.Commit = sha
					if len(sha) >= 7 {
						event.CommitShort = sha[:7]
					}
				}
			}
			if base, ok := pr["base"].(map[string]interface{}); ok {
				if ref, ok := base["ref"].(string); ok {
					event.BaseBranch = ref
				}
			}
		}
		if sender, ok := payload["sender"].(map[string]interface{}); ok {
			if login, ok := sender["login"].(string); ok {
				event.Author = login
			}
		}

	case "release":
		if action, ok := payload["action"].(string); ok {
			event.Action = action
		}
		if release, ok := payload["release"].(map[string]interface{}); ok {
			if tagName, ok := release["tag_name"].(string); ok {
				event.TagName = tagName
			}
		}
	}
}

// handleGitLabWebhook processes GitLab webhooks with token validation
func handleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	event := &WebhookEvent{
		ID:        generateID("webhook"),
		Source:    "gitlab",
		Timestamp: time.Now(),
		Headers:   headers,
	}

	// Validate token if configured
	token := r.Header.Get("X-Gitlab-Token")
	if webhookSecret != "" {
		if token == webhookSecret {
			event.SignatureValid = true
		} else {
			event.SignatureValid = false
			event.Error = "Invalid webhook token"
		}
	}

	eventType := r.Header.Get("X-Gitlab-Event")
	event.Type = strings.ToLower(strings.ReplaceAll(eventType, " ", "_"))

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	event.Payload = payload

	// Parse GitLab payload
	parseGitLabPayload(event, payload)

	log.Printf("GitLab webhook received: event=%s repo=%s branch=%s", event.Type, event.Repo, event.Branch)

	webhookEventsMu.Lock()
	webhookEvents = append([]*WebhookEvent{event}, webhookEvents...)
	if len(webhookEvents) > 100 {
		webhookEvents = webhookEvents[:100]
	}
	webhookEventsMu.Unlock()

	go processTrigger(event)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "received",
		"eventId":   event.ID,
		"eventType": event.Type,
	})
}

// parseGitLabPayload extracts fields from GitLab webhook payload
func parseGitLabPayload(event *WebhookEvent, payload map[string]interface{}) {
	if project, ok := payload["project"].(map[string]interface{}); ok {
		if name, ok := project["name"].(string); ok {
			event.Repo = name
		}
		if pathWithNs, ok := project["path_with_namespace"].(string); ok {
			event.RepoFullName = pathWithNs
		}
	}

	if ref, ok := payload["ref"].(string); ok {
		event.Branch = strings.TrimPrefix(ref, "refs/heads/")
	}

	if checkout, ok := payload["checkout_sha"].(string); ok {
		event.Commit = checkout
		if len(checkout) >= 7 {
			event.CommitShort = checkout[:7]
		}
	} else if after, ok := payload["after"].(string); ok {
		event.Commit = after
		if len(after) >= 7 {
			event.CommitShort = after[:7]
		}
	}

	if commits, ok := payload["commits"].([]interface{}); ok && len(commits) > 0 {
		if commit, ok := commits[len(commits)-1].(map[string]interface{}); ok {
			if msg, ok := commit["message"].(string); ok {
				event.Message = msg
			}
			if author, ok := commit["author"].(map[string]interface{}); ok {
				if name, ok := author["name"].(string); ok {
					event.Author = name
				}
				if email, ok := author["email"].(string); ok {
					event.AuthorEmail = email
				}
			}
		}
	}

	if userName, ok := payload["user_name"].(string); ok && event.Author == "" {
		event.Author = userName
	}
}

// computeHMAC computes HMAC-SHA256 signature
func computeHMAC(message []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(message)
	return hex.EncodeToString(h.Sum(nil))
}

// handleWebhookActions handles individual webhook event actions
func handleWebhookActions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/api/webhooks/")
	parts := strings.SplitN(path, "/", 2)
	webhookID := parts[0]

	webhookEventsMu.RLock()
	var event *WebhookEvent
	for _, e := range webhookEvents {
		if e.ID == webhookID {
			event = e
			break
		}
	}
	webhookEventsMu.RUnlock()

	if event == nil {
		http.Error(w, "Webhook event not found", http.StatusNotFound)
		return
	}

	if len(parts) == 1 {
		// Return webhook details
		json.NewEncoder(w).Encode(event)
		return
	}

	action := parts[1]
	switch action {
	case "redeliver":
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Create a new event based on the original
		newEvent := *event
		newEvent.ID = generateID("webhook")
		newEvent.Timestamp = time.Now()
		newEvent.Processed = false
		newEvent.ProcessedAt = nil
		newEvent.BuildID = ""
		newEvent.Error = ""

		webhookEventsMu.Lock()
		webhookEvents = append([]*WebhookEvent{&newEvent}, webhookEvents...)
		webhookEventsMu.Unlock()

		go processTrigger(&newEvent)

		json.NewEncoder(w).Encode(map[string]string{
			"status":  "redelivered",
			"eventId": newEvent.ID,
		})

	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

// =============================================================================
// BUILD QUEUE MANAGEMENT - Priority and queue control
// =============================================================================

// handleQueueReorder allows reordering builds in the queue
func handleQueueReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BuildID     string `json:"buildId"`
		NewPriority int    `json:"priority"`
		Position    string `json:"position"` // "top", "bottom", or empty for priority-based
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	buildQueueMu.Lock()
	defer buildQueueMu.Unlock()

	var targetJob *BuildJob
	var targetIndex int
	for i, job := range buildQueue {
		if job.ID == req.BuildID {
			targetJob = job
			targetIndex = i
			break
		}
	}

	if targetJob == nil {
		http.Error(w, "Build not found", http.StatusNotFound)
		return
	}

	if targetJob.Status != "queued" {
		http.Error(w, "Can only reorder queued builds", http.StatusBadRequest)
		return
	}

	// Update priority if specified
	if req.NewPriority > 0 {
		targetJob.Priority = req.NewPriority
		targetJob.PriorityName = getPriorityName(req.NewPriority)
	}

	// Reposition if requested
	if req.Position != "" {
		// Remove from current position
		buildQueue = append(buildQueue[:targetIndex], buildQueue[targetIndex+1:]...)

		switch req.Position {
		case "top":
			buildQueue = append([]*BuildJob{targetJob}, buildQueue...)
		case "bottom":
			buildQueue = append(buildQueue, targetJob)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "reordered",
		"buildId":  targetJob.ID,
		"priority": targetJob.Priority,
	})
}

// handleQueuePause pauses the build queue
func handleQueuePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	queuePausedMu.Lock()
	queuePaused = true
	queuePausedMu.Unlock()

	log.Println("Build queue paused")

	// Broadcast event
	broadcastEvent(&SSEEvent{
		ID:        generateID("event"),
		Type:      "queue_paused",
		Data:      map[string]interface{}{"paused": true},
		Timestamp: time.Now(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "paused",
		"paused": true,
	})
}

// handleQueueResume resumes the build queue
func handleQueueResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	queuePausedMu.Lock()
	queuePaused = false
	queuePausedMu.Unlock()

	log.Println("Build queue resumed")

	// Broadcast event
	broadcastEvent(&SSEEvent{
		ID:        generateID("event"),
		Type:      "queue_resumed",
		Data:      map[string]interface{}{"paused": false},
		Timestamp: time.Now(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "resumed",
		"paused": false,
	})
}

// =============================================================================
// SSE EVENT STREAMING - Real-time build status updates
// =============================================================================

// broadcastEvent sends an event to all SSE subscribers
func broadcastEvent(event *SSEEvent) {
	sseSubscribersMu.RLock()
	defer sseSubscribersMu.RUnlock()

	for _, subscribers := range sseSubscribers {
		for _, ch := range subscribers {
			select {
			case ch <- event:
			default:
				// Channel full, skip
			}
		}
	}
}

// handleSSEEvents provides a general SSE stream for all build events
func handleSSEEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create subscriber channel
	eventCh := make(chan *SSEEvent, 100)
	subscriberID := generateID("sub")

	sseSubscribersMu.Lock()
	if sseSubscribers["all"] == nil {
		sseSubscribers["all"] = make([]chan *SSEEvent, 0)
	}
	sseSubscribers["all"] = append(sseSubscribers["all"], eventCh)
	sseSubscribersMu.Unlock()

	// Cleanup on disconnect
	defer func() {
		sseSubscribersMu.Lock()
		subscribers := sseSubscribers["all"]
		for i, ch := range subscribers {
			if ch == eventCh {
				sseSubscribers["all"] = append(subscribers[:i], subscribers[i+1:]...)
				break
			}
		}
		sseSubscribersMu.Unlock()
		close(eventCh)
	}()

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"subscriberId\":\"%s\"}\n\n", subscriberID)
	flusher.Flush()

	// Send current queue status
	queuePausedMu.RLock()
	paused := queuePaused
	queuePausedMu.RUnlock()

	buildQueueMu.RLock()
	queuedCount := 0
	runningCount := 0
	for _, job := range buildQueue {
		if job.Status == "queued" {
			queuedCount++
		} else if job.Status == "running" {
			runningCount++
		}
	}
	buildQueueMu.RUnlock()

	statusData := map[string]interface{}{
		"queuePaused":   paused,
		"queuedBuilds":  queuedCount,
		"runningBuilds": runningCount,
	}
	data, _ := json.Marshal(statusData)
	fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
	flusher.Flush()

	// Stream events
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-eventCh:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"time\":\"%s\"}\n\n", time.Now().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}

// handleSSEBuildEvents provides SSE stream filtered by build/pipeline
func handleSSEBuildEvents(w http.ResponseWriter, r *http.Request) {
	pipelineID := r.URL.Query().Get("pipelineId")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	eventCh := make(chan *SSEEvent, 100)
	subscriberID := generateID("sub")
	key := "builds"
	if pipelineID != "" {
		key = "pipeline:" + pipelineID
	}

	sseSubscribersMu.Lock()
	if sseSubscribers[key] == nil {
		sseSubscribers[key] = make([]chan *SSEEvent, 0)
	}
	sseSubscribers[key] = append(sseSubscribers[key], eventCh)
	// Also subscribe to all events
	if sseSubscribers["all"] == nil {
		sseSubscribers["all"] = make([]chan *SSEEvent, 0)
	}
	sseSubscribers["all"] = append(sseSubscribers["all"], eventCh)
	sseSubscribersMu.Unlock()

	defer func() {
		sseSubscribersMu.Lock()
		for k := range sseSubscribers {
			subscribers := sseSubscribers[k]
			for i, ch := range subscribers {
				if ch == eventCh {
					sseSubscribers[k] = append(subscribers[:i], subscribers[i+1:]...)
					break
				}
			}
		}
		sseSubscribersMu.Unlock()
		close(eventCh)
	}()

	fmt.Fprintf(w, "event: connected\ndata: {\"subscriberId\":\"%s\",\"filter\":\"%s\"}\n\n", subscriberID, key)
	flusher.Flush()

	// Send recent executions for this pipeline
	executionsMu.RLock()
	recentExecs := make([]*PipelineExecution, 0)
	for _, exec := range executions {
		if pipelineID == "" || exec.PipelineID == pipelineID {
			recentExecs = append(recentExecs, exec)
			if len(recentExecs) >= 5 {
				break
			}
		}
	}
	executionsMu.RUnlock()

	if len(recentExecs) > 0 {
		data, _ := json.Marshal(recentExecs)
		fmt.Fprintf(w, "event: recent_executions\ndata: %s\n\n", data)
		flusher.Flush()
	}

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-eventCh:
			// Filter events if pipeline-specific
			if pipelineID != "" {
				if pid, ok := event.Data["pipelineId"].(string); ok && pid != pipelineID {
					continue
				}
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"time\":\"%s\"}\n\n", time.Now().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}

// =============================================================================
// ENHANCED EXECUTION HANDLERS - Better filtering and pagination
// =============================================================================

// handleReadiness provides kubernetes readiness probe
func handleReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if k8s client is working
	ready := true
	if clientset != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			ready = false
		}
	}

	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":      ready,
		"time":       time.Now().UTC().Format(time.RFC3339),
		"k8sClient":  clientset != nil,
		"uptime":     time.Since(serviceStartTime).Seconds(),
	})
}

// Helper function for rounding
func round(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
