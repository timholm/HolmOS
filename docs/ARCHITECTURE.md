# HolmOS Architecture

## Overview

HolmOS is a fully web-based operating system running on a 13-node Raspberry Pi Kubernetes cluster. It features an iPhone-style UI, 120+ microservices, and AI agents managing each service.

## System Architecture

```
                                    ┌─────────────────────────────────┐
                                    │         User Browser            │
                                    └───────────────┬─────────────────┘
                                                    │
                                                    ▼
┌───────────────────────────────────────────────────────────────────────────────┐
│                              HolmOS Shell (30000)                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐  │
│  │  Status Bar │ 9:41    HolmOS    ⚡ 85%  📶  🔋                          │  │
│  ├─────────────────────────────────────────────────────────────────────────┤  │
│  │                                                                          │  │
│  │   📁 Files    🖥️ Terminal   ⚙️ Settings   🎧 Audiobook                 │  │
│  │   🏪 Store    💬 Claude     📊 Cluster    🔐 Auth                       │  │
│  │   📝 Notes    📷 Photos     🎵 Music      📧 Mail                       │  │
│  │   🤖 Agents   📦 Registry   🔔 Alerts     📈 Metrics                    │  │
│  │                                                                          │  │
│  ├─────────────────────────────────────────────────────────────────────────┤  │
│  │  [Files]  [Claude]  [Store]  [Settings]  ─ Dock                         │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────────┘
                                        │
                    ┌───────────────────┼───────────────────┐
                    ▼                   ▼                   ▼
            ┌───────────────┐   ┌───────────────┐   ┌───────────────┐
            │   AI Agents   │   │    Apps       │   │ Infrastructure│
            │   (chat-hub)  │   │   (services)  │   │   (cluster)   │
            └───────────────┘   └───────────────┘   └───────────────┘
```

## Cluster Topology

```
┌─────────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                            │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                  Control Plane (rpi1)                     │   │
│  │                   192.168.8.197                           │   │
│  │  • K3s Server  • etcd  • API Server  • Scheduler         │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │  rpi2    │ │  rpi3    │ │  rpi4    │ │  rpi5    │           │
│  │  Agent   │ │  Agent   │ │  Agent   │ │  Agent   │           │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
│                                                                  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │  rpi6    │ │  rpi7    │ │  rpi8    │ │  rpi9    │           │
│  │  Agent   │ │  Agent   │ │  Agent   │ │  Agent   │           │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
│                                                                  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │  rpi10   │ │  rpi11   │ │  rpi12   │ │  rpi13   │           │
│  │  Agent   │ │  Agent   │ │  Agent   │ │  Agent   │           │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘           │
└─────────────────────────────────────────────────────────────────┘
```

## Technology Stack

| Layer | Technology |
|-------|------------|
| **Frontend** | Go templates + Alpine.js + Tailwind CSS |
| **Backend** | Go, Python, Node.js microservices |
| **Database** | PostgreSQL |
| **Cache** | In-memory (Redis planned) |
| **Message Queue** | In-memory event bus |
| **Container Runtime** | containerd |
| **Orchestration** | K3s (Kubernetes) |
| **Storage** | Longhorn distributed storage |
| **Registry** | Docker Registry v2 |
| **AI** | Claude API via chat-hub |

## Service Categories

### Core Services
Entry points and main interfaces:
- **holmos-shell** (30000) - Main iPhone-style UI
- **claude-pod** (30001) - AI chat interface
- **app-store-ai** (30002) - AI-powered app generator
- **chat-hub** (30003) - Unified agent messaging

### AI Agents
Each agent has a unique personality and manages a specific domain:
- **Nova** (30004) - Cluster management
- **Merchant** (30005) - App store AI
- **Pulse** (30006) - Metrics monitoring
- **Gateway** (30008) - API routing
- **Scribe** (30860) - Log aggregation
- **Vault** (30870) - Secret management

### Applications
User-facing apps with web UIs:
- **clock-app** (30007) - World clock, alarms, timer
- **calculator-app** (30010) - iPhone-style calculator
- **file-web-nautilus** (30088) - GNOME-style file manager
- **terminal-web** (30800) - Web-based terminal
- **audiobook-web** (30700) - Text-to-speech audiobook creator
- **settings-web** (30600) - System settings

### Infrastructure
Internal services supporting the platform:
- **holm-git** (30009) - Git repository server
- **cicd-controller** (30020) - CI/CD pipeline manager
- **deploy-controller** (30021) - Auto-deployment
- **registry-ui** (31750) - Container registry browser
- **backup-dashboard** (30850) - Backup management

### Monitoring
Observability and health checking:
- **test-dashboard** (30900) - Service health monitoring
- **metrics-dashboard** (30950) - Cluster metrics
- **health-agg** - Health aggregation

## Data Flow

```
┌────────────┐     ┌────────────┐     ┌────────────┐
│   User     │────▶│  Gateway   │────▶│  Service   │
│  Browser   │     │  (30008)   │     │   Pods     │
└────────────┘     └────────────┘     └────────────┘
                          │
                          ▼
                   ┌────────────┐
                   │  chat-hub  │◀──── AI Agents
                   │  (30003)   │
                   └────────────┘
                          │
                          ▼
                   ┌────────────┐
                   │ PostgreSQL │
                   │  Database  │
                   └────────────┘
```

## Storage Architecture

**Longhorn** provides distributed block storage across all nodes:
- Replicated volumes for high availability
- Snapshots and backups
- ~55 pods for storage management

**Persistent Volume Claims** are used for:
- Database storage (PostgreSQL)
- File storage (/data)
- Backup storage
- Registry storage

## Networking

### NodePort Ranges
- **30000-30099**: Core services and apps
- **30500-30599**: Admin services
- **30600-30699**: Settings services
- **30700-30799**: Media services
- **30800-30899**: Infrastructure
- **30900-30999**: Monitoring
- **31750**: Registry

### Internal DNS
Services communicate via Kubernetes DNS:
```
<service>.<namespace>.svc.cluster.local
```
Example: `postgres.holm.svc.cluster.local`

## Security

### Network Access
- **Local network**: Direct access via NodePort
- **Remote access**: Tailscale VPN (recommended)
- **SSH**: `rpi1@192.168.8.197`

### Authentication
- Auth gateway service handles login/logout
- Session-based authentication
- 2FA support (planned)

## High Availability

- Control plane on dedicated node (rpi1)
- Core services run with multiple replicas
- Longhorn provides storage redundancy
- Services auto-restart on failure
