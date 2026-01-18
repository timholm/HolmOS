# HolmOS Development Guide

## Getting Started

### Prerequisites

- Go 1.21+
- Python 3.11+
- Node.js 18+
- Docker with buildx
- kubectl
- SSH access to cluster

### Clone Repository

```bash
git clone https://github.com/timholm/HolmOS.git
cd HolmOS
```

### Environment Setup

Set the Pi password for cluster access:
```bash
export PI_PASS="your_password"
```

## Project Structure

```
HolmOS/
├── docs/               # Documentation
├── planning/           # Architecture blueprints
│   ├── AGENTS.md       # AI agent specifications
│   └── BLUEPRINT.md    # System design
├── scripts/            # Utility scripts
│   ├── setup-tailscale.sh
│   └── smoke-test.sh
├── services/           # All microservices
│   ├── holmos-shell/   # Main UI
│   ├── claude-pod/     # AI chat
│   ├── chat-hub/       # Message routing
│   └── ...             # 55+ services
├── tests/              # Test suites
│   ├── api/            # API tests
│   ├── e2e/            # End-to-end tests
│   ├── integration/    # Integration tests
│   ├── smoke/          # Smoke tests
│   └── performance/    # Performance tests
├── Makefile            # Build commands
├── services.yaml       # Service registry
└── README.md
```

## Service Development

### Creating a New Service

1. Create service directory:
```bash
mkdir -p services/my-service
cd services/my-service
```

2. Choose your language and create the main file:

**Go Service:**
```go
// main.go
package main

import (
    "encoding/json"
    "log"
    "net/http"
)

func main() {
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    })

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Your logic here
    })

    log.Println("Starting my-service on :3000")
    log.Fatal(http.ListenAndServe(":3000", nil))
}
```

**Python Service:**
```python
# app.py
from flask import Flask, jsonify

app = Flask(__name__)

@app.route('/health')
def health():
    return jsonify(status='ok')

@app.route('/')
def index():
    # Your logic here
    return jsonify(message='Hello')

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=3000)
```

**Node.js Service:**
```javascript
// server.js
const express = require('express');
const app = express();

app.get('/health', (req, res) => {
    res.json({ status: 'ok' });
});

app.get('/', (req, res) => {
    // Your logic here
    res.json({ message: 'Hello' });
});

app.listen(3000, () => {
    console.log('Starting my-service on :3000');
});
```

3. Create Dockerfile:
```dockerfile
# For Go
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o main .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/main .
EXPOSE 3000
CMD ["./main"]
```

4. Create deployment.yaml:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  namespace: holm
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-service
  template:
    metadata:
      labels:
        app: my-service
    spec:
      containers:
        - name: my-service
          image: 10.110.67.87:5000/my-service:latest
          ports:
            - containerPort: 3000
          resources:
            requests:
              memory: "64Mi"
              cpu: "50m"
            limits:
              memory: "256Mi"
              cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: holm
spec:
  type: NodePort
  ports:
    - port: 3000
      nodePort: 30XXX  # Choose available port
  selector:
    app: my-service
```

5. Register in services.yaml:
```yaml
my-service:
  port: 30XXX
  replicas: 1
  health: /health
  category: app
  description: My awesome service
```

## Building Services

### Build for ARM64 (Raspberry Pi)

```bash
# Single service
make build S=my-service

# Or manually
cd services/my-service
docker buildx build --platform linux/arm64 -t 10.110.67.87:5000/my-service:latest .
docker push 10.110.67.87:5000/my-service:latest
```

### Deploy to Cluster

```bash
# Via GitHub Actions
make deploy S=my-service

# Or manually via SSH
make ssh
kubectl apply -f /path/to/deployment.yaml
```

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make help` | Show all commands |
| `make status` | Show cluster status |
| `make health` | Check all services health |
| `make ssh` | SSH to control plane |
| `make pods` | List all pods |
| `make nodes` | List all nodes |
| `make services` | List all services with ports |
| `make images` | List registry images |
| `make top` | Show resource usage |
| `make sync` | Sync services from cluster |
| `make build S=name` | Build specific service |
| `make deploy S=name` | Deploy specific service |
| `make logs S=name` | View service logs |
| `make restart S=name` | Restart service |
| `make shell S=name` | Exec into service pod |

## Code Style

### Go
- Use `gofmt` for formatting
- Follow standard Go project layout
- Use meaningful variable names
- Add health endpoints to all services

### Python
- Use `black` for formatting
- Type hints encouraged
- Flask for web services
- Use requirements.txt or pyproject.toml

### Node.js
- Use ESLint + Prettier
- Express.js for web services
- Use package.json for dependencies

## UI Development

### Theme: Catppuccin Mocha

All UIs use the Catppuccin Mocha color palette:

```css
:root {
    --base: #1e1e2e;
    --mantle: #181825;
    --crust: #11111b;
    --text: #cdd6f4;
    --subtext0: #a6adc8;
    --surface0: #313244;
    --surface1: #45475a;
    --blue: #89b4fa;
    --green: #a6e3a1;
    --red: #f38ba8;
    --yellow: #f9e2af;
    --pink: #f5c2e7;
    --mauve: #cba6f7;
}
```

### Mobile-First Design

- Design for iPhone dimensions first (375x812)
- Use responsive breakpoints for larger screens
- Touch-friendly targets (44px minimum)
- Gesture support where applicable

## Git Workflow

### Branches
- `main` - Production code
- `claude/*` - Claude Code feature branches
- `feature/*` - Manual feature branches

### Commit Messages
Follow conventional commits:
```
feat: Add new password manager service
fix: Resolve memory leak in file-upload
docs: Update API documentation
refactor: Simplify gateway routing logic
```

### Pull Requests
1. Create feature branch
2. Make changes
3. Run tests: `make test`
4. Push and create PR
5. Auto-deploy on merge

## CI/CD

GitHub Actions automatically:
1. Builds changed services
2. Pushes to container registry
3. Deploys to cluster

### Manual Deployment
```bash
gh workflow run build-deploy.yml -f service=my-service
```

## Debugging

### View Logs
```bash
make logs S=my-service
```

### Exec into Pod
```bash
make shell S=my-service
```

### Check Pod Status
```bash
make ssh
kubectl describe pod -n holm -l app=my-service
```

### Port Forward for Local Testing
```bash
make ssh
kubectl port-forward -n holm svc/my-service 3000:3000 &
```

## Common Patterns

### Health Check Endpoint
Every service must have `/health`:
```go
func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "status": "ok",
        "service": "my-service",
    })
}
```

### Database Connection
```go
dsn := "host=postgres.holm.svc.cluster.local user=postgres password=xxx dbname=holmos"
db, err := sql.Open("postgres", dsn)
```

### Inter-Service Communication
```go
resp, err := http.Get("http://other-service.holm.svc.cluster.local:3000/api/data")
```

### WebSocket for Real-time
```javascript
const ws = new WebSocket('ws://chat-hub.holm.svc.cluster.local:3000/ws');
ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    // Handle message
};
```
