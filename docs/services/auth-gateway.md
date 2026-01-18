# Auth Gateway Service

## Purpose

Auth Gateway is the centralized authentication and authorization service for HolmOS. It provides JWT-based authentication, user management, session handling, and token validation for all HolmOS services. The service includes both API endpoints for programmatic access and a web UI for user authentication and administration.

## Deployment Details

| Property | Value |
|----------|-------|
| Image | `localhost:30500/auth-gateway:latest` |
| Language | Go |
| Namespace | `holm` |
| Internal Port | 8080 |
| Service Ports | ClusterIP: 80, NodePort: 30100 |
| Ingress Host | `auth.holm.local` |
| Replicas | 1 |

## API Endpoints

### Public Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check endpoint |
| GET | `/ready` | Readiness check (verifies database connection) |
| GET | `/login` | Login page (HTML) |
| POST | `/login` | Form-based login |
| GET | `/logout` | Logout and clear session |
| GET | `/register` | Registration page (HTML) |
| POST | `/register` | Form-based registration |

### API Endpoints (JSON)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/login` | Authenticate user, returns JWT tokens |
| POST | `/api/logout` | Invalidate session |
| POST | `/api/register` | Create new user account |
| GET | `/api/validate` | Validate JWT token |
| POST | `/api/refresh` | Refresh access token using refresh token |
| GET | `/api/me` | Get current authenticated user info |
| POST | `/api/change-password` | Change user password |

### Admin API Endpoints (requires admin role)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/users` | List all users |
| POST | `/api/users` | Create new user |
| GET | `/api/users/{id}` | Get user by ID |
| PUT | `/api/users/{id}` | Update user |
| DELETE | `/api/users/{id}` | Delete user |
| GET | `/api/sessions` | List active sessions |
| DELETE | `/api/sessions?id={id}` | Revoke specific session |

### Admin Web Pages

| Endpoint | Description |
|----------|-------------|
| `/admin` | Admin dashboard |
| `/admin/users` | User management interface |
| `/admin/sessions` | Session management interface |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_HOST` | `postgres.holm.svc.cluster.local` | PostgreSQL host |
| `DB_USER` | `postgres` | Database username |
| `DB_PASSWORD` | (from secret) | Database password |
| `DB_NAME` | `holm` | Database name |
| `JWT_SECRET` | (auto-generated) | Secret key for JWT signing |
| `ADMIN_PASSWORD` | `admin123` | Initial admin password |

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

### Health Probes

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Dependencies

- **PostgreSQL**: `postgres.holm.svc.cluster.local:5432`
- **Secrets**:
  - `postgres-secret` - Database password
  - `auth-jwt-secret` - JWT signing secret
  - `auth-admin-secret` - Initial admin password

## Database Schema

The service auto-creates the following tables:

### `auth_users`

| Column | Type | Description |
|--------|------|-------------|
| `id` | SERIAL | Primary key |
| `username` | VARCHAR(255) | Unique username |
| `email` | VARCHAR(255) | Unique email (optional) |
| `password_hash` | VARCHAR(255) | bcrypt hashed password |
| `role` | VARCHAR(50) | User role (`user`, `admin`) |
| `created_at` | TIMESTAMP | Account creation time |
| `last_login` | TIMESTAMP | Last login timestamp |

### `auth_sessions`

| Column | Type | Description |
|--------|------|-------------|
| `id` | VARCHAR(255) | Session ID (primary key) |
| `user_id` | INTEGER | Foreign key to auth_users |
| `token` | TEXT | Refresh token |
| `expires_at` | TIMESTAMP | Session expiration |
| `created_at` | TIMESTAMP | Session creation time |
| `ip` | VARCHAR(50) | Client IP address |
| `user_agent` | TEXT | Client user agent |

## Authentication Flow

### Token Structure

- **Access Token**: JWT valid for 15 minutes
- **Refresh Token**: JWT valid for 7 days
- **Token Type**: Bearer

### JWT Claims

```json
{
  "user_id": 1,
  "username": "admin",
  "role": "admin",
  "exp": 1234567890,
  "iat": 1234567000,
  "iss": "holmos-auth"
}
```

### Cookie Names

| Cookie | Purpose | Max Age |
|--------|---------|---------|
| `holmos_token` | Access token | 900s (15 min) |
| `holmos_session` | Session ID | 604800s (7 days) |

## Example Usage

### Login (API)

```bash
curl -X POST http://auth.holm.local/api/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "admin123"}'
```

**Response:**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_in": 900,
  "token_type": "Bearer"
}
```

### Validate Token

```bash
curl http://auth.holm.local/api/validate \
  -H "Authorization: Bearer <access_token>"
```

**Response:**
```json
{
  "valid": true,
  "user_id": 1,
  "username": "admin",
  "role": "admin"
}
```

### Register New User

```bash
curl -X POST http://auth.holm.local/api/register \
  -H "Content-Type: application/json" \
  -d '{"username": "newuser", "email": "user@example.com", "password": "securepass"}'
```

### Refresh Token

```bash
curl -X POST http://auth.holm.local/api/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "<refresh_token>"}'
```

### Service Integration

Other services can validate tokens by calling the auth-gateway:

```go
func validateToken(token string) (*ValidationResponse, error) {
    req, _ := http.NewRequest("GET", "http://auth-gateway.holm.svc.cluster.local/api/validate", nil)
    req.Header.Set("Authorization", "Bearer "+token)

    resp, err := http.DefaultClient.Do(req)
    // Handle response...
}
```

## Security Features

- **Password Hashing**: bcrypt with default cost
- **JWT Signing**: HMAC-SHA256
- **Session Tracking**: IP address and user agent logging
- **CORS**: Enabled for all origins (configurable)
- **Input Validation**: Username/email format validation
- **Rate Limiting**: Not currently implemented (consider adding)

## Access URLs

| Type | URL |
|------|-----|
| Internal (ClusterIP) | `http://auth-gateway.holm.svc.cluster.local` |
| NodePort | `http://<node-ip>:30100` |
| Ingress | `http://auth.holm.local` |

## Testing

E2E tests are located at `/Users/tim/HolmOS/tests/e2e/auth-gateway.test.js`:

```bash
cd /Users/tim/HolmOS/tests/e2e
node auth-gateway.test.js
```

Tests cover:
- Health and ready endpoints
- Successful/failed login
- Token validation
- User registration
- Token refresh
- Logout functionality
