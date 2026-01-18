# HolmOS Services Reference

Complete reference for all HolmOS services and their configurations.

## Service Registry

All services are defined in `services.yaml` and deployed to the `holm` namespace.

## Core Entry Points

### holmos-shell
The main web interface with iPhone-style UI.

| Property | Value |
|----------|-------|
| Port | 30000 |
| Replicas | 2 |
| Health | `/health` |
| Language | Go |

**Features:**
- Home screen with app grid
- Bottom dock with pinned apps
- Status bar (time, battery, network)
- Gesture navigation
- App switcher
- Control center

### claude-pod
AI chat interface powered by Claude.

| Property | Value |
|----------|-------|
| Port | 30001 |
| Replicas | 1 |
| Health | `/health` |
| Language | Node.js |

**Features:**
- WebSocket chat interface
- Full cluster access (kubectl, git)
- Persistent conversation history

### app-store-ai
AI-powered app generator - describe what you need.

| Property | Value |
|----------|-------|
| Port | 30002 |
| Replicas | 1 |
| Health | `/health` |
| Language | Python |

**Features:**
- Natural language app requests
- Dynamic pod deployment
- Automatic icon generation

### chat-hub
Unified messaging interface for all AI agents.

| Property | Value |
|----------|-------|
| Port | 30003 |
| Replicas | 1 |
| Health | `/health` |
| Language | Node.js |

**Features:**
- Routes messages to appropriate agents
- Agent personality system
- Real-time WebSocket communication

---

## AI Agent Services

### nova
Cluster management agent - "I see all 13 stars in our constellation"

| Property | Value |
|----------|-------|
| Port | 30004 |
| Replicas | 1 |
| Language | Go |

**Capabilities:**
- Monitor nodes and pods
- Deploy and scale services
- Cluster health overview

### merchant
App store agent - "Describe what you need, I'll make it happen"

| Property | Value |
|----------|-------|
| Port | 30005 |
| Replicas | 1 |
| Language | Python |

**Capabilities:**
- Analyze user requirements
- Build and deploy services
- Manage app lifecycle

### pulse
Metrics agent - "Vital signs are looking good"

| Property | Value |
|----------|-------|
| Port | 30006 |
| Replicas | 1 |
| Language | Go |

**Capabilities:**
- Collect system metrics
- Analyze trends
- Health reporting

### gateway
API gateway agent - "All roads lead through me"

| Property | Value |
|----------|-------|
| Port | 30008 |
| Replicas | 2 |
| Language | Go |

**Capabilities:**
- Route API requests
- Rate limiting
- Load balancing

### scribe
Log aggregation agent - "It's all in the records"

| Property | Value |
|----------|-------|
| Port | 30860 |
| Replicas | 1 |
| Language | Go |

**Capabilities:**
- Aggregate logs from all services
- Search and filter
- Pattern analysis

### vault
Secret management agent - "Your secrets are safe with me"

| Property | Value |
|----------|-------|
| Port | 30870 |
| Replicas | 1 |
| Language | Go |

**Capabilities:**
- Store secrets securely
- Key management
- Encryption services

---

## Application Services

### clock-app
World clock, alarms, and timer.

| Property | Value |
|----------|-------|
| Port | 30007 |
| Replicas | 1 |
| Language | Go |

### calculator-app
iPhone-style calculator.

| Property | Value |
|----------|-------|
| Port | 30010 |
| Replicas | 1 |
| Language | Go |

### file-web-nautilus
GNOME-style file manager.

| Property | Value |
|----------|-------|
| Port | 30088 |
| Replicas | 1 |
| Language | Go |

**Features:**
- Grid and list views
- Thumbnail generation
- Drag and drop
- Context menus
- Search with filters

### terminal-web
Web-based terminal.

| Property | Value |
|----------|-------|
| Port | 30800 |
| Replicas | 1 |
| Language | Go |

**Features:**
- Full terminal emulation
- SSH host management
- Command history

### audiobook-web
Text-to-speech audiobook creator.

| Property | Value |
|----------|-------|
| Port | 30700 |
| Replicas | 1 |
| Language | Go |

**Features:**
- EPUB/TXT upload
- AI-powered TTS
- Audio library management

### settings-web
System settings hub.

| Property | Value |
|----------|-------|
| Port | 30600 |
| Replicas | 1 |
| Language | Go |

**Features:**
- Theme management
- Notification preferences
- Display settings

---

## Infrastructure Services

### holm-git
Git repository server.

| Property | Value |
|----------|-------|
| Port | 30009 |
| Replicas | 1 |
| Language | Go |

### cicd-controller
CI/CD pipeline manager.

| Property | Value |
|----------|-------|
| Port | 30020 |
| Replicas | 1 |
| Language | Go |

**Features:**
- Build pipelines
- Deployment automation
- GitHub Actions integration

### deploy-controller
Auto-deployment controller.

| Property | Value |
|----------|-------|
| Port | 30021 |
| Replicas | 1 |
| Language | Go |

### registry-ui
Container registry browser.

| Property | Value |
|----------|-------|
| Port | 31750 |
| Replicas | 1 |
| Language | Go |

**Features:**
- Browse images and tags
- Image cleanup
- Push/pull management

### pxe-server
Network boot server for x86 laptops.

| Property | Value |
|----------|-------|
| Replicas | 1 |
| Language | Go |

**Features:**
- TFTP server
- Network boot images
- Auto-provisioning

### tailscale
VPN subnet router for remote access.

| Property | Value |
|----------|-------|
| Namespace | tailscale |
| Replicas | 1 |

**Features:**
- Secure remote access
- Subnet routing
- No port forwarding needed

---

## Monitoring Services

### test-dashboard
Service health monitoring.

| Property | Value |
|----------|-------|
| Port | 30900 |
| Replicas | 1 |
| Language | Go |

### metrics-dashboard
Cluster metrics visualization.

| Property | Value |
|----------|-------|
| Port | 30950 |
| Replicas | 1 |
| Language | Go |

### backup-dashboard
Backup management interface.

| Property | Value |
|----------|-------|
| Port | 30850 |
| Replicas | 1 |
| Language | Go |

### cluster-manager
Cluster admin dashboard.

| Property | Value |
|----------|-------|
| Port | 30502 |
| Replicas | 1 |
| Language | Python |

---

## File Services

Microservices for file operations:

| Service | Function |
|---------|----------|
| file-copy | Copy files |
| file-delete | Delete files |
| file-download | Download files |
| file-meta | File metadata |
| file-mkdir | Create directories |
| file-move | Move files |
| file-search | Search files |
| file-thumbnail | Generate thumbnails |
| file-upload | Upload files |

---

## Infrastructure Components

Located in `services/infrastructure/`:

| Service | Function |
|---------|----------|
| config-sync | Configuration synchronization |
| event-broker | Event message broker |
| event-dlq | Dead letter queue |
| event-persist | Event persistence |
| event-replay | Event replay |
| health-aggregator | Health check aggregation |

---

## Bot Services

| Service | Purpose |
|---------|---------|
| alice-bot | Chat bot |
| steve-bot | Chat bot |

---

## Deployment Defaults

From `services.yaml`:

```yaml
defaults:
  namespace: holm
  image_pull_policy: Always
  resources:
    requests:
      memory: "64Mi"
      cpu: "50m"
    limits:
      memory: "256Mi"
      cpu: "500m"
```

## Node Affinity

Some nodes are excluded from certain workloads:
- `openmediavault` - No insecure registry config
