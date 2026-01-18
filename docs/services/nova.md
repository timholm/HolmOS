# Nova

**Location:** `/Users/tim/HolmOS/services/nova/app.py`
**Service Name:** nova

## Purpose

Nova is the Cluster Guardian - a comprehensive Kubernetes dashboard and management interface for HolmOS. It provides real-time cluster visualization with an animated constellation-themed UI, cluster metrics, and operational controls.

Nova's identity:
- **Name:** Nova
- **Catchphrase:** "I see all 13 stars in our constellation."
- **Role:** Cluster Guardian with full visibility into all nodes and pods

Key features:
- Real-time cluster monitoring dashboard
- Node and pod status visualization
- Deployment scaling and restart capabilities
- Pod log viewing
- Background cache warming for responsive UI on slow Pi clusters

## Model Used

Nova does not use an AI model directly. It is a pure dashboard/API service that provides cluster visibility and management capabilities.

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Main dashboard UI (animated constellation theme) |
| `/health` | GET | Health check endpoint |
| `/capabilities` | GET | Returns list of Nova's capabilities |
| `/api/dashboard` | GET | Full dashboard data (nodes, pods, deployments, metrics) |
| `/api/nodes` | GET | Detailed node information with caching |
| `/api/pods` | GET | Detailed pod information with caching |
| `/api/scale` | POST | Scale a deployment up or down |
| `/api/restart` | POST | Restart a deployment |
| `/api/logs` | POST | Get pod logs |
| `/chat` | POST | Chat endpoint for API compatibility |

### Request/Response Examples

**POST /api/scale**
```json
// Request
{
  "deployment": "my-app",
  "namespace": "holm",
  "replicas": 3
}

// Response
{"status": "success", "message": "Scaled my-app to 3 replicas"}
```

**POST /api/restart**
```json
// Request
{
  "deployment": "my-app",
  "namespace": "holm"
}

// Response
{"status": "success", "message": "Restarted my-app"}
```

**POST /api/logs**
```json
// Request
{
  "pod": "my-app-xyz123",
  "namespace": "holm",
  "tail": 100
}

// Response
{"logs": "...pod log content..."}
```

## Conversation Flow

Nova provides an API-compatible `/chat` endpoint but does not have an autonomous conversation loop like Steve and Alice. It primarily serves as a dashboard and management interface.

The `/chat` endpoint exists for compatibility with other services but returns static capability information rather than AI-generated responses.

## Dashboard Features

The web dashboard (`/`) provides:

1. **Header Section**
   - Nova logo with gradient animation
   - Tagline and status indicators
   - Real-time status pills (nodes, pods, health)

2. **Constellation Background**
   - Animated stars with twinkling effects
   - Shooting star animations
   - Catppuccin Mocha color theme

3. **Main Grid Layout**
   - Node cards with status, resources, and conditions
   - Pod listings with status indicators
   - Deployment controls
   - Cluster metrics summary

4. **Interactive Controls**
   - Scale deployments up/down
   - Restart deployments
   - View pod logs
   - Refresh data

## Caching Strategy

Nova implements aggressive caching for the slow Pi cluster:

| Cache | TTL | Purpose |
|-------|-----|---------|
| Dashboard cache | 60s | Full dashboard data |
| Nodes cache | 60s | Node information |
| Pods cache | 60s | Pod information |

A background thread (`cache_warmer`) runs every 30 seconds to keep caches warm, ensuring instant dashboard loads.

## Kubernetes Integration

Nova uses the official Kubernetes Python client:

```python
from kubernetes import client, config
config.load_incluster_config()  # or load_kube_config()
v1 = client.CoreV1Api()
apps_v1 = client.AppsV1Api()
```

API calls have a 10-second timeout (`API_TIMEOUT_SECONDS`).

## Configuration

Nova uses the Catppuccin Mocha color palette for its UI, defined in the `CATPPUCCIN` dictionary with all standard colors (base, mantle, crust, surface0-2, overlay0-1, text, subtext0-1, and accent colors).

## Dependencies

- Flask (web framework)
- kubernetes (Python client)
- threading (background caching)
- concurrent.futures (parallel data fetching)
