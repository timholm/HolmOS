# Container Registry Service

## Purpose

The Container Registry is the private Docker/OCI registry for HolmOS, providing storage and distribution of container images. It consists of two components:

1. **Registry**: The core Docker Registry v2 server for storing and serving container images
2. **Registry UI**: A web-based interface for browsing, managing, and deleting images

## Deployment Details

### Registry (Core)

| Property | Value |
|----------|-------|
| Image | `registry:2` |
| Namespace | `holm` |
| Port | 5000 |
| NodePort | 31500 |
| Service Type | NodePort |
| Replicas | 1 |

### Registry UI

| Property | Value |
|----------|-------|
| Image | `localhost:31500/holm/registry-ui:latest` |
| Language | Go |
| Namespace | `holm` |
| Port | 8080 |
| NodePort | 31750 |
| Service Type | NodePort |
| Replicas | 1 |

## API Endpoints

### Registry (Docker Registry v2 API)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v2/` | API version check and health |
| GET | `/v2/_catalog` | List all repositories |
| GET | `/v2/{name}/tags/list` | List tags for a repository |
| GET | `/v2/{name}/manifests/{reference}` | Get image manifest |
| HEAD | `/v2/{name}/manifests/{reference}` | Check manifest exists |
| DELETE | `/v2/{name}/manifests/{digest}` | Delete image manifest |
| GET | `/v2/{name}/blobs/{digest}` | Download image layer |
| HEAD | `/v2/{name}/blobs/{digest}` | Check blob exists |
| POST | `/v2/{name}/blobs/uploads/` | Start blob upload |
| PUT | `/v2/{name}/blobs/uploads/{uuid}` | Complete blob upload |

### Registry UI

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check endpoint |
| GET | `/` | Web UI - repository list |
| GET | `/repo/{name}` | Web UI - repository details with tags |
| POST | `/delete` | Delete image by digest |
| GET | `/api/repos` | JSON - list repositories |
| GET | `/api/tags/{name}` | JSON - list tags for repository |

## Configuration

### Registry Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `REGISTRY_STORAGE_DELETE_ENABLED` | `true` | Enable image deletion |
| `REGISTRY_HTTP_HEADERS_Access-Control-Allow-Origin` | `[*]` | CORS allowed origins |
| `REGISTRY_HTTP_HEADERS_Access-Control-Allow-Methods` | `[HEAD,GET,OPTIONS,DELETE]` | CORS allowed methods |
| `REGISTRY_HTTP_HEADERS_Access-Control-Allow-Headers` | `[Authorization,Accept,Cache-Control]` | CORS headers |
| `REGISTRY_HTTP_HEADERS_Access-Control-Expose-Headers` | `[Docker-Content-Digest]` | Exposed headers |

### Registry UI Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `REGISTRY_URL` | `http://registry.holm.svc.cluster.local:5000` | Backend registry URL |

### Resource Limits

**Registry:**
```yaml
resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

**Registry UI:**
```yaml
resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

### Storage

- **PersistentVolumeClaim**: `registry-pvc`
- **Storage Size**: 10Gi
- **Mount Path**: `/var/lib/registry`
- **Access Mode**: ReadWriteOnce

### Health Probes

**Registry:**
```yaml
livenessProbe:
  httpGet:
    path: /v2/
    port: 5000
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /v2/
    port: 5000
  initialDelaySeconds: 3
  periodSeconds: 5
```

**Registry UI:**
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 3
  periodSeconds: 5
```

## Dependencies

- **PersistentVolumeClaim**: `registry-pvc` for image storage
- **Registry UI** depends on **Registry** being accessible

## Node Affinity

Both Registry and Registry UI are configured to avoid scheduling on the `openmediavault` node:

```yaml
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
      - matchExpressions:
        - key: kubernetes.io/hostname
          operator: NotIn
          values:
          - openmediavault
```

## Example Usage

### Pushing an Image

```bash
# Tag the image for the private registry
docker tag myapp:latest localhost:31500/myapp:latest

# Push to registry
docker push localhost:31500/myapp:latest
```

### Pulling an Image

```bash
# Pull from registry
docker pull localhost:31500/myapp:latest
```

### Using in Kubernetes

```yaml
containers:
- name: myapp
  image: localhost:31500/myapp:latest
  imagePullPolicy: Always
```

### List Repositories (API)

```bash
curl http://localhost:31500/v2/_catalog
```

**Response:**
```json
{
  "repositories": ["auth-gateway", "vault", "registry-ui", "myapp"]
}
```

### List Tags (API)

```bash
curl http://localhost:31500/v2/myapp/tags/list
```

**Response:**
```json
{
  "name": "myapp",
  "tags": ["latest", "v1.0.0", "v1.1.0"]
}
```

### Delete Image (via Registry UI)

```bash
# Get the digest first
DIGEST=$(curl -s -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
  http://localhost:31500/v2/myapp/manifests/latest \
  -I | grep Docker-Content-Digest | cut -d' ' -f2 | tr -d '\r')

# Delete via Registry UI
curl -X POST http://localhost:31750/delete \
  -d "repo=myapp&digest=$DIGEST"
```

### Registry UI JSON APIs

```bash
# List repositories
curl http://localhost:31750/api/repos

# List tags for a repository
curl http://localhost:31750/api/tags/myapp
```

## Access URLs

| Service | Type | URL |
|---------|------|-----|
| Registry | Internal | `http://registry.holm.svc.cluster.local:5000` |
| Registry | NodePort | `http://<node-ip>:31500` |
| Registry | Local | `http://localhost:31500` |
| Registry UI | Internal | `http://registry-ui.holm.svc.cluster.local:8080` |
| Registry UI | NodePort | `http://<node-ip>:31750` |

## Supported Manifest Types

The Registry UI supports both Docker and OCI manifest formats:

- `application/vnd.oci.image.manifest.v1+json` (OCI/Buildah/Podman)
- `application/vnd.docker.distribution.manifest.v2+json` (Docker)

## Features

### Registry UI Features

- **Repository Browser**: View all repositories in the registry
- **Tag Details**: View tag information including size, layers, and digest
- **Human-readable Sizes**: Automatic conversion (B, KB, MB, GB)
- **Delete Support**: Delete images by tag with confirmation modal
- **Responsive Design**: Dark theme UI that works on mobile

## Garbage Collection

After deleting images, run garbage collection to reclaim storage:

```bash
kubectl exec -n holm -it $(kubectl get pod -n holm -l app=registry -o jsonpath='{.items[0].metadata.name}') \
  -- registry garbage-collect /etc/docker/registry/config.yml
```

## Testing

API tests are located at `/Users/tim/HolmOS/tests/api/test_registry_ui.py`:

```bash
cd /Users/tim/HolmOS/tests
pytest api/test_registry_ui.py -v
```

Tests cover:
- Health endpoint
- Index page rendering
- Repository listing
- Tags listing
- Search functionality
