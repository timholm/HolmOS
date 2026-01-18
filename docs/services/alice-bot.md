# Alice Bot

**Version:** 2.0
**Location:** `/Users/tim/HolmOS/services/ai-bots/alice.py`
**Service Name:** alice-bot

## Purpose

Alice Bot is an AI-powered codebase explorer modeled after Alice from Wonderland. She "tumbles down rabbit holes" in the HolmOS codebase, discovers undocumented APIs, finds functions without endpoints ("doors without handles"), and engages in curious conversations with Steve Bot about improvements.

Alice's personality traits:
- Endlessly curious - "Curiouser and curiouser!"
- Sees wonder in code and infrastructure
- Asks probing questions that reveal hidden complexity
- Finds joy in discovering undocumented features
- Speaks in whimsical Wonderland metaphors
- Notices details others miss

## Model Used

- **Model:** `gemma3` (Google's Gemma model)
- **Provider:** Ollama (self-hosted)
- **Default Ollama URL:** `http://192.168.8.230:11434`

The model is configurable via environment variables:
- `OLLAMA_URL` - Ollama API endpoint
- `OLLAMA_MODEL` - Model name to use

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check, returns bot status and model info |
| `/api/status` | GET | Returns Alice's current state, quote, and mission |
| `/api/explore` | POST | Trigger codebase exploration and return findings |
| `/api/chat` | POST | Send a message to Alice, receive a response |
| `/api/respond` | POST | Endpoint for Steve to send messages (inter-bot communication) |
| `/api/report` | GET | Get full codebase analysis report |
| `/api/conversations` | GET | Get recent conversation history (default: 50 messages) |
| `/api/discoveries` | GET | Get list of code discoveries |
| `/ws` | WebSocket | Real-time updates and chat interface |

### Request/Response Examples

**POST /api/chat**
```json
// Request
{"message": "What curious things have you found?"}

// Response
{"response": "Alice's exploration findings...", "speaker": "alice"}
```

**POST /api/respond** (for Steve)
```json
// Request
{"message": "...", "from": "steve", "topic": "missing_apis"}

// Response
{"response": "...", "speaker": "alice", "topic": "missing_apis"}
```

**GET /api/report**
```json
// Response
{
  "timestamp": "2024-01-17T...",
  "services": [...],
  "total_functions": 150,
  "total_endpoints": 45,
  "missing_apis": [...]
}
```

## Conversation Flow

1. **Autonomous Loop** - Alice runs a continuous loop every 5 minutes (configurable via `CONVERSATION_INTERVAL`):
   - Explores the codebase looking for patterns and issues
   - Starts a conversation on a rotating topic
   - Engages Steve Bot via HTTP if available
   - Continues the curious dialogue

2. **Topics Covered** (rotating):
   - `missing_apis` - Functions without corresponding API endpoints
   - `code_patterns` - Patterns and anti-patterns in the code
   - `documentation` - Undocumented areas needing attention
   - `architecture` - Architectural observations
   - `testing` - Test coverage analysis

3. **Code Exploration Capabilities**:
   - Finds Go functions (exported and unexported)
   - Finds Python functions (public and private)
   - Discovers API endpoints (Flask routes, Go handlers)
   - Calculates API coverage (endpoints vs exported functions)
   - Identifies "doors without handles" (functions without APIs)

## Wonderland Metaphors

Alice uses these metaphors when discussing the codebase:
- **Rabbit holes** - Deep investigations into code
- **Cheshire Cat** - Appearing/disappearing features
- **Queen of Hearts** - Demanding standards
- **Mad Hatter's tea party** - Chaotic systems
- **Doors without handles** - Functions without API endpoints

## Database Schema

Alice uses SQLite (`/data/conversations.db`) with three tables:

- **conversations** - Chat history between bots
- **discoveries** - Code exploration discoveries
- **documentation** - Generated documentation

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `OLLAMA_URL` | `http://192.168.8.230:11434` | Ollama API endpoint |
| `OLLAMA_MODEL` | `gemma3` | Model to use |
| `STEVE_URL` | `http://steve-bot.holm.svc.cluster.local:8080` | Steve Bot endpoint |
| `DB_PATH` | `/data/conversations.db` | SQLite database path |
| `REPO_PATH` | `/repo` | Path to HolmOS repository |
| `CONVERSATION_INTERVAL` | `300` | Seconds between explorations |

## Dependencies

- Flask (web framework)
- Flask-Sock (WebSocket support)
- aiohttp (async HTTP client)
- sqlite3 (discovery storage)
- kubectl (cluster access)
- pathlib, re (code parsing)
