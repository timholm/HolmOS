# App Store AI

**Location:** `/Users/tim/HolmOS/services/app-store-ai/app.py`
**Service Name:** app-store-ai

## Purpose

App Store AI is a container application marketplace for HolmOS that integrates with Merchant AI (for chat-based app creation) and Forge (for container builds). It provides an iOS App Store-style interface for browsing, deploying, and managing containerized applications.

Key features:
- Browse available apps from the container registry
- Deploy apps to Kubernetes with one click
- Chat with Merchant AI to build new apps
- Monitor Forge/Kaniko build progress
- Track build sessions and history

## Model Used

App Store AI does not run an AI model directly. Instead, it acts as a frontend that communicates with:

- **Merchant AI** (`http://merchant.holm.svc.cluster.local`) - For chat-based app specification and build triggering
- **Forge** (`http://forge.holm.svc.cluster.local`) - For container image building via Kaniko

## API Endpoints

### Core Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Main UI (iOS App Store-style interface) |
| `/health` | GET | Health check |
| `/apps` | GET | List all apps from registry |
| `/api/apps` | GET | Alias for `/apps` |
| `/apps/<name>/deploy` | POST | Deploy an app to Kubernetes |

### Merchant AI Integration

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/merchant/catalog` | GET | Get available templates from Merchant |
| `/merchant/chat` | POST | Chat with Merchant AI to describe apps |
| `/merchant/build` | POST | Trigger a build via Merchant |

### Forge Integration

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/forge/builds` | GET | Get all builds from Forge |
| `/forge/build/<build_id>` | GET | Get specific build status |
| `/forge/trigger` | POST | Trigger a build with app spec |

### Build Session Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/build/session/<session_id>` | GET | Get build session status and history |
| `/build/stream/<session_id>` | GET | SSE stream for build progress |

### Kaniko Jobs

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/kaniko/jobs` | GET | List all Kaniko build jobs |
| `/kaniko/logs/<job_name>` | GET | Get logs for a Kaniko job |

### Request/Response Examples

**POST /apps/myapp/deploy**
```json
// Request
{
  "tag": "latest",
  "port": 8080
}

// Response
{"status": "deployed", "name": "myapp", "message": "...kubectl output..."}
```

**POST /merchant/chat**
```json
// Request
{
  "message": "Build me a todo list app with Flask",
  "session_id": "abc123"
}

// Response
{
  "response": "I'll help you build a todo list app...",
  "build_id": "build-xyz",
  "session_id": "abc123"
}
```

**POST /forge/trigger**
```json
// Request
{
  "name": "my-app",
  "app_code": "from flask import Flask...",
  "dockerfile": "FROM python:3.9...",
  "requirements": "flask>=2.0.0"
}

// Response
{
  "status": "building",
  "app_name": "my-app",
  "forge_response": {...}
}
```

## Conversation Flow

1. **User browses apps** - The UI shows available apps from the container registry
2. **User chats with Merchant** - Via `/merchant/chat`, users describe what they want
3. **Merchant triggers build** - Merchant interprets the request and triggers Forge
4. **Build progress streams** - `/build/stream/<session_id>` provides SSE updates
5. **App appears in store** - Once built, the app shows up in the registry listing
6. **User deploys** - One-click deploy creates Deployment + Service in Kubernetes

## UI Features

The web interface (`/`) provides an iOS App Store-inspired experience:

### Tabs
- **Featured** - Highlighted apps with banner
- **Apps** - Full app catalog grid
- **Installed** - Currently deployed applications
- **Updates** - Available app updates

### Components
- App cards with icons, names, descriptions, and tags
- Deploy modal with tag and port configuration
- Service status indicators (Merchant, Forge, Registry)
- Loading states and empty states

### Theme
Uses Catppuccin color palette with CSS variables for consistent styling.

## Caching Strategy

Apps list is cached for 2 minutes (`APPS_CACHE_TTL = 120`) to handle slow registry queries on the Pi cluster.

A background thread (`apps_cache_warmer`) refreshes the cache every 60 seconds.

## Build Session Tracking

Build sessions are stored in-memory:

```python
build_sessions = {
    "session_id": {
        "messages": [],      # Chat history
        "builds": [],        # Build IDs
        "status": "chatting" # or "building"
    }
}
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `REGISTRY_URL` | `http://registry.holm.svc.cluster.local:5000` | Container registry |
| `MERCHANT_URL` | `http://merchant.holm.svc.cluster.local` | Merchant AI service |
| `FORGE_URL` | `http://forge.holm.svc.cluster.local` | Forge build service |
| `NAMESPACE` | `holm` | Kubernetes namespace for deployments |

## Dependencies

- Flask (web framework)
- requests (HTTP client for Merchant/Forge/Registry)
- kubernetes (via kubectl subprocess calls)
- threading (background caching)
- uuid (session ID generation)
