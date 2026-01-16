from flask import Flask, request, jsonify, Response
from flask_cors import CORS
import requests
import json
import time
import threading
import subprocess
import os
import concurrent.futures
import socket

app = Flask(__name__)
CORS(app)

# Terminal service URL
TERMINAL_URL = "http://terminal-web.holm:8080"

# Known cluster nodes - statically configured for HolmOS cluster
# This can be extended to read from a config file or Kubernetes
KNOWN_NODES = [
    {"hostname": "rpi-1", "ip": "192.168.8.197"},
    {"hostname": "rpi-2", "ip": "192.168.8.201"},
    {"hostname": "rpi-3", "ip": "192.168.8.202"},
    {"hostname": "rpi-4", "ip": "192.168.8.203"},
    {"hostname": "rpi-5", "ip": "192.168.8.204"},
    {"hostname": "rpi-6", "ip": "192.168.8.205"},
    {"hostname": "rpi-7", "ip": "192.168.8.206"},
    {"hostname": "rpi-8", "ip": "192.168.8.207"},
    {"hostname": "rpi-9", "ip": "192.168.8.208"},
    {"hostname": "rpi-10", "ip": "192.168.8.209"},
    {"hostname": "rpi-11", "ip": "192.168.8.210"},
    {"hostname": "rpi-12", "ip": "192.168.8.211"},
    {"hostname": "openmediavault", "ip": "192.168.8.199"},
]

# Cache for node status
node_cache = {
    "nodes": [],
    "last_updated": 0,
    "metrics": {}
}
cache_lock = threading.Lock()

# Operation status tracking
operations = {}
operation_lock = threading.Lock()

def get_node_list():
    """Get list of all known nodes"""
    return KNOWN_NODES.copy()

def ping_node(ip):
    """Ping a single node to check if online using ICMP ping"""
    try:
        start_time = time.time()
        # Use subprocess to ping - works on Linux
        result = subprocess.run(
            ["ping", "-c", "1", "-W", "2", ip],
            capture_output=True,
            timeout=3
        )
        latency = round((time.time() - start_time) * 1000, 1)
        if result.returncode == 0:
            # Try to extract actual latency from ping output
            output = result.stdout.decode()
            if "time=" in output:
                try:
                    time_part = output.split("time=")[1].split()[0]
                    latency = float(time_part.replace("ms", ""))
                except:
                    pass
            return True, latency
    except subprocess.TimeoutExpired:
        pass
    except Exception as e:
        print(f"Error pinging {ip}: {e}")
    return False, 0

def update_node_cache():
    """Update the cached node status"""
    nodes = get_node_list()
    enriched_nodes = []

    # Parallel ping all nodes
    def ping_and_enrich(node):
        online, latency = ping_node(node.get("ip", ""))
        hostname = node.get("hostname", "")
        return {
            "hostname": hostname,
            "ip": node.get("ip", ""),
            "online": online,
            "latency_ms": latency,
            "is_control_plane": hostname == "rpi-1",
            "is_nas": hostname == "openmediavault",
            "role": "control-plane" if hostname == "rpi-1" else ("nas" if hostname == "openmediavault" else "worker")
        }

    with concurrent.futures.ThreadPoolExecutor(max_workers=15) as executor:
        enriched_nodes = list(executor.map(ping_and_enrich, nodes))

    with cache_lock:
        node_cache["nodes"] = enriched_nodes
        node_cache["last_updated"] = time.time()

    return enriched_nodes

@app.route("/")
def index():
    return get_dashboard_html()

@app.route("/health")
def health():
    """Health check endpoint"""
    return jsonify({"success": True, "data": {"service": "cluster-manager", "status": "healthy"}})

@app.route("/api/v1/nodes")
def api_nodes():
    """Get all nodes with their status"""
    # Check cache freshness (10 seconds)
    with cache_lock:
        if time.time() - node_cache["last_updated"] < 10 and node_cache["nodes"]:
            return jsonify({"success": True, "data": node_cache["nodes"]})

    nodes = update_node_cache()
    return jsonify({"success": True, "data": nodes})

@app.route("/api/v1/nodes/refresh", methods=["POST"])
def api_refresh_nodes():
    """Force refresh node cache"""
    nodes = update_node_cache()
    return jsonify({"success": True, "data": nodes})

@app.route("/api/v1/nodes/<hostname>/ping")
def api_ping_node(hostname):
    """Ping a specific node"""
    with cache_lock:
        node = next((n for n in node_cache["nodes"] if n["hostname"] == hostname), None)

    if not node:
        # Try to find from fresh list
        nodes = get_node_list()
        node = next((n for n in nodes if n.get("hostname") == hostname), None)

    if not node:
        return jsonify({"success": False, "error": "Node not found"}), 404

    online, latency = ping_node(node.get("ip", ""))
    return jsonify({"success": True, "data": {"hostname": hostname, "online": online, "latency_ms": latency}})

def run_ssh_command(ip, command, timeout=60):
    """Run a command on a remote node via SSH"""
    ssh_user = os.environ.get("SSH_USER", "rpi1")
    ssh_password = os.environ.get("SSH_PASSWORD", "19209746")

    try:
        # Use sshpass for password-based SSH
        result = subprocess.run(
            ["sshpass", "-p", ssh_password, "ssh",
             "-o", "StrictHostKeyChecking=no",
             "-o", "ConnectTimeout=10",
             f"{ssh_user}@{ip}", command],
            capture_output=True,
            timeout=timeout
        )
        return {
            "success": result.returncode == 0,
            "output": result.stdout.decode() if result.stdout else "",
            "error": result.stderr.decode() if result.stderr else "",
            "exit_code": result.returncode
        }
    except subprocess.TimeoutExpired:
        return {"success": False, "error": "Command timed out", "exit_code": -1}
    except FileNotFoundError:
        return {"success": False, "error": "sshpass not installed", "exit_code": -1}
    except Exception as e:
        return {"success": False, "error": str(e), "exit_code": -1}

@app.route("/api/v1/nodes/<hostname>/update", methods=["POST"])
def api_update_node(hostname):
    """Trigger apt update on a specific node"""
    with cache_lock:
        node = next((n for n in node_cache["nodes"] if n["hostname"] == hostname), None)

    if not node:
        nodes = get_node_list()
        node = next((n for n in nodes if n.get("hostname") == hostname), None)

    if not node:
        return jsonify({"success": False, "error": "Node not found"}), 404

    result = run_ssh_command(node["ip"], "sudo apt update && sudo apt upgrade -y", timeout=300)

    return jsonify({
        "success": result["success"],
        "data": {
            "hostname": hostname,
            "output": result.get("output", "")[:2000],
            "exit_code": result.get("exit_code", -1)
        }
    })

@app.route("/api/v1/nodes/<hostname>/reboot", methods=["POST"])
def api_reboot_node(hostname):
    """Reboot a specific node"""
    with cache_lock:
        node = next((n for n in node_cache["nodes"] if n["hostname"] == hostname), None)

    if not node:
        nodes = get_node_list()
        node = next((n for n in nodes if n.get("hostname") == hostname), None)

    if not node:
        return jsonify({"success": False, "error": "Node not found"}), 404

    # Start reboot in background - don't wait for response
    def do_reboot():
        run_ssh_command(node["ip"], "sudo reboot", timeout=10)

    threading.Thread(target=do_reboot, daemon=True).start()

    return jsonify({
        "success": True,
        "data": {
            "hostname": hostname,
            "message": "Reboot initiated"
        }
    })

@app.route("/api/v1/update-all", methods=["POST"])
def api_update_all():
    """Update all nodes sequentially"""
    results = []
    nodes = get_node_list()

    for node in nodes:
        result = run_ssh_command(node["ip"], "sudo apt update && sudo apt upgrade -y", timeout=300)
        results.append({
            "hostname": node["hostname"],
            "success": result["success"],
            "output": result.get("output", "")[:500]
        })

    return jsonify({"success": True, "data": {"results": results}})

@app.route("/api/v1/reboot-workers", methods=["POST"])
def api_reboot_workers():
    """Reboot all worker nodes (exclude control plane)"""
    results = []
    nodes = get_node_list()

    def reboot_worker(node):
        """Reboot a single worker node"""
        run_ssh_command(node["ip"], "sudo reboot", timeout=10)
        return {"hostname": node["hostname"], "success": True, "message": "Reboot initiated"}

    for node in nodes:
        # Skip control plane and NAS
        if node.get("hostname") in ["rpi-1", "openmediavault"]:
            continue

        # Start reboot in background
        threading.Thread(target=reboot_worker, args=(node,), daemon=True).start()
        results.append({
            "hostname": node["hostname"],
            "success": True,
            "message": "Reboot initiated"
        })

    return jsonify({"success": True, "data": {"results": results}})

@app.route("/api/v1/health")
def api_health():
    """Get cluster health overview"""
    with cache_lock:
        nodes = node_cache["nodes"] if node_cache["nodes"] else []

    if not nodes:
        nodes = update_node_cache()

    total = len(nodes)
    online = sum(1 for n in nodes if n.get("online"))
    offline = total - online

    control_plane_online = any(n.get("is_control_plane") and n.get("online") for n in nodes)
    nas_online = any(n.get("is_nas") and n.get("online") for n in nodes)
    workers_online = sum(1 for n in nodes if not n.get("is_control_plane") and not n.get("is_nas") and n.get("online"))
    workers_total = sum(1 for n in nodes if not n.get("is_control_plane") and not n.get("is_nas"))

    avg_latency = 0
    online_with_latency = [n for n in nodes if n.get("online") and n.get("latency_ms", 0) > 0]
    if online_with_latency:
        avg_latency = sum(n["latency_ms"] for n in online_with_latency) / len(online_with_latency)

    health_pct = round((online / total) * 100) if total > 0 else 0

    return jsonify({
        "success": True,
        "data": {
            "total_nodes": total,
            "online_nodes": online,
            "offline_nodes": offline,
            "health_percentage": health_pct,
            "control_plane_online": control_plane_online,
            "nas_online": nas_online,
            "workers_online": workers_online,
            "workers_total": workers_total,
            "average_latency_ms": round(avg_latency, 1),
            "last_updated": node_cache["last_updated"]
        }
    })

@app.route("/api/v1/terminal/url/<hostname>")
def api_terminal_url(hostname):
    """Get SSH terminal URL for a node"""
    with cache_lock:
        node = next((n for n in node_cache["nodes"] if n["hostname"] == hostname), None)

    if not node:
        nodes = get_node_list()
        node = next((n for n in nodes if n.get("hostname") == hostname), None)

    if not node:
        return jsonify({"success": False, "error": "Node not found"}), 404

    # Return terminal URL - works with holmos-shell
    terminal_url = f"http://192.168.8.197:30088/terminal?host={node['ip']}"

    return jsonify({
        "success": True,
        "data": {
            "hostname": hostname,
            "ip": node["ip"],
            "terminal_url": terminal_url
        }
    })


def get_dashboard_html():
    return '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <title>HolmOS Cluster Admin</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; -webkit-tap-highlight-color: transparent; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0f0f17; color: #e2e8f0; min-height: 100vh; padding-bottom: 80px; }

        .header {
            padding: 16px 20px;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            border-bottom: 1px solid #2d3748;
            position: sticky;
            top: 0;
            z-index: 100;
        }
        .header-top { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
        .header h1 { font-size: 22px; font-weight: 700; display: flex; align-items: center; gap: 10px; }
        .header h1 .logo { width: 32px; height: 32px; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); border-radius: 8px; display: flex; align-items: center; justify-content: center; font-size: 18px; }
        .header-actions { display: flex; gap: 8px; }
        .header-btn {
            padding: 8px 14px;
            background: rgba(255,255,255,0.1);
            border: none;
            border-radius: 8px;
            color: #e2e8f0;
            font-size: 13px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s;
        }
        .header-btn:hover { background: rgba(255,255,255,0.15); }
        .header-btn:active { transform: scale(0.97); }

        .health-overview {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 10px;
        }
        .health-stat {
            background: rgba(255,255,255,0.05);
            padding: 12px;
            border-radius: 10px;
            text-align: center;
        }
        .health-stat-value { font-size: 24px; font-weight: 700; margin-bottom: 2px; }
        .health-stat-value.good { color: #68d391; }
        .health-stat-value.warning { color: #f6e05e; }
        .health-stat-value.critical { color: #fc8181; }
        .health-stat-label { font-size: 11px; color: #a0aec0; text-transform: uppercase; letter-spacing: 0.5px; }

        .tabs {
            display: flex;
            background: #1a1a2e;
            border-bottom: 1px solid #2d3748;
            overflow-x: auto;
            -webkit-overflow-scrolling: touch;
        }
        .tab {
            padding: 14px 20px;
            color: #718096;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            white-space: nowrap;
            border-bottom: 2px solid transparent;
            transition: all 0.2s;
        }
        .tab:hover { color: #e2e8f0; }
        .tab.active { color: #667eea; border-bottom-color: #667eea; }

        .tab-content { display: none; }
        .tab-content.active { display: block; }

        .actions-bar {
            padding: 12px 20px;
            background: #1a1a2e;
            border-bottom: 1px solid #2d3748;
            display: flex;
            gap: 10px;
            overflow-x: auto;
            -webkit-overflow-scrolling: touch;
        }
        .action-btn {
            padding: 10px 16px;
            background: #2d3748;
            border: none;
            border-radius: 10px;
            color: #e2e8f0;
            font-size: 13px;
            font-weight: 600;
            cursor: pointer;
            white-space: nowrap;
            display: flex;
            align-items: center;
            gap: 6px;
            transition: all 0.2s;
        }
        .action-btn:hover { background: #4a5568; }
        .action-btn:active { transform: scale(0.97); }
        .action-btn.primary { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); }
        .action-btn.success { background: #48bb78; color: #1a202c; }
        .action-btn.danger { background: #fc8181; color: #1a202c; }
        .action-btn:disabled { opacity: 0.5; cursor: not-allowed; }

        /* Nodes Grid */
        .nodes-container { padding: 16px; }
        .nodes-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
            gap: 12px;
        }
        @media (max-width: 700px) {
            .nodes-grid { grid-template-columns: 1fr; }
        }

        .node-card {
            background: linear-gradient(135deg, #1a1a2e 0%, #1e2538 100%);
            border-radius: 16px;
            padding: 16px;
            border: 1px solid #2d3748;
            transition: all 0.3s;
            position: relative;
            overflow: hidden;
        }
        .node-card::before {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
            height: 3px;
            background: linear-gradient(90deg, #667eea 0%, #764ba2 100%);
            opacity: 0;
            transition: opacity 0.3s;
        }
        .node-card:hover { border-color: #4a5568; transform: translateY(-2px); }
        .node-card:hover::before { opacity: 1; }
        .node-card.offline { opacity: 0.7; }
        .node-card.updating { border-color: #f6e05e; }
        .node-card.updating::before { background: #f6e05e; opacity: 1; }

        .node-header {
            display: flex;
            align-items: flex-start;
            justify-content: space-between;
            margin-bottom: 14px;
        }
        .node-info { display: flex; align-items: center; gap: 12px; }
        .node-status {
            width: 12px;
            height: 12px;
            border-radius: 50%;
            flex-shrink: 0;
            margin-top: 4px;
        }
        .node-status.online { background: #68d391; box-shadow: 0 0 12px rgba(104, 211, 145, 0.5); }
        .node-status.offline { background: #fc8181; }
        .node-status.updating { background: #f6e05e; animation: pulse 1s infinite; }
        @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }

        .node-name { font-size: 17px; font-weight: 600; margin-bottom: 2px; }
        .node-ip { font-size: 12px; color: #718096; font-family: 'SF Mono', 'Consolas', monospace; }

        .node-badges { display: flex; gap: 6px; flex-wrap: wrap; }
        .node-badge {
            padding: 4px 10px;
            border-radius: 6px;
            font-size: 10px;
            font-weight: 700;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .node-badge.control-plane { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; }
        .node-badge.nas { background: linear-gradient(135deg, #ed8936 0%, #dd6b20 100%); color: white; }
        .node-badge.worker { background: #4a5568; color: #e2e8f0; }

        .node-metrics {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 8px;
            margin-bottom: 14px;
        }
        .node-metric {
            background: rgba(0,0,0,0.2);
            padding: 10px;
            border-radius: 10px;
            text-align: center;
        }
        .node-metric-label { font-size: 10px; color: #718096; margin-bottom: 4px; text-transform: uppercase; }
        .node-metric-value { font-size: 15px; font-weight: 600; }
        .node-metric-value.good { color: #68d391; }
        .node-metric-value.warning { color: #f6e05e; }
        .node-metric-value.bad { color: #fc8181; }

        .node-actions {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 8px;
        }
        .node-btn {
            padding: 10px 8px;
            background: #2d3748;
            border: none;
            border-radius: 10px;
            color: #e2e8f0;
            font-size: 11px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s;
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 4px;
        }
        .node-btn:hover { background: #4a5568; transform: translateY(-1px); }
        .node-btn:active { transform: scale(0.97); }
        .node-btn .icon { font-size: 16px; }
        .node-btn.ssh { background: linear-gradient(135deg, #48bb78 0%, #38a169 100%); color: #1a202c; }
        .node-btn.update { background: linear-gradient(135deg, #4299e1 0%, #3182ce 100%); color: white; }
        .node-btn.reboot { background: linear-gradient(135deg, #ed8936 0%, #dd6b20 100%); color: white; }
        .node-btn:disabled { opacity: 0.4; cursor: not-allowed; transform: none; }

        /* Terminal Tab */
        .terminal-container { padding: 16px; }
        .terminal-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            gap: 12px;
            margin-bottom: 16px;
            flex-wrap: wrap;
        }
        .terminal-selector {
            display: flex;
            gap: 10px;
            align-items: center;
            flex: 1;
            min-width: 200px;
        }
        .terminal-selector select {
            flex: 1;
            padding: 10px 14px;
            background: #2d3748;
            border: 1px solid #4a5568;
            border-radius: 10px;
            color: #e2e8f0;
            font-size: 14px;
        }
        .terminal-frame {
            background: #0d1117;
            border-radius: 12px;
            overflow: hidden;
            border: 1px solid #2d3748;
            min-height: 500px;
        }
        .terminal-frame iframe {
            width: 100%;
            height: 550px;
            border: none;
        }
        .terminal-placeholder {
            padding: 60px;
            text-align: center;
            color: #718096;
        }
        .terminal-placeholder .icon { font-size: 48px; margin-bottom: 16px; opacity: 0.5; }

        /* Health Dashboard */
        .health-dashboard { padding: 16px; }
        .health-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
            gap: 16px;
        }
        .health-card {
            background: linear-gradient(135deg, #1a1a2e 0%, #1e2538 100%);
            border-radius: 16px;
            padding: 20px;
            border: 1px solid #2d3748;
        }
        .health-card-header {
            display: flex;
            align-items: center;
            gap: 12px;
            margin-bottom: 16px;
        }
        .health-card-icon { font-size: 24px; }
        .health-card-title { font-size: 16px; font-weight: 600; }
        .health-card-value { font-size: 36px; font-weight: 700; margin-bottom: 4px; }
        .health-card-label { font-size: 13px; color: #718096; }
        .health-bar {
            height: 8px;
            background: #2d3748;
            border-radius: 4px;
            overflow: hidden;
            margin-top: 12px;
        }
        .health-bar-fill {
            height: 100%;
            border-radius: 4px;
            transition: width 0.5s ease;
        }
        .health-bar-fill.good { background: linear-gradient(90deg, #48bb78 0%, #68d391 100%); }
        .health-bar-fill.warning { background: linear-gradient(90deg, #ed8936 0%, #f6e05e 100%); }
        .health-bar-fill.critical { background: linear-gradient(90deg, #e53e3e 0%, #fc8181 100%); }

        .node-list-compact {
            margin-top: 16px;
        }
        .node-list-item {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 10px 12px;
            background: rgba(0,0,0,0.2);
            border-radius: 8px;
            margin-bottom: 6px;
        }
        .node-list-item-info { display: flex; align-items: center; gap: 10px; }
        .node-list-item-status { width: 8px; height: 8px; border-radius: 50%; }
        .node-list-item-status.online { background: #68d391; }
        .node-list-item-status.offline { background: #fc8181; }
        .node-list-item-name { font-size: 13px; font-weight: 500; }
        .node-list-item-latency { font-size: 12px; color: #718096; }

        /* Log Panel */
        .log-panel {
            position: fixed;
            bottom: 70px;
            left: 0;
            right: 0;
            background: #1a1a2e;
            border-top: 1px solid #2d3748;
            max-height: 50vh;
            transform: translateY(100%);
            transition: transform 0.3s ease;
            z-index: 200;
        }
        .log-panel.active { transform: translateY(0); }
        .log-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 14px 20px;
            border-bottom: 1px solid #2d3748;
            position: sticky;
            top: 0;
            background: #1a1a2e;
        }
        .log-title { font-size: 15px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
        .log-status {
            padding: 4px 10px;
            border-radius: 6px;
            font-size: 11px;
            font-weight: 600;
        }
        .log-status.running { background: #f6e05e; color: #1a202c; }
        .log-status.completed { background: #68d391; color: #1a202c; }
        .log-status.failed { background: #fc8181; color: #1a202c; }
        .log-content {
            padding: 14px 20px;
            max-height: 38vh;
            overflow-y: auto;
            font-family: 'SF Mono', 'Consolas', monospace;
            font-size: 12px;
            line-height: 1.6;
        }
        .log-entry {
            padding: 10px 14px;
            background: rgba(0,0,0,0.3);
            border-radius: 8px;
            margin-bottom: 8px;
            border-left: 3px solid #4a5568;
        }
        .log-entry.success { border-left-color: #68d391; }
        .log-entry.error { border-left-color: #fc8181; }
        .log-entry.running { border-left-color: #f6e05e; }
        .log-entry-header { display: flex; justify-content: space-between; margin-bottom: 4px; }
        .log-entry-host { font-weight: 600; color: #667eea; }
        .log-entry-status { font-size: 11px; color: #a0aec0; }
        .log-close {
            background: #2d3748;
            border: none;
            padding: 8px 16px;
            border-radius: 8px;
            color: #e2e8f0;
            cursor: pointer;
            font-size: 13px;
            font-weight: 500;
        }
        .log-close:hover { background: #4a5568; }

        /* Modal */
        .modal-overlay {
            position: fixed;
            inset: 0;
            background: rgba(0,0,0,0.85);
            display: flex;
            align-items: center;
            justify-content: center;
            z-index: 1000;
            opacity: 0;
            visibility: hidden;
            transition: all 0.2s;
            padding: 20px;
        }
        .modal-overlay.open { opacity: 1; visibility: visible; }
        .modal {
            background: #1a1a2e;
            border-radius: 20px;
            width: 100%;
            max-width: 420px;
            padding: 24px;
            border: 1px solid #2d3748;
        }
        .modal h3 { margin-bottom: 12px; font-size: 20px; display: flex; align-items: center; gap: 10px; }
        .modal p { margin-bottom: 20px; color: #a0aec0; font-size: 14px; line-height: 1.6; }
        .modal p.warning { color: #fc8181; background: rgba(252,129,129,0.1); padding: 12px; border-radius: 8px; }
        .modal-actions { display: flex; gap: 12px; }
        .modal-btn {
            flex: 1;
            padding: 14px;
            border: none;
            border-radius: 12px;
            font-size: 14px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s;
        }
        .modal-btn:active { transform: scale(0.97); }
        .modal-btn.cancel { background: #2d3748; color: #e2e8f0; }
        .modal-btn.confirm { background: #fc8181; color: #1a202c; }
        .modal-btn.primary { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; }

        /* Dock */
        .dock {
            position: fixed;
            bottom: 0;
            left: 0;
            right: 0;
            height: 70px;
            background: rgba(26, 26, 46, 0.95);
            backdrop-filter: blur(20px);
            border-top: 1px solid #2d3748;
            display: flex;
            justify-content: center;
            align-items: center;
            gap: 8px;
            padding: 0 20px;
            z-index: 100;
        }
        .dock-item {
            width: 48px;
            height: 48px;
            border-radius: 12px;
            background: #2d3748;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 22px;
            cursor: pointer;
            transition: all 0.2s;
        }
        .dock-item:hover { background: #4a5568; transform: translateY(-3px); }
        .dock-item:active { transform: scale(0.95); }
        .dock-item.active { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); }
        .dock-separator { width: 1px; height: 36px; background: #2d3748; margin: 0 6px; }

        .loading { text-align: center; padding: 60px; color: #718096; }
        .loading-spinner {
            width: 40px;
            height: 40px;
            border: 3px solid #2d3748;
            border-top-color: #667eea;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin: 0 auto 16px;
        }
        @keyframes spin { to { transform: rotate(360deg); } }

        /* Toast notifications */
        .toast-container {
            position: fixed;
            top: 80px;
            right: 20px;
            z-index: 1001;
            display: flex;
            flex-direction: column;
            gap: 8px;
        }
        .toast {
            background: #1a1a2e;
            border: 1px solid #2d3748;
            border-radius: 10px;
            padding: 12px 16px;
            display: flex;
            align-items: center;
            gap: 10px;
            animation: slideIn 0.3s ease;
            min-width: 250px;
            box-shadow: 0 4px 20px rgba(0,0,0,0.3);
        }
        .toast.success { border-left: 3px solid #68d391; }
        .toast.error { border-left: 3px solid #fc8181; }
        .toast.info { border-left: 3px solid #667eea; }
        @keyframes slideIn { from { transform: translateX(100%); opacity: 0; } to { transform: translateX(0); opacity: 1; } }
    </style>
</head>
<body>
    <div class="header">
        <div class="header-top">
            <h1><span class="logo">&#9881;</span> Cluster Admin</h1>
            <div class="header-actions">
                <button class="header-btn" onclick="refreshNodes()">&#8635; Refresh</button>
            </div>
        </div>
        <div class="health-overview" id="healthOverview">
            <div class="health-stat">
                <div class="health-stat-value good" id="statOnline">-</div>
                <div class="health-stat-label">Online</div>
            </div>
            <div class="health-stat">
                <div class="health-stat-value" id="statOffline">-</div>
                <div class="health-stat-label">Offline</div>
            </div>
            <div class="health-stat">
                <div class="health-stat-value" id="statLatency">-</div>
                <div class="health-stat-label">Avg Latency</div>
            </div>
            <div class="health-stat">
                <div class="health-stat-value" id="statHealth">-</div>
                <div class="health-stat-label">Health</div>
            </div>
        </div>
    </div>

    <div class="tabs">
        <div class="tab active" data-tab="nodes">&#128187; Nodes</div>
        <div class="tab" data-tab="terminal">&#128421; Terminal</div>
        <div class="tab" data-tab="health">&#128200; Health</div>
        <div class="tab" data-tab="operations">&#9881; Operations</div>
    </div>

    <!-- Nodes Tab -->
    <div class="tab-content active" id="tab-nodes">
        <div class="actions-bar">
            <button class="action-btn" onclick="pingAll()">&#128225; Ping All</button>
            <button class="action-btn primary" onclick="confirmUpdateAll()">&#128190; Update All</button>
            <button class="action-btn danger" onclick="confirmRebootWorkers()">&#9888; Reboot Workers</button>
        </div>
        <div class="nodes-container">
            <div class="nodes-grid" id="nodesGrid">
                <div class="loading">
                    <div class="loading-spinner"></div>
                    Loading nodes...
                </div>
            </div>
        </div>
    </div>

    <!-- Terminal Tab -->
    <div class="tab-content" id="tab-terminal">
        <div class="terminal-container">
            <div class="terminal-header">
                <div class="terminal-selector">
                    <select id="terminalNodeSelect" onchange="updateConnectButton()">
                        <option value="">Select a node...</option>
                    </select>
                    <button class="action-btn primary" id="connectBtn" onclick="connectTerminal()" disabled>&#128279; Connect</button>
                </div>
            </div>
            <div class="terminal-frame" id="terminalFrame">
                <div class="terminal-placeholder">
                    <div class="icon">&#128421;</div>
                    <div>Select a node above to open SSH terminal</div>
                </div>
            </div>
        </div>
    </div>

    <!-- Health Tab -->
    <div class="tab-content" id="tab-health">
        <div class="health-dashboard">
            <div class="health-grid" id="healthGrid">
                <div class="loading">
                    <div class="loading-spinner"></div>
                    Loading health data...
                </div>
            </div>
        </div>
    </div>

    <!-- Operations Tab -->
    <div class="tab-content" id="tab-operations">
        <div class="nodes-container">
            <div class="health-grid">
                <div class="health-card">
                    <div class="health-card-header">
                        <span class="health-card-icon">&#128190;</span>
                        <span class="health-card-title">System Updates</span>
                    </div>
                    <p style="color:#718096;margin-bottom:16px;">Run apt update on cluster nodes to check for package updates.</p>
                    <button class="action-btn primary" onclick="confirmUpdateAll()" style="width:100%;">Update All Nodes</button>
                </div>
                <div class="health-card">
                    <div class="health-card-header">
                        <span class="health-card-icon">&#128260;</span>
                        <span class="health-card-title">Worker Reboot</span>
                    </div>
                    <p style="color:#718096;margin-bottom:16px;">Reboot all worker nodes. Control plane and NAS excluded.</p>
                    <button class="action-btn danger" onclick="confirmRebootWorkers()" style="width:100%;">Reboot Workers</button>
                </div>
                <div class="health-card">
                    <div class="health-card-header">
                        <span class="health-card-icon">&#128225;</span>
                        <span class="health-card-title">Network Check</span>
                    </div>
                    <p style="color:#718096;margin-bottom:16px;">Ping all nodes to verify network connectivity.</p>
                    <button class="action-btn" onclick="pingAll()" style="width:100%;">Ping All Nodes</button>
                </div>
                <div class="health-card">
                    <div class="health-card-header">
                        <span class="health-card-icon">&#128260;</span>
                        <span class="health-card-title">Cluster Refresh</span>
                    </div>
                    <p style="color:#718096;margin-bottom:16px;">Force refresh node status cache.</p>
                    <button class="action-btn" onclick="refreshNodes()" style="width:100%;">Refresh Status</button>
                </div>
            </div>
        </div>
    </div>

    <!-- Log Panel -->
    <div class="log-panel" id="logPanel">
        <div class="log-header">
            <span class="log-title"><span id="logIcon">&#9881;</span> <span id="logTitle">Operation Log</span></span>
            <div style="display:flex;gap:10px;align-items:center;">
                <span class="log-status" id="logStatus">Running</span>
                <button class="log-close" onclick="closeLogPanel()">Close</button>
            </div>
        </div>
        <div class="log-content" id="logContent"></div>
    </div>

    <!-- Confirm Modal -->
    <div class="modal-overlay" id="confirmModal">
        <div class="modal">
            <h3 id="modalTitle">Confirm</h3>
            <p id="modalMessage">Are you sure?</p>
            <div class="modal-actions">
                <button class="modal-btn cancel" onclick="closeModal()">Cancel</button>
                <button class="modal-btn confirm" id="modalConfirm">Confirm</button>
            </div>
        </div>
    </div>

    <!-- Toast Container -->
    <div class="toast-container" id="toastContainer"></div>

    <!-- Dock -->
    <div class="dock">
        <div class="dock-item" onclick="window.location.href='http://192.168.8.197:30088/'">&#128193;</div>
        <div class="dock-item" onclick="window.location.href='http://192.168.8.197:30088/terminal'">&#128187;</div>
        <div class="dock-separator"></div>
        <div class="dock-item active">&#9881;</div>
        <div class="dock-item" onclick="window.location.href='http://192.168.8.197:31750/'">&#128230;</div>
    </div>

    <script>
        let nodes = [];
        let healthData = null;

        // Tab switching
        document.querySelectorAll('.tab').forEach(tab => {
            tab.addEventListener('click', () => {
                document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
                document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
                tab.classList.add('active');
                document.getElementById('tab-' + tab.dataset.tab).classList.add('active');

                if (tab.dataset.tab === 'health') loadHealthData();
            });
        });

        function showToast(message, type = 'info') {
            const container = document.getElementById('toastContainer');
            const toast = document.createElement('div');
            toast.className = 'toast ' + type;
            toast.innerHTML = '<span>' + message + '</span>';
            container.appendChild(toast);
            setTimeout(() => toast.remove(), 4000);
        }

        async function fetchNodes() {
            const endpoint = '/api/v1/nodes';
            const TIMEOUT_MS = 10000;
            const controller = new AbortController();
            const timeoutId = setTimeout(() => {
                controller.abort();
            }, TIMEOUT_MS);
            const startTime = Date.now();

            try {
                const res = await fetch(endpoint, { signal: controller.signal });
                clearTimeout(timeoutId);

                if (!res.ok) {
                    throw new Error('HTTP ' + res.status + ': ' + res.statusText);
                }

                const data = await res.json();
                if (data.success) {
                    nodes = data.data;
                    renderNodes();
                    updateHeaderStats();
                    updateTerminalSelect();
                } else {
                    throw new Error(data.error || 'API returned success=false');
                }
            } catch (err) {
                clearTimeout(timeoutId);
                const elapsed = Date.now() - startTime;
                console.error('Failed to fetch nodes:', err, 'after', elapsed, 'ms');

                let errorMsg = err.message;
                let errorType = 'Error';
                if (err.name === 'AbortError') {
                    errorMsg = 'Request timed out after ' + (TIMEOUT_MS / 1000) + ' seconds';
                    errorType = 'Timeout';
                } else if (err.message.includes('Failed to fetch') || err.message.includes('NetworkError')) {
                    errorMsg = 'Network error - backend may be unreachable';
                    errorType = 'Network Error';
                }

                // Show error in the grid with diagnostics
                const grid = document.getElementById('nodesGrid');
                const timestamp = new Date().toISOString();
                grid.innerHTML =
                    '<div class="error-state" style="padding:40px;text-align:center;background:#2d1f1f;border-radius:12px;border:1px solid #fc8181;max-width:500px;margin:0 auto;">' +
                        '<div style="font-size:48px;margin-bottom:16px;">&#9888;</div>' +
                        '<div style="font-size:20px;font-weight:600;color:#fc8181;margin-bottom:8px;">' + errorType + '</div>' +
                        '<div style="font-size:16px;color:#e2e8f0;margin-bottom:20px;">' + errorMsg + '</div>' +
                        '<div style="background:#1a1a2e;border-radius:8px;padding:16px;margin-bottom:20px;text-align:left;font-family:monospace;font-size:12px;color:#a0aec0;">' +
                            '<div style="margin-bottom:8px;"><span style="color:#718096;">Endpoint:</span> ' + window.location.origin + endpoint + '</div>' +
                            '<div style="margin-bottom:8px;"><span style="color:#718096;">Timestamp:</span> ' + timestamp + '</div>' +
                            '<div><span style="color:#718096;">Elapsed:</span> ' + elapsed + 'ms</div>' +
                        '</div>' +
                        '<button onclick="fetchNodes()" class="action-btn primary" style="padding:14px 28px;font-size:15px;">&#8635; Retry</button>' +
                    '</div>';

                showToast(errorType + ': ' + errorMsg, 'error');
            }
        }

        async function fetchHealth() {
            const TIMEOUT_MS = 10000;
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);

            try {
                const res = await fetch('/api/v1/health', { signal: controller.signal });
                clearTimeout(timeoutId);

                if (!res.ok) {
                    throw new Error('HTTP ' + res.status);
                }

                const data = await res.json();
                if (data.success) {
                    healthData = data.data;
                    updateHeaderStats();
                }
            } catch (err) {
                clearTimeout(timeoutId);
                console.error('Failed to fetch health:', err);
                // Don't show toast for health - it's a background operation
            }
        }

        function updateHeaderStats() {
            if (!nodes.length) return;

            const online = nodes.filter(n => n.online).length;
            const offline = nodes.filter(n => !n.online).length;
            const avgLatency = nodes.filter(n => n.online && n.latency_ms).reduce((a, n) => a + n.latency_ms, 0) / (online || 1);
            const healthPct = Math.round((online / nodes.length) * 100);

            document.getElementById('statOnline').textContent = online;
            document.getElementById('statOnline').className = 'health-stat-value ' + (online === nodes.length ? 'good' : online > 0 ? 'warning' : 'critical');

            document.getElementById('statOffline').textContent = offline;
            document.getElementById('statOffline').className = 'health-stat-value ' + (offline === 0 ? 'good' : offline < 3 ? 'warning' : 'critical');

            document.getElementById('statLatency').textContent = Math.round(avgLatency) + 'ms';
            document.getElementById('statLatency').className = 'health-stat-value ' + (avgLatency < 50 ? 'good' : avgLatency < 100 ? 'warning' : 'critical');

            document.getElementById('statHealth').textContent = healthPct + '%';
            document.getElementById('statHealth').className = 'health-stat-value ' + (healthPct >= 90 ? 'good' : healthPct >= 70 ? 'warning' : 'critical');
        }

        function renderNodes() {
            const grid = document.getElementById('nodesGrid');
            if (nodes.length === 0) {
                grid.innerHTML = '<div class="error-state" style="padding:40px;text-align:center;background:#2d3748;border-radius:12px;">' +
                    '<div style="font-size:40px;margin-bottom:16px;">&#128421;</div>' +
                    '<div style="font-size:16px;color:#a0aec0;margin-bottom:16px;">No nodes found in cluster</div>' +
                    '<div style="font-size:12px;color:#718096;">Expected 13 nodes (rpi-1 through rpi-12 + openmediavault)</div>' +
                '</div>';
                return;
            }

            // Sort: control plane first, then NAS, then workers
            const sorted = [...nodes].sort((a, b) => {
                if (a.is_control_plane) return -1;
                if (b.is_control_plane) return 1;
                if (a.is_nas) return -1;
                if (b.is_nas) return 1;
                return a.hostname.localeCompare(b.hostname);
            });

            grid.innerHTML = sorted.map(node => {
                const badgeClass = node.is_control_plane ? 'control-plane' : (node.is_nas ? 'nas' : 'worker');
                const badgeText = node.is_control_plane ? 'Control Plane' : (node.is_nas ? 'NAS' : 'Worker');
                const statusClass = node.online ? 'online' : 'offline';
                const cardClass = node.online ? '' : 'offline';

                return '<div class="node-card ' + cardClass + '" id="card-' + node.hostname + '">' +
                    '<div class="node-header">' +
                        '<div class="node-info">' +
                            '<div class="node-status ' + statusClass + '"></div>' +
                            '<div>' +
                                '<div class="node-name">' + node.hostname + '</div>' +
                                '<div class="node-ip">' + node.ip + '</div>' +
                            '</div>' +
                        '</div>' +
                        '<div class="node-badges">' +
                            '<span class="node-badge ' + badgeClass + '">' + badgeText + '</span>' +
                        '</div>' +
                    '</div>' +
                    '<div class="node-metrics">' +
                        '<div class="node-metric">' +
                            '<div class="node-metric-label">Status</div>' +
                            '<div class="node-metric-value ' + (node.online ? 'good' : 'bad') + '">' + (node.online ? 'Online' : 'Offline') + '</div>' +
                        '</div>' +
                        '<div class="node-metric">' +
                            '<div class="node-metric-label">Latency</div>' +
                            '<div class="node-metric-value" id="latency-' + node.hostname + '">' + (node.latency_ms ? node.latency_ms + 'ms' : '--') + '</div>' +
                        '</div>' +
                        '<div class="node-metric">' +
                            '<div class="node-metric-label">Role</div>' +
                            '<div class="node-metric-value">' + node.role + '</div>' +
                        '</div>' +
                    '</div>' +
                    '<div class="node-actions">' +
                        '<button class="node-btn" onclick="pingNode(\'' + node.hostname + '\')" ' + (!node.online ? 'disabled' : '') + '><span class="icon">&#128225;</span>Ping</button>' +
                        '<button class="node-btn update" onclick="updateNode(\'' + node.hostname + '\')" ' + (!node.online ? 'disabled' : '') + '><span class="icon">&#128190;</span>Update</button>' +
                        '<button class="node-btn reboot" onclick="confirmRebootNode(\'' + node.hostname + '\')" ' + (!node.online ? 'disabled' : '') + '><span class="icon">&#128260;</span>Reboot</button>' +
                        '<button class="node-btn ssh" onclick="openTerminal(\'' + node.hostname + '\')" ' + (!node.online ? 'disabled' : '') + '><span class="icon">&#128421;</span>SSH</button>' +
                    '</div>' +
                '</div>';
            }).join('');
        }

        function updateTerminalSelect() {
            const select = document.getElementById('terminalNodeSelect');
            const sorted = [...nodes].filter(n => n.online).sort((a, b) => {
                if (a.is_control_plane) return -1;
                if (b.is_control_plane) return 1;
                return a.hostname.localeCompare(b.hostname);
            });

            select.innerHTML = '<option value="">Select a node...</option>' +
                sorted.map(n =>
                    '<option value="' + n.ip + '" data-hostname="' + n.hostname + '">' + n.hostname + ' (' + n.ip + ')' + (n.is_control_plane ? ' [Control Plane]' : '') + '</option>'
                ).join('');
        }

        function updateConnectButton() {
            const select = document.getElementById('terminalNodeSelect');
            document.getElementById('connectBtn').disabled = !select.value;
        }

        async function pingNode(hostname) {
            const latencyEl = document.getElementById('latency-' + hostname);
            if (latencyEl) latencyEl.textContent = '...';

            try {
                const start = Date.now();
                const res = await fetch('/api/v1/nodes/' + hostname + '/ping');
                const data = await res.json();

                if (data.success && data.data.online) {
                    const latency = data.data.latency_ms || (Date.now() - start);
                    if (latencyEl) {
                        latencyEl.textContent = latency + 'ms';
                        latencyEl.className = 'node-metric-value ' + (latency < 50 ? 'good' : latency < 100 ? 'warning' : 'bad');
                    }
                    showToast(hostname + ' responded in ' + latency + 'ms', 'success');
                } else {
                    if (latencyEl) latencyEl.textContent = 'Fail';
                    showToast(hostname + ' not responding', 'error');
                }
            } catch (err) {
                if (latencyEl) latencyEl.textContent = 'Err';
                showToast('Error pinging ' + hostname, 'error');
            }
        }

        async function pingAll() {
            showToast('Pinging all nodes...', 'info');
            const onlineNodes = nodes.filter(n => n.online);
            for (const node of onlineNodes) {
                pingNode(node.hostname);
                await new Promise(r => setTimeout(r, 100)); // Slight delay between pings
            }
        }

        async function updateNode(hostname) {
            const card = document.getElementById('card-' + hostname);
            if (card) card.classList.add('updating');

            showLog('Updating ' + hostname, 'running');
            addLogEntry(hostname, 'Starting apt update...', 'running');

            try {
                const res = await fetch('/api/v1/nodes/' + hostname + '/update', { method: 'POST' });
                const data = await res.json();

                if (data.success) {
                    updateLogEntry(hostname, 'Update completed', 'success');
                    showToast(hostname + ' updated successfully', 'success');
                } else {
                    updateLogEntry(hostname, 'Update failed: ' + (data.error || 'Unknown error'), 'error');
                    showToast(hostname + ' update failed', 'error');
                }
            } catch (err) {
                updateLogEntry(hostname, 'Error: ' + err.message, 'error');
                showToast('Error updating ' + hostname, 'error');
            }

            if (card) card.classList.remove('updating');
            setLogStatus('completed');
        }

        function confirmRebootNode(hostname) {
            const node = nodes.find(n => n.hostname === hostname);
            const isCP = node && node.is_control_plane;
            const isNAS = node && node.is_nas;

            let warning = '';
            if (isCP) {
                warning = '<p class="warning">WARNING: This is the Control Plane! Rebooting will make the entire cluster temporarily unavailable.</p>';
            } else if (isNAS) {
                warning = '<p class="warning">WARNING: This is the NAS node! Storage-dependent services may be affected.</p>';
            }

            document.getElementById('modalTitle').innerHTML = '&#128260; Reboot ' + hostname;
            document.getElementById('modalMessage').innerHTML = warning + 'Are you sure you want to reboot <strong>' + hostname + '</strong>?<br><br>The node will be unavailable for 1-3 minutes.';
            document.getElementById('modalConfirm').onclick = () => rebootNode(hostname);
            document.getElementById('modalConfirm').className = 'modal-btn confirm';
            document.getElementById('confirmModal').classList.add('open');
        }

        async function rebootNode(hostname) {
            closeModal();
            showLog('Rebooting ' + hostname, 'running');
            addLogEntry(hostname, 'Initiating reboot...', 'running');

            try {
                const res = await fetch('/api/v1/nodes/' + hostname + '/reboot', { method: 'POST' });
                const data = await res.json();

                if (data.success) {
                    updateLogEntry(hostname, 'Reboot initiated - node will be back online shortly', 'success');
                    showToast(hostname + ' is rebooting', 'success');
                } else {
                    updateLogEntry(hostname, 'Reboot failed: ' + (data.error || 'Unknown'), 'error');
                    showToast(hostname + ' reboot failed', 'error');
                }
            } catch (err) {
                updateLogEntry(hostname, 'Reboot command sent (connection lost)', 'success');
                showToast(hostname + ' is rebooting', 'success');
            }

            setLogStatus('completed');
            setTimeout(fetchNodes, 15000);
        }

        function confirmUpdateAll() {
            const onlineCount = nodes.filter(n => n.online).length;
            document.getElementById('modalTitle').innerHTML = '&#128190; Update All Nodes';
            document.getElementById('modalMessage').innerHTML = 'This will run <code>apt update</code> on all <strong>' + onlineCount + '</strong> online nodes.<br><br>This may take several minutes. Continue?';
            document.getElementById('modalConfirm').onclick = updateAllNodes;
            document.getElementById('modalConfirm').className = 'modal-btn primary';
            document.getElementById('confirmModal').classList.add('open');
        }

        async function updateAllNodes() {
            closeModal();
            showLog('Updating All Nodes', 'running');

            const onlineNodes = nodes.filter(n => n.online);
            for (const node of onlineNodes) {
                addLogEntry(node.hostname, 'Queued...', 'running');
            }

            for (const node of onlineNodes) {
                updateLogEntry(node.hostname, 'Updating...', 'running');
                const card = document.getElementById('card-' + node.hostname);
                if (card) card.classList.add('updating');

                try {
                    const res = await fetch('/api/v1/nodes/' + node.hostname + '/update', { method: 'POST' });
                    const data = await res.json();
                    updateLogEntry(node.hostname, data.success ? 'Completed' : 'Failed', data.success ? 'success' : 'error');
                } catch (err) {
                    updateLogEntry(node.hostname, 'Error: ' + err.message, 'error');
                }

                if (card) card.classList.remove('updating');
            }

            setLogStatus('completed');
            showToast('All node updates completed', 'success');
        }

        function confirmRebootWorkers() {
            const workerCount = nodes.filter(n => !n.is_control_plane && !n.is_nas && n.online).length;
            document.getElementById('modalTitle').innerHTML = '&#9888; Reboot Workers';
            document.getElementById('modalMessage').innerHTML = '<p class="warning">This will reboot <strong>' + workerCount + '</strong> worker nodes!</p>Control Plane and NAS will NOT be affected.<br><br>The cluster will experience significant service disruption during the reboot. Are you sure?';
            document.getElementById('modalConfirm').onclick = rebootWorkers;
            document.getElementById('modalConfirm').className = 'modal-btn confirm';
            document.getElementById('confirmModal').classList.add('open');
        }

        async function rebootWorkers() {
            closeModal();
            showLog('Rebooting Workers', 'running');

            try {
                const res = await fetch('/api/v1/reboot-workers', { method: 'POST' });
                const data = await res.json();

                if (data.success && data.data.results) {
                    data.data.results.forEach(r => {
                        addLogEntry(r.hostname, r.success ? 'Reboot initiated' : ('Failed: ' + r.error), r.success ? 'success' : 'error');
                    });
                    showToast('Worker reboot initiated', 'success');
                }
            } catch (err) {
                addLogEntry('System', 'Error: ' + err.message, 'error');
                showToast('Error rebooting workers', 'error');
            }

            setLogStatus('completed');
            setTimeout(fetchNodes, 20000);
        }

        function openTerminal(hostname) {
            const node = nodes.find(n => n.hostname === hostname);
            if (node) {
                document.getElementById('terminalNodeSelect').value = node.ip;
                document.querySelector('[data-tab="terminal"]').click();
                connectTerminal();
            }
        }

        function connectTerminal() {
            const select = document.getElementById('terminalNodeSelect');
            const ip = select.value;
            const frame = document.getElementById('terminalFrame');

            if (!ip) {
                frame.innerHTML = '<div class="terminal-placeholder"><div class="icon">&#128421;</div><div>Select a node above to open SSH terminal</div></div>';
                return;
            }

            const hostname = select.options[select.selectedIndex].dataset.hostname || ip;
            showToast('Connecting to ' + hostname + '...', 'info');

            // Connect to holmos-shell terminal service
            frame.innerHTML = '<iframe src="http://192.168.8.197:30088/terminal?host=' + ip + '" allowfullscreen></iframe>';
        }

        function loadHealthData() {
            const grid = document.getElementById('healthGrid');

            if (!nodes.length) {
                grid.innerHTML = '<div class="loading"><div class="loading-spinner"></div>Loading health data...</div>';
                return;
            }

            const online = nodes.filter(n => n.online).length;
            const total = nodes.length;
            const pct = Math.round((online / total) * 100);
            const cpOnline = nodes.find(n => n.is_control_plane && n.online);
            const nasOnline = nodes.find(n => n.is_nas && n.online);
            const workersOnline = nodes.filter(n => !n.is_control_plane && !n.is_nas && n.online).length;
            const workersTotal = nodes.filter(n => !n.is_control_plane && !n.is_nas).length;
            const avgLatency = nodes.filter(n => n.online && n.latency_ms).reduce((a, n) => a + n.latency_ms, 0) / (online || 1);

            const barClass = pct >= 90 ? 'good' : pct >= 70 ? 'warning' : 'critical';

            grid.innerHTML =
                '<div class="health-card">' +
                    '<div class="health-card-header"><span class="health-card-icon">&#127760;</span><span class="health-card-title">Cluster Health</span></div>' +
                    '<div class="health-card-value" style="color:' + (pct >= 90 ? '#68d391' : pct >= 70 ? '#f6e05e' : '#fc8181') + '">' + pct + '%</div>' +
                    '<div class="health-card-label">' + online + ' of ' + total + ' nodes online</div>' +
                    '<div class="health-bar"><div class="health-bar-fill ' + barClass + '" style="width:' + pct + '%"></div></div>' +
                '</div>' +
                '<div class="health-card">' +
                    '<div class="health-card-header"><span class="health-card-icon">&#128187;</span><span class="health-card-title">Control Plane</span></div>' +
                    '<div class="health-card-value" style="color:' + (cpOnline ? '#68d391' : '#fc8181') + '">' + (cpOnline ? 'Online' : 'Offline') + '</div>' +
                    '<div class="health-card-label">rpi-1 (192.168.8.197)</div>' +
                '</div>' +
                '<div class="health-card">' +
                    '<div class="health-card-header"><span class="health-card-icon">&#128421;</span><span class="health-card-title">Worker Nodes</span></div>' +
                    '<div class="health-card-value">' + workersOnline + '/' + workersTotal + '</div>' +
                    '<div class="health-card-label">Workers online</div>' +
                    '<div class="health-bar"><div class="health-bar-fill ' + (workersOnline === workersTotal ? 'good' : workersOnline > 0 ? 'warning' : 'critical') + '" style="width:' + ((workersOnline/workersTotal)*100) + '%"></div></div>' +
                '</div>' +
                '<div class="health-card">' +
                    '<div class="health-card-header"><span class="health-card-icon">&#128190;</span><span class="health-card-title">NAS Storage</span></div>' +
                    '<div class="health-card-value" style="color:' + (nasOnline ? '#68d391' : '#fc8181') + '">' + (nasOnline ? 'Online' : 'Offline') + '</div>' +
                    '<div class="health-card-label">openmediavault (192.168.8.199)</div>' +
                '</div>' +
                '<div class="health-card">' +
                    '<div class="health-card-header"><span class="health-card-icon">&#128225;</span><span class="health-card-title">Network Latency</span></div>' +
                    '<div class="health-card-value" style="color:' + (avgLatency < 50 ? '#68d391' : avgLatency < 100 ? '#f6e05e' : '#fc8181') + '">' + Math.round(avgLatency) + 'ms</div>' +
                    '<div class="health-card-label">Average ping time</div>' +
                '</div>' +
                '<div class="health-card">' +
                    '<div class="health-card-header"><span class="health-card-icon">&#128202;</span><span class="health-card-title">Node Status</span></div>' +
                    '<div class="node-list-compact">' +
                        nodes.map(n =>
                            '<div class="node-list-item">' +
                                '<div class="node-list-item-info">' +
                                    '<div class="node-list-item-status ' + (n.online ? 'online' : 'offline') + '"></div>' +
                                    '<span class="node-list-item-name">' + n.hostname + '</span>' +
                                '</div>' +
                                '<span class="node-list-item-latency">' + (n.latency_ms ? n.latency_ms + 'ms' : '--') + '</span>' +
                            '</div>'
                        ).join('') +
                    '</div>' +
                '</div>';
        }

        // Log panel functions
        function showLog(title, status) {
            document.getElementById('logTitle').textContent = title;
            document.getElementById('logContent').innerHTML = '';
            setLogStatus(status);
            document.getElementById('logPanel').classList.add('active');
        }

        function setLogStatus(status) {
            const el = document.getElementById('logStatus');
            el.textContent = status.charAt(0).toUpperCase() + status.slice(1);
            el.className = 'log-status ' + status;
        }

        function addLogEntry(hostname, message, status) {
            const content = document.getElementById('logContent');
            const time = new Date().toLocaleTimeString();
            content.innerHTML += '<div class="log-entry ' + status + '" id="log-' + hostname + '">' +
                '<div class="log-entry-header">' +
                    '<span class="log-entry-host">' + hostname + '</span>' +
                    '<span class="log-entry-status">' + time + '</span>' +
                '</div>' +
                '<div>' + message + '</div>' +
            '</div>';
            content.scrollTop = content.scrollHeight;
        }

        function updateLogEntry(hostname, message, status) {
            const entry = document.getElementById('log-' + hostname);
            if (entry) {
                entry.className = 'log-entry ' + status;
                entry.querySelector('div:last-child').textContent = message;
            }
        }

        function closeLogPanel() {
            document.getElementById('logPanel').classList.remove('active');
        }

        function closeModal() {
            document.getElementById('confirmModal').classList.remove('open');
        }

        async function refreshNodes() {
            const grid = document.getElementById('nodesGrid');
            grid.innerHTML = '<div class="loading"><div class="loading-spinner"></div>Refreshing nodes...</div>';
            showToast('Refreshing node status...', 'info');

            const TIMEOUT_MS = 10000;
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);
            const startTime = Date.now();

            try {
                const res = await fetch('/api/v1/nodes/refresh', {
                    method: 'POST',
                    signal: controller.signal
                });
                clearTimeout(timeoutId);

                if (!res.ok) {
                    throw new Error('HTTP ' + res.status + ': ' + res.statusText);
                }

                await fetchNodes();
            } catch (err) {
                clearTimeout(timeoutId);
                const elapsed = Date.now() - startTime;

                let errorMsg = err.message;
                let errorType = 'Error';
                if (err.name === 'AbortError') {
                    errorMsg = 'Refresh timed out after ' + (TIMEOUT_MS / 1000) + ' seconds';
                    errorType = 'Timeout';
                } else if (err.message.includes('Failed to fetch') || err.message.includes('NetworkError')) {
                    errorMsg = 'Network error - backend may be unreachable';
                    errorType = 'Network Error';
                }

                // Show error state
                const timestamp = new Date().toISOString();
                grid.innerHTML =
                    '<div class="error-state" style="padding:40px;text-align:center;background:#2d1f1f;border-radius:12px;border:1px solid #fc8181;max-width:500px;margin:0 auto;">' +
                        '<div style="font-size:48px;margin-bottom:16px;">&#9888;</div>' +
                        '<div style="font-size:20px;font-weight:600;color:#fc8181;margin-bottom:8px;">' + errorType + '</div>' +
                        '<div style="font-size:16px;color:#e2e8f0;margin-bottom:20px;">' + errorMsg + '</div>' +
                        '<div style="background:#1a1a2e;border-radius:8px;padding:16px;margin-bottom:20px;text-align:left;font-family:monospace;font-size:12px;color:#a0aec0;">' +
                            '<div style="margin-bottom:8px;"><span style="color:#718096;">Endpoint:</span> ' + window.location.origin + '/api/v1/nodes/refresh</div>' +
                            '<div style="margin-bottom:8px;"><span style="color:#718096;">Timestamp:</span> ' + timestamp + '</div>' +
                            '<div><span style="color:#718096;">Elapsed:</span> ' + elapsed + 'ms</div>' +
                        '</div>' +
                        '<button onclick="refreshNodes()" class="action-btn primary" style="padding:14px 28px;font-size:15px;">&#8635; Retry</button>' +
                    '</div>';

                showToast(errorType + ': ' + errorMsg, 'error');
            }
        }

        // Close modal on overlay click
        document.getElementById('confirmModal').addEventListener('click', function(e) {
            if (e.target === this) closeModal();
        });

        // Initialize with loading timeout safety
        let initialLoadComplete = false;

        // Safety timeout: if loading takes more than 10s, show error
        const loadingTimeout = setTimeout(() => {
            if (!initialLoadComplete) {
                const grid = document.getElementById('nodesGrid');
                if (grid && grid.querySelector('.loading')) {
                    const timestamp = new Date().toISOString();
                    grid.innerHTML =
                        '<div class="error-state" style="padding:40px;text-align:center;background:#2d1f1f;border-radius:12px;border:1px solid #fc8181;max-width:500px;margin:0 auto;">' +
                            '<div style="font-size:48px;margin-bottom:16px;">&#9888;</div>' +
                            '<div style="font-size:20px;font-weight:600;color:#fc8181;margin-bottom:8px;">Loading Timeout</div>' +
                            '<div style="font-size:16px;color:#e2e8f0;margin-bottom:20px;">Initial load took too long (>10 seconds)</div>' +
                            '<div style="background:#1a1a2e;border-radius:8px;padding:16px;margin-bottom:20px;text-align:left;font-family:monospace;font-size:12px;color:#a0aec0;">' +
                                '<div style="margin-bottom:8px;"><span style="color:#718096;">Endpoint:</span> ' + window.location.origin + '/api/v1/nodes</div>' +
                                '<div><span style="color:#718096;">Timestamp:</span> ' + timestamp + '</div>' +
                            '</div>' +
                            '<button onclick="fetchNodes()" class="action-btn primary" style="padding:14px 28px;font-size:15px;">&#8635; Retry</button>' +
                        '</div>';
                    showToast('Loading timeout - click Retry to try again', 'error');
                }
            }
        }, 10000);

        // Wrap fetchNodes to track completion
        const originalFetchNodes = fetchNodes;
        fetchNodes = async function() {
            await originalFetchNodes();
            initialLoadComplete = true;
            clearTimeout(loadingTimeout);
        };

        fetchNodes();
        fetchHealth();
        setInterval(fetchNodes, 30000);
        setInterval(fetchHealth, 60000);
    </script>
</body>
</html>'''


if __name__ == "__main__":
    # Pre-populate cache
    threading.Thread(target=update_node_cache, daemon=True).start()

    from waitress import serve
    print("Cluster Manager running on port 8080")
    serve(app, host="0.0.0.0", port=8080, threads=4)
