package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Metrics storage
var (
	metricsHistory   = make(map[string][]MetricPoint)
	alertRules       = []AlertRule{}
	triggeredAlerts  = []TriggeredAlert{}
	historyMutex     = sync.RWMutex{}
	alertMutex       = sync.RWMutex{}
	maxHistoryPoints = 8640 // 24 hours at 10-second intervals or 30 days at 5-min intervals
)

// Data structures
type MetricPoint struct {
	Timestamp time.Time \`json:"timestamp"\`
	Value     float64   \`json:"value"\`
	Node      string    \`json:"node,omitempty"\`
}

type NodeMetrics struct {
	Name      string  \`json:"name"\`
	CPUCores  float64 \`json:"cpu_cores"\`
	CPUPct    float64 \`json:"cpu_pct"\`
	MemoryMB  float64 \`json:"memory_mb"\`
	MemoryPct float64 \`json:"memory_pct"\`
	Pods      int     \`json:"pods"\`
	Status    string  \`json:"status"\`
}

type PodMetrics struct {
	Name       string  \`json:"name"\`
	Namespace  string  \`json:"namespace"\`
	Node       string  \`json:"node"\`
	CPUm       float64 \`json:"cpu_m"\`
	MemoryMB   float64 \`json:"memory_mb"\`
	Status     string  \`json:"status"\`
	Deployment string  \`json:"deployment"\`
}

type DeploymentMetrics struct {
	Name       string       \`json:"name"\`
	Namespace  string       \`json:"namespace"\`
	Replicas   int          \`json:"replicas"\`
	Ready      int          \`json:"ready"\`
	CPUTotal   float64      \`json:"cpu_total"\`
	MemoryMB   float64      \`json:"memory_mb"\`
	PodMetrics []PodMetrics \`json:"pods"\`
}

type ClusterSummary struct {
	TotalNodes       int     \`json:"total_nodes"\`
	ReadyNodes       int     \`json:"ready_nodes"\`
	TotalPods        int     \`json:"total_pods"\`
	TotalDeployments int     \`json:"total_deployments"\`
	TotalCPUCores    float64 \`json:"total_cpu_cores"\`
	UsedCPUCores     float64 \`json:"used_cpu_cores"\`
	TotalMemoryGB    float64 \`json:"total_memory_gb"\`
	UsedMemoryGB     float64 \`json:"used_memory_gb"\`
	CPUPct           float64 \`json:"cpu_pct"\`
	MemoryPct        float64 \`json:"memory_pct"\`
}

type AlertRule struct {
	ID        string    \`json:"id"\`
	Name      string    \`json:"name"\`
	Metric    string    \`json:"metric"\`
	Condition string    \`json:"condition"\`
	Threshold float64   \`json:"threshold"\`
	Node      string    \`json:"node"\`
	Enabled   bool      \`json:"enabled"\`
	Created   time.Time \`json:"created"\`
}

type TriggeredAlert struct {
	ID        string    \`json:"id"\`
	RuleID    string    \`json:"rule_id"\`
	RuleName  string    \`json:"rule_name"\`
	Message   string    \`json:"message"\`
	Value     float64   \`json:"value"\`
	Threshold float64   \`json:"threshold"\`
	Timestamp time.Time \`json:"timestamp"\`
	Resolved  bool      \`json:"resolved"\`
}

// K8s API response structures
type K8sNodeMetricsList struct {
	Items []K8sNodeMetric \`json:"items"\`
}

type K8sNodeMetric struct {
	Metadata struct {
		Name string \`json:"name"\`
	} \`json:"metadata"\`
	Usage struct {
		CPU    string \`json:"cpu"\`
		Memory string \`json:"memory"\`
	} \`json:"usage"\`
}

type K8sPodMetricsList struct {
	Items []K8sPodMetric \`json:"items"\`
}

type K8sPodMetric struct {
	Metadata struct {
		Name      string \`json:"name"\`
		Namespace string \`json:"namespace"\`
	} \`json:"metadata"\`
	Containers []struct {
		Name  string \`json:"name"\`
		Usage struct {
			CPU    string \`json:"cpu"\`
			Memory string \`json:"memory"\`
		} \`json:"usage"\`
	} \`json:"containers"\`
}

type K8sNodeList struct {
	Items []K8sNode \`json:"items"\`
}

type K8sNode struct {
	Metadata struct {
		Name string \`json:"name"\`
	} \`json:"metadata"\`
	Status struct {
		Conditions []struct {
			Type   string \`json:"type"\`
			Status string \`json:"status"\`
		} \`json:"conditions"\`
		Capacity struct {
			CPU    string \`json:"cpu"\`
			Memory string \`json:"memory"\`
		} \`json:"capacity"\`
		Allocatable struct {
			CPU    string \`json:"cpu"\`
			Memory string \`json:"memory"\`
		} \`json:"allocatable"\`
	} \`json:"status"\`
}

type K8sPodList struct {
	Items []K8sPod \`json:"items"\`
}

type K8sPod struct {
	Metadata struct {
		Name            string            \`json:"name"\`
		Namespace       string            \`json:"namespace"\`
		OwnerReferences []struct {
			Kind string \`json:"kind"\`
			Name string \`json:"name"\`
		} \`json:"ownerReferences"\`
	} \`json:"metadata"\`
	Spec struct {
		NodeName string \`json:"nodeName"\`
	} \`json:"spec"\`
	Status struct {
		Phase string \`json:"phase"\`
	} \`json:"status"\`
}

type K8sDeploymentList struct {
	Items []K8sDeployment \`json:"items"\`
}

type K8sDeployment struct {
	Metadata struct {
		Name      string \`json:"name"\`
		Namespace string \`json:"namespace"\`
	} \`json:"metadata"\`
	Spec struct {
		Replicas int \`json:"replicas"\`
	} \`json:"spec"\`
	Status struct {
		Replicas      int \`json:"replicas"\`
		ReadyReplicas int \`json:"readyReplicas"\`
	} \`json:"status"\`
}

// PrometheusResponse for historical queries
type PrometheusResponse struct {
	Status string \`json:"status"\`
	Data   struct {
		ResultType string \`json:"resultType"\`
		Result     []struct {
			Metric map[string]string \`json:"metric"\`
			Values [][]interface{}   \`json:"values"\`
		} \`json:"result"\`
	} \`json:"data"\`
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

// Prometheus query helper
func queryPrometheus(query string, start, end time.Time, step string) (*PrometheusResponse, error) {
	promURL := os.Getenv("PROMETHEUS_URL")
	if promURL == "" {
		promURL = "http://prometheus-kube-prometheus-prometheus.monitoring.svc.cluster.local:9090"
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(start.Unix(), 10))
	params.Set("end", strconv.FormatInt(end.Unix(), 10))
	params.Set("step", step)

	resp, err := http.Get(fmt.Sprintf("%s/api/v1/query_range?%s", promURL, params.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result PrometheusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
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

// Fetch all metrics from K8s
func fetchNodeMetrics() ([]NodeMetrics, error) {
	// Get node metrics
	metricsData, err := k8sAPIRequest("/apis/metrics.k8s.io/v1beta1/nodes")
	if err != nil {
		return nil, err
	}

	var nodeMetricsList K8sNodeMetricsList
	if err := json.Unmarshal(metricsData, &nodeMetricsList); err != nil {
		return nil, err
	}

	// Get node info for capacity
	nodeData, err := k8sAPIRequest("/api/v1/nodes")
	if err != nil {
		return nil, err
	}

	var nodeList K8sNodeList
	if err := json.Unmarshal(nodeData, &nodeList); err != nil {
		return nil, err
	}

	// Get pod counts per node
	podData, err := k8sAPIRequest("/api/v1/pods")
	if err != nil {
		return nil, err
	}

	var podList K8sPodList
	if err := json.Unmarshal(podData, &podList); err != nil {
		return nil, err
	}

	podCounts := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == "Running" {
			podCounts[pod.Spec.NodeName]++
		}
	}

	// Build node capacity map
	type nodeCapInfo struct {
		CPUCores float64
		MemoryMB float64
		Status   string
	}
	nodeCapacity := make(map[string]nodeCapInfo)

	for _, node := range nodeList.Items {
		status := "NotReady"
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				status = "Ready"
				break
			}
		}
		nodeCapacity[node.Metadata.Name] = nodeCapInfo{
			CPUCores: parseCPU(node.Status.Allocatable.CPU),
			MemoryMB: parseMemory(node.Status.Allocatable.Memory),
			Status:   status,
		}
	}

	// Build result
	var result []NodeMetrics
	for _, nm := range nodeMetricsList.Items {
		cap := nodeCapacity[nm.Metadata.Name]
		cpuUsed := parseCPU(nm.Usage.CPU)
		memUsed := parseMemory(nm.Usage.Memory)

		cpuPct := 0.0
		if cap.CPUCores > 0 {
			cpuPct = (cpuUsed / cap.CPUCores) * 100
		}
		memPct := 0.0
		if cap.MemoryMB > 0 {
			memPct = (memUsed / cap.MemoryMB) * 100
		}

		result = append(result, NodeMetrics{
			Name:      nm.Metadata.Name,
			CPUCores:  cpuUsed,
			CPUPct:    cpuPct,
			MemoryMB:  memUsed,
			MemoryPct: memPct,
			Pods:      podCounts[nm.Metadata.Name],
			Status:    cap.Status,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func fetchPodMetrics() ([]PodMetrics, error) {
	metricsData, err := k8sAPIRequest("/apis/metrics.k8s.io/v1beta1/pods")
	if err != nil {
		return nil, err
	}

	var podMetricsList K8sPodMetricsList
	if err := json.Unmarshal(metricsData, &podMetricsList); err != nil {
		return nil, err
	}

	// Get pod info for node and status
	podData, err := k8sAPIRequest("/api/v1/pods")
	if err != nil {
		return nil, err
	}

	var podList K8sPodList
	if err := json.Unmarshal(podData, &podList); err != nil {
		return nil, err
	}

	type podInfo struct {
		Node       string
		Status     string
		Deployment string
	}
	podInfoMap := make(map[string]podInfo)

	for _, pod := range podList.Items {
		key := pod.Metadata.Namespace + "/" + pod.Metadata.Name
		deployment := ""
		for _, owner := range pod.Metadata.OwnerReferences {
			if owner.Kind == "ReplicaSet" {
				// Extract deployment name from replicaset name (remove hash suffix)
				parts := strings.Split(owner.Name, "-")
				if len(parts) > 1 {
					deployment = strings.Join(parts[:len(parts)-1], "-")
				}
			}
		}
		podInfoMap[key] = podInfo{
			Node:       pod.Spec.NodeName,
			Status:     pod.Status.Phase,
			Deployment: deployment,
		}
	}

	var result []PodMetrics
	for _, pm := range podMetricsList.Items {
		key := pm.Metadata.Namespace + "/" + pm.Metadata.Name
		info := podInfoMap[key]

		var totalCPU, totalMem float64
		for _, container := range pm.Containers {
			totalCPU += parseCPU(container.Usage.CPU)
			totalMem += parseMemory(container.Usage.Memory)
		}

		result = append(result, PodMetrics{
			Name:       pm.Metadata.Name,
			Namespace:  pm.Metadata.Namespace,
			Node:       info.Node,
			CPUm:       totalCPU,
			MemoryMB:   totalMem,
			Status:     info.Status,
			Deployment: info.Deployment,
		})
	}

	return result, nil
}

func fetchDeploymentMetrics() ([]DeploymentMetrics, error) {
	// Get deployments
	deployData, err := k8sAPIRequest("/apis/apps/v1/deployments")
	if err != nil {
		return nil, err
	}

	var deployList K8sDeploymentList
	if err := json.Unmarshal(deployData, &deployList); err != nil {
		return nil, err
	}

	// Get pod metrics
	pods, err := fetchPodMetrics()
	if err != nil {
		return nil, err
	}

	// Group pods by deployment
	deployPods := make(map[string][]PodMetrics)
	for _, pod := range pods {
		if pod.Deployment != "" {
			key := pod.Namespace + "/" + pod.Deployment
			deployPods[key] = append(deployPods[key], pod)
		}
	}

	var result []DeploymentMetrics
	for _, deploy := range deployList.Items {
		key := deploy.Metadata.Namespace + "/" + deploy.Metadata.Name
		pods := deployPods[key]

		var totalCPU, totalMem float64
		for _, pod := range pods {
			totalCPU += pod.CPUm
			totalMem += pod.MemoryMB
		}

		result = append(result, DeploymentMetrics{
			Name:       deploy.Metadata.Name,
			Namespace:  deploy.Metadata.Namespace,
			Replicas:   deploy.Status.Replicas,
			Ready:      deploy.Status.ReadyReplicas,
			CPUTotal:   totalCPU,
			MemoryMB:   totalMem,
			PodMetrics: pods,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CPUTotal > result[j].CPUTotal
	})

	return result, nil
}

func fetchClusterSummary() (*ClusterSummary, error) {
	nodes, err := fetchNodeMetrics()
	if err != nil {
		return nil, err
	}

	// Get node capacity info
	nodeData, err := k8sAPIRequest("/api/v1/nodes")
	if err != nil {
		return nil, err
	}

	var nodeList K8sNodeList
	if err := json.Unmarshal(nodeData, &nodeList); err != nil {
		return nil, err
	}

	var totalCPU, totalMem float64
	readyNodes := 0
	for _, node := range nodeList.Items {
		totalCPU += parseCPU(node.Status.Allocatable.CPU)
		totalMem += parseMemory(node.Status.Allocatable.Memory)
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				readyNodes++
				break
			}
		}
	}

	var usedCPU, usedMem float64
	totalPods := 0
	for _, node := range nodes {
		usedCPU += node.CPUCores
		usedMem += node.MemoryMB
		totalPods += node.Pods
	}

	// Count deployments
	deployData, err := k8sAPIRequest("/apis/apps/v1/deployments")
	if err != nil {
		return nil, err
	}

	var deployList K8sDeploymentList
	json.Unmarshal(deployData, &deployList)

	cpuPct := 0.0
	if totalCPU > 0 {
		cpuPct = (usedCPU / totalCPU) * 100
	}
	memPct := 0.0
	if totalMem > 0 {
		memPct = (usedMem / totalMem) * 100
	}

	return &ClusterSummary{
		TotalNodes:       len(nodeList.Items),
		ReadyNodes:       readyNodes,
		TotalPods:        totalPods,
		TotalDeployments: len(deployList.Items),
		TotalCPUCores:    totalCPU,
		UsedCPUCores:     usedCPU,
		TotalMemoryGB:    totalMem / 1024,
		UsedMemoryGB:     usedMem / 1024,
		CPUPct:           cpuPct,
		MemoryPct:        memPct,
	}, nil
}

// Record metrics for history
func recordMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		nodes, err := fetchNodeMetrics()
		if err != nil {
			log.Printf("Error fetching metrics: %v", err)
			continue
		}

		now := time.Now()
		historyMutex.Lock()

		// Record per-node metrics
		var totalCPU, totalMem float64
		for _, node := range nodes {
			totalCPU += node.CPUPct
			totalMem += node.MemoryPct

			cpuKey := "node_cpu_" + node.Name
			memKey := "node_mem_" + node.Name

			metricsHistory[cpuKey] = append(metricsHistory[cpuKey], MetricPoint{
				Timestamp: now,
				Value:     node.CPUPct,
				Node:      node.Name,
			})
			metricsHistory[memKey] = append(metricsHistory[memKey], MetricPoint{
				Timestamp: now,
				Value:     node.MemoryPct,
				Node:      node.Name,
			})

			// Trim history
			if len(metricsHistory[cpuKey]) > maxHistoryPoints {
				metricsHistory[cpuKey] = metricsHistory[cpuKey][len(metricsHistory[cpuKey])-maxHistoryPoints:]
			}
			if len(metricsHistory[memKey]) > maxHistoryPoints {
				metricsHistory[memKey] = metricsHistory[memKey][len(metricsHistory[memKey])-maxHistoryPoints:]
			}
		}

		// Record cluster-wide averages
		if len(nodes) > 0 {
			metricsHistory["cluster_cpu"] = append(metricsHistory["cluster_cpu"], MetricPoint{
				Timestamp: now,
				Value:     totalCPU / float64(len(nodes)),
			})
			metricsHistory["cluster_mem"] = append(metricsHistory["cluster_mem"], MetricPoint{
				Timestamp: now,
				Value:     totalMem / float64(len(nodes)),
			})

			if len(metricsHistory["cluster_cpu"]) > maxHistoryPoints {
				metricsHistory["cluster_cpu"] = metricsHistory["cluster_cpu"][len(metricsHistory["cluster_cpu"])-maxHistoryPoints:]
			}
			if len(metricsHistory["cluster_mem"]) > maxHistoryPoints {
				metricsHistory["cluster_mem"] = metricsHistory["cluster_mem"][len(metricsHistory["cluster_mem"])-maxHistoryPoints:]
			}
		}

		historyMutex.Unlock()

		// Check alert rules
		checkAlerts(nodes)
	}
}

func checkAlerts(nodes []NodeMetrics) {
	alertMutex.Lock()
	defer alertMutex.Unlock()

	for _, rule := range alertRules {
		if !rule.Enabled {
			continue
		}

		for _, node := range nodes {
			if rule.Node != "" && rule.Node != "*" && rule.Node != node.Name {
				continue
			}

			var value float64
			switch rule.Metric {
			case "cpu":
				value = node.CPUPct
			case "memory":
				value = node.MemoryPct
			default:
				continue
			}

			triggered := false
			switch rule.Condition {
			case ">":
				triggered = value > rule.Threshold
			case ">=":
				triggered = value >= rule.Threshold
			case "<":
				triggered = value < rule.Threshold
			case "<=":
				triggered = value <= rule.Threshold
			}

			if triggered {
				// Check if this alert is already triggered
				alreadyTriggered := false
				for _, ta := range triggeredAlerts {
					if ta.RuleID == rule.ID && !ta.Resolved && ta.Message == fmt.Sprintf("%s on %s", rule.Name, node.Name) {
						alreadyTriggered = true
						break
					}
				}

				if !alreadyTriggered {
					triggeredAlerts = append(triggeredAlerts, TriggeredAlert{
						ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
						RuleID:    rule.ID,
						RuleName:  rule.Name,
						Message:   fmt.Sprintf("%s on %s", rule.Name, node.Name),
						Value:     value,
						Threshold: rule.Threshold,
						Timestamp: time.Now(),
						Resolved:  false,
					})
				}
			}
		}
	}
}

// HTTP Handlers
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func handleNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := fetchNodeMetrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func handlePods(w http.ResponseWriter, r *http.Request) {
	pods, err := fetchPodMetrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter by namespace if provided
	namespace := r.URL.Query().Get("namespace")
	if namespace != "" {
		var filtered []PodMetrics
		for _, pod := range pods {
			if pod.Namespace == namespace {
				filtered = append(filtered, pod)
			}
		}
		pods = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pods)
}

func handleDeployments(w http.ResponseWriter, r *http.Request) {
	deployments, err := fetchDeploymentMetrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	if namespace != "" {
		var filtered []DeploymentMetrics
		for _, deploy := range deployments {
			if deploy.Namespace == namespace {
				filtered = append(filtered, deploy)
			}
		}
		deployments = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployments)
}

func handleClusterSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := fetchClusterSummary()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	rangeParam := r.URL.Query().Get("range")
	node := r.URL.Query().Get("node")

	if rangeParam == "" {
		rangeParam = "1h"
	}

	var duration time.Duration
	switch rangeParam {
	case "1h":
		duration = time.Hour
	case "6h":
		duration = 6 * time.Hour
	case "24h":
		duration = 24 * time.Hour
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	default:
		duration = time.Hour
	}

	cutoff := time.Now().Add(-duration)

	// Try Prometheus first for longer ranges
	if duration > time.Hour {
		promData, err := getPrometheusHistory(metric, node, duration)
		if err == nil && len(promData) > 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(promData)
			return
		}
	}

	// Fall back to in-memory history
	historyMutex.RLock()
	defer historyMutex.RUnlock()

	var key string
	if node != "" && node != "*" {
		key = metric + "_" + node
	} else {
		key = metric
	}

	var result []MetricPoint
	if points, ok := metricsHistory[key]; ok {
		for _, p := range points {
			if p.Timestamp.After(cutoff) {
				result = append(result, p)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func getPrometheusHistory(metric, node string, duration time.Duration) ([]MetricPoint, error) {
	end := time.Now()
	start := end.Add(-duration)

	var step string
	switch {
	case duration <= time.Hour:
		step = "15s"
	case duration <= 6*time.Hour:
		step = "1m"
	case duration <= 24*time.Hour:
		step = "5m"
	case duration <= 7*24*time.Hour:
		step = "30m"
	default:
		step = "2h"
	}

	var query string
	switch metric {
	case "cluster_cpu":
		query = "avg(100 - (avg by (instance) (rate(node_cpu_seconds_total{mode=\"idle\"}[5m])) * 100))"
	case "cluster_mem":
		query = "avg((1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100)"
	case "node_cpu":
		if node != "" {
			query = fmt.Sprintf("100 - (avg by (instance) (rate(node_cpu_seconds_total{mode=\"idle\", instance=~\"%s.*\"}[5m])) * 100)", node)
		} else {
			query = "100 - (avg by (instance) (rate(node_cpu_seconds_total{mode=\"idle\"}[5m])) * 100)"
		}
	case "node_mem":
		if node != "" {
			query = fmt.Sprintf("(1 - (node_memory_MemAvailable_bytes{instance=~\"%s.*\"} / node_memory_MemTotal_bytes{instance=~\"%s.*\"})) * 100", node, node)
		} else {
			query = "(1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100"
		}
	default:
		return nil, fmt.Errorf("unknown metric: %s", metric)
	}

	resp, err := queryPrometheus(query, start, end, step)
	if err != nil {
		return nil, err
	}

	var result []MetricPoint
	for _, series := range resp.Data.Result {
		nodeName := series.Metric["instance"]
		for _, val := range series.Values {
			ts, _ := val[0].(float64)
			valStr, _ := val[1].(string)
			value, _ := strconv.ParseFloat(valStr, 64)
			result = append(result, MetricPoint{
				Timestamp: time.Unix(int64(ts), 0),
				Value:     value,
				Node:      nodeName,
			})
		}
	}

	return result, nil
}

func handleAlertRules(w http.ResponseWriter, r *http.Request) {
	alertMutex.RLock()
	defer alertMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alertRules)
}

func handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	var rule AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	rule.Created = time.Now()
	if rule.Node == "" {
		rule.Node = "*"
	}

	alertMutex.Lock()
	alertRules = append(alertRules, rule)
	alertMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

func handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	alertMutex.Lock()
	defer alertMutex.Unlock()

	for i, rule := range alertRules {
		if rule.ID == id {
			alertRules = append(alertRules[:i], alertRules[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	http.Error(w, "rule not found", http.StatusNotFound)
}

func handleTriggeredAlerts(w http.ResponseWriter, r *http.Request) {
	alertMutex.RLock()
	defer alertMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(triggeredAlerts)
}

func handleResolveAlert(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	alertMutex.Lock()
	defer alertMutex.Unlock()

	for i, alert := range triggeredAlerts {
		if alert.ID == id {
			triggeredAlerts[i].Resolved = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	http.Error(w, "alert not found", http.StatusNotFound)
}

// Legacy API compatibility
func handleLegacyMetrics(w http.ResponseWriter, r *http.Request) {
	nodes, err := fetchNodeMetrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var cpuPoints, memPoints, reqPoints []map[string]interface{}
	now := time.Now()

	// Generate some historical-looking data points
	for i := 0; i < 20; i++ {
		ts := now.Add(time.Duration(-i*5) * time.Second)

		var totalCPU, totalMem float64
		for _, node := range nodes {
			totalCPU += node.CPUPct
			totalMem += node.MemoryPct
		}
		avgCPU := totalCPU / float64(len(nodes))
		avgMem := totalMem / float64(len(nodes))

		cpuPoints = append([]map[string]interface{}{{
			"name":      "cpu_usage",
			"value":     avgCPU,
			"unit":      "percent",
			"timestamp": ts.Format(time.RFC3339Nano),
		}}, cpuPoints...)

		memPoints = append([]map[string]interface{}{{
			"name":      "memory_usage",
			"value":     avgMem,
			"unit":      "percent",
			"timestamp": ts.Format(time.RFC3339Nano),
		}}, memPoints...)

		reqPoints = append([]map[string]interface{}{{
			"name":      "requests",
			"value":     float64(100 + i*5),
			"unit":      "req/s",
			"timestamp": ts.Format(time.RFC3339Nano),
		}}, reqPoints...)
	}

	response := map[string]interface{}{
		"cpu":      cpuPoints,
		"memory":   memPoints,
		"requests": reqPoints,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleLegacyServices(w http.ResponseWriter, r *http.Request) {
	deployments, err := fetchDeploymentMetrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var services []map[string]string
	for _, deploy := range deployments {
		if deploy.Namespace != "holm" {
			continue
		}
		status := "healthy"
		if deploy.Ready < deploy.Replicas {
			status = "unhealthy"
		}
		services = append(services, map[string]string{
			"name":      deploy.Name,
			"namespace": deploy.Namespace,
			"status":    status,
			"endpoint":  fmt.Sprintf("http://%s:8080", deploy.Name),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func main() {
	// Start metrics collection
	go recordMetrics()

	// Initialize default alert rules
	alertRules = []AlertRule{
		{ID: "default-1", Name: "High CPU", Metric: "cpu", Condition: ">", Threshold: 90, Node: "*", Enabled: true, Created: time.Now()},
		{ID: "default-2", Name: "High Memory", Metric: "memory", Condition: ">", Threshold: 90, Node: "*", Enabled: true, Created: time.Now()},
	}

	// Setup HTTP server
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/api/nodes", handleNodes)
	mux.HandleFunc("/api/pods", handlePods)
	mux.HandleFunc("/api/deployments", handleDeployments)
	mux.HandleFunc("/api/cluster", handleClusterSummary)
	mux.HandleFunc("/api/history", handleHistory)
	mux.HandleFunc("/api/alerts/rules", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			handleAlertRules(w, r)
		case "POST":
			handleCreateAlertRule(w, r)
		case "DELETE":
			handleDeleteAlertRule(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/alerts/triggered", handleTriggeredAlerts)
	mux.HandleFunc("/api/alerts/resolve", handleResolveAlert)

	// Legacy API for compatibility
	mux.HandleFunc("/api/metrics", handleLegacyMetrics)
	mux.HandleFunc("/api/services", handleLegacyServices)

	// Serve dashboard
	mux.HandleFunc("/", serveDashboard)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Metrics Dashboard on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
