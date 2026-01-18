# PostgreSQL Service

## Purpose

PostgreSQL is the primary relational database for HolmOS, providing persistent data storage for authentication, user management, backup metadata, and other system services. It runs as a clustered deployment within the Kubernetes namespace.

## Deployment Details

| Property | Value |
|----------|-------|
| Image | `postgres:15-alpine` |
| Namespace | `holm` |
| Port | 5432 |
| Service Type | ClusterIP |
| Replicas | 1 |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `POSTGRES_USER` | `postgres` | Database superuser username |
| `POSTGRES_PASSWORD` | (from secret) | Database password |
| `POSTGRES_DB` | `holm` / `backups` | Default database name |

### Resource Limits

```yaml
resources:
  requests:
    memory: "128Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"
```

### Storage

- **PersistentVolumeClaim**: `postgres-pvc`
- **Storage Size**: 5Gi
- **Mount Path**: `/var/lib/postgresql/data`
- **Access Mode**: ReadWriteOnce

## Dependencies

- Kubernetes PersistentVolumeClaim (`postgres-pvc`)
- Secret: `postgres-secret` containing database credentials

## Secrets

The PostgreSQL password is stored in a Kubernetes secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-secret
  namespace: holm
type: Opaque
stringData:
  password: "<secure-password>"
  database-url: "postgres://holm:<password>@postgres.holm.svc.cluster.local:5432/holm?sslmode=disable"
```

## Connection

### Internal (within cluster)

```
Host: postgres.holm.svc.cluster.local
Port: 5432
Database: holm
SSL: disabled
```

### Connection String Format

```
postgres://<user>:<password>@postgres.holm.svc.cluster.local:5432/<database>?sslmode=disable
```

## Databases

| Database | Description | Used By |
|----------|-------------|---------|
| `holm` | Primary system database | auth-gateway, user services |
| `backups` | Backup metadata storage | backup-storage service |

## Example Usage

### Connecting from a Service

```go
import (
    "database/sql"
    _ "github.com/lib/pq"
)

connStr := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable",
    "postgres.holm.svc.cluster.local", "postgres", password, "holm")

db, err := sql.Open("postgres", connStr)
```

### Python Connection

```python
import psycopg2

conn = psycopg2.connect(
    host="postgres.holm.svc.cluster.local",
    database="holm",
    user="postgres",
    password=os.environ.get("DB_PASSWORD")
)
```

### Kubernetes Deployment Reference

```yaml
env:
- name: DB_HOST
  value: "postgres.holm.svc.cluster.local"
- name: DB_USER
  value: "postgres"
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: postgres-secret
      key: password
- name: DB_NAME
  value: "holm"
```

## Health Checks

PostgreSQL health can be verified by connecting to port 5432 and executing a simple query:

```sql
SELECT 1;
```

## Maintenance

### Backup

Database backups should be scheduled via the backup-storage service or using `pg_dump`:

```bash
pg_dump -h postgres.holm.svc.cluster.local -U postgres -d holm > backup.sql
```

### Viewing Logs

```bash
kubectl logs -n holm -l app=postgres
```

### Scaling

Note: PostgreSQL is deployed as a single replica. For high availability, consider using PostgreSQL Operator or a managed database service.
