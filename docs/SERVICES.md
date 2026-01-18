# HolmOS Services Catalog

> **🤖 CLAUDE/AI AGENTS:** To access the cluster, use `https://cmd.holm.chat/run` - See [CLAUDE.md](../CLAUDE.md). Direct SSH/kubectl is blocked.

> **Version:** 3.0
> **Last Updated:** January 2026
> **Total Services:** 58
> **Maintainers:** Steve (Visionary Architect), Alice (Code Explorer)

---

## Overview

HolmOS is a comprehensive Kubernetes-native operating system running on a 13-node Raspberry Pi cluster. This document provides a complete catalog of all 58 services deployed in the HolmOS ecosystem.

**Cluster Details:**
- **Nodes:** 12 Raspberry Pi nodes (rpi-1 through rpi-12) + 1 storage node (openmediavault)
- **Namespace:** `holm` (services), `holm-system` (infrastructure)
- **Cluster IP:** 192.168.8.197
- **Theme:** Catppuccin Mocha

---

## Table of Contents

1. [Complete Service Inventory](#complete-service-inventory)
2. [Core Infrastructure](#core-infrastructure)
3. [Web Applications](#web-applications)
4. [AI/Bots](#aibots)
5. [File Services](#file-services)
6. [Development Tools](#development-tools)
7. [Backup & Storage](#backup--storage)
8. [Monitoring & Metrics](#monitoring--metrics)
9. [Notification Services](#notification-services)
10. [User & API Services](#user--api-services)
11. [Audiobook Services](#audiobook-services)
12. [Deployment Commands](#deployment-commands)
13. [Architecture Notes](#architecture-notes)

---

## Complete Service Inventory

### All 58 Services at a Glance

| # | Service | Port | NodePort | Category | Status | Description |
|---|---------|------|----------|----------|--------|-------------|
| 1 | gateway | 8080 | 30008 | Infrastructure | Active | API gateway with routing & rate limiting |
| 2 | auth-gateway | 8080 | 30100 | Infrastructure | Active | JWT authentication & user management |
| 3 | vault | 8080 | 30870 | Infrastructure | Active | Encrypted secrets management (AES-256-GCM) |
| 4 | config-sync | 8080 | ClusterIP | Infrastructure | Active | Configuration synchronization |
| 5 | health-aggregator | 8080 | ClusterIP | Infrastructure | Active | Multi-service health aggregation |
| 6 | event-broker | 8080, 4222 | ClusterIP | Infrastructure | Active | Event messaging broker (NATS) |
| 7 | event-persist | 8080 | ClusterIP | Infrastructure | Active | Event persistence to database |
| 8 | event-dlq | 8080 | ClusterIP | Infrastructure | Active | Dead letter queue for events |
| 9 | event-replay | 8080 | ClusterIP | Infrastructure | Active | Event replay service |
| 10 | cluster-manager | 8080 | 30502 | Infrastructure | Active | Kubernetes cluster management |
| 11 | pxe-server | TFTP:69 | Host | Infrastructure | Active | PXE boot server for network installs |
| 12 | holmos-shell | 8080 | 30000 | Web App | Active | Main HolmOS web shell interface |
| 13 | ios-shell | 8080 | 30001 | Web App | Active | iOS SpringBoard-style launcher |
| 14 | terminal-web | 8080 | 30800 | Web App | Active | Web-based SSH/kubectl terminal |
| 15 | settings-web | 8080 | 30600 | Web App | Active | System settings interface |
| 16 | settings-restore | 8080 | ClusterIP | Web App | Active | Settings restore service |
| 17 | calculator-app | 8080 | 30010 | Web App | Active | Scientific calculator |
| 18 | clock-app | 30007 | 30011 | Web App | Active | World clock & timer |
| 19 | app-store-ai | 8080 | 30002 | Web App | Active | AI-powered app store |
| 20 | chat-hub | 8080 | 30003 | Web App | Active | Real-time AI chat hub |
| 21 | steve-bot | 8080 | 30099 | AI/Bot | Active | Visionary architect (deepseek-r1:7b) |
| 22 | alice-bot | 8080 | 30668 | AI/Bot | Active | Code explorer (gemma3) |
| 23 | nova | 80 | 30004 | AI/Bot | Active | AI assistant service |
| 24 | claude-pod | 8080 | 30013 | AI/Bot | Active | Claude AI integration |
| 25 | ollama-server | 11434 | External | AI/Bot | Active | Local LLM server (GPU) |
| 26 | file-web-nautilus | 8080 | 30088 | Files | Active | Nautilus-style file browser |
| 27 | file-copy | 8080 | ClusterIP | Files | Active | File copy operations |
| 28 | file-delete | 8080 | ClusterIP | Files | Active | File deletion |
| 29 | file-download | 8080 | ClusterIP | Files | Active | File download service |
| 30 | file-upload | 8080 | ClusterIP | Files | Active | File upload service |
| 31 | file-move | 8080 | ClusterIP | Files | Active | File move/rename |
| 32 | file-mkdir | 8080 | ClusterIP | Files | Active | Directory creation |
| 33 | file-meta | 8080 | ClusterIP | Files | Active | File metadata |
| 34 | file-search | 8080 | ClusterIP | Files | Active | File search |
| 35 | file-thumbnail | 8080 | ClusterIP | Files | Active | Thumbnail generation |
| 36 | file-compress | 8080 | ClusterIP | Files | Active | File compression |
| 37 | file-decompress | 8080 | ClusterIP | Files | Active | File decompression |
| 38 | file-convert | 8080 | ClusterIP | Files | Active | File format conversion |
| 39 | file-encrypt | 8080 | ClusterIP | Files | Active | File encryption |
| 40 | file-permissions | 8080 | ClusterIP | Files | Active | Permissions management |
| 41 | file-preview | 8080 | ClusterIP | Files | Active | File preview generation |
| 42 | file-watch | 8080 | ClusterIP | Files | Active | File system watcher |
| 43 | file-share-create | 8080 | ClusterIP | Files | Active | Share link creation |
| 44 | file-share-validate | 8080 | ClusterIP | Files | Active | Share token validation |
| 45 | holm-git | 8080 | 30009 | DevOps | Active | HolmOS Git server |
| 46 | gitea | 3000, 22 | 30009, 30022 | DevOps | Active | Full-featured Git server |
| 47 | cicd-controller | 8080 | 30020 | DevOps | Active | CI/CD pipeline controller |
| 48 | deploy-controller | 8080 | 30015 | DevOps | Active | Deployment automation |
| 49 | registry-ui | 8080 | 31750 | DevOps | Active | Container registry UI |
| 50 | test-dashboard | 8080 | 30018 | DevOps | Active | Test results dashboard |
| 51 | holm-cli | 8080 | ClusterIP | DevOps | Active | HolmOS CLI service |
| 52 | scribe | 8080 | 30017 | DevOps | Active | Log aggregation & viewer |
| 53 | backup-service | 8080 | 30015 | Backup | Active | Backup orchestration |
| 54 | backup-dashboard | 8080 | 30012 | Backup | Active | Backup management UI |
| 55 | backup-storage | 8080 | ClusterIP | Backup | Active | Backup storage backend |
| 56 | metrics-dashboard | 8080 | 30950 | Monitoring | Active | Metrics visualization |
| 57 | metrics-collector | 8080 | ClusterIP | Monitoring | Active | Metrics collection agent |
| 58 | pulse | 8080 | 30006 | Monitoring | Active | System health monitor |

---

## Core Infrastructure

Essential services that power the HolmOS platform.

| Service | Purpose | Port | NodePort | Dependencies |
|---------|---------|------|----------|--------------|
| gateway | API gateway with rate limiting and routing | 8080 | 30008 | None |
| auth-gateway | Authentication and authorization gateway | 8080 | 30100 | PostgreSQL, JWT secrets |
| vault | Secrets management and storage | 8080 | 30870 | PVC storage |
| config-sync | Configuration synchronization across cluster | 8080 | ClusterIP | ConfigMap |
| health-aggregator | Aggregates health status from all services | 8080 | ClusterIP | None |
| event-broker | Event messaging broker (NATS-based) | 8080, 4222 | ClusterIP | None |
| event-persist | Persists events to database | 8080 | ClusterIP | PostgreSQL, event-broker |
| event-dlq | Dead letter queue for failed events | 8080 | ClusterIP | PostgreSQL, event-broker |
| event-replay | Event replay service | 8080 | ClusterIP | PostgreSQL, event-broker |
| cluster-manager | Kubernetes cluster management UI | 8080 | 30502 | Kubeconfig |
| pxe-server | PXE boot server for network installations | TFTP:69 | Host Network | dnsmasq, TFTP |

### Gateway

**Purpose:** API gateway providing routing, load balancing, rate limiting, and WebSocket proxying.

**Technical Details:**
- **Language:** Go
- **Port:** 8080 (NodePort: 30008)
- **Replicas:** 2 (HA)
- **Features:** Dynamic routing, per-client rate limiting (1000/min), WebSocket support

**API Endpoints:**
```
GET  /api/services          List registered services
GET  /api/routes            List routing rules
GET  /api/metrics           Gateway metrics
GET  /health                Health check
GET  /ready                 Readiness check
```

### Auth Gateway

**Purpose:** Central authentication service providing JWT-based auth and user management.

**Technical Details:**
- **Language:** Go
- **Port:** 8080 (NodePort: 30100)
- **Database:** PostgreSQL
- **Security:** bcrypt password hashing, JWT tokens

**API Endpoints:**
```
POST /api/login             API login (returns JWT)
POST /api/register          User registration
POST /api/validate          Token validation
POST /api/refresh           Token refresh
GET  /api/sessions          Active sessions
GET  /api/users             User list (admin only)
GET  /health                Health check
```

---

## Web Applications

User-facing web applications and shells.

| Service | Purpose | Port | NodePort | Dependencies |
|---------|---------|------|----------|--------------|
| holmos-shell | Main HolmOS web shell interface | 8080 | 30000 | None |
| ios-shell | iOS-optimized SpringBoard interface | 8080 | 30001 | None |
| terminal-web | Web-based terminal with cluster admin access | 8080 | 30800 | PostgreSQL (optional) |
| settings-web | System settings web interface | 8080 | 30600 | None |
| settings-restore | Settings restore service | 8080 | ClusterIP | PostgreSQL |
| calculator-app | Web calculator application | 8080 | 30010 | None |
| clock-app | Web clock/time application | 30007 | 30011 | None |
| app-store-ai | AI-powered app store interface | 8080 | 30002 | None |
| chat-hub | Real-time AI chat hub | 8080 | 30003 | steve-bot, alice-bot |

### Terminal Web

**Purpose:** Full-featured web-based terminal with SSH, kubectl, and local shell access.

**Technical Details:**
- **Language:** Go
- **Port:** 8080 (NodePort: 30800)
- **Features:** PTY support, SSH client, xterm.js, session management

**WebSocket Endpoints:**
```
WS   /ws/ssh                SSH WebSocket
WS   /ws/kubectl            kubectl exec WebSocket
WS   /ws/local              Local shell WebSocket
WS   /ws/pod                Pod exec WebSocket
```

---

## AI/Bots

AI-powered bots and agents running on local LLMs.

| Service | Purpose | Port | NodePort | Dependencies |
|---------|---------|------|----------|--------------|
| steve-bot | AI visionary bot using deepseek-r1:7b | 8080 | 30099 | Ollama, alice-bot |
| alice-bot | AI explorer bot using gemma3 | 8080 | 30668 | Ollama, steve-bot, HolmOS repo |
| nova | AI assistant service | 80 | 30004 | None |
| claude-pod | Claude AI integration pod | 8080 | 30013 | None |
| ollama-server | Local LLM server with GPU acceleration | 11434 | External | NVIDIA GPU (RTX 2070) |

### Steve Bot - The Visionary Kubernetes Architect

**Purpose:** AI-powered cluster analyst with Steve Jobs persona, providing continuous improvement recommendations.

**Technical Details:**
- **Language:** Python (Flask + async)
- **Port:** 8080 (NodePort: 30099)
- **AI Model:** deepseek-r1:7b via Ollama
- **Database:** SQLite for conversations

**Personality:**
- Perfectionist demanding excellence
- Brutally honest about infrastructure decisions
- Zero tolerance for mediocrity
- Passionate about user experience

**API Endpoints:**
```
GET  /health                Health check
GET  /api/status            Bot status & model info
POST /api/analyze           Trigger cluster analysis
POST /api/chat              Send message to Steve
POST /api/respond           Alice-to-Steve messaging
GET  /api/conversations     Conversation history
WS   /ws                    Real-time updates
```

### Alice Bot - The Curious Code Explorer

**Purpose:** AI-powered codebase explorer with Alice in Wonderland persona.

**Technical Details:**
- **Language:** Python (Flask + async)
- **Port:** 8080 (NodePort: 30668)
- **AI Model:** gemma3 via Ollama
- **Init Container:** Clones HolmOS repository

**API Endpoints:**
```
GET  /health                Health check
GET  /api/status            Bot status
POST /api/explore           Trigger codebase exploration
POST /api/chat              Send message to Alice
GET  /api/report            Full codebase report
GET  /api/discoveries       Code discoveries
WS   /ws                    Real-time updates
```

### Ollama Server

**Purpose:** Local LLM server with NVIDIA GPU acceleration.

**Deployment Details:**
- **Host:** lenovo (192.168.8.230)
- **GPU:** NVIDIA RTX 2070 Mobile (8GB VRAM)
- **OS:** Debian 12 (Bookworm)
- **API:** http://192.168.8.230:11434

**Available Models:**
| Model | Size | Use Case |
|-------|------|----------|
| qwen2.5-coder:7b | 4.7GB | Code generation |
| llama3.2:3b | 2.0GB | General chat |
| deepseek-r1:7b | 4.7GB | Reasoning & analysis |
| gemma3 | 5GB | Code exploration |

---

## File Services

Distributed file management microservices.

### Core File Operations

| Service | Purpose | Port | Dependencies |
|---------|---------|------|--------------|
| file-web-nautilus | Web file browser (Nautilus-like) | 30088 | Longhorn PVC (500Gi) |
| file-copy | File copy operations | ClusterIP | None |
| file-delete | File deletion operations | ClusterIP | None |
| file-download | File download service | ClusterIP | None |
| file-upload | File upload service | ClusterIP | None |
| file-move | File move/rename operations | ClusterIP | None |
| file-mkdir | Directory creation service | ClusterIP | None |
| file-meta | File metadata service | ClusterIP | None |
| file-search | File search service | ClusterIP | None |
| file-thumbnail | Thumbnail generation | ClusterIP | PVC storage |

### Advanced File Operations

| Service | Purpose | Port | Dependencies |
|---------|---------|------|--------------|
| file-compress | File compression (zip, tar, etc.) | ClusterIP | PVC storage |
| file-decompress | File decompression | ClusterIP | PVC storage |
| file-convert | File format conversion | ClusterIP | PVC storage |
| file-encrypt | File encryption service | ClusterIP | None |
| file-permissions | File permissions management | ClusterIP | PVC storage |
| file-preview | File preview generation | ClusterIP | PVC storage |
| file-watch | File system watcher | ClusterIP | PVC storage |
| file-share-create | Create file sharing links | ClusterIP | PVC storage |
| file-share-validate | Validate file sharing tokens | ClusterIP | PVC storage |

---

## Development Tools

CI/CD, version control, and development services.

| Service | Purpose | Port | NodePort | Dependencies |
|---------|---------|------|----------|--------------|
| holm-git | HolmOS Git server | 8080 | 30009 | PVC (20Gi) |
| gitea | Full-featured Git server (Gitea) | 3000, 22 | 30009, 30022 | PostgreSQL, PVC (10Gi) |
| cicd-controller | CI/CD pipeline controller | 8080 | 30020 | holm-git, Registry |
| deploy-controller | Deployment automation controller | 8080 | 30015 | holm-git, Registry |
| registry-ui | Container registry web UI | 8080 | 31750 | Registry |
| test-dashboard | Test results dashboard | 8080 | 30018 | None |
| holm-cli | HolmOS CLI service | 8080 | ClusterIP | None |
| scribe | Log aggregation and viewer | 8080 | 30017 | Kubernetes API |

### CI/CD Controller

**Purpose:** CI/CD pipeline management with Kubernetes Job execution.

**Technical Details:**
- **Language:** Go
- **Port:** 8080 (NodePort: 30020)
- **Execution:** Kubernetes Jobs
- **Concurrent Builds:** 3 (configurable)

**API Endpoints:**
```
GET  /api/pipelines         List pipelines
POST /api/pipelines         Create pipeline
POST /api/trigger           Trigger build
GET  /api/builds            Build history
GET  /api/builds/{id}/logs  Build logs
POST /webhooks/github       GitHub webhook
POST /webhooks/holmgit      HolmGit webhook
GET  /health                Health check
```

### Scribe - Log Aggregator

**Purpose:** Centralized log collection, search, and alerting.

**Technical Details:**
- **Language:** Go
- **Port:** 8080 (NodePort: 30017)
- **Collection Interval:** 30 seconds
- **Max Entries:** 50,000

**API Endpoints:**
```
GET  /api/logs              Query logs
GET  /api/stats             Log statistics
POST /api/search            Advanced search
GET  /api/alerts            List alert rules
GET  /api/export            Export logs (CSV)
GET  /health                Health check
```

---

## Backup & Storage

Backup management and storage services.

| Service | Purpose | Port | NodePort | Dependencies |
|---------|---------|------|----------|--------------|
| backup-service | Backup orchestration service | 8080 | 30015 | Kubernetes Jobs API |
| backup-dashboard | Backup management web UI | 8080 | 30012 | backup-service |
| backup-storage | Backup storage backend | 8080 | ClusterIP | PostgreSQL, PVC |

### Backup Service

**Purpose:** Kubernetes Job-based backup system for files and databases.

**Technical Details:**
- **Language:** Python (Flask)
- **Port:** 8080 (NodePort: 30015)
- **Storage:** /mnt/node13-ssd/backups
- **Execution:** Kubernetes Jobs with TTL cleanup

**API Endpoints:**
```
GET  /api/jobs              List backup jobs
POST /api/jobs              Trigger backup
GET  /api/jobs/{id}         Job status
GET  /api/jobs/{id}/logs    Job logs
GET  /api/history           Backup history
GET  /health                Health check
```

---

## Monitoring & Metrics

System monitoring and metrics collection.

| Service | Purpose | Port | NodePort | Dependencies |
|---------|---------|------|----------|--------------|
| metrics-dashboard | Metrics visualization dashboard | 8080 | 30950 | Prometheus |
| metrics-collector | Metrics collection agent | 8080 | ClusterIP | None |
| pulse | System health pulse monitor | 8080 | 30006 | None |

### Pulse - Health Monitor

**Purpose:** Real-time cluster health monitoring with vital signs tracking.

**Monitored Metrics:**
- Node status (all 13 nodes)
- Pod health and restart counts
- CPU/Memory usage per node
- Deployment health
- Control plane components

**Health Score Calculation:**
- Base: 100 points
- -15 per not-ready node
- -5 per failed pod
- -10 if >5 pending pods
- -15 if CPU/Memory >90%

**API Endpoints:**
```
GET  /api/status            Full cluster health status
GET  /api/nodes             Node statuses
GET  /api/pods              Problematic pods
GET  /api/alerts            Active alerts
WS   /ws                    Real-time health updates
GET  /health                Health check
```

---

## Notification Services

Notification routing and delivery.

| Service | Purpose | Port | NodePort | Dependencies |
|---------|---------|------|----------|--------------|
| notification-hub | Central notification routing hub | 8080 | ClusterIP | notification-queue, SMTP |
| notification-queue | Notification queue (PostgreSQL-backed) | 8080 | ClusterIP | PostgreSQL |

---

## User & API Services

User management, preferences, and API services.

| Service | Purpose | Port | NodePort | Dependencies |
|---------|---------|------|----------|--------------|
| user-preferences | User preferences storage | 8080 | ClusterIP | PostgreSQL |
| task-queue | Asynchronous task queue | 8080 | ClusterIP | None |

---

## Audiobook Services

Text-to-speech audiobook generation pipeline.

| Service | Purpose | Port | NodePort | Dependencies |
|---------|---------|------|----------|--------------|
| audiobook-web | Audiobook web interface | 8080 | 30700 | PostgreSQL, All audiobook services |
| audiobook-upload-epub | EPUB upload handler | 8080 | 30019 | None |
| audiobook-audio-normalize | Audio normalization | 8080 | ClusterIP | audiobook-pvc |

---

## Deployment Commands

### Deploy All Services

```bash
# Deploy all services at once
for dir in services/*/; do
  if [ -f "${dir}deployment.yaml" ]; then
    kubectl apply -f "${dir}deployment.yaml"
  fi
done
```

### By Category

#### Core Infrastructure
```bash
kubectl apply -f services/gateway/deployment.yaml
kubectl apply -f services/auth-gateway/deployment.yaml
kubectl apply -f services/vault/deployment.yaml
kubectl apply -f services/cluster-manager/deployment.yaml
kubectl apply -f services/pxe-server/deployment.yaml

# holm-system namespace
kubectl apply -f services/infrastructure/config-sync/deployment.yaml
kubectl apply -f services/infrastructure/health-aggregator/deployment.yaml
kubectl apply -f services/infrastructure/event-broker/deployment.yaml
kubectl apply -f services/infrastructure/event-persist/deployment.yaml
kubectl apply -f services/infrastructure/event-dlq/deployment.yaml
kubectl apply -f services/infrastructure/event-replay/deployment.yaml
```

#### Web Applications
```bash
kubectl apply -f services/holmos-shell/deployment.yaml
kubectl apply -f services/ios-shell/deployment.yaml
kubectl apply -f services/terminal-web/deployment.yaml
kubectl apply -f services/settings-web/deployment.yaml
kubectl apply -f services/settings-restore/deployment.yaml
kubectl apply -f services/calculator-app/deployment.yaml
kubectl apply -f services/clock-app/deployment.yaml
kubectl apply -f services/app-store-ai/deployment.yaml
kubectl apply -f services/chat-hub/deployment.yaml
```

#### AI/Bots
```bash
kubectl apply -f services/ai-bots/deployment.yaml
kubectl apply -f services/steve-bot/deployment.yaml
kubectl apply -f services/nova/deployment.yaml
kubectl apply -f services/claude-pod/deployment.yaml

# Ollama Server (external - run on Debian host)
curl -fsSL https://raw.githubusercontent.com/timholm/HolmOS/main/services/ollama-server/setup.sh | sudo bash
```

#### File Services
```bash
kubectl apply -f services/file-web-nautilus/deployment.yaml
kubectl apply -f services/file-copy/deployment.yaml
kubectl apply -f services/file-delete/deployment.yaml
kubectl apply -f services/file-download/deployment.yaml
kubectl apply -f services/file-upload/deployment.yaml
kubectl apply -f services/file-move/deployment.yaml
kubectl apply -f services/file-mkdir/deployment.yaml
kubectl apply -f services/file-meta/deployment.yaml
kubectl apply -f services/file-search/deployment.yaml
kubectl apply -f services/file-thumbnail/deployment.yaml
kubectl apply -f services/files/file-compress/deployment.yaml
kubectl apply -f services/files/file-decompress/deployment.yaml
kubectl apply -f services/files/file-convert/deployment.yaml
kubectl apply -f services/files/file-encrypt/deployment.yaml
kubectl apply -f services/files/file-permissions/deployment.yaml
kubectl apply -f services/files/file-preview/deployment.yaml
kubectl apply -f services/files/file-thumbnail/deployment.yaml
kubectl apply -f services/files/file-watch/deployment.yaml
kubectl apply -f services/files/file-share-create/deployment.yaml
kubectl apply -f services/files/file-share-validate/deployment.yaml
```

#### Development Tools
```bash
kubectl apply -f services/holm-git/deployment.yaml
kubectl apply -f services/gitea/deployment.yaml
kubectl apply -f services/cicd-controller/deployment.yaml
kubectl apply -f services/deploy-controller/deployment.yaml
kubectl apply -f services/registry-ui/deployment.yaml
kubectl apply -f services/test-dashboard/deployment.yaml
kubectl apply -f services/holm-cli/deployment.yaml
kubectl apply -f services/scribe/deployment.yaml
```

#### Backup & Monitoring
```bash
kubectl apply -f services/backup/deployment.yaml
kubectl apply -f services/backup-dashboard/deployment.yaml
kubectl apply -f services/backup/backup-storage/k8s/deployment.yaml
kubectl apply -f services/metrics-dashboard/deployment.yaml
kubectl apply -f services/monitoring/metrics-collector/deployment.yaml
kubectl apply -f services/pulse/deployment.yaml
```

#### Notifications & Users
```bash
kubectl apply -f services/notification-hub/deployment.yaml
kubectl apply -f services/notifications/notification-queue/deployment.yaml
kubectl apply -f services/users/user-preferences/k8s/deployment.yaml
kubectl apply -f services/api/task-queue/k8s/deployment.yaml
```

#### Audiobook
```bash
kubectl apply -f services/audiobook-web/deployment.yaml
kubectl apply -f services/audiobook-upload-epub/deployment.yaml
kubectl apply -f services/audiobook-audio-normalize/deployment.yaml
```

---

## Architecture Notes

### NodePort Assignments

| Port Range | Category |
|------------|----------|
| 30000-30010 | Shell & Core Apps |
| 30010-30020 | Utilities & Dashboards |
| 30020-30100 | Dev Tools & CI/CD |
| 30100-30500 | Auth & Security |
| 30500-30700 | Cluster Management |
| 30700-30900 | Terminals & Media |
| 30900-31000 | Monitoring |
| 31500-32000 | Registry & Infra |

### Service Endpoints

Access services via:
- **NodePort**: `http://192.168.8.197:<nodeport>`
- **ClusterIP**: `http://<service-name>.holm.svc.cluster.local:<port>`
- **Ingress**: `http://<service>.holm.local`

### Common Patterns

#### Health Check Standard
All services implement:
```
GET /health
```
Response:
```json
{
  "status": "healthy",
  "service": "<service-name>",
  "timestamp": "2026-01-17T12:00:00Z"
}
```

#### Standard Labels
```yaml
metadata:
  labels:
    app: <service-name>
    holmos.io/component: <category>
```

#### Resource Limits
```yaml
resources:
  requests:
    memory: "64Mi"
    cpu: "50m"
  limits:
    memory: "256Mi"
    cpu: "500m"
```

### Key Infrastructure Details

1. **Namespaces:**
   - `holm` - Application services
   - `holm-system` - Infrastructure services

2. **Node Affinity:** Services avoid scheduling on `openmediavault` node

3. **Storage:**
   - Longhorn for distributed storage
   - local-path for single-node PVCs

4. **Registries:**
   - `192.168.8.197:30500`
   - `10.110.67.87:5000`

5. **Database:** PostgreSQL at `postgres.holm.svc.cluster.local:5432`

6. **LLM Server:** Ollama at `192.168.8.230:11434` with GPU acceleration

### Catppuccin Theme Colors

All UIs use the Catppuccin Mocha palette:
- Base: `#1e1e2e`
- Surface: `#313244`
- Overlay: `#6c7086`
- Text: `#cdd6f4`
- Blue: `#89b4fa`
- Green: `#a6e3a1`
- Red: `#f38ba8`
- Mauve: `#cba6f7`
- Peach: `#fab387`
- Yellow: `#f9e2af`

---

## Contributing

When adding new services:

1. Follow the health check standard
2. Use Catppuccin Mocha theme
3. Document all API endpoints
4. Add to this SERVICES.md file
5. Register with Gateway for routing
6. Add to Health Aggregator for monitoring

---

*"Stay hungry, stay foolish." - Steve*

*"Curiouser and curiouser!" - Alice*
