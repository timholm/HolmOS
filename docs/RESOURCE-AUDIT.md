# Kubernetes Resource Limits Audit

**Audit Date:** 2026-01-17
**Total Deployments Scanned:** 68
**Services with Complete Resource Limits:** 54
**Services Missing Resource Limits:** 14

## Summary

This audit examines all deployment.yaml files in `/Users/tim/HolmOS/services/` for proper resource requests and limits configuration.

### Status Legend
- **Complete**: All four resource fields defined (memory request, memory limit, CPU request, CPU limit)
- **Missing**: One or more resource fields not defined

---

## Resource Configuration Table

| Service | Memory Req | Memory Limit | CPU Req | CPU Limit | Status |
|---------|------------|--------------|---------|-----------|--------|
| ai-bots (steve-bot) | 128Mi | 512Mi | 100m | 1000m | Complete |
| ai-bots (alice-bot) | 128Mi | 512Mi | 100m | 1000m | Complete |
| app-store-ai | 64Mi | 256Mi | 50m | 500m | Complete |
| audiobook-audio-normalize | 128Mi | 512Mi | 100m | 500m | Complete |
| audiobook-upload-epub | 64Mi | 256Mi | 50m | 500m | Complete |
| audiobook-web | 64Mi | 256Mi | 50m | 500m | Complete |
| auth-gateway | 64Mi | 256Mi | 50m | 500m | Complete |
| backup-dashboard | 64Mi | 256Mi | 50m | 500m | Complete |
| backup-service | 64Mi | 256Mi | 50m | 500m | Complete |
| backup-storage | 64Mi | 256Mi | 50m | 200m | Complete |
| calculator-app | 64Mi | 256Mi | 50m | 500m | Complete |
| chat-hub | 64Mi | 256Mi | 50m | 500m | Complete |
| cicd-controller | 128Mi | 512Mi | 50m | 500m | Complete |
| claude-pod | 64Mi | 256Mi | 50m | 500m | Complete |
| claude-terminal (merchant) | 64Mi | 256Mi | 50m | 500m | Complete |
| clock-app | 64Mi | 256Mi | 50m | 500m | Complete |
| cluster-manager | 64Mi | 256Mi | 50m | 500m | Complete |
| config-sync | 32Mi | 128Mi | 25m | 100m | Complete |
| deploy-controller | 64Mi | 256Mi | 50m | 500m | Complete |
| deploy-controller (devops) | 128Mi | 256Mi | 100m | 500m | Complete |
| event-broker | 64Mi | 256Mi | 50m | 200m | Complete |
| event-dlq | 64Mi | 256Mi | 50m | 200m | Complete |
| event-persist | 64Mi | 256Mi | 50m | 200m | Complete |
| event-replay | 64Mi | 256Mi | 50m | 200m | Complete |
| file-compress | 64Mi | 512Mi | 50m | 500m | Complete |
| file-convert | 128Mi | 512Mi | 100m | 500m | Complete |
| file-copy | - | - | - | - | **Missing** |
| file-decompress | 64Mi | 256Mi | 100m | 500m | Complete |
| file-delete | - | - | - | - | **Missing** |
| file-download | - | - | - | - | **Missing** |
| file-encrypt | 64Mi | 256Mi | 50m | 200m | Complete |
| file-meta | - | - | - | - | **Missing** |
| file-mkdir | - | - | - | - | **Missing** |
| file-move | - | - | - | - | **Missing** |
| file-permissions | 32Mi | 128Mi | 25m | 100m | Complete |
| file-preview | 64Mi | 256Mi | 50m | 200m | Complete |
| file-search | - | - | - | - | **Missing** |
| file-share-create | 32Mi | 128Mi | 25m | 100m | Complete |
| file-share-validate | 32Mi | 256Mi | 25m | 200m | Complete |
| file-thumbnail | 128Mi | 512Mi | 100m | 500m | Complete |
| file-thumbnail (files/) | 128Mi | 512Mi | 100m | 500m | Complete |
| file-upload | - | - | - | - | **Missing** |
| file-watch | 64Mi | 256Mi | 50m | 200m | Complete |
| file-web-nautilus | 64Mi | 256Mi | 50m | 500m | Complete |
| gateway | 64Mi | 256Mi | 50m | 500m | Complete |
| gitea | 256Mi | 512Mi | 100m | 500m | Complete |
| health-aggregator | 32Mi | 128Mi | 25m | 100m | Complete |
| holm-cli | - | - | - | - | **Missing** |
| holm-git | 128Mi | 256Mi | 50m | 200m | Complete |
| holm-git (holm-git-build) | 128Mi | 256Mi | 50m | 200m | Complete |
| holmos-shell | 64Mi | 256Mi | 50m | 500m | Complete |
| ios-shell | 64Mi | 128Mi | 50m | 200m | Complete |
| metrics-collector | 64Mi | 128Mi | 50m | 200m | Complete |
| metrics-dashboard | 64Mi | 256Mi | 50m | 500m | Complete |
| notification-hub | 64Mi | 256Mi | 50m | 500m | Complete |
| notification-queue | 64Mi | 128Mi | 50m | 200m | Complete |
| notification-webhook | 64Mi | 128Mi | 50m | 200m | Complete |
| nova | 64Mi | 256Mi | 50m | 500m | Complete |
| pulse | 64Mi | 256Mi | 50m | 200m | Complete |
| pxe-server | - | - | - | - | **Missing** |
| registry-ui | 64Mi | 256Mi | 50m | 500m | Complete |
| scribe | 64Mi | 256Mi | 50m | 500m | Complete |
| settings-restore | 32Mi | 64Mi | 25m | 100m | Complete |
| settings-web | 64Mi | 256Mi | 50m | 500m | Complete |
| steve-bot (standalone) | 64Mi | 256Mi | 50m | 500m | Complete |
| task-queue | 64Mi | 128Mi | 50m | 200m | Complete |
| terminal-web | 64Mi | 256Mi | 50m | 500m | Complete |
| test-dashboard | 64Mi | 256Mi | 50m | 500m | Complete |
| user-preferences | 64Mi | 128Mi | 50m | 200m | Complete |
| vault | 64Mi | 256Mi | 50m | 500m | Complete |

---

## Services Missing Resource Limits

The following 14 services have no resource limits defined:

### 1. file-copy
**Path:** `/Users/tim/HolmOS/services/file-copy/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

### 2. file-delete
**Path:** `/Users/tim/HolmOS/services/file-delete/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

### 3. file-download
**Path:** `/Users/tim/HolmOS/services/file-download/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

### 4. file-meta
**Path:** `/Users/tim/HolmOS/services/file-meta/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

### 5. file-mkdir
**Path:** `/Users/tim/HolmOS/services/file-mkdir/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

### 6. file-move
**Path:** `/Users/tim/HolmOS/services/file-move/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

### 7. file-search
**Path:** `/Users/tim/HolmOS/services/file-search/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

### 8. file-upload
**Path:** `/Users/tim/HolmOS/services/file-upload/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

### 9. holm-cli
**Path:** `/Users/tim/HolmOS/services/holm-cli/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

### 10. pxe-server
**Path:** `/Users/tim/HolmOS/services/pxe-server/deployment.yaml`
- Memory Request: Not defined
- Memory Limit: Not defined
- CPU Request: Not defined
- CPU Limit: Not defined

---

## Recommendations

### Priority 1: Add Resource Limits to File Services

The basic file operation services (`file-copy`, `file-delete`, `file-download`, `file-meta`, `file-mkdir`, `file-move`, `file-search`, `file-upload`) are all missing resource limits. These should be addressed first as they form a core functionality group.

**Recommended configuration for basic file services:**
```yaml
resources:
  requests:
    memory: "32Mi"
    cpu: "25m"
  limits:
    memory: "128Mi"
    cpu: "200m"
```

### Priority 2: Add Resource Limits to holm-cli

**Path:** `/Users/tim/HolmOS/services/holm-cli/deployment.yaml`

**Recommended configuration:**
```yaml
resources:
  requests:
    memory: "32Mi"
    cpu: "25m"
  limits:
    memory: "128Mi"
    cpu: "200m"
```

### Priority 3: Add Resource Limits to pxe-server

**Path:** `/Users/tim/HolmOS/services/pxe-server/deployment.yaml`

This is a special-purpose service running with host networking and privileged mode. Given it runs dnsmasq and TFTP:

**Recommended configuration:**
```yaml
resources:
  requests:
    memory: "64Mi"
    cpu: "50m"
  limits:
    memory: "256Mi"
    cpu: "500m"
```

### Priority 4: Review Existing Resource Allocations

Some services have notably different resource configurations that may need review:

1. **settings-restore** - Has very conservative limits (32Mi/64Mi memory, 25m/100m CPU). Verify this is sufficient.

2. **ai-bots (steve-bot, alice-bot)** - Has high CPU limits (1000m). Monitor actual usage to optimize.

3. **gitea** - Higher memory allocation (256Mi/512Mi) is appropriate for a git server.

4. **ios-shell** - Running 2 replicas with moderate resources. Consider if this is necessary.

---

## Resource Summary by Category

### Minimal Services (32Mi request)
- config-sync
- file-permissions
- file-share-create
- file-share-validate
- health-aggregator
- settings-restore

### Standard Services (64Mi request)
- Most application services
- Web frontends
- API services

### Memory-Intensive Services (128Mi+ request)
- ai-bots (steve-bot, alice-bot)
- audiobook-audio-normalize
- cicd-controller
- deploy-controller (devops)
- file-convert
- file-thumbnail
- gitea
- holm-git

---

## Compliance Score

**Overall Compliance:** 79.4% (54/68 deployments with complete resource limits)

To achieve 100% compliance, add resource limits to the 14 services listed above.
