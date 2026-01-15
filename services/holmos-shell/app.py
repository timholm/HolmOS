#!/usr/bin/env python3
"""HolmOS Shell - iPhone-style UI for Kubernetes cluster management."""

import os
from flask import Flask, jsonify, request, Response
from flask_cors import CORS

app = Flask(__name__)
CORS(app)

# Get the host from environment or use default
CLUSTER_HOST = os.environ.get('CLUSTER_HOST', '192.168.8.197')

# Embedded HTML template - iPhone-style home screen
INDEX_HTML = '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, user-scalable=no, viewport-fit=cover">
    <title>HolmOS</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600&display=swap" rel="stylesheet">
    <style>
        :root {
            --base: #1e1e2e;
            --mantle: #181825;
            --crust: #11111b;
            --text: #cdd6f4;
            --subtext0: #a6adc8;
            --subtext1: #bac2de;
            --surface0: #313244;
            --surface1: #45475a;
            --surface2: #585b70;
            --overlay0: #6c7086;
            --blue: #89b4fa;
            --lavender: #b4befe;
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
        * { margin: 0; padding: 0; box-sizing: border-box; -webkit-tap-highlight-color: transparent; }
        html, body { height: 100%; overflow: hidden; }
        body {
            font-family: "Inter", -apple-system, BlinkMacSystemFont, sans-serif;
            background: var(--base);
            color: var(--text);
            min-height: 100vh;
            min-height: -webkit-fill-available;
            display: flex;
            flex-direction: column;
        }
        body::before {
            content: "";
            position: fixed; top: 0; left: 0; right: 0; bottom: 0;
            background:
                radial-gradient(ellipse at 20% 20%, rgba(137, 180, 250, 0.12) 0%, transparent 50%),
                radial-gradient(ellipse at 80% 80%, rgba(203, 166, 247, 0.12) 0%, transparent 50%),
                radial-gradient(ellipse at 50% 50%, rgba(148, 226, 213, 0.06) 0%, transparent 70%);
            pointer-events: none;
            z-index: -1;
        }
        .status-bar {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 12px 24px;
            font-size: 14px;
            font-weight: 600;
            background: transparent;
            position: fixed;
            top: 0; left: 0; right: 0;
            z-index: 100;
        }
        .status-left { display: flex; align-items: center; gap: 4px; }
        .status-center { position: absolute; left: 50%; transform: translateX(-50%); }
        .status-right { display: flex; align-items: center; gap: 6px; }
        .status-icon { width: 18px; height: 18px; fill: var(--text); }
        .battery { display: flex; align-items: center; gap: 2px; }
        .battery-body { width: 24px; height: 12px; border: 2px solid var(--text); border-radius: 3px; position: relative; }
        .battery-level { position: absolute; top: 1px; left: 1px; bottom: 1px; width: 80%; background: var(--green); border-radius: 1px; }
        .battery-cap { width: 2px; height: 6px; background: var(--text); border-radius: 0 2px 2px 0; }
        
        .container { 
            flex: 1; 
            display: flex; 
            flex-direction: column; 
            padding-top: 50px; 
            padding-bottom: 110px; 
            overflow-y: auto; 
            overflow-x: hidden;
        }
        
        .app-grid { 
            display: grid; 
            grid-template-columns: repeat(4, 1fr); 
            gap: 24px 16px; 
            padding: 20px 24px; 
            max-width: 400px; 
            margin: 0 auto; 
            width: 100%; 
        }
        
        .app-icon { 
            display: flex; 
            flex-direction: column; 
            align-items: center; 
            gap: 6px; 
            cursor: pointer; 
            transition: transform 0.15s ease; 
            text-decoration: none;
        }
        .app-icon:active { transform: scale(0.88); }
        
        .app-icon-bg {
            width: 60px; height: 60px; 
            border-radius: 14px;
            display: flex; 
            align-items: center; 
            justify-content: center;
            font-size: 28px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.25);
            position: relative; 
            overflow: hidden;
        }
        .app-icon-bg::before {
            content: "";
            position: absolute; top: 0; left: 0; right: 0; height: 50%;
            background: linear-gradient(180deg, rgba(255,255,255,0.25) 0%, rgba(255,255,255,0) 100%);
            border-radius: 14px 14px 0 0;
        }
        
        .app-name { 
            font-size: 11px; 
            font-weight: 500; 
            text-align: center; 
            color: var(--text); 
            text-shadow: 0 1px 3px rgba(0, 0, 0, 0.6); 
            max-width: 70px; 
            overflow: hidden; 
            text-overflow: ellipsis; 
            white-space: nowrap; 
        }
        
        /* iPhone App Colors */
        .app-calculator .app-icon-bg { background: linear-gradient(135deg, #333333, #1a1a1a); }
        .app-notes .app-icon-bg { background: linear-gradient(135deg, #ffd60a, #ffb800); }
        .app-video .app-icon-bg { background: linear-gradient(135deg, #ff375f, #ff0033); }
        .app-clock .app-icon-bg { background: linear-gradient(135deg, #1c1c1e, #000000); }
        .app-maps .app-icon-bg { background: linear-gradient(135deg, #34c759, #30d158); }
        .app-photos .app-icon-bg { background: linear-gradient(135deg, #ff9f0a, #ff6b00); }
        .app-browser .app-icon-bg { background: linear-gradient(135deg, #007aff, #0055cc); }
        .app-music .app-icon-bg { background: linear-gradient(135deg, #fc3c44, #ff2d55); }
        .app-mail .app-icon-bg { background: linear-gradient(135deg, #5ac8fa, #007aff); }
        .app-reminders .app-icon-bg { background: linear-gradient(135deg, #ff9500, #ff6600); }
        .app-contacts .app-icon-bg { background: linear-gradient(135deg, #8e8e93, #636366); }
        
        /* System App Colors */
        .app-chat .app-icon-bg { background: linear-gradient(135deg, var(--green), var(--teal)); }
        .app-store .app-icon-bg { background: linear-gradient(135deg, var(--sapphire), var(--blue)); }
        .app-settings .app-icon-bg { background: linear-gradient(135deg, var(--surface1), var(--overlay0)); }
        .app-files .app-icon-bg { background: linear-gradient(135deg, var(--blue), var(--sapphire)); }
        .app-git .app-icon-bg { background: linear-gradient(135deg, #f05033, #d44000); }
        .app-cluster .app-icon-bg { background: linear-gradient(135deg, var(--mauve), var(--pink)); }
        .app-metrics .app-icon-bg { background: linear-gradient(135deg, var(--red), var(--maroon)); }
        .app-registry .app-icon-bg { background: linear-gradient(135deg, var(--sapphire), var(--blue)); }
        
        /* Dock */
        .dock {
            position: fixed; 
            bottom: 16px; 
            left: 50%; 
            transform: translateX(-50%);
            background: rgba(30, 30, 46, 0.75);
            backdrop-filter: blur(24px); 
            -webkit-backdrop-filter: blur(24px);
            border-radius: 28px; 
            padding: 12px 16px;
            display: flex; 
            gap: 16px;
            border: 1px solid rgba(205, 214, 244, 0.08);
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
        }
        .dock .app-icon { gap: 0; }
        .dock .app-icon-bg { width: 52px; height: 52px; border-radius: 12px; font-size: 24px; }
        .dock .app-name { display: none; }
        
        /* Page Indicator */
        .page-dots { 
            display: flex; 
            justify-content: center; 
            gap: 8px; 
            padding: 16px; 
        }
        .dot { 
            width: 8px; 
            height: 8px; 
            border-radius: 50%; 
            background: var(--surface2); 
            transition: background 0.2s;
        }
        .dot.active { background: var(--text); }
        
        /* Section Headers */
        .section-header {
            font-size: 13px;
            font-weight: 600;
            color: var(--subtext0);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            padding: 16px 24px 8px;
            max-width: 400px;
            margin: 0 auto;
            width: 100%;
        }
        
        /* App Modal */
        .app-modal { 
            position: fixed; 
            top: 0; left: 0; right: 0; bottom: 0; 
            background: var(--base); 
            z-index: 200; 
            display: none; 
            flex-direction: column; 
        }
        .app-modal.active { display: flex; }
        .app-modal-header { 
            display: flex; 
            align-items: center; 
            justify-content: space-between; 
            padding: 12px 16px; 
            background: var(--mantle); 
            border-bottom: 1px solid var(--surface0); 
        }
        .app-modal-title { font-size: 16px; font-weight: 600; }
        .app-modal-close { 
            width: 32px; height: 32px; 
            border-radius: 50%; 
            background: var(--surface0); 
            border: none; 
            color: var(--text); 
            font-size: 18px; 
            cursor: pointer; 
            display: flex; 
            align-items: center; 
            justify-content: center; 
        }
        .app-modal-content { flex: 1; overflow: hidden; }
        .app-modal-content iframe { width: 100%; height: 100%; border: none; }
        .app-loading { 
            display: flex; 
            align-items: center; 
            justify-content: center; 
            height: 100%; 
            flex-direction: column; 
            gap: 16px; 
        }
        .spinner { 
            width: 40px; height: 40px; 
            border: 3px solid var(--surface0); 
            border-top-color: var(--mauve); 
            border-radius: 50%; 
            animation: spin 1s linear infinite; 
        }
        @keyframes spin { to { transform: rotate(360deg); } }
        
        /* Responsive */
        @media (max-width: 380px) {
            .app-grid { gap: 20px 12px; padding: 16px 20px; }
            .app-icon-bg { width: 54px; height: 54px; font-size: 24px; }
            .dock { padding: 10px 14px; gap: 12px; }
            .dock .app-icon-bg { width: 48px; height: 48px; }
        }
        @media (min-width: 600px) { 
            .app-grid { grid-template-columns: repeat(5, 1fr); max-width: 500px; } 
        }
        @media (min-width: 768px) { 
            .app-grid { grid-template-columns: repeat(6, 1fr); max-width: 600px; } 
        }
    </style>
</head>
<body>
    <div class="status-bar">
        <div class="status-left"><span id="time">9:41</span></div>
        <div class="status-center"><span style="font-size: 12px; color: var(--subtext0);">HolmOS</span></div>
        <div class="status-right">
            <svg class="status-icon" viewBox="0 0 24 24">
                <path d="M1 9l2 2c4.97-4.97 13.03-4.97 18 0l2-2C16.93 2.93 7.08 2.93 1 9zm8 8l3 3 3-3c-1.65-1.66-4.34-1.66-6 0zm-4-4l2 2c2.76-2.76 7.24-2.76 10 0l2-2C15.14 9.14 8.87 9.14 5 13z"/>
            </svg>
            <div class="battery">
                <div class="battery-body"><div class="battery-level"></div></div>
                <div class="battery-cap"></div>
            </div>
        </div>
    </div>
    
    <div class="container">
        <div class="app-grid" id="appGrid"></div>
        <div class="section-header">Developer Tools</div>
        <div class="app-grid" id="devGrid"></div>
        <div class="page-dots"><div class="dot active"></div><div class="dot"></div></div>
    </div>
    
    <div class="dock" id="dock"></div>
    
    <div class="app-modal" id="appModal">
        <div class="app-modal-header">
            <span class="app-modal-title" id="modalTitle">App</span>
            <button class="app-modal-close" onclick="closeApp()">&times;</button>
        </div>
        <div class="app-modal-content" id="modalContent">
            <div class="app-loading"><div class="spinner"></div><span>Loading...</span></div>
        </div>
    </div>
    
    <script>
        const HOST = "''' + CLUSTER_HOST + '''";
        
        // iPhone-style apps (main grid)
        const mainApps = [
            { id: "calculator", name: "Calculator", icon: "&#128425;", class: "app-calculator", port: 30007 },
            { id: "notes", name: "Notes", icon: "&#128221;", class: "app-notes", port: 30010 },
            { id: "video", name: "Video", icon: "&#127916;", class: "app-video", port: null },
            { id: "clock", name: "Clock", icon: "&#9200;", class: "app-clock", port: 30007 },
            { id: "maps", name: "Maps", icon: "&#128506;", class: "app-maps", port: null },
            { id: "photos", name: "Photos", icon: "&#127749;", class: "app-photos", port: null },
            { id: "browser", name: "Browser", icon: "&#127760;", class: "app-browser", port: null },
            { id: "music", name: "Music", icon: "&#127925;", class: "app-music", port: null },
            { id: "mail", name: "Mail", icon: "&#9993;", class: "app-mail", port: null },
            { id: "reminders", name: "Reminders", icon: "&#128204;", class: "app-reminders", port: null },
            { id: "contacts", name: "Contacts", icon: "&#128100;", class: "app-contacts", port: null },
            { id: "cluster", name: "Nova", icon: "&#127760;", class: "app-cluster", port: 30004 },
            { id: "metrics", name: "Pulse", icon: "&#128202;", class: "app-metrics", port: 30006 },
            { id: "registry", name: "Harbor", icon: "&#128051;", class: "app-registry", port: 31750 }
        ];
        
        // Developer tools section
        const devApps = [
            { id: "git", name: "HolmGit", icon: "&#128256;", class: "app-git", port: 30009 }
        ];
        
        // Dock apps: Chat Hub, App Store, Settings, Files
        const dockApps = [
            { id: "chat", name: "Chat Hub", icon: "&#128172;", class: "app-chat", port: 30003 },
            { id: "store", name: "App Store", icon: "&#128242;", class: "app-store", port: 30002 },
            { id: "settings", name: "Settings", icon: "&#9881;", class: "app-settings", port: 30600 },
            { id: "files", name: "Files", icon: "&#128193;", class: "app-files", port: 30088 }
        ];
        
        function createAppIcon(app) {
            return "<div class=\"app-icon " + app.class + "\" onclick=\"launchApp(" + (app.port || "null") + ", '" + app.name + "')\"" +
                "><div class=\"app-icon-bg\">" + app.icon + "</div>" +
                "<span class=\"app-name\">" + app.name + "</span></div>";
        }
        
        function renderApps() {
            document.getElementById("appGrid").innerHTML = mainApps.map(createAppIcon).join("");
            document.getElementById("devGrid").innerHTML = devApps.map(createAppIcon).join("");
            document.getElementById("dock").innerHTML = dockApps.map(createAppIcon).join("");
        }
        
        function launchApp(port, appName) {
            if (!port) {
                showMessage(appName, "Coming soon!");
                return;
            }
            
            const modal = document.getElementById("appModal");
            const modalTitle = document.getElementById("modalTitle");
            const modalContent = document.getElementById("modalContent");
            
            modalTitle.textContent = appName;
            modalContent.innerHTML = "<div class=\"app-loading\"><div class=\"spinner\"></div><span>Loading " + appName + "...</span></div>";
            modal.classList.add("active");
            
            const url = "http://" + HOST + ":" + port;
            modalContent.innerHTML = "<iframe src=\"" + url + "\"></iframe>";
        }
        
        function showMessage(appName, message) {
            const modal = document.getElementById("appModal");
            const modalTitle = document.getElementById("modalTitle");
            const modalContent = document.getElementById("modalContent");
            
            modalTitle.textContent = appName;
            modalContent.innerHTML = "<div class=\"app-loading\"><span style=\"font-size: 48px;\">&#128736;</span><span>" + appName + "</span><span style=\"color: var(--subtext0); font-size: 14px;\">" + message + "</span></div>";
            modal.classList.add("active");
        }
        
        function closeApp() {
            document.getElementById("appModal").classList.remove("active");
            document.getElementById("modalContent").innerHTML = "";
        }
        
        function updateTime() {
            const now = new Date();
            const h = now.getHours();
            const m = now.getMinutes().toString().padStart(2, "0");
            document.getElementById("time").textContent = h + ":" + m;
        }
        
        renderApps();
        updateTime();
        setInterval(updateTime, 1000);
        
        document.addEventListener("keydown", e => { if (e.key === "Escape") closeApp(); });
    </script>
</body>
</html>'''


@app.route("/")
def index():
    """Serve the main iPhone-style UI."""
    return Response(INDEX_HTML, mimetype="text/html")


@app.route("/api/apps")
def list_apps():
    """List all available apps."""
    return jsonify({
        "status": "ok",
        "host": CLUSTER_HOST,
        "apps": [
            "calculator", "notes", "video", "clock", "maps", "photos",
            "browser", "music", "mail", "reminders", "contacts",
            "chat", "store", "settings", "files", "git"
        ]
    })


@app.route("/api/status")
def status():
    """Get system status."""
    return jsonify({
        "status": "running",
        "version": "2.0.0",
        "host": CLUSTER_HOST
    })


@app.route("/health")
def health():
    """Health check endpoint."""
    return jsonify({"status": "healthy"})


@app.route("/ready")
def ready():
    """Readiness check endpoint."""
    return jsonify({"status": "ready"})


if __name__ == "__main__":
    port = int(os.environ.get("PORT", 8080))
    debug = os.environ.get("DEBUG", "false").lower() == "true"
    app.run(host="0.0.0.0", port=port, debug=debug)
