# Settings Web

## Overview

Settings Web is the central configuration hub for HolmOS, providing a unified interface for managing themes, user preferences, system information, backups, and notification settings.

**Technology Stack:** Go with embedded HTML UI

**Default Port:** 8080

**Version:** 2.1.0

## Purpose

Settings Web serves as the central settings management interface for HolmOS, offering:
- Theme customization with Catppuccin variants
- User preference management
- System information and cluster monitoring
- Configuration backup and restore
- Notification settings
- Integration with specialized settings microservices

## Architecture

Settings Web acts as a central hub that proxies requests to specialized microservices:

| Service | URL | Purpose |
|---------|-----|---------|
| settings-theme | http://settings-theme.holm:8080 | Theme preferences |
| settings-backup | http://settings-backup.holm:8080 | Configuration export |
| settings-restore | http://settings-restore.holm:8080 | Configuration import |
| settings-tabs | http://settings-tabs.holm:8080 | Tab state persistence |

## UI Features

### Theme Settings
- Theme selection: Mocha, Macchiato, Frappe, Latte (Catppuccin variants)
- Compact mode toggle
- Animation enable/disable
- Font size adjustment (px)
- Accent color selection (lavender, blue, green, etc.)

### User Preferences
- Language selection
- Timezone configuration
- Date format (YYYY-MM-DD, DD/MM/YYYY, MM/DD/YYYY)
- Auto-save toggle
- Show hidden files toggle
- Default view mode (grid/list)

### Notification Settings
- Global enable/disable
- Sound notifications
- Desktop notifications
- Email notifications
- Build alerts
- Cluster alerts

### System Information
- Cluster nodes with status, role, CPU, memory, architecture
- Pod summary (total, running, pending, failed, completed)
- Services count
- Namespaces count
- Storage usage per volume
- Cluster uptime

### Backup/Restore
- Export all settings to JSON
- Import settings from backup file
- Validation before restore

### About Section
- Application name and version
- Build date
- Go version
- Platform and architecture
- Hostname
- Microservice status indicators

## API Endpoints

### Health & Info

#### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "healthy",
  "service": "settings-web",
  "version": "2.1.0"
}
```

#### GET /api/about
Returns application and service status information.

**Response:**
```json
{
  "appName": "HolmOS Settings Hub",
  "version": "2.1.0",
  "buildDate": "2026-01-15",
  "goVersion": "go1.21.0",
  "platform": "linux",
  "architecture": "arm64",
  "hostname": "settings-web-xxx",
  "services": [
    {"name": "settings-theme", "status": "online", "port": "8080"},
    {"name": "settings-backup", "status": "online", "port": "8080"}
  ]
}
```

### Theme Endpoints

#### GET /api/theme/preferences
Returns current theme preferences.

**Response:**
```json
{
  "theme": "mocha",
  "compactMode": false,
  "animations": true,
  "fontSize": 16,
  "accentColor": "lavender"
}
```

#### POST /api/theme/preferences
Updates theme preferences.

#### POST /api/theme/apply
Applies theme changes immediately.

### Backup/Restore Endpoints

#### GET /api/backup/export
Exports all settings as a downloadable JSON backup.

**Response:**
```json
{
  "version": "2.1.0",
  "exportDate": "2026-01-17T12:00:00Z",
  "theme": "mocha",
  "preferences": {...},
  "notifications": {...}
}
```

#### GET /api/backup/list
Lists available backup snapshots.

#### POST /api/restore/import
Imports settings from a backup file.

#### POST /api/restore/validate
Validates a backup file before import.

### Tab State

#### GET/POST /api/tabs/state
Persists or retrieves the active settings tab.

**Response:**
```json
{
  "activeTab": "theme"
}
```

### System Information

#### GET /api/system/info
Returns comprehensive system information.

**Response:**
```json
{
  "nodes": [...],
  "pods": {"total": 50, "running": 45, "pending": 2, "failed": 1, "completed": 2},
  "services": 25,
  "namespaces": 5,
  "storage": [...],
  "uptime": "15d 4h 32m"
}
```

#### GET /api/system/nodes
Returns detailed node information.

**Response:**
```json
[
  {
    "name": "node01",
    "status": "Ready",
    "role": "control-plane",
    "cpu": "4",
    "memory": "8Gi",
    "age": "45d",
    "arch": "arm64"
  }
]
```

#### GET /api/system/pods
Returns pod summary statistics.

#### GET /api/system/storage
Returns storage volume information.

**Response:**
```json
[
  {
    "name": "files-pvc",
    "size": "100Gi",
    "used": "45Gi",
    "available": "55Gi",
    "percent": "45%",
    "mount": "/data"
  }
]
```

### User Preferences

#### GET/POST /api/user/preferences
Gets or updates user preferences.

**Response:**
```json
{
  "language": "en",
  "timezone": "America/Los_Angeles",
  "dateFormat": "YYYY-MM-DD",
  "autoSave": true,
  "showHidden": false,
  "defaultView": "grid"
}
```

### Notifications

#### GET/POST /api/notifications/settings
Gets or updates notification settings.

**Response:**
```json
{
  "enabled": true,
  "sound": true,
  "desktop": true,
  "email": false,
  "buildAlerts": true,
  "clusterAlerts": true
}
```

## Data Structures

### Theme Preferences
```go
type ThemePreferences struct {
    Theme       string `json:"theme"`
    CompactMode bool   `json:"compactMode"`
    Animations  bool   `json:"animations"`
    FontSize    int    `json:"fontSize"`
    AccentColor string `json:"accentColor"`
}
```

### User Preferences
```go
type UserPreferences struct {
    Language     string `json:"language"`
    Timezone     string `json:"timezone"`
    DateFormat   string `json:"dateFormat"`
    AutoSave     bool   `json:"autoSave"`
    ShowHidden   bool   `json:"showHidden"`
    DefaultView  string `json:"defaultView"`
}
```

### Notification Settings
```go
type NotificationSettings struct {
    Enabled       bool `json:"enabled"`
    Sound         bool `json:"sound"`
    Desktop       bool `json:"desktop"`
    Email         bool `json:"email"`
    BuildAlerts   bool `json:"buildAlerts"`
    ClusterAlerts bool `json:"clusterAlerts"`
}
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | Server port |

## Build Process

The settings-web binary is built using a two-step process:
1. `combine.py` embeds the HTML UI into `main.go`
2. Go compiles the final binary with embedded assets

## Screenshot Description

The Settings Web interface presents a tabbed layout with sections for Theme, Preferences, Notifications, System, Backup, and About. The Theme tab shows color swatches for Catppuccin themes, toggles for compact mode and animations, a font size slider, and accent color options. The System tab displays a dashboard with node cards showing status, resource usage, and role, along with storage bars and pod statistics. The About tab shows version information and service health indicators with green/red status dots.
