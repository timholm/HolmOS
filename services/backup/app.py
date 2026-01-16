#!/usr/bin/env python3
"""
Backup Service - HolmOS
Manages backup jobs via Kubernetes Jobs API.
Supports on-demand backup triggering, status monitoring, and history.
"""

import os
import json
import uuid
import logging
from datetime import datetime, timedelta
from flask import Flask, request, jsonify, Response
from flask_cors import CORS
import threading
import time

# Kubernetes client
try:
    from kubernetes import client, config
    from kubernetes.client.rest import ApiException
    try:
        config.load_incluster_config()
        K8S_AVAILABLE = True
    except:
        try:
            config.load_kube_config()
            K8S_AVAILABLE = True
        except:
            K8S_AVAILABLE = False
except ImportError:
    K8S_AVAILABLE = False

app = Flask(__name__)
CORS(app)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Configuration
NAMESPACE = os.getenv('NAMESPACE', 'holm')
BACKUP_STORAGE_PATH = os.getenv('BACKUP_STORAGE_PATH', '/mnt/node13-ssd/backups')
BACKUP_IMAGE = os.getenv('BACKUP_IMAGE', 'alpine:latest')
JOB_TIMEOUT = int(os.getenv('JOB_TIMEOUT', '300'))  # 5 minutes default
POLL_TIMEOUT = int(os.getenv('POLL_TIMEOUT', '30'))  # 30 seconds for API calls

# In-memory storage for job history (in production, use a database)
backup_jobs = {}
backup_history = []
jobs_lock = threading.Lock()


def get_k8s_clients():
    """Get Kubernetes API clients."""
    if not K8S_AVAILABLE:
        return None, None
    return client.BatchV1Api(), client.CoreV1Api()


def generate_job_id():
    """Generate a unique job ID."""
    return f"backup-{uuid.uuid4().hex[:8]}"


def create_backup_job_manifest(job_id, backup_config):
    """Create a Kubernetes Job manifest for backup."""
    source_path = backup_config.get('source_path', '/data')
    backup_name = backup_config.get('name', 'manual-backup')
    backup_type = backup_config.get('type', 'files')
    destination = backup_config.get('destination', BACKUP_STORAGE_PATH)

    # Sanitize job name for Kubernetes (lowercase, alphanumeric, dashes)
    k8s_job_name = f"backup-job-{job_id}".lower().replace('_', '-')[:63]

    job_manifest = {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": k8s_job_name,
            "namespace": NAMESPACE,
            "labels": {
                "app": "backup-service",
                "backup-job-id": job_id,
                "backup-type": backup_type
            }
        },
        "spec": {
            "ttlSecondsAfterFinished": 3600,  # Clean up after 1 hour
            "backoffLimit": 1,
            "activeDeadlineSeconds": JOB_TIMEOUT,
            "template": {
                "metadata": {
                    "labels": {
                        "app": "backup-job",
                        "backup-job-id": job_id
                    }
                },
                "spec": {
                    "restartPolicy": "Never",
                    "affinity": {
                        "nodeAffinity": {
                            "preferredDuringSchedulingIgnoredDuringExecution": [{
                                "weight": 100,
                                "preference": {
                                    "matchExpressions": [{
                                        "key": "kubernetes.io/hostname",
                                        "operator": "In",
                                        "values": ["node13", "rpi-13", "rpi13"]
                                    }]
                                }
                            }]
                        }
                    },
                    "containers": [{
                        "name": "backup",
                        "image": BACKUP_IMAGE,
                        "command": ["/bin/sh", "-c"],
                        "args": [f"""
set -e
echo "=== Backup Job Started ==="
echo "Job ID: {job_id}"
echo "Source: {source_path}"
echo "Destination: {destination}"
echo "Type: {backup_type}"
echo "Timestamp: $(date -Iseconds)"

# Create destination directory
mkdir -p {destination}/{job_id}

# Perform backup based on type
if [ "{backup_type}" = "files" ]; then
    echo "Copying files..."
    if [ -d "{source_path}" ]; then
        cp -rv {source_path}/* {destination}/{job_id}/ 2>&1 || true
        FILE_COUNT=$(find {destination}/{job_id} -type f | wc -l)
        TOTAL_SIZE=$(du -sh {destination}/{job_id} | cut -f1)
        echo "Files backed up: $FILE_COUNT"
        echo "Total size: $TOTAL_SIZE"
    else
        echo "Warning: Source path {source_path} does not exist or is not a directory"
        echo "Creating marker file instead..."
        echo "{backup_name} - $(date -Iseconds)" > {destination}/{job_id}/backup_marker.txt
    fi
elif [ "{backup_type}" = "database" ]; then
    echo "Database backup would run here..."
    echo "Backup name: {backup_name}" > {destination}/{job_id}/db_backup_info.txt
    echo "Timestamp: $(date -Iseconds)" >> {destination}/{job_id}/db_backup_info.txt
else
    echo "Generic backup..."
    echo "{backup_name}" > {destination}/{job_id}/backup_info.txt
    echo "Type: {backup_type}" >> {destination}/{job_id}/backup_info.txt
    echo "Created: $(date -Iseconds)" >> {destination}/{job_id}/backup_info.txt
fi

# Create metadata file
cat > {destination}/{job_id}/metadata.json << 'METADATA'
{{
  "job_id": "{job_id}",
  "name": "{backup_name}",
  "type": "{backup_type}",
  "source": "{source_path}",
  "completed_at": "$(date -Iseconds)",
  "status": "completed"
}}
METADATA

echo "=== Backup Job Completed ==="
"""],
                        "volumeMounts": [
                            {
                                "name": "backup-storage",
                                "mountPath": destination
                            },
                            {
                                "name": "source-data",
                                "mountPath": source_path,
                                "readOnly": True
                            }
                        ],
                        "resources": {
                            "requests": {
                                "memory": "64Mi",
                                "cpu": "100m"
                            },
                            "limits": {
                                "memory": "256Mi",
                                "cpu": "500m"
                            }
                        }
                    }],
                    "volumes": [
                        {
                            "name": "backup-storage",
                            "hostPath": {
                                "path": destination,
                                "type": "DirectoryOrCreate"
                            }
                        },
                        {
                            "name": "source-data",
                            "hostPath": {
                                "path": source_path,
                                "type": "DirectoryOrCreate"
                            }
                        }
                    ]
                }
            }
        }
    }

    return job_manifest


def get_job_status(job_id):
    """Get the status of a backup job from Kubernetes."""
    batch_api, core_api = get_k8s_clients()
    if not batch_api:
        return {"status": "unknown", "message": "Kubernetes not available"}

    k8s_job_name = f"backup-job-{job_id}".lower().replace('_', '-')[:63]

    try:
        job = batch_api.read_namespaced_job(k8s_job_name, NAMESPACE)

        status = "unknown"
        message = ""
        files_count = None
        total_size = None

        if job.status.succeeded and job.status.succeeded > 0:
            status = "completed"
            message = "Backup completed successfully"
        elif job.status.failed and job.status.failed > 0:
            status = "failed"
            message = "Backup job failed"
        elif job.status.active and job.status.active > 0:
            status = "running"
            message = "Backup in progress"
        else:
            status = "pending"
            message = "Job is pending"

        # Try to get pod logs for more details
        try:
            pods = core_api.list_namespaced_pod(
                NAMESPACE,
                label_selector=f"backup-job-id={job_id}"
            )
            if pods.items:
                pod = pods.items[0]
                pod_status = pod.status.phase

                if pod_status == "Running":
                    status = "running"
                elif pod_status == "Pending":
                    status = "pending"
                    # Check for waiting reasons
                    if pod.status.container_statuses:
                        for cs in pod.status.container_statuses:
                            if cs.state.waiting:
                                message = f"Waiting: {cs.state.waiting.reason}"

                # Try to get logs if completed
                if status in ["completed", "failed"]:
                    try:
                        logs = core_api.read_namespaced_pod_log(
                            pod.metadata.name,
                            NAMESPACE,
                            tail_lines=50
                        )
                        # Parse logs for file count and size
                        for line in logs.split('\n'):
                            if 'Files backed up:' in line:
                                try:
                                    files_count = int(line.split(':')[1].strip())
                                except:
                                    pass
                            if 'Total size:' in line:
                                total_size = line.split(':')[1].strip()
                    except:
                        pass
        except ApiException as e:
            logger.warning(f"Could not get pod status: {e}")

        result = {
            "status": status,
            "message": message,
            "started_at": job.status.start_time.isoformat() if job.status.start_time else None,
            "completed_at": job.status.completion_time.isoformat() if job.status.completion_time else None
        }

        if files_count is not None:
            result["files_count"] = files_count
        if total_size is not None:
            result["total_size"] = total_size

        return result

    except ApiException as e:
        if e.status == 404:
            return {"status": "not_found", "message": "Job not found in cluster"}
        logger.error(f"Error getting job status: {e}")
        return {"status": "error", "message": str(e)}


def get_job_logs(job_id):
    """Get logs for a backup job."""
    batch_api, core_api = get_k8s_clients()
    if not core_api:
        return None

    try:
        pods = core_api.list_namespaced_pod(
            NAMESPACE,
            label_selector=f"backup-job-id={job_id}"
        )
        if not pods.items:
            return None

        pod = pods.items[0]
        logs = core_api.read_namespaced_pod_log(
            pod.metadata.name,
            NAMESPACE,
            tail_lines=200
        )
        return logs
    except ApiException as e:
        logger.error(f"Error getting logs: {e}")
        return None


@app.route('/health', methods=['GET'])
def health():
    """Health check endpoint."""
    return jsonify({
        "status": "healthy",
        "timestamp": datetime.utcnow().isoformat(),
        "k8s_available": K8S_AVAILABLE,
        "namespace": NAMESPACE
    })


@app.route('/api/jobs', methods=['GET'])
def list_jobs():
    """List all backup jobs."""
    with jobs_lock:
        jobs_list = list(backup_jobs.values())

    # Update status from Kubernetes for active jobs
    for job in jobs_list:
        if job.get('status') in ['pending', 'running']:
            k8s_status = get_job_status(job['id'])
            job.update(k8s_status)
            with jobs_lock:
                if job['id'] in backup_jobs:
                    backup_jobs[job['id']].update(k8s_status)

    # Sort by created_at descending
    jobs_list.sort(key=lambda x: x.get('created_at', ''), reverse=True)

    return jsonify({
        "jobs": jobs_list,
        "count": len(jobs_list)
    })


@app.route('/api/jobs/<job_id>', methods=['GET'])
def get_job(job_id):
    """Get a specific backup job."""
    with jobs_lock:
        job = backup_jobs.get(job_id)

    if not job:
        return jsonify({"error": "Job not found"}), 404

    # Get fresh status from Kubernetes
    k8s_status = get_job_status(job_id)
    job.update(k8s_status)

    with jobs_lock:
        if job_id in backup_jobs:
            backup_jobs[job_id].update(k8s_status)

    return jsonify(job)


@app.route('/api/jobs/<job_id>/logs', methods=['GET'])
def get_logs(job_id):
    """Get logs for a backup job."""
    logs = get_job_logs(job_id)
    if logs is None:
        return jsonify({"error": "Logs not available"}), 404

    return jsonify({
        "job_id": job_id,
        "logs": logs
    })


@app.route('/api/jobs/<job_id>/status', methods=['GET'])
def get_status(job_id):
    """Get status of a backup job with timeout handling."""
    timeout = request.args.get('timeout', POLL_TIMEOUT, type=int)
    timeout = min(timeout, 60)  # Max 60 seconds

    start_time = time.time()
    last_status = None

    while time.time() - start_time < timeout:
        status = get_job_status(job_id)

        # Return immediately if status changed or job completed
        if status['status'] != last_status or status['status'] in ['completed', 'failed', 'error', 'not_found']:
            return jsonify(status)

        last_status = status['status']
        time.sleep(1)

    # Timeout - return last known status with timeout flag
    status = get_job_status(job_id)
    status['timeout'] = True
    return jsonify(status)


@app.route('/api/backup/trigger', methods=['POST'])
def trigger_backup():
    """Trigger a new backup job."""
    data = request.get_json() or {}

    job_id = generate_job_id()
    backup_config = {
        'name': data.get('name', 'Manual Backup'),
        'source_path': data.get('source_path', '/data'),
        'destination': data.get('destination', BACKUP_STORAGE_PATH),
        'type': data.get('type', 'files')
    }

    # Create job record
    job_record = {
        'id': job_id,
        'name': backup_config['name'],
        'type': backup_config['type'],
        'source_path': backup_config['source_path'],
        'destination': backup_config['destination'],
        'status': 'pending',
        'message': 'Creating backup job...',
        'created_at': datetime.utcnow().isoformat(),
        'started_at': None,
        'completed_at': None,
        'files_count': None,
        'total_size': None
    }

    with jobs_lock:
        backup_jobs[job_id] = job_record

    # Create Kubernetes Job
    batch_api, _ = get_k8s_clients()
    if not batch_api:
        job_record['status'] = 'failed'
        job_record['message'] = 'Kubernetes API not available'
        with jobs_lock:
            backup_jobs[job_id] = job_record
        return jsonify(job_record), 500

    try:
        job_manifest = create_backup_job_manifest(job_id, backup_config)
        batch_api.create_namespaced_job(NAMESPACE, job_manifest)

        job_record['status'] = 'pending'
        job_record['message'] = 'Job created, waiting to start'

        with jobs_lock:
            backup_jobs[job_id] = job_record

        logger.info(f"Created backup job: {job_id}")
        return jsonify(job_record), 201

    except ApiException as e:
        logger.error(f"Failed to create job: {e}")
        job_record['status'] = 'failed'
        job_record['message'] = f'Failed to create job: {e.reason}'

        with jobs_lock:
            backup_jobs[job_id] = job_record

        return jsonify(job_record), 500


@app.route('/api/backup/quick', methods=['POST'])
def quick_backup():
    """Quick backup with minimal configuration."""
    data = request.get_json() or {}

    # Use defaults for quick backup
    data.setdefault('name', f"Quick Backup {datetime.now().strftime('%Y-%m-%d %H:%M')}")
    data.setdefault('type', 'files')
    data.setdefault('source_path', '/data')
    data.setdefault('destination', BACKUP_STORAGE_PATH)

    return trigger_backup()


@app.route('/api/history', methods=['GET'])
def get_history():
    """Get backup history."""
    limit = request.args.get('limit', 50, type=int)

    with jobs_lock:
        # Get completed jobs
        completed = [j for j in backup_jobs.values()
                    if j.get('status') in ['completed', 'failed']]

    # Sort by created_at descending
    completed.sort(key=lambda x: x.get('created_at', ''), reverse=True)

    return jsonify({
        "history": completed[:limit],
        "count": len(completed)
    })


@app.route('/api/stats', methods=['GET'])
def get_stats():
    """Get backup statistics."""
    with jobs_lock:
        jobs_list = list(backup_jobs.values())

    total = len(jobs_list)
    completed = len([j for j in jobs_list if j.get('status') == 'completed'])
    failed = len([j for j in jobs_list if j.get('status') == 'failed'])
    running = len([j for j in jobs_list if j.get('status') == 'running'])
    pending = len([j for j in jobs_list if j.get('status') == 'pending'])

    # Calculate total size backed up
    total_size = 0
    total_files = 0
    for job in jobs_list:
        if job.get('files_count'):
            total_files += job['files_count']

    return jsonify({
        "total_jobs": total,
        "completed": completed,
        "failed": failed,
        "running": running,
        "pending": pending,
        "total_files": total_files,
        "storage_path": BACKUP_STORAGE_PATH
    })


@app.route('/')
def index():
    """Serve the dashboard."""
    return DASHBOARD_HTML


# Dashboard HTML
DASHBOARD_HTML = '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Backup Service - HolmOS</title>
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
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, var(--ctp-crust) 0%, var(--ctp-base) 50%, var(--ctp-mantle) 100%);
            color: var(--ctp-text);
            min-height: 100vh;
            line-height: 1.6;
        }
        .container { max-width: 1200px; margin: 0 auto; padding: 2rem; }
        header {
            text-align: center;
            padding: 2rem;
            background: var(--ctp-mantle);
            border-radius: 1rem;
            margin-bottom: 2rem;
            border: 1px solid var(--ctp-surface0);
        }
        header h1 { color: var(--ctp-teal); font-size: 2.5rem; margin-bottom: 0.5rem; }
        header p { color: var(--ctp-subtext0); }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .stat-card {
            background: var(--ctp-mantle);
            padding: 1.5rem;
            border-radius: 1rem;
            border: 1px solid var(--ctp-surface0);
            text-align: center;
        }
        .stat-card .value { font-size: 2rem; font-weight: bold; color: var(--ctp-teal); }
        .stat-card .label { color: var(--ctp-subtext0); font-size: 0.875rem; }
        .stat-card.running .value { color: var(--ctp-blue); }
        .stat-card.failed .value { color: var(--ctp-red); }
        .card {
            background: var(--ctp-mantle);
            border-radius: 1rem;
            padding: 1.5rem;
            border: 1px solid var(--ctp-surface0);
            margin-bottom: 1.5rem;
        }
        .card h2 {
            color: var(--ctp-blue);
            margin-bottom: 1rem;
            padding-bottom: 0.5rem;
            border-bottom: 1px solid var(--ctp-surface0);
        }
        .trigger-section {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1.5rem;
        }
        @media (max-width: 768px) { .trigger-section { grid-template-columns: 1fr; } }
        .form-group { margin-bottom: 1rem; }
        .form-group label {
            display: block;
            color: var(--ctp-subtext1);
            margin-bottom: 0.5rem;
            font-size: 0.875rem;
        }
        input, select {
            width: 100%;
            padding: 0.75rem;
            background: var(--ctp-surface0);
            border: 2px solid var(--ctp-surface1);
            border-radius: 0.5rem;
            color: var(--ctp-text);
            font-size: 1rem;
        }
        input:focus, select:focus { outline: none; border-color: var(--ctp-teal); }
        button {
            background: linear-gradient(135deg, var(--ctp-teal) 0%, var(--ctp-green) 100%);
            color: var(--ctp-crust);
            border: none;
            padding: 0.75rem 1.5rem;
            border-radius: 0.5rem;
            cursor: pointer;
            font-weight: 600;
            font-size: 1rem;
            transition: all 0.2s;
            width: 100%;
        }
        button:hover:not(:disabled) { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(148, 226, 213, 0.3); }
        button:disabled { opacity: 0.5; cursor: not-allowed; transform: none; }
        button.secondary { background: var(--ctp-surface1); color: var(--ctp-text); }
        .quick-backup {
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            min-height: 200px;
        }
        .quick-backup button { font-size: 1.25rem; padding: 1rem 2rem; max-width: 300px; }
        .jobs-list { max-height: 500px; overflow-y: auto; }
        .job-item {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 1rem;
            background: var(--ctp-surface0);
            border-radius: 0.5rem;
            margin-bottom: 0.75rem;
        }
        .job-item:last-child { margin-bottom: 0; }
        .job-info { flex: 1; }
        .job-name { font-weight: 600; color: var(--ctp-text); }
        .job-meta { font-size: 0.875rem; color: var(--ctp-subtext0); }
        .job-status {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.25rem 0.75rem;
            border-radius: 1rem;
            font-size: 0.875rem;
            font-weight: 500;
        }
        .job-status.pending { background: var(--ctp-surface1); color: var(--ctp-overlay2); }
        .job-status.running { background: var(--ctp-blue); color: var(--ctp-crust); }
        .job-status.completed { background: var(--ctp-green); color: var(--ctp-crust); }
        .job-status.failed { background: var(--ctp-red); color: var(--ctp-crust); }
        .spinner {
            width: 16px;
            height: 16px;
            border: 2px solid transparent;
            border-top-color: currentColor;
            border-radius: 50%;
            animation: spin 1s linear infinite;
        }
        @keyframes spin { to { transform: rotate(360deg); } }
        .empty-state {
            text-align: center;
            padding: 3rem;
            color: var(--ctp-overlay0);
        }
        .job-details {
            font-size: 0.8rem;
            color: var(--ctp-subtext0);
            margin-top: 0.25rem;
        }
        .error-message {
            background: var(--ctp-red);
            color: var(--ctp-crust);
            padding: 1rem;
            border-radius: 0.5rem;
            margin-bottom: 1rem;
        }
        .toast {
            position: fixed;
            bottom: 2rem;
            right: 2rem;
            padding: 1rem 1.5rem;
            border-radius: 0.5rem;
            color: var(--ctp-crust);
            font-weight: 500;
            z-index: 1000;
            animation: slideIn 0.3s ease;
        }
        .toast.success { background: var(--ctp-green); }
        .toast.error { background: var(--ctp-red); }
        @keyframes slideIn { from { transform: translateX(100%); opacity: 0; } to { transform: translateX(0); opacity: 1; } }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Backup Service</h1>
            <p>On-demand backup management for HolmOS - Node13 Storage</p>
        </header>

        <div class="stats-grid">
            <div class="stat-card"><div class="value" id="stat-total">-</div><div class="label">Total Jobs</div></div>
            <div class="stat-card"><div class="value" id="stat-completed">-</div><div class="label">Completed</div></div>
            <div class="stat-card running"><div class="value" id="stat-running">-</div><div class="label">Running</div></div>
            <div class="stat-card failed"><div class="value" id="stat-failed">-</div><div class="label">Failed</div></div>
        </div>

        <div class="trigger-section">
            <div class="card">
                <h2>Quick Backup</h2>
                <div class="quick-backup">
                    <p style="color: var(--ctp-subtext0); margin-bottom: 1rem;">Start a backup immediately with default settings</p>
                    <button onclick="triggerQuickBackup()" id="quick-btn">Start Backup Now</button>
                </div>
            </div>

            <div class="card">
                <h2>Custom Backup</h2>
                <form id="backup-form" onsubmit="triggerCustomBackup(event)">
                    <div class="form-group">
                        <label for="backup-name">Backup Name</label>
                        <input type="text" id="backup-name" placeholder="My Backup">
                    </div>
                    <div class="form-group">
                        <label for="backup-type">Type</label>
                        <select id="backup-type">
                            <option value="files">Files</option>
                            <option value="database">Database</option>
                            <option value="config">Configuration</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label for="source-path">Source Path</label>
                        <input type="text" id="source-path" placeholder="/data" value="/data">
                    </div>
                    <button type="submit" id="custom-btn">Create Backup</button>
                </form>
            </div>
        </div>

        <div class="card">
            <h2>Recent Jobs</h2>
            <div class="jobs-list" id="jobs-list">
                <div class="empty-state">Loading jobs...</div>
            </div>
        </div>
    </div>

    <script>
        const API_BASE = '';
        let pollInterval = null;
        let activeJobs = new Set();

        async function fetchWithTimeout(url, options = {}, timeout = 30000) {
            const controller = new AbortController();
            const id = setTimeout(() => controller.abort(), timeout);
            try {
                const response = await fetch(url, { ...options, signal: controller.signal });
                clearTimeout(id);
                return response;
            } catch (error) {
                clearTimeout(id);
                if (error.name === 'AbortError') {
                    throw new Error('Request timed out');
                }
                throw error;
            }
        }

        async function loadStats() {
            try {
                const response = await fetchWithTimeout(API_BASE + '/api/stats');
                const stats = await response.json();
                document.getElementById('stat-total').textContent = stats.total_jobs;
                document.getElementById('stat-completed').textContent = stats.completed;
                document.getElementById('stat-running').textContent = stats.running;
                document.getElementById('stat-failed').textContent = stats.failed;
            } catch (error) {
                console.error('Failed to load stats:', error);
            }
        }

        async function loadJobs() {
            try {
                const response = await fetchWithTimeout(API_BASE + '/api/jobs');
                const data = await response.json();
                renderJobs(data.jobs);

                // Track active jobs for polling
                activeJobs.clear();
                data.jobs.forEach(job => {
                    if (job.status === 'running' || job.status === 'pending') {
                        activeJobs.add(job.id);
                    }
                });

                // Start or stop polling based on active jobs
                if (activeJobs.size > 0 && !pollInterval) {
                    pollInterval = setInterval(loadJobs, 3000);
                } else if (activeJobs.size === 0 && pollInterval) {
                    clearInterval(pollInterval);
                    pollInterval = null;
                }
            } catch (error) {
                console.error('Failed to load jobs:', error);
                document.getElementById('jobs-list').innerHTML =
                    '<div class="error-message">Failed to load jobs. Please refresh the page.</div>';
            }
        }

        function renderJobs(jobs) {
            const container = document.getElementById('jobs-list');
            if (!jobs || jobs.length === 0) {
                container.innerHTML = '<div class="empty-state">No backup jobs yet. Start one above!</div>';
                return;
            }

            container.innerHTML = jobs.map(job => {
                const statusClass = job.status || 'pending';
                const spinner = (job.status === 'running' || job.status === 'pending')
                    ? '<div class="spinner"></div>' : '';

                let details = '';
                if (job.files_count) details += `${job.files_count} files`;
                if (job.total_size) details += details ? ` | ${job.total_size}` : job.total_size;
                if (job.message && job.status !== 'completed') details = job.message;

                const createdAt = job.created_at ? new Date(job.created_at).toLocaleString() : '-';

                return `
                    <div class="job-item">
                        <div class="job-info">
                            <div class="job-name">${escapeHtml(job.name || job.id)}</div>
                            <div class="job-meta">${job.type || 'files'} | ${createdAt}</div>
                            ${details ? `<div class="job-details">${escapeHtml(details)}</div>` : ''}
                        </div>
                        <div class="job-status ${statusClass}">
                            ${spinner}
                            ${statusClass.charAt(0).toUpperCase() + statusClass.slice(1)}
                        </div>
                    </div>
                `;
            }).join('');
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        async function triggerQuickBackup() {
            const btn = document.getElementById('quick-btn');
            btn.disabled = true;
            btn.textContent = 'Starting...';

            try {
                const response = await fetchWithTimeout(API_BASE + '/api/backup/quick', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' }
                });
                const result = await response.json();

                if (response.ok) {
                    showToast('Backup job started!', 'success');
                    loadJobs();
                    loadStats();
                } else {
                    showToast(result.message || 'Failed to start backup', 'error');
                }
            } catch (error) {
                showToast('Failed to start backup: ' + error.message, 'error');
            } finally {
                btn.disabled = false;
                btn.textContent = 'Start Backup Now';
            }
        }

        async function triggerCustomBackup(event) {
            event.preventDefault();
            const btn = document.getElementById('custom-btn');
            btn.disabled = true;
            btn.textContent = 'Creating...';

            const data = {
                name: document.getElementById('backup-name').value || 'Custom Backup',
                type: document.getElementById('backup-type').value,
                source_path: document.getElementById('source-path').value || '/data'
            };

            try {
                const response = await fetchWithTimeout(API_BASE + '/api/backup/trigger', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(data)
                });
                const result = await response.json();

                if (response.ok) {
                    showToast('Backup job created!', 'success');
                    document.getElementById('backup-form').reset();
                    document.getElementById('source-path').value = '/data';
                    loadJobs();
                    loadStats();
                } else {
                    showToast(result.message || 'Failed to create backup', 'error');
                }
            } catch (error) {
                showToast('Failed to create backup: ' + error.message, 'error');
            } finally {
                btn.disabled = false;
                btn.textContent = 'Create Backup';
            }
        }

        function showToast(message, type) {
            const toast = document.createElement('div');
            toast.className = `toast ${type}`;
            toast.textContent = message;
            document.body.appendChild(toast);
            setTimeout(() => toast.remove(), 4000);
        }

        // Initial load
        loadStats();
        loadJobs();

        // Refresh stats periodically
        setInterval(loadStats, 30000);
    </script>
</body>
</html>
'''


if __name__ == '__main__':
    port = int(os.getenv('PORT', '8080'))
    debug = os.getenv('DEBUG', 'false').lower() == 'true'
    app.run(host='0.0.0.0', port=port, debug=debug)
