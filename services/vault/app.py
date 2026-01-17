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

def list_secret_versions(name: str):
    """List all versions of a secret"""
    secrets_data = load_secrets()
    if name not in secrets_data:
        raise KeyError(f"Secret '{name}' not found")

    secret = secrets_data[name]
    versions = []
    for v in secret['versions']:
        versions.append({
            'version': v['version'],
            'created_at': v['created_at'],
            'metadata': v.get('metadata', {})
        })
    log_audit('LIST_VERSIONS', name, details=f"Listed {len(versions)} versions")
    return {
        'name': name,
        'current_version': secret['current_version'],
        'versions': versions
    }

def rollback_secret(name: str, version: int):
    """Rollback a secret to a previous version"""
    with file_lock:
        secrets_data = load_secrets()
        if name not in secrets_data:
            raise KeyError(f"Secret '{name}' not found")

        secret = secrets_data[name]
        target_version = None
        for v in secret['versions']:
            if v['version'] == version:
                target_version = v
                break

        if not target_version:
            raise KeyError(f"Version {version} not found for secret '{name}'")

        # Create a new version with the old value
        new_version = secret['current_version'] + 1
        version_entry = {
            'version': new_version,
            'encrypted': target_version['encrypted'],
            'created_at': datetime.now().isoformat(),
            'metadata': {**target_version.get('metadata', {}), 'rollback_from': version}
        }

        secret['versions'].append(version_entry)
        secret['current_version'] = new_version
        secret['updated_at'] = datetime.now().isoformat()

        save_secrets(secrets_data)
        log_audit('ROLLBACK', name, details=f"Rolled back to version {version}, created version {new_version}")
        return version_entry

def update_secret_metadata(name: str, metadata: dict, version: int = None):
    """Update metadata for a secret without changing the value"""
    with file_lock:
        secrets_data = load_secrets()
        if name not in secrets_data:
            raise KeyError(f"Secret '{name}' not found")

        secret = secrets_data[name]
        target_version = version or secret['current_version']

        for v in secret['versions']:
            if v['version'] == target_version:
                v['metadata'] = {**v.get('metadata', {}), **metadata}
                save_secrets(secrets_data)
                log_audit('UPDATE_METADATA', name, details=f"Updated metadata for version {target_version}")
                return {'version': target_version, 'metadata': v['metadata']}

        raise KeyError(f"Version {target_version} not found for secret '{name}'")

def bulk_create_secrets(secrets_list: list):
    """Create multiple secrets at once"""
    results = {'created': [], 'errors': []}
    for item in secrets_list:
        try:
            name = item.get('name')
            value = item.get('value')
            metadata = item.get('metadata', {})
            if not name or not value:
                results['errors'].append({'name': name, 'error': 'Name and value are required'})
                continue
            create_secret(name, value, metadata)
            results['created'].append(name)
        except Exception as e:
            results['errors'].append({'name': item.get('name'), 'error': str(e)})
    return results

def bulk_delete_secrets(names: list):
    """Delete multiple secrets at once"""
    results = {'deleted': [], 'errors': []}
    for name in names:
        try:
            delete_secret(name)
            results['deleted'].append(name)
        except Exception as e:
            results['errors'].append({'name': name, 'error': str(e)})
    return results

def rotate_master_key():
    """Rotate the master encryption key and re-encrypt all secrets"""
    with file_lock:
        secrets_data = load_secrets()

        # Decrypt all values with old key
        decrypted_secrets = {}
        for name, secret in secrets_data.items():
            decrypted_secrets[name] = {
                'secret': secret,
                'decrypted_versions': []
            }
            for v in secret['versions']:
                try:
                    decrypted_value = decrypt_value(v['encrypted'])
                    decrypted_secrets[name]['decrypted_versions'].append({
                        'version': v['version'],
                        'value': decrypted_value,
                        'created_at': v['created_at'],
                        'metadata': v.get('metadata', {})
                    })
                except Exception:
                    # If decryption fails, skip this version
                    pass

        # Generate new key
        new_key = secrets.token_bytes(32)

        # Backup old key
        if os.path.exists(KEY_FILE):
            backup_file = KEY_FILE + '.backup.' + datetime.now().strftime('%Y%m%d%H%M%S')
            with open(KEY_FILE, 'rb') as f:
                old_key = f.read()
            with open(backup_file, 'wb') as f:
                f.write(old_key)
            os.chmod(backup_file, 0o600)

        # Write new key
        with open(KEY_FILE, 'wb') as f:
            f.write(new_key)
        os.chmod(KEY_FILE, 0o600)

        # Re-encrypt all secrets with new key
        for name, data in decrypted_secrets.items():
            secret = data['secret']
            new_versions = []
            for dv in data['decrypted_versions']:
                encrypted = encrypt_value(dv['value'])
                new_versions.append({
                    'version': dv['version'],
                    'encrypted': encrypted,
                    'created_at': dv['created_at'],
                    'metadata': dv['metadata']
                })
            secret['versions'] = new_versions
            secrets_data[name] = secret

        save_secrets(secrets_data)
        log_audit('KEY_ROTATE', '*', details='Master key rotated, all secrets re-encrypted')
        return {'success': True, 'secrets_reencrypted': len(secrets_data)}

def get_vault_stats():
    """Get vault statistics"""
    secrets_data = load_secrets()
    total_secrets = len(secrets_data)
    total_versions = sum(len(s['versions']) for s in secrets_data.values())

    # Get audit log stats
    audit_logs = get_audit_logs(1000)
    action_counts = {}
    for log in audit_logs:
        action = log.get('action', 'UNKNOWN')
        action_counts[action] = action_counts.get(action, 0) + 1

    # Key info
    key_exists = os.path.exists(KEY_FILE)
    key_created = None
    if key_exists:
        key_created = datetime.fromtimestamp(os.path.getctime(KEY_FILE)).isoformat()

    return {
        'total_secrets': total_secrets,
        'total_versions': total_versions,
        'encryption': 'AES-256-GCM',
        'key_exists': key_exists,
        'key_created': key_created,
        'audit_action_counts': action_counts,
        'total_audit_entries': len(audit_logs)
    }

def search_secrets(query: str, search_metadata: bool = True):
    """Search secrets by name or metadata"""
    secrets_data = load_secrets()
    results = []
    query_lower = query.lower()

    for name, data in secrets_data.items():
        matched = False
        match_reason = []

        # Search in name
        if query_lower in name.lower():
            matched = True
            match_reason.append('name')

        # Search in metadata
        if search_metadata:
            for v in data['versions']:
                metadata = v.get('metadata', {})
                for key, value in metadata.items():
                    if query_lower in str(key).lower() or query_lower in str(value).lower():
                        matched = True
                        match_reason.append(f'metadata.{key}')
                        break

        if matched:
            results.append({
                'name': name,
                'current_version': data['current_version'],
                'version_count': len(data['versions']),
                'created_at': data['created_at'],
                'updated_at': data['updated_at'],
                'match_reason': list(set(match_reason))
            })

    log_audit('SEARCH', '*', details=f"Searched for '{query}', found {len(results)} results")
    return results

def secret_exists(name: str):
    """Check if a secret exists"""
    secrets_data = load_secrets()
    return name in secrets_data

def export_secrets_metadata():
    """Export secrets metadata (without values) for backup purposes"""
    secrets_data = load_secrets()
    export_data = {
        'exported_at': datetime.now().isoformat(),
        'total_secrets': len(secrets_data),
        'secrets': []
    }

    for name, data in secrets_data.items():
        secret_meta = {
            'name': name,
            'current_version': data['current_version'],
            'created_at': data['created_at'],
            'updated_at': data['updated_at'],
            'versions': []
        }
        for v in data['versions']:
            secret_meta['versions'].append({
                'version': v['version'],
                'created_at': v['created_at'],
                'metadata': v.get('metadata', {})
            })
        export_data['secrets'].append(secret_meta)

    log_audit('EXPORT', '*', details=f"Exported metadata for {len(secrets_data)} secrets")
    return export_data

def get_key_info():
    """Get information about the master encryption key"""
    key_exists = os.path.exists(KEY_FILE)
    if not key_exists:
        return {
            'exists': False,
            'created_at': None,
            'algorithm': 'AES-256-GCM',
            'key_size': 256
        }

    key_stat = os.stat(KEY_FILE)
    return {
        'exists': True,
        'created_at': datetime.fromtimestamp(key_stat.st_ctime).isoformat(),
        'modified_at': datetime.fromtimestamp(key_stat.st_mtime).isoformat(),
        'algorithm': 'AES-256-GCM',
        'key_size': 256,
        'key_file_permissions': oct(key_stat.st_mode)[-3:]
    }

def copy_secret(source_name: str, dest_name: str, include_all_versions: bool = False):
    """Copy a secret to a new name"""
    with file_lock:
        secrets_data = load_secrets()
        if source_name not in secrets_data:
            raise KeyError(f"Secret '{source_name}' not found")
        if dest_name in secrets_data:
            raise ValueError(f"Secret '{dest_name}' already exists")

        source = secrets_data[source_name]

        if include_all_versions:
            # Copy all versions
            new_secret = {
                'current_version': source['current_version'],
                'versions': [],
                'created_at': datetime.now().isoformat(),
                'updated_at': datetime.now().isoformat()
            }
            for v in source['versions']:
                new_secret['versions'].append({
                    'version': v['version'],
                    'encrypted': v['encrypted'],
                    'created_at': datetime.now().isoformat(),
                    'metadata': {**v.get('metadata', {}), 'copied_from': source_name}
                })
        else:
            # Copy only current version as version 1
            current_version = None
            for v in source['versions']:
                if v['version'] == source['current_version']:
                    current_version = v
                    break

            new_secret = {
                'current_version': 1,
                'versions': [{
                    'version': 1,
                    'encrypted': current_version['encrypted'],
                    'created_at': datetime.now().isoformat(),
                    'metadata': {**current_version.get('metadata', {}), 'copied_from': source_name}
                }],
                'created_at': datetime.now().isoformat(),
                'updated_at': datetime.now().isoformat()
            }

        secrets_data[dest_name] = new_secret
        save_secrets(secrets_data)
        log_audit('COPY', source_name, details=f"Copied to '{dest_name}'")
        return {'source': source_name, 'destination': dest_name, 'versions_copied': len(new_secret['versions'])}

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

# ============ New API Endpoints ============

@app.route('/api/secrets/<name>/versions', methods=['GET'])
def api_list_secret_versions(name):
    """List all versions of a secret"""
    try:
        result = list_secret_versions(name)
        return jsonify(result)
    except KeyError as e:
        return jsonify({'error': str(e)}), 404
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets/<name>/rollback', methods=['POST'])
def api_rollback_secret(name):
    """Rollback a secret to a previous version"""
    try:
        data = request.get_json()
        version = data.get('version')
        if not version:
            return jsonify({'error': 'Version is required'}), 400
        result = rollback_secret(name, version)
        return jsonify({'success': True, 'new_version': result['version']})
    except KeyError as e:
        return jsonify({'error': str(e)}), 404
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets/<name>/metadata', methods=['PATCH'])
def api_update_secret_metadata(name):
    """Update metadata for a secret without changing the value"""
    try:
        data = request.get_json()
        metadata = data.get('metadata', {})
        version = data.get('version')
        if not metadata:
            return jsonify({'error': 'Metadata is required'}), 400
        result = update_secret_metadata(name, metadata, version)
        return jsonify({'success': True, 'result': result})
    except KeyError as e:
        return jsonify({'error': str(e)}), 404
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets/bulk', methods=['POST'])
def api_bulk_create_secrets():
    """Create multiple secrets at once"""
    try:
        data = request.get_json()
        secrets_list = data.get('secrets', [])
        if not secrets_list:
            return jsonify({'error': 'Secrets list is required'}), 400
        result = bulk_create_secrets(secrets_list)
        return jsonify(result)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets/bulk', methods=['DELETE'])
def api_bulk_delete_secrets():
    """Delete multiple secrets at once"""
    try:
        data = request.get_json()
        names = data.get('names', [])
        if not names:
            return jsonify({'error': 'Names list is required'}), 400
        result = bulk_delete_secrets(names)
        return jsonify(result)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/key/rotate', methods=['POST'])
def api_rotate_master_key():
    """Rotate the master encryption key and re-encrypt all secrets"""
    try:
        result = rotate_master_key()
        return jsonify(result)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/key/info', methods=['GET'])
def api_get_key_info():
    """Get information about the master encryption key"""
    try:
        result = get_key_info()
        return jsonify(result)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/stats', methods=['GET'])
def api_get_vault_stats():
    """Get vault statistics"""
    try:
        result = get_vault_stats()
        return jsonify(result)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/search', methods=['GET'])
def api_search_secrets():
    """Search secrets by name or metadata"""
    try:
        query = request.args.get('q', '')
        if not query:
            return jsonify({'error': 'Query parameter q is required'}), 400
        search_metadata = request.args.get('metadata', 'true').lower() == 'true'
        result = search_secrets(query, search_metadata)
        return jsonify(result)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets/<name>/exists', methods=['GET'])
def api_secret_exists(name):
    """Check if a secret exists"""
    try:
        exists = secret_exists(name)
        return jsonify({'name': name, 'exists': exists})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/export', methods=['GET'])
def api_export_secrets_metadata():
    """Export secrets metadata (without values) for backup purposes"""
    try:
        result = export_secrets_metadata()
        return jsonify(result)
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/secrets/<name>/copy', methods=['POST'])
def api_copy_secret(name):
    """Copy a secret to a new name"""
    try:
        data = request.get_json()
        dest_name = data.get('destination')
        if not dest_name:
            return jsonify({'error': 'Destination name is required'}), 400
        include_all_versions = data.get('include_all_versions', False)
        result = copy_secret(name, dest_name, include_all_versions)
        return jsonify({'success': True, 'result': result})
    except KeyError as e:
        return jsonify({'error': str(e)}), 404
    except ValueError as e:
        return jsonify({'error': str(e)}), 400
    except Exception as e:
        return jsonify({'error': str(e)}), 500

if __name__ == '__main__':
    # Ensure data directory exists
    os.makedirs(DATA_DIR, exist_ok=True)
    # Initialize master key on startup
    get_master_key()
    port = int(os.environ.get('PORT', 8080))
    app.run(host='0.0.0.0', port=port)
