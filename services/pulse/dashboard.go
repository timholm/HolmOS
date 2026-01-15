package main

func getDashboardHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Pulse - Cluster Health Monitor</title>
    <style>
        /* Catppuccin Mocha Theme */
        :root {
            --ctp-rosewater: #f5e0dc;
            --ctp-flamingo: #f2cdcd;
            --ctp-pink: #f5c2e7;
            --ctp-mauve: #cba6f7;
            --ctp-red: #f38ba8;
            --ctp-maroon: #eba0ac;
            --ctp-peach: #fab387;
            --ctp-yellow: #f9e2af;
            --ctp-green: #a6e3a1;
            --ctp-teal: #94e2d5;
            --ctp-sky: #89dceb;
            --ctp-sapphire: #74c7ec;
            --ctp-blue: #89b4fa;
            --ctp-lavender: #b4befe;
            --ctp-text: #cdd6f4;
            --ctp-subtext1: #bac2de;
            --ctp-subtext0: #a6adc8;
            --ctp-overlay2: #9399b2;
            --ctp-overlay1: #7f849c;
            --ctp-overlay0: #6c7086;
            --ctp-surface2: #585b70;
            --ctp-surface1: #45475a;
            --ctp-surface0: #313244;
            --ctp-base: #1e1e2e;
            --ctp-mantle: #181825;
            --ctp-crust: #11111b;
        }

        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Inter', sans-serif;
            background: var(--ctp-base);
            color: var(--ctp-text);
            min-height: 100vh;
            line-height: 1.5;
        }

        /* Header */
        .header {
            background: var(--ctp-mantle);
            border-bottom: 1px solid var(--ctp-surface0);
            padding: 16px 24px;
            position: sticky;
            top: 0;
            z-index: 100;
        }

        .header-content {
            max-width: 1600px;
            margin: 0 auto;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .header-left {
            display: flex;
            align-items: center;
            gap: 16px;
        }

        .logo {
            width: 40px;
            height: 40px;
            background: linear-gradient(135deg, var(--ctp-red) 0%, var(--ctp-pink) 100%);
            border-radius: 12px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 20px;
            animation: pulse-glow 2s ease-in-out infinite;
        }

        @keyframes pulse-glow {
            0%, 100% { box-shadow: 0 0 20px rgba(243, 139, 168, 0.3); }
            50% { box-shadow: 0 0 30px rgba(243, 139, 168, 0.5); }
        }

        .header-title h1 {
            font-size: 20px;
            font-weight: 600;
            color: var(--ctp-text);
        }

        .header-title p {
            font-size: 12px;
            color: var(--ctp-subtext0);
        }

        .header-right {
            display: flex;
            align-items: center;
            gap: 16px;
        }

        .status-indicator {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 8px 16px;
            background: var(--ctp-surface0);
            border-radius: 20px;
            font-size: 13px;
            font-weight: 500;
        }

        .status-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            animation: pulse-dot 2s ease-in-out infinite;
        }

        .status-dot.healthy { background: var(--ctp-green); }
        .status-dot.warning { background: var(--ctp-yellow); }
        .status-dot.degraded { background: var(--ctp-peach); }
        .status-dot.critical { background: var(--ctp-red); }
        .status-dot.unknown { background: var(--ctp-overlay0); }

        @keyframes pulse-dot {
            0%, 100% { opacity: 1; transform: scale(1); }
            50% { opacity: 0.7; transform: scale(1.1); }
        }

        .connection-status {
            font-size: 12px;
            color: var(--ctp-subtext0);
            display: flex;
            align-items: center;
            gap: 6px;
        }

        .connection-status.connected { color: var(--ctp-green); }
        .connection-status.disconnected { color: var(--ctp-red); }

        /* Main Content */
        .main {
            max-width: 1600px;
            margin: 0 auto;
            padding: 24px;
        }

        /* Health Score Card */
        .health-score-card {
            background: linear-gradient(135deg, var(--ctp-mantle) 0%, var(--ctp-surface0) 100%);
            border-radius: 20px;
            padding: 32px;
            margin-bottom: 24px;
            border: 1px solid var(--ctp-surface1);
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-wrap: wrap;
            gap: 24px;
        }

        .health-score-main {
            display: flex;
            align-items: center;
            gap: 24px;
        }

        .score-circle {
            width: 120px;
            height: 120px;
            position: relative;
        }

        .score-circle svg {
            width: 100%;
            height: 100%;
            transform: rotate(-90deg);
        }

        .score-circle .bg {
            stroke: var(--ctp-surface1);
        }

        .score-circle .progress {
            stroke-linecap: round;
            transition: stroke-dashoffset 0.5s ease, stroke 0.3s ease;
        }

        .score-circle .progress.healthy { stroke: var(--ctp-green); }
        .score-circle .progress.warning { stroke: var(--ctp-yellow); }
        .score-circle .progress.degraded { stroke: var(--ctp-peach); }
        .score-circle .progress.critical { stroke: var(--ctp-red); }

        .score-value {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            font-size: 32px;
            font-weight: 700;
        }

        .health-info h2 {
            font-size: 28px;
            font-weight: 600;
            margin-bottom: 4px;
        }

        .health-info p {
            font-size: 15px;
            color: var(--ctp-subtext0);
        }

        .health-info .message {
            color: var(--ctp-green);
            font-weight: 500;
            margin-top: 8px;
        }

        .health-info .message.warning { color: var(--ctp-yellow); }
        .health-info .message.degraded { color: var(--ctp-peach); }
        .health-info .message.critical { color: var(--ctp-red); }

        .vital-signs {
            display: flex;
            gap: 12px;
            flex-wrap: wrap;
        }

        .vital-sign {
            background: var(--ctp-surface0);
            padding: 12px 16px;
            border-radius: 12px;
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 13px;
        }

        .vital-sign .icon { font-size: 16px; }
        .vital-sign.healthy .status { color: var(--ctp-green); }
        .vital-sign.unhealthy .status { color: var(--ctp-red); }

        /* Grid Layout */
        .dashboard-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 20px;
            margin-bottom: 24px;
        }

        @media (max-width: 900px) {
            .dashboard-grid {
                grid-template-columns: 1fr;
            }
        }

        /* Cards */
        .card {
            background: var(--ctp-mantle);
            border-radius: 16px;
            border: 1px solid var(--ctp-surface0);
            overflow: hidden;
        }

        .card-header {
            padding: 16px 20px;
            border-bottom: 1px solid var(--ctp-surface0);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .card-title {
            font-size: 15px;
            font-weight: 600;
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .card-title .icon {
            width: 28px;
            height: 28px;
            border-radius: 8px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 14px;
        }

        .card-title .icon.nodes { background: var(--ctp-blue); color: var(--ctp-crust); }
        .card-title .icon.resources { background: var(--ctp-green); color: var(--ctp-crust); }
        .card-title .icon.pods { background: var(--ctp-mauve); color: var(--ctp-crust); }
        .card-title .icon.alerts { background: var(--ctp-red); color: var(--ctp-crust); }

        .card-content {
            padding: 16px 20px;
        }

        /* Stats Grid */
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 16px;
            margin-bottom: 24px;
        }

        @media (max-width: 1200px) {
            .stats-grid {
                grid-template-columns: repeat(2, 1fr);
            }
        }

        .stat-card {
            background: var(--ctp-mantle);
            border-radius: 16px;
            padding: 20px;
            border: 1px solid var(--ctp-surface0);
        }

        .stat-card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
        }

        .stat-label {
            font-size: 13px;
            color: var(--ctp-subtext0);
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }

        .stat-icon {
            width: 32px;
            height: 32px;
            border-radius: 10px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 16px;
        }

        .stat-icon.blue { background: rgba(137, 180, 250, 0.2); color: var(--ctp-blue); }
        .stat-icon.green { background: rgba(166, 227, 161, 0.2); color: var(--ctp-green); }
        .stat-icon.mauve { background: rgba(203, 166, 247, 0.2); color: var(--ctp-mauve); }
        .stat-icon.peach { background: rgba(250, 179, 135, 0.2); color: var(--ctp-peach); }

        .stat-value {
            font-size: 28px;
            font-weight: 700;
            margin-bottom: 4px;
        }

        .stat-detail {
            font-size: 12px;
            color: var(--ctp-subtext0);
        }

        /* Node List */
        .node-list {
            display: flex;
            flex-direction: column;
            gap: 8px;
        }

        .node-item {
            background: var(--ctp-surface0);
            border-radius: 12px;
            padding: 14px 16px;
            display: flex;
            align-items: center;
            justify-content: space-between;
            transition: background 0.2s;
        }

        .node-item:hover {
            background: var(--ctp-surface1);
        }

        .node-info {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .node-status-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
        }

        .node-status-dot.ready { background: var(--ctp-green); }
        .node-status-dot.not-ready { background: var(--ctp-red); }

        .node-name {
            font-weight: 500;
            font-size: 14px;
        }

        .node-pods {
            font-size: 12px;
            color: var(--ctp-subtext0);
        }

        .node-metrics {
            display: flex;
            gap: 16px;
        }

        .node-metric {
            text-align: right;
        }

        .node-metric-value {
            font-size: 13px;
            font-weight: 600;
        }

        .node-metric-label {
            font-size: 10px;
            color: var(--ctp-subtext0);
            text-transform: uppercase;
        }

        /* Resource Bars */
        .resource-section {
            margin-bottom: 20px;
        }

        .resource-section:last-child {
            margin-bottom: 0;
        }

        .resource-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 8px;
        }

        .resource-label {
            font-size: 13px;
            font-weight: 500;
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .resource-value {
            font-size: 13px;
            color: var(--ctp-subtext0);
        }

        .resource-bar {
            height: 8px;
            background: var(--ctp-surface0);
            border-radius: 4px;
            overflow: hidden;
        }

        .resource-bar-fill {
            height: 100%;
            border-radius: 4px;
            transition: width 0.5s ease;
        }

        .resource-bar-fill.cpu { background: linear-gradient(90deg, var(--ctp-blue), var(--ctp-sapphire)); }
        .resource-bar-fill.memory { background: linear-gradient(90deg, var(--ctp-green), var(--ctp-teal)); }
        .resource-bar-fill.warning { background: linear-gradient(90deg, var(--ctp-yellow), var(--ctp-peach)); }
        .resource-bar-fill.critical { background: linear-gradient(90deg, var(--ctp-red), var(--ctp-maroon)); }

        /* Pod Issues */
        .pod-list {
            display: flex;
            flex-direction: column;
            gap: 8px;
        }

        .pod-item {
            background: var(--ctp-surface0);
            border-radius: 10px;
            padding: 12px 14px;
            display: flex;
            align-items: center;
            justify-content: space-between;
        }

        .pod-info {
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .pod-phase {
            padding: 3px 8px;
            border-radius: 6px;
            font-size: 10px;
            font-weight: 600;
            text-transform: uppercase;
        }

        .pod-phase.running { background: rgba(166, 227, 161, 0.2); color: var(--ctp-green); }
        .pod-phase.pending { background: rgba(249, 226, 175, 0.2); color: var(--ctp-yellow); }
        .pod-phase.failed { background: rgba(243, 139, 168, 0.2); color: var(--ctp-red); }

        .pod-name {
            font-size: 13px;
            font-weight: 500;
        }

        .pod-namespace {
            font-size: 11px;
            color: var(--ctp-subtext0);
        }

        .pod-restarts {
            font-size: 12px;
            color: var(--ctp-peach);
        }

        /* Alerts */
        .alert-list {
            display: flex;
            flex-direction: column;
            gap: 8px;
        }

        .alert-item {
            background: var(--ctp-surface0);
            border-radius: 10px;
            padding: 12px 14px;
            border-left: 3px solid;
        }

        .alert-item.warning { border-color: var(--ctp-yellow); }
        .alert-item.critical { border-color: var(--ctp-red); }

        .alert-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 4px;
        }

        .alert-severity {
            font-size: 10px;
            font-weight: 600;
            text-transform: uppercase;
            padding: 2px 6px;
            border-radius: 4px;
        }

        .alert-severity.warning { background: rgba(249, 226, 175, 0.2); color: var(--ctp-yellow); }
        .alert-severity.critical { background: rgba(243, 139, 168, 0.2); color: var(--ctp-red); }

        .alert-time {
            font-size: 11px;
            color: var(--ctp-subtext0);
        }

        .alert-message {
            font-size: 13px;
        }

        .alert-node {
            font-size: 11px;
            color: var(--ctp-subtext0);
            margin-top: 4px;
        }

        /* Empty States */
        .empty-state {
            text-align: center;
            padding: 32px;
            color: var(--ctp-subtext0);
        }

        .empty-state .icon {
            font-size: 32px;
            margin-bottom: 12px;
            opacity: 0.5;
        }

        .empty-state p {
            font-size: 14px;
        }

        /* Last Updated */
        .last-updated {
            text-align: center;
            padding: 16px;
            font-size: 12px;
            color: var(--ctp-subtext0);
        }

        /* Loading */
        .loading {
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 48px;
            color: var(--ctp-subtext0);
        }

        .loading-spinner {
            width: 32px;
            height: 32px;
            border: 3px solid var(--ctp-surface1);
            border-top-color: var(--ctp-mauve);
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin-right: 12px;
        }

        @keyframes spin {
            to { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <header class="header">
        <div class="header-content">
            <div class="header-left">
                <div class="logo">&#128147;</div>
                <div class="header-title">
                    <h1>Pulse</h1>
                    <p>Cluster Health Monitor</p>
                </div>
            </div>
            <div class="header-right">
                <div class="status-indicator">
                    <span class="status-dot" id="statusDot"></span>
                    <span id="statusText">Connecting...</span>
                </div>
                <div class="connection-status" id="connectionStatus">
                    <span>&#9679;</span>
                    <span>Connecting...</span>
                </div>
            </div>
        </div>
    </header>

    <main class="main">
        <!-- Health Score Card -->
        <div class="health-score-card">
            <div class="health-score-main">
                <div class="score-circle">
                    <svg viewBox="0 0 100 100">
                        <circle class="bg" cx="50" cy="50" r="45" fill="none" stroke-width="8"/>
                        <circle class="progress" id="scoreProgress" cx="50" cy="50" r="45" fill="none" stroke-width="8"
                            stroke-dasharray="283" stroke-dashoffset="283"/>
                    </svg>
                    <div class="score-value" id="scoreValue">--</div>
                </div>
                <div class="health-info">
                    <h2>Health Score</h2>
                    <p>Overall cluster health status</p>
                    <p class="message" id="healthMessage">Loading...</p>
                </div>
            </div>
            <div class="vital-signs" id="vitalSigns">
                <!-- Vital signs will be populated -->
            </div>
        </div>

        <!-- Stats Grid -->
        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-card-header">
                    <span class="stat-label">Nodes</span>
                    <div class="stat-icon blue">&#128421;</div>
                </div>
                <div class="stat-value" id="nodeCount">--</div>
                <div class="stat-detail" id="nodeDetail">-- ready</div>
            </div>
            <div class="stat-card">
                <div class="stat-card-header">
                    <span class="stat-label">Pods</span>
                    <div class="stat-icon mauve">&#128230;</div>
                </div>
                <div class="stat-value" id="podCount">--</div>
                <div class="stat-detail" id="podDetail">-- running</div>
            </div>
            <div class="stat-card">
                <div class="stat-card-header">
                    <span class="stat-label">CPU</span>
                    <div class="stat-icon green">&#9889;</div>
                </div>
                <div class="stat-value" id="cpuUsage">--%</div>
                <div class="stat-detail" id="cpuDetail">-- / -- cores</div>
            </div>
            <div class="stat-card">
                <div class="stat-card-header">
                    <span class="stat-label">Memory</span>
                    <div class="stat-icon peach">&#128200;</div>
                </div>
                <div class="stat-value" id="memUsage">--%</div>
                <div class="stat-detail" id="memDetail">-- / -- GB</div>
            </div>
        </div>

        <!-- Dashboard Grid -->
        <div class="dashboard-grid">
            <!-- Nodes Card -->
            <div class="card">
                <div class="card-header">
                    <div class="card-title">
                        <div class="icon nodes">&#128421;</div>
                        Node Status
                    </div>
                </div>
                <div class="card-content">
                    <div class="node-list" id="nodeList">
                        <div class="loading">
                            <div class="loading-spinner"></div>
                            Loading nodes...
                        </div>
                    </div>
                </div>
            </div>

            <!-- Resources Card -->
            <div class="card">
                <div class="card-header">
                    <div class="card-title">
                        <div class="icon resources">&#128200;</div>
                        Resource Usage
                    </div>
                </div>
                <div class="card-content">
                    <div class="resource-section">
                        <div class="resource-header">
                            <span class="resource-label">&#9889; CPU Usage</span>
                            <span class="resource-value" id="cpuResourceValue">--%</span>
                        </div>
                        <div class="resource-bar">
                            <div class="resource-bar-fill cpu" id="cpuBar" style="width: 0%"></div>
                        </div>
                    </div>
                    <div class="resource-section">
                        <div class="resource-header">
                            <span class="resource-label">&#128190; Memory Usage</span>
                            <span class="resource-value" id="memResourceValue">--%</span>
                        </div>
                        <div class="resource-bar">
                            <div class="resource-bar-fill memory" id="memBar" style="width: 0%"></div>
                        </div>
                    </div>
                    <div id="nodeResources" style="margin-top: 20px;">
                        <!-- Per-node resources will be populated -->
                    </div>
                </div>
            </div>

            <!-- Pod Issues Card -->
            <div class="card">
                <div class="card-header">
                    <div class="card-title">
                        <div class="icon pods">&#128230;</div>
                        Pod Issues
                    </div>
                </div>
                <div class="card-content">
                    <div class="pod-list" id="podList">
                        <div class="empty-state">
                            <div class="icon">&#10004;</div>
                            <p>All pods healthy</p>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Alerts Card -->
            <div class="card">
                <div class="card-header">
                    <div class="card-title">
                        <div class="icon alerts">&#9888;</div>
                        Active Alerts
                    </div>
                </div>
                <div class="card-content">
                    <div class="alert-list" id="alertList">
                        <div class="empty-state">
                            <div class="icon">&#10004;</div>
                            <p>No active alerts</p>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <div class="last-updated">
            Last updated: <span id="lastUpdated">--</span>
        </div>
    </main>

    <script>
        let ws = null;
        let reconnectAttempts = 0;
        const maxReconnectAttempts = 10;

        function connectWebSocket() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = protocol + '//' + window.location.host + '/ws';

            ws = new WebSocket(wsUrl);

            ws.onopen = function() {
                console.log('WebSocket connected');
                reconnectAttempts = 0;
                updateConnectionStatus(true);
            };

            ws.onmessage = function(event) {
                const data = JSON.parse(event.data);
                if (data.type === 'health_update') {
                    updateDashboard(data.data);
                }
            };

            ws.onclose = function() {
                console.log('WebSocket disconnected');
                updateConnectionStatus(false);
                scheduleReconnect();
            };

            ws.onerror = function(error) {
                console.error('WebSocket error:', error);
            };
        }

        function scheduleReconnect() {
            if (reconnectAttempts < maxReconnectAttempts) {
                reconnectAttempts++;
                const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), 30000);
                console.log('Reconnecting in ' + delay + 'ms...');
                setTimeout(connectWebSocket, delay);
            }
        }

        function updateConnectionStatus(connected) {
            const el = document.getElementById('connectionStatus');
            if (connected) {
                el.className = 'connection-status connected';
                el.innerHTML = '<span>&#9679;</span><span>Live</span>';
            } else {
                el.className = 'connection-status disconnected';
                el.innerHTML = '<span>&#9679;</span><span>Disconnected</span>';
            }
        }

        function updateDashboard(health) {
            // Update status indicator
            const statusDot = document.getElementById('statusDot');
            const statusText = document.getElementById('statusText');
            statusDot.className = 'status-dot ' + health.status;
            statusText.textContent = health.status.charAt(0).toUpperCase() + health.status.slice(1);

            // Update health score
            const score = health.health_score || 0;
            const scoreValue = document.getElementById('scoreValue');
            const scoreProgress = document.getElementById('scoreProgress');
            const healthMessage = document.getElementById('healthMessage');

            scoreValue.textContent = score;
            const circumference = 2 * Math.PI * 45;
            const offset = circumference - (score / 100) * circumference;
            scoreProgress.style.strokeDashoffset = offset;
            scoreProgress.className = 'progress ' + health.status;

            healthMessage.textContent = health.message || 'Loading...';
            healthMessage.className = 'message ' + health.status;

            // Update vital signs
            const vitalSigns = document.getElementById('vitalSigns');
            if (health.vital_signs) {
                const vitals = health.vital_signs;
                vitalSigns.innerHTML = '' +
                    createVitalSign('&#128147;', 'Heartbeat', vitals.heartbeat || 'unknown') +
                    createVitalSign('&#9881;', 'API Server', vitals.api_server_status || 'unknown') +
                    createVitalSign('&#128190;', 'etcd', vitals.etcd_status || 'unknown') +
                    createVitalSign('&#128197;', 'Scheduler', vitals.scheduler_status || 'unknown') +
                    createVitalSign('&#127760;', 'DNS', vitals.dns_status || 'unknown');
            }

            // Update stats
            const ch = health.cluster_health || {};
            document.getElementById('nodeCount').textContent = ch.total_nodes || 0;
            document.getElementById('nodeDetail').textContent = (ch.ready_nodes || 0) + ' ready';
            document.getElementById('podCount').textContent = ch.total_pods || 0;
            document.getElementById('podDetail').textContent = (ch.running_pods || 0) + ' running, ' + (ch.pending_pods || 0) + ' pending';

            const cpuPct = Math.round(ch.cpu_percent || 0);
            document.getElementById('cpuUsage').textContent = cpuPct + '%';
            document.getElementById('cpuDetail').textContent = ((ch.used_cpu_cores || 0) / 1000).toFixed(1) + ' / ' + ((ch.total_cpu_cores || 0) / 1000).toFixed(1) + ' cores';

            const memPct = Math.round(ch.memory_percent || 0);
            document.getElementById('memUsage').textContent = memPct + '%';
            document.getElementById('memDetail').textContent = (ch.used_memory_gb || 0).toFixed(1) + ' / ' + (ch.total_memory_gb || 0).toFixed(1) + ' GB';

            // Update resource bars
            document.getElementById('cpuResourceValue').textContent = cpuPct + '%';
            document.getElementById('cpuBar').style.width = cpuPct + '%';
            updateBarColor('cpuBar', cpuPct);

            document.getElementById('memResourceValue').textContent = memPct + '%';
            document.getElementById('memBar').style.width = memPct + '%';
            updateBarColor('memBar', memPct);

            // Update node list
            const nodeList = document.getElementById('nodeList');
            if (health.node_statuses && health.node_statuses.length > 0) {
                nodeList.innerHTML = health.node_statuses.map(node => {
                    const statusClass = node.ready ? 'ready' : 'not-ready';
                    return '<div class="node-item">' +
                        '<div class="node-info">' +
                            '<div class="node-status-dot ' + statusClass + '"></div>' +
                            '<div>' +
                                '<div class="node-name">' + node.name + '</div>' +
                                '<div class="node-pods">' + node.pod_count + ' pods</div>' +
                            '</div>' +
                        '</div>' +
                        '<div class="node-metrics">' +
                            '<div class="node-metric">' +
                                '<div class="node-metric-value">' + Math.round(node.cpu_percent) + '%</div>' +
                                '<div class="node-metric-label">CPU</div>' +
                            '</div>' +
                            '<div class="node-metric">' +
                                '<div class="node-metric-value">' + Math.round(node.memory_percent) + '%</div>' +
                                '<div class="node-metric-label">Memory</div>' +
                            '</div>' +
                        '</div>' +
                    '</div>';
                }).join('');

                // Update per-node resources
                const nodeResources = document.getElementById('nodeResources');
                nodeResources.innerHTML = health.node_statuses.map(node => {
                    const cpuClass = getResourceClass(node.cpu_percent);
                    const memClass = getResourceClass(node.memory_percent);
                    return '<div class="resource-section">' +
                        '<div class="resource-header">' +
                            '<span class="resource-label">' + node.name + '</span>' +
                            '<span class="resource-value">' + Math.round(node.cpu_percent) + '% / ' + Math.round(node.memory_percent) + '%</span>' +
                        '</div>' +
                        '<div style="display: flex; gap: 8px;">' +
                            '<div class="resource-bar" style="flex: 1;">' +
                                '<div class="resource-bar-fill ' + cpuClass + '" style="width: ' + node.cpu_percent + '%"></div>' +
                            '</div>' +
                            '<div class="resource-bar" style="flex: 1;">' +
                                '<div class="resource-bar-fill ' + memClass + '" style="width: ' + node.memory_percent + '%"></div>' +
                            '</div>' +
                        '</div>' +
                    '</div>';
                }).join('');
            } else {
                nodeList.innerHTML = '<div class="empty-state"><div class="icon">&#128421;</div><p>No nodes found</p></div>';
            }

            // Update pod issues
            const podList = document.getElementById('podList');
            if (health.pod_statuses && health.pod_statuses.length > 0) {
                podList.innerHTML = health.pod_statuses.slice(0, 10).map(pod => {
                    const phaseClass = pod.phase.toLowerCase();
                    return '<div class="pod-item">' +
                        '<div class="pod-info">' +
                            '<span class="pod-phase ' + phaseClass + '">' + pod.phase + '</span>' +
                            '<div>' +
                                '<div class="pod-name">' + pod.name + '</div>' +
                                '<div class="pod-namespace">' + pod.namespace + '</div>' +
                            '</div>' +
                        '</div>' +
                        (pod.restarts > 0 ? '<span class="pod-restarts">' + pod.restarts + ' restarts</span>' : '') +
                    '</div>';
                }).join('');
            } else {
                podList.innerHTML = '<div class="empty-state"><div class="icon">&#10004;</div><p>All pods healthy</p></div>';
            }

            // Update alerts
            const alertList = document.getElementById('alertList');
            if (health.resource_alerts && health.resource_alerts.length > 0) {
                alertList.innerHTML = health.resource_alerts.slice(0, 10).map(alert => {
                    const time = new Date(alert.timestamp).toLocaleTimeString();
                    return '<div class="alert-item ' + alert.severity + '">' +
                        '<div class="alert-header">' +
                            '<span class="alert-severity ' + alert.severity + '">' + alert.severity + '</span>' +
                            '<span class="alert-time">' + time + '</span>' +
                        '</div>' +
                        '<div class="alert-message">' + alert.message + '</div>' +
                        '<div class="alert-node">' + alert.resource + ': ' + Math.round(alert.value) + '% (threshold: ' + alert.threshold + '%)</div>' +
                    '</div>';
                }).join('');
            } else {
                alertList.innerHTML = '<div class="empty-state"><div class="icon">&#10004;</div><p>No active alerts</p></div>';
            }

            // Update timestamp
            document.getElementById('lastUpdated').textContent = new Date(health.timestamp).toLocaleString();
        }

        function createVitalSign(icon, name, status) {
            const statusClass = status === 'healthy' || status === 'normal' ? 'healthy' : 'unhealthy';
            return '<div class="vital-sign ' + statusClass + '">' +
                '<span class="icon">' + icon + '</span>' +
                '<span>' + name + '</span>' +
                '<span class="status">' + (status === 'healthy' || status === 'normal' ? '&#10004;' : '&#10008;') + '</span>' +
            '</div>';
        }

        function updateBarColor(id, value) {
            const bar = document.getElementById(id);
            bar.classList.remove('cpu', 'memory', 'warning', 'critical');
            if (value > 90) {
                bar.classList.add('critical');
            } else if (value > 80) {
                bar.classList.add('warning');
            } else if (id === 'cpuBar') {
                bar.classList.add('cpu');
            } else {
                bar.classList.add('memory');
            }
        }

        function getResourceClass(value) {
            if (value > 90) return 'critical';
            if (value > 80) return 'warning';
            if (value > 50) return 'memory';
            return 'cpu';
        }

        // Initial connection
        connectWebSocket();

        // Fallback to polling if WebSocket fails
        setInterval(function() {
            if (!ws || ws.readyState !== WebSocket.OPEN) {
                fetch('/api/status')
                    .then(res => res.json())
                    .then(data => updateDashboard(data))
                    .catch(err => console.error('Polling error:', err));
            }
        }, 15000);
    </script>
</body>
</html>`
}
