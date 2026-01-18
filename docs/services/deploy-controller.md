# Deploy Controller

## Purpose

The Deploy Controller manages Kubernetes deployments across the HolmOS cluster. It handles automatic deployments triggered by container registry pushes, processes webhooks from build systems, tracks deployment history, performs health checks with automatic rollback, and provides a unified interface for deployment operations.

## How It Works

### Deployment Management
- Syncs with Kubernetes to track all deployments in monitored namespaces
- Maintains deployment state including replicas, readiness, and current images
- Provides a web UI for viewing and managing deployments

### Auto-Deploy Rules
- Configurable rules that match container image patterns to deployments
- When a new image is pushed to the registry, matching rules trigger automatic deployments
- Supports tag pattern matching (wildcards like `v*` or specific tags like `latest`)
- Can auto-create deployments if they don't exist (`AutoCreate` option)
- Rules are persisted in a ConfigMap for durability

### Health Checking
- Monitors deployment rollouts after updates
- Tracks pod status, readiness probes, and rollout conditions
- Detects stalled rollouts and progress deadline failures
- Automatic rollback on deployment failures (configurable per rule)

### Rollback Support
- Maintains version history for each deployment
- Supports manual rollback to any previous version
- Auto-rollback with configurable timeout and retry limits

### Deployment Metrics
- Tracks total deploys, success/failure rates, rollback counts
- Calculates average deployment time and Mean Time To Recovery (MTTR)
- Per-deployment statistics for monitoring deployment health

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `REGISTRY_URL` | `192.168.8.197:30500` | Container registry URL |
| `FORGE_URL` | `http://forge.holm.svc.cluster.local` | Forge service URL |
| `HOLMGIT_URL` | `http://holm-git.holm.svc.cluster.local` | HolmGit service URL |
| `RULES_CONFIGMAP` | `deploy-controller-rules` | ConfigMap for auto-deploy rules |

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/deployments` | GET | List all deployments |
| `/api/deploy` | POST | Trigger deployment |
| `/api/rollback` | POST | Rollback deployment |
| `/api/events` | GET | List recent deploy events |
| `/api/webhook` | POST | Generic webhook handler |
| `/api/webhook/git` | POST | Git webhook handler |
| `/api/webhook/build` | POST | Build system webhook |
| `/api/webhook/registry` | POST | Registry push webhook |
| `/api/autodeploy` | GET/POST/PUT/DELETE | Manage auto-deploy rules |
| `/api/images` | GET | List registry images |
| `/api/history` | GET | Deployment history |
| `/api/history/{deployment}` | GET | History for specific deployment |
| `/api/registry-events` | GET | Registry push events |
| `/api/health-checks` | GET | Active health checks |
| `/api/scale` | POST | Scale deployment replicas |
| `/api/logs` | GET | Pod logs |
| `/api/restart` | POST | Restart deployment |
| `/api/apply` | POST | Apply Kubernetes manifest |
| `/api/k8s-events` | GET | Kubernetes events |
| `/api/metrics` | GET | Deployment metrics |
| `/api/events/stream` | GET | SSE event stream |

### Auto-Deploy Rule Configuration

```json
{
  "imagePattern": "my-app",
  "deployment": "my-app",
  "namespace": "holm",
  "enabled": true,
  "autoCreate": true,
  "tagPattern": "v*",
  "healthCheckPath": "/health",
  "healthCheckPort": 8080,
  "servicePort": 80,
  "createService": true,
  "autoRollback": true,
  "rollbackTimeout": 300,
  "maxRollbackRetries": 3
}
```

## Dependencies

### Internal Services
- **CI/CD Controller**: Sends build completion webhooks
- **Container Registry**: Sends push notifications for new images
- **HolmGit**: Source for deployment manifests

### Kubernetes Resources
- Requires in-cluster configuration for managing Deployments and Services
- Uses `apps/v1` Deployments and `core/v1` Services
- Needs RBAC permissions to create/update/delete Deployments, Services, and read Pods/Events
- Stores rules in a ConfigMap for persistence

### External Dependencies
- Docker Registry API v2 for image discovery and digest retrieval
- Network access to container registry
