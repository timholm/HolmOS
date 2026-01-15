import os
import json
import subprocess
import requests
import time
import threading
import uuid
import re
from flask import Flask, request, jsonify, render_template_string, Response
from collections import defaultdict

app = Flask(__name__)

REGISTRY_URL = "http://10.110.67.87:5000"
MERCHANT_URL = "http://merchant.holm.svc.cluster.local"
FORGE_URL = "http://forge.holm.svc.cluster.local"
NAMESPACE = "holm"

# In-memory build tracking
build_sessions = {}

@app.route('/health')
def health():
    return jsonify({"status": "healthy", "service": "app-store-ai"})

@app.route('/apps')
def list_apps():
    try:
        resp = requests.get(f"{REGISTRY_URL}/v2/_catalog", timeout=5)
        repos = resp.json().get("repositories", [])
        apps = []
        icons = ['🚀', '🎯', '💡', '🔧', '📊', '🎨', '🔥', '⚡', '🌟', '🎮', '📱', '🎪', '🎭', '🎬', '🎵']
        for i, repo in enumerate(repos):
            try:
                tags_resp = requests.get(f"{REGISTRY_URL}/v2/{repo}/tags/list", timeout=3)
                tags = tags_resp.json().get("tags", [])
            except:
                tags = []
            apps.append({
                "name": repo,
                "icon": icons[i % len(icons)],
                "tags": tags[:5] if tags else ["latest"],
                "description": f"Container app: {repo}"
            })
        return jsonify({"apps": apps, "count": len(apps)})
    except Exception as e:
        return jsonify({"error": str(e), "apps": []}), 500

@app.route('/apps/<name>/deploy', methods=['POST'])
def deploy_app(name):
    try:
        data = request.json or {}
        tag = data.get('tag', 'latest')
        port = data.get('port', 8080)
        deployment_name = re.sub(r'[^a-z0-9-]', '-', name.lower())
        
        deployment = f'''apiVersion: apps/v1
kind: Deployment
metadata:
  name: {deployment_name}
  namespace: {NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {deployment_name}
  template:
    metadata:
      labels:
        app: {deployment_name}
    spec:
      containers:
      - name: {deployment_name}
        image: {REGISTRY_URL.replace("http://", "")}/{name}:{tag}
        ports:
        - containerPort: {port}
---
apiVersion: v1
kind: Service
metadata:
  name: {deployment_name}
  namespace: {NAMESPACE}
spec:
  selector:
    app: {deployment_name}
  ports:
  - port: {port}
    targetPort: {port}
'''
        yaml_path = f"/tmp/{deployment_name}-deploy.yaml"
        with open(yaml_path, 'w') as f:
            f.write(deployment)
        result = subprocess.run(['kubectl', 'apply', '-f', yaml_path], capture_output=True, text=True)
        if result.returncode == 0:
            return jsonify({"status": "deployed", "name": deployment_name, "message": result.stdout})
        else:
            return jsonify({"error": result.stderr}), 500
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/merchant/catalog')
def merchant_catalog():
    """Get available templates from Merchant"""
    try:
        resp = requests.get(f"{MERCHANT_URL}/catalog", timeout=5)
        return jsonify(resp.json())
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/merchant/chat', methods=['POST'])
def merchant_chat():
    """Chat with Merchant AI to interpret user request"""
    try:
        data = request.json
        message = data.get('message', '')
        session_id = data.get('session_id', str(uuid.uuid4()))
        
        # Send to Merchant
        resp = requests.post(f"{MERCHANT_URL}/chat", 
            json={"message": message},
            timeout=30
        )
        merchant_resp = resp.json()
        
        # Store session info
        if session_id not in build_sessions:
            build_sessions[session_id] = {
                "messages": [],
                "builds": [],
                "status": "chatting"
            }
        
        build_sessions[session_id]["messages"].append({
            "role": "user",
            "content": message
        })
        build_sessions[session_id]["messages"].append({
            "role": "merchant",
            "content": merchant_resp.get("response", "")
        })
        
        # If a build was triggered, track it
        if "build_id" in merchant_resp:
            build_sessions[session_id]["builds"].append(merchant_resp["build_id"])
            build_sessions[session_id]["status"] = "building"
        
        return jsonify({
            "response": merchant_resp.get("response", ""),
            "build_id": merchant_resp.get("build_id"),
            "session_id": session_id
        })
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/merchant/build', methods=['POST'])
def merchant_build():
    """Trigger build via Merchant"""
    try:
        data = request.json
        template = data.get('template', '')
        app_name = data.get('name', '')
        
        resp = requests.post(f"{MERCHANT_URL}/build",
            json={"template": template, "name": app_name},
            timeout=30
        )
        return jsonify(resp.json())
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/forge/builds')
def forge_builds():
    """Get all builds from Forge"""
    try:
        resp = requests.get(f"{FORGE_URL}/api/builds", timeout=5)
        return jsonify(resp.json())
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/forge/build/<build_id>')
def forge_build_status(build_id):
    """Get specific build status from Forge"""
    try:
        resp = requests.get(f"{FORGE_URL}/api/builds/{build_id}", timeout=5)
        if resp.status_code == 200:
            return jsonify(resp.json())
        return jsonify({"error": "Build not found"}), 404
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/forge/trigger', methods=['POST'])
def forge_trigger():
    """Trigger a build via Forge with app spec"""
    try:
        data = request.json
        app_name = data.get('name', f"app-{uuid.uuid4().hex[:8]}")
        app_name = re.sub(r'[^a-z0-9-]', '-', app_name.lower())
        
        app_code = data.get('app_code', '')
        dockerfile = data.get('dockerfile', '')
        requirements = data.get('requirements', 'flask>=2.0.0')
        
        # Create configmap with build context
        build_dir = f"/tmp/builds/{app_name}"
        os.makedirs(build_dir, exist_ok=True)
        
        with open(f"{build_dir}/app.py", 'w') as f:
            f.write(app_code)
        with open(f"{build_dir}/Dockerfile", 'w') as f:
            f.write(dockerfile)
        with open(f"{build_dir}/requirements.txt", 'w') as f:
            f.write(requirements)
        
        # Create configmap
        cm_result = subprocess.run([
            'kubectl', 'create', 'configmap', f'build-{app_name}-context',
            f'--from-file={build_dir}', '-n', NAMESPACE,
            '--dry-run=client', '-o', 'yaml'
        ], capture_output=True, text=True)
        
        if cm_result.returncode == 0:
            subprocess.run(['kubectl', 'apply', '-f', '-'], 
                input=cm_result.stdout, capture_output=True, text=True)
        
        # Trigger Forge build
        resp = requests.post(f"{FORGE_URL}/api/trigger",
            json={
                "name": app_name,
                "image": f"10.110.67.87:5000/{app_name}:latest",
                "context_path": f"configmap://build-{app_name}-context",
                "dockerfile": "Dockerfile",
                "namespace": NAMESPACE
            },
            timeout=30
        )
        
        forge_resp = resp.json()
        return jsonify({
            "status": "building",
            "app_name": app_name,
            "forge_response": forge_resp
        })
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/build/stream/<session_id>')
def build_stream(session_id):
    """SSE stream for build progress"""
    def generate():
        last_status = None
        while True:
            try:
                # Get Forge builds
                resp = requests.get(f"{FORGE_URL}/api/builds", timeout=5)
                builds = resp.json()
                
                # Find active builds
                status = {
                    "builds": builds,
                    "timestamp": time.time()
                }
                
                if status != last_status:
                    yield f"data: {json.dumps(status)}\n\n"
                    last_status = status
                
                time.sleep(2)
            except Exception as e:
                yield f"data: {json.dumps({'error': str(e)})}\n\n"
                time.sleep(5)
    
    return Response(generate(), mimetype='text/event-stream')

@app.route('/kaniko/jobs')
def kaniko_jobs():
    """Get all Kaniko build jobs"""
    try:
        result = subprocess.run([
            'kubectl', 'get', 'jobs', '-n', NAMESPACE,
            '-l', 'job-type=kaniko',
            '-o', 'json'
        ], capture_output=True, text=True)
        
        if result.returncode == 0:
            jobs = json.loads(result.stdout)
            return jsonify(jobs)
        
        # Fallback: get all jobs and filter
        result = subprocess.run([
            'kubectl', 'get', 'jobs', '-n', NAMESPACE,
            '-o', 'json'
        ], capture_output=True, text=True)
        
        if result.returncode == 0:
            data = json.loads(result.stdout)
            kaniko_jobs = [j for j in data.get('items', []) 
                         if 'kaniko' in j.get('metadata', {}).get('name', '').lower()]
            return jsonify({"items": kaniko_jobs})
        
        return jsonify({"items": []})
    except Exception as e:
        return jsonify({"error": str(e)}), 500

@app.route('/kaniko/logs/<job_name>')
def kaniko_logs(job_name):
    """Get logs for a Kaniko job"""
    try:
        result = subprocess.run([
            'kubectl', 'logs', f'job/{job_name}', '-n', NAMESPACE
        ], capture_output=True, text=True, timeout=30)
        
        return jsonify({
            "job": job_name,
            "logs": result.stdout,
            "errors": result.stderr
        })
    except Exception as e:
        return jsonify({"error": str(e)}), 500

UI_HTML = '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AI App Store - HolmOS</title>
    <style>
        :root {
            --ctp-rosewater: #f5e0dc; --ctp-flamingo: #f2cdcd; --ctp-pink: #f5c2e7;
            --ctp-mauve: #cba6f7; --ctp-red: #f38ba8; --ctp-maroon: #eba0ac;
            --ctp-peach: #fab387; --ctp-yellow: #f9e2af; --ctp-green: #a6e3a1;
            --ctp-teal: #94e2d5; --ctp-sky: #89dceb; --ctp-sapphire: #74c7ec;
            --ctp-blue: #89b4fa; --ctp-lavender: #b4befe; --ctp-text: #cdd6f4;
            --ctp-subtext1: #bac2de; --ctp-subtext0: #a6adc8; --ctp-overlay2: #9399b2;
            --ctp-overlay1: #7f849c; --ctp-overlay0: #6c7086; --ctp-surface2: #585b70;
            --ctp-surface1: #45475a; --ctp-surface0: #313244; --ctp-base: #1e1e2e;
            --ctp-mantle: #181825; --ctp-crust: #11111b;
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: 'SF Pro Display', system-ui, sans-serif; background: var(--ctp-base); color: var(--ctp-text); min-height: 100vh; }
        
        .header { background: linear-gradient(135deg, var(--ctp-surface0) 0%, var(--ctp-mantle) 100%); padding: 25px 30px; border-bottom: 2px solid var(--ctp-pink); }
        .header-content { max-width: 1400px; margin: 0 auto; display: flex; justify-content: space-between; align-items: center; }
        .logo { display: flex; align-items: center; gap: 15px; }
        .logo-icon { font-size: 2.5em; }
        .logo h1 { font-size: 1.8em; color: var(--ctp-pink); }
        .logo p { color: var(--ctp-subtext0); font-size: 0.9em; }
        .status-indicators { display: flex; gap: 15px; }
        .status-dot { display: flex; align-items: center; gap: 8px; padding: 8px 15px; background: var(--ctp-surface0); border-radius: 20px; font-size: 0.85em; }
        .dot { width: 8px; height: 8px; border-radius: 50%; animation: pulse 2s infinite; }
        .dot.green { background: var(--ctp-green); }
        .dot.yellow { background: var(--ctp-yellow); }
        .dot.red { background: var(--ctp-red); }
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
        
        .main { max-width: 1400px; margin: 0 auto; padding: 30px; display: grid; grid-template-columns: 1fr 400px; gap: 30px; }
        @media (max-width: 1000px) { .main { grid-template-columns: 1fr; } }
        
        .section { background: var(--ctp-surface0); border-radius: 16px; padding: 25px; border: 1px solid var(--ctp-surface1); }
        .section-title { font-size: 1.3em; color: var(--ctp-pink); margin-bottom: 20px; display: flex; align-items: center; gap: 10px; }
        
        .tabs { display: flex; gap: 10px; margin-bottom: 20px; }
        .tab { padding: 10px 20px; background: var(--ctp-surface1); border: none; border-radius: 10px; color: var(--ctp-subtext0); cursor: pointer; transition: all 0.2s; }
        .tab:hover { background: var(--ctp-surface2); }
        .tab.active { background: var(--ctp-pink); color: var(--ctp-crust); }
        
        .app-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 15px; max-height: 400px; overflow-y: auto; }
        .app-card { background: var(--ctp-mantle); border-radius: 12px; padding: 20px; border: 1px solid var(--ctp-surface1); cursor: pointer; transition: all 0.2s; }
        .app-card:hover { transform: translateY(-3px); border-color: var(--ctp-blue); box-shadow: 0 8px 25px rgba(137, 180, 250, 0.15); }
        .app-icon { font-size: 2em; margin-bottom: 10px; }
        .app-name { font-weight: 600; color: var(--ctp-text); margin-bottom: 5px; }
        .app-desc { font-size: 0.8em; color: var(--ctp-subtext0); margin-bottom: 10px; }
        .app-tags { display: flex; flex-wrap: wrap; gap: 5px; }
        .tag { background: var(--ctp-surface1); padding: 3px 8px; border-radius: 10px; font-size: 0.7em; color: var(--ctp-blue); }
        
        .chat-container { display: flex; flex-direction: column; height: 500px; }
        .chat-messages { flex: 1; overflow-y: auto; padding: 15px; background: var(--ctp-mantle); border-radius: 12px; margin-bottom: 15px; }
        .message { margin-bottom: 15px; padding: 12px 15px; border-radius: 12px; max-width: 90%; }
        .message.user { background: var(--ctp-surface1); margin-left: auto; }
        .message.ai { background: linear-gradient(135deg, var(--ctp-surface0), var(--ctp-surface1)); border-left: 3px solid var(--ctp-pink); }
        .message.system { background: var(--ctp-surface0); border-left: 3px solid var(--ctp-yellow); font-size: 0.9em; }
        .message-header { font-size: 0.75em; color: var(--ctp-subtext0); margin-bottom: 5px; display: flex; align-items: center; gap: 5px; }
        .message-content { line-height: 1.5; }
        .message-content pre { background: var(--ctp-crust); padding: 10px; border-radius: 8px; overflow-x: auto; margin-top: 10px; font-size: 0.85em; }
        
        .chat-input { display: flex; gap: 10px; }
        .chat-input textarea { flex: 1; padding: 15px; background: var(--ctp-mantle); border: 2px solid var(--ctp-surface1); border-radius: 12px; color: var(--ctp-text); font-family: inherit; font-size: 0.95em; resize: none; min-height: 60px; }
        .chat-input textarea:focus { outline: none; border-color: var(--ctp-pink); }
        .chat-input textarea::placeholder { color: var(--ctp-overlay0); }
        
        .btn { padding: 12px 20px; border: none; border-radius: 10px; font-size: 0.95em; cursor: pointer; transition: all 0.2s; font-weight: 600; }
        .btn-primary { background: linear-gradient(135deg, var(--ctp-pink), var(--ctp-mauve)); color: var(--ctp-crust); }
        .btn-primary:hover { transform: scale(1.02); box-shadow: 0 5px 20px rgba(245, 194, 231, 0.3); }
        .btn-secondary { background: var(--ctp-surface1); color: var(--ctp-text); }
        .btn-deploy { background: var(--ctp-green); color: var(--ctp-crust); padding: 8px 15px; font-size: 0.85em; }
        
        .build-progress { margin-top: 20px; }
        .build-item { background: var(--ctp-mantle); border-radius: 10px; padding: 15px; margin-bottom: 10px; border-left: 3px solid var(--ctp-blue); }
        .build-item.running { border-left-color: var(--ctp-yellow); animation: buildPulse 1.5s infinite; }
        .build-item.succeeded { border-left-color: var(--ctp-green); }
        .build-item.failed { border-left-color: var(--ctp-red); }
        @keyframes buildPulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.7; } }
        .build-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px; }
        .build-name { font-weight: 600; color: var(--ctp-text); }
        .build-status { padding: 4px 10px; border-radius: 15px; font-size: 0.75em; font-weight: 600; }
        .status-pending { background: rgba(249, 226, 175, 0.2); color: var(--ctp-yellow); }
        .status-running { background: rgba(250, 179, 135, 0.2); color: var(--ctp-peach); }
        .status-succeeded { background: rgba(166, 227, 161, 0.2); color: var(--ctp-green); }
        .status-failed { background: rgba(243, 139, 168, 0.2); color: var(--ctp-red); }
        .build-info { font-size: 0.8em; color: var(--ctp-subtext0); }
        .build-quote { font-style: italic; color: var(--ctp-peach); margin-top: 8px; font-size: 0.85em; }
        
        .template-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; margin-bottom: 15px; }
        .template-card { background: var(--ctp-mantle); padding: 12px; border-radius: 10px; cursor: pointer; border: 2px solid transparent; transition: all 0.2s; }
        .template-card:hover { border-color: var(--ctp-pink); }
        .template-card.selected { border-color: var(--ctp-green); background: rgba(166, 227, 161, 0.1); }
        .template-name { font-weight: 600; font-size: 0.9em; color: var(--ctp-text); }
        .template-desc { font-size: 0.75em; color: var(--ctp-subtext0); }
        
        .modal { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.8); justify-content: center; align-items: center; z-index: 1000; }
        .modal.active { display: flex; }
        .modal-content { background: var(--ctp-surface0); padding: 30px; border-radius: 16px; max-width: 500px; width: 90%; }
        .modal h3 { color: var(--ctp-pink); margin-bottom: 20px; }
        .modal input, .modal select { width: 100%; padding: 12px; margin: 10px 0; border: 1px solid var(--ctp-surface1); border-radius: 10px; background: var(--ctp-mantle); color: var(--ctp-text); }
        .modal-buttons { display: flex; gap: 10px; margin-top: 20px; justify-content: flex-end; }
        
        .loading { text-align: center; padding: 30px; }
        .spinner { width: 40px; height: 40px; border: 3px solid var(--ctp-surface1); border-top-color: var(--ctp-pink); border-radius: 50%; animation: spin 1s linear infinite; margin: 0 auto 15px; }
        @keyframes spin { to { transform: rotate(360deg); } }
        
        .empty-state { text-align: center; padding: 40px; color: var(--ctp-subtext0); }
        .empty-icon { font-size: 3em; margin-bottom: 15px; }
    </style>
</head>
<body>
    <header class="header">
        <div class="header-content">
            <div class="logo">
                <span class="logo-icon">🏪</span>
                <div>
                    <h1>AI App Store</h1>
                    <p>Powered by Merchant AI & Forge Builder</p>
                </div>
            </div>
            <div class="status-indicators">
                <div class="status-dot"><span class="dot green" id="merchantDot"></span> Merchant</div>
                <div class="status-dot"><span class="dot green" id="forgeDot"></span> Forge</div>
                <div class="status-dot"><span class="dot green" id="registryDot"></span> Registry</div>
            </div>
        </div>
    </header>
    
    <main class="main">
        <div class="left-panel">
            <section class="section">
                <h2 class="section-title">📦 App Registry</h2>
                <div class="tabs">
                    <button class="tab active" onclick="showTab('installed')">Installed</button>
                    <button class="tab" onclick="showTab('templates')">Templates</button>
                    <button class="tab" onclick="showTab('builds')">Builds</button>
                </div>
                
                <div id="installedTab" class="tab-content">
                    <div class="app-grid" id="appGrid">
                        <div class="loading"><div class="spinner"></div><p>Loading apps...</p></div>
                    </div>
                </div>
                
                <div id="templatesTab" class="tab-content" style="display:none">
                    <div class="template-grid" id="templateGrid"></div>
                </div>
                
                <div id="buildsTab" class="tab-content" style="display:none">
                    <div id="buildsList" class="build-progress"></div>
                </div>
            </section>
        </div>
        
        <div class="right-panel">
            <section class="section chat-container">
                <h2 class="section-title">🤖 Merchant AI</h2>
                <div class="chat-messages" id="chatMessages">
                    <div class="message ai">
                        <div class="message-header">🏪 Merchant</div>
                        <div class="message-content">
                            Welcome! I'm Merchant, your AI app builder.<br><br>
                            Tell me what app you need:<br>
                            • "I need a todo list"<br>
                            • "Build me a timer app"<br>
                            • "Create a note-taking app"<br><br>
                            Or browse templates in the Templates tab!
                        </div>
                    </div>
                </div>
                <div class="chat-input">
                    <textarea id="chatInput" placeholder="Describe the app you want..." onkeydown="if(event.key==='Enter' && !event.shiftKey){event.preventDefault();sendMessage()}"></textarea>
                    <button class="btn btn-primary" onclick="sendMessage()">🚀</button>
                </div>
            </section>
        </div>
    </main>
    
    <div class="modal" id="deployModal">
        <div class="modal-content">
            <h3>Deploy App</h3>
            <p id="deployAppName"></p>
            <input type="text" id="deployTag" placeholder="Tag (default: latest)">
            <input type="number" id="deployPort" placeholder="Port (default: 8080)" value="8080">
            <div class="modal-buttons">
                <button class="btn btn-secondary" onclick="closeModal()">Cancel</button>
                <button class="btn btn-deploy" onclick="confirmDeploy()">Deploy</button>
            </div>
        </div>
    </div>
    
    <script>
        let currentApp = null;
        let sessionId = 'session-' + Date.now();
        let buildEventSource = null;
        
        // Initialize
        document.addEventListener('DOMContentLoaded', () => {
            loadApps();
            loadTemplates();
            startBuildStream();
            checkServices();
            setInterval(loadBuilds, 5000);
            setInterval(checkServices, 10000);
        });
        
        async function checkServices() {
            try {
                const resp = await fetch('/health');
                document.getElementById('registryDot').className = 'dot green';
            } catch { document.getElementById('registryDot').className = 'dot red'; }
            
            try {
                const resp = await fetch('/merchant/catalog');
                document.getElementById('merchantDot').className = resp.ok ? 'dot green' : 'dot yellow';
            } catch { document.getElementById('merchantDot').className = 'dot red'; }
            
            try {
                const resp = await fetch('/forge/builds');
                document.getElementById('forgeDot').className = resp.ok ? 'dot green' : 'dot yellow';
            } catch { document.getElementById('forgeDot').className = 'dot red'; }
        }
        
        function showTab(tab) {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(t => t.style.display = 'none');
            event.target.classList.add('active');
            document.getElementById(tab + 'Tab').style.display = 'block';
            if (tab === 'builds') loadBuilds();
        }
        
        async function loadApps() {
            try {
                const resp = await fetch('/apps');
                const data = await resp.json();
                renderApps(data.apps || []);
            } catch (e) {
                document.getElementById('appGrid').innerHTML = '<div class="empty-state"><div class="empty-icon">📭</div><p>Failed to load apps</p></div>';
            }
        }
        
        function renderApps(apps) {
            const grid = document.getElementById('appGrid');
            if (apps.length === 0) {
                grid.innerHTML = '<div class="empty-state"><div class="empty-icon">📦</div><p>No apps yet. Build one with Merchant!</p></div>';
                return;
            }
            grid.innerHTML = apps.map(app => `
                <div class="app-card" onclick="showDeploy('${app.name}')">
                    <div class="app-icon">${app.icon}</div>
                    <div class="app-name">${app.name}</div>
                    <div class="app-desc">${app.description}</div>
                    <div class="app-tags">${app.tags.map(t => `<span class="tag">${t}</span>`).join('')}</div>
                </div>
            `).join('');
        }
        
        async function loadTemplates() {
            try {
                const resp = await fetch('/merchant/catalog');
                const data = await resp.json();
                renderTemplates(data.templates || []);
            } catch (e) {
                document.getElementById('templateGrid').innerHTML = '<p>Failed to load templates</p>';
            }
        }
        
        function renderTemplates(templates) {
            const grid = document.getElementById('templateGrid');
            grid.innerHTML = templates.map(t => `
                <div class="template-card" onclick="selectTemplate('${t.name}')">
                    <div class="template-name">${t.name}</div>
                    <div class="template-desc">${t.description}</div>
                </div>
            `).join('');
        }
        
        function selectTemplate(name) {
            document.querySelectorAll('.template-card').forEach(c => c.classList.remove('selected'));
            event.target.closest('.template-card').classList.add('selected');
            document.getElementById('chatInput').value = `Build me a ${name.replace(/-/g, ' ')}`;
            document.getElementById('chatInput').focus();
        }
        
        async function loadBuilds() {
            try {
                const resp = await fetch('/forge/builds');
                const builds = await resp.json();
                renderBuilds(builds);
            } catch (e) {
                console.error('Failed to load builds:', e);
            }
        }
        
        function renderBuilds(builds) {
            const list = document.getElementById('buildsList');
            if (!builds || builds.length === 0) {
                list.innerHTML = '<div class="empty-state"><div class="empty-icon">🔨</div><p>No builds yet</p></div>';
                return;
            }
            list.innerHTML = builds.slice(0, 10).map(b => `
                <div class="build-item ${b.status}">
                    <div class="build-header">
                        <span class="build-name">${b.name}</span>
                        <span class="build-status status-${b.status}">${b.status}</span>
                    </div>
                    <div class="build-info">
                        ${b.image}<br>
                        ${b.duration || 'In progress...'}
                    </div>
                    ${b.forge_quote ? `<div class="build-quote">"${b.forge_quote}"</div>` : ''}
                </div>
            `).join('');
        }
        
        function startBuildStream() {
            buildEventSource = new EventSource('/build/stream/' + sessionId);
            buildEventSource.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    if (data.builds) renderBuilds(data.builds);
                } catch (e) {}
            };
        }
        
        async function sendMessage() {
            const input = document.getElementById('chatInput');
            const message = input.value.trim();
            if (!message) return;
            
            const messages = document.getElementById('chatMessages');
            messages.innerHTML += `<div class="message user"><div class="message-header">You</div><div class="message-content">${escapeHtml(message)}</div></div>`;
            input.value = '';
            
            messages.innerHTML += `<div class="message system" id="thinking"><div class="message-header">⏳ Processing</div><div class="message-content">Merchant is thinking...</div></div>`;
            messages.scrollTop = messages.scrollHeight;
            
            try {
                const resp = await fetch('/merchant/chat', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ message, session_id: sessionId })
                });
                const data = await resp.json();
                
                document.getElementById('thinking')?.remove();
                
                let content = escapeHtml(data.response || 'No response');
                if (data.build_id) {
                    content += `<br><br><span class="tag" style="background: var(--ctp-green); color: var(--ctp-crust);">Build Started: ${data.build_id}</span>`;
                    showTab('builds');
                    setTimeout(loadBuilds, 1000);
                    setTimeout(loadApps, 5000);
                }
                
                messages.innerHTML += `<div class="message ai"><div class="message-header">🏪 Merchant</div><div class="message-content">${content.replace(/\\n/g, '<br>')}</div></div>`;
            } catch (e) {
                document.getElementById('thinking')?.remove();
                messages.innerHTML += `<div class="message system"><div class="message-header">❌ Error</div><div class="message-content">${e.message}</div></div>`;
            }
            messages.scrollTop = messages.scrollHeight;
        }
        
        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
        
        function showDeploy(name) {
            currentApp = name;
            document.getElementById('deployAppName').textContent = 'Deploy: ' + name;
            document.getElementById('deployModal').classList.add('active');
        }
        
        function closeModal() {
            document.getElementById('deployModal').classList.remove('active');
        }
        
        async function confirmDeploy() {
            const tag = document.getElementById('deployTag').value || 'latest';
            const port = parseInt(document.getElementById('deployPort').value) || 8080;
            
            try {
                const resp = await fetch('/apps/' + currentApp + '/deploy', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ tag, port })
                });
                const data = await resp.json();
                
                const messages = document.getElementById('chatMessages');
                if (data.status === 'deployed') {
                    messages.innerHTML += `<div class="message system"><div class="message-header">✅ Deployed</div><div class="message-content">${currentApp} deployed successfully!</div></div>`;
                } else {
                    messages.innerHTML += `<div class="message system"><div class="message-header">❌ Error</div><div class="message-content">${data.error}</div></div>`;
                }
                messages.scrollTop = messages.scrollHeight;
            } catch (e) {
                alert('Deploy failed: ' + e.message);
            }
            closeModal();
        }
    </script>
</body>
</html>'''

@app.route('/')
def index():
    return render_template_string(UI_HTML)

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8080)
