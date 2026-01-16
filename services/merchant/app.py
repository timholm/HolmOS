"""
Claude Terminal - SSH Terminal with Claude Code-style UI for HolmOS

A terminal-style interface that connects via SSH to cluster nodes,
with built-in context about HolmOS development workflow.
"""

import os
import json
import uuid
import threading
import time
from datetime import datetime
from flask import Flask, request, jsonify, render_template_string
from flask_cors import CORS
import paramiko
import select

app = Flask(__name__)
CORS(app)

# Configuration
SSH_HOST = os.environ.get("SSH_HOST", "192.168.8.197")
SSH_PORT = int(os.environ.get("SSH_PORT", "22"))
SSH_USER = os.environ.get("SSH_USER", "rpi1")
SSH_PASSWORD = os.environ.get("SSH_PASSWORD", "19209746")

# Session storage for SSH connections
ssh_sessions = {}
session_lock = threading.Lock()

# HolmOS Development Context Preset
HOLMOS_CONTEXT = """
# HolmOS Development Workflow

## Building Apps

HolmOS apps are typically Flask (Python) or Go services:

### Flask App Structure:
```
my-app/
  app.py          # Main Flask application
  requirements.txt # Python dependencies (flask, flask-cors, etc.)
  Dockerfile       # Container build instructions
  deployment.yaml  # Kubernetes deployment + service
```

### Go App Structure:
```
my-app/
  main.go          # Main Go application
  go.mod           # Go module file
  Dockerfile       # Multi-stage build
  deployment.yaml  # Kubernetes deployment + service
```

## Building with Buildah

```bash
# Navigate to service directory
cd /path/to/service

# Build for ARM64 (Raspberry Pi cluster)
buildah build --platform linux/arm64 -t localhost:31500/my-app:latest .

# Push to cluster registry
buildah push localhost:31500/my-app:latest
```

## Registry

- Local registry: localhost:31500 (or 192.168.8.197:31500)
- Images are stored in the cluster's internal registry
- Use `buildah push` to publish images

## Deploying to Cluster

```bash
# Apply deployment
kubectl apply -f deployment.yaml

# Check deployment status
kubectl get pods -n holm
kubectl logs -n holm deployment/my-app

# Restart deployment
kubectl rollout restart deployment/my-app -n holm
```

## Common Kubernetes Commands

```bash
# Get all pods in holm namespace
kubectl get pods -n holm

# Get all deployments
kubectl get deployments -n holm

# Get all services
kubectl get svc -n holm

# Describe a pod
kubectl describe pod <pod-name> -n holm

# View logs
kubectl logs -f deployment/<name> -n holm

# Delete and recreate
kubectl delete -f deployment.yaml && kubectl apply -f deployment.yaml
```

## NodePort Ranges

- 30000-30100: Core services (auth, etc.)
- 30100-30500: Applications
- 30500-30700: Developer tools
- 30700-30900: Utilities
- 31500: Container registry

## Cluster Nodes

- rpi1 (192.168.8.197): Control plane
- rpi2-rpi12: Worker nodes
- All running ARM64 architecture
"""

# Terminal UI with Claude Code styling
TERMINAL_UI = '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Claude Terminal - HolmOS</title>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --base: #1e1e2e;
            --mantle: #181825;
            --crust: #11111b;
            --surface0: #313244;
            --surface1: #45475a;
            --surface2: #585b70;
            --overlay0: #6c7086;
            --overlay1: #7f849c;
            --text: #cdd6f4;
            --subtext0: #a6adc8;
            --subtext1: #bac2de;
            --lavender: #b4befe;
            --blue: #89b4fa;
            --sapphire: #74c7ec;
            --sky: #89dceb;
            --teal: #94e2d5;
            --green: #a6e3a1;
            --yellow: #f9e2af;
            --peach: #fab387;
            --maroon: #eba0ac;
            --red: #f38ba8;
            --mauve: #cba6f7;
            --pink: #f5c2e7;
            --flamingo: #f2cdcd;
            --rosewater: #f5e0dc;
            --claude-orange: #da7756;
        }

        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            background: var(--crust);
            color: var(--text);
            font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
        }

        /* Header */
        .header {
            background: var(--mantle);
            padding: 12px 20px;
            border-bottom: 1px solid var(--surface0);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .header-left {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .logo {
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .logo-icon {
            width: 32px;
            height: 32px;
            background: linear-gradient(135deg, var(--claude-orange), var(--peach));
            border-radius: 8px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-weight: 700;
            font-size: 14px;
            color: var(--crust);
        }

        .logo-text {
            font-weight: 600;
            font-size: 16px;
            color: var(--text);
        }

        .logo-subtitle {
            font-size: 11px;
            color: var(--subtext0);
        }

        .connection-status {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 6px 12px;
            background: var(--surface0);
            border-radius: 20px;
            font-size: 12px;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--overlay0);
        }

        .status-dot.connected {
            background: var(--green);
            box-shadow: 0 0 8px var(--green);
        }

        .status-dot.connecting {
            background: var(--yellow);
            animation: pulse 1s infinite;
        }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }

        .header-right {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .header-btn {
            padding: 8px 14px;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            border-radius: 8px;
            color: var(--text);
            font-size: 12px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s;
            display: flex;
            align-items: center;
            gap: 6px;
        }

        .header-btn:hover {
            background: var(--surface1);
            border-color: var(--surface2);
        }

        .header-btn.primary {
            background: var(--claude-orange);
            border-color: var(--claude-orange);
            color: var(--crust);
        }

        .header-btn.primary:hover {
            opacity: 0.9;
        }

        /* Main Layout */
        .main-container {
            display: flex;
            flex: 1;
            overflow: hidden;
        }

        /* Sidebar */
        .sidebar {
            width: 280px;
            background: var(--mantle);
            border-right: 1px solid var(--surface0);
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }

        .sidebar-section {
            padding: 16px;
            border-bottom: 1px solid var(--surface0);
        }

        .sidebar-title {
            font-size: 11px;
            font-weight: 600;
            color: var(--subtext0);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 12px;
        }

        .host-selector {
            width: 100%;
            padding: 10px 12px;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            border-radius: 8px;
            color: var(--text);
            font-size: 13px;
            font-family: 'JetBrains Mono', monospace;
            cursor: pointer;
        }

        .host-selector:focus {
            outline: none;
            border-color: var(--claude-orange);
        }

        .quick-commands {
            display: flex;
            flex-direction: column;
            gap: 6px;
        }

        .quick-cmd {
            padding: 10px 12px;
            background: var(--surface0);
            border: 1px solid transparent;
            border-radius: 8px;
            color: var(--text);
            font-size: 12px;
            font-family: 'JetBrains Mono', monospace;
            cursor: pointer;
            text-align: left;
            transition: all 0.2s;
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .quick-cmd:hover {
            background: var(--surface1);
            border-color: var(--surface2);
        }

        .quick-cmd .icon {
            font-size: 14px;
        }

        .quick-cmd .label {
            flex: 1;
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        /* Context Panel */
        .context-panel {
            flex: 1;
            padding: 16px;
            overflow-y: auto;
        }

        .context-content {
            font-size: 12px;
            line-height: 1.6;
            color: var(--subtext1);
            font-family: 'JetBrains Mono', monospace;
            white-space: pre-wrap;
        }

        .context-content h1 {
            font-size: 14px;
            color: var(--claude-orange);
            margin-bottom: 12px;
        }

        .context-content h2 {
            font-size: 13px;
            color: var(--blue);
            margin-top: 16px;
            margin-bottom: 8px;
        }

        .context-content h3 {
            font-size: 12px;
            color: var(--lavender);
            margin-top: 12px;
            margin-bottom: 6px;
        }

        .context-content code {
            background: var(--surface0);
            padding: 2px 6px;
            border-radius: 4px;
            font-size: 11px;
        }

        .context-content pre {
            background: var(--surface0);
            padding: 12px;
            border-radius: 8px;
            margin: 8px 0;
            overflow-x: auto;
        }

        .context-content pre code {
            background: none;
            padding: 0;
        }

        /* Terminal Area */
        .terminal-area {
            flex: 1;
            display: flex;
            flex-direction: column;
            background: var(--base);
        }

        .terminal-header {
            padding: 8px 16px;
            background: var(--mantle);
            border-bottom: 1px solid var(--surface0);
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .terminal-tabs {
            display: flex;
            gap: 4px;
        }

        .terminal-tab {
            padding: 6px 14px;
            background: var(--surface0);
            border: 1px solid transparent;
            border-radius: 6px;
            color: var(--subtext1);
            font-size: 12px;
            cursor: pointer;
            display: flex;
            align-items: center;
            gap: 6px;
            transition: all 0.2s;
        }

        .terminal-tab:hover {
            background: var(--surface1);
        }

        .terminal-tab.active {
            background: var(--claude-orange);
            color: var(--crust);
        }

        .terminal-tab .close {
            font-size: 14px;
            opacity: 0.7;
        }

        .terminal-tab .close:hover {
            opacity: 1;
        }

        .terminal-output {
            flex: 1;
            padding: 16px;
            overflow-y: auto;
            font-family: 'JetBrains Mono', monospace;
            font-size: 13px;
            line-height: 1.5;
            background: var(--base);
        }

        .output-line {
            margin-bottom: 2px;
            word-wrap: break-word;
        }

        .output-line.command {
            color: var(--green);
        }

        .output-line.command::before {
            content: "$ ";
            color: var(--claude-orange);
        }

        .output-line.output {
            color: var(--text);
        }

        .output-line.error {
            color: var(--red);
        }

        .output-line.system {
            color: var(--blue);
            font-style: italic;
        }

        .output-line.prompt {
            color: var(--claude-orange);
        }

        /* Input Area */
        .input-area {
            padding: 16px;
            background: var(--mantle);
            border-top: 1px solid var(--surface0);
        }

        .input-container {
            display: flex;
            gap: 12px;
            align-items: flex-end;
        }

        .input-wrapper {
            flex: 1;
            display: flex;
            align-items: center;
            background: var(--surface0);
            border: 2px solid var(--surface1);
            border-radius: 12px;
            padding: 0 16px;
            transition: all 0.2s;
        }

        .input-wrapper:focus-within {
            border-color: var(--claude-orange);
        }

        .input-prompt {
            color: var(--claude-orange);
            font-family: 'JetBrains Mono', monospace;
            font-weight: 600;
            margin-right: 8px;
        }

        .command-input {
            flex: 1;
            padding: 14px 0;
            background: transparent;
            border: none;
            color: var(--text);
            font-family: 'JetBrains Mono', monospace;
            font-size: 14px;
            outline: none;
        }

        .command-input::placeholder {
            color: var(--overlay0);
        }

        .send-btn {
            padding: 14px 20px;
            background: var(--claude-orange);
            border: none;
            border-radius: 12px;
            color: var(--crust);
            font-weight: 600;
            font-size: 14px;
            cursor: pointer;
            transition: all 0.2s;
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .send-btn:hover {
            opacity: 0.9;
        }

        .send-btn:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }

        /* Scrollbar */
        ::-webkit-scrollbar {
            width: 8px;
        }

        ::-webkit-scrollbar-track {
            background: var(--mantle);
        }

        ::-webkit-scrollbar-thumb {
            background: var(--surface2);
            border-radius: 4px;
        }

        ::-webkit-scrollbar-thumb:hover {
            background: var(--overlay0);
        }

        /* Toast */
        .toast-container {
            position: fixed;
            top: 20px;
            right: 20px;
            z-index: 1000;
            display: flex;
            flex-direction: column;
            gap: 8px;
        }

        .toast {
            padding: 12px 20px;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            border-radius: 10px;
            font-size: 13px;
            animation: slideIn 0.3s ease;
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .toast.success { border-left: 3px solid var(--green); }
        .toast.error { border-left: 3px solid var(--red); }
        .toast.info { border-left: 3px solid var(--blue); }

        @keyframes slideIn {
            from { transform: translateX(100%); opacity: 0; }
            to { transform: translateX(0); opacity: 1; }
        }

        /* Responsive */
        @media (max-width: 900px) {
            .sidebar {
                display: none;
            }
        }

        /* Welcome screen */
        .welcome-screen {
            flex: 1;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            padding: 40px;
            text-align: center;
        }

        .welcome-logo {
            width: 80px;
            height: 80px;
            background: linear-gradient(135deg, var(--claude-orange), var(--peach));
            border-radius: 20px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 32px;
            font-weight: 700;
            color: var(--crust);
            margin-bottom: 24px;
        }

        .welcome-title {
            font-size: 24px;
            font-weight: 700;
            margin-bottom: 8px;
        }

        .welcome-subtitle {
            color: var(--subtext0);
            margin-bottom: 32px;
            max-width: 400px;
            line-height: 1.6;
        }

        .welcome-actions {
            display: flex;
            gap: 12px;
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="header-left">
            <div class="logo">
                <div class="logo-icon">CT</div>
                <div>
                    <div class="logo-text">Claude Terminal</div>
                    <div class="logo-subtitle">HolmOS Development Shell</div>
                </div>
            </div>
        </div>
        <div class="connection-status" id="connectionStatus">
            <span class="status-dot" id="statusDot"></span>
            <span id="statusText">Disconnected</span>
        </div>
        <div class="header-right">
            <button class="header-btn" onclick="toggleContext()">
                <span>Context</span>
            </button>
            <button class="header-btn primary" id="connectBtn" onclick="connect()">
                <span>Connect</span>
            </button>
        </div>
    </div>

    <div class="main-container">
        <div class="sidebar" id="sidebar">
            <div class="sidebar-section">
                <div class="sidebar-title">Connection</div>
                <select class="host-selector" id="hostSelect">
                    <option value="192.168.8.197">rpi1 - Control Plane (192.168.8.197)</option>
                    <option value="192.168.8.198">rpi2 (192.168.8.198)</option>
                    <option value="192.168.8.199">rpi3 (192.168.8.199)</option>
                    <option value="192.168.8.200">rpi4 (192.168.8.200)</option>
                    <option value="192.168.8.201">rpi5 (192.168.8.201)</option>
                    <option value="192.168.8.202">rpi6 (192.168.8.202)</option>
                    <option value="192.168.8.203">rpi7 (192.168.8.203)</option>
                    <option value="192.168.8.204">rpi8 (192.168.8.204)</option>
                    <option value="192.168.8.205">rpi9 (192.168.8.205)</option>
                    <option value="192.168.8.206">rpi10 (192.168.8.206)</option>
                    <option value="192.168.8.207">rpi11 (192.168.8.207)</option>
                    <option value="192.168.8.208">rpi12 (192.168.8.208)</option>
                </select>
            </div>

            <div class="sidebar-section">
                <div class="sidebar-title">Quick Commands</div>
                <div class="quick-commands">
                    <button class="quick-cmd" onclick="runQuickCmd('kubectl get pods -n holm')">
                        <span class="icon">K</span>
                        <span class="label">Get Pods</span>
                    </button>
                    <button class="quick-cmd" onclick="runQuickCmd('kubectl get deployments -n holm')">
                        <span class="icon">D</span>
                        <span class="label">Get Deployments</span>
                    </button>
                    <button class="quick-cmd" onclick="runQuickCmd('kubectl get svc -n holm')">
                        <span class="icon">S</span>
                        <span class="label">Get Services</span>
                    </button>
                    <button class="quick-cmd" onclick="runQuickCmd('kubectl get nodes -o wide')">
                        <span class="icon">N</span>
                        <span class="label">Get Nodes</span>
                    </button>
                    <button class="quick-cmd" onclick="runQuickCmd('buildah images')">
                        <span class="icon">I</span>
                        <span class="label">List Images</span>
                    </button>
                    <button class="quick-cmd" onclick="runQuickCmd('df -h')">
                        <span class="icon">F</span>
                        <span class="label">Disk Usage</span>
                    </button>
                    <button class="quick-cmd" onclick="runQuickCmd('free -h')">
                        <span class="icon">M</span>
                        <span class="label">Memory Usage</span>
                    </button>
                    <button class="quick-cmd" onclick="runQuickCmd('top -bn1 | head -20')">
                        <span class="icon">T</span>
                        <span class="label">Top Processes</span>
                    </button>
                </div>
            </div>

            <div class="context-panel" id="contextPanel">
                <div class="sidebar-title">HolmOS Context</div>
                <div class="context-content" id="contextContent"></div>
            </div>
        </div>

        <div class="terminal-area">
            <div class="terminal-header">
                <div class="terminal-tabs">
                    <div class="terminal-tab active" id="terminalTab">
                        <span>Terminal</span>
                    </div>
                </div>
            </div>

            <div class="terminal-output" id="terminalOutput">
                <div class="welcome-screen" id="welcomeScreen">
                    <div class="welcome-logo">CT</div>
                    <div class="welcome-title">Claude Terminal</div>
                    <div class="welcome-subtitle">
                        SSH terminal for HolmOS cluster development.
                        Connect to any node and execute commands with
                        built-in context about the development workflow.
                    </div>
                    <div class="welcome-actions">
                        <button class="header-btn primary" onclick="connect()">Connect to Cluster</button>
                    </div>
                </div>
            </div>

            <div class="input-area">
                <div class="input-container">
                    <div class="input-wrapper">
                        <span class="input-prompt">$</span>
                        <input type="text" class="command-input" id="commandInput"
                            placeholder="Enter command..." autocomplete="off"
                            onkeydown="handleKeydown(event)" disabled>
                    </div>
                    <button class="send-btn" id="sendBtn" onclick="sendCommand()" disabled>
                        Run
                    </button>
                </div>
            </div>
        </div>
    </div>

    <div class="toast-container" id="toastContainer"></div>

    <script>
        let sessionId = null;
        let isConnected = false;
        let commandHistory = [];
        let historyIndex = -1;

        // HolmOS Context (loaded from server)
        const holmosContext = `''' + HOLMOS_CONTEXT.replace('`', '\\`').replace("'", "\\'") + '''`;

        // Initialize context panel
        document.getElementById('contextContent').innerHTML = formatMarkdown(holmosContext);

        function formatMarkdown(text) {
            return text
                .replace(/^### (.+)$/gm, '<h3>$1</h3>')
                .replace(/^## (.+)$/gm, '<h2>$1</h2>')
                .replace(/^# (.+)$/gm, '<h1>$1</h1>')
                .replace(/```([\\s\\S]*?)```/g, '<pre><code>$1</code></pre>')
                .replace(/`([^`]+)`/g, '<code>$1</code>')
                .replace(/\\*\\*(.+?)\\*\\*/g, '<strong>$1</strong>')
                .replace(/\\n/g, '<br>');
        }

        function showToast(message, type = 'info') {
            const container = document.getElementById('toastContainer');
            const toast = document.createElement('div');
            toast.className = 'toast ' + type;
            toast.textContent = message;
            container.appendChild(toast);
            setTimeout(() => toast.remove(), 4000);
        }

        function updateConnectionStatus(status, text) {
            const dot = document.getElementById('statusDot');
            const statusText = document.getElementById('statusText');
            const connectBtn = document.getElementById('connectBtn');

            dot.className = 'status-dot ' + status;
            statusText.textContent = text;

            if (status === 'connected') {
                connectBtn.textContent = 'Disconnect';
                connectBtn.classList.remove('primary');
            } else {
                connectBtn.textContent = 'Connect';
                connectBtn.classList.add('primary');
            }
        }

        async function connect() {
            if (isConnected) {
                disconnect();
                return;
            }

            const host = document.getElementById('hostSelect').value;
            updateConnectionStatus('connecting', 'Connecting...');

            try {
                const response = await fetch('/api/connect', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ host })
                });

                const data = await response.json();

                if (data.success) {
                    sessionId = data.session_id;
                    isConnected = true;
                    updateConnectionStatus('connected', 'Connected to ' + host);

                    // Hide welcome, show terminal
                    document.getElementById('welcomeScreen').style.display = 'none';
                    document.getElementById('commandInput').disabled = false;
                    document.getElementById('sendBtn').disabled = false;
                    document.getElementById('commandInput').focus();

                    addOutput('Connected to ' + host, 'system');
                    addOutput('Type commands below. Use quick commands in sidebar for common operations.', 'system');

                    showToast('Connected to ' + host, 'success');
                } else {
                    updateConnectionStatus('', 'Connection failed');
                    showToast(data.error || 'Connection failed', 'error');
                }
            } catch (error) {
                updateConnectionStatus('', 'Connection error');
                showToast('Connection error: ' + error.message, 'error');
            }
        }

        function disconnect() {
            if (sessionId) {
                fetch('/api/disconnect', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ session_id: sessionId })
                });
            }

            sessionId = null;
            isConnected = false;
            updateConnectionStatus('', 'Disconnected');
            document.getElementById('commandInput').disabled = true;
            document.getElementById('sendBtn').disabled = true;
            addOutput('Disconnected', 'system');
            showToast('Disconnected', 'info');
        }

        async function sendCommand() {
            const input = document.getElementById('commandInput');
            const command = input.value.trim();

            if (!command || !isConnected) return;

            // Add to history
            commandHistory.push(command);
            historyIndex = commandHistory.length;

            addOutput(command, 'command');
            input.value = '';

            try {
                const response = await fetch('/api/execute', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        session_id: sessionId,
                        command: command
                    })
                });

                const data = await response.json();

                if (data.success) {
                    if (data.output) {
                        addOutput(data.output, 'output');
                    }
                    if (data.error_output) {
                        addOutput(data.error_output, 'error');
                    }
                } else {
                    addOutput('Error: ' + (data.error || 'Command execution failed'), 'error');
                }
            } catch (error) {
                addOutput('Error: ' + error.message, 'error');
            }
        }

        function runQuickCmd(cmd) {
            if (!isConnected) {
                showToast('Connect to a host first', 'error');
                return;
            }

            document.getElementById('commandInput').value = cmd;
            sendCommand();
        }

        function addOutput(text, type) {
            const output = document.getElementById('terminalOutput');
            const line = document.createElement('div');
            line.className = 'output-line ' + type;
            line.textContent = text;
            output.appendChild(line);
            output.scrollTop = output.scrollHeight;
        }

        function handleKeydown(event) {
            if (event.key === 'Enter') {
                sendCommand();
            } else if (event.key === 'ArrowUp') {
                if (historyIndex > 0) {
                    historyIndex--;
                    document.getElementById('commandInput').value = commandHistory[historyIndex] || '';
                }
                event.preventDefault();
            } else if (event.key === 'ArrowDown') {
                if (historyIndex < commandHistory.length - 1) {
                    historyIndex++;
                    document.getElementById('commandInput').value = commandHistory[historyIndex] || '';
                } else {
                    historyIndex = commandHistory.length;
                    document.getElementById('commandInput').value = '';
                }
                event.preventDefault();
            }
        }

        function toggleContext() {
            const sidebar = document.getElementById('sidebar');
            sidebar.style.display = sidebar.style.display === 'none' ? 'flex' : 'none';
        }

        // Auto-reconnect on page load if session exists
        window.addEventListener('beforeunload', () => {
            if (sessionId) {
                navigator.sendBeacon('/api/disconnect', JSON.stringify({ session_id: sessionId }));
            }
        });
    </script>
</body>
</html>
'''


class SSHSession:
    """Manages an SSH session with command execution."""

    def __init__(self, host, port=22, username="rpi1", password=None):
        self.host = host
        self.port = port
        self.username = username
        self.password = password
        self.client = None
        self.connected = False
        self.last_activity = time.time()

    def connect(self):
        """Establish SSH connection."""
        try:
            self.client = paramiko.SSHClient()
            self.client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
            self.client.connect(
                hostname=self.host,
                port=self.port,
                username=self.username,
                password=self.password,
                timeout=10,
                allow_agent=False,
                look_for_keys=False
            )
            self.connected = True
            return True
        except Exception as e:
            self.connected = False
            raise e

    def execute(self, command, timeout=30):
        """Execute a command and return output."""
        if not self.connected or not self.client:
            raise Exception("Not connected")

        self.last_activity = time.time()

        try:
            stdin, stdout, stderr = self.client.exec_command(command, timeout=timeout)

            # Wait for command to complete
            exit_status = stdout.channel.recv_exit_status()

            output = stdout.read().decode('utf-8', errors='replace')
            error_output = stderr.read().decode('utf-8', errors='replace')

            return {
                "output": output.rstrip(),
                "error_output": error_output.rstrip(),
                "exit_status": exit_status
            }
        except Exception as e:
            raise e

    def close(self):
        """Close the SSH connection."""
        if self.client:
            try:
                self.client.close()
            except:
                pass
        self.connected = False


def cleanup_sessions():
    """Clean up inactive sessions."""
    while True:
        time.sleep(60)
        with session_lock:
            to_remove = []
            for sid, session in ssh_sessions.items():
                if time.time() - session.last_activity > 300:  # 5 min timeout
                    session.close()
                    to_remove.append(sid)
            for sid in to_remove:
                del ssh_sessions[sid]


# Start cleanup thread
cleanup_thread = threading.Thread(target=cleanup_sessions, daemon=True)
cleanup_thread.start()


@app.route('/')
def index():
    """Serve the terminal UI."""
    return render_template_string(TERMINAL_UI)


@app.route('/health')
def health():
    """Health check endpoint."""
    return jsonify({
        "status": "healthy",
        "service": "claude-terminal",
        "timestamp": datetime.utcnow().isoformat()
    })


@app.route('/api/connect', methods=['POST'])
def api_connect():
    """Create a new SSH session."""
    data = request.get_json() or {}
    host = data.get('host', SSH_HOST)

    try:
        session = SSHSession(
            host=host,
            port=SSH_PORT,
            username=SSH_USER,
            password=SSH_PASSWORD
        )
        session.connect()

        session_id = str(uuid.uuid4())

        with session_lock:
            ssh_sessions[session_id] = session

        return jsonify({
            "success": True,
            "session_id": session_id,
            "host": host,
            "message": f"Connected to {host}"
        })
    except Exception as e:
        return jsonify({
            "success": False,
            "error": str(e)
        })


@app.route('/api/disconnect', methods=['POST'])
def api_disconnect():
    """Close an SSH session."""
    data = request.get_json() or {}
    session_id = data.get('session_id')

    if not session_id:
        return jsonify({"success": False, "error": "No session ID"})

    with session_lock:
        session = ssh_sessions.pop(session_id, None)
        if session:
            session.close()

    return jsonify({"success": True, "message": "Disconnected"})


@app.route('/api/execute', methods=['POST'])
def api_execute():
    """Execute a command on the SSH session."""
    data = request.get_json() or {}
    session_id = data.get('session_id')
    command = data.get('command', '').strip()

    if not session_id:
        return jsonify({"success": False, "error": "No session ID"})

    if not command:
        return jsonify({"success": False, "error": "No command provided"})

    with session_lock:
        session = ssh_sessions.get(session_id)

    if not session or not session.connected:
        return jsonify({"success": False, "error": "Session not found or disconnected"})

    try:
        result = session.execute(command)
        return jsonify({
            "success": True,
            "output": result["output"],
            "error_output": result["error_output"],
            "exit_status": result["exit_status"]
        })
    except Exception as e:
        return jsonify({
            "success": False,
            "error": str(e)
        })


@app.route('/api/context')
def api_context():
    """Get the HolmOS development context."""
    return jsonify({
        "context": HOLMOS_CONTEXT,
        "sections": [
            "Building Apps",
            "Building with Buildah",
            "Registry",
            "Deploying to Cluster",
            "Common Kubernetes Commands",
            "NodePort Ranges",
            "Cluster Nodes"
        ]
    })


@app.route('/api/sessions')
def api_sessions():
    """Get active sessions (for debugging)."""
    with session_lock:
        sessions_info = []
        for sid, session in ssh_sessions.items():
            sessions_info.append({
                "session_id": sid,
                "host": session.host,
                "connected": session.connected,
                "last_activity": session.last_activity
            })

    return jsonify({
        "sessions": sessions_info,
        "count": len(sessions_info)
    })


if __name__ == "__main__":
    port = int(os.environ.get("PORT", 8080))
    print(f"Claude Terminal starting on port {port}")
    print(f"Default SSH host: {SSH_HOST}")
    app.run(host="0.0.0.0", port=port)
