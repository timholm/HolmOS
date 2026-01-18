# Vault Service

## Purpose

Vault is the secret management service for HolmOS with the motto: "Your secrets are safe with me." It provides secure storage, versioning, and rotation of secrets with AES-256-GCM encryption. The service includes both a REST API for programmatic access and a web UI for secret management.

## Deployment Details

| Property | Value |
|----------|-------|
| Image | `localhost:30500/vault:latest` |
| Language | Python (Flask) |
| Namespace | `holm` |
| Internal Port | 8080 |
| NodePort | 30870 |
| Service Type | NodePort |
| Replicas | 1 |

## API Endpoints

### Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | Health check with encryption info |

### Secret Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/secrets` | List all secrets (metadata only) |
| POST | `/api/secrets` | Create a new secret |
| GET | `/api/secrets/{name}` | Read secret value |
| GET | `/api/secrets/{name}?version={n}` | Read specific version |
| PUT | `/api/secrets/{name}` | Update secret (creates new version) |
| DELETE | `/api/secrets/{name}` | Delete secret and all versions |
| DELETE | `/api/secrets/{name}?version={n}` | Delete specific version |
| GET | `/api/secrets/{name}/versions` | List all versions (metadata) |

### Secret Rotation

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/secrets/{name}/rotate` | Rotate secret to new value |
| GET | `/api/secrets/{name}/rotation-policy` | Get rotation policy |
| PUT | `/api/secrets/{name}/rotation-policy` | Set rotation policy |
| GET | `/api/rotation/pending` | List secrets needing rotation |

### Audit

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/audit` | Get audit log entries |
| GET | `/api/audit?limit={n}` | Get limited audit entries |

### Web UI

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/` | Vault web interface |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |

### Resource Limits

```yaml
resources:
  requests:
    memory: "64Mi"
    cpu: "50m"
  limits:
    memory: "256Mi"
    cpu: "500m"
```

### Storage

- **PersistentVolumeClaim**: `vault-data`
- **Storage Size**: 1Gi
- **Mount Path**: `/data`
- **Access Mode**: ReadWriteOnce

### Data Files

| File | Description |
|------|-------------|
| `/data/secrets.json` | Encrypted secrets storage |
| `/data/audit.log` | Audit log (JSON lines format) |
| `/data/master.key` | 256-bit AES encryption key |

### Health Probes

```yaml
livenessProbe:
  httpGet:
    path: /api/health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /api/health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Dependencies

- **PersistentVolumeClaim**: `vault-data` for secret storage
- Python cryptography library for AES-256-GCM encryption

## Security Features

### Encryption

- **Algorithm**: AES-256-GCM (Galois/Counter Mode)
- **Key Size**: 256 bits (32 bytes)
- **Nonce Size**: 96 bits (12 bytes) per encryption
- **Key Storage**: Master key stored in `/data/master.key` with 0600 permissions
- **Data at Rest**: All secret values are encrypted before storage

### File Permissions

```python
os.chmod(KEY_FILE, 0o600)   # Master key
os.chmod(SECRETS_FILE, 0o600)  # Secrets database
```

### Audit Logging

All operations are logged to the audit file with:
- Timestamp
- Action type (CREATE, READ, UPDATE, DELETE, ROTATE, LIST)
- Secret name
- User (currently "system")
- Details

## Secret Versioning

Each secret maintains a version history:

```json
{
  "my-secret": {
    "current_version": 3,
    "versions": [
      {"version": 1, "encrypted": {...}, "created_at": "...", "metadata": {}},
      {"version": 2, "encrypted": {...}, "created_at": "...", "metadata": {}},
      {"version": 3, "encrypted": {...}, "created_at": "...", "metadata": {"rotated": true}}
    ],
    "created_at": "2024-01-15T10:00:00",
    "updated_at": "2024-01-17T14:30:00"
  }
}
```

## Rotation Policy

Secrets can have automatic rotation policies configured:

```json
{
  "enabled": true,
  "interval_days": 30,
  "auto_rotate": false,
  "notify_before_days": 7
}
```

When `auto_rotate` is enabled, the service can generate a new random value using `secrets.token_urlsafe(32)`.

## Example Usage

### Create a Secret

```bash
curl -X POST http://localhost:30870/api/secrets \
  -H "Content-Type: application/json" \
  -d '{
    "name": "database-password",
    "value": "super-secret-password",
    "metadata": {"description": "Production database password"}
  }'
```

**Response:**
```json
{
  "success": true,
  "version": 1
}
```

### Read a Secret

```bash
curl http://localhost:30870/api/secrets/database-password
```

**Response:**
```json
{
  "name": "database-password",
  "value": "super-secret-password",
  "version": 1,
  "created_at": "2024-01-17T10:00:00",
  "metadata": {"description": "Production database password"}
}
```

### Read Specific Version

```bash
curl http://localhost:30870/api/secrets/database-password?version=1
```

### Update a Secret

```bash
curl -X PUT http://localhost:30870/api/secrets/database-password \
  -H "Content-Type: application/json" \
  -d '{"value": "new-secret-password"}'
```

**Response:**
```json
{
  "success": true,
  "version": 2
}
```

### List All Secrets

```bash
curl http://localhost:30870/api/secrets
```

**Response:**
```json
[
  {
    "name": "database-password",
    "current_version": 2,
    "version_count": 2,
    "created_at": "2024-01-17T10:00:00",
    "updated_at": "2024-01-17T11:00:00",
    "rotation_policy": null,
    "last_rotated": null,
    "next_rotation": null
  }
]
```

### Set Rotation Policy

```bash
curl -X PUT http://localhost:30870/api/secrets/database-password/rotation-policy \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "interval_days": 30,
    "auto_rotate": true,
    "notify_before_days": 7
  }'
```

### Rotate a Secret

```bash
# With specific value
curl -X POST http://localhost:30870/api/secrets/database-password/rotate \
  -H "Content-Type: application/json" \
  -d '{"value": "rotated-password"}'

# Auto-generate (if auto_rotate policy is enabled)
curl -X POST http://localhost:30870/api/secrets/database-password/rotate \
  -H "Content-Type: application/json" \
  -d '{}'
```

**Response:**
```json
{
  "success": true,
  "version": 3,
  "rotated_at": "2024-01-17T12:00:00",
  "next_rotation": "2024-02-16T12:00:00",
  "auto_generated": false
}
```

### Get Pending Rotations

```bash
curl http://localhost:30870/api/rotation/pending
```

**Response:**
```json
{
  "count": 1,
  "secrets": [
    {
      "name": "api-key",
      "status": "overdue",
      "next_rotation": "2024-01-10T00:00:00",
      "days_until": -7,
      "auto_rotate": true
    }
  ]
}
```

### View Audit Log

```bash
curl http://localhost:30870/api/audit?limit=10
```

**Response:**
```json
[
  {
    "timestamp": "2024-01-17T12:00:00",
    "action": "ROTATE",
    "secret_name": "database-password",
    "user": "system",
    "details": "Rotated to version 3"
  }
]
```

### Delete a Secret

```bash
curl -X DELETE http://localhost:30870/api/secrets/database-password
```

## Access URLs

| Type | URL |
|------|-----|
| Internal (ClusterIP) | `http://vault.holm.svc.cluster.local` |
| NodePort | `http://<node-ip>:30870` |
| Local | `http://localhost:30870` |

## Web UI Features

The Vault web interface (Catppuccin Mocha theme) provides:

- **Dashboard**: Overview with total secrets, versions, and pending rotations
- **Secret Browser**: List and select secrets
- **Secret Details**: View value (with blur toggle), metadata, and versions
- **Create Secrets**: Form for adding new secrets
- **Update Secrets**: Create new versions of existing secrets
- **Rotation Management**: Configure policies and trigger manual rotation
- **Audit Log Viewer**: Browse recent audit entries

## Tolerations

Vault can run on control-plane nodes:

```yaml
tolerations:
- key: "node-role.kubernetes.io/control-plane"
  operator: "Exists"
  effect: "NoSchedule"
```

## Testing

API tests are located at `/Users/tim/HolmOS/tests/api/test_vault.py`:

```bash
cd /Users/tim/HolmOS/tests
pytest api/test_vault.py -v
```

Tests cover:
- Health endpoint
- Secret CRUD operations
- Version management
- Duplicate secret handling
- Input validation
- Audit logging
- Web UI rendering
