from flask import Flask, request, jsonify, render_template_string
import os
import json
import hashlib
import base64
import secrets
import time
from datetime import datetime
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from functools import wraps
import threading

app = Flask(__name__)

# Vault configuration
VAULT_NAME = "Vault"
VAULT_MOTTO = "Your secrets are safe with me"
DATA_DIR = "/data"
SECRETS_FILE = os.path.join(DATA_DIR, "secrets.json")
AUDIT_FILE = os.path.join(DATA_DIR, "audit.log")
KEY_FILE = os.path.join(DATA_DIR, "master.key")

# Thread lock for file operations
file_lock = threading.Lock()

# Catppuccin Mocha theme
CATPPUCCIN = {
    "base": "#1e1e2e",
    "mantle": "#181825",
    "crust": "#11111b",
    "surface0": "#313244",
    "surface1": "#45475a",
    "surface2": "#585b70",
    "overlay0": "#6c7086",
    "text": "#cdd6f4",
    "subtext0": "#a6adc8",
    "lavender": "#b4befe",
    "blue": "#89b4fa",
    "sapphire": "#74c7ec",
    "teal": "#94e2d5",
    "green": "#a6e3a1",
    "yellow": "#f9e2af",
    "peach": "#fab387",
    "red": "#f38ba8",
    "mauve": "#cba6f7",
    "pink": "#f5c2e7"
}

# ============ Encryption Functions ============

def get_master_key():
    """Get or create master encryption key"""
    if os.path.exists(KEY_FILE):
        with open(KEY_FILE, 'rb') as f:
            return f.read()
    else:
        key = secrets.token_bytes(32)  # 256-bit key for AES-256
        os.makedirs(DATA_DIR, exist_ok=True)
        with open(KEY_FILE, 'wb') as f:
            f.write(key)
        os.chmod(KEY_FILE, 0o600)
        return key

def encrypt_value(plaintext: str) -> dict:
    """Encrypt a value using AES-256-GCM"""
    key = get_master_key()
    aesgcm = AESGCM(key)
    nonce = secrets.token_bytes(12)  # 96-bit nonce for GCM
    ciphertext = aesgcm.encrypt(nonce, plaintext.encode('utf-8'), None)
    return {
        'nonce': base64.b64encode(nonce).decode('utf-8'),
        'ciphertext': base64.b64encode(ciphertext).decode('utf-8')
    }

def decrypt_value(encrypted: dict) -> str:
    """Decrypt a value using AES-256-GCM"""
    key = get_master_key()
    aesgcm = AESGCM(key)
    nonce = base64.b64decode(encrypted['nonce'])
    ciphertext = base64.b64decode(encrypted['ciphertext'])
    plaintext = aesgcm.decrypt(nonce, ciphertext, None)
    return plaintext.decode('utf-8')

# ============ Storage Functions ============

def load_secrets():
    """Load secrets from file"""
    if not os.path.exists(SECRETS_FILE):
        return {}
    with open(SECRETS_FILE, 'r') as f:
        return json.load(f)

def save_secrets(secrets_data):
    """Save secrets to file"""
    os.makedirs(DATA_DIR, exist_ok=True)
    with open(SECRETS_FILE, 'w') as f:
        json.dump(secrets_data, f, indent=2)
    os.chmod(SECRETS_FILE, 0o600)

def log_audit(action: str, secret_name: str, user: str = "system", details: str = ""):
    """Log an audit event"""
    os.makedirs(DATA_DIR, exist_ok=True)
    timestamp = datetime.now().isoformat()
    entry = {
        'timestamp': timestamp,
        'action': action,
        'secret_name': secret_name,
        'user': user,
        'details': details
    }
    with file_lock:
        with open(AUDIT_FILE, 'a') as f:
            f.write(json.dumps(entry) + '\n')

def get_audit_logs(limit: int = 100):
    """Get recent audit logs"""
    if not os.path.exists(AUDIT_FILE):
        return []
    logs = []
    with open(AUDIT_FILE, 'r') as f:
        for line in f:
            if line.strip():
                logs.append(json.loads(line))
    return logs[-limit:][::-1]  # Return most recent first

# ============ Secret Operations ============

def create_secret(name: str, value: str, metadata: dict = None):
    """Create a new secret with versioning"""
    with file_lock:
        secrets_data = load_secrets()
        if name in secrets_data:
            raise ValueError(f"Secret '{name}' already exists. Use update instead.")

        encrypted = encrypt_value(value)
        version_entry = {
            'version': 1,
            'encrypted': encrypted,
            'created_at': datetime.now().isoformat(),
            'metadata': metadata or {}
        }

        secrets_data[name] = {
            'current_version': 1,
            'versions': [version_entry],
            'created_at': datetime.now().isoformat(),
            'updated_at': datetime.now().isoformat()
        }

        save_secrets(secrets_data)
        log_audit('CREATE', name, details=f"Version 1 created")
        return version_entry

def read_secret(name: str, version: int = None):
    """Read a secret (optionally specific version)"""
    secrets_data = load_secrets()
    if name not in secrets_data:
        raise KeyError(f"Secret '{name}' not found")

    secret = secrets_data[name]
    target_version = version or secret['current_version']

    for v in secret['versions']:
        if v['version'] == target_version:
            decrypted = decrypt_value(v['encrypted'])
            log_audit('READ', name, details=f"Version {target_version} accessed")
            return {
                'name': name,
                'value': decrypted,
                'version': v['version'],
                'created_at': v['created_at'],
                'metadata': v.get('metadata', {})
            }

    raise KeyError(f"Version {target_version} not found for secret '{name}'")

def update_secret(name: str, value: str, metadata: dict = None):
    """Update a secret (creates new version)"""
    with file_lock:
        secrets_data = load_secrets()
        if name not in secrets_data:
            raise KeyError(f"Secret '{name}' not found")

        secret = secrets_data[name]
        new_version = secret['current_version'] + 1

        encrypted = encrypt_value(value)
        version_entry = {
            'version': new_version,
            'encrypted': encrypted,
            'created_at': datetime.now().isoformat(),
            'metadata': metadata or {}
        }

        secret['versions'].append(version_entry)
        secret['current_version'] = new_version
        secret['updated_at'] = datetime.now().isoformat()

        save_secrets(secrets_data)
        log_audit('UPDATE', name, details=f"Version {new_version} created")
        return version_entry

def delete_secret(name: str, version: int = None):
    """Delete a secret or specific version"""
    with file_lock:
        secrets_data = load_secrets()
        if name not in secrets_data:
            raise KeyError(f"Secret '{name}' not found")

        if version:
            # Delete specific version
            secret = secrets_data[name]
            original_count = len(secret['versions'])
            secret['versions'] = [v for v in secret['versions'] if v['version'] != version]

            if len(secret['versions']) == original_count:
                raise KeyError(f"Version {version} not found")

            if len(secret['versions']) == 0:
                del secrets_data[name]
                log_audit('DELETE', name, details=f"All versions deleted")
            else:
                # Update current version if needed
                if secret['current_version'] == version:
                    secret['current_version'] = max(v['version'] for v in secret['versions'])
                log_audit('DELETE', name, details=f"Version {version} deleted")
        else:
            # Delete entire secret
            del secrets_data[name]
            log_audit('DELETE', name, details="Secret and all versions deleted")

        save_secrets(secrets_data)

def list_secrets():
    """List all secrets (without values)"""
    secrets_data = load_secrets()
    result = []
    for name, data in secrets_data.items():
        result.append({
            'name': name,
            'current_version': data['current_version'],
            'version_count': len(data['versions']),
            'created_at': data['created_at'],
            'updated_at': data['updated_at']
        })
    log_audit('LIST', '*', details=f"Listed {len(result)} secrets")
    return result

# ============ HTML Template ============

VAULT_HTML = '''
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Vault - Secret Manager</title>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --base: ''' + CATPPUCCIN['base'] + ''';
            --mantle: ''' + CATPPUCCIN['mantle'] + ''';
            --crust: ''' + CATPPUCCIN['crust'] + ''';
            --surface0: ''' + CATPPUCCIN['surface0'] + ''';
            --surface1: ''' + CATPPUCCIN['surface1'] + ''';
            --text: ''' + CATPPUCCIN['text'] + ''';
            --subtext0: ''' + CATPPUCCIN['subtext0'] + ''';
            --lavender: ''' + CATPPUCCIN['lavender'] + ''';
            --blue: ''' + CATPPUCCIN['blue'] + ''';
            --teal: ''' + CATPPUCCIN['teal'] + ''';
            --green: ''' + CATPPUCCIN['green'] + ''';
            --yellow: ''' + CATPPUCCIN['yellow'] + ''';
            --peach: ''' + CATPPUCCIN['peach'] + ''';
            --red: ''' + CATPPUCCIN['red'] + ''';
            --mauve: ''' + CATPPUCCIN['mauve'] + ''';
        }

        * { margin: 0; padding: 0; box-sizing: border-box; }

        body {
            font-family: "Inter", sans-serif;
            background: var(--base);
            color: var(--text);
            min-height: 100vh;
        }

        .header {
            background: var(--mantle);
            padding: 1.5rem 2rem;
            border-bottom: 1px solid var(--surface0);
            display: flex;
            align-items: center;
            justify-content: space-between;
        }

        .logo {
            display: flex;
            align-items: center;
            gap: 1rem;
        }

        .logo-icon {
            width: 48px;
            height: 48px;
            background: linear-gradient(135deg, var(--mauve), var(--lavender));
            border-radius: 12px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 24px;
        }

        .logo-text h1 {
            font-size: 1.5rem;
            font-weight: 700;
            color: var(--text);
        }

        .logo-text .motto {
            font-size: 0.85rem;
            color: var(--subtext0);
            font-style: italic;
        }

        .stats {
            display: flex;
            gap: 2rem;
        }

        .stat {
            text-align: center;
        }

        .stat-value {
            font-size: 1.5rem;
            font-weight: 700;
            font-family: "JetBrains Mono", monospace;
            color: var(--mauve);
        }

        .stat-label {
            font-size: 0.75rem;
            color: var(--subtext0);
            text-transform: uppercase;
        }

        .container {
            display: grid;
            grid-template-columns: 1fr 400px;
            gap: 1.5rem;
            padding: 1.5rem;
            max-width: 1600px;
            margin: 0 auto;
        }

        .panel {
            background: var(--mantle);
            border-radius: 12px;
            border: 1px solid var(--surface0);
            overflow: hidden;
        }

        .panel-header {
            padding: 1rem 1.25rem;
            background: var(--surface0);
            font-weight: 600;
            display: flex;
            align-items: center;
            justify-content: space-between;
        }

        .panel-content {
            padding: 1rem;
        }

        .secrets-list {
            max-height: 400px;
            overflow-y: auto;
        }

        .secret-item {
            padding: 1rem;
            border-radius: 8px;
            background: var(--surface0);
            margin-bottom: 0.75rem;
            cursor: pointer;
            transition: all 0.2s;
        }

        .secret-item:hover {
            background: var(--surface1);
            transform: translateX(4px);
        }

        .secret-item.selected {
            border-left: 3px solid var(--mauve);
        }

        .secret-name {
            font-family: "JetBrains Mono", monospace;
            font-weight: 600;
            color: var(--lavender);
            margin-bottom: 0.25rem;
        }

        .secret-meta {
            font-size: 0.8rem;
            color: var(--subtext0);
            display: flex;
            gap: 1rem;
        }

        .form-group {
            margin-bottom: 1rem;
        }

        .form-group label {
            display: block;
            margin-bottom: 0.5rem;
            font-size: 0.85rem;
            color: var(--subtext0);
        }

        .form-group input, .form-group textarea, .form-group select {
            width: 100%;
            padding: 0.75rem;
            border-radius: 8px;
            border: 1px solid var(--surface1);
            background: var(--surface0);
            color: var(--text);
            font-family: "JetBrains Mono", monospace;
            font-size: 0.9rem;
        }

        .form-group input:focus, .form-group textarea:focus {
            outline: none;
            border-color: var(--mauve);
        }

        .form-group textarea {
            min-height: 100px;
            resize: vertical;
        }

        .btn {
            padding: 0.75rem 1.5rem;
            border-radius: 8px;
            border: none;
            cursor: pointer;
            font-weight: 600;
            font-size: 0.9rem;
            transition: all 0.2s;
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
        }

        .btn-primary {
            background: var(--mauve);
            color: var(--crust);
        }

        .btn-primary:hover {
            filter: brightness(1.1);
        }

        .btn-success {
            background: var(--green);
            color: var(--crust);
        }

        .btn-danger {
            background: var(--red);
            color: var(--crust);
        }

        .btn-secondary {
            background: var(--surface1);
            color: var(--text);
        }

        .btn-group {
            display: flex;
            gap: 0.5rem;
            margin-top: 1rem;
        }

        .audit-log {
            max-height: 300px;
            overflow-y: auto;
        }

        .audit-entry {
            padding: 0.75rem;
            border-radius: 6px;
            background: var(--surface0);
            margin-bottom: 0.5rem;
            font-size: 0.85rem;
        }

        .audit-entry .action {
            font-weight: 600;
            text-transform: uppercase;
            font-size: 0.75rem;
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            margin-right: 0.5rem;
        }

        .audit-entry .action.CREATE { background: var(--green); color: var(--crust); }
        .audit-entry .action.READ { background: var(--blue); color: var(--crust); }
        .audit-entry .action.UPDATE { background: var(--yellow); color: var(--crust); }
        .audit-entry .action.DELETE { background: var(--red); color: var(--crust); }
        .audit-entry .action.LIST { background: var(--teal); color: var(--crust); }

        .audit-time {
            color: var(--subtext0);
            font-size: 0.75rem;
            font-family: "JetBrains Mono", monospace;
        }

        .secret-value-display {
            background: var(--surface0);
            padding: 1rem;
            border-radius: 8px;
            font-family: "JetBrains Mono", monospace;
            word-break: break-all;
            position: relative;
        }

        .secret-value-display.hidden {
            filter: blur(8px);
            user-select: none;
        }

        .toggle-visibility {
            position: absolute;
            top: 0.5rem;
            right: 0.5rem;
            background: var(--surface1);
            border: none;
            padding: 0.5rem;
            border-radius: 4px;
            cursor: pointer;
            color: var(--text);
        }

        .version-select {
            display: flex;
            align-items: center;
            gap: 0.5rem;
            margin-bottom: 1rem;
        }

        .version-badge {
            background: var(--mauve);
            color: var(--crust);
            padding: 0.25rem 0.75rem;
            border-radius: 12px;
            font-size: 0.8rem;
            font-weight: 600;
        }

        .empty-state {
            text-align: center;
            padding: 3rem;
            color: var(--subtext0);
        }

        .empty-state svg {
            width: 64px;
            height: 64px;
            margin-bottom: 1rem;
            opacity: 0.5;
        }

        .tabs {
            display: flex;
            gap: 0.5rem;
            padding: 1rem;
            background: var(--surface0);
        }

        .tab {
            padding: 0.5rem 1rem;
            border-radius: 6px;
            cursor: pointer;
            font-size: 0.9rem;
            transition: all 0.2s;
            border: none;
            background: transparent;
            color: var(--subtext0);
        }

        .tab.active {
            background: var(--mauve);
            color: var(--crust);
        }

        .tab:hover:not(.active) {
            background: var(--surface1);
            color: var(--text);
        }

        .toast {
            position: fixed;
            bottom: 2rem;
            right: 2rem;
            padding: 1rem 1.5rem;
            border-radius: 8px;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            display: none;
            animation: slideIn 0.3s ease;
        }

        .toast.success { border-color: var(--green); }
        .toast.error { border-color: var(--red); }

        @keyframes slideIn {
            from { transform: translateY(20px); opacity: 0; }
            to { transform: translateY(0); opacity: 1; }
        }

        @media (max-width: 1024px) {
            .container {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="logo">
            <div class="logo-icon">🔐</div>
            <div class="logo-text">
                <h1>Vault</h1>
                <div class="motto">Your secrets are safe with me</div>
            </div>
        </div>
        <div class="stats">
            <div class="stat">
                <div class="stat-value" id="totalSecrets">0</div>
                <div class="stat-label">Secrets</div>
            </div>
            <div class="stat">
                <div class="stat-value" id="totalVersions">0</div>
                <div class="stat-label">Versions</div>
            </div>
            <div class="stat">
                <div class="stat-value" id="encryption">AES-256</div>
                <div class="stat-label">Encryption</div>
            </div>
        </div>
    </div>

    <div class="container">
        <div class="main-area">
            <div class="panel">
                <div class="tabs">
                    <button class="tab active" onclick="showTab('secrets')">Secrets</button>
                    <button class="tab" onclick="showTab('create')">Create New</button>
                    <button class="tab" onclick="showTab('audit')">Audit Log</button>
                </div>

                <div id="secretsTab" class="panel-content">
                    <div class="secrets-list" id="secretsList">
                        <div class="empty-state">
                            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                                <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
                                <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
                            </svg>
                            <p>No secrets stored yet</p>
                            <p>Click "Create New" to add your first secret</p>
                        </div>
                    </div>
                </div>

                <div id="createTab" class="panel-content" style="display: none;">
                    <form id="createForm" onsubmit="createSecret(event)">
                        <div class="form-group">
                            <label>Secret Name</label>
                            <input type="text" id="newSecretName" placeholder="my-secret-key" required>
                        </div>
                        <div class="form-group">
                            <label>Secret Value</label>
                            <textarea id="newSecretValue" placeholder="Enter your secret value..." required></textarea>
                        </div>
                        <div class="form-group">
                            <label>Description (Optional)</label>
                            <input type="text" id="newSecretDesc" placeholder="What is this secret for?">
                        </div>
                        <div class="btn-group">
                            <button type="submit" class="btn btn-success">Create Secret</button>
                        </div>
                    </form>
                </div>

                <div id="auditTab" class="panel-content" style="display: none;">
                    <div class="audit-log" id="auditLog">
                        <div class="empty-state">
                            <p>No audit entries yet</p>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <div class="sidebar">
            <div class="panel">
                <div class="panel-header">
                    <span>Secret Details</span>
                    <span id="selectedSecretName">-</span>
                </div>
                <div class="panel-content" id="secretDetails">
                    <div class="empty-state">
                        <p>Select a secret to view details</p>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <div class="toast" id="toast"></div>

    <script>
        let selectedSecret = null;
        let secretValueVisible = false;

        function showToast(message, type = 'success') {
            const toast = document.getElementById('toast');
            toast.textContent = message;
            toast.className = 'toast ' + type;
            toast.style.display = 'block';
            setTimeout(() => toast.style.display = 'none', 3000);
        }

        function showTab(tabName) {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('[id$="Tab"]').forEach(t => t.style.display = 'none');

            event.target.classList.add('active');
            document.getElementById(tabName + 'Tab').style.display = 'block';

            if (tabName === 'audit') loadAuditLog();
        }

        async function loadSecrets() {
            try {
                const res = await fetch('/api/secrets');
                const data = await res.json();

                const list = document.getElementById('secretsList');
                document.getElementById('totalSecrets').textContent = data.length;

                let totalVersions = 0;
                data.forEach(s => totalVersions += s.version_count);
                document.getElementById('totalVersions').textContent = totalVersions;

                if (data.length === 0) {
                    list.innerHTML = '<div class="empty-state"><p>No secrets stored yet</p></div>';
                    return;
                }

                list.innerHTML = data.map(s => `
                    <div class="secret-item" onclick="selectSecret('${s.name}')">
                        <div class="secret-name">${s.name}</div>
                        <div class="secret-meta">
                            <span>v${s.current_version}</span>
                            <span>${s.version_count} version(s)</span>
                            <span>${new Date(s.updated_at).toLocaleDateString()}</span>
                        </div>
                    </div>
                `).join('');
            } catch (err) {
                showToast('Failed to load secrets', 'error');
            }
        }

        async function selectSecret(name) {
            selectedSecret = name;
            secretValueVisible = false;
            document.getElementById('selectedSecretName').textContent = name;

            try {
                const res = await fetch('/api/secrets/' + encodeURIComponent(name));
                const data = await res.json();

                const details = document.getElementById('secretDetails');
                details.innerHTML = `
                    <div class="version-select">
                        <label>Version:</label>
                        <span class="version-badge">v${data.version}</span>
                    </div>
                    <div class="form-group">
                        <label>Value</label>
                        <div class="secret-value-display hidden" id="secretValue">${data.value}</div>
                        <button class="toggle-visibility" onclick="toggleVisibility()">👁</button>
                    </div>
                    <div class="form-group">
                        <label>Created</label>
                        <div style="color: var(--subtext0); font-size: 0.9rem;">${new Date(data.created_at).toLocaleString()}</div>
                    </div>
                    <div class="btn-group">
                        <button class="btn btn-primary" onclick="showUpdateForm()">Update</button>
                        <button class="btn btn-secondary" onclick="copySecret()">Copy</button>
                        <button class="btn btn-danger" onclick="deleteSecret('${name}')">Delete</button>
                    </div>
                    <div id="updateForm" style="display: none; margin-top: 1rem;">
                        <div class="form-group">
                            <label>New Value</label>
                            <textarea id="updateValue" placeholder="Enter new value..."></textarea>
                        </div>
                        <div class="btn-group">
                            <button class="btn btn-success" onclick="updateSecret('${name}')">Save Update</button>
                            <button class="btn btn-secondary" onclick="hideUpdateForm()">Cancel</button>
                        </div>
                    </div>
                `;
            } catch (err) {
                showToast('Failed to load secret', 'error');
            }
        }

        function toggleVisibility() {
            secretValueVisible = !secretValueVisible;
            const el = document.getElementById('secretValue');
            el.classList.toggle('hidden', !secretValueVisible);
        }

        function showUpdateForm() {
            document.getElementById('updateForm').style.display = 'block';
        }

        function hideUpdateForm() {
            document.getElementById('updateForm').style.display = 'none';
        }

        async function createSecret(e) {
            e.preventDefault();
            const name = document.getElementById('newSecretName').value;
            const value = document.getElementById('newSecretValue').value;
            const desc = document.getElementById('newSecretDesc').value;

            try {
                const res = await fetch('/api/secrets', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name, value, metadata: { description: desc } })
                });

                if (!res.ok) {
                    const err = await res.json();
                    throw new Error(err.error);
                }

                showToast('Secret created successfully');
                document.getElementById('createForm').reset();
                loadSecrets();
                showTab('secrets');
                document.querySelector('.tab').click();
            } catch (err) {
                showToast(err.message, 'error');
            }
        }

        async function updateSecret(name) {
            const value = document.getElementById('updateValue').value;
            if (!value) {
                showToast('Please enter a new value', 'error');
                return;
            }

            try {
                const res = await fetch('/api/secrets/' + encodeURIComponent(name), {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ value })
                });

                if (!res.ok) throw new Error('Update failed');

                showToast('Secret updated (new version created)');
                loadSecrets();
                selectSecret(name);
            } catch (err) {
                showToast('Failed to update secret', 'error');
            }
        }

        async function deleteSecret(name) {
            if (!confirm('Are you sure you want to delete "' + name + '"? This cannot be undone.')) return;

            try {
                const res = await fetch('/api/secrets/' + encodeURIComponent(name), {
                    method: 'DELETE'
                });

                if (!res.ok) throw new Error('Delete failed');

                showToast('Secret deleted');
                document.getElementById('secretDetails').innerHTML = '<div class="empty-state"><p>Select a secret to view details</p></div>';
                document.getElementById('selectedSecretName').textContent = '-';
                loadSecrets();
            } catch (err) {
                showToast('Failed to delete secret', 'error');
            }
        }

        function copySecret() {
            const el = document.getElementById('secretValue');
            navigator.clipboard.writeText(el.textContent);
            showToast('Secret copied to clipboard');
        }

        async function loadAuditLog() {
            try {
                const res = await fetch('/api/audit');
                const data = await res.json();

                const log = document.getElementById('auditLog');
                if (data.length === 0) {
                    log.innerHTML = '<div class="empty-state"><p>No audit entries yet</p></div>';
                    return;
                }

                log.innerHTML = data.map(e => `
                    <div class="audit-entry">
                        <span class="action ${e.action}">${e.action}</span>
                        <strong>${e.secret_name}</strong>
                        <span style="color: var(--subtext0);">${e.details}</span>
                        <div class="audit-time">${new Date(e.timestamp).toLocaleString()}</div>
                    </div>
                `).join('');
            } catch (err) {
                showToast('Failed to load audit log', 'error');
            }
        }

        // Initial load
        loadSecrets();
    </script>
</body>
</html>
'''

# ============ API Routes ============

@app.route('/')
def index():
    return render_template_string(VAULT_HTML)

@app.route('/api/secrets', methods=['GET'])
def api_list_secrets():
    try:
        secrets = list_secrets()
        return jsonify(secrets)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets', methods=['POST'])
def api_create_secret():
    try:
        data = request.get_json()
        name = data.get('name')
        value = data.get('value')
        metadata = data.get('metadata', {})

        if not name or not value:
            return jsonify({'error': 'Name and value are required'}), 400

        result = create_secret(name, value, metadata)
        return jsonify({'success': True, 'version': result['version']})
    except ValueError as e:
        return jsonify({'error': str(e)}), 400
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets/<name>', methods=['GET'])
def api_read_secret(name):
    try:
        version = request.args.get('version', type=int)
        secret = read_secret(name, version)
        return jsonify(secret)
    except KeyError as e:
        return jsonify({'error': str(e)}), 404
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets/<name>', methods=['PUT'])
def api_update_secret(name):
    try:
        data = request.get_json()
        value = data.get('value')
        metadata = data.get('metadata', {})

        if not value:
            return jsonify({'error': 'Value is required'}), 400

        result = update_secret(name, value, metadata)
        return jsonify({'success': True, 'version': result['version']})
    except KeyError as e:
        return jsonify({'error': str(e)}), 404
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets/<name>', methods=['DELETE'])
def api_delete_secret(name):
    try:
        version = request.args.get('version', type=int)
        delete_secret(name, version)
        return jsonify({'success': True})
    except KeyError as e:
        return jsonify({'error': str(e)}), 404
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/audit', methods=['GET'])
def api_audit_log():
    try:
        limit = request.args.get('limit', 100, type=int)
        logs = get_audit_logs(limit)
        return jsonify(logs)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/health', methods=['GET'])
def health():
    return jsonify({
        'status': 'healthy',
        'service': VAULT_NAME,
        'motto': VAULT_MOTTO,
        'encryption': 'AES-256-GCM'
    })

if __name__ == '__main__':
    # Ensure data directory exists
    os.makedirs(DATA_DIR, exist_ok=True)
    # Initialize master key on startup
    get_master_key()
    port = int(os.environ.get('PORT', 8080))
    app.run(host='0.0.0.0', port=port)
