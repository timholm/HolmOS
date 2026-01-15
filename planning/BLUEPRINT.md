# HolmOS - AI-Native Web Operating System

## Vision
A fully web-based operating system running on Kubernetes with AI agents managing every service. Mobile-first iPhone-style UI with an AI App Store that builds apps on demand.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        HolmOS Web Shell                          │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Status Bar                             │   │
│  │  9:41          HolmOS           ⚡ 85%  📶  🔋            │   │
│  ├──────────────────────────────────────────────────────────┤   │
│  │                                                           │   │
│  │   📁 Files    🖥️ Terminal   ⚙️ Settings   🎧 Audio      │   │
│  │                                                           │   │
│  │   🏪 Store    💬 Claude     📊 Cluster    🔐 Auth        │   │
│  │                                                           │   │
│  │   📝 Notes    📷 Photos     🎵 Music      📧 Mail        │   │
│  │                                                           │   │
│  │   🤖 Agents   📦 Registry   🔔 Alerts     📈 Metrics     │   │
│  │                                                           │   │
│  ├──────────────────────────────────────────────────────────┤   │
│  │  [Files]  [Claude]  [Store]  [Settings]                  │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Core Systems

### 1. HolmOS Shell (holmos-shell)
The main web interface - mobile-first, iPhone-style
- Home screen with app grid
- Dock with pinned apps
- Status bar (time, battery, network)
- Gesture navigation (swipe up for home, swipe down for notifications)
- App switcher (swipe up and hold)
- Control center (swipe down from top-right)
- Notification center (swipe down from top-left)

### 2. AI App Store (app-store-ai)
Revolutionary: Don't download apps, describe what you need
- Chat interface: "I need a password manager"
- AI analyzes request, finds/builds appropriate service
- Deploys as new pod with dedicated agent
- Adds icon to home screen
- Each "app" is actually a microservice + AI agent

### 3. Claude Code Pod (claude-pod)
Claude Code running in the cluster
- WebSocket chat interface
- Full access to kubectl, git, code editing
- Can manage all other pods
- Integrated with all services
- Persistent conversation history

### 4. Pod Agent System
Each pod has a named AI agent personality:

| Pod | Agent Name | Personality |
|-----|------------|-------------|
| files | Atlas | File system expert, organized, methodical |
| terminal | Shell | Command line wizard, efficient, terse |
| cluster | Nova | Infrastructure guru, monitors everything |
| auth | Guardian | Security focused, protective |
| settings | Config | Customization expert, remembers preferences |
| audiobook | Narrator | Audio specialist, storyteller |
| registry | Harbor | Container expert, manages images |
| store | Merchant | Finds/builds what you need |
| claude | Claude | General purpose, can do anything |
| metrics | Pulse | Performance analyst, data driven |
| alerts | Sentinel | Watchdog, proactive notifications |
| logs | Scribe | Historian, finds patterns in logs |
| backup | Vault | Data protector, recovery specialist |
| notifications | Herald | Messenger, keeps you informed |
| tasks | Foreman | Work coordinator, queues jobs |
| cache | Flash | Speed optimizer, memory expert |
| secrets | Cipher | Encryption specialist |
| scheduler | Tempo | Timing expert, cron master |
| websocket | Bridge | Real-time communications |
| events | Relay | Pub/sub coordinator |

### 5. Nautilus-Style File Manager
Full-featured file browser:
- Grid view with thumbnails
- List view with details
- Column view (Miller columns)
- Preview pane
- Breadcrumb navigation
- Drag and drop
- Context menus
- Quick actions
- Search with filters
- Favorites sidebar
- Recent files
- Trash

## Service Categories

### System Services (Core OS)
1. holmos-shell - Main UI shell
2. holmos-launcher - App launcher
3. holmos-dock - Bottom dock
4. holmos-statusbar - Top status bar
5. holmos-control-center - Quick settings
6. holmos-notifications - Notification center
7. holmos-search - Spotlight-style search
8. holmos-switcher - App switcher

### AI Services
9. claude-pod - Claude Code instance
10. agent-orchestrator - Manages all agents
11. agent-router - Routes messages to agents
12. app-store-ai - AI app store
13. chat-hub - Central chat interface
14. prompt-library - Saved prompts/commands

### File Services
15. file-web - Main file UI (Nautilus-style)
16. file-list - List files
17. file-upload - Upload files
18. file-download - Download files
19. file-preview - Preview files
20. file-thumbnail - Generate thumbnails
21. file-search - Search files
22. file-compress - Compress files
23. file-share - Share links
24. file-encrypt - Encrypt files
25. file-convert - Convert formats
26. file-watch - Watch for changes
27. file-trash - Trash management
28. file-favorites - Favorites
29. file-recent - Recent files
30. file-tags - File tagging

### Terminal Services
31. terminal-web - Terminal UI
32. terminal-host-list - SSH hosts
33. terminal-host-add - Add hosts
34. terminal-session - Session manager
35. terminal-history - Command history
36. terminal-snippets - Saved commands

### Cluster Services
37. cluster-manager - Cluster UI
38. cluster-node-list - List nodes
39. cluster-node-ping - Ping nodes
40. cluster-apt-update - System updates
41. cluster-reboot-exec - Reboot nodes
42. cluster-metrics - Node metrics
43. cluster-logs - Node logs
44. cluster-events - Cluster events

### Auth Services
45. auth-gateway - Auth UI
46. auth-login - Login
47. auth-logout - Logout
48. auth-register - Registration
49. auth-refresh - Token refresh
50. auth-validate - Token validation
51. auth-2fa - Two-factor auth
52. auth-oauth - OAuth providers
53. auth-sessions - Session management

### Settings Services
54. settings-web - Settings UI
55. settings-theme - Theme manager
56. settings-tabs - Tab preferences
57. settings-backup - Export settings
58. settings-restore - Import settings
59. settings-sync - Cross-device sync
60. settings-privacy - Privacy controls
61. settings-notifications - Notification prefs
62. settings-display - Display settings
63. settings-sounds - Sound settings
64. settings-language - Language/locale

### Audiobook Services
65. audiobook-web - Audiobook UI
66. audiobook-upload-epub - EPUB upload
67. audiobook-upload-txt - TXT upload
68. audiobook-parse-epub - Parse EPUB
69. audiobook-chunk-text - Chunk text
70. audiobook-tts-convert - Text to speech
71. audiobook-audio-concat - Concat audio
72. audiobook-audio-normalize - Normalize audio
73. audiobook-player - Audio player
74. audiobook-library - Library manager
75. audiobook-progress - Progress tracker

### Registry Services
76. registry-ui - Registry browser
77. registry-list-repos - List repos
78. registry-list-tags - List tags
79. registry-push - Push images
80. registry-pull - Pull images
81. registry-cleanup - Garbage collection

### Monitoring Services
82. metrics-collector - Collect metrics
83. metrics-dashboard - Display metrics
84. alerting - Alert rules
85. log-aggregator - Aggregate logs
86. health-checker - Health checks
87. tracing - Distributed tracing
88. profiler - Performance profiling

### Notification Services
89. notification-email - Email notifications
90. notification-webhook - Webhook notifications
91. notification-queue - Notification queue
92. notification-push - Push notifications
93. notification-sms - SMS notifications (future)

### Infrastructure Services
94. api-gateway - Central API gateway
95. rate-limiter - Rate limiting
96. cache-service - Caching layer
97. task-queue - Background tasks
98. websocket-hub - WebSocket connections
99. service-registry - Service discovery
100. config-server - Centralized config
101. event-bus - Event pub/sub
102. cron-scheduler - Scheduled jobs
103. secret-manager - Secret storage

### Productivity Apps
104. notes-app - Notes/documents
105. photos-app - Photo gallery
106. music-app - Music player
107. mail-app - Email client
108. calendar-app - Calendar
109. contacts-app - Contact manager
110. reminders-app - Reminders/todos
111. calculator-app - Calculator
112. weather-app - Weather info

## Implementation Phases

### Phase 1: Core Shell (Week 1)
- holmos-shell (main container)
- holmos-statusbar
- holmos-dock
- holmos-launcher
- Mobile-first CSS framework
- Gesture recognition

### Phase 2: AI Integration (Week 2)
- claude-pod (Claude Code in cluster)
- chat-hub (unified chat interface)
- agent-orchestrator
- agent-router
- WebSocket infrastructure

### Phase 3: App Store AI (Week 3)
- app-store-ai service
- Dynamic pod deployment
- Agent personality system
- App icon generation
- Home screen management

### Phase 4: Enhanced File Manager (Week 4)
- Nautilus-style file-web
- Thumbnail generation
- Preview pane
- Drag and drop
- Context menus

### Phase 5: Agent System (Week 5)
- Named agents for each pod
- Agent chat routing
- Agent collaboration
- Personality prompts
- Context awareness

### Phase 6: Full Integration (Week 6)
- All services connected
- Cross-service workflows
- Unified search
- Notification system
- Settings sync

## Technical Stack

- **Frontend**: Go + HTML templates + Alpine.js + Tailwind CSS
- **Backend**: Go microservices
- **Database**: PostgreSQL
- **Cache**: In-memory + Redis (future)
- **Message Queue**: In-memory event bus
- **Container Runtime**: containerd
- **Orchestration**: Kubernetes
- **Registry**: Docker Registry v2
- **AI**: Claude API via claude-pod

## NodePort Assignments

| Port | Service |
|------|---------|
| 30000 | holmos-shell (main entry) |
| 30001 | claude-pod |
| 30002 | app-store-ai |
| 30003 | chat-hub |
| 30088 | file-web |
| 30089 | terminal |
| 30502 | cluster-manager |
| 30600 | settings-web |
| 30700 | audiobook-web |
| 30800 | auth-gateway |
| 30900 | test-dashboard |
| 30950 | metrics-dashboard |
| 31750 | registry-ui |

## Getting Started

Access HolmOS at: http://192.168.8.197:30000

Main entry point is holmos-shell which provides the iPhone-style interface.
All other services are accessed through this shell.

Chat with any agent by tapping their app icon or using the chat hub.
