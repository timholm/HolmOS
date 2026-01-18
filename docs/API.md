# HolmOS API Reference

## Overview

All HolmOS services expose REST APIs accessible via NodePort services on the cluster.

**Base URL:** `http://192.168.8.197:<port>`

## Authentication

Most APIs require authentication via the auth-gateway service.

### Login

```http
POST http://192.168.8.197:30008/auth/login
Content-Type: application/json

{
    "username": "admin",
    "password": "password"
}
```

**Response:**
```json
{
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_at": "2024-01-01T00:00:00Z"
}
```

### Using Token

```http
GET http://192.168.8.197:30004/api/status
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

---

## Common Endpoints

### Health Check

All services implement a health endpoint:

```http
GET /<service>:port/health
```

**Response:**
```json
{
    "status": "ok",
    "service": "service-name",
    "version": "1.0.0"
}
```

---

## Core Services API

### HolmOS Shell (30000)

Main web interface.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Home screen |
| `/health` | GET | Health check |
| `/api/apps` | GET | List installed apps |
| `/api/apps/:id/launch` | POST | Launch app |

### Claude Pod (30001)

AI chat interface.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/chat` | POST | Send message |
| `/api/history` | GET | Get chat history |
| `/ws` | WS | WebSocket chat |

**Chat Request:**
```json
{
    "message": "How do I deploy a new service?",
    "context": {
        "session_id": "abc123"
    }
}
```

### Chat Hub (30003)

Agent message routing.

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/agents` | GET | List agents |
| `/api/message` | POST | Send to agent |
| `/ws` | WS | WebSocket connection |

**Message to Agent:**
```json
{
    "to": "nova",
    "message": "Show cluster status",
    "from": "user"
}
```

---

## AI Agent APIs

### Nova - Cluster Manager (30004)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/status` | GET | Cluster overview |
| `/api/nodes` | GET | List nodes |
| `/api/pods` | GET | List pods |
| `/api/deploy` | POST | Deploy service |

**Cluster Status Response:**
```json
{
    "nodes": {
        "total": 13,
        "healthy": 13
    },
    "pods": {
        "running": 85,
        "pending": 0,
        "failed": 0
    },
    "resources": {
        "cpu_percent": 45,
        "memory_percent": 62
    }
}
```

### Pulse - Metrics (30006)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/metrics` | GET | Current metrics |
| `/api/metrics/history` | GET | Historical data |
| `/api/alerts` | GET | Active alerts |

### Gateway (30008)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/routes` | GET | List routes |
| `/api/stats` | GET | Traffic stats |

### Scribe - Logs (30860)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/logs` | GET | Query logs |
| `/api/logs/stream` | WS | Stream logs |

**Log Query:**
```http
GET /api/logs?service=holmos-shell&level=error&since=1h
```

### Vault - Secrets (30870)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/secrets` | GET | List secrets |
| `/api/secrets/:key` | GET | Get secret |
| `/api/secrets/:key` | PUT | Set secret |
| `/api/secrets/:key` | DELETE | Delete secret |

---

## Application APIs

### File Manager (30088)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/files` | GET | List files |
| `/api/files/upload` | POST | Upload file |
| `/api/files/download/:path` | GET | Download file |
| `/api/files/delete` | DELETE | Delete file |
| `/api/files/move` | POST | Move file |
| `/api/files/copy` | POST | Copy file |
| `/api/files/mkdir` | POST | Create directory |

**List Files:**
```http
GET /api/files?path=/data/documents
```

**Response:**
```json
{
    "path": "/data/documents",
    "files": [
        {
            "name": "report.pdf",
            "type": "file",
            "size": 2456789,
            "modified": "2024-01-01T12:00:00Z"
        },
        {
            "name": "images",
            "type": "directory",
            "modified": "2024-01-01T10:00:00Z"
        }
    ]
}
```

### Terminal (30800)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/hosts` | GET | List SSH hosts |
| `/api/hosts` | POST | Add SSH host |
| `/ws/terminal` | WS | Terminal session |

### Clock App (30007)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/time` | GET | Current time |
| `/api/alarms` | GET | List alarms |
| `/api/alarms` | POST | Create alarm |
| `/api/timers` | GET | List timers |
| `/api/timers` | POST | Create timer |

### Settings (30600)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/settings` | GET | Get all settings |
| `/api/settings/:key` | GET | Get setting |
| `/api/settings/:key` | PUT | Update setting |
| `/api/theme` | GET | Get theme |
| `/api/theme` | PUT | Set theme |

---

## Infrastructure APIs

### CI/CD Controller (30020)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/pipelines` | GET | List pipelines |
| `/api/pipelines/:id` | GET | Pipeline status |
| `/api/pipelines/:id/trigger` | POST | Trigger build |
| `/api/builds` | GET | Recent builds |

### Registry UI (31750)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/repositories` | GET | List repos |
| `/api/repositories/:name/tags` | GET | List tags |
| `/api/repositories/:name/:tag` | DELETE | Delete image |

### Backup Dashboard (30850)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/backups` | GET | List backups |
| `/api/backups` | POST | Create backup |
| `/api/backups/:id/restore` | POST | Restore backup |

---

## WebSocket APIs

### Chat Hub WebSocket

```javascript
const ws = new WebSocket('ws://192.168.8.197:30003/ws');

// Send message
ws.send(JSON.stringify({
    type: 'message',
    to: 'nova',
    content: 'Show status'
}));

// Receive message
ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    console.log(data);
};
```

### Log Stream WebSocket

```javascript
const ws = new WebSocket('ws://192.168.8.197:30860/api/logs/stream?service=holmos-shell');

ws.onmessage = (event) => {
    const log = JSON.parse(event.data);
    console.log(`[${log.level}] ${log.message}`);
};
```

### Terminal WebSocket

```javascript
const ws = new WebSocket('ws://192.168.8.197:30800/ws/terminal');

// Send command
ws.send(JSON.stringify({ type: 'input', data: 'ls -la\n' }));

// Receive output
ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.type === 'output') {
        terminal.write(data.data);
    }
};
```

---

## Error Handling

All APIs return consistent error responses:

```json
{
    "error": {
        "code": "NOT_FOUND",
        "message": "Resource not found",
        "details": {}
    }
}
```

**Common Error Codes:**
| Code | HTTP Status | Description |
|------|-------------|-------------|
| `BAD_REQUEST` | 400 | Invalid request |
| `UNAUTHORIZED` | 401 | Authentication required |
| `FORBIDDEN` | 403 | Permission denied |
| `NOT_FOUND` | 404 | Resource not found |
| `INTERNAL_ERROR` | 500 | Server error |

---

## Rate Limiting

The gateway implements rate limiting:
- **Default:** 100 requests/minute per IP
- **Authenticated:** 1000 requests/minute per user

Rate limit headers:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1704067200
```
