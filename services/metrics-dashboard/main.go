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
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Node      string    `json:"node,omitempty"`
}

type NodeMetrics struct {
	Name      string  `json:"name"`
	CPUCores  float64 `json:"cpu_cores"`
	CPUPct    float64 `json:"cpu_pct"`
	MemoryMB  float64 `json:"memory_mb"`
	MemoryPct float64 `json:"memory_pct"`
	Pods      int     `json:"pods"`
	Status    string  `json:"status"`
}

type PodMetrics struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	Node       string  `json:"node"`
	CPUm       float64 `json:"cpu_m"`
	MemoryMB   float64 `json:"memory_mb"`
	Status     string  `json:"status"`
	Deployment string  `json:"deployment"`
}

type DeploymentMetrics struct {
	Name       string       `json:"name"`
	Namespace  string       `json:"namespace"`
	Replicas   int          `json:"replicas"`
	Ready      int          `json:"ready"`
	CPUTotal   float64      `json:"cpu_total"`
	MemoryMB   float64      `json:"memory_mb"`
	PodMetrics []PodMetrics `json:"pods"`
}

type ClusterSummary struct {
	TotalNodes       int     `json:"total_nodes"`
	ReadyNodes       int     `json:"ready_nodes"`
	TotalPods        int     `json:"total_pods"`
	TotalDeployments int     `json:"total_deployments"`
	TotalCPUCores    float64 `json:"total_cpu_cores"`
	UsedCPUCores     float64 `json:"used_cpu_cores"`
	TotalMemoryGB    float64 `json:"total_memory_gb"`
	UsedMemoryGB     float64 `json:"used_memory_gb"`
	CPUPct           float64 `json:"cpu_pct"`
	MemoryPct        float64 `json:"memory_pct"`
}

type AlertRule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Metric      string    `json:"metric"`
	Condition   string    `json:"condition"`
	Threshold   float64   `json:"threshold"`
	Node        string    `json:"node"`
	Namespace   string    `json:"namespace,omitempty"`
	Pod         string    `json:"pod,omitempty"`
	Severity    string    `json:"severity"` // critical, warning, info
	Duration    int       `json:"duration"` // seconds threshold must be exceeded
	Enabled     bool      `json:"enabled"`
	Created     time.Time `json:"created"`
	LastChecked time.Time `json:"last_checked,omitempty"`
}

type TriggeredAlert struct {
	ID           string    `json:"id"`
	RuleID       string    `json:"rule_id"`
	RuleName     string    `json:"rule_name"`
	Message      string    `json:"message"`
	Value        float64   `json:"value"`
	Threshold    float64   `json:"threshold"`
	Severity     string    `json:"severity"`
	Node         string    `json:"node,omitempty"`
	Namespace    string    `json:"namespace,omitempty"`
	Pod          string    `json:"pod,omitempty"`
	Metric       string    `json:"metric"`
	Timestamp    time.Time `json:"timestamp"`
	ResolvedAt   time.Time `json:"resolved_at,omitempty"`
	Resolved     bool      `json:"resolved"`
	Acknowledged bool      `json:"acknowledged"`
}

// ClusterMetrics represents comprehensive cluster-wide metrics
type ClusterMetrics struct {
	Timestamp      time.Time              `json:"timestamp"`
	Summary        ClusterSummary         `json:"summary"`
	NodeMetrics    []NodeMetrics          `json:"node_metrics"`
	TopPodsByCPU   []PodMetrics           `json:"top_pods_by_cpu"`
	TopPodsByMem   []PodMetrics           `json:"top_pods_by_memory"`
	Namespaces     []NamespaceMetrics     `json:"namespaces"`
	ResourceQuotas []ResourceQuotaStatus  `json:"resource_quotas,omitempty"`
	Alerts         AlertsSummary          `json:"alerts"`
}

// NamespaceMetrics contains metrics aggregated by namespace
type NamespaceMetrics struct {
	Name        string  `json:"name"`
	PodCount    int     `json:"pod_count"`
	CPUUsage    float64 `json:"cpu_usage_m"`
	MemoryUsage float64 `json:"memory_usage_mb"`
	CPUPct      float64 `json:"cpu_pct"`
	MemoryPct   float64 `json:"memory_pct"`
}

// ResourceQuotaStatus represents quota usage
type ResourceQuotaStatus struct {
	Namespace  string  `json:"namespace"`
	CPUUsed    float64 `json:"cpu_used"`
	CPULimit   float64 `json:"cpu_limit"`
	MemUsed    float64 `json:"mem_used_mb"`
	MemLimit   float64 `json:"mem_limit_mb"`
	PodsUsed   int     `json:"pods_used"`
	PodsLimit  int     `json:"pods_limit"`
}

// AlertsSummary provides alert statistics
type AlertsSummary struct {
	Total      int `json:"total"`
	Critical   int `json:"critical"`
	Warning    int `json:"warning"`
	Info       int `json:"info"`
	Active     int `json:"active"`
	Resolved   int `json:"resolved"`
}

// NodeDetailedMetrics provides in-depth metrics for a single node
type NodeDetailedMetrics struct {
	NodeMetrics
	Allocatable   ResourceCapacity  `json:"allocatable"`
	Capacity      ResourceCapacity  `json:"capacity"`
	Conditions    []NodeCondition   `json:"conditions"`
	Pods          []PodMetrics      `json:"pods"`
	CPUHistory    []MetricPoint     `json:"cpu_history"`
	MemoryHistory []MetricPoint     `json:"memory_history"`
	Labels        map[string]string `json:"labels,omitempty"`
	Taints        []string          `json:"taints,omitempty"`
}

// ResourceCapacity represents node resource capacity
type ResourceCapacity struct {
	CPU     float64 `json:"cpu_cores"`
	Memory  float64 `json:"memory_mb"`
	Pods    int     `json:"pods"`
	Storage float64 `json:"storage_gb,omitempty"`
}

// NodeCondition represents node status conditions
type NodeCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// PodDetailedMetrics provides in-depth metrics for a single pod
type PodDetailedMetrics struct {
	PodMetrics
	Containers    []ContainerMetrics `json:"containers"`
	RestartCount  int                `json:"restart_count"`
	StartTime     time.Time          `json:"start_time"`
	CPUHistory    []MetricPoint      `json:"cpu_history"`
	MemoryHistory []MetricPoint      `json:"memory_history"`
	Events        []PodEvent         `json:"events,omitempty"`
	Labels        map[string]string  `json:"labels,omitempty"`
}

// ContainerMetrics represents metrics for individual containers
type ContainerMetrics struct {
	Name         string  `json:"name"`
	CPUm         float64 `json:"cpu_m"`
	MemoryMB     float64 `json:"memory_mb"`
	RestartCount int     `json:"restart_count"`
	Ready        bool    `json:"ready"`
	State        string  `json:"state"`
}

// PodEvent represents a Kubernetes event
type PodEvent struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Count     int       `json:"count"`
	Timestamp time.Time `json:"timestamp"`
}

// HistoricalData represents a time-series response
type HistoricalData struct {
	Metric     string        `json:"metric"`
	Node       string        `json:"node,omitempty"`
	Namespace  string        `json:"namespace,omitempty"`
	Pod        string        `json:"pod,omitempty"`
	Resolution string        `json:"resolution"`
	Start      time.Time     `json:"start"`
	End        time.Time     `json:"end"`
	Points     []MetricPoint `json:"points"`
}

// AlertsResponse provides unified alerts API response
type AlertsResponse struct {
	Rules     []AlertRule      `json:"rules"`
	Triggered []TriggeredAlert `json:"triggered"`
	Summary   AlertsSummary    `json:"summary"`
}

// K8s API response structures
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

type K8sNodeList struct {
	Items []K8sNode `json:"items"`
}

type K8sNode struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Taints []struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Effect string `json:"effect"`
		} `json:"taints"`
	} `json:"spec"`
	Status struct {
		Conditions []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"conditions"`
		Capacity struct {
			CPU     string `json:"cpu"`
			Memory  string `json:"memory"`
			Pods    string `json:"pods"`
			Storage string `json:"ephemeral-storage"`
		} `json:"capacity"`
		Allocatable struct {
			CPU     string `json:"cpu"`
			Memory  string `json:"memory"`
			Pods    string `json:"pods"`
			Storage string `json:"ephemeral-storage"`
		} `json:"allocatable"`
	} `json:"status"`
}

type K8sPodList struct {
	Items []K8sPod `json:"items"`
}

type K8sPod struct {
	Metadata struct {
		Name              string            `json:"name"`
		Namespace         string            `json:"namespace"`
		Labels            map[string]string `json:"labels"`
		CreationTimestamp string            `json:"creationTimestamp"`
		OwnerReferences   []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
	Spec struct {
		NodeName   string `json:"nodeName"`
		Containers []struct {
			Name      string `json:"name"`
			Resources struct {
				Requests struct {
					CPU    string `json:"cpu"`
					Memory string `json:"memory"`
				} `json:"requests"`
				Limits struct {
					CPU    string `json:"cpu"`
					Memory string `json:"memory"`
				} `json:"limits"`
			} `json:"resources"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase             string    `json:"phase"`
		StartTime         string    `json:"startTime"`
		ContainerStatuses []struct {
			Name         string `json:"name"`
			Ready        bool   `json:"ready"`
			RestartCount int    `json:"restartCount"`
			State        struct {
				Running    *struct{} `json:"running"`
				Waiting    *struct{ Reason string `json:"reason"` } `json:"waiting"`
				Terminated *struct{ Reason string `json:"reason"` } `json:"terminated"`
			} `json:"state"`
		} `json:"containerStatuses"`
	} `json:"status"`
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
		Replicas      int `json:"replicas"`
		ReadyReplicas int `json:"readyReplicas"`
	} `json:"status"`
}

// PrometheusResponse for historical queries
type PrometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
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

	now := time.Now()

	for i := range alertRules {
		rule := &alertRules[i]
		if !rule.Enabled {
			continue
		}
		rule.LastChecked = now

		for _, node := range nodes {
			if rule.Node != "" && rule.Node != "*" && rule.Node != node.Name {
				continue
			}

			var value float64
			var metricName string
			switch rule.Metric {
			case "cpu":
				value = node.CPUPct
				metricName = "CPU"
			case "memory":
				value = node.MemoryPct
				metricName = "Memory"
			case "pods":
				value = float64(node.Pods)
				metricName = "Pods"
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
			case "==":
				triggered = value == rule.Threshold
			case "!=":
				triggered = value != rule.Threshold
			}

			alertKey := fmt.Sprintf("%s-%s", rule.ID, node.Name)

			if triggered {
				// Check if this alert is already triggered
				alreadyTriggered := false
				for _, ta := range triggeredAlerts {
					if ta.RuleID == rule.ID && !ta.Resolved && ta.Node == node.Name {
						alreadyTriggered = true
						break
					}
				}

				if !alreadyTriggered {
					severity := rule.Severity
					if severity == "" {
						severity = "warning"
					}

					message := fmt.Sprintf("%s %s %.1f%% %s %.1f%% on node %s",
						metricName, rule.Condition, rule.Threshold, "- Current:", value, node.Name)

					triggeredAlerts = append(triggeredAlerts, TriggeredAlert{
						ID:        fmt.Sprintf("alert-%d-%s", now.UnixNano(), alertKey),
						RuleID:    rule.ID,
						RuleName:  rule.Name,
						Message:   message,
						Value:     value,
						Threshold: rule.Threshold,
						Severity:  severity,
						Node:      node.Name,
						Metric:    rule.Metric,
						Timestamp: now,
						Resolved:  false,
					})
					log.Printf("Alert triggered: %s (severity: %s)", message, severity)
				}
			} else {
				// Check if we should resolve an existing alert
				for i, ta := range triggeredAlerts {
					if ta.RuleID == rule.ID && !ta.Resolved && ta.Node == node.Name {
						triggeredAlerts[i].Resolved = true
						triggeredAlerts[i].ResolvedAt = now
						log.Printf("Alert resolved: %s on %s", rule.Name, node.Name)
						break
					}
				}
			}
		}
	}

	// Clean up old resolved alerts (keep for 24 hours)
	cutoff := now.Add(-24 * time.Hour)
	var activeAlerts []TriggeredAlert
	for _, alert := range triggeredAlerts {
		if !alert.Resolved || alert.ResolvedAt.After(cutoff) {
			activeAlerts = append(activeAlerts, alert)
		}
	}
	triggeredAlerts = activeAlerts
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

// Fetch comprehensive cluster-wide metrics
func fetchClusterMetrics() (*ClusterMetrics, error) {
	summary, err := fetchClusterSummary()
	if err != nil {
		return nil, err
	}

	nodes, err := fetchNodeMetrics()
	if err != nil {
		return nil, err
	}

	pods, err := fetchPodMetrics()
	if err != nil {
		return nil, err
	}

	// Get top pods by CPU
	sortedByCPU := make([]PodMetrics, len(pods))
	copy(sortedByCPU, pods)
	sort.Slice(sortedByCPU, func(i, j int) bool {
		return sortedByCPU[i].CPUm > sortedByCPU[j].CPUm
	})
	topCPU := sortedByCPU
	if len(topCPU) > 10 {
		topCPU = topCPU[:10]
	}

	// Get top pods by memory
	sortedByMem := make([]PodMetrics, len(pods))
	copy(sortedByMem, pods)
	sort.Slice(sortedByMem, func(i, j int) bool {
		return sortedByMem[i].MemoryMB > sortedByMem[j].MemoryMB
	})
	topMem := sortedByMem
	if len(topMem) > 10 {
		topMem = topMem[:10]
	}

	// Aggregate by namespace
	nsMetrics := make(map[string]*NamespaceMetrics)
	for _, pod := range pods {
		if _, ok := nsMetrics[pod.Namespace]; !ok {
			nsMetrics[pod.Namespace] = &NamespaceMetrics{Name: pod.Namespace}
		}
		nsMetrics[pod.Namespace].PodCount++
		nsMetrics[pod.Namespace].CPUUsage += pod.CPUm
		nsMetrics[pod.Namespace].MemoryUsage += pod.MemoryMB
	}

	var namespaces []NamespaceMetrics
	for _, ns := range nsMetrics {
		if summary.UsedCPUCores > 0 {
			ns.CPUPct = (ns.CPUUsage / summary.UsedCPUCores) * 100
		}
		if summary.UsedMemoryGB > 0 {
			ns.MemoryPct = (ns.MemoryUsage / (summary.UsedMemoryGB * 1024)) * 100
		}
		namespaces = append(namespaces, *ns)
	}
	sort.Slice(namespaces, func(i, j int) bool {
		return namespaces[i].CPUUsage > namespaces[j].CPUUsage
	})

	// Get alerts summary
	alertMutex.RLock()
	alertsSummary := AlertsSummary{}
	for _, alert := range triggeredAlerts {
		alertsSummary.Total++
		if alert.Resolved {
			alertsSummary.Resolved++
		} else {
			alertsSummary.Active++
			switch alert.Severity {
			case "critical":
				alertsSummary.Critical++
			case "warning":
				alertsSummary.Warning++
			case "info":
				alertsSummary.Info++
			}
		}
	}
	alertMutex.RUnlock()

	return &ClusterMetrics{
		Timestamp:    time.Now(),
		Summary:      *summary,
		NodeMetrics:  nodes,
		TopPodsByCPU: topCPU,
		TopPodsByMem: topMem,
		Namespaces:   namespaces,
		Alerts:       alertsSummary,
	}, nil
}

// Fetch detailed metrics for a specific node
func fetchNodeDetailedMetrics(nodeName string) (*NodeDetailedMetrics, error) {
	// Get node info
	nodeData, err := k8sAPIRequest("/api/v1/nodes/" + nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %v", err)
	}

	var node K8sNode
	if err := json.Unmarshal(nodeData, &node); err != nil {
		return nil, err
	}

	// Get node metrics
	metricsData, err := k8sAPIRequest("/apis/metrics.k8s.io/v1beta1/nodes/" + nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get node metrics: %v", err)
	}

	var nodeMetric K8sNodeMetric
	if err := json.Unmarshal(metricsData, &nodeMetric); err != nil {
		return nil, err
	}

	// Get pods on this node
	allPods, err := fetchPodMetrics()
	if err != nil {
		return nil, err
	}

	var nodePods []PodMetrics
	for _, pod := range allPods {
		if pod.Node == nodeName {
			nodePods = append(nodePods, pod)
		}
	}

	// Calculate metrics
	allocatableCPU := parseCPU(node.Status.Allocatable.CPU)
	allocatableMem := parseMemory(node.Status.Allocatable.Memory)
	cpuUsed := parseCPU(nodeMetric.Usage.CPU)
	memUsed := parseMemory(nodeMetric.Usage.Memory)

	cpuPct := 0.0
	if allocatableCPU > 0 {
		cpuPct = (cpuUsed / allocatableCPU) * 100
	}
	memPct := 0.0
	if allocatableMem > 0 {
		memPct = (memUsed / allocatableMem) * 100
	}

	// Get status
	status := "NotReady"
	var conditions []NodeCondition
	for _, cond := range node.Status.Conditions {
		conditions = append(conditions, NodeCondition{
			Type:    cond.Type,
			Status:  cond.Status,
			Message: cond.Message,
		})
		if cond.Type == "Ready" && cond.Status == "True" {
			status = "Ready"
		}
	}

	// Get taints
	var taints []string
	for _, taint := range node.Spec.Taints {
		taints = append(taints, fmt.Sprintf("%s=%s:%s", taint.Key, taint.Value, taint.Effect))
	}

	// Get history from memory
	historyMutex.RLock()
	cpuHistory := metricsHistory["node_cpu_"+nodeName]
	memHistory := metricsHistory["node_mem_"+nodeName]
	historyMutex.RUnlock()

	// Parse capacity values
	allocPods, _ := strconv.Atoi(node.Status.Allocatable.Pods)
	capPods, _ := strconv.Atoi(node.Status.Capacity.Pods)
	allocStorage := parseMemory(node.Status.Allocatable.Storage) / 1024 // to GB
	capStorage := parseMemory(node.Status.Capacity.Storage) / 1024

	return &NodeDetailedMetrics{
		NodeMetrics: NodeMetrics{
			Name:      nodeName,
			CPUCores:  cpuUsed,
			CPUPct:    cpuPct,
			MemoryMB:  memUsed,
			MemoryPct: memPct,
			Pods:      len(nodePods),
			Status:    status,
		},
		Allocatable: ResourceCapacity{
			CPU:     allocatableCPU,
			Memory:  allocatableMem,
			Pods:    allocPods,
			Storage: allocStorage,
		},
		Capacity: ResourceCapacity{
			CPU:     parseCPU(node.Status.Capacity.CPU),
			Memory:  parseMemory(node.Status.Capacity.Memory),
			Pods:    capPods,
			Storage: capStorage,
		},
		Conditions:    conditions,
		Pods:          nodePods,
		CPUHistory:    cpuHistory,
		MemoryHistory: memHistory,
		Labels:        node.Metadata.Labels,
		Taints:        taints,
	}, nil
}

// Fetch detailed metrics for a specific pod
func fetchPodDetailedMetrics(namespace, podName string) (*PodDetailedMetrics, error) {
	// Get pod info
	podData, err := k8sAPIRequest(fmt.Sprintf("/api/v1/namespaces/%s/pods/%s", namespace, podName))
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %v", err)
	}

	var pod K8sPod
	if err := json.Unmarshal(podData, &pod); err != nil {
		return nil, err
	}

	// Get pod metrics
	metricsData, err := k8sAPIRequest(fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/namespaces/%s/pods/%s", namespace, podName))
	if err != nil {
		return nil, fmt.Errorf("failed to get pod metrics: %v", err)
	}

	var podMetric K8sPodMetric
	if err := json.Unmarshal(metricsData, &podMetric); err != nil {
		return nil, err
	}

	// Build container metrics
	var containers []ContainerMetrics
	var totalCPU, totalMem float64
	totalRestarts := 0

	containerUsage := make(map[string]struct{ cpu, mem float64 })
	for _, c := range podMetric.Containers {
		cpu := parseCPU(c.Usage.CPU)
		mem := parseMemory(c.Usage.Memory)
		containerUsage[c.Name] = struct{ cpu, mem float64 }{cpu, mem}
		totalCPU += cpu
		totalMem += mem
	}

	for _, cs := range pod.Status.ContainerStatuses {
		usage := containerUsage[cs.Name]
		state := "unknown"
		if cs.State.Running != nil {
			state = "running"
		} else if cs.State.Waiting != nil {
			state = "waiting: " + cs.State.Waiting.Reason
		} else if cs.State.Terminated != nil {
			state = "terminated: " + cs.State.Terminated.Reason
		}

		containers = append(containers, ContainerMetrics{
			Name:         cs.Name,
			CPUm:         usage.cpu,
			MemoryMB:     usage.mem,
			RestartCount: cs.RestartCount,
			Ready:        cs.Ready,
			State:        state,
		})
		totalRestarts += cs.RestartCount
	}

	// Get deployment name
	deployment := ""
	for _, owner := range pod.Metadata.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			parts := strings.Split(owner.Name, "-")
			if len(parts) > 1 {
				deployment = strings.Join(parts[:len(parts)-1], "-")
			}
		}
	}

	// Parse start time
	startTime, _ := time.Parse(time.RFC3339, pod.Status.StartTime)

	// Get history from memory (if we stored pod-level history)
	historyMutex.RLock()
	cpuKey := fmt.Sprintf("pod_cpu_%s_%s", namespace, podName)
	memKey := fmt.Sprintf("pod_mem_%s_%s", namespace, podName)
	cpuHistory := metricsHistory[cpuKey]
	memHistory := metricsHistory[memKey]
	historyMutex.RUnlock()

	return &PodDetailedMetrics{
		PodMetrics: PodMetrics{
			Name:       podName,
			Namespace:  namespace,
			Node:       pod.Spec.NodeName,
			CPUm:       totalCPU,
			MemoryMB:   totalMem,
			Status:     pod.Status.Phase,
			Deployment: deployment,
		},
		Containers:    containers,
		RestartCount:  totalRestarts,
		StartTime:     startTime,
		CPUHistory:    cpuHistory,
		MemoryHistory: memHistory,
		Labels:        pod.Metadata.Labels,
	}, nil
}

// HTTP Handlers for new endpoints

func handleClusterMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := fetchClusterMetrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func handleNodeDetails(w http.ResponseWriter, r *http.Request) {
	// Extract node name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
	nodeName := strings.Split(path, "/")[0]

	if nodeName == "" {
		// List all nodes
		handleNodes(w, r)
		return
	}

	details, err := fetchNodeDetailedMetrics(nodeName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(details)
}

func handlePodDetails(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	podName := r.URL.Query().Get("name")

	// Also support path-based: /api/pods/namespace/name
	path := strings.TrimPrefix(r.URL.Path, "/api/pods/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && namespace == "" {
		namespace = parts[0]
		podName = parts[1]
	}

	if namespace == "" || podName == "" {
		// List all pods
		handlePods(w, r)
		return
	}

	details, err := fetchPodDetailedMetrics(namespace, podName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(details)
}

func handleUnifiedAlerts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		alertMutex.RLock()
		defer alertMutex.RUnlock()

		summary := AlertsSummary{}
		for _, alert := range triggeredAlerts {
			summary.Total++
			if alert.Resolved {
				summary.Resolved++
			} else {
				summary.Active++
				switch alert.Severity {
				case "critical":
					summary.Critical++
				case "warning":
					summary.Warning++
				case "info":
					summary.Info++
				}
			}
		}

		response := AlertsResponse{
			Rules:     alertRules,
			Triggered: triggeredAlerts,
			Summary:   summary,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case "POST":
		handleCreateAlertRule(w, r)

	case "DELETE":
		handleDeleteAlertRule(w, r)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	alertMutex.Lock()
	defer alertMutex.Unlock()

	for i, alert := range triggeredAlerts {
		if alert.ID == id {
			triggeredAlerts[i].Acknowledged = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	http.Error(w, "alert not found", http.StatusNotFound)
}

func handleHistoryExtended(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	rangeParam := r.URL.Query().Get("range")
	node := r.URL.Query().Get("node")
	namespace := r.URL.Query().Get("namespace")
	pod := r.URL.Query().Get("pod")
	resolution := r.URL.Query().Get("resolution")

	if rangeParam == "" {
		rangeParam = "1h"
	}

	var duration time.Duration
	switch rangeParam {
	case "5m":
		duration = 5 * time.Minute
	case "15m":
		duration = 15 * time.Minute
	case "30m":
		duration = 30 * time.Minute
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
	end := time.Now()

	// Determine resolution if not specified
	if resolution == "" {
		switch {
		case duration <= 30*time.Minute:
			resolution = "10s"
		case duration <= time.Hour:
			resolution = "30s"
		case duration <= 6*time.Hour:
			resolution = "1m"
		case duration <= 24*time.Hour:
			resolution = "5m"
		default:
			resolution = "15m"
		}
	}

	// Try Prometheus first for longer ranges
	if duration > time.Hour {
		promData, err := getPrometheusHistory(metric, node, duration)
		if err == nil && len(promData) > 0 {
			response := HistoricalData{
				Metric:     metric,
				Node:       node,
				Namespace:  namespace,
				Pod:        pod,
				Resolution: resolution,
				Start:      cutoff,
				End:        end,
				Points:     promData,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// Fall back to in-memory history
	historyMutex.RLock()
	defer historyMutex.RUnlock()

	var key string
	if pod != "" && namespace != "" {
		key = fmt.Sprintf("pod_%s_%s_%s", metric, namespace, pod)
	} else if node != "" && node != "*" {
		key = metric + "_" + node
	} else {
		key = metric
	}

	var points []MetricPoint
	if history, ok := metricsHistory[key]; ok {
		for _, p := range history {
			if p.Timestamp.After(cutoff) {
				points = append(points, p)
			}
		}
	}

	response := HistoricalData{
		Metric:     metric,
		Node:       node,
		Namespace:  namespace,
		Pod:        pod,
		Resolution: resolution,
		Start:      cutoff,
		End:        end,
		Points:     points,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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
		{ID: "default-1", Name: "Critical CPU", Metric: "cpu", Condition: ">", Threshold: 95, Node: "*", Severity: "critical", Duration: 60, Enabled: true, Created: time.Now()},
		{ID: "default-2", Name: "High CPU", Metric: "cpu", Condition: ">", Threshold: 85, Node: "*", Severity: "warning", Duration: 120, Enabled: true, Created: time.Now()},
		{ID: "default-3", Name: "Critical Memory", Metric: "memory", Condition: ">", Threshold: 95, Node: "*", Severity: "critical", Duration: 60, Enabled: true, Created: time.Now()},
		{ID: "default-4", Name: "High Memory", Metric: "memory", Condition: ">", Threshold: 85, Node: "*", Severity: "warning", Duration: 120, Enabled: true, Created: time.Now()},
		{ID: "default-5", Name: "Node Not Ready", Metric: "cpu", Condition: "<", Threshold: 0, Node: "*", Severity: "critical", Duration: 30, Enabled: false, Created: time.Now()},
	}

	// Setup HTTP server
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/health", handleHealth)

	// Cluster-wide metrics (new comprehensive endpoint)
	mux.HandleFunc("/api/metrics", handleClusterMetrics)

	// Node endpoints with detail support
	mux.HandleFunc("/api/nodes", handleNodes)
	mux.HandleFunc("/api/nodes/", handleNodeDetails) // /api/nodes/{name} for details

	// Pod endpoints with detail support
	mux.HandleFunc("/api/pods", handlePods)
	mux.HandleFunc("/api/pods/", handlePodDetails) // /api/pods/{namespace}/{name} for details

	// Deployments
	mux.HandleFunc("/api/deployments", handleDeployments)

	// Cluster summary
	mux.HandleFunc("/api/cluster", handleClusterSummary)

	// Historical data (enhanced)
	mux.HandleFunc("/api/history", handleHistoryExtended)

	// Unified alerts endpoint
	mux.HandleFunc("/api/alerts", handleUnifiedAlerts)

	// Alert management endpoints
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
	mux.HandleFunc("/api/alerts/acknowledge", handleAcknowledgeAlert)

	// Legacy API for backward compatibility
	mux.HandleFunc("/api/metrics/legacy", handleLegacyMetrics)
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
