# Terminal Web

## Overview

Terminal Web is a comprehensive web-based terminal emulator that provides SSH connections to remote hosts, kubectl access to Kubernetes clusters, local shell access, and direct pod execution capabilities.

**Technology Stack:** Go with Gorilla WebSocket, PostgreSQL for host storage, creack/pty for PTY management

**Default Port:** 8080

## Purpose

Terminal Web serves as a centralized terminal management interface for the HolmOS cluster, enabling:
- SSH connections to Raspberry Pi nodes and other remote hosts
- Direct kubectl shell access for cluster administration
- Local shell sessions within the terminal-web container
- Interactive shell sessions inside Kubernetes pods

## UI Features

### Theme Support
Five built-in terminal themes based on Catppuccin color palette:
- **Catppuccin Mocha** (default) - Dark theme with pastel colors
- **Catppuccin Macchiato** - Slightly lighter dark theme
- **Catppuccin Frappe** - Medium dark theme
- **Catppuccin Latte** - Light theme
- **HolmOS Dark** - Custom HolmOS branded dark theme

### Terminal Features
- Full xterm-256color terminal emulation
- Window resizing support
- Command history tracking per session
- Privilege escalation detection (sudo commands)
- Session activity tracking

### Host Management
- Built-in hosts: kubectl (cluster access), local shell
- Custom SSH host configuration with password or SSH key authentication
- Pre-configured initialization for 13 Raspberry Pi nodes (rpi1-rpi13)
- Color-coded hosts for visual identification

## API Endpoints

### GET /
Serves the embedded HTML terminal interface.

### GET /health
Health check endpoint returning "ok".

### GET /api/hosts
Lists all configured hosts including built-in kubectl and local shell options.

**Response:**
```json
[
  {"id": -1, "name": "kubectl", "hostname": "cluster", "port": 0, "type": "kubectl", "color": "#89b4fa"},
  {"id": -2, "name": "local shell", "hostname": "localhost", "port": 0, "type": "local", "color": "#a6e3a1"},
  {"id": 1, "name": "rpi1", "hostname": "192.168.8.197", "port": 22, "username": "rpi1", "type": "ssh", "color": "#f38ba8"}
]
```

### POST /api/hosts/add
Adds a new SSH host configuration.

**Request Body:**
```json
{
  "name": "myserver",
  "hostname": "192.168.1.100",
  "port": 22,
  "username": "admin",
  "password": "secret",
  "auth_type": "password",
  "color": "#89b4fa",
  "type": "ssh"
}
```

### DELETE /api/hosts/delete?id={id}
Removes a host configuration.

### POST /api/hosts/init
Initializes the database with 13 pre-configured Raspberry Pi nodes (rpi1-rpi13).

### GET /api/themes
Returns available terminal themes with full color configurations.

### POST /api/exec
Executes a command with a configurable timeout (default 10s, max 60s).

**Request Body:**
```json
{
  "command": "kubectl get pods",
  "timeout": 30
}
```

**Response:**
```json
{
  "success": true,
  "stdout": "...",
  "stderr": ""
}
```

### GET /api/sessions
Lists all active terminal sessions.

**Response:**
```json
{
  "success": true,
  "sessions": [
    {
      "id": "session-123-1",
      "type": "ssh",
      "host_name": "rpi1",
      "started_at": "...",
      "last_active": "...",
      "is_privileged": false
    }
  ],
  "count": 1
}
```

### DELETE /api/sessions?id={session_id}
Terminates a specific session.

### GET /api/sessions/history?session_id={id}
Returns command history for a session.

### GET /api/namespaces
Lists all Kubernetes namespaces.

### GET /api/pods?namespace={ns}
Lists pods in a namespace with container information.

**Response:**
```json
{
  "success": true,
  "pods": [
    {
      "name": "my-pod",
      "namespace": "default",
      "status": "Running",
      "containers": ["container1", "container2"]
    }
  ]
}
```

## WebSocket Endpoints

### WS /ws/terminal?host={id}
Opens an SSH terminal session to the specified host.

### WS /ws/kubectl
Opens a kubectl shell session with bash (or sh fallback).

### WS /ws/local
Opens a local shell session using the system's default shell.

### WS /ws/pod?namespace={ns}&pod={name}&container={container}
Opens an interactive shell session inside a Kubernetes pod.

### WebSocket Protocol
- Binary messages for terminal I/O
- Resize messages: `[1, cols_high, cols_low, rows_high, rows_low]`
- Command tracking via input buffer analysis

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | postgres://holm:holm-secret-123@postgres.holm.svc.cluster.local:5432/holm | PostgreSQL connection string |

## Database Schema

```sql
CREATE TABLE terminal_hosts (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    port INTEGER DEFAULT 22,
    username VARCHAR(255) NOT NULL,
    password VARCHAR(255),
    ssh_key TEXT,
    auth_type VARCHAR(50) DEFAULT 'password',
    color VARCHAR(50),
    type VARCHAR(50) DEFAULT 'ssh',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Screenshot Description

The terminal web interface displays a full-screen terminal emulator with a dark Catppuccin Mocha theme. The terminal shows a command prompt with syntax-highlighted output. A sidebar or dropdown menu provides access to different hosts, showing color-coded entries for kubectl, local shell, and various SSH hosts. The interface supports standard terminal interactions including copy/paste, scrollback, and window resizing.

## Session Management

- Sessions are tracked in memory with unique IDs
- Each session records: type, host name, start time, last activity, privilege status
- Command history is maintained per session (last 1000 commands)
- Privilege escalation is detected when sudo commands are executed
