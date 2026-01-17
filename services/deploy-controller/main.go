package main

import (
	"bufio"
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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	registryURL    = os.Getenv("REGISTRY_URL")
	forgeURL       = os.Getenv("FORGE_URL")
	holmGitURL     = os.Getenv("HOLMGIT_URL")
	port           = os.Getenv("PORT")
	rulesConfigMap = os.Getenv("RULES_CONFIGMAP")
	clientset      *kubernetes.Clientset

	deployments         = make(map[string]*DeploymentInfo)
	deploymentsMu       sync.RWMutex
	imageDigests        = make(map[string]string)
	imageDigestsMu      sync.RWMutex
	recentDeploys       []DeployEvent
	recentDeploysMu     sync.RWMutex
	webhookEvents       []WebhookEvent
	webhookEventsMu     sync.RWMutex
	autoDeployRules     = make(map[string]AutoDeployRule)
	autoDeployMu        sync.RWMutex
	deploymentHistory   = make(map[string][]DeploymentVersion)
	historyMu           sync.RWMutex
	registryEvents      []RegistryEvent
	registryEventsMu    sync.RWMutex
	pendingHealthChecks = make(map[string]*HealthCheckStatus)
	healthCheckMu       sync.RWMutex

	// Deployment metrics tracking
	deployMetrics   = &DeploymentMetrics{}
	deployMetricsMu sync.RWMutex
)

type DeploymentInfo struct {
	Name       string    `json:"name"`
	Namespace  string    `json:"namespace"`
	Image      string    `json:"image"`
	Status     string    `json:"status"`
	LastDeploy time.Time `json:"lastDeploy"`
	AutoDeploy bool      `json:"autoDeploy"`
	Replicas   int32     `json:"replicas"`
	Ready      int32     `json:"ready"`
}

type DeploymentVersion struct {
	Version   int       `json:"version"`
	Image     string    `json:"image"`
	Trigger   string    `json:"trigger"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Duration  float64   `json:"duration"`
	Message   string    `json:"message"`
	Digest    string    `json:"digest"`
}

type DeployEvent struct {
	ID         string    `json:"id"`
	Deployment string    `json:"deployment"`
	Namespace  string    `json:"namespace"`
	Image      string    `json:"image"`
	OldImage   string    `json:"oldImage"`
	Trigger    string    `json:"trigger"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	Timestamp  time.Time `json:"timestamp"`
	Duration   float64   `json:"duration"`
}

type WebhookEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Repo      string                 `json:"repo"`
	Branch    string                 `json:"branch"`
	Commit    string                 `json:"commit"`
	Payload   map[string]interface{} `json:"payload"`
	Timestamp time.Time              `json:"timestamp"`
	Processed bool                   `json:"processed"`
}

type RegistryEvent struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`
	Repository string    `json:"repository"`
	Tag        string    `json:"tag"`
	Digest     string    `json:"digest"`
	Timestamp  time.Time `json:"timestamp"`
	Processed  bool      `json:"processed"`
	AutoDeploy bool      `json:"autoDeploy"`
}

type AutoDeployRule struct {
	ImagePattern       string `json:"imagePattern"`
	Deployment         string `json:"deployment"`
	Namespace          string `json:"namespace"`
	Enabled            bool   `json:"enabled"`
	AutoCreate         bool   `json:"autoCreate"`
	TagPattern         string `json:"tagPattern"`
	LastTriggered      string `json:"lastTriggered"`
	HealthCheckPath    string `json:"healthCheckPath"`
	HealthCheckPort    int    `json:"healthCheckPort"`
	ServicePort        int    `json:"servicePort"`
	CreateService      bool   `json:"createService"`
	AutoRollback       bool   `json:"autoRollback"`       // Enable auto rollback on failure
	RollbackTimeout    int    `json:"rollbackTimeout"`    // Timeout in seconds before auto rollback
	MaxRollbackRetries int    `json:"maxRollbackRetries"` // Max consecutive rollbacks before stopping
}

type HealthCheckStatus struct {
	Deployment        string         `json:"deployment"`
	Namespace         string         `json:"namespace"`
	Image             string         `json:"image"`
	PreviousImage     string         `json:"previousImage,omitempty"`
	Status            string         `json:"status"`
	StartTime         time.Time      `json:"startTime"`
	LastCheck         time.Time      `json:"lastCheck"`
	Attempts          int            `json:"attempts"`
	MaxAttempts       int            `json:"maxAttempts"`
	Message           string         `json:"message"`
	PodStatuses       []PodStatus    `json:"podStatuses"`
	RolloutStatus     *RolloutStatus `json:"rolloutStatus,omitempty"`
	AutoRollback      bool           `json:"autoRollback"`
	RollbackTriggered bool           `json:"rollbackTriggered"`
	RollbackReason    string         `json:"rollbackReason,omitempty"`
}

type RolloutStatus struct {
	Replicas            int32              `json:"replicas"`
	UpdatedReplicas     int32              `json:"updatedReplicas"`
	ReadyReplicas       int32              `json:"readyReplicas"`
	AvailableReplicas   int32              `json:"availableReplicas"`
	UnavailableReplicas int32              `json:"unavailableReplicas"`
	ProgressDeadline    bool               `json:"progressDeadline"`
	Stalled             bool               `json:"stalled"`
	StalledReason       string             `json:"stalledReason,omitempty"`
	Conditions          []RolloutCondition `json:"conditions,omitempty"`
}

type RolloutCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type PodStatus struct {
	Name    string `json:"name"`
	Ready   bool   `json:"ready"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

type RegistryImage struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// DeploymentMetrics tracks deployment statistics
type DeploymentMetrics struct {
	TotalDeploys      int64                      `json:"totalDeploys"`
	SuccessfulDeploys int64                      `json:"successfulDeploys"`
	FailedDeploys     int64                      `json:"failedDeploys"`
	RollbackCount     int64                      `json:"rollbackCount"`
	AutoRollbackCount int64                      `json:"autoRollbackCount"`
	LastDeployTime    time.Time                  `json:"lastDeployTime"`
	DeploysByHour     map[string]int             `json:"deploysByHour"`
	DeploysByDay      map[string]int             `json:"deploysByDay"`
	PerDeployment     map[string]*DeploymentStat `json:"perDeployment"`
	AverageDeployTime float64                    `json:"averageDeployTime"`
	TotalDeployTime   float64                    `json:"totalDeployTime"`
}

// DeploymentStat tracks per-deployment statistics
type DeploymentStat struct {
	Name              string    `json:"name"`
	Namespace         string    `json:"namespace"`
	TotalDeploys      int64     `json:"totalDeploys"`
	SuccessfulDeploys int64     `json:"successfulDeploys"`
	FailedDeploys     int64     `json:"failedDeploys"`
	RollbackCount     int64     `json:"rollbackCount"`
	AutoRollbackCount int64     `json:"autoRollbackCount"`
	LastDeploy        time.Time `json:"lastDeploy"`
	LastSuccess       time.Time `json:"lastSuccess"`
	LastFailure       time.Time `json:"lastFailure"`
	AverageDeployTime float64   `json:"averageDeployTime"`
	TotalDeployTime   float64   `json:"totalDeployTime"`
	CurrentImage      string    `json:"currentImage"`
	PreviousImage     string    `json:"previousImage"`
	SuccessRate       float64   `json:"successRate"`
	MTTR              float64   `json:"mttr"` // Mean Time To Recovery
}

// Docker Registry notification event structure
type RegistryNotification struct {
	Events []struct {
		ID        string `json:"id"`
		Timestamp string `json:"timestamp"`
		Action    string `json:"action"`
		Target    struct {
			MediaType  string `json:"mediaType"`
			Digest     string `json:"digest"`
			Repository string `json:"repository"`
			URL        string `json:"url"`
			Tag        string `json:"tag"`
		} `json:"target"`
		Request struct {
			ID        string `json:"id"`
			Addr      string `json:"addr"`
			Host      string `json:"host"`
			Method    string `json:"method"`
			UserAgent string `json:"useragent"`
		} `json:"request"`
	} `json:"events"`
}

func main() {
	if registryURL == "" {
		registryURL = "192.168.8.197:30500"
	}
	if forgeURL == "" {
		forgeURL = "http://forge.holm.svc.cluster.local"
	}
	if holmGitURL == "" {
		holmGitURL = "http://holm-git.holm.svc.cluster.local"
	}
	if port == "" {
		port = "8080"
	}
	if rulesConfigMap == "" {
		rulesConfigMap = "deploy-controller-rules"
	}

	log.Printf("Deploy Controller v4 starting on port %s", port)
	log.Printf("Registry URL: %s", registryURL)
	log.Printf("Forge URL: %s", forgeURL)
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

	// Load existing deployment history and rules
	loadDeploymentHistory()
	loadAutoDeployRules()
	initializeMetrics()

	go watchRegistry()
	go syncDeployments()
	go healthCheckWorker()

	http.HandleFunc("/", handleUI)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/api/deployments", handleDeployments)
	http.HandleFunc("/api/deploy", handleDeploy)
	http.HandleFunc("/api/rollback", handleRollback)
	http.HandleFunc("/api/events", handleEvents)
	http.HandleFunc("/api/webhook", handleWebhook)
	http.HandleFunc("/api/webhook/git", handleGitWebhook)
	http.HandleFunc("/api/webhook/build", handleBuildWebhook)
	http.HandleFunc("/api/webhook/registry", handleRegistryWebhook)
	http.HandleFunc("/api/autodeploy", handleAutoDeploy)
	http.HandleFunc("/api/images", handleImages)
	http.HandleFunc("/api/history", handleHistory)
	http.HandleFunc("/api/registry-events", handleRegistryEvents)
	http.HandleFunc("/api/forge/builds", handleForgeBuilds)
	http.HandleFunc("/api/trigger-build", handleTriggerBuild)
	http.HandleFunc("/api/health-checks", handleHealthChecks)
	http.HandleFunc("/api/scale", handleScale)
	http.HandleFunc("/api/logs", handlePodLogs)
	http.HandleFunc("/api/events/stream", handleEventStream)
	http.HandleFunc("/api/apply", handleApplyManifest)
	http.HandleFunc("/api/restart", handleRestart)
	http.HandleFunc("/api/k8s-events", handleK8sEvents)
	http.HandleFunc("/api/metrics", handleMetrics)
	http.HandleFunc("/api/history/", handleHistoryByDeployment)

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func loadDeploymentHistory() {
	// Initialize from recent deploys if available
	recentDeploysMu.RLock()
	for _, event := range recentDeploys {
		historyMu.Lock()
		history := deploymentHistory[event.Deployment]
		version := DeploymentVersion{
			Version:   len(history) + 1,
			Image:     event.Image,
			Trigger:   event.Trigger,
			Status:    event.Status,
			Timestamp: event.Timestamp,
			Duration:  event.Duration,
			Message:   event.Message,
		}
		deploymentHistory[event.Deployment] = append([]DeploymentVersion{version}, history...)
		historyMu.Unlock()
	}
	recentDeploysMu.RUnlock()
}

// Registry webhook handler - receives notifications from Docker Registry
func handleRegistryWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Registry webhook: failed to read body: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Registry webhook received: %s", string(body))

	var notification RegistryNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		log.Printf("Registry webhook: failed to parse: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, evt := range notification.Events {
		if evt.Action != "push" {
			continue
		}

		// Skip if no tag (manifest list push)
		if evt.Target.Tag == "" {
			continue
		}

		regEvent := RegistryEvent{
			ID:         fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s%s%d", evt.Target.Repository, evt.Target.Tag, time.Now().UnixNano()))))[:12],
			Action:     evt.Action,
			Repository: evt.Target.Repository,
			Tag:        evt.Target.Tag,
			Digest:     evt.Target.Digest,
			Timestamp:  time.Now(),
			Processed:  false,
		}

		registryEventsMu.Lock()
		registryEvents = append([]RegistryEvent{regEvent}, registryEvents...)
		if len(registryEvents) > 100 {
			registryEvents = registryEvents[:100]
		}
		registryEventsMu.Unlock()

		log.Printf("Registry push: %s:%s (digest: %s)", evt.Target.Repository, evt.Target.Tag, evt.Target.Digest[:12])

		// Update image digest cache
		imageRef := fmt.Sprintf("%s/%s:%s", registryURL, evt.Target.Repository, evt.Target.Tag)
		imageDigestsMu.Lock()
		imageDigests[imageRef] = evt.Target.Digest
		imageDigestsMu.Unlock()

		// Trigger auto-deploy
		go processRegistryPush(evt.Target.Repository, evt.Target.Tag, evt.Target.Digest, regEvent.ID)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "received",
		"events": len(notification.Events),
	})
}

func processRegistryPush(repo, tag, digest, eventID string) {
	imageRef := fmt.Sprintf("%s/%s:%s", registryURL, repo, tag)
	autoDeployed := false

	autoDeployMu.Lock()
	for name, rule := range autoDeployRules {
		if !rule.Enabled {
			continue
		}

		// Check if image matches pattern
		if !strings.Contains(repo, rule.ImagePattern) && !strings.Contains(imageRef, rule.ImagePattern) {
			continue
		}

		// Check tag pattern if specified
		if rule.TagPattern != "" && !matchTagPattern(tag, rule.TagPattern) {
			continue
		}

		log.Printf("Auto-deploy triggered: %s -> %s/%s", imageRef, rule.Namespace, rule.Deployment)
		rule.LastTriggered = time.Now().Format(time.RFC3339)
		autoDeployRules[name] = rule

		// Check if deployment exists, create if autoCreate is enabled
		if rule.AutoCreate {
			go ensureDeploymentExists(rule.Namespace, rule.Deployment, imageRef)
		}

		go deployImage(rule.Namespace, rule.Deployment, imageRef, "registry-webhook")
		autoDeployed = true
	}
	autoDeployMu.Unlock()

	// Mark event as processed
	registryEventsMu.Lock()
	for i := range registryEvents {
		if registryEvents[i].ID == eventID {
			registryEvents[i].Processed = true
			registryEvents[i].AutoDeploy = autoDeployed
			break
		}
	}
	registryEventsMu.Unlock()
}

func matchTagPattern(tag, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	if pattern == "latest" && tag == "latest" {
		return true
	}
	// Simple wildcard matching
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(tag, prefix)
	}
	return tag == pattern
}

func ensureDeploymentExists(namespace, name, image string) {
	if clientset == nil {
		return
	}

	ctx := context.Background()
	_, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return // Deployment already exists
	}

	if !errors.IsNotFound(err) {
		log.Printf("Error checking deployment %s: %v", name, err)
		return
	}

	// Get rule configuration for this deployment
	autoDeployMu.RLock()
	rule, hasRule := autoDeployRules[name]
	autoDeployMu.RUnlock()

	port := int32(8080)
	healthPath := "/health"
	if hasRule {
		if rule.HealthCheckPort > 0 {
			port = int32(rule.HealthCheckPort)
		}
		if rule.HealthCheckPath != "" {
			healthPath = rule.HealthCheckPath
		}
	}

	// Create new deployment
	log.Printf("Auto-creating deployment %s/%s with image %s", namespace, name, image)
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                            name,
				"deploy-controller/auto-created": "true",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
					Annotations: map[string]string{
						"deploy-controller/created-at": time.Now().Format(time.RFC3339),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: image,
							Ports: []corev1.ContainerPort{
								{ContainerPort: port},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: healthPath,
										Port: intstr.FromInt(int(port)),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: healthPath,
										Port: intstr.FromInt(int(port)),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil {
		log.Printf("Failed to create deployment %s: %v", name, err)
		return
	}

	addDeployEvent(name, namespace, image, "", "auto-create", "success", "Deployment auto-created", 0)
	log.Printf("Created deployment %s/%s", namespace, name)

	// Create service if configured
	if hasRule && rule.CreateService && rule.ServicePort > 0 {
		if err := ensureServiceExists(namespace, name, rule.ServicePort); err != nil {
			log.Printf("Failed to create service for %s: %v", name, err)
		}
	}

	// Start health check
	startHealthCheck(name, namespace, image)
}

func watchRegistry() {
	log.Println("Starting registry watcher (polling mode)")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	scanRegistry()
	for range ticker.C {
		scanRegistry()
	}
}

func scanRegistry() {
	repos, err := getRegistryRepos()
	if err != nil {
		log.Printf("Registry scan error: %v", err)
		return
	}
	for _, repo := range repos {
		tags, err := getRepoTags(repo)
		if err != nil {
			continue
		}
		for _, tag := range tags {
			imageRef := fmt.Sprintf("%s/%s:%s", registryURL, repo, tag)
			newDigest, err := getImageDigest(repo, tag)
			if err != nil {
				continue
			}
			imageDigestsMu.Lock()
			oldDigest, exists := imageDigests[imageRef]
			if !exists || oldDigest != newDigest {
				imageDigests[imageRef] = newDigest
				if exists {
					log.Printf("Image updated (poll): %s", imageRef)
					go triggerAutoDeployIfEnabled(imageRef, newDigest)
				}
			}
			imageDigestsMu.Unlock()
		}
	}
}

func triggerAutoDeployIfEnabled(imageRef, digest string) {
	autoDeployMu.RLock()
	defer autoDeployMu.RUnlock()
	for _, rule := range autoDeployRules {
		if !rule.Enabled {
			continue
		}
		if strings.Contains(imageRef, rule.ImagePattern) {
			log.Printf("Auto-deploying (poll) %s to %s/%s", imageRef, rule.Namespace, rule.Deployment)
			deployImage(rule.Namespace, rule.Deployment, imageRef, "registry-poll")
		}
	}
}

func getRegistryRepos() ([]string, error) {
	resp, err := http.Get(fmt.Sprintf("http://%s/v2/_catalog", registryURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Repositories []string `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Repositories, nil
}

func getRepoTags(repo string) ([]string, error) {
	resp, err := http.Get(fmt.Sprintf("http://%s/v2/%s/tags/list", registryURL, repo))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Tags, nil
}

func getImageDigest(repo, tag string) (string, error) {
	req, err := http.NewRequest("HEAD", fmt.Sprintf("http://%s/v2/%s/manifests/%s", registryURL, repo, tag), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return resp.Header.Get("Docker-Content-Digest"), nil
}

func syncDeployments() {
	log.Println("Starting deployment sync")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if clientset == nil {
			continue
		}
		ctx := context.Background()
		deps, err := clientset.AppsV1().Deployments("holm").List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("Failed to list deployments: %v", err)
			continue
		}
		deploymentsMu.Lock()
		for _, dep := range deps.Items {
			var image string
			if len(dep.Spec.Template.Spec.Containers) > 0 {
				image = dep.Spec.Template.Spec.Containers[0].Image
			}
			status := "Running"
			if dep.Status.ReadyReplicas < dep.Status.Replicas {
				status = "Updating"
			}
			if dep.Status.ReadyReplicas == 0 {
				status = "NotReady"
			}
			autoDeployMu.RLock()
			_, hasAutoDeploy := autoDeployRules[dep.Name]
			autoDeployMu.RUnlock()

			// Get last deploy time from annotations
			var lastDeploy time.Time
			if dep.Spec.Template.Annotations != nil {
				if ts, ok := dep.Spec.Template.Annotations["deploy-controller/deployed-at"]; ok {
					lastDeploy, _ = time.Parse(time.RFC3339, ts)
				}
			}

			deployments[dep.Name] = &DeploymentInfo{
				Name:       dep.Name,
				Namespace:  dep.Namespace,
				Image:      image,
				Status:     status,
				AutoDeploy: hasAutoDeploy,
				Replicas:   dep.Status.Replicas,
				Ready:      dep.Status.ReadyReplicas,
				LastDeploy: lastDeploy,
			}
		}
		deploymentsMu.Unlock()
	}
}

func deployImage(namespace, deployment, image, trigger string) error {
	if clientset == nil {
		return fmt.Errorf("kubernetes client not initialized")
	}
	start := time.Now()
	ctx := context.Background()
	dep, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		addDeployEvent(deployment, namespace, image, "", trigger, "failed", err.Error(), time.Since(start).Seconds())
		return err
	}
	oldImage := ""
	if len(dep.Spec.Template.Spec.Containers) > 0 {
		oldImage = dep.Spec.Template.Spec.Containers[0].Image
		dep.Spec.Template.Spec.Containers[0].Image = image
	}

	// Skip if image hasn't changed
	if oldImage == image {
		log.Printf("Skipping deploy - image unchanged: %s", image)
		return nil
	}

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}
	dep.Spec.Template.Annotations["deploy-controller/deployed-at"] = time.Now().Format(time.RFC3339)
	dep.Spec.Template.Annotations["deploy-controller/trigger"] = trigger
	dep.Spec.Template.Annotations["deploy-controller/previous-image"] = oldImage

	_, err = clientset.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		addDeployEvent(deployment, namespace, image, oldImage, trigger, "failed", err.Error(), time.Since(start).Seconds())
		return err
	}

	duration := time.Since(start).Seconds()
	addDeployEvent(deployment, namespace, image, oldImage, trigger, "deploying", "Deployment updated, starting health checks", duration)
	addToHistory(deployment, image, oldImage, trigger, "deploying", duration, "")

	// Start health check monitoring
	startHealthCheck(deployment, namespace, image)

	log.Printf("Deployed %s to %s/%s (trigger: %s)", image, namespace, deployment, trigger)
	return nil
}

func addDeployEvent(deployment, namespace, image, oldImage, trigger, status, message string, duration float64) {
	event := DeployEvent{
		ID:         fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s%d", deployment, time.Now().UnixNano()))))[:12],
		Deployment: deployment,
		Namespace:  namespace,
		Image:      image,
		OldImage:   oldImage,
		Trigger:    trigger,
		Status:     status,
		Message:    message,
		Timestamp:  time.Now(),
		Duration:   duration,
	}
	recentDeploysMu.Lock()
	recentDeploys = append([]DeployEvent{event}, recentDeploys...)
	if len(recentDeploys) > 100 {
		recentDeploys = recentDeploys[:100]
	}
	recentDeploysMu.Unlock()
}

func addToHistory(deployment, image, oldImage, trigger, status string, duration float64, digest string) {
	historyMu.Lock()
	defer historyMu.Unlock()

	history := deploymentHistory[deployment]
	version := len(history) + 1

	entry := DeploymentVersion{
		Version:   version,
		Image:     image,
		Trigger:   trigger,
		Status:    status,
		Timestamp: time.Now(),
		Duration:  duration,
		Digest:    digest,
	}

	deploymentHistory[deployment] = append([]DeploymentVersion{entry}, history...)
	if len(deploymentHistory[deployment]) > 50 {
		deploymentHistory[deployment] = deploymentHistory[deployment][:50]
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": "4.0.0",
	})
}

func handleDeployments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Support query parameter for specific deployment
	deploymentName := r.URL.Query().Get("name")
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "holm"
	}

	deploymentsMu.RLock()
	defer deploymentsMu.RUnlock()

	// If specific deployment requested
	if deploymentName != "" {
		if d, ok := deployments[deploymentName]; ok {
			// Enrich with history and metrics
			response := enrichDeploymentInfo(d)
			json.NewEncoder(w).Encode(response)
			return
		}
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	// Return all deployments
	deps := make([]*DeploymentInfoEnriched, 0, len(deployments))
	for _, d := range deployments {
		deps = append(deps, enrichDeploymentInfo(d))
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Name < deps[j].Name })
	json.NewEncoder(w).Encode(deps)
}

// DeploymentInfoEnriched includes additional deployment details
type DeploymentInfoEnriched struct {
	*DeploymentInfo
	History       []DeploymentVersion `json:"history,omitempty"`
	HealthCheck   *HealthCheckStatus  `json:"healthCheck,omitempty"`
	Stats         *DeploymentStat     `json:"stats,omitempty"`
	PendingImages []string            `json:"pendingImages,omitempty"`
}

func enrichDeploymentInfo(d *DeploymentInfo) *DeploymentInfoEnriched {
	enriched := &DeploymentInfoEnriched{
		DeploymentInfo: d,
	}

	// Add recent history (last 5 versions)
	historyMu.RLock()
	if hist, ok := deploymentHistory[d.Name]; ok {
		limit := 5
		if len(hist) < limit {
			limit = len(hist)
		}
		enriched.History = hist[:limit]
	}
	historyMu.RUnlock()

	// Add health check status if any
	healthCheckMu.RLock()
	key := fmt.Sprintf("%s/%s", d.Namespace, d.Name)
	if hc, ok := pendingHealthChecks[key]; ok {
		enriched.HealthCheck = hc
	}
	healthCheckMu.RUnlock()

	// Add deployment stats
	deployMetricsMu.RLock()
	if deployMetrics.PerDeployment != nil {
		if stat, ok := deployMetrics.PerDeployment[d.Name]; ok {
			enriched.Stats = stat
		}
	}
	deployMetricsMu.RUnlock()

	return enriched
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "holm"
	}
	if err := deployImage(req.Namespace, req.Deployment, req.Image, "manual"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deployed"})
}

func handleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Deployment string `json:"deployment"`
		Namespace  string `json:"namespace"`
		Version    int    `json:"version"`
		ToImage    string `json:"toImage"` // Direct image specification
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "holm"
	}

	var targetImage string
	var rollbackVersion int

	// Priority: toImage > version > previous successful
	if req.ToImage != "" {
		targetImage = req.ToImage
	} else if req.Version > 0 {
		// Rollback to specific version
		historyMu.RLock()
		history := deploymentHistory[req.Deployment]
		for _, ver := range history {
			if ver.Version == req.Version {
				targetImage = ver.Image
				rollbackVersion = ver.Version
				break
			}
		}
		historyMu.RUnlock()
	} else {
		// Find previous successful version
		historyMu.RLock()
		history := deploymentHistory[req.Deployment]
		// Skip the first entry (current), find next successful
		for i, ver := range history {
			if i > 0 && ver.Status == "success" {
				targetImage = ver.Image
				rollbackVersion = ver.Version
				break
			}
		}
		historyMu.RUnlock()

		// Fallback to recentDeploys
		if targetImage == "" {
			recentDeploysMu.RLock()
			for _, event := range recentDeploys {
				if event.Deployment == req.Deployment && event.Status == "success" && event.OldImage != "" {
					targetImage = event.OldImage
					break
				}
			}
			recentDeploysMu.RUnlock()
		}
	}

	if targetImage == "" {
		http.Error(w, "No previous image found for rollback", http.StatusNotFound)
		return
	}

	// Get current image for metrics
	var currentImage string
	deploymentsMu.RLock()
	if d, ok := deployments[req.Deployment]; ok {
		currentImage = d.Image
	}
	deploymentsMu.RUnlock()

	if err := performRollback(req.Namespace, req.Deployment, targetImage, currentImage, "manual"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update metrics
	updateRollbackMetrics(req.Deployment, req.Namespace, false)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "rolled back",
		"deployment":      req.Deployment,
		"namespace":       req.Namespace,
		"fromImage":       currentImage,
		"toImage":         targetImage,
		"rollbackVersion": rollbackVersion,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	})
}

// performRollback executes the rollback operation
func performRollback(namespace, deployment, targetImage, currentImage, trigger string) error {
	if clientset == nil {
		return fmt.Errorf("kubernetes client not initialized")
	}

	start := time.Now()
	ctx := context.Background()
	dep, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		addDeployEvent(deployment, namespace, targetImage, currentImage, trigger+"-rollback", "failed", err.Error(), time.Since(start).Seconds())
		return err
	}

	if len(dep.Spec.Template.Spec.Containers) > 0 {
		dep.Spec.Template.Spec.Containers[0].Image = targetImage
	}

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}
	dep.Spec.Template.Annotations["deploy-controller/rolled-back-at"] = time.Now().Format(time.RFC3339)
	dep.Spec.Template.Annotations["deploy-controller/trigger"] = trigger + "-rollback"
	dep.Spec.Template.Annotations["deploy-controller/rolled-back-from"] = currentImage

	_, err = clientset.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		addDeployEvent(deployment, namespace, targetImage, currentImage, trigger+"-rollback", "failed", err.Error(), time.Since(start).Seconds())
		return err
	}

	duration := time.Since(start).Seconds()
	addDeployEvent(deployment, namespace, targetImage, currentImage, trigger+"-rollback", "deploying", "Rollback initiated", duration)
	addToHistory(deployment, targetImage, currentImage, trigger+"-rollback", "deploying", duration, "")

	// Start health check for rollback
	startHealthCheckWithPrevious(deployment, namespace, targetImage, currentImage, false)

	log.Printf("Rollback initiated: %s/%s from %s to %s (trigger: %s)", namespace, deployment, currentImage, targetImage, trigger)
	return nil
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	recentDeploysMu.RLock()
	defer recentDeploysMu.RUnlock()
	json.NewEncoder(w).Encode(recentDeploys)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	deployment := r.URL.Query().Get("deployment")

	historyMu.RLock()
	defer historyMu.RUnlock()

	if deployment != "" {
		history := deploymentHistory[deployment]
		if history == nil {
			history = []DeploymentVersion{}
		}
		json.NewEncoder(w).Encode(history)
	} else {
		json.NewEncoder(w).Encode(deploymentHistory)
	}
}

func handleRegistryEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	registryEventsMu.RLock()
	defer registryEventsMu.RUnlock()
	json.NewEncoder(w).Encode(registryEvents)
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	webhookEventsMu.RLock()
	defer webhookEventsMu.RUnlock()
	json.NewEncoder(w).Encode(webhookEvents)
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
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	repo := ""
	branch := ""
	commit := ""
	if repoData, ok := payload["repository"].(map[string]interface{}); ok {
		if name, ok := repoData["name"].(string); ok {
			repo = name
		}
		if fullName, ok := repoData["full_name"].(string); ok {
			repo = fullName
		}
	}
	if ref, ok := payload["ref"].(string); ok {
		branch = strings.TrimPrefix(ref, "refs/heads/")
	}
	if after, ok := payload["after"].(string); ok && len(after) >= 12 {
		commit = after[:12]
	}
	event := WebhookEvent{
		ID:        fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s%d", repo, time.Now().UnixNano()))))[:12],
		Type:      "git-push",
		Source:    "holm-git",
		Repo:      repo,
		Branch:    branch,
		Commit:    commit,
		Payload:   payload,
		Timestamp: time.Now(),
		Processed: false,
	}
	webhookEventsMu.Lock()
	webhookEvents = append([]WebhookEvent{event}, webhookEvents...)
	if len(webhookEvents) > 50 {
		webhookEvents = webhookEvents[:50]
	}
	webhookEventsMu.Unlock()
	log.Printf("Git webhook: repo=%s branch=%s commit=%s", repo, branch, commit)
	go triggerForgeBuild(repo, branch, commit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "received", "eventId": event.ID})
}

func handleBuildWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		BuildID    string `json:"buildId"`
		Image      string `json:"image"`
		Status     string `json:"status"`
		Deployment string `json:"deployment"`
		Namespace  string `json:"namespace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("Build webhook: image=%s status=%s", payload.Image, payload.Status)
	if payload.Status == "success" && payload.Image != "" {
		namespace := payload.Namespace
		if namespace == "" {
			namespace = "holm"
		}
		deployment := payload.Deployment
		if deployment == "" {
			parts := strings.Split(payload.Image, "/")
			if len(parts) > 0 {
				deployment = strings.Split(parts[len(parts)-1], ":")[0]
			}
		}
		if deployment != "" {
			go deployImage(namespace, deployment, payload.Image, "build-webhook")
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "processed"})
}

func handleAutoDeploy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case "GET":
		autoDeployMu.RLock()
		defer autoDeployMu.RUnlock()
		json.NewEncoder(w).Encode(autoDeployRules)
	case "POST":
		var rule AutoDeployRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if rule.Namespace == "" {
			rule.Namespace = "holm"
		}
		if rule.HealthCheckPath == "" {
			rule.HealthCheckPath = "/health"
		}
		if rule.HealthCheckPort == 0 {
			rule.HealthCheckPort = 8080
		}
		autoDeployMu.Lock()
		autoDeployRules[rule.Deployment] = rule
		autoDeployMu.Unlock()
		go saveAutoDeployRules()
		log.Printf("Auto-deploy rule added: %s -> %s/%s (autoCreate: %v, createService: %v)", rule.ImagePattern, rule.Namespace, rule.Deployment, rule.AutoCreate, rule.CreateService)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})
	case "DELETE":
		deployment := r.URL.Query().Get("deployment")
		autoDeployMu.Lock()
		delete(autoDeployRules, deployment)
		autoDeployMu.Unlock()
		go saveAutoDeployRules()
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleImages(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	repos, err := getRegistryRepos()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var images []RegistryImage
	for _, repo := range repos {
		tags, _ := getRepoTags(repo)
		images = append(images, RegistryImage{Name: repo, Tags: tags})
	}
	json.NewEncoder(w).Encode(images)
}

func handleForgeBuilds(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp, err := http.Get(forgeURL + "/api/builds")
	if err != nil {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}
	defer resp.Body.Close()
	var builds []interface{}
	json.NewDecoder(resp.Body).Decode(&builds)
	json.NewEncoder(w).Encode(builds)
}

func triggerForgeBuild(repo, branch, commit string) {
	payload := map[string]string{"repo": repo, "branch": branch, "commit": commit}
	data, _ := json.Marshal(payload)
	resp, err := http.Post(forgeURL+"/api/build", "application/json", strings.NewReader(string(data)))
	if err != nil {
		log.Printf("Failed to trigger Forge build: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("Forge build triggered for %s@%s", repo, branch)
}

func handleTriggerBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	go triggerForgeBuild(req.Repo, req.Branch, "")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "triggered"})
}

func handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html, err := os.ReadFile("/app/ui.html")
	if err != nil {
		http.Error(w, "UI not found", http.StatusInternalServerError)
		return
	}
	w.Write(html)
}

// Health check worker - monitors deployment rollouts
func healthCheckWorker() {
	log.Println("Starting health check worker")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		healthCheckMu.Lock()
		for key, hc := range pendingHealthChecks {
			if hc.Status == "pending" || hc.Status == "checking" {
				go checkDeploymentHealth(key, hc)
			}
		}
		healthCheckMu.Unlock()
	}
}

func checkDeploymentHealth(key string, hc *HealthCheckStatus) {
	if clientset == nil {
		return
	}

	ctx := context.Background()
	dep, err := clientset.AppsV1().Deployments(hc.Namespace).Get(ctx, hc.Deployment, metav1.GetOptions{})
	if err != nil {
		updateHealthCheckStatusFull(key, "failed", fmt.Sprintf("Failed to get deployment: %v", err), nil, nil)
		return
	}

	// Build rollout status from deployment
	rolloutStatus := buildRolloutStatus(dep)

	// Get pod statuses
	pods, err := clientset.CoreV1().Pods(hc.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", hc.Deployment),
	})

	var podStatuses []PodStatus
	if err == nil {
		for _, pod := range pods.Items {
			ready := false
			message := pod.Status.Message
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					ready = true
					break
				}
				// Capture waiting container reasons
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionFalse && cond.Message != "" {
					message = cond.Message
				}
			}
			// Check container statuses for more detailed error messages
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
					message = fmt.Sprintf("%s: %s", cs.State.Waiting.Reason, cs.State.Waiting.Message)
				}
				if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
					message = fmt.Sprintf("%s: %s (exit code %d)", cs.State.Terminated.Reason, cs.State.Terminated.Message, cs.State.Terminated.ExitCode)
				}
			}
			podStatuses = append(podStatuses, PodStatus{
				Name:    pod.Name,
				Ready:   ready,
				Phase:   string(pod.Status.Phase),
				Message: message,
			})
		}
	}

	healthCheckMu.Lock()
	hc.Attempts++
	hc.LastCheck = time.Now()
	hc.PodStatuses = podStatuses
	hc.RolloutStatus = rolloutStatus
	healthCheckMu.Unlock()

	// Check if rollout is stalled due to deployment conditions
	if rolloutStatus.Stalled {
		reason := fmt.Sprintf("Rollout stalled: %s", rolloutStatus.StalledReason)
		updateHealthCheckStatusFull(key, "failed", reason, podStatuses, rolloutStatus)
		addDeployEvent(hc.Deployment, hc.Namespace, hc.Image, hc.PreviousImage, "health-check", "failed", reason, time.Since(hc.StartTime).Seconds())
		updateDeployMetrics(hc.Deployment, hc.Namespace, hc.Image, hc.PreviousImage, "deploy", "failed", time.Since(hc.StartTime).Seconds())
		log.Printf("Health check failed for %s/%s: rollout stalled - %s", hc.Namespace, hc.Deployment, rolloutStatus.StalledReason)

		// Trigger auto-rollback if enabled
		if hc.AutoRollback && hc.PreviousImage != "" && !hc.RollbackTriggered {
			go triggerAutoRollback(hc.Deployment, hc.Namespace, hc.Image, hc.PreviousImage, reason)
			healthCheckMu.Lock()
			hc.RollbackTriggered = true
			hc.RollbackReason = reason
			healthCheckMu.Unlock()
		}
		return
	}

	// Check if deployment is ready
	if dep.Status.ReadyReplicas >= *dep.Spec.Replicas && dep.Status.UpdatedReplicas >= *dep.Spec.Replicas && dep.Status.AvailableReplicas >= *dep.Spec.Replicas {
		updateHealthCheckStatusFull(key, "healthy", "Deployment is healthy and ready", podStatuses, rolloutStatus)
		addDeployEvent(hc.Deployment, hc.Namespace, hc.Image, hc.PreviousImage, "health-check", "success", "Rollout completed successfully", time.Since(hc.StartTime).Seconds())
		addToHistory(hc.Deployment, hc.Image, hc.PreviousImage, "health-check", "success", time.Since(hc.StartTime).Seconds(), "")
		updateDeployMetrics(hc.Deployment, hc.Namespace, hc.Image, hc.PreviousImage, "deploy", "success", time.Since(hc.StartTime).Seconds())
		log.Printf("Health check passed for %s/%s", hc.Namespace, hc.Deployment)
		return
	}

	// Check if max attempts reached
	if hc.Attempts >= hc.MaxAttempts {
		reason := "Max health check attempts reached - deployment timed out"
		updateHealthCheckStatusFull(key, "failed", reason, podStatuses, rolloutStatus)
		addDeployEvent(hc.Deployment, hc.Namespace, hc.Image, hc.PreviousImage, "health-check", "failed", "Health check timed out", time.Since(hc.StartTime).Seconds())
		updateDeployMetrics(hc.Deployment, hc.Namespace, hc.Image, hc.PreviousImage, "deploy", "failed", time.Since(hc.StartTime).Seconds())
		log.Printf("Health check failed for %s/%s: max attempts reached", hc.Namespace, hc.Deployment)

		// Trigger auto-rollback if enabled
		if hc.AutoRollback && hc.PreviousImage != "" && !hc.RollbackTriggered {
			go triggerAutoRollback(hc.Deployment, hc.Namespace, hc.Image, hc.PreviousImage, reason)
			healthCheckMu.Lock()
			hc.RollbackTriggered = true
			hc.RollbackReason = reason
			healthCheckMu.Unlock()
		}
		return
	}

	msg := fmt.Sprintf("Rollout in progress: %d/%d updated, %d/%d ready, %d/%d available",
		rolloutStatus.UpdatedReplicas, rolloutStatus.Replicas,
		rolloutStatus.ReadyReplicas, rolloutStatus.Replicas,
		rolloutStatus.AvailableReplicas, rolloutStatus.Replicas)
	updateHealthCheckStatusFull(key, "checking", msg, podStatuses, rolloutStatus)
}

// buildRolloutStatus extracts rollout status from deployment conditions
func buildRolloutStatus(dep *appsv1.Deployment) *RolloutStatus {
	rs := &RolloutStatus{
		Replicas:            dep.Status.Replicas,
		UpdatedReplicas:     dep.Status.UpdatedReplicas,
		ReadyReplicas:       dep.Status.ReadyReplicas,
		AvailableReplicas:   dep.Status.AvailableReplicas,
		UnavailableReplicas: dep.Status.UnavailableReplicas,
		Stalled:             false,
	}

	// Check deployment conditions for stalled rollout
	for _, cond := range dep.Status.Conditions {
		rc := RolloutCondition{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		}
		rs.Conditions = append(rs.Conditions, rc)

		// Detect stalled rollout from Progressing condition
		if cond.Type == appsv1.DeploymentProgressing {
			if cond.Status == corev1.ConditionFalse {
				rs.Stalled = true
				rs.StalledReason = fmt.Sprintf("%s: %s", cond.Reason, cond.Message)
			}
			// Check for ProgressDeadlineExceeded
			if cond.Reason == "ProgressDeadlineExceeded" {
				rs.ProgressDeadline = true
				rs.Stalled = true
				rs.StalledReason = cond.Message
			}
		}

		// Detect replica failure
		if cond.Type == appsv1.DeploymentReplicaFailure && cond.Status == corev1.ConditionTrue {
			rs.Stalled = true
			rs.StalledReason = fmt.Sprintf("ReplicaFailure: %s", cond.Message)
		}
	}

	return rs
}

func updateHealthCheckStatus(key, status, message string, podStatuses []PodStatus) {
	updateHealthCheckStatusFull(key, status, message, podStatuses, nil)
}

func updateHealthCheckStatusFull(key, status, message string, podStatuses []PodStatus, rolloutStatus *RolloutStatus) {
	healthCheckMu.Lock()
	defer healthCheckMu.Unlock()
	if hc, ok := pendingHealthChecks[key]; ok {
		hc.Status = status
		hc.Message = message
		if podStatuses != nil {
			hc.PodStatuses = podStatuses
		}
		if rolloutStatus != nil {
			hc.RolloutStatus = rolloutStatus
		}
		if status == "healthy" || status == "failed" {
			// Remove from pending after a delay
			go func(k string) {
				time.Sleep(5 * time.Minute)
				healthCheckMu.Lock()
				delete(pendingHealthChecks, k)
				healthCheckMu.Unlock()
			}(key)
		}
	}
}

func startHealthCheck(deployment, namespace, image string) {
	// Check if auto-rollback is enabled for this deployment
	autoDeployMu.RLock()
	rule, hasRule := autoDeployRules[deployment]
	autoRollback := hasRule && rule.AutoRollback
	autoDeployMu.RUnlock()

	// Get previous image from current deployment
	var previousImage string
	deploymentsMu.RLock()
	if d, ok := deployments[deployment]; ok {
		previousImage = d.Image
	}
	deploymentsMu.RUnlock()

	key := fmt.Sprintf("%s/%s", namespace, deployment)
	healthCheckMu.Lock()
	pendingHealthChecks[key] = &HealthCheckStatus{
		Deployment:    deployment,
		Namespace:     namespace,
		Image:         image,
		PreviousImage: previousImage,
		Status:        "pending",
		StartTime:     time.Now(),
		LastCheck:     time.Now(),
		Attempts:      0,
		MaxAttempts:   60, // 5 minutes with 5-second intervals
		Message:       "Waiting for deployment to start",
		AutoRollback:  autoRollback,
	}
	healthCheckMu.Unlock()
	log.Printf("Started health check for %s/%s (autoRollback: %v)", namespace, deployment, autoRollback)
}

func handleHealthChecks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	healthCheckMu.RLock()
	defer healthCheckMu.RUnlock()
	json.NewEncoder(w).Encode(pendingHealthChecks)
}

func handleScale(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Deployment string `json:"deployment"`
		Namespace  string `json:"namespace"`
		Replicas   int32  `json:"replicas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "holm"
	}
	if clientset == nil {
		http.Error(w, "Kubernetes client not initialized", http.StatusInternalServerError)
		return
	}

	ctx := context.Background()
	dep, err := clientset.AppsV1().Deployments(req.Namespace).Get(ctx, req.Deployment, metav1.GetOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	dep.Spec.Replicas = &req.Replicas
	_, err = clientset.AppsV1().Deployments(req.Namespace).Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "scaled", "replicas": req.Replicas})
}

// Load auto-deploy rules from ConfigMap
func loadAutoDeployRules() {
	if clientset == nil {
		log.Println("Kubernetes client not available, skipping rules load")
		return
	}

	ctx := context.Background()
	cm, err := clientset.CoreV1().ConfigMaps("holm").Get(ctx, rulesConfigMap, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			log.Printf("ConfigMap %s not found, will create on first rule", rulesConfigMap)
			return
		}
		log.Printf("Failed to load rules: %v", err)
		return
	}

	if data, ok := cm.Data["rules"]; ok {
		autoDeployMu.Lock()
		if err := json.Unmarshal([]byte(data), &autoDeployRules); err != nil {
			log.Printf("Failed to parse rules: %v", err)
		} else {
			log.Printf("Loaded %d auto-deploy rules", len(autoDeployRules))
		}
		autoDeployMu.Unlock()
	}
}

// Save auto-deploy rules to ConfigMap
func saveAutoDeployRules() {
	if clientset == nil {
		return
	}

	autoDeployMu.RLock()
	data, err := json.Marshal(autoDeployRules)
	autoDeployMu.RUnlock()
	if err != nil {
		log.Printf("Failed to marshal rules: %v", err)
		return
	}

	ctx := context.Background()
	cm, err := clientset.CoreV1().ConfigMaps("holm").Get(ctx, rulesConfigMap, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new ConfigMap
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rulesConfigMap,
					Namespace: "holm",
				},
				Data: map[string]string{"rules": string(data)},
			}
			_, err = clientset.CoreV1().ConfigMaps("holm").Create(ctx, cm, metav1.CreateOptions{})
			if err != nil {
				log.Printf("Failed to create ConfigMap: %v", err)
			}
			return
		}
		log.Printf("Failed to get ConfigMap: %v", err)
		return
	}

	cm.Data["rules"] = string(data)
	_, err = clientset.CoreV1().ConfigMaps("holm").Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("Failed to update ConfigMap: %v", err)
	}
}

// Create Service for deployment
func ensureServiceExists(namespace, name string, port int) error {
	if clientset == nil || port == 0 {
		return nil
	}

	ctx := context.Background()
	_, err := clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil // Service exists
	}
	if !errors.IsNotFound(err) {
		return err
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                            name,
				"deploy-controller/auto-created": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{
					Port:       int32(port),
					TargetPort: intstr.FromInt(port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		log.Printf("Failed to create service %s: %v", name, err)
		return err
	}
	log.Printf("Created service %s/%s on port %d", namespace, name, port)
	return nil
}

// handlePodLogs returns logs from pods of a deployment
func handlePodLogs(w http.ResponseWriter, r *http.Request) {
	if clientset == nil {
		http.Error(w, "Kubernetes client not initialized", http.StatusInternalServerError)
		return
	}

	deployment := r.URL.Query().Get("deployment")
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "holm"
	}
	tailLines := int64(100)

	ctx := context.Background()

	// Get pods for this deployment
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", deployment),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type PodLog struct {
		PodName   string `json:"podName"`
		Container string `json:"container"`
		Logs      string `json:"logs"`
		Phase     string `json:"phase"`
		Ready     bool   `json:"ready"`
	}

	var podLogs []PodLog

	for _, pod := range pods.Items {
		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}

		for _, container := range pod.Spec.Containers {
			opts := &corev1.PodLogOptions{
				Container: container.Name,
				TailLines: &tailLines,
			}

			req := clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, opts)
			stream, err := req.Stream(ctx)
			if err != nil {
				podLogs = append(podLogs, PodLog{
					PodName:   pod.Name,
					Container: container.Name,
					Logs:      fmt.Sprintf("Error getting logs: %v", err),
					Phase:     string(pod.Status.Phase),
					Ready:     ready,
				})
				continue
			}

			buf := new(bytes.Buffer)
			_, err = io.Copy(buf, stream)
			stream.Close()

			logs := buf.String()
			if err != nil {
				logs = fmt.Sprintf("Error reading logs: %v", err)
			}

			podLogs = append(podLogs, PodLog{
				PodName:   pod.Name,
				Container: container.Name,
				Logs:      logs,
				Phase:     string(pod.Status.Phase),
				Ready:     ready,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deployment": deployment,
		"namespace":  namespace,
		"pods":       podLogs,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

// handleEventStream provides Server-Sent Events for real-time updates
func handleEventStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Send initial data
	sendSSEEvent(w, flusher, "init", map[string]interface{}{
		"message": "Connected to deploy controller",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastEventCount := 0
	lastHealthCheckHash := ""

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Send deployment updates with live rollout status
			deploymentsMu.RLock()
			deps := make([]*DeploymentInfo, 0, len(deployments))
			for _, d := range deployments {
				deps = append(deps, d)
			}
			deploymentsMu.RUnlock()
			sendSSEEvent(w, flusher, "deployments", deps)

			// Send new events if any
			recentDeploysMu.RLock()
			currentEventCount := len(recentDeploys)
			var newEvents []DeployEvent
			if currentEventCount > lastEventCount {
				newEvents = recentDeploys[:currentEventCount-lastEventCount]
			}
			recentDeploysMu.RUnlock()

			if len(newEvents) > 0 {
				sendSSEEvent(w, flusher, "events", newEvents)
				lastEventCount = currentEventCount
			}

			// Always send health checks with rollout status for active rollouts
			healthCheckMu.RLock()
			hasActiveChecks := false
			for _, hc := range pendingHealthChecks {
				if hc.Status == "checking" || hc.Status == "pending" {
					hasActiveChecks = true
					break
				}
			}

			// Compute a simple hash to detect changes
			currentHash := fmt.Sprintf("%d-%v", len(pendingHealthChecks), hasActiveChecks)
			for _, hc := range pendingHealthChecks {
				currentHash += fmt.Sprintf("-%s-%d", hc.Status, hc.Attempts)
			}
			healthCheckMu.RUnlock()

			// Send health checks if changed or if there are active checks
			if currentHash != lastHealthCheckHash || hasActiveChecks {
				healthCheckMu.RLock()
				sendSSEEvent(w, flusher, "health-checks", pendingHealthChecks)
				healthCheckMu.RUnlock()
				lastHealthCheckHash = currentHash
			}

			// Send rollout progress for each active deployment
			if hasActiveChecks {
				healthCheckMu.RLock()
				for key, hc := range pendingHealthChecks {
					if hc.Status == "checking" || hc.Status == "pending" {
						progress := map[string]interface{}{
							"key":           key,
							"deployment":    hc.Deployment,
							"namespace":     hc.Namespace,
							"status":        hc.Status,
							"message":       hc.Message,
							"attempts":      hc.Attempts,
							"maxAttempts":   hc.MaxAttempts,
							"rolloutStatus": hc.RolloutStatus,
							"podStatuses":   hc.PodStatuses,
							"elapsed":       time.Since(hc.StartTime).Seconds(),
						}
						sendSSEEvent(w, flusher, "rollout-progress", progress)
					}
				}
				healthCheckMu.RUnlock()
			}
		}
	}
}

func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\n", eventType)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

// handleApplyManifest applies a Kubernetes manifest (similar to kubectl apply -f)
func handleApplyManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if clientset == nil {
		http.Error(w, "Kubernetes client not initialized", http.StatusInternalServerError)
		return
	}

	var req struct {
		Manifest  string `json:"manifest"`
		Namespace string `json:"namespace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "holm"
	}

	ctx := context.Background()
	results := []map[string]interface{}{}

	// Parse YAML documents
	decoder := yaml.NewYAMLOrJSONDecoder(bufio.NewReader(strings.NewReader(req.Manifest)), 4096)

	for {
		var rawObj map[string]interface{}
		if err := decoder.Decode(&rawObj); err != nil {
			if err == io.EOF {
				break
			}
			results = append(results, map[string]interface{}{
				"status":  "error",
				"message": fmt.Sprintf("Failed to parse manifest: %v", err),
			})
			break
		}

		if rawObj == nil {
			continue
		}

		kind, _ := rawObj["kind"].(string)
		metadata, _ := rawObj["metadata"].(map[string]interface{})
		name, _ := metadata["name"].(string)
		ns, _ := metadata["namespace"].(string)
		if ns == "" {
			ns = req.Namespace
		}

		switch kind {
		case "Deployment":
			var dep appsv1.Deployment
			jsonBytes, _ := json.Marshal(rawObj)
			if err := json.Unmarshal(jsonBytes, &dep); err != nil {
				results = append(results, map[string]interface{}{
					"kind":    kind,
					"name":    name,
					"status":  "error",
					"message": err.Error(),
				})
				continue
			}

			if dep.Namespace == "" {
				dep.Namespace = ns
			}

			// Try update first, then create
			existing, err := clientset.AppsV1().Deployments(dep.Namespace).Get(ctx, dep.Name, metav1.GetOptions{})
			if err == nil {
				// Update existing
				dep.ResourceVersion = existing.ResourceVersion
				_, err = clientset.AppsV1().Deployments(dep.Namespace).Update(ctx, &dep, metav1.UpdateOptions{})
				if err != nil {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "error",
						"message": err.Error(),
					})
				} else {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "updated",
						"message": fmt.Sprintf("deployment.apps/%s configured", name),
					})
					addDeployEvent(name, dep.Namespace, dep.Spec.Template.Spec.Containers[0].Image, "", "manifest-apply", "deploying", "Manifest applied", 0)
					startHealthCheck(name, dep.Namespace, dep.Spec.Template.Spec.Containers[0].Image)
				}
			} else if errors.IsNotFound(err) {
				// Create new
				_, err = clientset.AppsV1().Deployments(dep.Namespace).Create(ctx, &dep, metav1.CreateOptions{})
				if err != nil {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "error",
						"message": err.Error(),
					})
				} else {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "created",
						"message": fmt.Sprintf("deployment.apps/%s created", name),
					})
					addDeployEvent(name, dep.Namespace, dep.Spec.Template.Spec.Containers[0].Image, "", "manifest-apply", "deploying", "Deployment created", 0)
					startHealthCheck(name, dep.Namespace, dep.Spec.Template.Spec.Containers[0].Image)
				}
			} else {
				results = append(results, map[string]interface{}{
					"kind":    kind,
					"name":    name,
					"status":  "error",
					"message": err.Error(),
				})
			}

		case "Service":
			var svc corev1.Service
			jsonBytes, _ := json.Marshal(rawObj)
			if err := json.Unmarshal(jsonBytes, &svc); err != nil {
				results = append(results, map[string]interface{}{
					"kind":    kind,
					"name":    name,
					"status":  "error",
					"message": err.Error(),
				})
				continue
			}

			if svc.Namespace == "" {
				svc.Namespace = ns
			}

			existing, err := clientset.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
			if err == nil {
				svc.ResourceVersion = existing.ResourceVersion
				svc.Spec.ClusterIP = existing.Spec.ClusterIP // Preserve ClusterIP
				_, err = clientset.CoreV1().Services(svc.Namespace).Update(ctx, &svc, metav1.UpdateOptions{})
				if err != nil {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "error",
						"message": err.Error(),
					})
				} else {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "updated",
						"message": fmt.Sprintf("service/%s configured", name),
					})
				}
			} else if errors.IsNotFound(err) {
				_, err = clientset.CoreV1().Services(svc.Namespace).Create(ctx, &svc, metav1.CreateOptions{})
				if err != nil {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "error",
						"message": err.Error(),
					})
				} else {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "created",
						"message": fmt.Sprintf("service/%s created", name),
					})
				}
			} else {
				results = append(results, map[string]interface{}{
					"kind":    kind,
					"name":    name,
					"status":  "error",
					"message": err.Error(),
				})
			}

		case "ConfigMap":
			var cm corev1.ConfigMap
			jsonBytes, _ := json.Marshal(rawObj)
			if err := json.Unmarshal(jsonBytes, &cm); err != nil {
				results = append(results, map[string]interface{}{
					"kind":    kind,
					"name":    name,
					"status":  "error",
					"message": err.Error(),
				})
				continue
			}

			if cm.Namespace == "" {
				cm.Namespace = ns
			}

			existing, err := clientset.CoreV1().ConfigMaps(cm.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
			if err == nil {
				cm.ResourceVersion = existing.ResourceVersion
				_, err = clientset.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, &cm, metav1.UpdateOptions{})
				if err != nil {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "error",
						"message": err.Error(),
					})
				} else {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "updated",
						"message": fmt.Sprintf("configmap/%s configured", name),
					})
				}
			} else if errors.IsNotFound(err) {
				_, err = clientset.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, &cm, metav1.CreateOptions{})
				if err != nil {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "error",
						"message": err.Error(),
					})
				} else {
					results = append(results, map[string]interface{}{
						"kind":    kind,
						"name":    name,
						"status":  "created",
						"message": fmt.Sprintf("configmap/%s created", name),
					})
				}
			} else {
				results = append(results, map[string]interface{}{
					"kind":    kind,
					"name":    name,
					"status":  "error",
					"message": err.Error(),
				})
			}

		default:
			results = append(results, map[string]interface{}{
				"kind":    kind,
				"name":    name,
				"status":  "skipped",
				"message": fmt.Sprintf("Unsupported kind: %s (only Deployment, Service, ConfigMap supported)", kind),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "completed",
		"results":   results,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// handleRestart restarts a deployment by patching the pod template annotation
func handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if clientset == nil {
		http.Error(w, "Kubernetes client not initialized", http.StatusInternalServerError)
		return
	}

	var req struct {
		Deployment string `json:"deployment"`
		Namespace  string `json:"namespace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "holm"
	}

	ctx := context.Background()
	dep, err := clientset.AppsV1().Deployments(req.Namespace).Get(ctx, req.Deployment, metav1.GetOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Add restart annotation to trigger rolling update
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}
	dep.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
	dep.Spec.Template.Annotations["deploy-controller/trigger"] = "restart"

	_, err = clientset.AppsV1().Deployments(req.Namespace).Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	image := ""
	if len(dep.Spec.Template.Spec.Containers) > 0 {
		image = dep.Spec.Template.Spec.Containers[0].Image
	}

	addDeployEvent(req.Deployment, req.Namespace, image, image, "restart", "deploying", "Deployment restarted", 0)
	startHealthCheck(req.Deployment, req.Namespace, image)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "restarted",
		"deployment": req.Deployment,
		"namespace":  req.Namespace,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

// handleK8sEvents returns Kubernetes events for a deployment
func handleK8sEvents(w http.ResponseWriter, r *http.Request) {
	if clientset == nil {
		http.Error(w, "Kubernetes client not initialized", http.StatusInternalServerError)
		return
	}

	deployment := r.URL.Query().Get("deployment")
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "holm"
	}

	ctx := context.Background()

	// Get events related to the deployment
	eventList, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", deployment),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type K8sEvent struct {
		Type           string    `json:"type"`
		Reason         string    `json:"reason"`
		Message        string    `json:"message"`
		Count          int32     `json:"count"`
		FirstTimestamp time.Time `json:"firstTimestamp"`
		LastTimestamp  time.Time `json:"lastTimestamp"`
		Source         string    `json:"source"`
	}

	var events []K8sEvent
	for _, e := range eventList.Items {
		events = append(events, K8sEvent{
			Type:           e.Type,
			Reason:         e.Reason,
			Message:        e.Message,
			Count:          e.Count,
			FirstTimestamp: e.FirstTimestamp.Time,
			LastTimestamp:  e.LastTimestamp.Time,
			Source:         e.Source.Component,
		})
	}

	// Also get pod events
	pods, _ := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", deployment),
	})

	for _, pod := range pods.Items {
		podEvents, _ := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("involvedObject.name=%s", pod.Name),
		})
		for _, e := range podEvents.Items {
			events = append(events, K8sEvent{
				Type:           e.Type,
				Reason:         e.Reason,
				Message:        fmt.Sprintf("[Pod: %s] %s", pod.Name, e.Message),
				Count:          e.Count,
				FirstTimestamp: e.FirstTimestamp.Time,
				LastTimestamp:  e.LastTimestamp.Time,
				Source:         e.Source.Component,
			})
		}
	}

	// Sort by last timestamp
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastTimestamp.After(events[j].LastTimestamp)
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deployment": deployment,
		"namespace":  namespace,
		"events":     events,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

// handleHistoryByDeployment handles /api/history/{deployment} endpoint
func handleHistoryByDeployment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract deployment name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/history/")
	deployment := strings.TrimSuffix(path, "/")

	if deployment == "" {
		http.Error(w, "Deployment name required", http.StatusBadRequest)
		return
	}

	historyMu.RLock()
	history := deploymentHistory[deployment]
	historyMu.RUnlock()

	if history == nil {
		history = []DeploymentVersion{}
	}

	// Include summary stats
	var successCount, failCount, rollbackCount int
	var totalDuration float64
	for _, v := range history {
		if v.Status == "success" {
			successCount++
			totalDuration += v.Duration
		} else if v.Status == "failed" {
			failCount++
		}
		if strings.Contains(v.Trigger, "rollback") {
			rollbackCount++
		}
	}

	avgDuration := 0.0
	if successCount > 0 {
		avgDuration = totalDuration / float64(successCount)
	}

	successRate := 0.0
	if len(history) > 0 {
		successRate = float64(successCount) / float64(len(history)) * 100
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"deployment":    deployment,
		"history":       history,
		"totalVersions": len(history),
		"successCount":  successCount,
		"failCount":     failCount,
		"rollbackCount": rollbackCount,
		"successRate":   successRate,
		"avgDeployTime": avgDuration,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})
}

// handleMetrics returns deployment metrics and statistics
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	deployMetricsMu.RLock()
	defer deployMetricsMu.RUnlock()

	// Calculate derived metrics
	successRate := 0.0
	if deployMetrics.TotalDeploys > 0 {
		successRate = float64(deployMetrics.SuccessfulDeploys) / float64(deployMetrics.TotalDeploys) * 100
	}

	rollbackRate := 0.0
	if deployMetrics.TotalDeploys > 0 {
		rollbackRate = float64(deployMetrics.RollbackCount) / float64(deployMetrics.TotalDeploys) * 100
	}

	// Calculate deploy frequency (deploys per hour over last 24 hours)
	deployFrequency := 0.0
	hourCount := len(deployMetrics.DeploysByHour)
	if hourCount > 0 {
		total := 0
		for _, count := range deployMetrics.DeploysByHour {
			total += count
		}
		deployFrequency = float64(total) / float64(hourCount)
	}

	response := map[string]interface{}{
		"summary": map[string]interface{}{
			"totalDeploys":      deployMetrics.TotalDeploys,
			"successfulDeploys": deployMetrics.SuccessfulDeploys,
			"failedDeploys":     deployMetrics.FailedDeploys,
			"rollbackCount":     deployMetrics.RollbackCount,
			"autoRollbackCount": deployMetrics.AutoRollbackCount,
			"successRate":       successRate,
			"rollbackRate":      rollbackRate,
			"deployFrequency":   deployFrequency,
			"avgDeployTime":     deployMetrics.AverageDeployTime,
			"lastDeployTime":    deployMetrics.LastDeployTime,
		},
		"deploysByHour": deployMetrics.DeploysByHour,
		"deploysByDay":  deployMetrics.DeploysByDay,
		"perDeployment": deployMetrics.PerDeployment,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(response)
}

// initializeMetrics sets up initial metrics structure
func initializeMetrics() {
	deployMetricsMu.Lock()
	defer deployMetricsMu.Unlock()

	deployMetrics = &DeploymentMetrics{
		DeploysByHour: make(map[string]int),
		DeploysByDay:  make(map[string]int),
		PerDeployment: make(map[string]*DeploymentStat),
	}

	// Initialize from existing history
	historyMu.RLock()
	for depName, history := range deploymentHistory {
		stat := &DeploymentStat{
			Name: depName,
		}
		for _, ver := range history {
			stat.TotalDeploys++
			deployMetrics.TotalDeploys++
			stat.TotalDeployTime += ver.Duration

			if ver.Status == "success" {
				stat.SuccessfulDeploys++
				deployMetrics.SuccessfulDeploys++
				if stat.LastSuccess.IsZero() || ver.Timestamp.After(stat.LastSuccess) {
					stat.LastSuccess = ver.Timestamp
				}
			} else if ver.Status == "failed" {
				stat.FailedDeploys++
				deployMetrics.FailedDeploys++
				if stat.LastFailure.IsZero() || ver.Timestamp.After(stat.LastFailure) {
					stat.LastFailure = ver.Timestamp
				}
			}

			if strings.Contains(ver.Trigger, "rollback") {
				stat.RollbackCount++
				deployMetrics.RollbackCount++
				if strings.Contains(ver.Trigger, "auto") {
					stat.AutoRollbackCount++
					deployMetrics.AutoRollbackCount++
				}
			}

			if stat.LastDeploy.IsZero() || ver.Timestamp.After(stat.LastDeploy) {
				stat.LastDeploy = ver.Timestamp
				stat.CurrentImage = ver.Image
			}

			// Track by hour and day
			hourKey := ver.Timestamp.Format("2006-01-02-15")
			dayKey := ver.Timestamp.Format("2006-01-02")
			deployMetrics.DeploysByHour[hourKey]++
			deployMetrics.DeploysByDay[dayKey]++
		}

		if stat.TotalDeploys > 0 {
			stat.AverageDeployTime = stat.TotalDeployTime / float64(stat.TotalDeploys)
			stat.SuccessRate = float64(stat.SuccessfulDeploys) / float64(stat.TotalDeploys) * 100
		}

		deployMetrics.PerDeployment[depName] = stat
	}
	historyMu.RUnlock()

	if deployMetrics.TotalDeploys > 0 {
		deployMetrics.AverageDeployTime = deployMetrics.TotalDeployTime / float64(deployMetrics.TotalDeploys)
	}

	log.Printf("Metrics initialized: %d total deploys, %.1f%% success rate",
		deployMetrics.TotalDeploys,
		float64(deployMetrics.SuccessfulDeploys)/float64(max(1, deployMetrics.TotalDeploys))*100)
}

// updateDeployMetrics updates metrics after a deployment
func updateDeployMetrics(deployment, namespace, image, oldImage, trigger, status string, duration float64) {
	deployMetricsMu.Lock()
	defer deployMetricsMu.Unlock()

	now := time.Now()
	deployMetrics.TotalDeploys++
	deployMetrics.LastDeployTime = now
	deployMetrics.TotalDeployTime += duration

	hourKey := now.Format("2006-01-02-15")
	dayKey := now.Format("2006-01-02")
	deployMetrics.DeploysByHour[hourKey]++
	deployMetrics.DeploysByDay[dayKey]++

	// Cleanup old hour entries (keep last 24 hours)
	cutoff := now.Add(-24 * time.Hour)
	for key := range deployMetrics.DeploysByHour {
		t, err := time.Parse("2006-01-02-15", key)
		if err == nil && t.Before(cutoff) {
			delete(deployMetrics.DeploysByHour, key)
		}
	}

	// Cleanup old day entries (keep last 30 days)
	cutoffDay := now.Add(-30 * 24 * time.Hour)
	for key := range deployMetrics.DeploysByDay {
		t, err := time.Parse("2006-01-02", key)
		if err == nil && t.Before(cutoffDay) {
			delete(deployMetrics.DeploysByDay, key)
		}
	}

	// Update per-deployment stats
	stat, ok := deployMetrics.PerDeployment[deployment]
	if !ok {
		stat = &DeploymentStat{
			Name:      deployment,
			Namespace: namespace,
		}
		deployMetrics.PerDeployment[deployment] = stat
	}

	stat.TotalDeploys++
	stat.LastDeploy = now
	stat.TotalDeployTime += duration
	stat.PreviousImage = oldImage
	stat.CurrentImage = image

	if status == "success" {
		stat.SuccessfulDeploys++
		deployMetrics.SuccessfulDeploys++
		stat.LastSuccess = now
	} else if status == "failed" {
		stat.FailedDeploys++
		deployMetrics.FailedDeploys++
		stat.LastFailure = now
	}

	if stat.TotalDeploys > 0 {
		stat.AverageDeployTime = stat.TotalDeployTime / float64(stat.TotalDeploys)
		stat.SuccessRate = float64(stat.SuccessfulDeploys) / float64(stat.TotalDeploys) * 100
	}

	if deployMetrics.TotalDeploys > 0 {
		deployMetrics.AverageDeployTime = deployMetrics.TotalDeployTime / float64(deployMetrics.TotalDeploys)
	}
}

// updateRollbackMetrics updates metrics after a rollback
func updateRollbackMetrics(deployment, namespace string, isAuto bool) {
	deployMetricsMu.Lock()
	defer deployMetricsMu.Unlock()

	deployMetrics.RollbackCount++
	if isAuto {
		deployMetrics.AutoRollbackCount++
	}

	if stat, ok := deployMetrics.PerDeployment[deployment]; ok {
		stat.RollbackCount++
		if isAuto {
			stat.AutoRollbackCount++
		}
	}
}

// startHealthCheckWithPrevious starts a health check with previous image info for auto-rollback
func startHealthCheckWithPrevious(deployment, namespace, image, previousImage string, autoRollback bool) {
	key := fmt.Sprintf("%s/%s", namespace, deployment)
	healthCheckMu.Lock()
	pendingHealthChecks[key] = &HealthCheckStatus{
		Deployment:    deployment,
		Namespace:     namespace,
		Image:         image,
		PreviousImage: previousImage,
		Status:        "pending",
		StartTime:     time.Now(),
		LastCheck:     time.Now(),
		Attempts:      0,
		MaxAttempts:   60, // 5 minutes with 5-second intervals
		Message:       "Waiting for deployment to start",
		AutoRollback:  autoRollback,
	}
	healthCheckMu.Unlock()
	log.Printf("Started health check for %s/%s (autoRollback: %v)", namespace, deployment, autoRollback)
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// triggerAutoRollback initiates an automatic rollback after deployment failure
func triggerAutoRollback(deployment, namespace, failedImage, previousImage, reason string) {
	log.Printf("Auto-rollback triggered for %s/%s: %s -> %s (reason: %s)",
		namespace, deployment, failedImage, previousImage, reason)

	// Add event for auto-rollback initiation
	addDeployEvent(deployment, namespace, previousImage, failedImage, "auto-rollback", "deploying",
		fmt.Sprintf("Auto-rollback initiated: %s", reason), 0)

	// Perform the rollback
	if err := performRollback(namespace, deployment, previousImage, failedImage, "auto"); err != nil {
		log.Printf("Auto-rollback failed for %s/%s: %v", namespace, deployment, err)
		addDeployEvent(deployment, namespace, previousImage, failedImage, "auto-rollback", "failed",
			fmt.Sprintf("Auto-rollback failed: %v", err), 0)
		return
	}

	// Update rollback metrics
	updateRollbackMetrics(deployment, namespace, true)

	log.Printf("Auto-rollback completed for %s/%s", namespace, deployment)
}

// getDeploymentVersions returns all versions for a deployment
func getDeploymentVersions(deployment string) []DeploymentVersion {
	historyMu.RLock()
	defer historyMu.RUnlock()

	if history, ok := deploymentHistory[deployment]; ok {
		result := make([]DeploymentVersion, len(history))
		copy(result, history)
		return result
	}
	return []DeploymentVersion{}
}

// findPreviousSuccessfulVersion finds the last successful deployment version
func findPreviousSuccessfulVersion(deployment string, skipCurrent bool) *DeploymentVersion {
	historyMu.RLock()
	defer historyMu.RUnlock()

	history := deploymentHistory[deployment]
	startIdx := 0
	if skipCurrent && len(history) > 0 {
		startIdx = 1
	}

	for i := startIdx; i < len(history); i++ {
		if history[i].Status == "success" {
			ver := history[i]
			return &ver
		}
	}
	return nil
}
