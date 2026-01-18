# Backup Service

## Purpose

The Backup Service (backup-storage) provides a centralized backup management system for HolmOS. It stores, retrieves, verifies, and manages backup data with metadata tracking in PostgreSQL and file storage on disk. The service supports multiple backup types, search functionality, and data integrity verification.

## How It Works

### Backup Storage
- Accepts backup data as base64-encoded content via REST API
- Stores backup files in a configurable directory on disk
- Maintains metadata in PostgreSQL including name, type, size, and timestamps
- Generates unique UUIDs for each backup

### File Management
- Files are stored with format: `{uuid}_{sanitized_name}`
- Filenames are sanitized to remove path traversal characters
- Supports downloading backup files with proper Content-Disposition headers
- Deletion removes both file and database metadata

### Backup Types
- Supports categorization of backups by type (e.g., database, config, application)
- Default type is "generic" if not specified
- Filtering and statistics by backup type

### Data Verification
- Verify endpoint checks backup integrity
- Confirms file existence on disk
- Validates that stored file size matches metadata
- Reports verification status with detailed diagnostics

### Statistics and Search
- Aggregate statistics: total count, total size, counts by type
- Oldest and newest backup timestamps
- Full-text search across backup names and types

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8080` | HTTP server port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `postgres` | Database username |
| `DB_PASSWORD` | `postgres` | Database password |
| `DB_NAME` | `backups` | Database name |
| `BACKUP_DIR` | `/data/backups` | Backup file storage directory |

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Service health check |
| `/backups` | GET | List all backups (supports `type`, `limit`, `offset` params) |
| `/backups` | POST | Create new backup |
| `/backups/stats` | GET | Get backup statistics |
| `/backups/search` | GET | Search backups (requires `q` param) |
| `/backups/{id}` | GET | Get backup metadata |
| `/backups/{id}` | PUT | Update backup metadata |
| `/backups/{id}` | DELETE | Delete backup |
| `/backups/{id}/download` | GET | Download backup file |
| `/backups/{id}/restore` | POST | Get backup with base64 data |
| `/backups/{id}/verify` | POST | Verify backup integrity |

### Request/Response Examples

**Create Backup:**
```json
POST /backups
{
  "name": "database-backup-2024-01-15",
  "type": "database",
  "data": "base64-encoded-content"
}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "database-backup-2024-01-15",
  "type": "database",
  "size": 1048576,
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

**Statistics Response:**
```json
{
  "total_count": 150,
  "total_size": 5368709120,
  "count_by_type": {
    "database": 50,
    "config": 80,
    "application": 20
  },
  "size_by_type": {
    "database": 4294967296,
    "config": 536870912,
    "application": 536870912
  },
  "oldest_backup": "2023-01-01T00:00:00Z",
  "newest_backup": "2024-01-15T10:30:00Z"
}
```

**Verify Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "valid": true,
  "file_exists": true,
  "size_matches": true,
  "expected_size": 1048576,
  "actual_size": 1048576,
  "message": "Backup is valid"
}
```

### Database Schema

```sql
CREATE TABLE backups (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(100) NOT NULL,
    size BIGINT NOT NULL DEFAULT 0,
    file_path VARCHAR(500) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_backups_name ON backups(name);
CREATE INDEX idx_backups_type ON backups(type);
CREATE INDEX idx_backups_created_at ON backups(created_at);
```

## Dependencies

### Internal Services
- **PostgreSQL**: Metadata storage (requires database named `backups`)

### External Dependencies
- `github.com/gorilla/mux`: HTTP router
- `github.com/lib/pq`: PostgreSQL driver
- `github.com/google/uuid`: UUID generation

### Storage Requirements
- Persistent volume mounted at `BACKUP_DIR`
- Sufficient disk space for backup files
- Read/write permissions on backup directory

### Health Check
The health endpoint verifies:
- Database connectivity (PostgreSQL ping)
- Storage directory existence and accessibility
