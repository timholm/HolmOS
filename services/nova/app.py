from flask import Flask, request, jsonify, render_template_string
import subprocess
import json
import re
import os
from datetime import datetime

app = Flask(__name__)

# Nova's personality
NOVA_NAME = "Nova"
NOVA_CATCHPHRASE = "I see all 13 stars in our constellation."

# Catppuccin Mocha theme colors
CATPPUCCIN = {
    "base": "#1e1e2e",
    "mantle": "#181825",
    "crust": "#11111b",
    "surface0": "#313244",
    "surface1": "#45475a",
    "surface2": "#585b70",
    "overlay0": "#6c7086",
    "overlay1": "#7f849c",
    "text": "#cdd6f4",
    "subtext0": "#a6adc8",
    "subtext1": "#bac2de",
    "lavender": "#b4befe",
    "blue": "#89b4fa",
    "sapphire": "#74c7ec",
    "sky": "#89dceb",
    "teal": "#94e2d5",
    "green": "#a6e3a1",
    "yellow": "#f9e2af",
    "peach": "#fab387",
    "maroon": "#eba0ac",
    "red": "#f38ba8",
    "mauve": "#cba6f7",
    "pink": "#f5c2e7",
    "flamingo": "#f2cdcd",
    "rosewater": "#f5e0dc"
}

DASHBOARD_HTML = '''
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Nova - Cluster Guardian</title>
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
        }
        
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            background: var(--base);
            color: var(--text);
            font-family: 'Inter', sans-serif;
            min-height: 100vh;
            overflow-x: hidden;
        }
        
        .stars-bg {
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            pointer-events: none;
            z-index: 0;
            overflow: hidden;
        }
        
        .star {
            position: absolute;
            background: var(--lavender);
            border-radius: 50%;
            animation: twinkle 3s infinite;
        }
        
        @keyframes twinkle {
            0%, 100% { opacity: 0.3; transform: scale(1); }
            50% { opacity: 1; transform: scale(1.2); }
        }
        
        .container {
            position: relative;
            z-index: 1;
            max-width: 1800px;
            margin: 0 auto;
            padding: 20px;
        }
        
        header {
            text-align: center;
            padding: 30px 0;
            border-bottom: 1px solid var(--surface1);
            margin-bottom: 30px;
        }
        
        .logo {
            font-size: 3.5rem;
            font-weight: 700;
            background: linear-gradient(135deg, var(--mauve), var(--blue), var(--teal));
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
            margin-bottom: 10px;
            text-shadow: 0 0 60px rgba(203, 166, 247, 0.5);
        }
        
        .tagline {
            font-size: 1.1rem;
            color: var(--subtext0);
            font-style: italic;
        }
        
        .status-bar {
            display: flex;
            gap: 15px;
            justify-content: center;
            margin-top: 20px;
            flex-wrap: wrap;
        }
        
        .status-pill {
            background: var(--surface0);
            padding: 8px 20px;
            border-radius: 30px;
            display: flex;
            align-items: center;
            gap: 8px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.9rem;
            border: 1px solid var(--surface1);
            transition: all 0.3s ease;
        }
        
        .status-pill:hover {
            border-color: var(--lavender);
            transform: translateY(-2px);
        }
        
        .status-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            animation: pulse 2s infinite;
        }
        
        .status-dot.green { background: var(--green); box-shadow: 0 0 10px var(--green); }
        .status-dot.yellow { background: var(--yellow); box-shadow: 0 0 10px var(--yellow); }
        .status-dot.red { background: var(--red); box-shadow: 0 0 10px var(--red); }
        
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(350px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        
        .card {
            background: linear-gradient(145deg, var(--surface0), var(--mantle));
            border-radius: 16px;
            padding: 20px;
            border: 1px solid var(--surface1);
            transition: all 0.3s ease;
        }
        
        .card:hover {
            border-color: var(--lavender);
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
            transform: translateY(-3px);
        }
        
        .card-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 15px;
            padding-bottom: 10px;
            border-bottom: 1px solid var(--surface1);
        }
        
        .card-title {
            font-size: 1.1rem;
            font-weight: 600;
            color: var(--lavender);
            display: flex;
            align-items: center;
            gap: 10px;
        }
        
        .card-icon {
            font-size: 1.3rem;
        }
        
        .node-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
            gap: 10px;
        }
        
        .node-card {
            background: var(--surface0);
            border-radius: 12px;
            padding: 12px;
            text-align: center;
            border: 1px solid transparent;
            transition: all 0.3s ease;
            cursor: pointer;
        }
        
        .node-card:hover {
            border-color: var(--blue);
            background: var(--surface1);
        }
        
        .node-card.control-plane {
            border-color: var(--mauve);
        }
        
        .node-card.healthy {
            border-left: 3px solid var(--green);
        }
        
        .node-card.unhealthy {
            border-left: 3px solid var(--red);
        }
        
        .node-name {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.85rem;
            font-weight: 600;
            color: var(--text);
            margin-bottom: 8px;
        }
        
        .node-stats {
            display: flex;
            gap: 8px;
            justify-content: center;
            font-size: 0.75rem;
        }
        
        .node-stat {
            background: var(--mantle);
            padding: 4px 8px;
            border-radius: 6px;
            font-family: 'JetBrains Mono', monospace;
        }
        
        .node-stat.cpu { color: var(--peach); }
        .node-stat.mem { color: var(--blue); }
        
        .progress-bar {
            height: 6px;
            background: var(--surface1);
            border-radius: 3px;
            overflow: hidden;
            margin-top: 8px;
        }
        
        .progress-fill {
            height: 100%;
            border-radius: 3px;
            transition: width 0.5s ease;
        }
        
        .progress-fill.low { background: var(--green); }
        .progress-fill.medium { background: var(--yellow); }
        .progress-fill.high { background: var(--peach); }
        .progress-fill.critical { background: var(--red); }
        
        .deployment-list {
            max-height: 400px;
            overflow-y: auto;
        }
        
        .deployment-list::-webkit-scrollbar {
            width: 6px;
        }
        
        .deployment-list::-webkit-scrollbar-track {
            background: var(--surface0);
            border-radius: 3px;
        }
        
        .deployment-list::-webkit-scrollbar-thumb {
            background: var(--surface2);
            border-radius: 3px;
        }
        
        .deployment-item {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 10px;
            background: var(--surface0);
            border-radius: 8px;
            margin-bottom: 8px;
            border-left: 3px solid var(--surface2);
            transition: all 0.2s ease;
        }
        
        .deployment-item:hover {
            background: var(--surface1);
        }
        
        .deployment-item.healthy { border-left-color: var(--green); }
        .deployment-item.warning { border-left-color: var(--yellow); }
        .deployment-item.error { border-left-color: var(--red); }
        
        .deployment-info {
            flex: 1;
        }
        
        .deployment-name {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.85rem;
            color: var(--text);
        }
        
        .deployment-namespace {
            font-size: 0.7rem;
            color: var(--subtext0);
        }
        
        .deployment-replicas {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.8rem;
            padding: 4px 10px;
            background: var(--mantle);
            border-radius: 12px;
            margin-right: 10px;
        }
        
        .deployment-replicas.full { color: var(--green); }
        .deployment-replicas.partial { color: var(--yellow); }
        .deployment-replicas.zero { color: var(--red); }
        
        .action-btns {
            display: flex;
            gap: 5px;
        }
        
        .action-btn {
            background: var(--surface1);
            border: none;
            color: var(--subtext0);
            padding: 6px 10px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 0.75rem;
            transition: all 0.2s ease;
        }
        
        .action-btn:hover {
            background: var(--surface2);
            color: var(--text);
        }
        
        .action-btn.restart:hover { background: var(--peach); color: var(--crust); }
        .action-btn.scale:hover { background: var(--blue); color: var(--crust); }
        
        .pod-distribution {
            display: flex;
            flex-wrap: wrap;
            gap: 6px;
        }
        
        .pod-node {
            background: var(--surface0);
            padding: 8px 12px;
            border-radius: 8px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.8rem;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        
        .pod-count {
            background: var(--mauve);
            color: var(--crust);
            padding: 2px 8px;
            border-radius: 10px;
            font-weight: 600;
        }
        
        .metric-row {
            display: flex;
            align-items: center;
            gap: 15px;
            padding: 12px;
            background: var(--surface0);
            border-radius: 10px;
            margin-bottom: 10px;
        }
        
        .metric-label {
            min-width: 100px;
            color: var(--subtext0);
            font-size: 0.85rem;
        }
        
        .metric-bar {
            flex: 1;
            height: 24px;
            background: var(--mantle);
            border-radius: 12px;
            overflow: hidden;
            position: relative;
        }
        
        .metric-fill {
            height: 100%;
            border-radius: 12px;
            transition: width 0.5s ease;
            display: flex;
            align-items: center;
            justify-content: flex-end;
            padding-right: 10px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.75rem;
            font-weight: 600;
            color: var(--crust);
        }
        
        .metric-fill.cpu { background: linear-gradient(90deg, var(--peach), var(--yellow)); }
        .metric-fill.mem { background: linear-gradient(90deg, var(--blue), var(--sapphire)); }
        
        .quick-actions {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 10px;
        }
        
        .quick-action {
            background: var(--surface0);
            border: 1px solid var(--surface1);
            color: var(--text);
            padding: 15px;
            border-radius: 12px;
            cursor: pointer;
            text-align: center;
            transition: all 0.3s ease;
            font-size: 0.9rem;
        }
        
        .quick-action:hover {
            border-color: var(--lavender);
            transform: scale(1.02);
        }
        
        .quick-action-icon {
            font-size: 1.5rem;
            margin-bottom: 8px;
            display: block;
        }
        
        .namespace-filter {
            display: flex;
            gap: 8px;
            margin-bottom: 15px;
            flex-wrap: wrap;
        }
        
        .ns-btn {
            background: var(--surface0);
            border: 1px solid var(--surface1);
            color: var(--subtext0);
            padding: 6px 14px;
            border-radius: 20px;
            cursor: pointer;
            font-size: 0.8rem;
            transition: all 0.2s ease;
        }
        
        .ns-btn:hover, .ns-btn.active {
            background: var(--mauve);
            color: var(--crust);
            border-color: var(--mauve);
        }
        
        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0, 0, 0, 0.7);
            z-index: 1000;
            align-items: center;
            justify-content: center;
        }
        
        .modal.active { display: flex; }
        
        .modal-content {
            background: var(--surface0);
            border-radius: 16px;
            padding: 25px;
            max-width: 500px;
            width: 90%;
            border: 1px solid var(--surface1);
        }
        
        .modal-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
        }
        
        .modal-title {
            font-size: 1.2rem;
            color: var(--lavender);
        }
        
        .modal-close {
            background: none;
            border: none;
            color: var(--subtext0);
            font-size: 1.5rem;
            cursor: pointer;
        }
        
        .form-group {
            margin-bottom: 15px;
        }
        
        .form-label {
            display: block;
            margin-bottom: 8px;
            color: var(--subtext1);
            font-size: 0.9rem;
        }
        
        .form-input {
            width: 100%;
            padding: 12px;
            background: var(--mantle);
            border: 1px solid var(--surface1);
            border-radius: 8px;
            color: var(--text);
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.9rem;
        }
        
        .form-input:focus {
            outline: none;
            border-color: var(--lavender);
        }
        
        .form-btn {
            width: 100%;
            padding: 12px;
            background: linear-gradient(135deg, var(--mauve), var(--blue));
            border: none;
            border-radius: 8px;
            color: var(--crust);
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s ease;
        }
        
        .form-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 5px 20px rgba(203, 166, 247, 0.3);
        }
        
        .toast {
            position: fixed;
            bottom: 30px;
            right: 30px;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            padding: 15px 25px;
            border-radius: 12px;
            display: none;
            z-index: 1001;
            animation: slideIn 0.3s ease;
        }
        
        .toast.success { border-left: 4px solid var(--green); }
        .toast.error { border-left: 4px solid var(--red); }
        .toast.show { display: block; }
        
        @keyframes slideIn {
            from { transform: translateX(100px); opacity: 0; }
            to { transform: translateX(0); opacity: 1; }
        }
        
        .refresh-btn {
            position: fixed;
            bottom: 30px;
            left: 30px;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            color: var(--text);
            padding: 15px;
            border-radius: 50%;
            cursor: pointer;
            z-index: 100;
            transition: all 0.3s ease;
        }
        
        .refresh-btn:hover {
            background: var(--mauve);
            color: var(--crust);
            transform: rotate(180deg);
        }
        
        .timestamp {
            text-align: center;
            color: var(--subtext0);
            font-size: 0.8rem;
            margin-top: 20px;
            font-family: 'JetBrains Mono', monospace;
        }
        
        .full-width { grid-column: 1 / -1; }
        
        @media (max-width: 768px) {
            .grid { grid-template-columns: 1fr; }
            .node-grid { grid-template-columns: repeat(2, 1fr); }
        }
    </style>
</head>
<body>
    <div class="stars-bg" id="starsBg"></div>
    
    <div class="container">
        <header>
            <div class="logo">Nova</div>
            <div class="tagline" id="tagline">I see all 13 stars in our constellation.</div>
            <div class="status-bar" id="statusBar">
                <!-- Populated by JS -->
            </div>
        </header>
        
        <div class="grid">
            <!-- Cluster Overview -->
            <div class="card">
                <div class="card-header">
                    <span class="card-title"><span class="card-icon">&#127775;</span> Cluster Health</span>
                    <span id="clusterHealth" style="font-size: 1.5rem;">&#9632;</span>
                </div>
                <div class="metric-row">
                    <span class="metric-label">CPU Usage</span>
                    <div class="metric-bar">
                        <div class="metric-fill cpu" id="cpuBar" style="width: 0%">0%</div>
                    </div>
                </div>
                <div class="metric-row">
                    <span class="metric-label">Memory</span>
                    <div class="metric-bar">
                        <div class="metric-fill mem" id="memBar" style="width: 0%">0%</div>
                    </div>
                </div>
            </div>
            
            <!-- Quick Actions -->
            <div class="card">
                <div class="card-header">
                    <span class="card-title"><span class="card-icon">&#9889;</span> Quick Actions</span>
                </div>
                <div class="quick-actions">
                    <button class="quick-action" onclick="showScaleModal()">
                        <span class="quick-action-icon">&#128200;</span>
                        Scale Deploy
                    </button>
                    <button class="quick-action" onclick="showRestartModal()">
                        <span class="quick-action-icon">&#128260;</span>
                        Restart Pod
                    </button>
                    <button class="quick-action" onclick="refreshDashboard()">
                        <span class="quick-action-icon">&#128259;</span>
                        Refresh All
                    </button>
                    <button class="quick-action" onclick="showLogsModal()">
                        <span class="quick-action-icon">&#128196;</span>
                        View Logs
                    </button>
                </div>
            </div>
            
            <!-- Node Constellation -->
            <div class="card full-width">
                <div class="card-header">
                    <span class="card-title"><span class="card-icon">&#11088;</span> Node Constellation (13 Stars)</span>
                    <span id="nodeCount">0/13 Ready</span>
                </div>
                <div class="node-grid" id="nodeGrid">
                    <!-- Populated by JS -->
                </div>
            </div>
            
            <!-- Pod Distribution -->
            <div class="card">
                <div class="card-header">
                    <span class="card-title"><span class="card-icon">&#128230;</span> Pod Distribution</span>
                    <span id="totalPods">0 pods</span>
                </div>
                <div class="pod-distribution" id="podDistribution">
                    <!-- Populated by JS -->
                </div>
            </div>
            
            <!-- Resource Usage per Node -->
            <div class="card">
                <div class="card-header">
                    <span class="card-title"><span class="card-icon">&#128202;</span> Top Resource Users</span>
                </div>
                <div id="topResources">
                    <!-- Populated by JS -->
                </div>
            </div>
            
            <!-- Deployments -->
            <div class="card full-width">
                <div class="card-header">
                    <span class="card-title"><span class="card-icon">&#128640;</span> Deployments</span>
                    <span id="deploymentCount">0 deployments</span>
                </div>
                <div class="namespace-filter" id="namespaceFilter">
                    <button class="ns-btn active" data-ns="all">All</button>
                </div>
                <div class="deployment-list" id="deploymentList">
                    <!-- Populated by JS -->
                </div>
            </div>
        </div>
        
        <div class="timestamp" id="timestamp">Last updated: --</div>
    </div>
    
    <!-- Scale Modal -->
    <div class="modal" id="scaleModal">
        <div class="modal-content">
            <div class="modal-header">
                <span class="modal-title">Scale Deployment</span>
                <button class="modal-close" onclick="closeModal('scaleModal')">&times;</button>
            </div>
            <div class="form-group">
                <label class="form-label">Deployment Name</label>
                <input type="text" class="form-input" id="scaleDeployment" placeholder="e.g., web-api">
            </div>
            <div class="form-group">
                <label class="form-label">Namespace</label>
                <input type="text" class="form-input" id="scaleNamespace" value="holm">
            </div>
            <div class="form-group">
                <label class="form-label">Replicas</label>
                <input type="number" class="form-input" id="scaleReplicas" min="0" max="20" value="1">
            </div>
            <button class="form-btn" onclick="scaleDeployment()">Scale Deployment</button>
        </div>
    </div>
    
    <!-- Restart Modal -->
    <div class="modal" id="restartModal">
        <div class="modal-content">
            <div class="modal-header">
                <span class="modal-title">Restart Deployment</span>
                <button class="modal-close" onclick="closeModal('restartModal')">&times;</button>
            </div>
            <div class="form-group">
                <label class="form-label">Deployment Name</label>
                <input type="text" class="form-input" id="restartDeployment" placeholder="e.g., web-api">
            </div>
            <div class="form-group">
                <label class="form-label">Namespace</label>
                <input type="text" class="form-input" id="restartNamespace" value="holm">
            </div>
            <button class="form-btn" onclick="restartDeployment()">Restart Deployment</button>
        </div>
    </div>
    
    <!-- Logs Modal -->
    <div class="modal" id="logsModal">
        <div class="modal-content" style="max-width: 800px;">
            <div class="modal-header">
                <span class="modal-title">Pod Logs</span>
                <button class="modal-close" onclick="closeModal('logsModal')">&times;</button>
            </div>
            <div class="form-group">
                <label class="form-label">Pod Name</label>
                <input type="text" class="form-input" id="logsPod" placeholder="e.g., web-api-xyz123">
            </div>
            <div class="form-group">
                <label class="form-label">Namespace</label>
                <input type="text" class="form-input" id="logsNamespace" value="holm">
            </div>
            <button class="form-btn" onclick="fetchLogs()" style="margin-bottom: 15px;">Fetch Logs</button>
            <pre id="logsContent" style="background: var(--mantle); padding: 15px; border-radius: 8px; max-height: 300px; overflow-y: auto; font-family: 'JetBrains Mono', monospace; font-size: 0.8rem; color: var(--text);"></pre>
        </div>
    </div>
    
    <div class="toast" id="toast"></div>
    
    <button class="refresh-btn" onclick="refreshDashboard()">&#128260;</button>
    
    <script>
        // Create stars background
        function createStars() {
            const container = document.getElementById('starsBg');
            for (let i = 0; i < 100; i++) {
                const star = document.createElement('div');
                star.className = 'star';
                const size = Math.random() * 3 + 1;
                star.style.width = size + 'px';
                star.style.height = size + 'px';
                star.style.left = Math.random() * 100 + '%';
                star.style.top = Math.random() * 100 + '%';
                star.style.animationDelay = Math.random() * 3 + 's';
                container.appendChild(star);
            }
        }
        createStars();
        
        let clusterData = {};
        let currentNamespace = 'all';
        
        function showToast(message, type = 'success') {
            const toast = document.getElementById('toast');
            toast.textContent = message;
            toast.className = 'toast show ' + type;
            setTimeout(() => toast.className = 'toast', 3000);
        }
        
        function showModal(id) {
            document.getElementById(id).classList.add('active');
        }
        
        function closeModal(id) {
            document.getElementById(id).classList.remove('active');
        }
        
        function showScaleModal() { showModal('scaleModal'); }
        function showRestartModal() { showModal('restartModal'); }
        function showLogsModal() { showModal('logsModal'); }
        
        async function fetchDashboardData() {
            try {
                const response = await fetch('/api/dashboard');
                clusterData = await response.json();
                updateDashboard();
            } catch (error) {
                showToast('Failed to fetch cluster data', 'error');
            }
        }
        
        function updateDashboard() {
            // Update status bar
            const statusBar = document.getElementById('statusBar');
            const nodesHealthy = clusterData.nodes?.filter(n => n.status === 'Ready').length || 0;
            const totalNodes = clusterData.nodes?.length || 0;
            const podsRunning = clusterData.pods?.filter(p => p.status === 'Running').length || 0;
            const totalPods = clusterData.pods?.length || 0;
            
            statusBar.innerHTML = `
                <div class="status-pill">
                    <span class="status-dot ${nodesHealthy === totalNodes ? 'green' : 'yellow'}"></span>
                    <span>${nodesHealthy}/${totalNodes} Nodes</span>
                </div>
                <div class="status-pill">
                    <span class="status-dot ${podsRunning === totalPods ? 'green' : 'yellow'}"></span>
                    <span>${podsRunning}/${totalPods} Pods</span>
                </div>
                <div class="status-pill">
                    <span class="status-dot green"></span>
                    <span>${clusterData.deployments?.length || 0} Deployments</span>
                </div>
            `;
            
            // Update cluster health
            document.getElementById('clusterHealth').innerHTML = nodesHealthy === totalNodes ? 
                '<span style="color: var(--green);">&#10003;</span>' : '<span style="color: var(--yellow);">&#9888;</span>';
            
            // Update CPU/Memory bars
            const cpuBar = document.getElementById('cpuBar');
            const memBar = document.getElementById('memBar');
            const cpuAvg = clusterData.metrics?.cpu_avg || 0;
            const memAvg = clusterData.metrics?.mem_avg || 0;
            
            cpuBar.style.width = Math.max(cpuAvg, 5) + '%';
            cpuBar.textContent = cpuAvg + '%';
            memBar.style.width = Math.max(memAvg, 5) + '%';
            memBar.textContent = memAvg + '%';
            
            // Update node grid
            const nodeGrid = document.getElementById('nodeGrid');
            nodeGrid.innerHTML = (clusterData.nodes || []).map(node => `
                <div class="node-card ${node.status === 'Ready' ? 'healthy' : 'unhealthy'} ${node.roles?.includes('control-plane') ? 'control-plane' : ''}">
                    <div class="node-name">${node.name}</div>
                    <div class="node-stats">
                        <span class="node-stat cpu">${node.cpu || '--'}%</span>
                        <span class="node-stat mem">${node.mem || '--'}%</span>
                    </div>
                    <div class="progress-bar">
                        <div class="progress-fill ${getProgressClass(node.cpu || 0)}" style="width: ${node.cpu || 0}%"></div>
                    </div>
                </div>
            `).join('');
            
            document.getElementById('nodeCount').textContent = `${nodesHealthy}/${totalNodes} Ready`;
            
            // Update pod distribution
            const podsByNode = {};
            (clusterData.pods || []).forEach(pod => {
                const node = pod.node || 'Unknown';
                podsByNode[node] = (podsByNode[node] || 0) + 1;
            });
            
            const podDist = document.getElementById('podDistribution');
            podDist.innerHTML = Object.entries(podsByNode).sort((a, b) => b[1] - a[1]).map(([node, count]) => `
                <div class="pod-node">
                    <span>${node}</span>
                    <span class="pod-count">${count}</span>
                </div>
            `).join('');
            
            document.getElementById('totalPods').textContent = `${totalPods} pods`;
            
            // Update top resources
            const topRes = document.getElementById('topResources');
            topRes.innerHTML = (clusterData.top_pods || []).slice(0, 5).map((pod, i) => `
                <div class="deployment-item healthy">
                    <div class="deployment-info">
                        <div class="deployment-name">${pod.name}</div>
                        <div class="deployment-namespace">${pod.namespace}</div>
                    </div>
                    <span class="deployment-replicas" style="color: var(--peach);">${pod.cpu}</span>
                    <span class="deployment-replicas" style="color: var(--blue);">${pod.memory}</span>
                </div>
            `).join('');
            
            // Update namespace filter
            const namespaces = [...new Set((clusterData.deployments || []).map(d => d.namespace))];
            const nsFilter = document.getElementById('namespaceFilter');
            nsFilter.innerHTML = `<button class="ns-btn ${currentNamespace === 'all' ? 'active' : ''}" data-ns="all" onclick="filterNamespace('all')">All</button>` +
                namespaces.map(ns => `<button class="ns-btn ${currentNamespace === ns ? 'active' : ''}" data-ns="${ns}" onclick="filterNamespace('${ns}')">${ns}</button>`).join('');
            
            // Update deployments
            updateDeploymentList();
            
            document.getElementById('deploymentCount').textContent = `${clusterData.deployments?.length || 0} deployments`;
            
            // Update timestamp
            document.getElementById('timestamp').textContent = `Last updated: ${new Date().toLocaleTimeString()}`;
        }
        
        function filterNamespace(ns) {
            currentNamespace = ns;
            document.querySelectorAll('.ns-btn').forEach(btn => {
                btn.classList.toggle('active', btn.dataset.ns === ns);
            });
            updateDeploymentList();
        }
        
        function updateDeploymentList() {
            const deployList = document.getElementById('deploymentList');
            const filtered = (clusterData.deployments || []).filter(d => 
                currentNamespace === 'all' || d.namespace === currentNamespace
            );
            
            deployList.innerHTML = filtered.map(deploy => {
                const ready = deploy.ready || 0;
                const replicas = deploy.replicas || 0;
                const status = ready === replicas ? 'healthy' : ready === 0 ? 'error' : 'warning';
                const replicaClass = ready === replicas ? 'full' : ready === 0 ? 'zero' : 'partial';
                
                return `
                    <div class="deployment-item ${status}">
                        <div class="deployment-info">
                            <div class="deployment-name">${deploy.name}</div>
                            <div class="deployment-namespace">${deploy.namespace}</div>
                        </div>
                        <span class="deployment-replicas ${replicaClass}">${ready}/${replicas}</span>
                        <div class="action-btns">
                            <button class="action-btn scale" onclick="quickScale('${deploy.name}', '${deploy.namespace}')">Scale</button>
                            <button class="action-btn restart" onclick="quickRestart('${deploy.name}', '${deploy.namespace}')">Restart</button>
                        </div>
                    </div>
                `;
            }).join('');
        }
        
        function getProgressClass(value) {
            if (value < 50) return 'low';
            if (value < 70) return 'medium';
            if (value < 90) return 'high';
            return 'critical';
        }
        
        function quickScale(name, namespace) {
            document.getElementById('scaleDeployment').value = name;
            document.getElementById('scaleNamespace').value = namespace;
            showScaleModal();
        }
        
        function quickRestart(name, namespace) {
            document.getElementById('restartDeployment').value = name;
            document.getElementById('restartNamespace').value = namespace;
            showRestartModal();
        }
        
        async function scaleDeployment() {
            const name = document.getElementById('scaleDeployment').value;
            const namespace = document.getElementById('scaleNamespace').value;
            const replicas = document.getElementById('scaleReplicas').value;
            
            try {
                const response = await fetch('/api/scale', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ deployment: name, namespace, replicas: parseInt(replicas) })
                });
                const result = await response.json();
                if (result.success) {
                    showToast(`Scaled ${name} to ${replicas} replicas`);
                    closeModal('scaleModal');
                    setTimeout(refreshDashboard, 2000);
                } else {
                    showToast(result.error || 'Scale failed', 'error');
                }
            } catch (error) {
                showToast('Scale operation failed', 'error');
            }
        }
        
        async function restartDeployment() {
            const name = document.getElementById('restartDeployment').value;
            const namespace = document.getElementById('restartNamespace').value;
            
            try {
                const response = await fetch('/api/restart', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ deployment: name, namespace })
                });
                const result = await response.json();
                if (result.success) {
                    showToast(`Restarting ${name}`);
                    closeModal('restartModal');
                    setTimeout(refreshDashboard, 2000);
                } else {
                    showToast(result.error || 'Restart failed', 'error');
                }
            } catch (error) {
                showToast('Restart operation failed', 'error');
            }
        }
        
        async function fetchLogs() {
            const pod = document.getElementById('logsPod').value;
            const namespace = document.getElementById('logsNamespace').value;
            const logsContent = document.getElementById('logsContent');
            
            logsContent.textContent = 'Loading...';
            
            try {
                const response = await fetch('/api/logs', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ pod, namespace, tail: 100 })
                });
                const result = await response.json();
                logsContent.textContent = result.logs || result.error || 'No logs available';
            } catch (error) {
                logsContent.textContent = 'Failed to fetch logs';
            }
        }
        
        function refreshDashboard() {
            fetchDashboardData();
        }
        
        // Initial load and auto-refresh
        fetchDashboardData();
        setInterval(fetchDashboardData, 30000);
    </script>
</body>
</html>
'''

# Use the kubernetes Python client for better in-cluster support
try:
    from kubernetes import client, config
    from kubernetes.client.rest import ApiException
    
    # Try to load in-cluster config, fall back to kubeconfig
    try:
        config.load_incluster_config()
        print("Loaded in-cluster Kubernetes config")
    except:
        config.load_kube_config()
        print("Loaded kubeconfig")
    
    v1 = client.CoreV1Api()
    apps_v1 = client.AppsV1Api()
    USE_K8S_CLIENT = True
except Exception as e:
    print(f"Kubernetes client not available: {e}")
    USE_K8S_CLIENT = False

def get_nodes_detailed():
    """Get detailed node information with metrics."""
    if not USE_K8S_CLIENT:
        return []
    
    try:
        nodes_list = v1.list_node()
        nodes = []
        
        # Try to get metrics
        metrics = {}
        try:
            custom_api = client.CustomObjectsApi()
            node_metrics = custom_api.list_cluster_custom_object(
                "metrics.k8s.io", "v1beta1", "nodes"
            )
            for item in node_metrics.get("items", []):
                name = item["metadata"]["name"]
                # Parse CPU usage
                cpu_str = item["usage"].get("cpu", "0")
                if cpu_str.endswith("n"):
                    cpu_val = int(cpu_str[:-1]) / 1000000000  # nanocores to cores
                elif cpu_str.endswith("m"):
                    cpu_val = int(cpu_str[:-1]) / 1000  # millicores to cores
                else:
                    cpu_val = float(cpu_str)
                
                # Parse memory usage  
                mem_str = item["usage"].get("memory", "0")
                if mem_str.endswith("Ki"):
                    mem_val = int(mem_str[:-2]) * 1024
                elif mem_str.endswith("Mi"):
                    mem_val = int(mem_str[:-2]) * 1024 * 1024
                elif mem_str.endswith("Gi"):
                    mem_val = int(mem_str[:-2]) * 1024 * 1024 * 1024
                else:
                    mem_val = int(mem_str)
                
                metrics[name] = {"cpu_cores": cpu_val, "mem_bytes": mem_val}
        except Exception as e:
            print(f"Failed to get metrics: {e}")
        
        for node in nodes_list.items:
            name = node.metadata.name
            labels = node.metadata.labels or {}
            
            # Get status
            status = "NotReady"
            for condition in node.status.conditions or []:
                if condition.type == "Ready" and condition.status == "True":
                    status = "Ready"
                    break
            
            # Get roles
            roles = []
            for key in labels:
                if "node-role.kubernetes.io/" in key:
                    roles.append(key.split("/")[1])
            
            # Get allocatable resources
            allocatable = node.status.allocatable or {}
            cpu_alloc = allocatable.get("cpu", "1")
            if cpu_alloc.endswith("m"):
                cpu_alloc_val = int(cpu_alloc[:-1]) / 1000
            else:
                cpu_alloc_val = float(cpu_alloc)
            
            mem_alloc = allocatable.get("memory", "1Gi")
            if mem_alloc.endswith("Ki"):
                mem_alloc_val = int(mem_alloc[:-2]) * 1024
            elif mem_alloc.endswith("Mi"):
                mem_alloc_val = int(mem_alloc[:-2]) * 1024 * 1024
            elif mem_alloc.endswith("Gi"):
                mem_alloc_val = int(mem_alloc[:-2]) * 1024 * 1024 * 1024
            else:
                mem_alloc_val = int(mem_alloc)
            
            node_metrics = metrics.get(name, {})
            cpu_pct = int((node_metrics.get("cpu_cores", 0) / cpu_alloc_val) * 100) if cpu_alloc_val else 0
            mem_pct = int((node_metrics.get("mem_bytes", 0) / mem_alloc_val) * 100) if mem_alloc_val else 0
            
            nodes.append({
                "name": name,
                "status": status,
                "roles": ",".join(roles) if roles else "worker",
                "cpu": min(cpu_pct, 100),
                "mem": min(mem_pct, 100)
            })
        
        return nodes
    except Exception as e:
        print(f"Failed to get nodes: {e}")
        return []

def get_pods_detailed():
    """Get detailed pod information."""
    if not USE_K8S_CLIENT:
        return []
    
    try:
        pods_list = v1.list_pod_for_all_namespaces()
        pods = []
        
        for pod in pods_list.items:
            restarts = 0
            for cs in pod.status.container_statuses or []:
                restarts += cs.restart_count
            
            pods.append({
                "name": pod.metadata.name,
                "namespace": pod.metadata.namespace,
                "status": pod.status.phase,
                "node": pod.spec.node_name or "Unscheduled",
                "restarts": restarts
            })
        
        return pods
    except Exception as e:
        print(f"Failed to get pods: {e}")
        return []

def get_deployments_detailed():
    """Get detailed deployment information."""
    if not USE_K8S_CLIENT:
        return []
    
    try:
        deployments_list = apps_v1.list_deployment_for_all_namespaces()
        deployments = []
        
        for deploy in deployments_list.items:
            deployments.append({
                "name": deploy.metadata.name,
                "namespace": deploy.metadata.namespace,
                "replicas": deploy.spec.replicas or 0,
                "ready": deploy.status.ready_replicas or 0,
                "available": deploy.status.available_replicas or 0
            })
        
        return deployments
    except Exception as e:
        print(f"Failed to get deployments: {e}")
        return []

def get_top_pods():
    """Get top resource consuming pods."""
    if not USE_K8S_CLIENT:
        return []
    
    try:
        custom_api = client.CustomObjectsApi()
        pod_metrics = custom_api.list_cluster_custom_object(
            "metrics.k8s.io", "v1beta1", "pods"
        )
        
        pods = []
        for item in pod_metrics.get("items", []):
            namespace = item["metadata"]["namespace"]
            name = item["metadata"]["name"]
            
            cpu_total = 0
            mem_total = 0
            
            for container in item.get("containers", []):
                cpu_str = container["usage"].get("cpu", "0")
                if cpu_str.endswith("n"):
                    cpu_total += int(cpu_str[:-1]) / 1000000  # nanocores to millicores
                elif cpu_str.endswith("m"):
                    cpu_total += int(cpu_str[:-1])
                
                mem_str = container["usage"].get("memory", "0")
                if mem_str.endswith("Ki"):
                    mem_total += int(mem_str[:-2]) / 1024  # Ki to Mi
                elif mem_str.endswith("Mi"):
                    mem_total += int(mem_str[:-2])
                elif mem_str.endswith("Gi"):
                    mem_total += int(mem_str[:-2]) * 1024
            
            pods.append({
                "namespace": namespace,
                "name": name,
                "cpu": f"{int(cpu_total)}m",
                "memory": f"{int(mem_total)}Mi",
                "cpu_val": cpu_total
            })
        
        pods.sort(key=lambda x: x["cpu_val"], reverse=True)
        return pods[:10]
    except Exception as e:
        print(f"Failed to get top pods: {e}")
        return []

def get_cluster_metrics():
    """Get overall cluster metrics."""
    nodes = get_nodes_detailed()
    if not nodes:
        return {"cpu_avg": 0, "mem_avg": 0}
    
    cpu_values = [n["cpu"] for n in nodes if n["cpu"] > 0]
    mem_values = [n["mem"] for n in nodes if n["mem"] > 0]
    
    return {
        "cpu_avg": sum(cpu_values) // len(cpu_values) if cpu_values else 0,
        "mem_avg": sum(mem_values) // len(mem_values) if mem_values else 0
    }

def scale_deployment_k8s(deployment, namespace, replicas):
    """Scale a deployment using k8s client."""
    if not USE_K8S_CLIENT:
        return {"success": False, "error": "Kubernetes client not available"}
    
    try:
        body = {"spec": {"replicas": replicas}}
        apps_v1.patch_namespaced_deployment_scale(deployment, namespace, body)
        return {"success": True, "message": f"Scaled {deployment} to {replicas}"}
    except ApiException as e:
        return {"success": False, "error": str(e)}

def restart_deployment_k8s(deployment, namespace):
    """Restart a deployment using k8s client."""
    if not USE_K8S_CLIENT:
        return {"success": False, "error": "Kubernetes client not available"}
    
    try:
        now = datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
        body = {
            "spec": {
                "template": {
                    "metadata": {
                        "annotations": {
                            "kubectl.kubernetes.io/restartedAt": now
                        }
                    }
                }
            }
        }
        apps_v1.patch_namespaced_deployment(deployment, namespace, body)
        return {"success": True, "message": f"Restarted {deployment}"}
    except ApiException as e:
        return {"success": False, "error": str(e)}

def get_pod_logs(pod, namespace, tail=100):
    """Get pod logs using k8s client."""
    if not USE_K8S_CLIENT:
        return {"logs": None, "error": "Kubernetes client not available"}
    
    try:
        logs = v1.read_namespaced_pod_log(pod, namespace, tail_lines=tail)
        return {"logs": logs}
    except ApiException as e:
        return {"logs": None, "error": str(e)}

@app.route("/")
def dashboard():
    """Serve the main dashboard."""
    return render_template_string(DASHBOARD_HTML)

@app.route("/api/dashboard")
def api_dashboard():
    """Get all dashboard data."""
    return jsonify({
        "nodes": get_nodes_detailed(),
        "pods": get_pods_detailed(),
        "deployments": get_deployments_detailed(),
        "top_pods": get_top_pods(),
        "metrics": get_cluster_metrics(),
        "timestamp": datetime.now().isoformat()
    })

@app.route("/api/scale", methods=["POST"])
def api_scale():
    """Scale a deployment."""
    data = request.get_json()
    deployment = data.get("deployment")
    namespace = data.get("namespace", "holm")
    replicas = data.get("replicas", 1)
    
    result = scale_deployment_k8s(deployment, namespace, replicas)
    return jsonify(result)

@app.route("/api/restart", methods=["POST"])
def api_restart():
    """Restart a deployment."""
    data = request.get_json()
    deployment = data.get("deployment")
    namespace = data.get("namespace", "holm")
    
    result = restart_deployment_k8s(deployment, namespace)
    return jsonify(result)

@app.route("/api/logs", methods=["POST"])
def api_logs():
    """Get pod logs."""
    data = request.get_json()
    pod = data.get("pod")
    namespace = data.get("namespace", "holm")
    tail = data.get("tail", 100)
    
    result = get_pod_logs(pod, namespace, tail)
    return jsonify(result)

@app.route("/health", methods=["GET"])
def health():
    """Health check endpoint."""
    return jsonify({"status": "healthy", "agent": NOVA_NAME})

@app.route("/capabilities", methods=["GET"])
def capabilities():
    """List Nova's capabilities."""
    return jsonify({
        "agent": NOVA_NAME,
        "catchphrase": NOVA_CATCHPHRASE,
        "version": "2.0.0",
        "features": [
            "Real-time cluster dashboard",
            "13-node constellation view",
            "Pod distribution visualization",
            "Resource usage monitoring",
            "119+ deployment management",
            "Quick actions: scale, restart",
            "Live log viewing"
        ]
    })

# Keep the chat endpoint for API compatibility
@app.route("/chat", methods=["POST"])
def chat():
    """Chat endpoint for interacting with Nova."""
    data = request.get_json()
    if not data or "message" not in data:
        return jsonify({"error": "Missing 'message' in request body"}), 400
    
    message = data["message"].lower()
    
    if "status" in message or "health" in message or "how" in message:
        nodes = get_nodes_detailed()
        healthy = sum(1 for n in nodes if n["status"] == "Ready")
        total = len(nodes)
        metrics = get_cluster_metrics()
        
        response = f"""{NOVA_CATCHPHRASE}

Cluster Status:
- Nodes: {healthy}/{total} healthy
- CPU: {metrics['cpu_avg']}% average
- Memory: {metrics['mem_avg']}% average

Visit the dashboard at / for the full constellation view!"""
    else:
        response = f"""I'm Nova, your cluster guardian. {NOVA_CATCHPHRASE}

Check out the visual dashboard at / for:
- Real-time node constellation (13 stars)
- Pod distribution across nodes
- Resource usage monitoring
- Deployment management with quick actions

Or ask me about: status, nodes, pods, deployments"""
    
    return jsonify({
        "response": response,
        "agent": NOVA_NAME
    })

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 80))
    app.run(host="0.0.0.0", port=port)
