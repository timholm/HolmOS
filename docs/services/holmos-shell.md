# HolmOS Shell

## Overview

HolmOS Shell is an iPhone-style home screen interface that serves as the main launcher for accessing all HolmOS microservices and applications in the Kubernetes cluster.

**Technology Stack:** Python Flask with embedded HTML/CSS/JavaScript

**Default Port:** 8080

## Purpose

HolmOS Shell provides a mobile-friendly, iPhone-inspired launcher interface that:
- Displays all available HolmOS applications as app icons
- Organizes apps into main grid, developer tools, and dock
- Launches applications in modal iframes
- Provides a consistent visual experience across the cluster

## UI Features

### Status Bar (iOS-style)
- Current time display (HH:MM format)
- "HolmOS" branding in center
- Wi-Fi icon
- Battery indicator with level visualization

### Main App Grid
12 application icons arranged in a 4-column grid:

| App | Icon | Port | Description |
|-----|------|------|-------------|
| Calculator | Calculator | 30010 | Calculator service |
| Clock | Clock | 30011 | Clock/timer service |
| Audiobook | Headphones | 30700 | Audiobook player |
| Terminal | Laptop | 30800 | Terminal web interface |
| Vault | Lock | 30870 | Secrets/password vault |
| Scribe | Notes | 30860 | Note-taking service |
| Backup | Disk | 30850 | Backup management |
| Nova | Robot | 30004 | AI assistant (Nova) |
| Metrics | Chart | 30950 | System metrics |
| Registry | Package | 31750 | Container registry |
| Tests | Checkmark | 30900 | Test runner |
| Auth | Key | 30100 | Authentication service |

### Developer Tools Section
4 developer-focused applications:

| App | Icon | Port | Description |
|-----|------|------|-------------|
| HolmGit | Tools | 30500 | Git server interface |
| CI/CD | Gear | 30020 | CI/CD pipeline |
| Deploy | Rocket | 30021 | Deployment manager |
| Cluster | Kubernetes | 30502 | Cluster management |

### Dock (Fixed at bottom)
4 primary applications always accessible:

| App | Icon | Port | Description |
|-----|------|------|-------------|
| Chat | Speech bubble | 30003 | Chat Hub |
| Store | Device | 30002 | App Store |
| Settings | Gear | 30600 | Settings Web |
| Files | Folder | 30088 | File manager |

### Page Indicator
- Dot indicators showing current page
- Supports swipe/scroll between pages

### App Modal
- Full-screen modal for launched applications
- Header with app name and close button
- Loading spinner during iframe load
- Iframe-based app rendering

### Theme
- Catppuccin color palette
- Dark background with gradient overlays
- iOS-style app icons with glossy highlights
- 60x60px icons with 14px border radius
- Press animation (scale to 0.88)

### Responsive Design
- < 380px: Smaller icons (54px) and spacing
- > 600px: 5-column grid
- > 768px: 6-column grid

## API Endpoints

### GET /
Serves the main iPhone-style home screen UI.

### GET /api/apps
Lists all available applications.

**Response:**
```json
{
  "status": "ok",
  "host": "192.168.8.197",
  "apps": [
    "calculator", "notes", "video", "clock", "maps", "photos",
    "browser", "music", "mail", "reminders", "contacts",
    "chat", "store", "settings", "files", "git"
  ]
}
```

### GET /api/status
Returns system status.

**Response:**
```json
{
  "status": "running",
  "version": "2.0.0",
  "host": "192.168.8.197"
}
```

### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "healthy"
}
```

### GET /ready
Readiness check endpoint.

**Response:**
```json
{
  "status": "ready"
}
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CLUSTER_HOST` | 192.168.8.197 | Cluster host IP for app URLs |
| `PORT` | 8080 | Server port |
| `DEBUG` | false | Enable debug mode |

## App Launch Mechanism

When an app icon is tapped:
1. Modal overlay appears with app name
2. Loading spinner displays while iframe loads
3. Iframe src is set to `http://{CLUSTER_HOST}:{port}`
4. Close button (X) or Escape key closes the modal

Apps without a configured port show a "Coming soon!" message.

## Color Scheme

App icons use gradient backgrounds inspired by iOS:
- Calculator: Dark gray
- Notes: Yellow/gold
- Video: Red
- Clock: Black
- Maps: Green
- Photos: Orange
- Browser: Blue
- Music: Pink/red
- Mail: Blue gradient
- Reminders: Orange

System apps use Catppuccin colors:
- Chat: Green/teal gradient
- Store: Blue/sapphire gradient
- Settings: Gray surface gradient
- Files: Blue/sapphire gradient
- Git: Orange/red gradient
- Cluster: Purple/pink gradient

## Screenshot Description

The interface mimics an iPhone home screen with a dark Catppuccin theme. The top status bar shows time, "HolmOS" branding, and battery indicator. Below is a grid of colorful app icons with iOS-style glossy highlights and rounded corners. Each icon has a name label beneath it. A "Developer Tools" section header separates the main apps from dev tools. At the bottom, a translucent dock with blur effect holds the four primary apps (Chat, Store, Settings, Files). Page dots indicate multiple screens are available.
