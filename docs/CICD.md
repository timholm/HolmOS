# HolmOS CI/CD Guide

## Overview

HolmOS uses GitHub Actions for continuous integration and deployment. When changes are pushed to `services/`, the workflow automatically:

1. Detects which services changed
2. Builds ARM64 Docker images
3. Connects to the cluster via Tailscale VPN
4. Pushes images to the cluster registry
5. Deploys to Kubernetes

## GitHub Secrets Setup

You must configure the following secrets in your GitHub repository settings.

Go to: **Settings > Secrets and variables > Actions > New repository secret**

### Required Secrets

| Secret | Description | How to Get |
|--------|-------------|------------|
| `TS_OAUTH_CLIENT_ID` | Tailscale OAuth client ID | [Tailscale Admin](https://login.tailscale.com/admin/settings/oauth) |
| `TS_OAUTH_SECRET` | Tailscale OAuth client secret | Same as above |
| `PI_SSH_KEY` | SSH private key for Pi access | Generate with `ssh-keygen` |

### Tailscale OAuth Setup

1. Go to [Tailscale OAuth Clients](https://login.tailscale.com/admin/settings/oauth)
2. Click **Generate OAuth client**
3. Set scopes:
   - `devices:read`
   - `devices:write`
4. Add tag: `tag:ci`
5. Copy the **Client ID** → `TS_OAUTH_CLIENT_ID`
6. Copy the **Client Secret** → `TS_OAUTH_SECRET`

### SSH Key Setup

1. Generate a new SSH key pair:
```bash
ssh-keygen -t ed25519 -f ~/.ssh/holmos_deploy -N ""
```

2. Copy the public key to your Pi:
```bash
ssh-copy-id -i ~/.ssh/holmos_deploy.pub rpi1@192.168.8.197
```

3. Copy the private key content:
```bash
cat ~/.ssh/holmos_deploy
```

4. Add to GitHub Secrets as `PI_SSH_KEY`

### Alternative: Auth Key (Simpler)

Instead of OAuth, you can use a Tailscale auth key:

1. Go to [Tailscale Auth Keys](https://login.tailscale.com/admin/settings/keys)
2. Generate a new key with:
   - Reusable: Yes
   - Ephemeral: Yes
   - Tags: `tag:ci`
3. Add to GitHub Secrets as `TAILSCALE_AUTHKEY`
4. Modify the workflow to use `authkey` instead of OAuth

## Workflows

### deploy.yml

Main deployment workflow.

**Triggers:**
- Push to `main` branch (changes in `services/`)
- Manual trigger via workflow_dispatch

**Inputs:**
- `service`: Service name or "all"
- `skip_build`: Skip build and just redeploy

**Usage:**
```bash
# Deploy specific service
gh workflow run deploy.yml -f service=holmos-shell

# Deploy all services
gh workflow run deploy.yml -f service=all

# Just restart deployments (no rebuild)
gh workflow run deploy.yml -f service=all -f skip_build=true
```

### ci.yml

Runs on every push/PR for validation.

### test.yml

Runs the test suite.

### smoke-test.yml

Quick health checks.

### health.yml

Periodic health monitoring.

### sync.yml

Syncs cluster state to repository.

## Manual Deployment

If the automated pipeline fails:

### Option 1: Download and Deploy Manually

1. Download artifacts from the Actions tab
2. Copy to Pi:
```bash
scp holmos-shell.tar rpi1@192.168.8.197:/tmp/
```
3. Import and deploy:
```bash
ssh rpi1@192.168.8.197
sudo ctr -n k8s.io images import /tmp/holmos-shell.tar
kubectl rollout restart deployment/holmos-shell -n holm
```

### Option 2: Build Locally

```bash
# Build for ARM64
cd services/holmos-shell
docker buildx build --platform linux/arm64 -t 10.110.67.87:5000/holmos-shell:latest .

# Push to registry (requires network access)
docker push 10.110.67.87:5000/holmos-shell:latest

# Deploy
make restart S=holmos-shell
```

## Self-Hosted Runner (Alternative)

For faster deployments without Tailscale, install a GitHub Actions runner on the cluster.

### Install Runner on Pi

```bash
# On the control plane node
cd /home/rpi1
mkdir actions-runner && cd actions-runner

# Download runner (ARM64)
curl -o actions-runner-linux-arm64.tar.gz -L \
  https://github.com/actions/runner/releases/download/v2.311.0/actions-runner-linux-arm64-2.311.0.tar.gz
tar xzf actions-runner-linux-arm64.tar.gz

# Configure (get token from GitHub repo settings > Actions > Runners)
./config.sh --url https://github.com/timholm/HolmOS --token YOUR_TOKEN

# Install as service
sudo ./svc.sh install
sudo ./svc.sh start
```

### Update Workflow for Self-Hosted

Change `runs-on` in the deploy job:

```yaml
deploy:
  runs-on: self-hosted
  # Remove Tailscale setup steps
```

## Troubleshooting

### Tailscale Connection Failed

```bash
# Check Tailscale status in workflow logs
tailscale status

# Verify OAuth client has correct permissions
# Check that tag:ci is approved in ACLs
```

### SSH Connection Failed

```bash
# Verify SSH key is correct
ssh -i ~/.ssh/holmos_deploy rpi1@192.168.8.197

# Check known_hosts
ssh-keyscan 192.168.8.197
```

### Image Push Failed

```bash
# Verify registry is accessible
curl http://10.110.67.87:5000/v2/_catalog

# Check containerd is running
sudo systemctl status containerd
```

### Deployment Not Updating

```bash
# Force pull latest image
kubectl set image deployment/holmos-shell \
  holmos-shell=10.110.67.87:5000/holmos-shell:latest -n holm

# Check image pull policy
kubectl get deployment holmos-shell -n holm -o yaml | grep imagePullPolicy
```

## Monitoring Deployments

### GitHub Actions Dashboard

View at: `https://github.com/timholm/HolmOS/actions`

### Workflow Status Badge

Add to README:
```markdown
![Deploy](https://github.com/timholm/HolmOS/actions/workflows/deploy.yml/badge.svg)
```

### Notifications

Set up notifications in repository settings:
- Email on failure
- Slack integration
- Discord webhook
