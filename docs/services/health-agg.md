# Health Aggregator

## Purpose

The Health Aggregator (health-agg) is a centralized health monitoring service that continuously checks the status of all HolmOS services and provides aggregated health information. It serves as the single source of truth for system-wide health status, supporting dashboards, alerting, and operational visibility.

## How It Works

### Service Health Checking
- Maintains a list of all monitored services with their health endpoints
- Performs HTTP GET requests to each service's health endpoint
- Classifies responses based on status code and response time:
  - **Healthy**: 2xx/3xx status with response time under 1 second
  - **Degraded**: 4xx status or slow response (over 1 second)
  - **Unhealthy**: 5xx status or connection failure

### Background Monitoring
- Runs health checks every 30 seconds automatically
- Uses goroutines for parallel service checking
- Caches results between check intervals for fast API responses

### Health Aggregation
- Calculates overall system status based on individual service health
- Tracks critical vs non-critical service failures
- System status rules:
  - **Healthy**: All services healthy
  - **Degraded**: Some services unhealthy/degraded but no critical failures
  - **Unhealthy**: Any critical service failure

### Uptime Tracking
- Maintains a rolling window of 100 health check results per service
- Calculates uptime percentage based on historical checks
- Stores health history for trend analysis (up to 1000 entries)

### Prometheus Metrics
- Exposes metrics in Prometheus format at `/metrics`
- Service availability gauges (`holmos_service_up`)
- Response time metrics (`holmos_service_response_time_ms`)
- Health totals by status (`holmos_health_total`)
- Overall uptime percentage (`holmos_uptime_percent`)

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |

### Monitored Services

The health aggregator monitors these services:

| Service | URL | Critical |
|---------|-----|----------|
| Nova Dashboard | http://192.168.8.197:30004/api/dashboard | Yes |
| Nova Nodes API | http://192.168.8.197:30004/api/nodes | No |
| Nova Pods API | http://192.168.8.197:30004/api/pods | No |
| Cluster Manager | http://192.168.8.197:30502/api/v1/nodes | Yes |
| CI/CD Dashboard | http://192.168.8.197:30020/ | Yes |
| CI/CD Builds API | http://192.168.8.197:30020/api/builds | Yes |
| CI/CD Pipelines | http://192.168.8.197:30020/api/pipelines | Yes |
| HolmGit UI | http://192.168.8.197:30009/ | No |
| HolmGit Repos | http://192.168.8.197:30009/api/repos | Yes |
| Container Registry | http://192.168.8.197:30009/api/registry/repos | No |
| Deploy Controller | http://192.168.8.197:30015/ | No |
| Deployments API | http://192.168.8.197:30015/api/deployments | Yes |
| Scribe Logs | http://192.168.8.197:30017/api/logs | No |
| Backup Jobs | http://192.168.8.197:30016/api/jobs | No |
| App Store | http://192.168.8.197:30002/api/apps | No |
| Metrics API | http://192.168.8.197:30950/api/metrics | No |
| iOS Shell | http://192.168.8.197:30001/ | No |
| Files API | http://192.168.8.197:30088/api/list?path=/ | No |
| Terminal | http://192.168.8.197:30800/ | No |
| Calculator | http://192.168.8.197:30010/ | No |
| Vault | http://192.168.8.197:30870/ | No |
| Steve Bot | http://192.168.8.197:30666/health | No |

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Quick status check |
| `/health` | GET | Cached health data |
| `/api/health` | GET | Cached health data |
| `/api/health/refresh` | GET | Force refresh and return health |
| `/api/health/history` | GET | Historical health data |
| `/metrics` | GET | Prometheus metrics |

### Health Response Structure

```json
{
  "status": "healthy|degraded|unhealthy",
  "totalServices": 22,
  "healthyCount": 20,
  "unhealthyCount": 1,
  "degradedCount": 1,
  "criticalFailures": 0,
  "avgResponseTimeMs": 45.5,
  "uptimePercent": 99.5,
  "timestamp": "2024-01-15T10:30:00Z",
  "services": [
    {
      "name": "Service Name",
      "url": "http://...",
      "status": "healthy",
      "responseTimeMs": 25.3,
      "statusCode": 200,
      "critical": true,
      "lastChecked": "2024-01-15T10:30:00Z"
    }
  ]
}
```

## Dependencies

### Internal Services
- Network access to all monitored services listed above
- Each service should expose a health endpoint that returns 2xx for healthy status

### External Dependencies
- Standard Go HTTP client (net/http)
- 10-second timeout per service check

### Integration Points
- **Prometheus**: Scrapes `/metrics` endpoint for monitoring
- **Alerting Systems**: Can poll `/api/health` for status
- **Dashboards**: Display aggregated health status
