# HolmOS Scripts Reference

## Overview

Utility scripts for cluster setup, maintenance, and testing.

## Available Scripts

### scripts/setup-tailscale.sh

Installs and configures Tailscale VPN for secure remote access.

**Location:** `scripts/setup-tailscale.sh`

**Usage:**
```bash
# Basic install
curl -fsSL https://raw.githubusercontent.com/timholm/HolmOS/main/scripts/setup-tailscale.sh | sudo bash

# With auth key (unattended)
AUTH_KEY="tskey-auth-xxx" sudo -E bash scripts/setup-tailscale.sh

# As subnet router
SUBNET_ROUTER=1 sudo -E bash scripts/setup-tailscale.sh

# Custom routes
ADVERTISE_ROUTES="10.42.0.0/16,10.43.0.0/16,192.168.8.0/24" SUBNET_ROUTER=1 sudo -E bash scripts/setup-tailscale.sh
```

**Environment Variables:**
| Variable | Description | Default |
|----------|-------------|---------|
| `AUTH_KEY` | Tailscale auth key for unattended setup | (interactive) |
| `SUBNET_ROUTER` | Enable subnet routing | `0` |
| `ADVERTISE_ROUTES` | Routes to advertise | `10.42.0.0/16,10.43.0.0/16` |

**Supports:**
- Raspberry Pi (arm64)
- Debian/Ubuntu (x86_64)
- Raspbian

**What it does:**
1. Detects OS and architecture
2. Adds Tailscale repository
3. Installs tailscale package
4. Enables IP forwarding
5. Starts tailscaled service
6. Provides authentication instructions

---

### scripts/smoke-test.sh

Runs quick health checks against core services.

**Location:** `scripts/smoke-test.sh`

**Usage:**
```bash
./scripts/smoke-test.sh
```

**Checks:**
- holmos-shell (30000)
- claude-pod (30001)
- chat-hub (30003)
- nova (30004)
- gateway (30008)

---

## Makefile Commands

The `Makefile` provides convenient shortcuts for common operations.

### Cluster Status

```bash
make status     # Show nodes and pods
make health     # Check all services health
make pods       # List all pods in holm namespace
make nodes      # List all nodes with details
make services   # List services with NodePorts
make images     # List images in registry
make top        # Show resource usage
```

### SSH Access

```bash
make ssh        # SSH to control plane (rpi1)
```

Requires: `export PI_PASS="your_password"`

### Service Management

```bash
make deploy S=service-name   # Deploy via GitHub Actions
make build S=service-name    # Build Docker image
make logs S=service-name     # View service logs
make restart S=service-name  # Restart deployment
make shell S=service-name    # Exec into pod
```

### Testing

```bash
make test             # Run all tests
make smoke            # Run smoke tests
make smoke-quick      # Quick single service check
make smoke-degraded   # Allow partial failures
```

### Code Sync

```bash
make sync    # Sync services from cluster to local
make push    # Git add, commit, push
```

---

## Creating New Scripts

### Template

```bash
#!/bin/bash
# Script Name - Brief description
# Usage: ./script-name.sh [options]

set -e  # Exit on error

# Check root if needed
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root"
    exit 1
fi

# Configuration
CLUSTER_IP="${CLUSTER_IP:-192.168.8.197}"
NAMESPACE="${NAMESPACE:-holm}"

# Functions
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

# Main logic
log "Starting script..."

# Your code here

log "Done!"
```

### Best Practices

1. **Use `set -e`** - Exit on errors
2. **Check prerequisites** - Verify required tools exist
3. **Use environment variables** - Allow configuration override
4. **Add logging** - Include timestamps
5. **Document usage** - Add header comments
6. **Make idempotent** - Safe to run multiple times

---

## Environment Variables

### Required for Cluster Access

```bash
export PI_PASS="raspberry_pi_password"
```

### Optional Configuration

```bash
export PI_HOST="192.168.8.197"    # Control plane IP
export PI_USER="rpi1"             # SSH user
export REGISTRY="10.110.67.87:5000"  # Container registry
export NAMESPACE="holm"           # Kubernetes namespace
```

### Setting Permanently

Add to `~/.bashrc` or `~/.zshrc`:

```bash
export PI_PASS="your_password"
```

Or create `.env` file:

```bash
# .env
PI_PASS=your_password
PI_HOST=192.168.8.197
```

And source it:

```bash
source .env
```

---

## Troubleshooting

### Script Permission Denied

```bash
chmod +x scripts/script-name.sh
```

### sshpass Not Found

```bash
# macOS
brew install hudochenkov/sshpass/sshpass

# Ubuntu/Debian
sudo apt-get install sshpass

# Fedora/RHEL
sudo dnf install sshpass
```

### SSH Connection Timeout

Check:
1. Network connectivity to cluster
2. Correct IP address
3. SSH service running on target
4. Firewall rules

### curl/wget Not Found

```bash
# Install curl
sudo apt-get install curl

# Or use wget
wget -qO- http://url
```
