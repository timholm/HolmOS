# PostgreSQL High Availability Plan

**Date**: January 2026
**Status**: Planning
**Author**: HolmOS Infrastructure Team

---

## Executive Summary

This document outlines the plan to migrate HolmOS PostgreSQL from a single-instance deployment to a highly available configuration. Given the 13-node Raspberry Pi cluster environment and current workload patterns, we evaluate three approaches and recommend the most suitable solution.

---

## 1. Current Setup Analysis

### 1.1 Existing Configuration

**Deployment Location**: `/Users/tim/HolmOS/services/backup/backup-storage/k8s/postgres.yaml`

```yaml
# Current single-instance PostgreSQL deployment
- Image: postgres:15-alpine
- Replicas: 1
- Namespace: holm
- Service: ClusterIP (postgres.holm.svc.cluster.local:5432)
- Storage: 5Gi PVC (local-path provisioner)
- Resources:
  - Requests: 128Mi memory, 100m CPU
  - Limits: 512Mi memory, 500m CPU
```

### 1.2 Current Architecture

```
+------------------+
|   Applications   |
|  (8+ services)   |
+--------+---------+
         |
         v
+------------------+     +------------------+
|  postgres        |---->|  postgres-pvc    |
|  (single pod)    |     |  5Gi local-path  |
|  postgres:15     |     +------------------+
+------------------+
```

### 1.3 Dependent Services

The following services connect to PostgreSQL:

| Service | Connection Pattern | Database |
|---------|-------------------|----------|
| backup-storage | DB_HOST env var | backups |
| auth-gateway | postgres-secret | postgres |
| terminal-web | DATABASE_URL secret | holm |
| audiobook-web | DB_HOST env var | postgres |
| settings-restore | DB_HOST env var | postgres |
| gitea | DB config | gitea |
| notification-webhook | postgres-secret | postgres |
| notification-queue | postgres-credentials | holm |
| event-persist | postgres-credentials | holm |
| event-replay | postgres-credentials | holm |
| event-dlq | postgres-credentials | holm |
| user-preferences | user-preferences-db-secret | holm |

### 1.4 Current Limitations

| Issue | Impact | Severity |
|-------|--------|----------|
| Single point of failure | Full database outage if pod fails | Critical |
| No automatic failover | Manual intervention required | High |
| Local-path storage | Data tied to single node | High |
| No read replicas | All read/write load on single instance | Medium |
| Manual backups only | Risk of data loss | Medium |

### 1.5 Existing Protection Measures

- **PodDisruptionBudget**: `minAvailable: 1` (prevents voluntary eviction)
- **PVC**: Persists data across pod restarts (same node only)
- **Manual backup procedures**: Documented in OPERATIONS.md

---

## 2. High Availability Options

### Option 1: PostgreSQL Streaming Replication

Native PostgreSQL replication with manual failover or Keepalived.

#### Architecture

```
+-------------------+       +-------------------+
|  postgres-primary |------>|  postgres-replica |
|  (read-write)     | WAL   |  (read-only)      |
|  192.168.x.x      | stream|  192.168.x.x      |
+-------------------+       +-------------------+
         ^                           |
         |                           |
+--------+---------+     +-----------v--------+
|  pg-service      |     |  pg-readonly-svc   |
|  (write traffic) |     |  (read traffic)    |
+------------------+     +--------------------+
```

#### Implementation Requirements

1. **Primary PostgreSQL Pod**
   - Configure `wal_level = replica`
   - Configure `max_wal_senders = 3`
   - Create replication user
   - Configure `pg_hba.conf` for replication

2. **Replica PostgreSQL Pod**
   - Start with `pg_basebackup` from primary
   - Configure `primary_conninfo` in `standby.signal`
   - Read-only queries only

3. **Failover Mechanism**
   - Manual: Promote replica with `pg_ctl promote`
   - Semi-automatic: Keepalived with VIP

#### Pros

| Advantage | Description |
|-----------|-------------|
| Native PostgreSQL | No external dependencies |
| Well-documented | Extensive PostgreSQL documentation |
| Low resource overhead | Minimal additional resource usage |
| Read scaling | Replica can serve read queries |
| Simple architecture | Easy to understand and debug |

#### Cons

| Disadvantage | Description |
|--------------|-------------|
| Manual failover | Requires human intervention (unless scripted) |
| Split-brain risk | Possible with improper failover |
| No automatic leader election | Must implement separately |
| Configuration complexity | Requires careful pg_hba.conf setup |
| DNS/service updates | Manual or scripted service endpoint updates |

#### Resource Requirements

| Component | CPU | Memory | Storage |
|-----------|-----|--------|---------|
| Primary | 200m | 512Mi | 5Gi |
| Replica | 200m | 512Mi | 5Gi |
| Total additional | 100m | 256Mi | 5Gi |

#### Estimated Implementation Time

- Initial setup: 4-6 hours
- Testing: 2-4 hours
- Runbook creation: 2-3 hours
- **Total: 8-13 hours**

---

### Option 2: Patroni + etcd

Patroni is a template for PostgreSQL HA with automatic failover using distributed consensus.

#### Architecture

```
+-------------------+     +-------------------+     +-------------------+
|  etcd-0           |<--->|  etcd-1           |<--->|  etcd-2           |
|  (consensus)      |     |  (consensus)      |     |  (consensus)      |
+-------------------+     +-------------------+     +-------------------+
         ^                         ^                         ^
         |                         |                         |
         v                         v                         v
+-------------------+     +-------------------+     +-------------------+
|  postgres-0       |     |  postgres-1       |     |  postgres-2       |
|  + patroni        |<--->|  + patroni        |<--->|  + patroni        |
|  (leader)         |     |  (replica)        |     |  (replica)        |
+-------------------+     +-------------------+     +-------------------+
         |
         v
+-------------------+
|  pg-service       |
|  (auto-updates)   |
+-------------------+
```

#### Implementation Requirements

1. **etcd Cluster** (3 nodes for quorum)
   - Deploy as StatefulSet
   - Persistent storage per node
   - Cluster peer URLs configured

2. **Patroni Configuration**
   - Bootstrap configuration
   - PostgreSQL configuration
   - etcd endpoint configuration
   - Replica creation method

3. **Service Discovery**
   - Patroni REST API for health
   - Automatic endpoint updates
   - Optional: HAProxy for connection pooling

#### Pros

| Advantage | Description |
|-----------|-------------|
| Automatic failover | Leader election within seconds |
| Distributed consensus | No split-brain with proper quorum |
| Battle-tested | Used by GitLab, Zalando, others |
| Configuration management | Centralized via etcd |
| Automatic replica provisioning | New replicas bootstrap automatically |

#### Cons

| Disadvantage | Description |
|--------------|-------------|
| Resource overhead | etcd requires 3 nodes minimum |
| Complexity | Multiple components to manage |
| Memory footprint | etcd + Patroni sidecar per pod |
| Learning curve | Requires understanding Patroni + etcd |
| ARM64 compatibility | Need verified ARM64 images |

#### Resource Requirements

| Component | CPU | Memory | Storage | Instances |
|-----------|-----|--------|---------|-----------|
| PostgreSQL + Patroni | 250m | 600Mi | 5Gi | 3 |
| etcd | 100m | 256Mi | 1Gi | 3 |
| **Total** | **1050m** | **2.6Gi** | **18Gi** | 6 pods |

#### Estimated Implementation Time

- etcd cluster setup: 3-4 hours
- Patroni configuration: 4-6 hours
- Testing failover: 3-4 hours
- Migration: 2-3 hours
- Documentation: 2-3 hours
- **Total: 14-20 hours**

---

### Option 3: CloudNativePG Operator

Kubernetes-native PostgreSQL operator designed for cloud-native environments.

#### Architecture

```
+------------------------+
|  CloudNativePG         |
|  Operator              |
|  (watches CRDs)        |
+-----------+------------+
            |
            v
+------------------------+
|  Cluster CRD           |
|  (PostgreSQL cluster)  |
+-----------+------------+
            |
   +--------+--------+
   |        |        |
   v        v        v
+------+ +------+ +------+
| pg-1 | | pg-2 | | pg-3 |
|leader| |replica |replica|
+------+ +------+ +------+
   |        |        |
   v        v        v
+------+ +------+ +------+
| pvc-1| | pvc-2| | pvc-3|
+------+ +------+ +------+
```

#### Implementation Requirements

1. **Operator Installation**
   ```bash
   kubectl apply -f \
     https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.22/releases/cnpg-1.22.0.yaml
   ```

2. **Cluster Definition**
   ```yaml
   apiVersion: postgresql.cnpg.io/v1
   kind: Cluster
   metadata:
     name: postgres-cluster
     namespace: holm
   spec:
     instances: 3
     postgresql:
       parameters:
         shared_buffers: 128MB
         max_connections: 100
     storage:
       size: 5Gi
       storageClass: local-path
   ```

3. **Service Configuration**
   - Automatic `-rw` service (read-write, leader only)
   - Automatic `-r` service (read-only, all replicas)
   - Automatic `-ro` service (read-only, replicas only)

#### Pros

| Advantage | Description |
|-----------|-------------|
| Kubernetes-native | Uses CRDs and operators |
| Automatic failover | Built-in leader election |
| Declarative configuration | GitOps-friendly |
| Built-in backup/restore | Integrated with S3/MinIO |
| Point-in-time recovery | WAL archiving included |
| Connection pooling | Built-in PgBouncer option |
| Monitoring | Prometheus metrics included |
| Rolling updates | Zero-downtime upgrades |

#### Cons

| Disadvantage | Description |
|--------------|-------------|
| Operator dependency | Another component to maintain |
| Resource usage | Operator pod always running |
| Learning curve | New CRD concepts |
| ARM64 support | Verify operator ARM64 compatibility |
| Abstraction | Less direct control over PostgreSQL |

#### Resource Requirements

| Component | CPU | Memory | Storage | Instances |
|-----------|-----|--------|---------|-----------|
| Operator | 100m | 256Mi | - | 1 |
| PostgreSQL instance | 150m | 384Mi | 5Gi | 3 |
| **Total** | **550m** | **1.4Gi** | **15Gi** | 4 pods |

#### Estimated Implementation Time

- Operator installation: 1 hour
- Cluster configuration: 2-3 hours
- Testing failover: 2-3 hours
- Migration: 2-3 hours
- Backup configuration: 2-3 hours
- Documentation: 2 hours
- **Total: 11-15 hours**

---

## 3. Comparison Matrix

| Criteria | Streaming Replication | Patroni + etcd | CloudNativePG |
|----------|----------------------|----------------|---------------|
| **Automatic Failover** | No (manual) | Yes | Yes |
| **Resource Overhead** | Low (+256Mi) | High (+2.6Gi) | Medium (+1.4Gi) |
| **Complexity** | Medium | High | Low-Medium |
| **Kubernetes-Native** | No | Partial | Yes |
| **Built-in Backup** | No | No | Yes |
| **ARM64 Support** | Native | Requires verification | Yes |
| **Implementation Time** | 8-13 hrs | 14-20 hrs | 11-15 hrs |
| **Maintenance Burden** | Medium | High | Low |
| **Community Support** | PostgreSQL | Patroni/Zalando | CNCF Sandbox |
| **Monitoring** | Manual | Patroni REST | Prometheus built-in |

---

## 4. Recommendation

### Recommended Approach: CloudNativePG Operator

**Rationale:**

1. **Cluster Size and Resources**: The 13-node Raspberry Pi cluster has limited resources per node. CloudNativePG provides the best balance of HA features with moderate resource usage (~1.4Gi total vs ~2.6Gi for Patroni).

2. **Kubernetes-Native**: HolmOS is built entirely on Kubernetes. CloudNativePG uses CRDs and follows Kubernetes patterns, making it consistent with the existing architecture.

3. **Built-in Backup**: Integrated backup/restore eliminates the need for separate backup infrastructure, addressing the current manual backup limitation.

4. **Automatic Failover**: Critical for a self-hosted home system where manual intervention may not be immediately available.

5. **Lower Maintenance**: Operator handles rolling updates, failover, and replica management automatically.

6. **ARM64 Support**: CloudNativePG officially supports ARM64 architecture.

### Alternative Consideration

If resources are severely constrained, **PostgreSQL Streaming Replication** (Option 1) provides a simpler path with lower overhead. However, the lack of automatic failover makes it less suitable for a home cluster where response time to failures may be slow.

---

## 5. Migration Plan

### Phase 1: Preparation (Day 1)

#### 1.1 Install CloudNativePG Operator

```bash
# Install the operator
kubectl apply -f \
  https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.22/releases/cnpg-1.22.0.yaml

# Verify installation
kubectl get deployment -n cnpg-system cnpg-controller-manager
kubectl get pods -n cnpg-system
```

#### 1.2 Create Backup of Existing Data

```bash
# Get current postgres pod
POSTGRES_POD=$(kubectl get pods -n holm -l app=postgres -o jsonpath='{.items[0].metadata.name}')

# Full database dump
kubectl exec -n holm $POSTGRES_POD -- pg_dumpall -U postgres > pre_migration_backup_$(date +%Y%m%d_%H%M%S).sql

# Store backup safely
cp pre_migration_backup_*.sql /path/to/backup/location/
```

#### 1.3 Document Current Connection Strings

```bash
# Export all secrets referencing postgres
kubectl get secrets -n holm -o yaml | grep -A5 postgres > current_secrets.yaml
```

### Phase 2: Deploy New Cluster (Day 1-2)

#### 2.1 Create CloudNativePG Cluster Definition

```yaml
# File: k8s/postgres-ha/cluster.yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: postgres-ha
  namespace: holm
spec:
  instances: 3

  # PostgreSQL configuration
  postgresql:
    parameters:
      shared_buffers: "128MB"
      max_connections: "100"
      effective_cache_size: "256MB"
      maintenance_work_mem: "32MB"
      checkpoint_completion_target: "0.9"
      wal_buffers: "4MB"
      default_statistics_target: "100"
      random_page_cost: "1.1"
      effective_io_concurrency: "200"
      work_mem: "2MB"
      min_wal_size: "512MB"
      max_wal_size: "1GB"

  # Bootstrap from existing data
  bootstrap:
    recovery:
      source: postgres-backup

  # Storage configuration
  storage:
    size: 5Gi
    storageClass: local-path

  # Resource limits (per instance)
  resources:
    requests:
      memory: "256Mi"
      cpu: "100m"
    limits:
      memory: "512Mi"
      cpu: "500m"

  # Affinity to spread across nodes
  affinity:
    enablePodAntiAffinity: true
    topologyKey: kubernetes.io/hostname
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: kubernetes.io/hostname
            operator: NotIn
            values:
            - openmediavault

  # Monitoring
  monitoring:
    enablePodMonitor: true
```

#### 2.2 Apply Cluster Configuration

```bash
# Create the cluster (initially empty)
kubectl apply -f k8s/postgres-ha/cluster.yaml

# Wait for cluster to be ready
kubectl wait --for=condition=Ready cluster/postgres-ha -n holm --timeout=300s

# Check cluster status
kubectl get cluster postgres-ha -n holm
kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha
```

### Phase 3: Data Migration (Day 2)

#### 3.1 Restore Data to New Cluster

```bash
# Get the new primary pod
NEW_PRIMARY=$(kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha,role=primary -o jsonpath='{.items[0].metadata.name}')

# Copy backup to new primary
kubectl cp pre_migration_backup_*.sql holm/$NEW_PRIMARY:/tmp/backup.sql

# Restore data
kubectl exec -n holm $NEW_PRIMARY -- psql -U postgres -f /tmp/backup.sql

# Verify data
kubectl exec -n holm $NEW_PRIMARY -- psql -U postgres -c "\l"
kubectl exec -n holm $NEW_PRIMARY -- psql -U postgres -c "\du"
```

#### 3.2 Create Required Databases and Users

```bash
# Create databases (if not in backup)
kubectl exec -n holm $NEW_PRIMARY -- psql -U postgres -c "CREATE DATABASE backups;"
kubectl exec -n holm $NEW_PRIMARY -- psql -U postgres -c "CREATE DATABASE holm;"
kubectl exec -n holm $NEW_PRIMARY -- psql -U postgres -c "CREATE DATABASE gitea;"

# Verify
kubectl exec -n holm $NEW_PRIMARY -- psql -U postgres -c "\l"
```

### Phase 4: Service Migration (Day 2-3)

#### 4.1 Update Connection Secrets

CloudNativePG creates services automatically:
- `postgres-ha-rw` - Read-write (connects to primary)
- `postgres-ha-r` - Read (connects to any instance)
- `postgres-ha-ro` - Read-only (connects to replicas)

```yaml
# File: k8s/secrets/postgres-ha-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-ha-secret
  namespace: holm
type: Opaque
stringData:
  password: "SECURE_PASSWORD_HERE"
  database-url: "postgres://postgres:SECURE_PASSWORD_HERE@postgres-ha-rw.holm.svc.cluster.local:5432/holm?sslmode=disable"
  database-url-readonly: "postgres://postgres:SECURE_PASSWORD_HERE@postgres-ha-ro.holm.svc.cluster.local:5432/holm?sslmode=disable"
```

#### 4.2 Update Service Deployments

Update each dependent service to use the new connection:

```yaml
# Example: Update backup-storage deployment
env:
- name: DB_HOST
  value: "postgres-ha-rw.holm.svc.cluster.local"  # Changed from postgres.holm
```

Services to update:
1. backup-storage
2. auth-gateway
3. terminal-web
4. audiobook-web
5. settings-restore
6. gitea
7. notification-webhook
8. notification-queue
9. event-persist
10. event-replay
11. event-dlq
12. user-preferences

#### 4.3 Rolling Update Services

```bash
# Update deployments one by one
for service in backup-storage auth-gateway terminal-web audiobook-web settings-restore gitea notification-webhook notification-queue event-persist event-replay event-dlq user-preferences; do
  kubectl apply -f services/$service/deployment.yaml
  kubectl rollout status deployment/$service -n holm
  sleep 10  # Allow service to stabilize
done
```

### Phase 5: Validation (Day 3)

#### 5.1 Verify All Services

```bash
# Check all pods are running
kubectl get pods -n holm | grep -E "(postgres|backup|auth|terminal|audiobook|settings|gitea|notification|event|user)"

# Test health endpoints
for port in 30850 30100 30800 30700; do
  echo "Port $port: $(curl -s -o /dev/null -w '%{http_code}' http://192.168.8.197:$port/health)"
done
```

#### 5.2 Test Failover

```bash
# Identify current primary
kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha -o wide

# Force failover (delete primary pod)
kubectl delete pod -n holm $(kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha,role=primary -o jsonpath='{.items[0].metadata.name}')

# Watch failover
kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha -w

# Verify new primary elected
kubectl get cluster postgres-ha -n holm
```

#### 5.3 Verify Data Integrity

```bash
# Connect to new primary and verify data
kubectl exec -n holm -it $(kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha,role=primary -o jsonpath='{.items[0].metadata.name}') -- psql -U postgres -c "SELECT count(*) FROM pg_stat_user_tables;"
```

### Phase 6: Cleanup (Day 4)

#### 6.1 Remove Old PostgreSQL Deployment

```bash
# Scale down old postgres (keep PVC for rollback)
kubectl scale deployment postgres -n holm --replicas=0

# After 1 week of stable operation, remove completely
# kubectl delete deployment postgres -n holm
# kubectl delete service postgres -n holm
# kubectl delete pvc postgres-pvc -n holm  # WARNING: Deletes data
```

#### 6.2 Update Documentation

- Update ARCHITECTURE.md with new postgres-ha references
- Update OPERATIONS.md with new failover procedures
- Update service diagrams

---

## 6. Backup Strategy Integration

### 6.1 CloudNativePG Backup Configuration

```yaml
# File: k8s/postgres-ha/backup-schedule.yaml
apiVersion: postgresql.cnpg.io/v1
kind: ScheduledBackup
metadata:
  name: postgres-ha-daily-backup
  namespace: holm
spec:
  # Run daily at 2:00 AM
  schedule: "0 2 * * *"
  backupOwnerReference: self
  cluster:
    name: postgres-ha
  # Keep 7 daily backups
  target: prefer-standby
---
# For local storage (NFS/local-path)
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: postgres-ha
  namespace: holm
spec:
  # ... (previous config) ...

  backup:
    barmanObjectStore:
      destinationPath: "s3://postgres-backups/"
      endpointURL: "http://minio.holm.svc.cluster.local:9000"
      s3Credentials:
        accessKeyId:
          name: minio-credentials
          key: ACCESS_KEY_ID
        secretAccessKey:
          name: minio-credentials
          key: ACCESS_SECRET_KEY
    retentionPolicy: "7d"
```

### 6.2 Manual Backup Procedure

```bash
# Create on-demand backup
kubectl apply -f - <<EOF
apiVersion: postgresql.cnpg.io/v1
kind: Backup
metadata:
  name: postgres-ha-manual-$(date +%Y%m%d%H%M%S)
  namespace: holm
spec:
  cluster:
    name: postgres-ha
EOF

# Check backup status
kubectl get backups -n holm
```

### 6.3 Restore Procedure

```bash
# Point-in-time recovery
kubectl apply -f - <<EOF
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: postgres-ha-restored
  namespace: holm
spec:
  instances: 3
  bootstrap:
    recovery:
      source: postgres-ha
      recoveryTarget:
        targetTime: "2026-01-17T12:00:00Z"
  storage:
    size: 5Gi
EOF
```

### 6.4 Integration with Existing Backup System

The backup-storage service should be updated to:
1. Monitor CloudNativePG backup status via API
2. Display backup history in backup-dashboard
3. Trigger manual backups on demand
4. Store backup metadata in its database

---

## 7. Monitoring and Alerting

### 7.1 Built-in Metrics

CloudNativePG exposes Prometheus metrics automatically:

```bash
# View metrics endpoint
kubectl exec -n holm -it $(kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha,role=primary -o jsonpath='{.items[0].metadata.name}') -- curl -s localhost:9187/metrics | head -50
```

### 7.2 Key Metrics to Monitor

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `cnpg_pg_replication_lag` | Replication lag in bytes | > 10MB |
| `cnpg_pg_connections_total` | Active connections | > 80% of max |
| `cnpg_collector_up` | Collector health | != 1 |
| `cnpg_pg_database_size_bytes` | Database size | > 4Gi (80% of PVC) |

### 7.3 Pulse Integration

Update Pulse (port 30006) to scrape CloudNativePG metrics:

```yaml
# Add to Pulse configuration
scrape_configs:
  - job_name: 'postgres-ha'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names: ['holm']
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_cnpg_io_cluster]
        regex: postgres-ha
        action: keep
```

---

## 8. Rollback Plan

### If Migration Fails

1. **Immediate Rollback** (within migration window):
   ```bash
   # Scale up old postgres
   kubectl scale deployment postgres -n holm --replicas=1

   # Revert service connections
   # (Restore old deployment.yaml files from git)
   git checkout -- services/*/deployment.yaml
   kubectl apply -f services/*/deployment.yaml

   # Scale down new cluster
   kubectl scale cluster postgres-ha -n holm --replicas=0
   ```

2. **Post-Migration Rollback** (after cutover):
   ```bash
   # Export data from new cluster
   kubectl exec -n holm $NEW_PRIMARY -- pg_dumpall -U postgres > rollback_backup.sql

   # Scale up old postgres
   kubectl scale deployment postgres -n holm --replicas=1

   # Restore data
   kubectl cp rollback_backup.sql holm/$OLD_POD:/tmp/
   kubectl exec -n holm $OLD_POD -- psql -U postgres -f /tmp/rollback_backup.sql

   # Revert connections
   # ...
   ```

---

## 9. Timeline and Resources

### Implementation Schedule

| Phase | Duration | Resources Needed |
|-------|----------|------------------|
| Phase 1: Preparation | 2-3 hours | 1 engineer |
| Phase 2: Deploy Cluster | 3-4 hours | 1 engineer |
| Phase 3: Data Migration | 2-3 hours | 1 engineer |
| Phase 4: Service Migration | 3-4 hours | 1 engineer |
| Phase 5: Validation | 2-3 hours | 1 engineer |
| Phase 6: Cleanup | 1-2 hours | 1 engineer |
| **Total** | **13-19 hours** | Spread over 4 days |

### Cluster Resource Impact

| Metric | Before | After | Delta |
|--------|--------|-------|-------|
| PostgreSQL pods | 1 | 3 | +2 |
| Operator pods | 0 | 1 | +1 |
| Memory usage | 512Mi | 1.8Gi | +1.3Gi |
| CPU usage | 500m | 1.6 cores | +1.1 cores |
| Storage | 5Gi | 15Gi | +10Gi |

---

## 10. Success Criteria

| Criteria | Target | Measurement |
|----------|--------|-------------|
| Failover time | < 30 seconds | Timed test |
| Data loss on failover | 0 | WAL verification |
| Service availability | 99.9% | Uptime monitoring |
| Backup success rate | 100% | Backup logs |
| Replication lag | < 1 second | Metrics |
| All services connected | 12/12 | Health checks |

---

## Appendix A: CloudNativePG Commands Reference

```bash
# Cluster status
kubectl get cluster -n holm
kubectl describe cluster postgres-ha -n holm

# Pod status
kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha

# Identify primary
kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha,role=primary

# View logs
kubectl logs -n holm -l cnpg.io/cluster=postgres-ha -f

# Connect to PostgreSQL
kubectl exec -n holm -it $(kubectl get pods -n holm -l cnpg.io/cluster=postgres-ha,role=primary -o jsonpath='{.items[0].metadata.name}') -- psql -U postgres

# Force switchover (promote a specific replica)
kubectl annotate cluster postgres-ha -n holm cnpg.io/primarySwitchover=postgres-ha-2

# Scale cluster
kubectl patch cluster postgres-ha -n holm --type=merge -p '{"spec":{"instances":5}}'

# Create backup
kubectl apply -f backup.yaml

# List backups
kubectl get backups -n holm
```

---

## Appendix B: Troubleshooting

### Cluster Won't Start

```bash
# Check operator logs
kubectl logs -n cnpg-system -l app.kubernetes.io/name=cloudnative-pg

# Check cluster events
kubectl describe cluster postgres-ha -n holm

# Check PVC status
kubectl get pvc -n holm -l cnpg.io/cluster=postgres-ha
```

### Replication Lag

```bash
# Check replication status
kubectl exec -n holm -it $PRIMARY -- psql -U postgres -c "SELECT * FROM pg_stat_replication;"

# Check WAL position
kubectl exec -n holm -it $PRIMARY -- psql -U postgres -c "SELECT pg_current_wal_lsn();"
kubectl exec -n holm -it $REPLICA -- psql -U postgres -c "SELECT pg_last_wal_replay_lsn();"
```

### Connection Issues

```bash
# Verify service endpoints
kubectl get endpoints -n holm postgres-ha-rw postgres-ha-r postgres-ha-ro

# Test connectivity
kubectl run test-pg --rm -it --image=postgres:15-alpine --restart=Never -- \
  psql -h postgres-ha-rw.holm.svc.cluster.local -U postgres -c "SELECT 1;"
```

---

**Document Version**: 1.0
**Last Updated**: January 2026
**Next Review**: After implementation
