# HolmOS Deployment Guide

## Overview

HolmOS runs on a 13-node Raspberry Pi Kubernetes cluster using K3s. Services are deployed as containers from a private registry.

## Cluster Access

### SSH Access

```bash
# Using Makefile (requires PI_PASS env var)
export PI_PASS="your_password"
make ssh

# Direct SSH
ssh rpi1@192.168.8.197
```

### Tailscale VPN (Recommended for Remote Access)

```bash
# Install on cluster
kubectl apply -f services/tailscale/deployment.yaml

# Or install on nodes directly
curl -fsSL https://raw.githubusercontent.com/timholm/HolmOS/main/scripts/setup-tailscale.sh | sudo bash
sudo tailscale up --accept-routes
```

See [services/tailscale/README.md](../services/tailscale/README.md) for full setup.

## Container Registry

**Registry Address:** `10.110.67.87:5000`

### List Images

```bash
make images
# or
curl -s http://10.110.67.87:5000/v2/_catalog | jq
```

### Push Image

```bash
docker tag my-service:latest 10.110.67.87:5000/my-service:latest
docker push 10.110.67.87:5000/my-service:latest
```

### Pull Image

```bash
docker pull 10.110.67.87:5000/my-service:latest
```

## Deployment Methods

### Method 1: GitHub Actions (Recommended)

Automatic deployment when changes are pushed to `services/`:

```bash
# Trigger manually
make deploy S=my-service

# Or via gh CLI
gh workflow run build-deploy.yml -f service=my-service
```

### Method 2: Manual Deployment

1. Build for ARM64:
```bash
cd services/my-service
docker buildx build --platform linux/arm64 -t 10.110.67.87:5000/my-service:latest .
docker push 10.110.67.87:5000/my-service:latest
```

2. Deploy to cluster:
```bash
make ssh
kubectl apply -f /path/to/deployment.yaml
```

### Method 3: kubectl Apply

```bash
# From local machine with kubeconfig
kubectl apply -f services/my-service/deployment.yaml

# Or SSH and apply
make ssh
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
...
EOF
```

## Deployment Configuration

### Standard Deployment Template

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
      containers:
        - name: my-service
          image: 10.110.67.87:5000/my-service:latest
          imagePullPolicy: Always
          ports:
            - containerPort: 3000
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
              port: 3000
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /health
              port: 3000
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
  ports:
    - port: 3000
      targetPort: 3000
      nodePort: 30XXX
  selector:
    app: my-service
```

### With Persistent Storage

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-service-data
  namespace: holm
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: longhorn
  resources:
    requests:
      storage: 1Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  namespace: holm
spec:
  # ...
  template:
    spec:
      containers:
        - name: my-service
          # ...
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: my-service-data
```

### With Environment Variables

```yaml
spec:
  containers:
    - name: my-service
      env:
        - name: DATABASE_URL
          value: "postgres://postgres.holm.svc.cluster.local/holmos"
        - name: SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: my-service-secrets
              key: secret-key
```

## Managing Deployments

### View Status

```bash
make status
make pods
make health
```

### View Logs

```bash
make logs S=my-service
```

### Restart Service

```bash
make restart S=my-service
```

### Scale Service

```bash
make ssh
kubectl scale deployment/my-service -n holm --replicas=3
```

### Delete Service

```bash
make ssh
kubectl delete deployment my-service -n holm
kubectl delete service my-service -n holm
```

## Rolling Updates

Kubernetes performs rolling updates by default:

```bash
# Update image
kubectl set image deployment/my-service my-service=10.110.67.87:5000/my-service:v2 -n holm

# Check rollout status
kubectl rollout status deployment/my-service -n holm

# Rollback if needed
kubectl rollout undo deployment/my-service -n holm
```

## Node Management

### List Nodes

```bash
make nodes
```

### Cordon Node (Prevent Scheduling)

```bash
make ssh
kubectl cordon rpi3
```

### Drain Node (Evict Pods)

```bash
make ssh
kubectl drain rpi3 --ignore-daemonsets --delete-emptydir-data
```

### Uncordon Node

```bash
make ssh
kubectl uncordon rpi3
```

## Troubleshooting

### Pod Not Starting

```bash
# Check pod status
kubectl describe pod -n holm -l app=my-service

# Check events
kubectl get events -n holm --sort-by='.lastTimestamp'

# Check logs
kubectl logs -n holm -l app=my-service --previous
```

### Image Pull Errors

```bash
# Verify image exists
curl -s http://10.110.67.87:5000/v2/my-service/tags/list

# Check node can pull
make ssh
crictl pull 10.110.67.87:5000/my-service:latest
```

### Resource Issues

```bash
# Check resource usage
make top

# Check node capacity
kubectl describe nodes | grep -A 5 "Allocated resources"
```

### Network Issues

```bash
# Test service DNS
kubectl run test --rm -it --image=busybox -- nslookup my-service.holm.svc.cluster.local

# Test connectivity
kubectl run test --rm -it --image=busybox -- wget -qO- http://my-service.holm.svc.cluster.local:3000/health
```

## Backup and Recovery

### Database Backup

```bash
# Via backup-dashboard (30850)
# Or manually:
make ssh
kubectl exec -n holm postgres-0 -- pg_dump holmos > backup.sql
```

### Service Config Backup

```bash
make sync  # Syncs cluster configs to local
git add -A && git commit -m "Backup configs" && git push
```

## Production Checklist

Before deploying to production:

- [ ] Health endpoint implemented (`/health`)
- [ ] Resource limits defined
- [ ] Liveness and readiness probes configured
- [ ] Logs going to stdout/stderr
- [ ] No hardcoded secrets (use ConfigMaps/Secrets)
- [ ] Service registered in `services.yaml`
- [ ] Tests passing (`make smoke`)
- [ ] Documentation updated
