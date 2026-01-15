# HolmOS

A fully web-based operating system running on a 13-node Raspberry Pi Kubernetes cluster with 120+ microservices.

## Architecture

- **13 Raspberry Pi nodes** running Kubernetes
- **120+ microservices** in Go, Python, and Node.js
- **Longhorn** distributed storage (55 pods)
- **Custom HolmGit** for code repository
- **Catppuccin Mocha** theme throughout

## Access Points

| Service | Port | Description |
|---------|------|-------------|
| holmos-shell | 30000 | iPhone-style home screen |
| claude-pod | 30001 | AI chat interface |
| app-store | 30002 | AI-powered app generator |
| chat-hub | 30003 | Unified agent messaging |
| nova | 30004 | Cluster dashboard |
| merchant | 30005 | App store AI agent |
| pulse | 30006 | Metrics monitoring |
| clock-app | 30007 | World clock, alarms |
| gateway | 30008 | API gateway |
| holm-git | 30009 | Git repository |
| cicd-controller | 30020 | CI/CD pipelines |
| file-web-nautilus | 30088 | File manager |
| cluster-manager | 30502 | Admin dashboard |
| settings-web | 30600 | Settings hub |
| audiobook-web | 30700 | Audiobook TTS |
| terminal-web | 30800 | Web terminal |
| backup-dashboard | 30850 | Backup management |
| scribe | 30860 | Log aggregation |
| vault | 30870 | Secret management |
| test-dashboard | 30900 | Health monitoring |
| metrics-dashboard | 30950 | Cluster metrics |
| registry-ui | 31750 | Container registry |

## AI Agents

| Agent | Motto |
|-------|-------|
| Nova | "I see all 13 stars in our constellation" |
| Merchant | "Describe what you need, I'll make it happen" |
| Pulse | "Vital signs are looking good" |
| Gateway | "All roads lead through me" |
| Helix | "Data spirals through my coils" |
| Compass | "I know where everything is" |
| Scribe | "It's all in the records" |
| Vault | "Your secrets are safe with me" |
| Forge | "Another masterpiece from the workshop" |
| Echo | "Your message, delivered" |
| Sentinel | "I'm always watching" |

## CI/CD

GitHub Actions automatically builds and deploys services when changes are pushed to `services/`.

### Manual Deployment

```bash
# Trigger workflow for specific service
gh workflow run build-deploy.yml -f service=holmos-shell
```

## Local Development

```bash
# Clone
git clone https://github.com/timholm/HolmOS.git
cd HolmOS

# Build a service
cd services/holmos-shell
docker build -t holmos-shell .

# Deploy to cluster
kubectl apply -f deployment.yaml
```

## Credentials

- **SSH**: `rpi1@192.168.8.197`
- **Registry**: `10.110.67.87:5000`
- **PostgreSQL**: `postgres.holm.svc.cluster.local`

## License

MIT
