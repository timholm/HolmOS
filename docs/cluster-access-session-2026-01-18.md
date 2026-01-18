# Cluster Access Troubleshooting Session

**Date:** 2026-01-18
**Branch:** `claude/check-pod-status-xojjg`
**Objective:** Access the Kubernetes cluster and check pod status

---

## Summary

This document details the complete troubleshooting session attempting to access the HolmOS Pi cluster to check pod status. The session concluded that **direct cluster access is not possible from the CI/CD environment** due to the cluster being on a private network (192.168.8.197).

---

## Tools Used

| Tool | Purpose |
|------|---------|
| `Bash` | Execute shell commands |
| `Read` | Read file contents |
| `Glob` | File pattern matching (implicit via ls) |

---

## Step-by-Step Attempts

### Step 1: Direct kubectl Access

**Command:**
```bash
kubectl get pods --all-namespaces
```

**Result:**
```
Exit code 127
/bin/bash: line 1: kubectl: command not found
```

**Conclusion:** kubectl is not installed in this environment.

---

### Step 2: Repository Exploration

**Commands:**
```bash
ls -la /home/user/HolmOS
which kubectl k3s microk8s || echo "No kubernetes CLI tools found in PATH"
```

**Results:**
- Found repository structure with `k8s/`, `services/`, `scripts/` directories
- Found empty `kubeconfig.yaml` file (0 bytes)
- No Kubernetes CLI tools available in PATH

**Key files discovered:**
- `Makefile` - Contains cluster management commands
- `.github/workflows/` - CI/CD workflows
- `services.yaml` - Service registry with 25+ services

---

### Step 3: Makefile Analysis

**File:** `/home/user/HolmOS/Makefile`

**Key findings:**
- Cluster accessed via SSH to `rpi1@192.168.8.197`
- Uses `sshpass` with `PI_PASS` environment variable
- Registry at `10.110.67.87:5000`

**Available make targets:**
| Target | Description |
|--------|-------------|
| `make status` | Show cluster status |
| `make pods` | List pods in holm namespace |
| `make health` | Quick health check |
| `make ssh` | SSH to control plane |
| `make logs S=<service>` | View service logs |
| `make top` | Resource usage |

---

### Step 4: SSH Access Attempt

**Command:**
```bash
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 rpi1@192.168.8.197 "kubectl get pods -n holm"
```

**Result:**
```
Exit code 127
/bin/bash: line 1: ssh: command not found
```

**Conclusion:** SSH client is not available in this environment.

---

### Step 5: GitHub Workflows Analysis

**Files examined:**
- `.github/workflows/health.yml`
- `.github/workflows/deploy.yml`

**Key findings from `health.yml`:**
- Health check workflow is disabled (Pi on private network)
- Comment: "Pi cluster is on private network, unreachable from GitHub Actions"
- Checks services via HTTP on ports 30000, 30004, 30100, 31750

**Key findings from `deploy.yml`:**
- Deploys via SSH through Mac mini jump host
- Uses secrets: `SSH_PRIVATE_KEY`, `SSH_HOST`, `SSH_PORT`, `SSH_USER`
- Mac mini then SSHes to Pi at 192.168.8.197
- Images built for `linux/arm64` platform

---

### Step 6: Services Registry Review

**File:** `/home/user/HolmOS/services.yaml`

**Services discovered (25+ total):**

| Service | Port | Category |
|---------|------|----------|
| holmos-shell | 30000 | core |
| claude-pod | 30001 | core |
| app-store | 30002 | core |
| chat-hub | 30003 | core |
| nova | 30004 | agent |
| merchant | 30005 | agent |
| pulse | 30006 | agent |
| gateway | 30008 | agent |
| cluster-manager | 30502 | admin |
| test-dashboard | 30900 | monitoring |
| metrics-dashboard | 30950 | monitoring |
| registry-ui | 31750 | devops |

**Namespace:** `holm`

---

### Step 7: curl Availability Check

**Command:**
```bash
curl --version
```

**Result:** curl 8.5.0 available with full feature set (SSL, HTTP2, etc.)

---

### Step 8: Direct HTTP Access Attempts

**Commands:**
```bash
curl -s --connect-timeout 5 http://192.168.8.197:30000/health
curl -s --connect-timeout 5 http://192.168.8.197:30502/health
```

**Result:**
```
Access denied: private_ipv4_blocked
```

**Conclusion:** Private IP addresses are blocked from this environment (security policy).

---

## Network Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    CI/CD Environment                         │
│                  (This session runs here)                    │
│                                                              │
│  ❌ No kubectl    ❌ No SSH    ❌ Private IPs blocked        │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ Cannot reach
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Private Network                          │
│                                                              │
│  ┌─────────────┐         ┌─────────────────────────────┐    │
│  │  Mac mini   │ ──SSH──▶│    Pi Cluster (k3s)         │    │
│  │ (Jump host) │         │    192.168.8.197            │    │
│  │             │         │                             │    │
│  │ Accessible  │         │  ┌─────────────────────┐    │    │
│  │ via ngrok   │         │  │ Namespace: holm     │    │    │
│  │ (secrets)   │         │  │ - 25+ services      │    │    │
│  └─────────────┘         │  │ - Ports 30000-31750 │    │    │
│                          │  └─────────────────────┘    │    │
│                          └─────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

---

## Recommendations for Accessing the Cluster

### Option 1: Local Network Access
If on the same network as the Pi cluster:
```bash
# Set password
export PI_PASS="your_password"

# Use Makefile commands
make pods
make status
make health
```

Or direct SSH:
```bash
ssh rpi1@192.168.8.197
kubectl get pods -n holm
```

### Option 2: GitHub Actions Workflow
```bash
gh workflow run health.yml
```
*Note: This may fail due to private network restrictions.*

### Option 3: Expose via Tunnel
Set up ngrok or similar to expose cluster endpoints publicly for remote access.

---

## Files Referenced

| File | Purpose |
|------|---------|
| `/home/user/HolmOS/Makefile` | Cluster management commands |
| `/home/user/HolmOS/kubeconfig.yaml` | Kubernetes config (empty) |
| `/home/user/HolmOS/services.yaml` | Service registry |
| `/home/user/HolmOS/.github/workflows/health.yml` | Health check workflow |
| `/home/user/HolmOS/.github/workflows/deploy.yml` | Deployment workflow |

---

## Conclusion

**Access Status:** BLOCKED

The HolmOS Pi cluster cannot be accessed from this CI/CD environment because:

1. **No kubectl:** Kubernetes CLI not installed
2. **No SSH:** SSH client not available
3. **Network isolation:** Private IP addresses (192.168.8.x) are blocked
4. **No tunnel configured:** No public endpoint available

To check pod status, you must either:
- Be on the same local network as the cluster
- Use the Mac mini jump host with proper SSH credentials
- Configure a public tunnel (ngrok) to expose cluster endpoints

---

*Generated by Claude Code session on 2026-01-18*
