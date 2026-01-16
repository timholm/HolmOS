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
            --accent: #cba6f7;
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        html, body { height: 100%; overflow: hidden; }
        body { font-family: 'SF Pro Display', system-ui, sans-serif; background: var(--ctp-base); color: var(--ctp-text); display: flex; flex-direction: column; }

        .header { background: linear-gradient(135deg, var(--ctp-surface0) 0%, var(--ctp-mantle) 100%); padding: 15px 20px; border-bottom: 1px solid var(--ctp-surface1); flex-shrink: 0; }
        .header-content { max-width: 1200px; margin: 0 auto; display: flex; justify-content: space-between; align-items: center; }
        .logo { display: flex; align-items: center; gap: 12px; }
        .logo-icon { font-size: 1.8em; }
        .logo h1 { font-size: 1.3em; color: var(--accent); }
        .logo p { color: var(--ctp-subtext0); font-size: 0.75em; }
        .status-indicators { display: flex; gap: 10px; }
        .status-dot { display: flex; align-items: center; gap: 6px; padding: 5px 10px; background: var(--ctp-surface0); border-radius: 15px; font-size: 0.75em; }
        .dot { width: 6px; height: 6px; border-radius: 50%; }
        .dot.green { background: var(--ctp-green); }
        .dot.yellow { background: var(--ctp-yellow); }
        .dot.red { background: var(--ctp-red); }

        .content-area { flex: 1; overflow-y: auto; overflow-x: hidden; padding: 20px; padding-bottom: 80px; }

        .tab-view { display: none; max-width: 1200px; margin: 0 auto; }
        .tab-view.active { display: block; }

        .view-title { font-size: 1.5em; color: var(--accent); margin-bottom: 20px; display: flex; align-items: center; gap: 10px; }

        /* Featured Tab */
        .featured-banner { background: linear-gradient(135deg, var(--ctp-surface0), var(--ctp-mantle)); border-radius: 16px; padding: 30px; margin-bottom: 25px; border: 1px solid var(--ctp-surface1); }
        .featured-banner h2 { color: var(--accent); margin-bottom: 10px; }
        .featured-banner p { color: var(--ctp-subtext0); margin-bottom: 20px; }
        .featured-apps { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 15px; }

        /* App Cards */
        .app-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 15px; }
        .app-card { background: var(--ctp-surface0); border-radius: 12px; padding: 20px; border: 1px solid var(--ctp-surface1); cursor: pointer; transition: all 0.2s; }
        .app-card:hover { transform: translateY(-3px); border-color: var(--accent); box-shadow: 0 8px 25px rgba(203, 166, 247, 0.15); }
        .app-card.featured { background: linear-gradient(135deg, var(--ctp-surface0), var(--ctp-surface1)); border-color: var(--accent); }
        .app-icon { font-size: 2.5em; margin-bottom: 12px; }
        .app-name { font-weight: 600; color: var(--ctp-text); margin-bottom: 5px; font-size: 1.1em; }
        .app-desc { font-size: 0.85em; color: var(--ctp-subtext0); margin-bottom: 12px; line-height: 1.4; }
        .app-tags { display: flex; flex-wrap: wrap; gap: 5px; }
        .tag { background: var(--ctp-surface1); padding: 4px 10px; border-radius: 12px; font-size: 0.7em; color: var(--ctp-blue); }
        .tag.new { background: rgba(166, 227, 161, 0.2); color: var(--ctp-green); }

        /* Installed Tab */
        .installed-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
        .installed-count { background: var(--ctp-surface0); padding: 8px 16px; border-radius: 20px; font-size: 0.85em; color: var(--ctp-subtext0); }

        /* Updates Tab */
        .update-card { background: var(--ctp-surface0); border-radius: 12px; padding: 20px; margin-bottom: 15px; border: 1px solid var(--ctp-surface1); display: flex; align-items: center; gap: 15px; }
        .update-icon { font-size: 2em; }
        .update-info { flex: 1; }
        .update-name { font-weight: 600; color: var(--ctp-text); }
        .update-version { font-size: 0.8em; color: var(--ctp-subtext0); margin-top: 3px; }
        .update-btn { background: var(--accent); color: var(--ctp-crust); border: none; padding: 8px 16px; border-radius: 8px; font-weight: 600; cursor: pointer; transition: all 0.2s; }
        .update-btn:hover { transform: scale(1.05); }
        .update-all-btn { background: var(--ctp-green); color: var(--ctp-crust); border: none; padding: 10px 20px; border-radius: 10px; font-weight: 600; cursor: pointer; margin-bottom: 20px; }

        /* Bottom Tab Bar */
        .bottom-tabs { position: fixed; bottom: 0; left: 0; right: 0; background: var(--ctp-mantle); border-top: 1px solid var(--ctp-surface1); padding: 8px 0; padding-bottom: max(8px, env(safe-area-inset-bottom)); z-index: 100; flex-shrink: 0; }
        .bottom-tabs-inner { display: flex; justify-content: space-around; max-width: 500px; margin: 0 auto; }
        .bottom-tab { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 8px 16px; border: none; background: none; color: var(--ctp-overlay0); font-size: 11px; cursor: pointer; transition: all 0.2s; min-width: 70px; }
        .bottom-tab:hover { color: var(--ctp-subtext0); }
        .bottom-tab.active { color: var(--accent); }
        .bottom-tab.active .tab-icon { transform: scale(1.1); }
        .tab-icon { font-size: 24px; transition: transform 0.2s; }
        .tab-label { font-weight: 500; }

        /* Loading & Empty States */
        .loading { text-align: center; padding: 40px; }
        .spinner { width: 40px; height: 40px; border: 3px solid var(--ctp-surface1); border-top-color: var(--accent); border-radius: 50%; animation: spin 1s linear infinite; margin: 0 auto 15px; }
        @keyframes spin { to { transform: rotate(360deg); } }
        .empty-state { text-align: center; padding: 60px 20px; color: var(--ctp-subtext0); }
        .empty-icon { font-size: 4em; margin-bottom: 20px; opacity: 0.5; }
        .empty-text { font-size: 1.1em; margin-bottom: 10px; }
        .empty-subtext { font-size: 0.9em; color: var(--ctp-overlay0); }

        /* Modal */
        .modal { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.8); justify-content: center; align-items: center; z-index: 1000; }
        .modal.active { display: flex; }
        .modal-content { background: var(--ctp-surface0); padding: 30px; border-radius: 16px; max-width: 400px; width: 90%; }
        .modal h3 { color: var(--accent); margin-bottom: 20px; }
        .modal input { width: 100%; padding: 12px; margin: 10px 0; border: 1px solid var(--ctp-surface1); border-radius: 10px; background: var(--ctp-mantle); color: var(--ctp-text); }
        .modal-buttons { display: flex; gap: 10px; margin-top: 20px; justify-content: flex-end; }
        .btn { padding: 10px 20px; border: none; border-radius: 8px; font-weight: 600; cursor: pointer; transition: all 0.2s; }
        .btn-primary { background: var(--accent); color: var(--ctp-crust); }
        .btn-secondary { background: var(--ctp-surface1); color: var(--ctp-text); }
        .btn-deploy { background: var(--ctp-green); color: var(--ctp-crust); }

        @media (max-width: 600px) {
            .header { padding: 12px 15px; }
            .header-content { flex-direction: column; gap: 10px; }
            .logo h1 { font-size: 1.1em; }
            .status-indicators { display: none; }
            .content-area { padding: 15px; }
            .app-grid { grid-template-columns: repeat(2, 1fr); gap: 10px; }
            .app-card { padding: 15px; }
            .app-icon { font-size: 2em; }
            .featured-apps { grid-template-columns: 1fr; }
        }
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

    <div class="content-area">
        <!-- Featured Tab -->
        <div id="featuredView" class="tab-view active">
            <h1 class="view-title">⭐ Featured</h1>
            <div class="featured-banner">
                <h2>Welcome to AI App Store</h2>
                <p>Discover and deploy apps built with Merchant AI. Browse featured apps below or check the Apps tab for the full catalog.</p>
            </div>
            <div class="featured-apps" id="featuredGrid">
                <div class="loading"><div class="spinner"></div><p>Loading featured apps...</p></div>
            </div>
        </div>

        <!-- Apps Tab -->
        <div id="appsView" class="tab-view">
            <h1 class="view-title">⊞ All Apps</h1>
            <div class="app-grid" id="appGrid">
                <div class="loading"><div class="spinner"></div><p>Loading apps...</p></div>
            </div>
        </div>

        <!-- Installed Tab -->
        <div id="installedView" class="tab-view">
            <h1 class="view-title">✓ Installed</h1>
            <div class="installed-header">
                <span>Your deployed applications</span>
                <span class="installed-count" id="installedCount">0 apps</span>
            </div>
            <div class="app-grid" id="installedGrid">
                <div class="loading"><div class="spinner"></div><p>Loading installed apps...</p></div>
            </div>
        </div>

        <!-- Updates Tab -->
        <div id="updatesView" class="tab-view">
            <h1 class="view-title">↻ Updates</h1>
            <div id="updatesContainer">
                <div class="loading"><div class="spinner"></div><p>Checking for updates...</p></div>
            </div>
        </div>
    </div>

    <nav class="bottom-tabs">
        <div class="bottom-tabs-inner">
            <button class="bottom-tab active" onclick="switchTab('featured')" data-tab="featured">
                <span class="tab-icon">⭐</span>
                <span class="tab-label">Featured</span>
            </button>
            <button class="bottom-tab" onclick="switchTab('apps')" data-tab="apps">
                <span class="tab-icon">⊞</span>
                <span class="tab-label">Apps</span>
            </button>
            <button class="bottom-tab" onclick="switchTab('installed')" data-tab="installed">
                <span class="tab-icon">✓</span>
                <span class="tab-label">Installed</span>
            </button>
            <button class="bottom-tab" onclick="switchTab('updates')" data-tab="updates">
                <span class="tab-icon">↻</span>
                <span class="tab-label">Updates</span>
            </button>
        </div>
    </nav>

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
        let allApps = [];
        let installedApps = [];

        document.addEventListener('DOMContentLoaded', () => {
            loadApps();
            checkServices();
            setInterval(checkServices, 15000);
        });

        function switchTab(tab) {
            document.querySelectorAll('.bottom-tab').forEach(t => t.classList.remove('active'));
            document.querySelector(`[data-tab="${tab}"]`).classList.add('active');

            document.querySelectorAll('.tab-view').forEach(v => v.classList.remove('active'));
            document.getElementById(tab + 'View').classList.add('active');

            if (tab === 'updates') loadUpdates();
            if (tab === 'installed') loadInstalled();
        }

        async function checkServices() {
            try {
                await fetch('/health');
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

        async function loadApps() {
            try {
                const resp = await fetch('/apps');
                const data = await resp.json();
                allApps = data.apps || [];
                renderFeatured(allApps);
                renderAllApps(allApps);
                renderInstalled(allApps);
            } catch (e) {
                showError('featuredGrid', 'Failed to load apps');
                showError('appGrid', 'Failed to load apps');
            }
        }

        function renderFeatured(apps) {
            const grid = document.getElementById('featuredGrid');
            if (apps.length === 0) {
                grid.innerHTML = '<div class="empty-state"><div class="empty-icon">⭐</div><div class="empty-text">No featured apps yet</div><div class="empty-subtext">Apps will appear here once available</div></div>';
                return;
            }
            const featured = apps.slice(0, 6);
            grid.innerHTML = featured.map(app => `
                <div class="app-card featured" onclick="showDeploy('${app.name}')">
                    <div class="app-icon">${app.icon}</div>
                    <div class="app-name">${app.name}</div>
                    <div class="app-desc">${app.description}</div>
                    <div class="app-tags">
                        <span class="tag new">Featured</span>
                        ${app.tags.slice(0, 2).map(t => `<span class="tag">${t}</span>`).join('')}
                    </div>
                </div>
            `).join('');
        }

        function renderAllApps(apps) {
            const grid = document.getElementById('appGrid');
            if (apps.length === 0) {
                grid.innerHTML = '<div class="empty-state"><div class="empty-icon">⊞</div><div class="empty-text">No apps available</div><div class="empty-subtext">Build your first app with Merchant AI</div></div>';
                return;
            }
            grid.innerHTML = apps.map(app => `
                <div class="app-card" onclick="showDeploy('${app.name}')">
                    <div class="app-icon">${app.icon}</div>
                    <div class="app-name">${app.name}</div>
                    <div class="app-desc">${app.description}</div>
                    <div class="app-tags">${app.tags.slice(0, 3).map(t => `<span class="tag">${t}</span>`).join('')}</div>
                </div>
            `).join('');
        }

        function renderInstalled(apps) {
            const grid = document.getElementById('installedGrid');
            const countEl = document.getElementById('installedCount');

            // For demo purposes, show all apps as "installed"
            installedApps = apps;
            countEl.textContent = `${apps.length} apps`;

            if (apps.length === 0) {
                grid.innerHTML = '<div class="empty-state"><div class="empty-icon">✓</div><div class="empty-text">No installed apps</div><div class="empty-subtext">Deploy apps from the Apps tab</div></div>';
                return;
            }
            grid.innerHTML = apps.map(app => `
                <div class="app-card" onclick="showDeploy('${app.name}')">
                    <div class="app-icon">${app.icon}</div>
                    <div class="app-name">${app.name}</div>
                    <div class="app-desc">${app.description}</div>
                    <div class="app-tags">
                        <span class="tag" style="background: rgba(166, 227, 161, 0.2); color: var(--ctp-green);">Installed</span>
                    </div>
                </div>
            `).join('');
        }

        function loadInstalled() {
            renderInstalled(allApps);
        }

        function loadUpdates() {
            const container = document.getElementById('updatesContainer');

            if (allApps.length === 0) {
                container.innerHTML = '<div class="empty-state"><div class="empty-icon">↻</div><div class="empty-text">No updates available</div><div class="empty-subtext">All your apps are up to date</div></div>';
                return;
            }

            // Simulate some updates available
            const updates = allApps.slice(0, 3);
            if (updates.length === 0) {
                container.innerHTML = '<div class="empty-state"><div class="empty-icon">✓</div><div class="empty-text">All apps up to date</div><div class="empty-subtext">Check back later for updates</div></div>';
                return;
            }

            container.innerHTML = `
                <button class="update-all-btn" onclick="updateAll()">Update All (${updates.length})</button>
                ${updates.map(app => `
                    <div class="update-card">
                        <div class="update-icon">${app.icon}</div>
                        <div class="update-info">
                            <div class="update-name">${app.name}</div>
                            <div class="update-version">New version available</div>
                        </div>
                        <button class="update-btn" onclick="updateApp('${app.name}')">Update</button>
                    </div>
                `).join('')}
            `;
        }

        function updateApp(name) {
            alert('Updating ' + name + '...');
        }

        function updateAll() {
            alert('Updating all apps...');
        }

        function showError(gridId, message) {
            document.getElementById(gridId).innerHTML = `<div class="empty-state"><div class="empty-icon">⚠</div><div class="empty-text">${message}</div></div>`;
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

                if (data.status === 'deployed') {
                    alert(currentApp + ' deployed successfully!');
                } else {
                    alert('Deploy error: ' + data.error);
                }
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
