# HolmOS Operations Runbook

Operations guide for the HolmOS 13-node Raspberry Pi Kubernetes cluster.

## Quick Reference

| Resource | Value |
|----------|-------|
| Cluster IP | 192.168.8.197 |
| SSH User | rpi1 |
| Registry | 10.110.67.87:5000 |
| Namespace | holm |
| Node Count | 13 |
| PostgreSQL | postgres.holm.svc.cluster.local:5432 |

---

## 1. Common Operations

### 1.1 How to Deploy a New Service

**Step 1: Create the service directory structure**
```bash
mkdir -p services/my-service
cd services/my-service
```

**Step 2: Create the Dockerfile**
```dockerfile
FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production
COPY . .
EXPOSE 8080
CMD ["node", "server.js"]
```

**Step 3: Create the deployment.yaml**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  namespace: holm
  labels:
    app: my-service
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-service
  template:
    metadata:
      labels:
        app: my-service
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: kubernetes.io/hostname
                operator: NotIn
                values:
                - openmediavault
      containers:
      - name: my-service
        image: 10.110.67.87:5000/my-service:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "256Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: holm
spec:
  type: NodePort
  selector:
    app: my-service
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30XXX  # Choose an available port
```

**Step 4: Build and push the image**
```bash
# Build for ARM64 (Pi architecture)
docker buildx build --platform linux/arm64 \
  -t 10.110.67.87:5000/my-service:latest \
  --push .

# Or use make
make build S=my-service
```

**Step 5: Deploy to cluster**
```bash
kubectl apply -f deployment.yaml -n holm
```

### 1.2 How to Update an Existing Service

**Option A: Rolling update (recommended)**
```bash
# Update the image tag in deployment.yaml, then:
kubectl apply -f services/my-service/deployment.yaml

# Or force a re-pull of :latest
kubectl rollout restart deployment/my-service -n holm
```

**Option B: Using make**
```bash
make restart S=my-service
```

**Option C: Via GitHub Actions**
```bash
gh workflow run deploy.yml -f service=my-service
```

### 1.3 How to Check Service Health

**Check all pods in the holm namespace**
```bash
kubectl get pods -n holm
```

**Check a specific service**
```bash
kubectl get pods -n holm -l app=my-service
kubectl describe pod -n holm -l app=my-service
```

**Check deployment status**
```bash
kubectl rollout status deployment/my-service -n holm
```

**Check service endpoints**
```bash
kubectl get endpoints my-service -n holm
```

**Hit the health endpoint directly**
```bash
# From within the cluster
kubectl run curl --rm -it --image=curlimages/curl --restart=Never -- \
  curl -s http://my-service.holm.svc.cluster.local:8080/health

# From outside (using NodePort)
curl http://192.168.8.197:30XXX/health
```

**Using make**
```bash
make health
make status
```

### 1.4 How to View Logs

**View logs for a service**
```bash
kubectl logs -n holm -l app=my-service --tail=100

# Follow logs in real-time
kubectl logs -n holm -l app=my-service -f

# View logs from all containers
kubectl logs -n holm -l app=my-service --all-containers=true
```

**View logs from a specific pod**
```bash
kubectl logs -n holm my-service-abc123-xyz
```

**View previous container logs (after crash)**
```bash
kubectl logs -n holm -l app=my-service --previous
```

**Using make**
```bash
make logs S=my-service
```

**Using Scribe (log aggregation)**
Access at http://192.168.8.197:30860

---

## 2. Troubleshooting

### 2.1 Service Not Starting

**Symptoms**: Pod stays in `Pending` or `ContainerCreating` state

**Diagnostic steps**:
```bash
# Check pod events
kubectl describe pod -n holm -l app=my-service

# Check resource availability
kubectl top nodes
kubectl describe nodes | grep -A5 "Allocated resources"

# Check if the image exists
curl -s http://10.110.67.87:5000/v2/my-service/tags/list
```

**Common causes**:
1. **Insufficient resources**: Reduce resource requests in deployment.yaml
2. **Image not found**: Verify image is pushed to registry
3. **Node selector issues**: Check nodeAffinity rules
4. **PVC not bound**: Check storage class and PVC status

**Resolution**:
```bash
# If stuck on image pull
kubectl delete pod -n holm -l app=my-service

# If resource constrained
kubectl scale deployment/my-service -n holm --replicas=0
kubectl scale deployment/my-service -n holm --replicas=1
```

### 2.2 Pod in CrashLoopBackOff

**Symptoms**: Pod repeatedly restarts, status shows `CrashLoopBackOff`

**Diagnostic steps**:
```bash
# Check the exit code and reason
kubectl describe pod -n holm -l app=my-service | grep -A10 "State:"

# View crash logs
kubectl logs -n holm -l app=my-service --previous

# Check for OOMKilled
kubectl get pod -n holm -l app=my-service -o jsonpath='{.items[0].status.containerStatuses[0].lastState.terminated.reason}'
```

**Common causes**:
1. **Application error**: Check logs for stack traces
2. **OOMKilled**: Increase memory limits
3. **Liveness probe failing**: Increase `initialDelaySeconds`
4. **Missing config/secrets**: Check environment variables

**Resolution**:
```bash
# Increase memory if OOMKilled
kubectl patch deployment my-service -n holm --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/memory", "value": "512Mi"}]'

# Increase probe delays
kubectl patch deployment my-service -n holm --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/livenessProbe/initialDelaySeconds", "value": 30}]'
```

### 2.3 Image Pull Errors

**Symptoms**: `ErrImagePull` or `ImagePullBackOff` status

**Diagnostic steps**:
```bash
# Check pod events for details
kubectl describe pod -n holm -l app=my-service | grep -A5 "Events:"

# Verify image exists in registry
curl -s http://10.110.67.87:5000/v2/_catalog | jq
curl -s http://10.110.67.87:5000/v2/my-service/tags/list | jq

# Check image name in deployment
kubectl get deployment my-service -n holm -o jsonpath='{.spec.template.spec.containers[0].image}'
```

**Common causes**:
1. **Typo in image name**: Check deployment.yaml
2. **Image not pushed**: Build and push the image
3. **Registry unreachable**: Check registry pod status
4. **Wrong architecture**: Ensure image is built for linux/arm64

**Resolution**:
```bash
# Rebuild and push
docker buildx build --platform linux/arm64 \
  -t 10.110.67.87:5000/my-service:latest --push .

# Force pod recreation
kubectl delete pod -n holm -l app=my-service
```

### 2.4 Storage Issues

**Symptoms**: PVC stuck in `Pending`, or pod can't mount volume

**Diagnostic steps**:
```bash
# Check PVC status
kubectl get pvc -n holm
kubectl describe pvc my-pvc -n holm

# Check Longhorn status (55 pods)
kubectl get pods -n longhorn-system

# Check storage class
kubectl get storageclass
```

**Common causes**:
1. **Longhorn not ready**: Wait for Longhorn pods
2. **No available nodes**: Check node status
3. **Disk pressure**: Check node disk usage

**Resolution**:
```bash
# Check Longhorn volumes
kubectl get volumes.longhorn.io -n longhorn-system

# If stuck, delete and recreate PVC (WARNING: data loss)
kubectl delete pvc my-pvc -n holm
kubectl apply -f pvc.yaml -n holm
```

---

## 3. Rollback Procedures

### 3.1 How to Rollback a Deployment

**Check rollout history**
```bash
kubectl rollout history deployment/my-service -n holm
```

**Rollback to previous version**
```bash
kubectl rollout undo deployment/my-service -n holm
```

**Rollback to a specific revision**
```bash
# First, check available revisions
kubectl rollout history deployment/my-service -n holm

# Rollback to specific revision (e.g., revision 3)
kubectl rollout undo deployment/my-service -n holm --to-revision=3
```

**Verify rollback**
```bash
kubectl rollout status deployment/my-service -n holm
kubectl get pods -n holm -l app=my-service
```

### 3.2 Maximum Rollback Depth

Kubernetes keeps **10 revisions** by default (configurable via `revisionHistoryLimit`).

**Check current revision limit**
```bash
kubectl get deployment my-service -n holm -o jsonpath='{.spec.revisionHistoryLimit}'
```

**View all available revisions**
```bash
kubectl rollout history deployment/my-service -n holm
```

**To increase history limit** (edit deployment.yaml):
```yaml
spec:
  revisionHistoryLimit: 20  # Keep more revisions
```

### 3.3 Acceptable Error Rates and Rollback Triggers

**Automated rollback conditions** (manual intervention required):
- Pod restart count > 3 in 5 minutes
- Health check failures for > 2 minutes
- Error rate > 5% (monitor via Pulse at port 30006)

**Manual rollback decision matrix**:

| Condition | Action |
|-----------|--------|
| 100% pods failing | Immediate rollback |
| 50%+ error rate | Rollback within 5 minutes |
| Gradual degradation | Investigate, then rollback |
| Single pod crash | Let Kubernetes reschedule |

**Quick rollback command**
```bash
# Emergency rollback
kubectl rollout undo deployment/my-service -n holm && \
kubectl rollout status deployment/my-service -n holm
```

---

## 4. Backup & Recovery

### 4.1 Database Backup Procedures

**Manual PostgreSQL backup**
```bash
# Get the postgres pod
POSTGRES_POD=$(kubectl get pods -n holm -l app=postgres -o jsonpath='{.items[0].metadata.name}')

# Create backup
kubectl exec -n holm $POSTGRES_POD -- pg_dump -U postgres backups > backup_$(date +%Y%m%d_%H%M%S).sql

# For all databases
kubectl exec -n holm $POSTGRES_POD -- pg_dumpall -U postgres > full_backup_$(date +%Y%m%d_%H%M%S).sql
```

**Automated backup via backup-storage service**
```bash
# Trigger backup via API
curl -X POST http://192.168.8.197:30850/api/backup/create

# List existing backups
curl http://192.168.8.197:30850/api/backups
```

**Backup Longhorn volumes**
```bash
# Create snapshot via Longhorn UI or API
kubectl -n longhorn-system get volumes
```

### 4.2 How to Restore from Backup

**Restore PostgreSQL**
```bash
# Copy backup file to pod
kubectl cp backup.sql holm/$POSTGRES_POD:/tmp/backup.sql

# Restore
kubectl exec -n holm $POSTGRES_POD -- psql -U postgres -d backups -f /tmp/backup.sql
```

**Restore via backup-storage service**
```bash
# List available backups
curl http://192.168.8.197:30850/api/backups

# Restore specific backup
curl -X POST http://192.168.8.197:30850/api/backup/restore \
  -H "Content-Type: application/json" \
  -d '{"backup_id": "backup-20240115-120000"}'
```

**Restore Longhorn volume from snapshot**
1. Access Longhorn UI
2. Find the volume
3. Select snapshot
4. Click "Restore"

---

## 5. Monitoring

### 5.1 Health Check Endpoints

All services should implement these endpoints:

| Endpoint | Purpose | Expected Response |
|----------|---------|-------------------|
| `/health` | Liveness check | `200 OK` |
| `/ready` | Readiness check | `200 OK` |
| `/metrics` | Prometheus metrics | Metrics format |

**Test service health**
```bash
# Test all core services
for port in 30000 30001 30004 30006 30008; do
  echo "Port $port: $(curl -s -o /dev/null -w '%{http_code}' http://192.168.8.197:$port/health)"
done
```

**Health aggregation**
```bash
# Via health-aggregator service
curl http://health-aggregator.holm.svc.cluster.local/api/health/all
```

### 5.2 Key Metrics to Watch

**Cluster-level metrics**
```bash
# Node resource usage
kubectl top nodes

# Pod resource usage
kubectl top pods -n holm --sort-by=memory
```

**Key metrics via Pulse (port 30006)**
- CPU utilization per node (target: <80%)
- Memory usage per node (target: <85%)
- Pod restart count (alert if >3/hour)
- Request latency p99 (target: <500ms)
- Error rate per service (target: <1%)

**Longhorn storage metrics**
- Volume usage percentage
- Replica health status
- I/O latency

**Monitoring dashboards**
| Dashboard | Port | Purpose |
|-----------|------|---------|
| Nova | 30004 | Cluster overview |
| Pulse | 30006 | Metrics monitoring |
| Metrics Dashboard | 30950 | Detailed metrics |
| Test Dashboard | 30900 | Health status |

---

## 6. CI/CD Pipeline

### 6.1 How Kaniko Builds Work

Kaniko builds container images inside Kubernetes without requiring Docker daemon access.

**Kaniko build flow**:
1. Source code is loaded into a ConfigMap
2. Kaniko Job is created with the ConfigMap mounted
3. Kaniko builds the image and pushes to registry
4. Job completes and is cleaned up (TTL: 300s)

**Example kaniko-build.yaml**:
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kaniko-my-service
  namespace: holm
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: Never
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: kubernetes.io/hostname
                operator: NotIn
                values:
                - openmediavault
      containers:
      - name: kaniko
        image: gcr.io/kaniko-project/executor:latest
        args:
        - "--dockerfile=/workspace/Dockerfile"
        - "--context=/workspace"
        - "--destination=10.110.67.87:5000/my-service:latest"
        - "--insecure"
        - "--skip-tls-verify"
        volumeMounts:
        - name: source
          mountPath: /workspace
        resources:
          requests:
            memory: "1Gi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2"
      volumes:
      - name: source
        configMap:
          name: my-service-src
```

### 6.2 How to Trigger a Build

**Option A: In-cluster Kaniko build**
```bash
# Create ConfigMap with source
kubectl create configmap my-service-src \
  --from-file=services/my-service/ -n holm

# Run Kaniko job
kubectl apply -f services/my-service/kaniko-build.yaml

# Monitor build
kubectl logs -n holm -l job-name=kaniko-my-service -f
```

**Option B: Via CI/CD Controller**
```bash
# Trigger build via API
curl -X POST http://192.168.8.197:30020/api/build \
  -H "Content-Type: application/json" \
  -d '{"service": "my-service", "branch": "main"}'
```

**Option C: Via GitHub Actions**
```bash
gh workflow run deploy.yml -f service=my-service
```

**Option D: Local build and push**
```bash
# Build for ARM64
docker buildx build --platform linux/arm64 \
  -t 10.110.67.87:5000/my-service:latest \
  --push services/my-service/
```

### 6.3 Registry Usage

**Registry URL**: `10.110.67.87:5000`

**List all images**
```bash
curl -s http://10.110.67.87:5000/v2/_catalog | jq
```

**List tags for an image**
```bash
curl -s http://10.110.67.87:5000/v2/my-service/tags/list | jq
```

**Delete an image tag**
```bash
# Get the digest
DIGEST=$(curl -s -I -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
  http://10.110.67.87:5000/v2/my-service/manifests/old-tag | grep Docker-Content-Digest | awk '{print $2}' | tr -d '\r')

# Delete by digest
curl -X DELETE http://10.110.67.87:5000/v2/my-service/manifests/$DIGEST
```

**Registry UI**: http://192.168.8.197:31750

---

## Appendix: Quick Command Reference

```bash
# Status
kubectl get pods -n holm
kubectl get nodes
kubectl top nodes
kubectl top pods -n holm

# Logs
kubectl logs -n holm -l app=SERVICE -f
kubectl logs -n holm -l app=SERVICE --previous

# Deployment
kubectl apply -f deployment.yaml
kubectl rollout restart deployment/SERVICE -n holm
kubectl rollout status deployment/SERVICE -n holm

# Rollback
kubectl rollout undo deployment/SERVICE -n holm
kubectl rollout history deployment/SERVICE -n holm

# Debug
kubectl describe pod -n holm -l app=SERVICE
kubectl exec -it -n holm POD_NAME -- /bin/sh
kubectl get events -n holm --sort-by='.lastTimestamp'

# Scale
kubectl scale deployment/SERVICE -n holm --replicas=N

# Delete
kubectl delete pod -n holm -l app=SERVICE
kubectl delete deployment SERVICE -n holm
```

---

## Contact & Escalation

- **Nova Dashboard**: http://192.168.8.197:30004
- **Scribe Logs**: http://192.168.8.197:30860
- **Pulse Metrics**: http://192.168.8.197:30006
- **SSH Access**: `ssh rpi1@192.168.8.197`

## Remote Command Execution

Run cluster commands remotely via cmd.holm.chat:
```bash
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "sshpass -p YOUR_PASSWORD ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 kubectl get pods -n holm"}'
```
