# HolmOS Missing Services Analysis

> **Generated:** January 2026
> **Analyzed by:** Service Gap Analysis Tool
> **Source:** Steve & Alice conversation recommendations, BLUEPRINT.md, SERVICES.md

---

## Summary

This document identifies services that are recommended in the HolmOS architecture but not yet implemented. The analysis compares the planned services from BLUEPRINT.md against the current implementation in the `services/` directory.

**Statistics:**
- Planned Services: 112+
- Implemented Services: ~45
- Missing Critical Services: 8
- Missing Enhancement Services: 25+

---

## Critical Infrastructure Services (Missing)

### 1. Service Mesh / Service Discovery

| Attribute | Value |
|-----------|-------|
| **Priority** | HIGH |
| **Complexity** | High |
| **Planned Name** | service-registry (BLUEPRINT #99) |
| **Status** | Not Implemented |

**Description:** A dedicated service registry for dynamic service discovery. Currently, services use hardcoded Kubernetes DNS names. A proper service mesh would enable:
- Dynamic service registration/deregistration
- Service health tracking beyond simple health checks
- Load balancing policies
- Circuit breaker patterns
- Retry policies

**Dependencies:**
- Gateway service (exists)
- Health-agg service (exists)

**Recommendation:** Consider implementing a lightweight service registry that integrates with the existing gateway, or evaluate Kubernetes-native solutions like Headless Services with custom DNS.

---

### 2. Distributed Tracing

| Attribute | Value |
|-----------|-------|
| **Priority** | HIGH |
| **Complexity** | Medium-High |
| **Planned Name** | tracing (BLUEPRINT #87) |
| **Status** | Not Implemented |

**Description:** The BLUEPRINT mentions a "tracing - Distributed tracing" service. Currently, the gateway adds `X-Request-ID` headers but there's no centralized trace collection or visualization.

**Current State:**
- Gateway adds `X-Request-ID` header (line 401 of gateway/main.go)
- No trace propagation between services
- No trace collection or visualization

**What's Needed:**
- Trace context propagation (W3C Trace Context or B3 headers)
- Trace collector service (could use Jaeger, Zipkin, or custom)
- Trace visualization UI
- Integration with all services

**Dependencies:**
- All services need instrumentation
- Storage backend (PostgreSQL or dedicated)
- Scribe service could be extended

---

### 3. Secrets Rotation

| Attribute | Value |
|-----------|-------|
| **Priority** | HIGH |
| **Complexity** | Medium |
| **Planned Name** | secrets-rotation (part of secret-manager #103) |
| **Status** | Partially Implemented |

**Description:** The Vault service exists for secrets storage (AES-256-GCM encryption), but automatic secrets rotation is not implemented.

**Current State (Vault service):**
- Manual secret creation/update/deletion
- Encrypted at-rest storage
- Audit logging
- NO automatic rotation
- NO expiration tracking
- NO rotation policies

**What's Needed:**
- Rotation scheduler
- Expiration tracking and alerts
- Rotation policies (time-based, event-based)
- Integration with external secret sources
- Rotation notification system

**Dependencies:**
- Vault service (exists)
- Notification-hub service (exists)
- Cron-scheduler service (not implemented)

---

### 4. Rate Limiter Service (Dedicated)

| Attribute | Value |
|-----------|-------|
| **Priority** | MEDIUM |
| **Complexity** | Low |
| **Planned Name** | rate-limiter (BLUEPRINT #95) |
| **Status** | Partially Implemented |

**Description:** Rate limiting exists within the Gateway service but is not a standalone service.

**Current State:**
- Gateway has per-client rate limiting (default 1000/min)
- Per-route rate limits configurable
- In-memory rate tracking (not distributed)

**What's Needed for Full Implementation:**
- Distributed rate limiting (Redis or shared storage)
- Per-user rate limiting (not just per-IP)
- Rate limit quotas (daily/monthly limits)
- Rate limit bypass for internal services
- Rate limit metrics and dashboards
- Configurable rate limit tiers

**Dependencies:**
- Redis or distributed cache (not implemented)
- Auth-gateway integration

---

## High Priority Missing Services

### 5. Config Server (Centralized Configuration)

| Attribute | Value |
|-----------|-------|
| **Priority** | HIGH |
| **Complexity** | Medium |
| **Planned Name** | config-server (BLUEPRINT #100) |
| **Status** | Not Implemented |

**Description:** Centralized configuration management for all services. Currently, each service manages its own configuration via environment variables.

**What's Needed:**
- Centralized config storage
- Config versioning
- Hot reload support
- Environment-specific configs
- Secret injection from Vault

---

### 6. Event Bus (Pub/Sub)

| Attribute | Value |
|-----------|-------|
| **Priority** | HIGH |
| **Complexity** | Medium |
| **Planned Name** | event-bus (BLUEPRINT #101) |
| **Status** | Not Implemented |

**Description:** Currently services communicate via direct HTTP calls. An event bus would enable:
- Async event-driven architecture
- Event replay
- Decoupled services
- Better scalability

**Dependencies:**
- Message queue (in-memory or Redis)

---

### 7. Task Queue (Background Tasks)

| Attribute | Value |
|-----------|-------|
| **Priority** | HIGH |
| **Complexity** | Medium |
| **Planned Name** | task-queue (BLUEPRINT #97) |
| **Status** | Not Implemented |

**Description:** Background task processing system for long-running operations.

**Current Workaround:** CI/CD and Backup use Kubernetes Jobs

**What's Needed:**
- Priority queuing
- Task scheduling
- Retry logic
- Progress tracking
- Dead letter queue

---

### 8. Cron Scheduler

| Attribute | Value |
|-----------|-------|
| **Priority** | HIGH |
| **Complexity** | Low |
| **Planned Name** | cron-scheduler (BLUEPRINT #102) |
| **Status** | Not Implemented |

**Description:** Scheduled job execution service.

**Current Workaround:** Kubernetes CronJobs

**What's Needed:**
- UI for managing schedules
- Job history
- Failure alerting
- Manual trigger capability

---

## Medium Priority Missing Services

### 9. Cache Service (Redis)

| Attribute | Value |
|-----------|-------|
| **Priority** | MEDIUM |
| **Complexity** | Low |
| **Planned Name** | cache-service (BLUEPRINT #96) |
| **Status** | Not Implemented |

**Description:** Centralized caching layer. Currently services use in-memory caching.

---

### 10. WebSocket Hub (Dedicated)

| Attribute | Value |
|-----------|-------|
| **Priority** | MEDIUM |
| **Complexity** | Medium |
| **Planned Name** | websocket-hub (BLUEPRINT #98) |
| **Status** | Partially Implemented |

**Current State:** Multiple services have their own WebSocket endpoints (chat-hub, pulse, terminal-web, gateway)

**What's Needed:** Centralized WebSocket management with routing

---

### 11. Profiler Service

| Attribute | Value |
|-----------|-------|
| **Priority** | MEDIUM |
| **Complexity** | Medium |
| **Planned Name** | profiler (BLUEPRINT #88) |
| **Status** | Not Implemented |

**Description:** Performance profiling for services (CPU, memory, goroutine analysis)

---

### 12. Alerting Service (Dedicated)

| Attribute | Value |
|-----------|-------|
| **Priority** | MEDIUM |
| **Complexity** | Medium |
| **Planned Name** | alerting (BLUEPRINT #84) |
| **Status** | Partially Implemented |

**Current State:** Scribe has alerting rules, metrics-dashboard has alerts

**What's Needed:** Unified alerting service with multiple channels

---

## Missing Application Services

### File Management Extensions

| Service | Priority | Complexity | Status |
|---------|----------|------------|--------|
| file-compress | Medium | Low | Not Implemented |
| file-encrypt | Medium | Low | Not Implemented |
| file-share | Medium | Medium | Not Implemented |
| file-watch | Low | Medium | Not Implemented |
| file-trash | Medium | Low | Exists in file-web-nautilus |
| file-favorites | Low | Low | Exists in file-web-nautilus |
| file-recent | Low | Low | Exists in file-web-nautilus |
| file-tags | Low | Medium | Not Implemented |
| file-preview | Medium | Medium | Exists in file-web-nautilus |

### Notification Extensions

| Service | Priority | Complexity | Status |
|---------|----------|------------|--------|
| notification-email | High | Low | Not Implemented |
| notification-push | Medium | Medium | Not Implemented |
| notification-sms | Low | Medium | Not Implemented |

### Productivity Apps

| Service | Priority | Complexity | Status |
|---------|----------|------------|--------|
| notes-app | Medium | Medium | Not Implemented |
| photos-app | Medium | Medium | Not Implemented |
| music-app | Low | Medium | Not Implemented |
| mail-app | Medium | High | Not Implemented |
| calendar-app | Medium | Medium | Not Implemented |
| contacts-app | Low | Low | Not Implemented |
| reminders-app | Medium | Low | Not Implemented |
| weather-app | Low | Low | Not Implemented |

### HolmOS Shell Components

| Service | Priority | Complexity | Status |
|---------|----------|------------|--------|
| holmos-control-center | Medium | Medium | Not Implemented |
| holmos-notifications | Medium | Medium | Not Implemented |
| holmos-search | Medium | Medium | Not Implemented |
| holmos-switcher | Low | Low | Not Implemented |

---

## Implementation Recommendations

### Phase 1: Critical Infrastructure (2-3 weeks)
1. **Secrets Rotation** - Extend Vault service with rotation capabilities
2. **Distributed Tracing** - Implement trace collection in Scribe, add propagation to Gateway
3. **Config Server** - Centralized configuration with Vault integration

### Phase 2: Event-Driven Architecture (2 weeks)
4. **Event Bus** - In-memory pub/sub with optional Redis backend
5. **Task Queue** - Background job processing with priority support
6. **Cron Scheduler** - Scheduled job management UI

### Phase 3: Enhanced Rate Limiting (1 week)
7. **Rate Limiter Enhancement** - Add distributed rate limiting, per-user limits

### Phase 4: Service Mesh (2-3 weeks)
8. **Service Registry** - Dynamic service discovery with health integration

---

## Steve & Alice Recommendations

Based on the AI agents' analysis patterns, they would likely prioritize:

**Steve (Visionary Architect):**
> "The lack of distributed tracing is inexcusable. How can we ship insanely great products if we can't see the full request journey? This should be our top priority."

**Alice (Code Explorer):**
> "Curiouser and curiouser! I found many services speaking to each other through direct HTTP calls, like doors without hallways. An event bus would make this Wonderland much more navigable!"

---

## Notes

1. **Rate Limiting** is already implemented in the Gateway but could be enhanced with distributed state
2. **Service Discovery** is handled by Kubernetes DNS but lacks dynamic registration
3. Many **file-* microservices** exist as functions within file-web-nautilus rather than separate services
4. The **AI Agents** (Steve, Alice, Nova, Pulse, Scribe) are well implemented
5. **Infrastructure services** (PostgreSQL, Registry, Traefik) use standard Kubernetes deployments

---

*Document generated from analysis of HolmOS codebase and architecture documentation.*
