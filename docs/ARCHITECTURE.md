# HolmOS System Architecture

> **🤖 CLAUDE/AI AGENTS:** To access the cluster, use `https://cmd.holm.chat/run` - See [CLAUDE.md](../CLAUDE.md). Direct SSH/kubectl is blocked from sandboxed environments.

## System Overview

HolmOS is a fully web-based operating system running on a 13-node Raspberry Pi Kubernetes cluster. It provides an iPhone-style mobile-first interface with AI agents managing every service. The system is built on K3s (lightweight Kubernetes) and designed for home self-hosting with full microservices architecture.

```
+------------------------------------------------------------------+
|                         HolmOS Shell                              |
|                    (Mobile-first Web UI)                          |
+------------------------------------------------------------------+
         |              |              |              |
    +----v----+   +----v----+   +-----v-----+   +----v----+
    |   AI    |   |  Core   |   |   Apps    |   | Infra   |
    | Agents  |   |Services |   |           |   |         |
    +---------+   +---------+   +-----------+   +---------+
         |              |              |              |
+------------------------------------------------------------------+
|                    Kubernetes (K3s)                               |
|                  13 Raspberry Pi Nodes                            |
+------------------------------------------------------------------+
```

---

## Node Topology

### Cluster Configuration

| Node | Role | IP Address | Hardware | OS |
|------|------|------------|----------|-----|
| rpi-1 | Control Plane | 192.168.8.197 | Raspberry Pi | Debian 13 (trixie) |
| rpi-2 | Worker | 192.168.8.196 | Raspberry Pi 4 | Debian 13 (trixie) |
| rpi-3 | Worker | 192.168.8.195 | Raspberry Pi 4 | Debian 13 (trixie) |
| rpi-4 | Worker | 192.168.8.194 | Raspberry Pi 4 | Debian 13 (trixie) |
| rpi-5 | Worker | 192.168.8.108 | Raspberry Pi 5 | Debian 13 (trixie) |
| rpi-6 | Worker | 192.168.8.235 | Raspberry Pi 5 | Debian 13 (trixie) |
| rpi-7 | Worker | 192.168.8.209 | Raspberry Pi 5 | Debian 13 (trixie) |
| rpi-8 | Worker | 192.168.8.202 | Raspberry Pi 5 | Debian 13 (trixie) |
| rpi-9 | Worker | 192.168.8.187 | Raspberry Pi 5 | Debian 13 (trixie) |
| rpi-10 | Worker | 192.168.8.210 | Raspberry Pi 5 | Debian 13 (trixie) |
| rpi-11 | Worker | 192.168.8.231 | Raspberry Pi 5 | Debian 13 (trixie) |
| rpi-12 | Worker | 192.168.8.105 | Raspberry Pi 5 | Debian 13 (trixie) |
| openmediavault | Worker | 192.168.8.199 | x86 Server | Debian 12 (bookworm) |

### Node Topology Diagram

```
                              +------------------+
                              |     rpi-1        |
                              |  Control Plane   |
                              |  192.168.8.197   |
                              +--------+---------+
                                       |
            +--------------------------+--------------------------+
            |                          |                          |
     +------v------+           +-------v------+           +-------v-------+
     | rpi-2,3,4   |           | rpi-5,6,7,8  |           | rpi-9,10,11,12|
     | Pi 4 Workers|           | Pi 5 Workers |           | Pi 5 Workers  |
     | (3 nodes)   |           | (4 nodes)    |           | (4 nodes)     |
     +-------------+           +--------------+           +---------------+
                                                                  |
                                                          +-------v--------+
                                                          | openmediavault |
                                                          |  x86 Server    |
                                                          +----------------+
```

### Node Responsibilities

- **rpi-1 (Control Plane)**: Runs K3s control plane, API server, scheduler, etcd. Hosts core services like registry, PostgreSQL, and most web UIs.
- **rpi-2,3,4 (Pi 4 Workers)**: General workload nodes for lighter services.
- **rpi-5 through rpi-12 (Pi 5 Workers)**: Higher-performance workers for compute-intensive services (AI agents, CI/CD, terminal).
- **openmediavault**: Storage server running CoreDNS and Traefik load balancing.

---

## Service Architecture

### Layer Diagram

```
+=========================================================================+
|                          APPLICATION LAYER                               |
|  +------------+  +------------+  +------------+  +------------+          |
|  | holmos-    |  | calculator |  | clock-app  |  | audiobook- |          |
|  | shell      |  | app        |  |            |  | web        |          |
|  +------------+  +------------+  +------------+  +------------+          |
+=========================================================================+
|                           AI AGENTS LAYER                                |
|  +------------+  +------------+  +------------+  +------------+          |
|  | nova       |  | alice-bot  |  | steve-bot  |  | app-store- |          |
|  | (cluster)  |  | (AI)       |  | (AI)       |  | ai         |          |
|  +------------+  +------------+  +------------+  +------------+          |
|  +------------+  +------------+  +------------+  +------------+          |
|  | chat-hub   |  | pulse      |  | scribe     |  | vault      |          |
|  | (router)   |  | (metrics)  |  | (logs)     |  | (secrets)  |          |
|  +------------+  +------------+  +------------+  +------------+          |
+=========================================================================+
|                          CORE SERVICES LAYER                             |
|  +------------+  +------------+  +------------+  +------------+          |
|  | auth-      |  | settings-  |  | file-web-  |  | terminal-  |          |
|  | gateway    |  | web        |  | nautilus   |  | web        |          |
|  +------------+  +------------+  +------------+  +------------+          |
|  +------------+  +------------+  +------------+  +------------+          |
|  | holm-git   |  | deploy-    |  | cicd-      |  | cluster-   |          |
|  |            |  | controller |  | controller |  | manager    |          |
|  +------------+  +------------+  +------------+  +------------+          |
+=========================================================================+
|                        INFRASTRUCTURE LAYER                              |
|  +------------+  +------------+  +------------+  +------------+          |
|  | postgres   |  | registry   |  | traefik    |  | coredns    |          |
|  |            |  |            |  | (ingress)  |  |            |          |
|  +------------+  +------------+  +------------+  +------------+          |
|  +------------+  +------------+  +------------+                          |
|  | metrics-   |  | local-path |  | pxe-       |                          |
|  | server     |  | provisioner|  | server     |                          |
|  +------------+  +------------+  +------------+                          |
+=========================================================================+
```

### Core Services

| Service | Port | Description |
|---------|------|-------------|
| holmos-shell | 30000 | Main iPhone-style home screen UI |
| ios-shell | 30001 | iOS-optimized shell interface |
| auth-gateway | 30100 | Authentication and authorization gateway |
| settings-web | 30600 | System settings management |
| file-web-nautilus | 30088 | Nautilus-style file manager |
| terminal-web | 30800 | Web-based terminal emulator |
| holm-git | 30009 | Internal Git repository service |

### AI Agents Layer

| Agent | Service | Port | Role |
|-------|---------|------|------|
| Nova | nova | 30004 | Cluster dashboard and management |
| Pulse | pulse | 30006 | Metrics monitoring and health |
| Scribe | scribe | 30017 | Log aggregation and search |
| Vault | vault | 30870 | Secret management |
| Alice | alice-bot | 30668 | AI assistant bot |
| Steve | steve-bot | 30099 | AI assistant bot |
| Merchant | app-store-ai | 30002 | AI-powered app store |
| Chat Hub | chat-hub | 30003 | Unified agent messaging router |

### Application Layer

| App | Port | Description |
|-----|------|-------------|
| calculator-app | 30010 | Calculator utility |
| clock-app | 30011 | World clock and alarms |
| audiobook-web | 30700 | Audiobook TTS converter |
| metrics-dashboard | 30950 | Cluster metrics visualization |
| registry-ui | 31750 | Container registry browser |

### Infrastructure Layer

| Component | Description |
|-----------|-------------|
| PostgreSQL | Primary database (postgres.holm.svc.cluster.local:5432) |
| Registry | Container image registry (localhost:31500) |
| Traefik | Ingress controller and load balancer |
| CoreDNS | Internal cluster DNS |
| Local-path Provisioner | Dynamic PVC provisioning |
| Metrics Server | Kubernetes metrics collection |
| PXE Server | Network boot for cluster nodes |

---

## Data Flow

### Request Flow: User to Service

```
+--------+     +----------+     +------------+     +---------+
| Browser| --> | Traefik  | --> | NodePort   | --> | Service |
| :30000 |     | Ingress  |     | Service    |     | Pod     |
+--------+     +----------+     +------------+     +---------+
                                      |
                                      v
                              +---------------+
                              | ClusterIP     |
                              | Internal DNS  |
                              +---------------+
```

1. User accesses `http://192.168.8.197:30000` (HolmOS Shell)
2. Request hits any node's NodePort (Traefik handles load balancing)
3. Kubernetes routes to the appropriate pod via ClusterIP
4. Internal services communicate via DNS: `<service>.holm.svc.cluster.local`

### AI Bot Communication Flow

```
+--------+     +----------+     +------------+     +-----------+
| User   | --> | chat-hub | --> | Agent      | --> | Backend   |
| Input  |     | Router   |     | (nova/     |     | Services  |
+--------+     +----------+     | pulse/etc) |     +-----------+
                    |           +------------+           |
                    |                                    |
                    v                                    v
             +------------+                    +----------------+
             | AI Context |                    | kubectl/       |
             | & State    |                    | Prometheus/    |
             +------------+                    | Postgres       |
                                               +----------------+
```

1. User sends message to chat-hub (port 30003)
2. Chat-hub routes to appropriate agent based on context
3. Agent processes request using its specialized knowledge
4. Agent queries backend services (Kubernetes API, Prometheus, etc.)
5. Response flows back through chat-hub to user

### CI/CD Pipeline Flow

```
+--------+     +----------+     +------------+     +------------+
| GitHub | --> | Actions  | --> | Build ARM64| --> | Artifacts  |
| Push   |     | Trigger  |     | Image      |     | Upload     |
+--------+     +----------+     +------------+     +------------+
                                                        |
                                                        v (manual)
+----------+     +------------+     +------------+     +----------+
| Registry | <-- | ctr import | <-- | Download   | <-- | Pi Cluster|
| :31500   |     |            |     | Artifact   |     |           |
+----------+     +------------+     +------------+     +----------+
      |
      v
+------------+     +------------+
| cicd-      | --> | kubectl    |
| controller |     | rollout    |
+------------+     +------------+
```

1. **GitHub Actions Trigger**: Push to `services/**` triggers CI workflow
2. **Build**: Cross-compile ARM64 images using Docker Buildx with QEMU
3. **Artifact**: Upload built images as GitHub artifacts
4. **Manual Deploy**: Download artifacts to Pi cluster (private network)
5. **Import**: `ctr -n k8s.io images import <service>.tar`
6. **Rollout**: `kubectl rollout restart deployment/<service> -n holm`

---

## Storage Architecture

### PersistentVolumeClaims

| PVC | Capacity | Purpose |
|-----|----------|---------|
| holm-git-data | 20Gi | Git repository storage |
| registry-pvc (holm) | 10Gi | Container image storage |
| registry-pvc (registry) | 10Gi | Registry namespace images |
| backup-storage-pvc | 5Gi | Backup data storage |
| audiobook-pvc | 5Gi | Audiobook files and conversions |
| backup-postgres-pvc | 1Gi | PostgreSQL backup snapshots |
| vault-data | 1Gi | Secrets and encrypted data |
| ai-bots-data | 1Gi | AI agent persistent state |

### Storage Class

```yaml
Name:            local-path (default)
Provisioner:     rancher.io/local-path
ReclaimPolicy:   Delete
VolumeBindMode:  WaitForFirstConsumer
```

The local-path provisioner dynamically creates PersistentVolumes on the node where the pod is scheduled. Data is stored in `/var/lib/rancher/k3s/storage/` on each node.

### Storage Topology

```
+------------------+
|    rpi-1         |
|  /var/lib/...    |
|  +------------+  |
|  | postgres   |  |
|  | registry   |  |
|  | holm-git   |  |
|  | vault      |  |
|  +------------+  |
+------------------+

+------------------+
|  openmediavault  |
|  (NAS storage)   |
|  +------------+  |
|  | backups    |  |
|  | media      |  |
|  +------------+  |
+------------------+
```

---

## Network Architecture

### NodePort Assignments

NodePorts are assigned in the 30000-32767 range. HolmOS uses the following convention:

| Range | Purpose |
|-------|---------|
| 30000-30099 | Core shell and AI services |
| 30100-30199 | Authentication services |
| 30500-30599 | Cluster management |
| 30600-30699 | Settings and configuration |
| 30700-30799 | Media services |
| 30800-30899 | Terminal and tools |
| 30900-30999 | Monitoring and metrics |
| 31500-31999 | Registry and infrastructure |

### Full Port Map

| Port | Service | Type |
|------|---------|------|
| 30000 | holmos-shell | NodePort |
| 30001 | ios-shell | NodePort |
| 30002 | app-store-ai | NodePort |
| 30003 | chat-hub | NodePort |
| 30004 | nova | NodePort |
| 30006 | pulse | NodePort |
| 30009 | holm-git | NodePort |
| 30010 | calculator-app | NodePort |
| 30011 | clock-app | NodePort |
| 30015 | deploy-controller | NodePort |
| 30016 | backup-svc | NodePort |
| 30017 | scribe | NodePort |
| 30020 | cicd-controller | NodePort |
| 30088 | file-web-nautilus | NodePort |
| 30099 | steve-bot | NodePort |
| 30100 | auth-gateway | NodePort |
| 30502 | cluster-manager | NodePort |
| 30600 | settings-web | NodePort |
| 30667 | health-agg | NodePort |
| 30668 | alice-bot | NodePort |
| 30700 | audiobook-web | NodePort |
| 30800 | terminal-web | NodePort |
| 30870 | vault | NodePort |
| 30950 | metrics-dashboard | NodePort |
| 31500 | registry | NodePort |
| 31750 | registry-ui | NodePort |

### Service Discovery

Internal services use Kubernetes DNS for discovery:

```
<service-name>.<namespace>.svc.cluster.local

Examples:
  postgres.holm.svc.cluster.local:5432
  chat-hub.holm.svc.cluster.local:80
  registry.holm.svc.cluster.local:5000
```

### Network Diagram

```
                          Internet
                              |
                              X (No external access)
                              |
+-----------------------------+-----------------------------+
|                     Home Network (192.168.8.0/24)         |
+-----------------------------+-----------------------------+
            |                           |
    +-------v-------+           +-------v-------+
    | User Device   |           | Other Home    |
    | Browser       |           | Devices       |
    +-------+-------+           +---------------+
            |
            | HTTP :30000-32767
            v
+------------------------------------------------------------------+
|                    Kubernetes Cluster Network                     |
|                                                                   |
|  +------------------+  Service Network: 10.43.0.0/16             |
|  | CoreDNS          |                                            |
|  | 10.43.0.10       |                                            |
|  +------------------+                                            |
|                                                                   |
|  +------------------+  Pod Network: 10.42.0.0/16                 |
|  | Traefik LB       |                                            |
|  | All Node IPs     |                                            |
|  +------------------+                                            |
|                                                                   |
+------------------------------------------------------------------+
```

---

## Security Model

### RBAC Configuration

#### Service Accounts

| Service Account | Namespace | Purpose |
|-----------------|-----------|---------|
| default | holm | Default service identity |
| cluster-manager | holm | Read-only cluster access |
| cicd-controller | holm | Build and deploy operations |
| deploy-controller | holm | Deployment management |
| nova | holm | Cluster monitoring |
| pulse | holm | Metrics access |
| scribe | holm | Log aggregation |
| backup-service | holm | Backup operations |
| terminal-web | holm | Terminal execution |
| metrics-dashboard | holm | Metrics viewing |
| ai-bot | holm | AI agent operations |

#### ClusterRoles

**cluster-manager** (Read-only cluster monitoring):
```
Resources: nodes, pods, services, deployments, daemonsets,
           replicasets, statefulsets, namespaces, pods/log
Verbs: get, list, watch
```

**cicd-controller** (Build and deploy):
```
Resources: pods, services, secrets, configmaps, jobs, pods/log
Verbs: get, list, watch, create, update, patch, delete

Resources: deployments, replicasets
Verbs: get, list, watch, update, patch
```

**deploy-controller** (Deployment operations):
```
Resources: deployments
Verbs: get, list, watch, create, update, patch, delete

Resources: services, configmaps
Verbs: get, list, watch, create, update, patch

Resources: pods, pods/log, events, deployments/status
Verbs: get, list, watch
```

### Security Architecture Diagram

```
+------------------------------------------------------------------+
|                         RBAC Boundary                             |
+------------------------------------------------------------------+
|                                                                   |
|  +------------------+        +------------------+                 |
|  | cluster-manager  |        | cicd-controller  |                 |
|  | (read-only)      |        | (build/deploy)   |                 |
|  +--------+---------+        +--------+---------+                 |
|           |                           |                           |
|           v                           v                           |
|  +------------------+        +------------------+                 |
|  | nova, pulse,     |        | kaniko jobs,     |                 |
|  | metrics-dashboard|        | deploy-controller|                 |
|  +------------------+        +------------------+                 |
|                                                                   |
+------------------------------------------------------------------+
|                                                                   |
|  +------------------+        +------------------+                 |
|  | deploy-controller|        | default          |                 |
|  | (deployments)    |        | (minimal access) |                 |
|  +--------+---------+        +--------+---------+                 |
|           |                           |                           |
|           v                           v                           |
|  +------------------+        +------------------+                 |
|  | deployment ops   |        | application pods |                 |
|  +------------------+        +------------------+                 |
|                                                                   |
+------------------------------------------------------------------+
```

### Security Principles

1. **Least Privilege**: Each service account has only the permissions needed for its function
2. **Namespace Isolation**: All HolmOS services run in the `holm` namespace
3. **No External Access**: Cluster is only accessible from the home network
4. **Secret Management**: Vault service handles sensitive credentials
5. **Image Security**: Private registry at localhost:31500 for controlled images

---

## Quick Reference

### Access the System

```bash
# Main entry point
http://192.168.8.197:30000

# SSH to control plane
ssh rpi1@192.168.8.197
```

### Common kubectl Commands

```bash
# View all pods
kubectl get pods -n holm

# View logs
kubectl logs -n holm <pod-name>

# Restart a service
kubectl rollout restart deployment/<service> -n holm

# Scale a service
kubectl scale deployment/<service> --replicas=2 -n holm
```

### Service DNS Names

```
postgres.holm.svc.cluster.local:5432
chat-hub.holm.svc.cluster.local:80
registry.holm.svc.cluster.local:5000
vault.holm.svc.cluster.local:80
```

---

## Version Information

- **K3s Version**: v1.34.3+k3s1
- **Container Runtime**: containerd 2.1.5-k3s1
- **OS**: Debian 13 (trixie) / Debian 12 (bookworm)
- **Kernel**: 6.12.x (Raspberry Pi)
