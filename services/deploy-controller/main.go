package main

import (
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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	registryURL = os.Getenv("REGISTRY_URL")
	forgeURL    = os.Getenv("FORGE_URL")
	port        = os.Getenv("PORT")
	clientset   *kubernetes.Clientset

	deployments       = make(map[string]*DeploymentInfo)
	deploymentsMu     sync.RWMutex
	imageDigests      = make(map[string]string)
	imageDigestsMu    sync.RWMutex
	recentDeploys     []DeployEvent
	recentDeploysMu   sync.RWMutex
	webhookEvents     []WebhookEvent
	webhookEventsMu   sync.RWMutex
	autoDeployRules   = make(map[string]AutoDeployRule)
	autoDeployMu      sync.RWMutex
	deploymentHistory = make(map[string][]DeploymentVersion)
	historyMu         sync.RWMutex
	registryEvents    []RegistryEvent
	registryEventsMu  sync.RWMutex
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
	ImagePattern   string `json:"imagePattern"`
	Deployment     string `json:"deployment"`
	Namespace      string `json:"namespace"`
	Enabled        bool   `json:"enabled"`
	AutoCreate     bool   `json:"autoCreate"`
	TagPattern     string `json:"tagPattern"`
	LastTriggered  string `json:"lastTriggered"`
}

type RegistryImage struct {
	Name   string   `json:"name"`
	Tags   []string `json:"tags"`
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
		registryURL = "10.110.67.87:5000"
	}
	if forgeURL == "" {
		forgeURL = "http://forge.holm.svc.cluster.local"
	}
	if port == "" {
		port = "8080"
	}

	log.Printf("Deploy Controller v3 starting on port %s", port)
	log.Printf("Registry URL: %s", registryURL)
	log.Printf("Forge URL: %s", forgeURL)

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Warning: Running outside cluster: %v", err)
	} else {
		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			log.Fatalf("Failed to create k8s client: %v", err)
		}
	}

	// Load existing deployment history
	loadDeploymentHistory()

	go watchRegistry()
	go syncDeployments()

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
		"status":   "received",
		"events":   len(notification.Events),
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

	// Create new deployment
	log.Printf("Auto-creating deployment %s/%s with image %s", namespace, name, image)
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          name,
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
								{ContainerPort: 8080},
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
			deployments[dep.Name] = &DeploymentInfo{
				Name:       dep.Name,
				Namespace:  dep.Namespace,
				Image:      image,
				Status:     status,
				AutoDeploy: hasAutoDeploy,
				Replicas:   dep.Status.Replicas,
				Ready:      dep.Status.ReadyReplicas,
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
	addDeployEvent(deployment, namespace, image, oldImage, trigger, "success", "Deployment updated", duration)
	addToHistory(deployment, image, oldImage, trigger, "success", duration, "")
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
		"version": "3.0.0",
	})
}

func handleDeployments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	deploymentsMu.RLock()
	defer deploymentsMu.RUnlock()
	deps := make([]*DeploymentInfo, 0, len(deployments))
	for _, d := range deployments {
		deps = append(deps, d)
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Name < deps[j].Name })
	json.NewEncoder(w).Encode(deps)
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Namespace == "" {
		req.Namespace = "holm"
	}

	var targetImage string

	// If version specified, rollback to that specific version
	if req.Version > 0 {
		historyMu.RLock()
		history := deploymentHistory[req.Deployment]
		for _, ver := range history {
			if ver.Version == req.Version && ver.Status == "success" {
				targetImage = ver.Image
				break
			}
		}
		historyMu.RUnlock()
	} else {
		// Otherwise, rollback to previous successful version
		recentDeploysMu.RLock()
		for _, event := range recentDeploys {
			if event.Deployment == req.Deployment && event.Status == "success" && event.OldImage != "" {
				targetImage = event.OldImage
				break
			}
		}
		recentDeploysMu.RUnlock()
	}

	if targetImage == "" {
		http.Error(w, "No previous image found", http.StatusNotFound)
		return
	}
	if err := deployImage(req.Namespace, req.Deployment, targetImage, "rollback"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "rolled back", "image": targetImage})
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
		autoDeployMu.Lock()
		autoDeployRules[rule.Deployment] = rule
		autoDeployMu.Unlock()
		log.Printf("Auto-deploy rule added: %s -> %s/%s (autoCreate: %v)", rule.ImagePattern, rule.Namespace, rule.Deployment, rule.AutoCreate)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})
	case "DELETE":
		deployment := r.URL.Query().Get("deployment")
		autoDeployMu.Lock()
		delete(autoDeployRules, deployment)
		autoDeployMu.Unlock()
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
