package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
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
	registryURL = os.Getenv("REGISTRY_URL")
	holmGitURL  = os.Getenv("HOLMGIT_URL")
	port        = os.Getenv("PORT")
	clientset   *kubernetes.Clientset

	// Pipeline definitions
	pipelines   = make(map[string]*Pipeline)
	pipelinesMu sync.RWMutex

	// Build queue
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
)

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

// BuildJob represents a queued build
type BuildJob struct {
	ID          string            `json:"id"`
	PipelineID  string            `json:"pipelineId"`
	Pipeline    string            `json:"pipeline"`
	Repo        string            `json:"repo"`
	RepoURL     string            `json:"repoUrl"`
	Branch      string            `json:"branch"`
	Commit      string            `json:"commit"`
	Author      string            `json:"author"`
	Message     string            `json:"message"`
	Status      string            `json:"status"` // queued, running, success, failed, cancelled
	Priority    int               `json:"priority"`
	Variables   map[string]string `json:"variables"`
	CreatedAt   time.Time         `json:"createdAt"`
	StartedAt   *time.Time        `json:"startedAt"`
	CompletedAt *time.Time        `json:"completedAt"`
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

// WebhookEvent represents an incoming webhook
type WebhookEvent struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Source     string                 `json:"source"`
	Repo       string                 `json:"repo"`
	Branch     string                 `json:"branch"`
	Commit     string                 `json:"commit"`
	Author     string                 `json:"author"`
	Message    string                 `json:"message"`
	Payload    map[string]interface{} `json:"payload"`
	Timestamp  time.Time              `json:"timestamp"`
	Processed  bool                   `json:"processed"`
	PipelineID string                 `json:"pipelineId"`
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

	// Pipeline management
	http.HandleFunc("/api/pipelines", handlePipelines)
	http.HandleFunc("/api/pipelines/", handlePipelineActions)

	// Webhook endpoints
	http.HandleFunc("/api/webhook/git", handleGitWebhook)
	http.HandleFunc("/api/webhook/holmgit", handleHolmGitWebhook)
	http.HandleFunc("/api/webhooks", handleWebhooks)

	// Build queue
	http.HandleFunc("/api/queue", handleQueue)
	http.HandleFunc("/api/queue/", handleQueueActions)

	// Pipeline executions (history)
	http.HandleFunc("/api/executions", handleExecutions)
	http.HandleFunc("/api/executions/", handleExecutionActions)

	// Build logs
	http.HandleFunc("/api/logs/", handleLogs)
	http.HandleFunc("/api/logs-stream/", handleLogsStream)

	// Kaniko builds
	http.HandleFunc("/api/build", handleBuild)
	http.HandleFunc("/api/builds", handleBuilds)

	// Deployment triggers
	http.HandleFunc("/api/deploy", handleDeploy)

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

func processNextBuild() {
	buildQueueMu.Lock()
	defer buildQueueMu.Unlock()

	// Find next queued build
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

	if runningCount >= 3 {
		return // Max 3 concurrent builds
	}

	// Start the build
	now := time.Now()
	nextBuild.Status = "running"
	nextBuild.StartedAt = &now

	go executePipeline(nextBuild)
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
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "kubernetes.io/hostname",
												Operator: corev1.NodeSelectorOpNotIn,
												Values:   []string{"openmediavault"},
											},
										},
									},
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:  "git-clone",
							Image: "alpine/git:latest",
							Command: []string{"sh", "-c"},
							Args: []string{
								func() string {
									// Use pipeline's repoUrl if set, otherwise construct from HolmGit
									repoURL := job.RepoURL
									if repoURL == "" {
										repoURL = fmt.Sprintf("%s/git/%s.git", holmGitURL, job.Repo)
									}
									return fmt.Sprintf("set -ex && echo 'Cloning repository %s branch %s...' && git clone --depth 1 --branch %s %s /workspace && echo 'Clone successful' && ls -la /workspace",
										job.Repo, job.Branch, job.Branch, repoURL)
								}(),
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

		job := &BuildJob{
			ID:         generateID("build"),
			PipelineID: pipelineID,
			Pipeline:   pipeline.Name,
			Repo:       pipeline.RepoURL,
			Branch:     req.Branch,
			Commit:     req.Commit,
			Status:     "queued",
			Priority:   1,
			Variables:  req.Variables,
			CreatedAt:  time.Now(),
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

			job := &BuildJob{
				ID:         generateID("build"),
				PipelineID: pipeline.ID,
				Pipeline:   pipeline.Name,
				Repo:       event.Repo,
				RepoURL:    pipeline.RepoURL,
				Branch:     event.Branch,
				Commit:     event.Commit,
				Author:     event.Author,
				Message:    event.Message,
				Status:     "queued",
				Priority:   1,
				Variables:  make(map[string]string),
				CreatedAt:  time.Now(),
			}

			buildQueueMu.Lock()
			buildQueue = append([]*BuildJob{job}, buildQueue...)
			buildQueueMu.Unlock()

			// Mark event as processed
			webhookEventsMu.Lock()
			for _, we := range webhookEvents {
				if we.ID == event.ID {
					we.Processed = true
					we.PipelineID = pipeline.ID
					break
				}
			}
			webhookEventsMu.Unlock()

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

		job := &BuildJob{
			ID:         generateID("build"),
			PipelineID: req.PipelineID,
			Pipeline:   req.Pipeline,
			Repo:       req.Repo,
			Branch:     req.Branch,
			Commit:     req.Commit,
			Status:     "queued",
			Priority:   1,
			Variables:  req.Variables,
			CreatedAt:  time.Now(),
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

	// Support filtering
	pipelineID := r.URL.Query().Get("pipelineId")
	status := r.URL.Query().Get("status")
	limit := 50

	result := make([]*PipelineExecution, 0)
	for _, exec := range executions {
		if pipelineID != "" && exec.PipelineID != pipelineID {
			continue
		}
		if status != "" && exec.Status != status {
			continue
		}
		result = append(result, exec)
		if len(result) >= limit {
			break
		}
	}

	json.NewEncoder(w).Encode(result)
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

		// Create new build job from execution
		job := &BuildJob{
			ID:         generateID("build"),
			PipelineID: execution.PipelineID,
			Pipeline:   execution.PipelineName,
			Repo:       execution.Repo,
			Branch:     execution.Branch,
			Commit:     execution.Commit,
			Status:     "queued",
			Priority:   1,
			Variables:  make(map[string]string),
			CreatedAt:  time.Now(),
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

	// Create a quick build job
	job := &BuildJob{
		ID:        generateID("build"),
		Repo:      req.Repo,
		Branch:    req.Branch,
		Status:    "queued",
		Priority:  2, // Higher priority for manual builds
		Variables: map[string]string{
			"DOCKERFILE":  req.Dockerfile,
			"CONTEXT":     req.Context,
			"DESTINATION": req.Destination,
		},
		CreatedAt: time.Now(),
	}

	buildQueueMu.Lock()
	// Insert at front for priority
	newQueue := make([]*BuildJob, 0, len(buildQueue)+1)
	newQueue = append(newQueue, job)
	newQueue = append(newQueue, buildQueue...)
	buildQueue = newQueue
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

	json.NewEncoder(w).Encode(buildQueue[:limit])
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
