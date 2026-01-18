# CI/CD Controller

## Purpose

The CI/CD Controller is a comprehensive continuous integration and continuous deployment service that manages build pipelines, handles webhooks from Git repositories, executes container builds using Kaniko, and coordinates deployments across the HolmOS cluster. It provides a complete GitOps workflow from code push to deployment.

## How It Works

### Pipeline Management
- Supports multi-stage pipelines with configurable stages (build, test, deploy, custom)
- Each stage can specify a container image, commands, environment variables, and timeout
- Stages can have dependencies on other stages and conditional execution (always, on_success, on_failure)
- Pipelines can be triggered via webhooks, schedules, or manually

### Build Queue
- Priority-based queue system with four levels: low (1), normal (2), high (3), critical (4)
- Limits concurrent builds via `MAX_CONCURRENT_BUILDS` (default: 3)
- Queue can be paused/resumed for maintenance
- Estimates completion time based on historical build duration data

### Webhook Processing
- Validates webhook signatures using HMAC-SHA256
- Supports multiple Git platforms: GitHub, GitLab, HolmGit, Bitbucket
- Processes push events, pull requests, tags, and releases
- Matches webhooks to pipelines based on repository and branch patterns

### Container Builds
- Uses Kaniko executor to build container images without Docker daemon
- Pushes built images to the configured container registry
- Supports build arguments and custom Dockerfiles

### Real-time Events
- Server-Sent Events (SSE) for live build status updates
- Streaming build logs via `/api/logs-stream/`
- Build statistics and trend tracking

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `REGISTRY_URL` | `10.110.67.87:5000` | Container registry URL |
| `HOLMGIT_URL` | `http://holm-git.holm.svc.cluster.local` | HolmGit service URL |
| `WEBHOOK_SECRET` | - | Secret for validating webhook signatures |
| `MAX_CONCURRENT_BUILDS` | `3` | Maximum concurrent build jobs |
| `MAX_QUEUE_SIZE` | `100` | Maximum builds in queue |
| `EXECUTION_HISTORY_SIZE` | `500` | Number of executions to retain |

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/pipelines` | GET/POST | List or create pipelines |
| `/api/pipelines/{id}` | GET/PUT/DELETE | Manage specific pipeline |
| `/api/webhook/git` | POST | Generic Git webhook |
| `/api/webhook/github` | POST | GitHub webhook |
| `/api/webhook/gitlab` | POST | GitLab webhook |
| `/api/webhook/holmgit` | POST | HolmGit webhook |
| `/api/queue` | GET | List build queue |
| `/api/queue/reorder` | POST | Reorder queue items |
| `/api/queue/pause` | POST | Pause queue processing |
| `/api/queue/resume` | POST | Resume queue processing |
| `/api/executions` | GET | List pipeline executions |
| `/api/executions/{id}` | GET | Get execution details |
| `/api/stats` | GET | Build statistics |
| `/api/events` | GET | SSE event stream |
| `/api/logs/{id}` | GET | Build logs |
| `/api/build` | POST | Trigger Kaniko build |
| `/api/builds` | GET | List builds |

## Dependencies

### Internal Services
- **HolmGit**: Source code repository service for fetching code
- **Container Registry**: Stores built container images
- **Deploy Controller**: Receives deployment triggers after successful builds

### Kubernetes Resources
- Requires in-cluster configuration for creating Kaniko build Jobs
- Uses `batch/v1` Jobs for container builds
- Needs RBAC permissions to create/manage Jobs in the cluster

### External Dependencies
- Kaniko executor image (`gcr.io/kaniko-project/executor:latest`)
- Network access to Git repositories and container registry
