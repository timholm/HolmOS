# HolmOS API Catalog

Complete catalog of all API endpoints across HolmOS services.

**Last Updated:** 2026-01-17
**Total Services with APIs:** 16
**Total Endpoints:** 150+

---

## Table of Contents

1. [Chat Hub](#chat-hub)
2. [Claude Pod](#claude-pod)
3. [Deploy Controller](#deploy-controller)
4. [Atlas Agent](#atlas-agent)
5. [Alice Bot](#alice-bot)
6. [Steve Bot](#steve-bot)
7. [Vault](#vault)
8. [Nova Dashboard](#nova-dashboard)
9. [Steve-Bot CI/CD](#steve-bot-cicd)
10. [File Web Nautilus](#file-web-nautilus)
11. [Cluster Manager](#cluster-manager)
12. [App Store AI](#app-store-ai)
13. [Backup Service](#backup-service)
14. [Merchant Terminal](#merchant-terminal)
15. [iOS Shell](#ios-shell)
16. [HolmOS Shell](#holmos-shell)

---

## Chat Hub

**Path:** `/Users/tim/HolmOS/services/chat-hub/server.js`
**Framework:** Express.js (Node.js)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serve HTML chat UI for Steve & Alice conversation |
| GET | `/health` | Health check endpoint |
| GET | `/api/bot-conversation` | Fetch bot conversation history (proxies to Steve) |
| POST | `/api/inject` | Inject user message into bot conversation |

**WebSocket:** Yes - Real-time updates on `wss://`

**Environment Variables:**
- `STEVE_URL` - Steve bot URL (default: `http://steve-bot.holm.svc.cluster.local:8080`)
- `ALICE_URL` - Alice bot URL (default: `http://alice-bot.holm.svc.cluster.local:8080`)

---

## Claude Pod

**Path:** `/Users/tim/HolmOS/services/claude-pod/server.js`
**Framework:** Express.js (Node.js)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None (uses API keys for Claude API)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serve HTML chat UI |
| GET | `/health` | Health check endpoint |
| GET | `/api/health` | API health check with model status |
| GET | `/api/model` | Get current model information |
| GET | `/api/conversations` | List all conversations |
| POST | `/api/conversations` | Create new conversation |
| GET | `/api/conversations/:id/messages` | Get messages for a conversation |
| DELETE | `/api/conversations/:id` | Delete a conversation |
| PUT | `/api/conversations/:id` | Update conversation title |
| POST | `/api/chat` | Send chat message and get response |

**WebSocket:** Yes - Streaming responses

**Environment Variables:**
- `ANTHROPIC_API_KEY` - Claude API key
- `DATABASE_URL` - PostgreSQL connection string

---

## Deploy Controller

**Path:** `/Users/tim/HolmOS/services/devops/deploy-controller/main.py`
**Framework:** FastAPI (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Dashboard HTML UI |
| GET | `/api/deployments` | List all deployments in namespace |
| POST | `/api/deploy` | Deploy an image to the cluster |
| POST | `/api/rollback/{name}` | Rollback a deployment |
| GET | `/api/images` | List available images from registry |
| GET | `/api/pending` | List pending deployments |
| GET | `/api/history` | Get deployment history |
| POST | `/api/auto-deploy/{enabled}` | Toggle auto-deploy feature |
| GET | `/health` | Health check endpoint |

**Environment Variables:**
- `NAMESPACE` - Kubernetes namespace (default: `holm`)
- `REGISTRY_URL` - Container registry URL

---

## Atlas Agent

**Path:** `/Users/tim/HolmOS/services/agents/atlas/app.py`
**Framework:** FastAPI (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check endpoint |
| GET | `/capabilities` | List agent capabilities |
| POST | `/chat` | Chat with Atlas file management agent |

**Description:** AI agent for file management operations.

---

## Alice Bot

**Path:** `/Users/tim/HolmOS/services/ai-bots/alice.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check endpoint |
| GET | `/api/status` | Get bot status and current activity |
| POST | `/api/explore` | Trigger codebase exploration |
| POST | `/api/chat` | Send chat message to Alice |
| POST | `/api/respond` | Receive response from Steve |
| GET | `/api/report` | Get exploration report |
| GET | `/api/conversations` | Get conversation history |
| GET | `/api/discoveries` | Get code discoveries |

**WebSocket:** `/ws` - Real-time updates

**Environment Variables:**
- `OLLAMA_URL` - Ollama API URL
- `MODEL` - Model name (default: `gemma3`)

---

## Steve Bot

**Path:** `/Users/tim/HolmOS/services/ai-bots/steve.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check endpoint |
| GET | `/api/status` | Get bot status and metrics |
| POST | `/api/analyze` | Trigger cluster analysis |
| POST | `/api/chat` | Send chat message to Steve |
| POST | `/api/respond` | Receive response from Alice |
| GET | `/api/conversations` | Get conversation history |
| GET | `/api/improvements` | Get suggested improvements |
| GET | `/api/cluster` | Get cluster information |

**WebSocket:** `/ws` - Real-time updates

**Environment Variables:**
- `OLLAMA_URL` - Ollama API URL
- `MODEL` - Model name (default: `deepseek-r1`)

---

## Vault

**Path:** `/Users/tim/HolmOS/services/vault/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None (encryption at rest with AES-256-GCM)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serve HTML secrets management UI |
| GET | `/api/secrets` | List all secrets (metadata only) |
| POST | `/api/secrets` | Create a new secret |
| GET | `/api/secrets/<name>` | Read a secret value |
| PUT | `/api/secrets/<name>` | Update a secret |
| DELETE | `/api/secrets/<name>` | Delete a secret |
| GET | `/api/audit` | Get audit log of secret access |
| GET | `/api/secrets/<name>/versions` | Get secret version history |
| POST | `/api/secrets/<name>/rotate` | Rotate a secret |
| GET | `/api/secrets/<name>/rotation-policy` | Get rotation policy |
| PUT | `/api/secrets/<name>/rotation-policy` | Set rotation policy |
| GET | `/api/rotation/pending` | Get secrets pending rotation |
| GET | `/api/health` | Health check endpoint |

**Security:** Secrets encrypted with AES-256-GCM before storage.

---

## Nova Dashboard

**Path:** `/Users/tim/HolmOS/services/nova/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Kubernetes dashboard HTML UI |
| GET | `/health` | Health check endpoint |
| GET | `/api/pods` | List pods with caching |
| GET | `/api/deployments` | List deployments |
| GET | `/api/services` | List services |
| GET | `/api/nodes` | List cluster nodes |
| GET | `/api/namespaces` | List namespaces |
| POST | `/api/pods/<name>/restart` | Restart a pod |
| GET | `/api/pods/<name>/logs` | Get pod logs |
| DELETE | `/api/pods/<name>` | Delete a pod |

**Note:** Includes response caching for performance.

---

## Steve-Bot CI/CD

**Path:** `/Users/tim/HolmOS/services/steve-bot/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check endpoint |
| GET | `/bugs` | Get list of bugs/issues |
| GET | `/rants` | Get developer rants/notes |
| GET | `/status` | Get system status |
| GET | `/cicd` | Get CI/CD pipeline status |
| GET | `/test` | Get test results |

---

## File Web Nautilus

**Path:** `/Users/tim/HolmOS/services/file-web-nautilus/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | File manager HTML UI |
| GET | `/health` | Health check endpoint |
| GET | `/api/files` | List files in directory |
| GET | `/api/files/<path>` | Get file contents or info |
| POST | `/api/files` | Create new file |
| PUT | `/api/files/<path>` | Update file contents |
| DELETE | `/api/files/<path>` | Delete file |
| POST | `/api/upload` | Upload file(s) |
| GET | `/api/download/<path>` | Download file |
| POST | `/api/mkdir` | Create directory |
| POST | `/api/move` | Move file or directory |
| POST | `/api/copy` | Copy file or directory |
| POST | `/api/rename` | Rename file or directory |
| GET | `/api/search` | Search for files |
| GET | `/api/recent` | Get recently modified files |
| GET | `/api/favorites` | Get favorite files |
| POST | `/api/favorites` | Add to favorites |
| DELETE | `/api/favorites/<path>` | Remove from favorites |
| POST | `/api/compress` | Compress files to archive |
| POST | `/api/decompress` | Extract archive |
| GET | `/api/trash` | List trash contents |
| POST | `/api/trash` | Move to trash |
| POST | `/api/trash/restore` | Restore from trash |
| DELETE | `/api/trash` | Empty trash |
| GET | `/api/storage` | Get storage information |
| GET | `/api/storage/configs` | List storage configurations |
| POST | `/api/storage/configs` | Add storage configuration |
| PUT | `/api/storage/configs/<id>` | Update storage configuration |
| DELETE | `/api/storage/configs/<id>` | Delete storage configuration |
| GET | `/api/preview/<path>` | Get file preview |
| GET | `/api/thumbnail/<path>` | Get file thumbnail |

---

## Cluster Manager

**Path:** `/Users/tim/HolmOS/services/cluster-manager/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Cluster management HTML UI |
| GET | `/health` | Health check endpoint |
| GET | `/api/nodes` | List all cluster nodes |
| GET | `/api/nodes/<name>` | Get node details |
| POST | `/api/nodes/<name>/ssh` | Execute SSH command on node |
| POST | `/api/nodes/<name>/reboot` | Reboot a node |
| POST | `/api/nodes/<name>/shutdown` | Shutdown a node |
| GET | `/api/nodes/<name>/metrics` | Get node metrics |
| GET | `/api/nodes/<name>/processes` | Get running processes |
| GET | `/api/nodes/<name>/services` | Get systemd services |
| POST | `/api/nodes/<name>/services/<svc>/restart` | Restart a service |
| GET | `/api/nodes/<name>/logs` | Get system logs |
| GET | `/api/nodes/<name>/disk` | Get disk usage |
| GET | `/api/nodes/<name>/network` | Get network info |
| GET | `/api/cluster/status` | Get overall cluster status |
| GET | `/api/cluster/metrics` | Get cluster-wide metrics |
| POST | `/api/cluster/command` | Execute command on all nodes |

**Environment Variables:**
- `SSH_USER` - SSH username for nodes
- `SSH_KEY_PATH` - Path to SSH private key

---

## App Store AI

**Path:** `/Users/tim/HolmOS/services/app-store-ai/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | App store HTML UI |
| GET | `/health` | Health check endpoint |
| GET | `/apps` | List available apps |
| GET | `/api/apps` | List apps (API endpoint) |
| GET | `/api/apps/<name>` | Get app details |
| POST | `/apps/<name>/deploy` | Deploy an app |
| DELETE | `/apps/<name>` | Remove an app |
| GET | `/api/categories` | List app categories |
| GET | `/api/featured` | Get featured apps |
| GET | `/api/search` | Search apps |
| POST | `/api/merchant/*` | Proxy to Merchant AI |
| POST | `/api/forge/*` | Proxy to Forge Builder |

---

## Backup Service

**Path:** `/Users/tim/HolmOS/services/backup/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Backup dashboard HTML UI |
| GET | `/health` | Health check endpoint |
| GET | `/api/jobs` | List all backup jobs |
| GET | `/api/jobs/<job_id>` | Get specific backup job |
| GET | `/api/jobs/<job_id>/logs` | Get backup job logs |
| GET | `/api/jobs/<job_id>/status` | Get job status (with long-polling) |
| POST | `/api/backup/trigger` | Trigger a new backup job |
| POST | `/api/backup/quick` | Quick backup with defaults |
| GET | `/api/history` | Get backup history |
| GET | `/api/stats` | Get backup statistics |

**Environment Variables:**
- `NAMESPACE` - Kubernetes namespace (default: `holm`)
- `BACKUP_STORAGE_PATH` - Backup storage path (default: `/mnt/node13-ssd/backups`)
- `BACKUP_IMAGE` - Container image for backup jobs

---

## Merchant Terminal

**Path:** `/Users/tim/HolmOS/services/merchant/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Terminal HTML UI |
| GET | `/health` | Health check endpoint |
| POST | `/api/connect` | Create SSH session |
| POST | `/api/disconnect` | Close SSH session |
| POST | `/api/execute` | Execute command in session |
| GET | `/api/context` | Get HolmOS development context |
| GET | `/api/sessions` | List active sessions |

**Environment Variables:**
- `SSH_HOST` - Default SSH host
- `SSH_PORT` - SSH port (default: 22)
- `SSH_USER` - SSH username
- `SSH_PASSWORD` - SSH password

---

## iOS Shell

**Path:** `/Users/tim/HolmOS/services/ios-shell/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | iOS-style SpringBoard UI |
| GET | `/manifest.json` | PWA manifest |
| GET | `/api/registry` | Get app registry |
| GET | `/health` | Health check endpoint |
| GET | `/ready` | Readiness check endpoint |

**Description:** iPhone SpringBoard-style interface with app launcher.

---

## HolmOS Shell

**Path:** `/Users/tim/HolmOS/services/holmos-shell/app.py`
**Framework:** Flask (Python)
**Port:** 8080 (configurable via PORT env)
**Authentication:** None

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | iPhone-style home screen UI |
| GET | `/api/apps` | List available apps |
| GET | `/api/status` | Get system status |
| GET | `/health` | Health check endpoint |
| GET | `/ready` | Readiness check endpoint |

**Description:** iPhone-style home screen for cluster management.

---

## Services Missing API Documentation

The following services exist in `/Users/tim/HolmOS/services/` but either:
- Have no discoverable API endpoints
- Use a different framework not scanned
- Are frontend-only services
- Are configuration/infrastructure services

| Service Directory | Notes |
|-------------------|-------|
| `alice-bot/` | May duplicate ai-bots/alice |
| `api/` | Gateway/routing service |
| `auth-gateway/` | Authentication layer |
| `backup-dashboard/` | UI for backup service |
| `calculator-app/` | Simple calculator UI |
| `cicd-controller/` | CI/CD orchestration |
| `clock-app/` | Clock UI application |
| `devops/` | Parent directory |
| `files/` | File service (check subdirs) |
| `gateway/` | API gateway |
| `health-agg/` | Health aggregation |
| `holm-git/` | Git integration |
| `metrics-dashboard/` | Metrics visualization |
| `notification-hub/` | Push notifications |
| `pxe-server/` | Network boot service |
| `registry-ui/` | Container registry UI |
| `scribe/` | Note-taking service |
| `settings-web/` | Settings UI |
| `terminal-web/` | Web terminal |
| `test-dashboard/` | Testing dashboard |

---

## Authentication Summary

| Service | Auth Type | Notes |
|---------|-----------|-------|
| All services | None | No authentication required |
| Vault | Encryption | Secrets encrypted at rest (AES-256-GCM) |
| Claude Pod | API Key | Uses Anthropic API key for Claude |
| Cluster Manager | SSH Keys | SSH key auth for node access |
| Merchant | SSH Creds | SSH password stored in env |

**Recommendation:** Consider adding authentication layer via auth-gateway for production use.

---

## Common Patterns

### Health Check Endpoints
All services implement:
- `GET /health` - Returns `{"status": "healthy"}` or similar

### API Conventions
- JSON request/response format
- REST-style resource naming
- Error responses include `{"error": "message"}`

### WebSocket Support
- Chat Hub, Claude Pod, Alice Bot, Steve Bot support WebSocket connections

### Environment Variables
Most services use:
- `PORT` - Server port (default: 8080)
- `DEBUG` - Enable debug mode
- `NAMESPACE` - Kubernetes namespace (default: `holm`)

---

## Quick Reference by Port

| Port | Service | Primary Use |
|------|---------|-------------|
| 30002 | App Store AI | App deployment |
| 30003 | Chat Hub | Bot conversation |
| 30004 | Nova | K8s dashboard |
| 30010 | Calculator | Calculator app |
| 30011 | Clock | Clock app |
| 30020 | CI/CD Controller | Pipeline management |
| 30021 | Deploy Controller | Deployment automation |
| 30088 | File Nautilus | File management |
| 30100 | Auth Gateway | Authentication |
| 30500 | HolmGit | Git operations |
| 30502 | Cluster Manager | Node management |
| 30600 | Settings | System settings |
| 30700 | Audiobook | Audio player |
| 30800 | Terminal | Web terminal |
| 30850 | Backup | Backup management |
| 30860 | Scribe | Note-taking |
| 30870 | Vault | Secrets management |
| 30900 | Test Dashboard | Testing |
| 30950 | Metrics | Metrics dashboard |
| 31500 | Registry | Container registry |
| 31750 | Registry UI | Registry web UI |
