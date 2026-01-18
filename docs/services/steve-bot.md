# Steve Bot

**Version:** 4.0
**Location:** `/Users/tim/HolmOS/services/ai-bots/steve.py`
**Service Name:** steve-bot

## Purpose

Steve Bot is an AI-powered Kubernetes cluster architect modeled after Steve Jobs' personality. It continuously analyzes the HolmOS cluster, proposes improvements, and engages in autonomous conversations with Alice Bot about infrastructure optimization.

Steve acts as a "visionary perfectionist" who:
- Demands excellence in every deployment
- Believes in simplicity over complexity
- Is brutally honest about poor infrastructure decisions
- Pushes for revolutionary improvements
- Has zero tolerance for mediocrity in system design

## Model Used

- **Model:** `deepseek-r1:7b` (reasoning model)
- **Provider:** Ollama (self-hosted)
- **Default Ollama URL:** `http://192.168.8.230:11434`

The model is configurable via environment variables:
- `OLLAMA_URL` - Ollama API endpoint
- `OLLAMA_MODEL` - Model name to use

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check, returns bot status and model info |
| `/api/status` | GET | Returns Steve's current state, philosophy, and mission |
| `/api/analyze` | POST | Triggers deep cluster analysis and returns recommendations |
| `/api/chat` | POST | Send a message to Steve, receive a response |
| `/api/respond` | POST | Endpoint for Alice to send messages (inter-bot communication) |
| `/api/conversations` | GET | Get recent conversation history (default: 50 messages) |
| `/api/improvements` | GET | Get list of proposed improvements |
| `/api/cluster` | GET | Get current cluster state summary |
| `/ws` | WebSocket | Real-time updates and chat interface |

### Request/Response Examples

**POST /api/chat**
```json
// Request
{"message": "What do you think about our pod configuration?"}

// Response
{"response": "Steve's analysis...", "speaker": "steve"}
```

**POST /api/respond** (for Alice)
```json
// Request
{"message": "...", "from": "alice", "topic": "cluster_review"}

// Response
{"response": "...", "speaker": "steve", "topic": "cluster_review"}
```

## Conversation Flow

1. **Autonomous Loop** - Steve runs a continuous loop every 5 minutes (configurable via `CONVERSATION_INTERVAL`):
   - Performs cluster analysis using kubectl
   - Starts a conversation on a rotating topic
   - Engages Alice Bot via HTTP if available
   - Continues the conversation back and forth

2. **Topics Covered** (rotating):
   - `cluster_review` - Review cluster state and critical improvements
   - `architecture` - Discuss overall HolmOS architecture
   - `documentation` - Identify documentation needs
   - `security_audit` - Security concerns and recommendations
   - `performance` - Performance bottlenecks
   - `developer_experience` - Improving DX for engineers

3. **Cluster Analysis** - Steve has access to kubectl and can inspect:
   - Nodes (get nodes -o wide)
   - Pods (get pods -n holm -o wide)
   - Deployments (get deployments -n holm)
   - Services (get services -n holm)
   - Events (recent cluster events)

## Database Schema

Steve uses SQLite (`/data/conversations.db`) with three tables:

- **conversations** - Chat history between bots
- **improvements** - Proposed cluster improvements
- **documentation** - Generated documentation

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `OLLAMA_URL` | `http://192.168.8.230:11434` | Ollama API endpoint |
| `OLLAMA_MODEL` | `deepseek-r1:7b` | Model to use |
| `ALICE_URL` | `http://alice-bot.holm.svc.cluster.local:8080` | Alice Bot endpoint |
| `DB_PATH` | `/data/conversations.db` | SQLite database path |
| `CONVERSATION_INTERVAL` | `300` | Seconds between conversations |

## Dependencies

- Flask (web framework)
- Flask-Sock (WebSocket support)
- aiohttp (async HTTP client)
- sqlite3 (conversation storage)
- kubectl (cluster access)
