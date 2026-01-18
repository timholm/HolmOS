#!/usr/bin/env python3
"""
Deploy Controller - Automatic deployment after successful builds.
Watches for new images in registry and auto-deploys them.
"""

import asyncio
import os
import subprocess
import json
import yaml
import httpx
from datetime import datetime
from typing import Optional
from fastapi import FastAPI, HTTPException, BackgroundTasks
from fastapi.responses import HTMLResponse
from pydantic import BaseModel
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="Deploy Controller", version="1.0.0")

# Configuration
REGISTRY_URL = os.getenv("REGISTRY_URL", "http://registry.holm.svc.cluster.local:5000")
NAMESPACE = "holm"
CHECK_INTERVAL = 30  # seconds

# State tracking
deployments_state: dict = {}
deployment_history: list = []
auto_deploy_enabled = False  # Disabled by default - enable via API when needed
last_check_time: Optional[datetime] = None


class DeployRequest(BaseModel):
    image: str
    replicas: int = 1


class DeploymentInfo(BaseModel):
    name: str
    image: str
    replicas: int
    ready: int
    status: str
    last_updated: str


def generate_deployment_yaml(service_name: str, replicas: int = 1) -> str:
    """Generate Kubernetes deployment YAML for a service."""
    deployment = {
        "apiVersion": "apps/v1",
        "kind": "Deployment",
        "metadata": {
            "name": service_name,
            "namespace": NAMESPACE,
            "labels": {
                "app": service_name,
                "managed-by": "deploy-controller"
            }
        },
        "spec": {
            "replicas": replicas,
            "selector": {
                "matchLabels": {
                    "app": service_name
                }
            },
            "template": {
                "metadata": {
                    "labels": {
                        "app": service_name
                    }
                },
                "spec": {
                    "affinity": {
                        "nodeAffinity": {
                            "requiredDuringSchedulingIgnoredDuringExecution": {
                                "nodeSelectorTerms": [{
                                    "matchExpressions": [{
                                        "key": "kubernetes.io/hostname",
                                        "operator": "NotIn",
                                        "values": ["openmediavault"]
                                    }]
                                }]
                            }
                        }
                    },
                    "containers": [{
                        "name": service_name,
                        "image": f"localhost:31500/{service_name}:latest",
                        "ports": [{
                            "containerPort": 8080
                        }],
                        "imagePullPolicy": "Always"
                    }]
                }
            }
        }
    }
    return yaml.dump(deployment, default_flow_style=False)


def run_kubectl(args: list, input_data: str = None) -> tuple[bool, str]:
    """Run a kubectl command and return success status and output."""
    cmd = ["kubectl"] + args
    try:
        result = subprocess.run(
            cmd,
            input=input_data,
            capture_output=True,
            text=True,
            timeout=60
        )
        if result.returncode == 0:
            return True, result.stdout
        return False, result.stderr
    except subprocess.TimeoutExpired:
        return False, "Command timed out"
    except Exception as e:
        return False, str(e)


async def get_registry_images() -> list[str]:
    """Get list of images from the registry."""
    try:
        async with httpx.AsyncClient(timeout=10) as client:
            response = await client.get(f"{REGISTRY_URL}/v2/_catalog")
            if response.status_code == 200:
                data = response.json()
                repos = data.get("repositories", [])
                # Return all images (no holm/ prefix filter needed)
                return repos
            return []
    except Exception as e:
        logger.error(f"Failed to get registry images: {e}")
        return []


async def get_image_tags(image: str) -> list[str]:
    """Get tags for an image from the registry."""
    try:
        async with httpx.AsyncClient(timeout=10) as client:
            response = await client.get(f"{REGISTRY_URL}/v2/{image}/tags/list")
            if response.status_code == 200:
                data = response.json()
                return data.get("tags", [])
            return []
    except Exception as e:
        logger.error(f"Failed to get tags for {image}: {e}")
        return []


async def get_image_digest(image: str, tag: str = "latest") -> Optional[str]:
    """Get the digest of an image tag."""
    try:
        async with httpx.AsyncClient(timeout=10) as client:
            response = await client.head(
                f"{REGISTRY_URL}/v2/{image}/manifests/{tag}",
                headers={"Accept": "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"}
            )
            if response.status_code == 200:
                return response.headers.get("Docker-Content-Digest")
            return None
    except Exception as e:
        logger.error(f"Failed to get digest for {image}:{tag}: {e}")
        return None


def get_current_deployments() -> dict:
    """Get current deployments in the holm namespace."""
    success, output = run_kubectl([
        "get", "deployments", "-n", NAMESPACE,
        "-o", "json"
    ])
    if not success:
        logger.error(f"Failed to get deployments: {output}")
        return {}

    try:
        data = json.loads(output)
        deployments = {}
        for item in data.get("items", []):
            name = item["metadata"]["name"]
            containers = item["spec"]["template"]["spec"]["containers"]
            if containers:
                image = containers[0].get("image", "")
                replicas = item["spec"].get("replicas", 1)
                ready = item.get("status", {}).get("readyReplicas", 0)
                deployments[name] = {
                    "image": image,
                    "replicas": replicas,
                    "ready": ready or 0,
                    "labels": item["metadata"].get("labels", {})
                }
        return deployments
    except json.JSONDecodeError as e:
        logger.error(f"Failed to parse deployments: {e}")
        return {}


def deploy_service(service_name: str, replicas: int = 1) -> tuple[bool, str]:
    """Deploy a service to the cluster."""
    yaml_content = generate_deployment_yaml(service_name, replicas)
    success, output = run_kubectl(
        ["apply", "-f", "-"],
        input_data=yaml_content
    )

    if success:
        deployment_history.append({
            "action": "deploy",
            "service": service_name,
            "timestamp": datetime.now().isoformat(),
            "success": True,
            "message": output.strip()
        })
        logger.info(f"Deployed {service_name}: {output.strip()}")
    else:
        deployment_history.append({
            "action": "deploy",
            "service": service_name,
            "timestamp": datetime.now().isoformat(),
            "success": False,
            "message": output.strip()
        })
        logger.error(f"Failed to deploy {service_name}: {output}")

    return success, output


def rollback_deployment(name: str) -> tuple[bool, str]:
    """Rollback a deployment to previous version."""
    success, output = run_kubectl([
        "rollout", "undo", "deployment", name, "-n", NAMESPACE
    ])

    deployment_history.append({
        "action": "rollback",
        "service": name,
        "timestamp": datetime.now().isoformat(),
        "success": success,
        "message": output.strip()
    })

    return success, output


def restart_deployment(name: str) -> tuple[bool, str]:
    """Restart a deployment to pull latest image."""
    success, output = run_kubectl([
        "rollout", "restart", "deployment", name, "-n", NAMESPACE
    ])

    if success:
        deployment_history.append({
            "action": "restart",
            "service": name,
            "timestamp": datetime.now().isoformat(),
            "success": True,
            "message": output.strip()
        })
        logger.info(f"Restarted {name}: {output.strip()}")

    return success, output


async def check_deployment_health(name: str, timeout: int = 120) -> bool:
    """Wait for deployment to be healthy."""
    success, output = run_kubectl([
        "rollout", "status", "deployment", name,
        "-n", NAMESPACE, "--timeout", f"{timeout}s"
    ])
    return success


async def auto_deploy_loop():
    """Main auto-deploy loop that checks for new images."""
    global last_check_time, deployments_state

    while True:
        if auto_deploy_enabled:
            try:
                last_check_time = datetime.now()
                logger.info("Checking for new images...")

                # Get images from registry
                registry_images = await get_registry_images()
                logger.info(f"Found {len(registry_images)} images in registry")

                # Get current deployments
                current_deployments = get_current_deployments()
                deployments_state = current_deployments

                # Check each image
                for image in registry_images:
                    # Image name is the service name (no holm/ prefix)
                    service_name = image

                    # Get image digest
                    current_digest = await get_image_digest(image, "latest")

                    if service_name not in current_deployments:
                        # New service - deploy it
                        logger.info(f"New service detected: {service_name}")
                        success, msg = deploy_service(service_name)
                        if success:
                            # Wait for health check
                            healthy = await check_deployment_health(service_name)
                            if not healthy:
                                logger.warning(f"Deployment {service_name} not healthy, rolling back")
                                rollback_deployment(service_name)
                    else:
                        # Check if image has changed (using digest stored in state)
                        stored_digest = deployments_state.get(service_name, {}).get("digest")
                        if current_digest and stored_digest and current_digest != stored_digest:
                            logger.info(f"Image updated for {service_name}, restarting deployment")
                            success, msg = restart_deployment(service_name)
                            if success:
                                healthy = await check_deployment_health(service_name)
                                if not healthy:
                                    logger.warning(f"Deployment {service_name} not healthy after update, rolling back")
                                    rollback_deployment(service_name)

                        # Update stored digest
                        if service_name in deployments_state:
                            deployments_state[service_name]["digest"] = current_digest

            except Exception as e:
                logger.error(f"Error in auto-deploy loop: {e}")

        await asyncio.sleep(CHECK_INTERVAL)


@app.on_event("startup")
async def startup_event():
    """Start the auto-deploy background task."""
    asyncio.create_task(auto_deploy_loop())


@app.get("/", response_class=HTMLResponse)
async def dashboard():
    """Deployment dashboard."""
    current_deployments = get_current_deployments()
    registry_images = await get_registry_images()

    # Calculate pending deployments
    deployed_services = set(current_deployments.keys())
    registry_services = set(registry_images)
    pending = registry_services - deployed_services

    html = """
    <!DOCTYPE html>
    <html>
    <head>
        <title>Deploy Controller</title>
        <style>
            body {
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                margin: 0;
                padding: 20px;
                background: #0a0a0a;
                color: #e0e0e0;
            }
            h1, h2 { color: #4fc3f7; margin-bottom: 20px; }
            .container { max-width: 1200px; margin: 0 auto; }
            .card {
                background: #1a1a2e;
                border-radius: 8px;
                padding: 20px;
                margin-bottom: 20px;
                border: 1px solid #333;
            }
            .status-badge {
                display: inline-block;
                padding: 4px 12px;
                border-radius: 20px;
                font-size: 12px;
                font-weight: 500;
            }
            .status-running { background: #1b5e20; color: #a5d6a7; }
            .status-pending { background: #e65100; color: #ffcc80; }
            .status-error { background: #b71c1c; color: #ef9a9a; }
            table {
                width: 100%;
                border-collapse: collapse;
            }
            th, td {
                text-align: left;
                padding: 12px;
                border-bottom: 1px solid #333;
            }
            th { color: #4fc3f7; font-weight: 500; }
            .btn {
                padding: 8px 16px;
                border-radius: 4px;
                border: none;
                cursor: pointer;
                font-size: 14px;
                margin-right: 8px;
            }
            .btn-primary { background: #4fc3f7; color: #000; }
            .btn-danger { background: #ef5350; color: #fff; }
            .btn:hover { opacity: 0.8; }
            .stats {
                display: grid;
                grid-template-columns: repeat(4, 1fr);
                gap: 20px;
                margin-bottom: 20px;
            }
            .stat-card {
                background: #1a1a2e;
                border-radius: 8px;
                padding: 20px;
                text-align: center;
                border: 1px solid #333;
            }
            .stat-value { font-size: 32px; font-weight: bold; color: #4fc3f7; }
            .stat-label { color: #888; margin-top: 8px; }
            .auto-deploy-status {
                display: inline-block;
                padding: 8px 16px;
                border-radius: 4px;
                margin-left: 20px;
            }
            .auto-deploy-enabled { background: #1b5e20; }
            .auto-deploy-disabled { background: #b71c1c; }
            .history-item {
                padding: 10px;
                border-left: 3px solid;
                margin-bottom: 10px;
                background: #0d0d1a;
            }
            .history-success { border-color: #4caf50; }
            .history-failure { border-color: #ef5350; }
            .refresh-info { color: #666; font-size: 14px; }
        </style>
    </head>
    <body>
        <div class="container">
            <h1>Deploy Controller
                <span class="auto-deploy-status """ + ("auto-deploy-enabled" if auto_deploy_enabled else "auto-deploy-disabled") + """">
                    Auto-Deploy: """ + ("Enabled" if auto_deploy_enabled else "Disabled") + """
                </span>
            </h1>

            <div class="stats">
                <div class="stat-card">
                    <div class="stat-value">""" + str(len(current_deployments)) + """</div>
                    <div class="stat-label">Deployments</div>
                </div>
                <div class="stat-card">
                    <div class="stat-value">""" + str(len(registry_images)) + """</div>
                    <div class="stat-label">Registry Images</div>
                </div>
                <div class="stat-card">
                    <div class="stat-value">""" + str(len(pending)) + """</div>
                    <div class="stat-label">Pending Deploy</div>
                </div>
                <div class="stat-card">
                    <div class="stat-value">""" + str(sum(1 for d in current_deployments.values() if d.get('ready', 0) >= d.get('replicas', 1))) + """</div>
                    <div class="stat-label">Healthy</div>
                </div>
            </div>

            <div class="card">
                <h2>Current Deployments</h2>
                <table>
                    <thead>
                        <tr>
                            <th>Service</th>
                            <th>Image</th>
                            <th>Replicas</th>
                            <th>Status</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody>
    """

    for name, info in sorted(current_deployments.items()):
        ready = info.get('ready', 0)
        replicas = info.get('replicas', 1)
        status_class = "status-running" if ready >= replicas else "status-pending"
        status_text = "Running" if ready >= replicas else f"Starting ({ready}/{replicas})"

        html += f"""
                        <tr>
                            <td><strong>{name}</strong></td>
                            <td style="font-family: monospace; font-size: 12px;">{info.get('image', 'unknown')}</td>
                            <td>{ready}/{replicas}</td>
                            <td><span class="status-badge {status_class}">{status_text}</span></td>
                            <td>
                                <button class="btn btn-danger" onclick="rollback('{name}')">Rollback</button>
                            </td>
                        </tr>
        """

    html += """
                    </tbody>
                </table>
            </div>

            <div class="card">
                <h2>Pending Deployments</h2>
    """

    if pending:
        html += "<ul>"
        for service in sorted(pending):
            html += f'<li>{service} <button class="btn btn-primary" onclick="deploy(\'{service}\')">Deploy Now</button></li>'
        html += "</ul>"
    else:
        html += "<p>No pending deployments</p>"

    html += """
            </div>

            <div class="card">
                <h2>Recent Activity</h2>
    """

    for entry in reversed(deployment_history[-10:]):
        status_class = "history-success" if entry.get('success') else "history-failure"
        html += f"""
                <div class="history-item {status_class}">
                    <strong>{entry.get('action', 'unknown').upper()}</strong> - {entry.get('service', 'unknown')}
                    <br><small>{entry.get('timestamp', '')} - {entry.get('message', '')}</small>
                </div>
        """

    if not deployment_history:
        html += "<p>No recent activity</p>"

    html += """
            </div>

            <p class="refresh-info">
                Last check: """ + (last_check_time.strftime("%Y-%m-%d %H:%M:%S") if last_check_time else "Never") + """ |
                Auto-refresh every """ + str(CHECK_INTERVAL) + """ seconds
            </p>
        </div>

        <script>
            async function deploy(service) {
                if (confirm('Deploy ' + service + '?')) {
                    const response = await fetch('/api/deploy', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({image: service})
                    });
                    const result = await response.json();
                    alert(result.message || 'Deployed');
                    location.reload();
                }
            }

            async function rollback(service) {
                if (confirm('Rollback ' + service + '?')) {
                    const response = await fetch('/api/rollback/' + service, {
                        method: 'POST'
                    });
                    const result = await response.json();
                    alert(result.message || 'Rolled back');
                    location.reload();
                }
            }

            // Auto-refresh every 30 seconds
            setTimeout(() => location.reload(), 30000);
        </script>
    </body>
    </html>
    """

    return html


@app.get("/api/deployments")
async def list_deployments():
    """List all deployments."""
    deployments = get_current_deployments()
    return {
        "deployments": [
            {
                "name": name,
                "image": info.get("image", ""),
                "replicas": info.get("replicas", 1),
                "ready": info.get("ready", 0),
                "status": "running" if info.get("ready", 0) >= info.get("replicas", 1) else "pending"
            }
            for name, info in deployments.items()
        ],
        "total": len(deployments)
    }


@app.post("/api/deploy")
async def deploy_image(request: DeployRequest, background_tasks: BackgroundTasks):
    """Deploy an image."""
    # Extract service name from image
    image = request.image
    if image.startswith("holm/"):
        service_name = image.replace("holm/", "")
    else:
        service_name = image

    success, message = deploy_service(service_name, request.replicas)

    if not success:
        raise HTTPException(status_code=500, detail=message)

    return {
        "success": True,
        "message": f"Deployed {service_name}",
        "service": service_name
    }


@app.post("/api/rollback/{name}")
async def rollback_service(name: str):
    """Rollback a deployment."""
    success, message = rollback_deployment(name)

    if not success:
        raise HTTPException(status_code=500, detail=message)

    return {
        "success": True,
        "message": f"Rolled back {name}",
        "service": name
    }


@app.get("/api/images")
async def list_images():
    """List images in registry."""
    images = await get_registry_images()
    result = []

    for image in images:
        tags = await get_image_tags(image)
        result.append({
            "name": image,
            "tags": tags
        })

    return {
        "images": result,
        "total": len(result)
    }


@app.get("/api/pending")
async def list_pending():
    """List images not yet deployed."""
    registry_images = await get_registry_images()
    current_deployments = get_current_deployments()

    deployed_services = set(current_deployments.keys())

    pending = []
    for image in registry_images:
        service_name = image
        if service_name not in deployed_services:
            pending.append({
                "image": image,
                "service": service_name
            })

    return {
        "pending": pending,
        "total": len(pending)
    }


@app.get("/api/history")
async def get_history():
    """Get deployment history."""
    return {
        "history": list(reversed(deployment_history[-50:])),
        "total": len(deployment_history)
    }


@app.post("/api/auto-deploy/{enabled}")
async def set_auto_deploy(enabled: bool):
    """Enable or disable auto-deploy."""
    global auto_deploy_enabled
    auto_deploy_enabled = enabled
    return {
        "auto_deploy": auto_deploy_enabled,
        "message": f"Auto-deploy {'enabled' if enabled else 'disabled'}"
    }


@app.get("/health")
async def health_check():
    """Health check endpoint."""
    # Try to connect to registry
    registry_ok = False
    try:
        async with httpx.AsyncClient(timeout=5) as client:
            response = await client.get(f"{REGISTRY_URL}/v2/")
            registry_ok = response.status_code == 200
    except:
        pass

    # Try kubectl
    kubectl_ok, _ = run_kubectl(["version", "--client"])

    return {
        "status": "healthy" if registry_ok and kubectl_ok else "degraded",
        "registry": "connected" if registry_ok else "disconnected",
        "kubectl": "available" if kubectl_ok else "unavailable",
        "auto_deploy": auto_deploy_enabled,
        "last_check": last_check_time.isoformat() if last_check_time else None,
        "timestamp": datetime.now().isoformat()
    }


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8080)
