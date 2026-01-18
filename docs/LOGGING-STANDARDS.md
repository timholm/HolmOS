# HolmOS Logging Standards

This document defines the logging standards for all HolmOS services to ensure consistent, queryable, and actionable logs across the platform.

## Log Format

All services MUST use **JSON structured logging**. This enables efficient log aggregation, searching, and analysis in tools like Elasticsearch, Grafana Loki, or CloudWatch.

### Required Fields

Every log entry MUST include the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `timestamp` | string (ISO 8601) | When the event occurred (e.g., `2025-01-17T14:32:00.123Z`) |
| `service` | string | Name of the service emitting the log (e.g., `auth-gateway`, `deploy-controller`) |
| `level` | string | Log level: `ERROR`, `WARN`, `INFO`, `DEBUG` |
| `message` | string | Human-readable description of the event |
| `traceId` | string | Unique identifier for request tracing across services (UUID or similar) |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `requestId` | string | HTTP request identifier |
| `userId` | string | User ID if authenticated |
| `duration` | number | Operation duration in milliseconds |
| `error` | object | Error details (code, stack trace) |
| `metadata` | object | Additional context-specific data |

### Example Log Entry

```json
{
  "timestamp": "2025-01-17T14:32:00.123Z",
  "service": "deploy-controller",
  "level": "INFO",
  "message": "Deployment completed successfully",
  "traceId": "550e8400-e29b-41d4-a716-446655440000",
  "metadata": {
    "namespace": "default",
    "deployment": "my-app",
    "image": "registry.holmos.local/my-app:v1.2.3",
    "duration": 4523
  }
}
```

## Log Levels

Use log levels consistently across all services:

| Level | When to Use |
|-------|-------------|
| `ERROR` | Operation failed, requires attention. Include error details and context for debugging. |
| `WARN` | Unexpected condition that does not prevent operation but may indicate a problem. |
| `INFO` | Normal operational events: startup, shutdown, configuration, successful operations. |
| `DEBUG` | Detailed diagnostic information for troubleshooting. Disabled in production by default. |

### Level Guidelines

- **ERROR**: Database connection failures, API errors, unhandled exceptions, critical business logic failures
- **WARN**: Deprecated API usage, retry attempts, fallback to defaults, approaching resource limits
- **INFO**: Service startup/shutdown, request completion, configuration loaded, scheduled job execution
- **DEBUG**: Request/response bodies, SQL queries, cache hits/misses, detailed execution flow

## Implementation Examples

### Node.js (with winston)

```javascript
const winston = require('winston');
const { v4: uuidv4 } = require('uuid');

const logger = winston.createLogger({
  level: process.env.LOG_LEVEL || 'info',
  format: winston.format.combine(
    winston.format.timestamp(),
    winston.format.json()
  ),
  defaultMeta: { service: 'claude-pod' },
  transports: [
    new winston.transports.Console()
  ]
});

// Middleware to add traceId
function traceMiddleware(req, res, next) {
  req.traceId = req.headers['x-trace-id'] || uuidv4();
  res.setHeader('x-trace-id', req.traceId);
  next();
}

// Usage
function logInfo(message, traceId, metadata = {}) {
  logger.info({
    message,
    traceId,
    ...metadata
  });
}

function logError(message, traceId, error, metadata = {}) {
  logger.error({
    message,
    traceId,
    error: {
      name: error.name,
      message: error.message,
      stack: error.stack
    },
    ...metadata
  });
}

// Example usage
app.get('/api/conversations', traceMiddleware, async (req, res) => {
  const start = Date.now();
  try {
    const conversations = await getConversations();
    logInfo('Fetched conversations', req.traceId, {
      count: conversations.length,
      duration: Date.now() - start
    });
    res.json(conversations);
  } catch (error) {
    logError('Failed to fetch conversations', req.traceId, error);
    res.status(500).json({ error: 'Internal server error' });
  }
});
```

### Python (with structlog)

```python
import structlog
import uuid
from datetime import datetime
from functools import wraps

# Configure structlog for JSON output
structlog.configure(
    processors=[
        structlog.stdlib.add_log_level,
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.JSONRenderer()
    ],
    wrapper_class=structlog.stdlib.BoundLogger,
    context_class=dict,
    logger_factory=structlog.stdlib.LoggerFactory(),
)

SERVICE_NAME = "backup"

def get_logger(trace_id=None):
    """Get a logger bound with service name and trace ID."""
    return structlog.get_logger().bind(
        service=SERVICE_NAME,
        traceId=trace_id or str(uuid.uuid4())
    )

# Flask middleware example
def trace_middleware(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        trace_id = request.headers.get('X-Trace-Id', str(uuid.uuid4()))
        g.trace_id = trace_id
        g.logger = get_logger(trace_id)
        response = f(*args, **kwargs)
        response.headers['X-Trace-Id'] = trace_id
        return response
    return decorated

# Usage
@app.route('/api/backup', methods=['POST'])
@trace_middleware
def create_backup():
    start = datetime.now()
    try:
        job = create_backup_job(request.json)
        g.logger.info(
            "Backup job created",
            job_id=job.id,
            duration_ms=(datetime.now() - start).total_seconds() * 1000
        )
        return jsonify(job.to_dict())
    except Exception as e:
        g.logger.error(
            "Failed to create backup job",
            error=str(e),
            error_type=type(e).__name__
        )
        return jsonify({"error": "Internal server error"}), 500
```

### Go (with zerolog)

```go
package main

import (
    "net/http"
    "os"
    "time"

    "github.com/google/uuid"
    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
)

const serviceName = "deploy-controller"

func init() {
    // Configure zerolog for JSON output
    zerolog.TimeFieldFormat = time.RFC3339Nano
    log.Logger = zerolog.New(os.Stdout).With().
        Str("service", serviceName).
        Timestamp().
        Logger()
}

// Middleware to handle trace ID
func traceMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        traceID := r.Header.Get("X-Trace-Id")
        if traceID == "" {
            traceID = uuid.New().String()
        }
        w.Header().Set("X-Trace-Id", traceID)

        // Add trace ID to request context
        ctx := r.Context()
        logger := log.With().Str("traceId", traceID).Logger()
        ctx = logger.WithContext(ctx)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// Usage in handlers
func deployHandler(w http.ResponseWriter, r *http.Request) {
    logger := zerolog.Ctx(r.Context())
    start := time.Now()

    deployment, err := performDeploy(r)
    if err != nil {
        logger.Error().
            Err(err).
            Str("namespace", deployment.Namespace).
            Str("name", deployment.Name).
            Msg("Deployment failed")
        http.Error(w, "Deployment failed", http.StatusInternalServerError)
        return
    }

    logger.Info().
        Str("namespace", deployment.Namespace).
        Str("name", deployment.Name).
        Str("image", deployment.Image).
        Dur("duration", time.Since(start)).
        Msg("Deployment completed successfully")

    w.WriteHeader(http.StatusOK)
}

func main() {
    log.Info().Msg("Deploy Controller starting")

    mux := http.NewServeMux()
    mux.HandleFunc("/deploy", deployHandler)

    handler := traceMiddleware(mux)

    log.Info().Str("port", "8080").Msg("Server listening")
    if err := http.ListenAndServe(":8080", handler); err != nil {
        log.Fatal().Err(err).Msg("Server failed")
    }
}
```

## Best Practices

### DO

- Include enough context to understand the event without reading code
- Use consistent field names across all services
- Propagate trace IDs across service boundaries via headers
- Log at appropriate levels (not everything is INFO)
- Include timing information for performance analysis
- Sanitize sensitive data (passwords, tokens, PII)

### DO NOT

- Log sensitive information (passwords, API keys, tokens, PII)
- Use string interpolation for log messages (use structured fields)
- Log entire request/response bodies in production
- Use `fmt.Print` or `console.log` directly (use the logging library)
- Ignore errors silently

### Sensitive Data Handling

Never log the following:
- Passwords or password hashes
- API keys or tokens (including JWTs)
- Credit card numbers
- Social security numbers
- Personal health information
- Full email addresses (mask if needed)

Example of masking:
```json
{
  "message": "User authenticated",
  "userId": "user-123",
  "email": "j***@example.com"
}
```

## Configuration

All services should support the following environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_LEVEL` | `info` | Minimum log level to output |
| `LOG_FORMAT` | `json` | Output format (`json` or `text` for local dev) |

## Trace ID Propagation

When making requests to other services, always forward the trace ID:

```javascript
// Node.js example
const response = await fetch(`${SERVICE_URL}/api/resource`, {
  headers: {
    'X-Trace-Id': traceId,
    'Content-Type': 'application/json'
  }
});
```

```python
# Python example
response = requests.get(
    f"{SERVICE_URL}/api/resource",
    headers={"X-Trace-Id": trace_id}
)
```

```go
// Go example
req, _ := http.NewRequest("GET", serviceURL+"/api/resource", nil)
req.Header.Set("X-Trace-Id", traceID)
```

## Migration Guide

For services currently using unstructured logging:

1. Add the appropriate logging library (winston, structlog, zerolog)
2. Configure JSON output format
3. Replace `console.log`/`print`/`fmt.Print` with structured logger calls
4. Add trace middleware to HTTP handlers
5. Update all log statements to include required fields
6. Test in development with `LOG_FORMAT=text` for readability

## Monitoring Integration

Structured logs enable powerful queries in log aggregation tools:

```
# Find all errors for a specific trace
traceId:"550e8400-e29b-41d4-a716-446655440000" AND level:"ERROR"

# Find slow requests (>1s) in deploy-controller
service:"deploy-controller" AND duration:>1000

# Find all deployment failures in the last hour
service:"deploy-controller" AND level:"ERROR" AND message:"Deployment failed"
```
