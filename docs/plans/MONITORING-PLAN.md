# HolmOS Monitoring Implementation Plan

## Executive Summary

This document outlines the plan to enhance HolmOS monitoring capabilities with a production-grade observability stack. The goal is to transition from custom in-house monitoring services to an industry-standard Prometheus + Grafana stack optimized for Raspberry Pi resource constraints.

---

## 1. Current State Analysis

### 1.1 Pulse Service (`/services/pulse/`)

**Purpose:** Real-time cluster health monitoring with WebSocket-based dashboard

**Capabilities:**
- Queries Kubernetes API directly for node, pod, and deployment metrics
- Polls metrics-server API (`/apis/metrics.k8s.io/v1beta1/`) for CPU/memory usage
- Calculates health scores (0-100) based on node readiness, pod failures, and resource utilization
- Generates resource alerts when thresholds exceeded (CPU > 80%, Memory > 80%)
- Provides WebSocket real-time updates at 10-second intervals
- Monitors vital signs: API Server, etcd, Scheduler, Controller Manager, CoreDNS

**Endpoints:**
- `/api/status` - Full cluster health status
- `/api/nodes` - Per-node status with CPU/memory percentages
- `/api/pods` - Problematic pods (failed, pending, high restarts)
- `/api/alerts` - Active resource alerts
- `/api/vitals` - Kubernetes component health
- `/ws` - WebSocket for real-time updates

**Resource Usage:**
- Requests: 50m CPU, 64Mi memory
- Limits: 200m CPU, 256Mi memory
- NodePort: 30006

**Limitations:**
- No historical data persistence (in-memory only)
- No external alerting (Slack, email, PagerDuty)
- Hardcoded 13-node cluster list
- No PromQL or flexible query language

### 1.2 Metrics Dashboard Service (`/services/metrics-dashboard/`)

**Purpose:** Comprehensive metrics visualization with historical data and alerting

**Capabilities:**
- In-memory metrics history (up to 8640 points = 24h at 10s intervals)
- Time-range queries: 1h, 6h, 24h, 7d, 30d
- Prometheus integration for historical queries (optional backend)
- Alert rules with conditions (>, >=, <, <=, ==, !=)
- Alert severity levels: critical, warning, info
- Namespace and deployment aggregation
- Top pods by CPU/memory views
- Legacy API compatibility endpoints

**Endpoints:**
- `/api/metrics` - Comprehensive cluster metrics
- `/api/nodes/{name}` - Detailed node metrics with history
- `/api/pods/{namespace}/{name}` - Detailed pod metrics
- `/api/history` - Time-series data with resolution options
- `/api/alerts` - Unified alerts API (rules + triggered)
- `/api/alerts/rules` - CRUD for alert rules
- `/api/cluster` - Cluster summary statistics

**Resource Usage:**
- Requests: 50m CPU, 64Mi memory
- Limits: 500m CPU, 256Mi memory
- NodePort: 30950

**Prometheus Integration:**
- Configured to use: `http://prometheus-kube-prometheus-prometheus.monitoring.svc.cluster.local:9090`
- Uses PromQL queries for CPU (`node_cpu_seconds_total`) and memory (`node_memory_MemAvailable_bytes`)
- Falls back to in-memory history if Prometheus unavailable

**Limitations:**
- In-memory history lost on pod restart
- No persistent alert state
- Alert rules not persisted (reset to defaults on restart)
- No Grafana-style dashboards
- Limited visualization options

### 1.3 Health Aggregation (Not Found)

The `health-agg` service was not found in the codebase. This functionality appears to be partially implemented within the Pulse service's cluster health aggregation logic.

### 1.4 Current Architecture Summary

```
                    +-------------------+
                    |  Kubernetes API   |
                    |  + Metrics Server |
                    +--------+----------+
                             |
            +----------------+----------------+
            |                                 |
    +-------v-------+               +---------v---------+
    |    Pulse      |               | Metrics Dashboard |
    | (Real-time)   |               |   (Historical)    |
    | Port: 30006   |               |   Port: 30950     |
    +---------------+               +-------------------+
            |                                 |
            |                        +--------v--------+
            |                        |   Prometheus    |
            |                        | (Optional/Ext)  |
            |                        +-----------------+
            |
    +-------v--------+
    |   WebSocket    |
    |   Dashboard    |
    +----------------+
```

---

## 2. Recommended Stack

### 2.1 Primary Recommendation: Prometheus + Grafana

**Why Prometheus:**
- Industry standard for Kubernetes monitoring
- Native Kubernetes service discovery
- Efficient time-series database with compression
- Rich ecosystem of exporters
- PromQL query language
- ARM64 native builds available

**Why Grafana:**
- Feature-rich visualization
- Pre-built Kubernetes dashboards
- Alert management with multiple notification channels
- ARM64 native builds available
- Low memory footprint in read-only mode

### 2.2 Alternative: VictoriaMetrics

**Why Consider VictoriaMetrics:**
- 10x lower memory usage vs Prometheus
- Faster query performance
- Long-term storage optimized
- Drop-in Prometheus replacement (PromQL compatible)
- Single binary deployment
- ARM64 native support

**Resource Comparison (per instance):**

| Component | Prometheus | VictoriaMetrics |
|-----------|------------|-----------------|
| Memory (idle) | 200-500 MB | 50-100 MB |
| Memory (active) | 500 MB - 2 GB | 100-300 MB |
| CPU (scraping) | 100-500m | 50-200m |
| Disk (1 week) | 5-20 GB | 2-8 GB |

**Recommendation:** Start with Prometheus for ecosystem compatibility, consider VictoriaMetrics if memory constraints become critical.

---

## 3. Implementation Phases

### Phase 1: Basic Metrics Collection (Week 1-2)

**Objectives:**
- Deploy lightweight Prometheus stack
- Configure Kubernetes metrics scraping
- Retain 7 days of metrics
- Replace direct K8s API polling in existing services

**Components to Deploy:**
1. **Prometheus Server** (ARM64)
   - Scrape interval: 30s (RPi optimized)
   - Retention: 7 days
   - Memory limit: 512 MB

2. **kube-state-metrics** (ARM64)
   - Exposes Kubernetes object metrics
   - Memory limit: 128 MB

3. **node-exporter** (DaemonSet, ARM64)
   - System-level metrics per node
   - Memory limit: 64 MB per pod

**Configuration Changes:**
- Update metrics-dashboard to use Prometheus exclusively
- Deprecate direct K8s API metrics fetching
- Keep Pulse for real-time WebSocket but source from Prometheus

**Success Criteria:**
- All 12 RPi nodes reporting metrics
- CPU, memory, disk, network metrics available
- Pod/deployment metrics exposed via kube-state-metrics
- Prometheus accessible at internal service URL

### Phase 2: Alerting (Week 3-4)

**Objectives:**
- Deploy Alertmanager
- Configure alert rules for critical conditions
- Set up notification channels

**Components to Deploy:**
1. **Alertmanager** (ARM64)
   - Memory limit: 64 MB
   - Persistent volume: 1 GB

**Alert Rules to Configure:**

| Alert Name | Condition | Severity | For |
|------------|-----------|----------|-----|
| NodeDown | up{job="node"} == 0 | critical | 2m |
| NodeMemoryPressure | node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes < 0.1 | critical | 5m |
| NodeCPUHigh | avg(rate(node_cpu_seconds_total{mode!="idle"}[5m])) > 0.9 | warning | 10m |
| NodeDiskPressure | node_filesystem_avail_bytes / node_filesystem_size_bytes < 0.1 | critical | 5m |
| PodCrashLooping | rate(kube_pod_container_status_restarts_total[15m]) > 0 | warning | 15m |
| PodNotReady | kube_pod_status_ready{condition="false"} == 1 | warning | 5m |
| DeploymentReplicasMismatch | kube_deployment_status_replicas_available != kube_deployment_spec_replicas | warning | 10m |
| PersistentVolumeSpaceLow | kubelet_volume_stats_available_bytes / kubelet_volume_stats_capacity_bytes < 0.15 | warning | 5m |

**Notification Channels:**
1. **Webhook** to existing notification-hub service
2. **Slack** (optional, if configured)
3. **Email** (optional, via SMTP)

**Success Criteria:**
- Alertmanager receiving alerts from Prometheus
- Test alert fires and reaches notification-hub
- Alert silencing and inhibition rules working

### Phase 3: Dashboards (Week 5-6)

**Objectives:**
- Deploy Grafana
- Import Kubernetes dashboards
- Create custom HolmOS dashboards
- Integrate with existing services

**Components to Deploy:**
1. **Grafana** (ARM64)
   - Memory limit: 256 MB
   - Persistent volume: 1 GB (dashboard storage)
   - Anonymous read-only access for dashboard embedding

**Dashboards to Create/Import:**

1. **Cluster Overview** (Custom)
   - Total nodes (ready/not ready)
   - Total pods (running/pending/failed)
   - Cluster CPU/memory utilization
   - Top 10 pods by resource usage
   - Recent alerts summary

2. **Node Detail** (Based on Node Exporter Full)
   - CPU usage breakdown
   - Memory usage and swap
   - Disk I/O and space
   - Network traffic
   - System load

3. **Kubernetes Resources** (Based on kube-prometheus)
   - Namespace resource usage
   - Deployment status
   - Pod restart rates
   - Container resource requests vs limits

4. **HolmOS Services** (Custom)
   - Per-service CPU/memory trends
   - Request rates (if instrumented)
   - Error rates
   - Response times (if instrumented)

**Integration Points:**
- Embed Grafana panels in existing dashboards (iframe)
- Link from Pulse dashboard to Grafana for detailed views
- Unified navigation across monitoring UIs

**Success Criteria:**
- Grafana accessible via ingress/NodePort
- All dashboards loading with data
- Anonymous access working for embedding
- Mobile-responsive layouts

---

## 4. Resource Requirements for Raspberry Pi Cluster

### 4.1 Hardware Assumptions

| Node | Role | RAM | Storage |
|------|------|-----|---------|
| rpi-1 to rpi-12 | Worker | 4-8 GB | 64-256 GB |
| openmediavault | Storage/Worker | 8+ GB | 1+ TB |

### 4.2 Recommended Deployment Strategy

**High-Availability Considerations:**
- Single Prometheus instance (RPi cluster size doesn't justify HA overhead)
- Grafana: Single instance with persistent storage
- Alertmanager: Single instance (can be replicated if needed)

**Node Placement:**
- Prometheus: Schedule on node with most RAM (8 GB preferred)
- Grafana: Any node, low resource footprint
- node-exporter: DaemonSet on all nodes
- kube-state-metrics: Any node

### 4.3 Total Resource Budget

| Component | Instances | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-----------|-----------|-------------|-----------|----------------|--------------|
| prometheus | 1 | 200m | 1000m | 256Mi | 512Mi |
| grafana | 1 | 50m | 200m | 128Mi | 256Mi |
| alertmanager | 1 | 25m | 100m | 32Mi | 64Mi |
| kube-state-metrics | 1 | 50m | 100m | 64Mi | 128Mi |
| node-exporter | 13 | 25m each | 100m each | 32Mi each | 64Mi each |

**Total Cluster Overhead:**
- CPU: ~1 core (requests), ~2.5 cores (limits)
- Memory: ~1 GB (requests), ~1.8 GB (limits)

### 4.4 Storage Requirements

| Component | PVC Size | Retention |
|-----------|----------|-----------|
| prometheus-data | 20 GB | 7 days |
| grafana-data | 1 GB | N/A |
| alertmanager-data | 1 GB | 5 days |

---

## 5. Sample Prometheus Configuration for ARM64

### 5.1 Prometheus Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: monitoring
  labels:
    app: prometheus
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      serviceAccountName: prometheus
      # Prefer nodes with more RAM
      affinity:
        nodeAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            preference:
              matchExpressions:
              - key: kubernetes.io/hostname
                operator: NotIn
                values:
                - openmediavault  # Avoid storage node
      containers:
      - name: prometheus
        image: prom/prometheus:v2.48.0  # ARM64 multi-arch
        args:
        - '--config.file=/etc/prometheus/prometheus.yml'
        - '--storage.tsdb.path=/prometheus'
        - '--storage.tsdb.retention.time=7d'
        - '--storage.tsdb.retention.size=15GB'
        - '--web.enable-lifecycle'
        - '--web.console.libraries=/usr/share/prometheus/console_libraries'
        - '--web.console.templates=/usr/share/prometheus/consoles'
        # RPi optimizations
        - '--query.max-concurrency=2'
        - '--query.timeout=2m'
        ports:
        - containerPort: 9090
          name: http
        resources:
          requests:
            cpu: 200m
            memory: 256Mi
          limits:
            cpu: 1000m
            memory: 512Mi
        volumeMounts:
        - name: prometheus-config
          mountPath: /etc/prometheus
        - name: prometheus-data
          mountPath: /prometheus
        livenessProbe:
          httpGet:
            path: /-/healthy
            port: http
          initialDelaySeconds: 30
          timeoutSeconds: 10
        readinessProbe:
          httpGet:
            path: /-/ready
            port: http
          initialDelaySeconds: 5
          timeoutSeconds: 5
      volumes:
      - name: prometheus-config
        configMap:
          name: prometheus-config
      - name: prometheus-data
        persistentVolumeClaim:
          claimName: prometheus-data
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: monitoring
spec:
  type: ClusterIP
  ports:
  - port: 9090
    targetPort: 9090
    name: http
  selector:
    app: prometheus
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus-nodeport
  namespace: monitoring
spec:
  type: NodePort
  ports:
  - port: 9090
    targetPort: 9090
    nodePort: 30090
    name: http
  selector:
    app: prometheus
```

### 5.2 Prometheus Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: monitoring
data:
  prometheus.yml: |
    global:
      scrape_interval: 30s      # RPi-optimized (less frequent)
      evaluation_interval: 30s
      scrape_timeout: 25s

    alerting:
      alertmanagers:
      - static_configs:
        - targets:
          - alertmanager:9093

    rule_files:
    - /etc/prometheus/rules/*.yml

    scrape_configs:
    # Prometheus self-monitoring
    - job_name: 'prometheus'
      static_configs:
      - targets: ['localhost:9090']

    # Kubernetes API server
    - job_name: 'kubernetes-apiservers'
      kubernetes_sd_configs:
      - role: endpoints
      scheme: https
      tls_config:
        ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        insecure_skip_verify: true
      bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
      relabel_configs:
      - source_labels: [__meta_kubernetes_namespace, __meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
        action: keep
        regex: default;kubernetes;https

    # Kubernetes nodes (kubelet)
    - job_name: 'kubernetes-nodes'
      kubernetes_sd_configs:
      - role: node
      scheme: https
      tls_config:
        ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        insecure_skip_verify: true
      bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
      relabel_configs:
      - action: labelmap
        regex: __meta_kubernetes_node_label_(.+)
      - target_label: __address__
        replacement: kubernetes.default.svc:443
      - source_labels: [__meta_kubernetes_node_name]
        regex: (.+)
        target_label: __metrics_path__
        replacement: /api/v1/nodes/${1}/proxy/metrics

    # cAdvisor (container metrics via kubelet)
    - job_name: 'kubernetes-cadvisor'
      kubernetes_sd_configs:
      - role: node
      scheme: https
      tls_config:
        ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        insecure_skip_verify: true
      bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
      relabel_configs:
      - action: labelmap
        regex: __meta_kubernetes_node_label_(.+)
      - target_label: __address__
        replacement: kubernetes.default.svc:443
      - source_labels: [__meta_kubernetes_node_name]
        regex: (.+)
        target_label: __metrics_path__
        replacement: /api/v1/nodes/${1}/proxy/metrics/cadvisor
      metric_relabel_configs:
      # Drop high-cardinality container metrics to save memory
      - source_labels: [__name__]
        regex: 'container_(network_tcp_usage_total|network_udp_usage_total|tasks_state)'
        action: drop

    # Node Exporter
    - job_name: 'node-exporter'
      kubernetes_sd_configs:
      - role: endpoints
      relabel_configs:
      - source_labels: [__meta_kubernetes_endpoints_name]
        action: keep
        regex: node-exporter
      - source_labels: [__meta_kubernetes_endpoint_node_name]
        target_label: node

    # kube-state-metrics
    - job_name: 'kube-state-metrics'
      static_configs:
      - targets: ['kube-state-metrics:8080']

    # Service endpoints with prometheus.io annotations
    - job_name: 'kubernetes-service-endpoints'
      kubernetes_sd_configs:
      - role: endpoints
      relabel_configs:
      - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_scheme]
        action: replace
        target_label: __scheme__
        regex: (https?)
      - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__address__, __meta_kubernetes_service_annotation_prometheus_io_port]
        action: replace
        target_label: __address__
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
      - action: labelmap
        regex: __meta_kubernetes_service_label_(.+)
      - source_labels: [__meta_kubernetes_namespace]
        action: replace
        target_label: kubernetes_namespace
      - source_labels: [__meta_kubernetes_service_name]
        action: replace
        target_label: kubernetes_name

    # Pods with prometheus.io annotations
    - job_name: 'kubernetes-pods'
      kubernetes_sd_configs:
      - role: pod
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
        target_label: __address__
      - action: labelmap
        regex: __meta_kubernetes_pod_label_(.+)
      - source_labels: [__meta_kubernetes_namespace]
        action: replace
        target_label: kubernetes_namespace
      - source_labels: [__meta_kubernetes_pod_name]
        action: replace
        target_label: kubernetes_pod_name
```

### 5.3 Node Exporter DaemonSet

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-exporter
  namespace: monitoring
  labels:
    app: node-exporter
spec:
  selector:
    matchLabels:
      app: node-exporter
  template:
    metadata:
      labels:
        app: node-exporter
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9100"
    spec:
      hostNetwork: true
      hostPID: true
      containers:
      - name: node-exporter
        image: prom/node-exporter:v1.7.0  # ARM64 multi-arch
        args:
        - '--path.procfs=/host/proc'
        - '--path.sysfs=/host/sys'
        - '--path.rootfs=/host/root'
        - '--collector.filesystem.mount-points-exclude=^/(dev|proc|sys|var/lib/docker/.+|var/lib/kubelet/.+)($|/)'
        - '--collector.netclass.ignored-devices=^(veth.*|docker.*|br-.*)$'
        # Disable collectors that are noisy or not needed
        - '--no-collector.arp'
        - '--no-collector.bcache'
        - '--no-collector.bonding'
        - '--no-collector.btrfs'
        - '--no-collector.infiniband'
        - '--no-collector.ipvs'
        - '--no-collector.nfs'
        - '--no-collector.nfsd'
        - '--no-collector.pressure'
        - '--no-collector.rapl'
        - '--no-collector.schedstat'
        - '--no-collector.softirqs'
        - '--no-collector.tapestats'
        - '--no-collector.xfs'
        - '--no-collector.zfs'
        ports:
        - containerPort: 9100
          hostPort: 9100
          name: metrics
        resources:
          requests:
            cpu: 25m
            memory: 32Mi
          limits:
            cpu: 100m
            memory: 64Mi
        volumeMounts:
        - name: proc
          mountPath: /host/proc
          readOnly: true
        - name: sys
          mountPath: /host/sys
          readOnly: true
        - name: root
          mountPath: /host/root
          mountPropagation: HostToContainer
          readOnly: true
        securityContext:
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65534
      volumes:
      - name: proc
        hostPath:
          path: /proc
      - name: sys
        hostPath:
          path: /sys
      - name: root
        hostPath:
          path: /
      tolerations:
      - operator: Exists
---
apiVersion: v1
kind: Service
metadata:
  name: node-exporter
  namespace: monitoring
  labels:
    app: node-exporter
spec:
  clusterIP: None
  ports:
  - port: 9100
    targetPort: 9100
    name: metrics
  selector:
    app: node-exporter
```

### 5.4 kube-state-metrics Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-state-metrics
  namespace: monitoring
  labels:
    app: kube-state-metrics
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-state-metrics
  template:
    metadata:
      labels:
        app: kube-state-metrics
    spec:
      serviceAccountName: kube-state-metrics
      containers:
      - name: kube-state-metrics
        image: registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.10.1  # ARM64 multi-arch
        args:
        - '--resources=certificatesigningrequests,configmaps,cronjobs,daemonsets,deployments,endpoints,horizontalpodautoscalers,ingresses,jobs,leases,limitranges,namespaces,networkpolicies,nodes,persistentvolumeclaims,persistentvolumes,poddisruptionbudgets,pods,replicasets,replicationcontrollers,resourcequotas,secrets,services,statefulsets,storageclasses'
        - '--metric-labels-allowlist=pods=[*],deployments=[*],nodes=[*]'
        ports:
        - containerPort: 8080
          name: http-metrics
        - containerPort: 8081
          name: telemetry
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 100m
            memory: 128Mi
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 5
          timeoutSeconds: 5
        readinessProbe:
          httpGet:
            path: /
            port: 8081
          initialDelaySeconds: 5
          timeoutSeconds: 5
        securityContext:
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65534
---
apiVersion: v1
kind: Service
metadata:
  name: kube-state-metrics
  namespace: monitoring
  labels:
    app: kube-state-metrics
spec:
  ports:
  - port: 8080
    targetPort: 8080
    name: http-metrics
  - port: 8081
    targetPort: 8081
    name: telemetry
  selector:
    app: kube-state-metrics
```

### 5.5 Alertmanager Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alertmanager
  namespace: monitoring
  labels:
    app: alertmanager
spec:
  replicas: 1
  selector:
    matchLabels:
      app: alertmanager
  template:
    metadata:
      labels:
        app: alertmanager
    spec:
      containers:
      - name: alertmanager
        image: prom/alertmanager:v0.26.0  # ARM64 multi-arch
        args:
        - '--config.file=/etc/alertmanager/alertmanager.yml'
        - '--storage.path=/alertmanager'
        - '--data.retention=120h'
        ports:
        - containerPort: 9093
          name: http
        resources:
          requests:
            cpu: 25m
            memory: 32Mi
          limits:
            cpu: 100m
            memory: 64Mi
        volumeMounts:
        - name: alertmanager-config
          mountPath: /etc/alertmanager
        - name: alertmanager-data
          mountPath: /alertmanager
        livenessProbe:
          httpGet:
            path: /-/healthy
            port: http
          initialDelaySeconds: 10
          timeoutSeconds: 5
        readinessProbe:
          httpGet:
            path: /-/ready
            port: http
          initialDelaySeconds: 5
          timeoutSeconds: 5
      volumes:
      - name: alertmanager-config
        configMap:
          name: alertmanager-config
      - name: alertmanager-data
        persistentVolumeClaim:
          claimName: alertmanager-data
---
apiVersion: v1
kind: Service
metadata:
  name: alertmanager
  namespace: monitoring
spec:
  ports:
  - port: 9093
    targetPort: 9093
    name: http
  selector:
    app: alertmanager
```

### 5.6 Grafana Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: monitoring
  labels:
    app: grafana
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grafana
  template:
    metadata:
      labels:
        app: grafana
    spec:
      securityContext:
        fsGroup: 472
      containers:
      - name: grafana
        image: grafana/grafana:10.2.3  # ARM64 multi-arch
        ports:
        - containerPort: 3000
          name: http
        env:
        - name: GF_SECURITY_ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: grafana-secrets
              key: admin-password
        - name: GF_INSTALL_PLUGINS
          value: ""
        - name: GF_AUTH_ANONYMOUS_ENABLED
          value: "true"
        - name: GF_AUTH_ANONYMOUS_ORG_ROLE
          value: "Viewer"
        - name: GF_SERVER_ROOT_URL
          value: "http://grafana.holm.local"
        resources:
          requests:
            cpu: 50m
            memory: 128Mi
          limits:
            cpu: 200m
            memory: 256Mi
        volumeMounts:
        - name: grafana-data
          mountPath: /var/lib/grafana
        - name: grafana-datasources
          mountPath: /etc/grafana/provisioning/datasources
        - name: grafana-dashboards-config
          mountPath: /etc/grafana/provisioning/dashboards
        livenessProbe:
          httpGet:
            path: /api/health
            port: http
          initialDelaySeconds: 60
          timeoutSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/health
            port: http
          initialDelaySeconds: 10
          timeoutSeconds: 5
      volumes:
      - name: grafana-data
        persistentVolumeClaim:
          claimName: grafana-data
      - name: grafana-datasources
        configMap:
          name: grafana-datasources
      - name: grafana-dashboards-config
        configMap:
          name: grafana-dashboards-config
---
apiVersion: v1
kind: Service
metadata:
  name: grafana
  namespace: monitoring
spec:
  type: NodePort
  ports:
  - port: 3000
    targetPort: 3000
    nodePort: 30030
    name: http
  selector:
    app: grafana
```

---

## 6. Migration Strategy

### 6.1 Parallel Running Phase

1. Deploy Prometheus stack alongside existing services
2. Configure metrics-dashboard to prefer Prometheus data
3. Validate data consistency between old and new systems
4. Run both systems for 2 weeks minimum

### 6.2 Cutover Phase

1. Update all dashboards to use Grafana
2. Configure alerts in Alertmanager
3. Deprecate in-memory history in metrics-dashboard
4. Keep Pulse for real-time WebSocket (source from Prometheus)

### 6.3 Cleanup Phase

1. Remove deprecated code paths from metrics-dashboard
2. Consider consolidating Pulse into metrics-dashboard
3. Document final architecture

---

## 7. Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Scrape success rate | > 99% | Prometheus metric |
| Alert latency | < 2 minutes | Alert timestamp diff |
| Dashboard load time | < 5 seconds | Grafana performance |
| Memory usage (total) | < 2 GB | kubectl top |
| Data retention | 7 days minimum | Prometheus query |

---

## 8. Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Memory pressure on RPi | Medium | High | Use VictoriaMetrics if needed |
| SD card wear from TSDB | Low | Medium | Use NFS volume from openmediavault |
| Network saturation from scraping | Low | Low | 30s scrape interval, metric filtering |
| Prometheus single point of failure | Medium | Medium | Regular config backups, fast recovery plan |

---

## 9. Timeline

| Week | Phase | Deliverables |
|------|-------|--------------|
| 1 | Setup | Namespace, RBAC, PVCs created |
| 2 | Phase 1 | Prometheus + node-exporter + kube-state-metrics deployed |
| 3 | Phase 2 Part 1 | Alertmanager deployed, basic rules configured |
| 4 | Phase 2 Part 2 | Notification channels configured, testing complete |
| 5 | Phase 3 Part 1 | Grafana deployed, datasource configured |
| 6 | Phase 3 Part 2 | Dashboards imported/created, integration tested |
| 7-8 | Migration | Parallel running, validation, cutover |

---

## 10. Appendix: VictoriaMetrics Alternative Configuration

If resource constraints require VictoriaMetrics, use this configuration:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: victoria-metrics
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: victoria-metrics
  template:
    metadata:
      labels:
        app: victoria-metrics
    spec:
      containers:
      - name: victoria-metrics
        image: victoriametrics/victoria-metrics:v1.96.0  # ARM64 multi-arch
        args:
        - '-storageDataPath=/victoria-metrics-data'
        - '-retentionPeriod=7d'
        - '-memory.allowedPercent=60'
        - '-search.maxUniqueTimeseries=50000'
        - '-promscrape.config=/etc/victoria-metrics/prometheus.yml'
        ports:
        - containerPort: 8428
          name: http
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 256Mi
        volumeMounts:
        - name: vm-data
          mountPath: /victoria-metrics-data
        - name: vm-config
          mountPath: /etc/victoria-metrics
      volumes:
      - name: vm-data
        persistentVolumeClaim:
          claimName: victoria-metrics-data
      - name: vm-config
        configMap:
          name: prometheus-config  # Reuse Prometheus config
```

VictoriaMetrics is fully PromQL-compatible and can use the same Prometheus configuration file.
