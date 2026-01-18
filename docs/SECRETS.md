# Kubernetes Secrets Management

This document describes the secrets used in the HolmOS cluster and how to manage them securely.

## Important Security Rules

1. **NEVER commit actual secret values to this repository**
2. Secret manifests in `/k8s/secrets/` contain placeholder values only
3. Real values must be set directly in the cluster using `kubectl`

## Secrets Overview

| Secret Name | Namespace | Keys | Used By |
|-------------|-----------|------|---------|
| `postgres-secret` | holm | `password`, `database-url` | terminal-web, auth-gateway, notification-webhook, audiobook-web, and other database-connected services |
| `auth-jwt-secret` | holm | `secret` | auth-gateway |
| `auth-admin-secret` | holm | `password` | auth-gateway |
| `ssh-credentials` | holm | `password` | merchant (claude-terminal) |
| `backup-storage-db-secret` | holm | `username`, `password` | backup-storage |
| `user-preferences-db-secret` | holm | `database-url` | user-preferences |

## Setting Real Values in the Cluster

### Option 1: Create secrets imperatively (recommended for production)

```bash
# PostgreSQL credentials
kubectl create secret generic postgres-secret \
  --from-literal=password='YOUR_ACTUAL_PASSWORD' \
  --from-literal=database-url='postgres://holm:YOUR_ACTUAL_PASSWORD@postgres.holm.svc.cluster.local:5432/holm?sslmode=disable' \
  -n holm

# Auth JWT secret
kubectl create secret generic auth-jwt-secret \
  --from-literal=secret='YOUR_JWT_SECRET_KEY' \
  -n holm

# Auth admin password
kubectl create secret generic auth-admin-secret \
  --from-literal=password='YOUR_ADMIN_PASSWORD' \
  -n holm

# SSH credentials (for claude-terminal/merchant)
kubectl create secret generic ssh-credentials \
  --from-literal=password='YOUR_SSH_PASSWORD' \
  -n holm

# Backup storage database credentials
kubectl create secret generic backup-storage-db-secret \
  --from-literal=username='YOUR_DB_USERNAME' \
  --from-literal=password='YOUR_DB_PASSWORD' \
  -n holm

# User preferences database URL
kubectl create secret generic user-preferences-db-secret \
  --from-literal=database-url='postgres://USER:PASSWORD@HOST:PORT/DB?sslmode=disable' \
  -n holm
```

### Option 2: Update existing secrets

```bash
# Delete and recreate
kubectl delete secret postgres-secret -n holm
kubectl create secret generic postgres-secret \
  --from-literal=password='NEW_PASSWORD' \
  --from-literal=database-url='postgres://holm:NEW_PASSWORD@postgres.holm.svc.cluster.local:5432/holm?sslmode=disable' \
  -n holm

# Restart deployments to pick up new secrets
kubectl rollout restart deployment/terminal-web -n holm
```

### Option 3: Patch existing secrets

```bash
# Base64 encode the new value
echo -n 'new-password' | base64

# Patch the secret
kubectl patch secret postgres-secret -n holm \
  -p '{"data":{"password":"bmV3LXBhc3N3b3Jk"}}'
```

## Verifying Secrets

```bash
# List all secrets in the holm namespace
kubectl get secrets -n holm

# View secret details (without revealing values)
kubectl describe secret postgres-secret -n holm

# Decode a secret value (be careful with this in shared terminals)
kubectl get secret postgres-secret -n holm -o jsonpath='{.data.password}' | base64 -d
```

## Template Files

The `/k8s/secrets/` directory contains template manifests with placeholder values:

- `postgres-secret.yaml` - Main PostgreSQL credentials
- `auth-secrets.yaml` - JWT and admin authentication secrets
- `ssh-secret.yaml` - SSH credentials for terminal services
- `backup-storage-db-secret.yaml` - Backup service database credentials
- `user-preferences-db-secret.yaml` - User preferences service database URL

These templates are useful for:
- Understanding what secrets are needed
- Local development (with test values)
- Documentation purposes

**WARNING**: Do not apply these templates directly to production without changing the placeholder values!

## Best Practices

1. Use strong, randomly generated passwords (at least 32 characters)
2. Rotate secrets periodically
3. Use different credentials for different environments (dev, staging, prod)
4. Consider using a secrets management solution like:
   - HashiCorp Vault
   - Kubernetes External Secrets
   - Sealed Secrets
5. Audit secret access using Kubernetes audit logs
