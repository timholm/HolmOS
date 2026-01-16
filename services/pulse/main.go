package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WebSocket clients
var (
	clients     = make(map[*websocket.Conn]bool)
	clientsMux  sync.RWMutex
	broadcast   = make(chan HealthUpdate, 100)
)

// Health state
var (
	currentHealth HealthStatus
	healthMux     sync.RWMutex
	alertHistory  []Alert
	alertMux      sync.RWMutex
)

// Data structures
type HealthStatus struct {
	Status           string          `json:"status"`
	Message          string          `json:"message"`
	Timestamp        time.Time       `json:"timestamp"`
	ClusterHealth    ClusterHealth   `json:"cluster_health"`
	NodeStatuses     []NodeStatus    `json:"node_statuses"`
	PodStatuses      []PodStatus     `json:"pod_statuses"`
	ResourceAlerts   []ResourceAlert `json:"resource_alerts"`
	HealthScore      int             `json:"health_score"`
	VitalSigns       VitalSigns      `json:"vital_signs"`
}

type ClusterHealth struct {
	TotalNodes       int     `json:"total_nodes"`
	ReadyNodes       int     `json:"ready_nodes"`
	TotalPods        int     `json:"total_pods"`
	RunningPods      int     `json:"running_pods"`
	PendingPods      int     `json:"pending_pods"`
	FailedPods       int     `json:"failed_pods"`
	TotalCPUCores    float64 `json:"total_cpu_cores"`
	UsedCPUCores     float64 `json:"used_cpu_cores"`
	CPUPercent       float64 `json:"cpu_percent"`
	TotalMemoryGB    float64 `json:"total_memory_gb"`
	UsedMemoryGB     float64 `json:"used_memory_gb"`
	MemoryPercent    float64 `json:"memory_percent"`
	TotalDeployments int     `json:"total_deployments"`
	HealthyDeploys   int     `json:"healthy_deployments"`
}

type NodeStatus struct {
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	Ready        bool      `json:"ready"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemoryPercent float64  `json:"memory_percent"`
	PodCount     int       `json:"pod_count"`
	Conditions   []string  `json:"conditions"`
	LastChecked  time.Time `json:"last_checked"`
}

type PodStatus struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Node       string `json:"node"`
	Phase      string `json:"phase"`
	Ready      bool   `json:"ready"`
	Restarts   int    `json:"restarts"`
	CPUm       float64 `json:"cpu_m"`
	MemoryMB   float64 `json:"memory_mb"`
}

type ResourceAlert struct {
	ID        string    `json:"id"`
	Severity  string    `json:"severity"`
	Resource  string    `json:"resource"`
	Node      string    `json:"node"`
	Message   string    `json:"message"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Timestamp time.Time `json:"timestamp"`
}

type VitalSigns struct {
	Heartbeat       string `json:"heartbeat"`
	APIServerStatus string `json:"api_server_status"`
	ETCDStatus      string `json:"etcd_status"`
	SchedulerStatus string `json:"scheduler_status"`
	ControllerStatus string `json:"controller_status"`
	DNSStatus       string `json:"dns_status"`
}

type Alert struct {
	ID        string    `json:"id"`
	Severity  string    `json:"severity"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Resource  string    `json:"resource"`
	Timestamp time.Time `json:"timestamp"`
	Resolved  bool      `json:"resolved"`
}

type HealthUpdate struct {
	Type   string      `json:"type"`
	Data   interface{} `json:"data"`
}

// K8s API structures
type K8sNodeList struct {
	Items []K8sNode `json:"items"`
}

type K8sNode struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
		Capacity struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"capacity"`
		Allocatable struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"allocatable"`
	} `json:"status"`
}

type K8sNodeMetricsList struct {
	Items []K8sNodeMetric `json:"items"`
}

type K8sNodeMetric struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Usage struct {
		CPU    string `json:"cpu"`
		Memory string `json:"memory"`
	} `json:"usage"`
}

type K8sPodList struct {
	Items []K8sPod `json:"items"`
}

type K8sPod struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		NodeName string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		Phase             string `json:"phase"`
		ContainerStatuses []struct {
			Ready        bool `json:"ready"`
			RestartCount int  `json:"restartCount"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

type K8sPodMetricsList struct {
	Items []K8sPodMetric `json:"items"`
}

type K8sPodMetric struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Containers []struct {
		Name  string `json:"name"`
		Usage struct {
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
		} `json:"usage"`
	} `json:"containers"`
}

type K8sDeploymentList struct {
	Items []K8sDeployment `json:"items"`
}

type K8sDeployment struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Replicas int `json:"replicas"`
	} `json:"spec"`
	Status struct {
		ReadyReplicas int `json:"readyReplicas"`
		Replicas      int `json:"replicas"`
	} `json:"status"`
}

type K8sComponentStatusList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
	} `json:"items"`
}

// All 13 cluster nodes - hardcoded list
var allClusterNodes = []string{
	"rpi-1", "rpi-2", "rpi-3", "rpi-4", "rpi-5", "rpi-6",
	"rpi-7", "rpi-8", "rpi-9", "rpi-10", "rpi-11", "rpi-12",
	"openmediavault",
}

// K8s API client
func k8sAPIRequest(path string) ([]byte, error) {
	token, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, fmt.Errorf("failed to read service account token: %v", err)
	}

	apiServer := os.Getenv("KUBERNETES_SERVICE_HOST")
	apiPort := os.Getenv("KUBERNETES_SERVICE_PORT")
	if apiServer == "" {
		apiServer = "kubernetes.default.svc"
		apiPort = "443"
	}

	apiURL := fmt.Sprintf("https://%s:%s%s", apiServer, apiPort, path)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+string(token))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// Parse K8s resource values
func parseCPU(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "n") {
		val, _ := strconv.ParseFloat(strings.TrimSuffix(s, "n"), 64)
		return val / 1e9 * 1000 // Convert to millicores
	}
	if strings.HasSuffix(s, "m") {
		val, _ := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		return val
	}
	val, _ := strconv.ParseFloat(s, 64)
	return val * 1000 // Cores to millicores
}

func parseMemory(s string) float64 {
	s = strings.TrimSpace(s)
	multiplier := 1.0
	if strings.HasSuffix(s, "Ki") {
		s = strings.TrimSuffix(s, "Ki")
		multiplier = 1024
	} else if strings.HasSuffix(s, "Mi") {
		s = strings.TrimSuffix(s, "Mi")
		multiplier = 1024 * 1024
	} else if strings.HasSuffix(s, "Gi") {
		s = strings.TrimSuffix(s, "Gi")
		multiplier = 1024 * 1024 * 1024
	}
	val, _ := strconv.ParseFloat(s, 64)
	return val * multiplier / (1024 * 1024) // Return in MB
}

// Fetch cluster health
func fetchClusterHealth() (*HealthStatus, error) {
	status := &HealthStatus{
		Timestamp: time.Now(),
	}

	// Fetch nodes
	nodeData, err := k8sAPIRequest("/api/v1/nodes")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nodes: %v", err)
	}

	var nodeList K8sNodeList
	if err := json.Unmarshal(nodeData, &nodeList); err != nil {
		return nil, err
	}

	// Fetch node metrics
	nodeMetricsData, _ := k8sAPIRequest("/apis/metrics.k8s.io/v1beta1/nodes")
	var nodeMetricsList K8sNodeMetricsList
	json.Unmarshal(nodeMetricsData, &nodeMetricsList)

	metricsMap := make(map[string]K8sNodeMetric)
	for _, m := range nodeMetricsList.Items {
		metricsMap[m.Metadata.Name] = m
	}

	// Fetch pods
	podData, err := k8sAPIRequest("/api/v1/pods")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pods: %v", err)
	}

	var podList K8sPodList
	if err := json.Unmarshal(podData, &podList); err != nil {
		return nil, err
	}

	// Fetch pod metrics
	podMetricsData, _ := k8sAPIRequest("/apis/metrics.k8s.io/v1beta1/pods")
	var podMetricsList K8sPodMetricsList
	json.Unmarshal(podMetricsData, &podMetricsList)

	podMetricsMap := make(map[string]K8sPodMetric)
	for _, m := range podMetricsList.Items {
		key := m.Metadata.Namespace + "/" + m.Metadata.Name
		podMetricsMap[key] = m
	}

	// Fetch deployments
	deployData, _ := k8sAPIRequest("/apis/apps/v1/deployments")
	var deployList K8sDeploymentList
	json.Unmarshal(deployData, &deployList)

	// Build node statuses
	var totalCPUCores, usedCPUCores, totalMemMB, usedMemMB float64
	podCountByNode := make(map[string]int)

	for _, pod := range podList.Items {
		if pod.Status.Phase == "Running" {
			podCountByNode[pod.Spec.NodeName]++
		}
	}

	// Build a map of k8s nodes for quick lookup
	k8sNodeMap := make(map[string]K8sNode)
	for _, node := range nodeList.Items {
		k8sNodeMap[node.Metadata.Name] = node
	}

	// Include ALL 13 predefined cluster nodes, even if not in k8s
	for _, nodeName := range allClusterNodes {
		nodeStatus := NodeStatus{
			Name:        nodeName,
			LastChecked: time.Now(),
		}

		// Check if node exists in Kubernetes
		if node, exists := k8sNodeMap[nodeName]; exists {
			// Node is in k8s - get its real status
			for _, cond := range node.Status.Conditions {
				if cond.Type == "Ready" {
					nodeStatus.Ready = cond.Status == "True"
					if nodeStatus.Ready {
						nodeStatus.Status = "Ready"
					} else {
						nodeStatus.Status = "NotReady"
					}
				}
				if cond.Status == "True" && cond.Type != "Ready" {
					nodeStatus.Conditions = append(nodeStatus.Conditions, cond.Type)
				}
			}

			// Get resource usage
			allocCPU := parseCPU(node.Status.Allocatable.CPU)
			allocMem := parseMemory(node.Status.Allocatable.Memory)
			totalCPUCores += allocCPU
			totalMemMB += allocMem

			if metrics, ok := metricsMap[nodeName]; ok {
				usedCPU := parseCPU(metrics.Usage.CPU)
				usedMem := parseMemory(metrics.Usage.Memory)
				usedCPUCores += usedCPU
				usedMemMB += usedMem

				if allocCPU > 0 {
					nodeStatus.CPUPercent = (usedCPU / allocCPU) * 100
				}
				if allocMem > 0 {
					nodeStatus.MemoryPercent = (usedMem / allocMem) * 100
				}
			}
		} else {
			// Node is NOT in k8s yet - show as unknown/not joined
			nodeStatus.Status = "NotJoined"
			nodeStatus.Ready = false
			nodeStatus.Conditions = []string{"Not in Kubernetes cluster"}
		}

		nodeStatus.PodCount = podCountByNode[nodeName]
		status.NodeStatuses = append(status.NodeStatuses, nodeStatus)
	}

	// Sort nodes: rpi-1 through rpi-12 numerically, then openmediavault
	sort.Slice(status.NodeStatuses, func(i, j int) bool {
		nameI := status.NodeStatuses[i].Name
		nameJ := status.NodeStatuses[j].Name

		// Extract numeric part for rpi-N nodes
		getNodeOrder := func(name string) int {
			if name == "openmediavault" {
				return 100 // Always last
			}
			if strings.HasPrefix(name, "rpi-") {
				numStr := strings.TrimPrefix(name, "rpi-")
				num, err := strconv.Atoi(numStr)
				if err == nil {
					return num
				}
			}
			return 50 // Unknown nodes in the middle
		}

		return getNodeOrder(nameI) < getNodeOrder(nameJ)
	})

	// Build pod statuses (focus on non-running or high restart pods)
	for _, pod := range podList.Items {
		key := pod.Metadata.Namespace + "/" + pod.Metadata.Name

		podStatus := PodStatus{
			Name:      pod.Metadata.Name,
			Namespace: pod.Metadata.Namespace,
			Node:      pod.Spec.NodeName,
			Phase:     pod.Status.Phase,
		}

		// Check container statuses
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				podStatus.Ready = true
			}
			podStatus.Restarts += cs.RestartCount
		}

		// Get pod metrics
		if metrics, ok := podMetricsMap[key]; ok {
			for _, container := range metrics.Containers {
				podStatus.CPUm += parseCPU(container.Usage.CPU)
				podStatus.MemoryMB += parseMemory(container.Usage.Memory)
			}
		}

		// Only include problematic pods or sample of running pods
		if pod.Status.Phase != "Running" || podStatus.Restarts > 5 || !podStatus.Ready {
			status.PodStatuses = append(status.PodStatuses, podStatus)
		}
	}

	// Count pod phases
	var runningPods, pendingPods, failedPods int
	for _, pod := range podList.Items {
		switch pod.Status.Phase {
		case "Running":
			runningPods++
		case "Pending":
			pendingPods++
		case "Failed":
			failedPods++
		}
	}

	// Count deployments
	healthyDeploys := 0
	for _, deploy := range deployList.Items {
		if deploy.Status.ReadyReplicas >= deploy.Spec.Replicas {
			healthyDeploys++
		}
	}

	// Build cluster health
	readyNodes := 0
	for _, ns := range status.NodeStatuses {
		if ns.Ready {
			readyNodes++
		}
	}

	status.ClusterHealth = ClusterHealth{
		TotalNodes:       len(allClusterNodes), // Always show all 13 nodes
		ReadyNodes:       readyNodes,
		TotalPods:        len(podList.Items),
		RunningPods:      runningPods,
		PendingPods:      pendingPods,
		FailedPods:       failedPods,
		TotalCPUCores:    totalCPUCores,
		UsedCPUCores:     usedCPUCores,
		TotalMemoryGB:    totalMemMB / 1024,
		UsedMemoryGB:     usedMemMB / 1024,
		TotalDeployments: len(deployList.Items),
		HealthyDeploys:   healthyDeploys,
	}

	if totalCPUCores > 0 {
		status.ClusterHealth.CPUPercent = (usedCPUCores / totalCPUCores) * 100
	}
	if totalMemMB > 0 {
		status.ClusterHealth.MemoryPercent = (usedMemMB / totalMemMB) * 100
	}

	// Generate resource alerts
	for _, ns := range status.NodeStatuses {
		if ns.CPUPercent > 90 {
			status.ResourceAlerts = append(status.ResourceAlerts, ResourceAlert{
				ID:        fmt.Sprintf("cpu-high-%s", ns.Name),
				Severity:  "critical",
				Resource:  "cpu",
				Node:      ns.Name,
				Message:   fmt.Sprintf("CPU usage critical on %s", ns.Name),
				Value:     ns.CPUPercent,
				Threshold: 90,
				Timestamp: time.Now(),
			})
		} else if ns.CPUPercent > 80 {
			status.ResourceAlerts = append(status.ResourceAlerts, ResourceAlert{
				ID:        fmt.Sprintf("cpu-warn-%s", ns.Name),
				Severity:  "warning",
				Resource:  "cpu",
				Node:      ns.Name,
				Message:   fmt.Sprintf("CPU usage high on %s", ns.Name),
				Value:     ns.CPUPercent,
				Threshold: 80,
				Timestamp: time.Now(),
			})
		}

		if ns.MemoryPercent > 90 {
			status.ResourceAlerts = append(status.ResourceAlerts, ResourceAlert{
				ID:        fmt.Sprintf("mem-high-%s", ns.Name),
				Severity:  "critical",
				Resource:  "memory",
				Node:      ns.Name,
				Message:   fmt.Sprintf("Memory usage critical on %s", ns.Name),
				Value:     ns.MemoryPercent,
				Threshold: 90,
				Timestamp: time.Now(),
			})
		} else if ns.MemoryPercent > 80 {
			status.ResourceAlerts = append(status.ResourceAlerts, ResourceAlert{
				ID:        fmt.Sprintf("mem-warn-%s", ns.Name),
				Severity:  "warning",
				Resource:  "memory",
				Node:      ns.Name,
				Message:   fmt.Sprintf("Memory usage high on %s", ns.Name),
				Value:     ns.MemoryPercent,
				Threshold: 80,
				Timestamp: time.Now(),
			})
		}

		if !ns.Ready {
			var msg string
			var severity string
			if ns.Status == "NotJoined" {
				msg = fmt.Sprintf("Node %s has not joined the cluster", ns.Name)
				severity = "warning" // Not joined is warning, not critical
			} else {
				msg = fmt.Sprintf("Node %s is not ready", ns.Name)
				severity = "critical"
			}
			status.ResourceAlerts = append(status.ResourceAlerts, ResourceAlert{
				ID:        fmt.Sprintf("node-notready-%s", ns.Name),
				Severity:  severity,
				Resource:  "node",
				Node:      ns.Name,
				Message:   msg,
				Value:     0,
				Threshold: 0,
				Timestamp: time.Now(),
			})
		}
	}

	// Check vital signs
	status.VitalSigns = VitalSigns{
		Heartbeat:       "normal",
		APIServerStatus: "healthy",
	}

	// Try to get component statuses
	compData, err := k8sAPIRequest("/api/v1/componentstatuses")
	if err == nil {
		var compList K8sComponentStatusList
		if json.Unmarshal(compData, &compList) == nil {
			for _, comp := range compList.Items {
				healthy := false
				for _, cond := range comp.Conditions {
					if cond.Type == "Healthy" && cond.Status == "True" {
						healthy = true
						break
					}
				}
				switch comp.Metadata.Name {
				case "etcd-0", "etcd":
					if healthy {
						status.VitalSigns.ETCDStatus = "healthy"
					} else {
						status.VitalSigns.ETCDStatus = "unhealthy"
					}
				case "scheduler":
					if healthy {
						status.VitalSigns.SchedulerStatus = "healthy"
					} else {
						status.VitalSigns.SchedulerStatus = "unhealthy"
					}
				case "controller-manager":
					if healthy {
						status.VitalSigns.ControllerStatus = "healthy"
					} else {
						status.VitalSigns.ControllerStatus = "unhealthy"
					}
				}
			}
		}
	}

	// Check CoreDNS
	for _, deploy := range deployList.Items {
		if deploy.Metadata.Name == "coredns" && deploy.Metadata.Namespace == "kube-system" {
			if deploy.Status.ReadyReplicas > 0 {
				status.VitalSigns.DNSStatus = "healthy"
			} else {
				status.VitalSigns.DNSStatus = "unhealthy"
			}
			break
		}
	}

	// Calculate health score
	score := 100
	if status.ClusterHealth.ReadyNodes < status.ClusterHealth.TotalNodes {
		score -= (status.ClusterHealth.TotalNodes - status.ClusterHealth.ReadyNodes) * 15
	}
	if status.ClusterHealth.FailedPods > 0 {
		score -= status.ClusterHealth.FailedPods * 5
	}
	if status.ClusterHealth.PendingPods > 5 {
		score -= 10
	}
	if status.ClusterHealth.CPUPercent > 90 {
		score -= 15
	} else if status.ClusterHealth.CPUPercent > 80 {
		score -= 5
	}
	if status.ClusterHealth.MemoryPercent > 90 {
		score -= 15
	} else if status.ClusterHealth.MemoryPercent > 80 {
		score -= 5
	}
	if len(status.ResourceAlerts) > 0 {
		for _, alert := range status.ResourceAlerts {
			if alert.Severity == "critical" {
				score -= 10
			} else {
				score -= 5
			}
		}
	}
	if score < 0 {
		score = 0
	}
	status.HealthScore = score

	// Determine overall status and message
	if score >= 90 {
		status.Status = "healthy"
		status.Message = "Vital signs are looking good"
	} else if score >= 70 {
		status.Status = "warning"
		status.Message = "Some vital signs need attention"
	} else if score >= 50 {
		status.Status = "degraded"
		status.Message = "Cluster health is degraded"
	} else {
		status.Status = "critical"
		status.Message = "Critical issues detected"
	}

	return status, nil
}

// Health monitor loop
func healthMonitor() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		health, err := fetchClusterHealth()
		if err != nil {
			log.Printf("Error fetching health: %v", err)
			health = &HealthStatus{
				Status:    "unknown",
				Message:   "Unable to fetch cluster health",
				Timestamp: time.Now(),
			}
		}

		healthMux.Lock()
		currentHealth = *health
		healthMux.Unlock()

		// Broadcast to WebSocket clients
		broadcast <- HealthUpdate{
			Type: "health_update",
			Data: health,
		}

		<-ticker.C
	}
}

// WebSocket broadcaster
func handleBroadcast() {
	for update := range broadcast {
		clientsMux.RLock()
		for client := range clients {
			err := client.WriteJSON(update)
			if err != nil {
				log.Printf("WebSocket write error: %v", err)
				client.Close()
				clientsMux.RUnlock()
				clientsMux.Lock()
				delete(clients, client)
				clientsMux.Unlock()
				clientsMux.RLock()
			}
		}
		clientsMux.RUnlock()
	}
}

// HTTP Handlers
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "pulse",
		"message": "Vital signs are looking good",
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	healthMux.RLock()
	health := currentHealth
	healthMux.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

func handleNodes(w http.ResponseWriter, r *http.Request) {
	healthMux.RLock()
	nodes := currentHealth.NodeStatuses
	healthMux.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func handlePods(w http.ResponseWriter, r *http.Request) {
	healthMux.RLock()
	pods := currentHealth.PodStatuses
	healthMux.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pods)
}

func handleAlerts(w http.ResponseWriter, r *http.Request) {
	healthMux.RLock()
	alerts := currentHealth.ResourceAlerts
	healthMux.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

func handleVitals(w http.ResponseWriter, r *http.Request) {
	healthMux.RLock()
	vitals := currentHealth.VitalSigns
	healthMux.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vitals)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	clientsMux.Lock()
	clients[conn] = true
	clientsMux.Unlock()

	// Send current health immediately
	healthMux.RLock()
	health := currentHealth
	healthMux.RUnlock()

	conn.WriteJSON(HealthUpdate{
		Type: "health_update",
		Data: health,
	})

	// Keep connection alive and handle disconnection
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			clientsMux.Lock()
			delete(clients, conn)
			clientsMux.Unlock()
			conn.Close()
			break
		}
	}
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(getDashboardHTML()))
}

func main() {
	// Initialize health status
	currentHealth = HealthStatus{
		Status:    "initializing",
		Message:   "Starting health monitor...",
		Timestamp: time.Now(),
	}

	// Start health monitor
	go healthMonitor()

	// Start WebSocket broadcaster
	go handleBroadcast()

	// Setup HTTP server
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/nodes", handleNodes)
	mux.HandleFunc("/api/pods", handlePods)
	mux.HandleFunc("/api/alerts", handleAlerts)
	mux.HandleFunc("/api/vitals", handleVitals)
	mux.HandleFunc("/ws", handleWebSocket)

	// Dashboard
	mux.HandleFunc("/", handleDashboard)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Pulse health monitor starting on port %s", port)
	log.Printf("Vital signs are looking good")
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
