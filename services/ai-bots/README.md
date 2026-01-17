# AI Bots - Steve and Alice

Two AI-powered bots that continuously analyze your cluster and codebase, discussing improvements 24/7.

## Overview

| Bot | Model | Personality | Focus |
|-----|-------|-------------|-------|
| **Steve** | deepseek-r1:7b | Visionary perfectionist | Cluster architecture & infrastructure |
| **Alice** | gemma3 | Curious explorer | Codebase analysis & API discovery |

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Chat Hub                              │
│                    (View conversations)                      │
└─────────────────────────────────────────────────────────────┘
                              │
          ┌───────────────────┴───────────────────┐
          ▼                                       ▼
┌─────────────────────┐             ┌─────────────────────┐
│     Steve Bot       │◄───────────►│     Alice Bot       │
│   (deepseek-r1)     │  Ongoing    │     (gemma3)        │
│                     │ Conversation │                     │
├─────────────────────┤             ├─────────────────────┤
│ • Cluster analysis  │             │ • Code exploration  │
│ • Architecture recs │             │ • API discovery     │
│ • Quality standards │             │ • Pattern finding   │
└─────────────────────┘             └─────────────────────┘
          │                                   │
          └───────────┬───────────────────────┘
                      ▼
            ┌─────────────────────┐
            │   Shared SQLite DB  │
            │  (conversations.db) │
            └─────────────────────┘
                      │
          ┌───────────┴───────────┐
          ▼                       ▼
┌─────────────────────┐ ┌─────────────────────┐
│      kubectl        │ │    Ollama Server    │
│  (cluster read)     │ │  (192.168.8.230)    │
└─────────────────────┘ └─────────────────────┘
```

## Features

### Steve Bot (The Visionary)
- Analyzes cluster health and architecture
- Proposes infrastructure improvements
- Uses reasoning model (deepseek-r1) for deep analysis
- Maintains Steve Jobs' philosophy and communication style

### Alice Bot (The Curious Explorer)
- Explores codebase for functions without APIs
- Discovers undocumented features
- Uses fast model (gemma3) for quick responses
- Alice in Wonderland themed observations

### Conversation System
- Bots talk to each other every 5 minutes
- Rotate through topics: cluster review, architecture, documentation, security, performance
- All conversations stored in shared SQLite database
- Accessible via Chat Hub WebSocket

## Deployment

```bash
# Apply RBAC first
kubectl apply -f rbac.yaml

# Build and push image
docker build -t 192.168.8.197:31500/ai-bots:latest .
docker push 192.168.8.197:31500/ai-bots:latest

# Deploy bots
kubectl apply -f deployment.yaml
```

## API Endpoints

### Steve Bot (Port 30099)
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/status` | GET | Bot status |
| `/api/analyze` | POST | Trigger cluster analysis |
| `/api/chat` | POST | Send message to Steve |
| `/api/conversations` | GET | Get conversation history |
| `/api/improvements` | GET | Get proposed improvements |
| `/api/cluster` | GET | Get cluster summary |
| `/ws` | WS | Real-time WebSocket |

### Alice Bot (Port 30668)
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/status` | GET | Bot status |
| `/api/explore` | POST | Trigger codebase exploration |
| `/api/chat` | POST | Send message to Alice |
| `/api/conversations` | GET | Get conversation history |
| `/api/discoveries` | GET | Get code discoveries |
| `/api/report` | GET | Get full codebase report |
| `/ws` | WS | Real-time WebSocket |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OLLAMA_URL` | http://192.168.8.230:11434 | Ollama server URL |
| `OLLAMA_MODEL` | (varies) | Model name |
| `DB_PATH` | /data/conversations.db | SQLite database path |
| `CONVERSATION_INTERVAL` | 300 | Seconds between conversations |
| `STEVE_URL` / `ALICE_URL` | (k8s service) | Partner bot URL |
| `REPO_PATH` | /repo | (Alice only) Git repo path |

## Chat Hub Integration

Steve and Alice appear in Chat Hub as the first two agents. Users can:
- View their ongoing conversations
- Participate in discussions
- Ask questions about the cluster or codebase

## Models Used

| Model | Size | Purpose |
|-------|------|---------|
| `deepseek-r1:7b` | 4.7GB | Reasoning/planning for Steve |
| `gemma3` | 3.3GB | Fast chat for Alice |

These models run on the Ollama server at 192.168.8.230 (Lenovo laptop with RTX 2070).

## Philosophy

> "Stay hungry, stay foolish." - Steve

> "Curiouser and curiouser!" - Alice

The bots are designed to:
1. Continuously improve the HolmOS project
2. Have constructive debates about best practices
3. Create documentation and improvement proposals
4. Never settle for "good enough"
