# HolmOS Documentation

Welcome to the HolmOS documentation. HolmOS is a fully web-based operating system running on a 13-node Raspberry Pi Kubernetes cluster.

## Quick Links

| Document | Description |
|----------|-------------|
| [Architecture](ARCHITECTURE.md) | System design and cluster topology |
| [Services](SERVICES.md) | Complete service reference |
| [Development](DEVELOPMENT.md) | How to build and develop services |
| [Deployment](DEPLOYMENT.md) | How to deploy to the cluster |
| [Testing](TESTING.md) | Testing strategy and commands |
| [Scripts](SCRIPTS.md) | Utility scripts reference |
| [API](API.md) | REST API documentation |

## Additional Resources

| Document | Location | Description |
|----------|----------|-------------|
| [Blueprint](../planning/BLUEPRINT.md) | planning/ | Original system design |
| [Agents](../planning/AGENTS.md) | planning/ | AI agent specifications |
| [Service Registry](../services.yaml) | root | Service configuration |
| [Tailscale Setup](../services/tailscale/README.md) | services/ | VPN setup guide |

## Getting Started

### 1. Clone the Repository

```bash
git clone https://github.com/timholm/HolmOS.git
cd HolmOS
```

### 2. Set Up Access

```bash
# Set password for cluster access
export PI_PASS="your_password"

# Or use Tailscale for remote access
kubectl apply -f services/tailscale/deployment.yaml
```

### 3. Check Cluster Status

```bash
make status
make health
```

### 4. Access the UI

Open in browser: `http://192.168.8.197:30000`

## Key Concepts

### Services
HolmOS consists of 120+ microservices written in Go, Python, and Node.js. Each service runs as a Kubernetes deployment with a NodePort service for access.

### AI Agents
Each major service has an AI agent with a unique personality. Agents can be chatted with via the chat-hub service to manage their respective domains.

### iPhone-Style UI
The main interface (holmos-shell) provides an iOS-like experience with app icons, dock, status bar, and gesture navigation.

### Distributed Storage
Longhorn provides replicated storage across all nodes for high availability.

## Common Tasks

| Task | Command |
|------|---------|
| View cluster status | `make status` |
| Check service health | `make health` |
| SSH to cluster | `make ssh` |
| View service logs | `make logs S=service-name` |
| Deploy service | `make deploy S=service-name` |
| Run tests | `make test` |
| Run smoke tests | `make smoke` |

## Support

- GitHub Issues: [github.com/timholm/HolmOS/issues](https://github.com/timholm/HolmOS/issues)
- Chat with Claude: Access via claude-pod (port 30001)

## License

MIT
