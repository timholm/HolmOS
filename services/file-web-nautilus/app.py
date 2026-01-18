from flask import Flask, request, jsonify, send_file, Response
import os
import json
import shutil
import mimetypes
import hashlib
import time
import requests
from datetime import datetime
from pathlib import Path
from functools import wraps
from prometheus_client import Counter, Histogram, Gauge, generate_latest, CONTENT_TYPE_LATEST

app = Flask(__name__)

# Prometheus metrics
REQUEST_COUNT = Counter('nautilus_requests_total', 'Total HTTP requests', ['method', 'endpoint', 'status'])
REQUEST_LATENCY = Histogram('nautilus_request_duration_seconds', 'Request latency', ['method', 'endpoint'])
STORAGE_TOTAL = Gauge('nautilus_storage_bytes_total', 'Total storage bytes')
STORAGE_USED = Gauge('nautilus_storage_bytes_used', 'Used storage bytes')
STORAGE_FREE = Gauge('nautilus_storage_bytes_free', 'Free storage bytes')
FILES_COUNT = Gauge('nautilus_files_total', 'Total number of files')
DIRS_COUNT = Gauge('nautilus_directories_total', 'Total number of directories')

@app.before_request
def before_request():
    request.start_time = time.time()

@app.after_request
def after_request(response):
    if hasattr(request, 'start_time'):
        latency = time.time() - request.start_time
        endpoint = request.endpoint or 'unknown'
        REQUEST_COUNT.labels(method=request.method, endpoint=endpoint, status=response.status_code).inc()
        REQUEST_LATENCY.labels(method=request.method, endpoint=endpoint).observe(latency)
    return response

# Configuration
BASE_PATH = os.environ.get('FILE_BASE_PATH', '/data')
CONFIG_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), '.storage_config.json')

# Default storage configuration
DEFAULT_STORAGE_CONFIG = {
    'basePath': BASE_PATH,
    'storageTarget': {
        'type': 'pvc',  # pvc, storageclass, hostpath
        'name': 'files-pvc',
        'storageClass': 'local-path',
        'hostPath': '/mnt/node13-ssd/files',  # Default to node13's 16TB SSDs
        'node': 'node13'
    },
    'mountPaths': [
        {'name': 'Primary (node13 SSDs)', 'path': '/mnt/node13-ssd/files', 'node': 'node13', 'default': True},
        {'name': 'Secondary Storage', 'path': '/mnt/storage/files', 'node': 'node01', 'default': False},
        {'name': 'Archive', 'path': '/mnt/archive/files', 'node': 'node02', 'default': False},
    ]
}

def load_storage_config():
    """Load storage configuration from file"""
    try:
        if os.path.exists(CONFIG_FILE):
            with open(CONFIG_FILE, 'r') as f:
                config = json.load(f)
                # Merge with defaults to ensure all keys exist
                merged = DEFAULT_STORAGE_CONFIG.copy()
                merged.update(config)
                return merged
    except Exception as e:
        print(f"Error loading storage config: {e}")
    return DEFAULT_STORAGE_CONFIG.copy()

def save_storage_config(config):
    """Save storage configuration to file"""
    try:
        with open(CONFIG_FILE, 'w') as f:
            json.dump(config, f, indent=2)
        return True
    except Exception as e:
        print(f"Error saving storage config: {e}")
        return False

def get_base_path():
    """Get the current base path from config"""
    config = load_storage_config()
    return config.get('basePath', BASE_PATH)

# Initialize paths based on config
def init_paths():
    base = get_base_path()
    return {
        'base': base,
        'trash': os.path.join(base, '.trash'),
        'recent': os.path.join(base, '.recent.json'),
        'favorites': os.path.join(base, '.favorites.json'),
        'thumbnails': os.path.join(base, '.thumbnails'),
        'bookmarks': os.path.join(base, '.bookmarks.json'),
    }

PATHS = init_paths()
TRASH_PATH = PATHS['trash']
RECENT_FILE = PATHS['recent']
FAVORITES_FILE = PATHS['favorites']
THUMBNAIL_CACHE = PATHS['thumbnails']
BOOKMARKS_FILE = PATHS['bookmarks']

# Microservice URLs
SERVICES = {
    'file-list': os.environ.get('FILE_LIST_URL', 'http://file-list.holm.svc.cluster.local:8080'),
    'file-upload': os.environ.get('FILE_UPLOAD_URL', 'http://file-upload.holm.svc.cluster.local:8080'),
    'file-download': os.environ.get('FILE_DOWNLOAD_URL', 'http://file-download.holm.svc.cluster.local:8080'),
    'file-delete': os.environ.get('FILE_DELETE_URL', 'http://file-delete.holm.svc.cluster.local:8080'),
    'file-copy': os.environ.get('FILE_COPY_URL', 'http://file-copy.holm.svc.cluster.local:8080'),
    'file-move': os.environ.get('FILE_MOVE_URL', 'http://file-move.holm.svc.cluster.local:8080'),
    'file-mkdir': os.environ.get('FILE_MKDIR_URL', 'http://file-mkdir.holm.svc.cluster.local:8080'),
    'file-search': os.environ.get('FILE_SEARCH_URL', 'http://file-search.holm.svc.cluster.local:8080'),
    'file-preview': os.environ.get('FILE_PREVIEW_URL', 'http://file-preview.holm.svc.cluster.local:8080'),
    'file-thumbnail': os.environ.get('FILE_THUMBNAIL_URL', 'http://file-thumbnail.holm.svc.cluster.local:8080'),
    'file-compress': os.environ.get('FILE_COMPRESS_URL', 'http://file-compress.holm.svc.cluster.local:8080'),
    'file-decompress': os.environ.get('FILE_DECOMPRESS_URL', 'http://file-decompress.holm.svc.cluster.local:8080'),
    'file-encrypt': os.environ.get('FILE_ENCRYPT_URL', 'http://file-encrypt.holm.svc.cluster.local:80'),
    'file-share': os.environ.get('FILE_SHARE_URL', 'http://file-share.holm.svc.cluster.local:80'),
    'file-meta': os.environ.get('FILE_META_URL', 'http://file-meta.holm.svc.cluster.local:8080'),
    'file-convert': os.environ.get('FILE_CONVERT_URL', 'http://file-convert.holm.svc.cluster.local:80'),
}

# Ensure directories exist
os.makedirs(TRASH_PATH, exist_ok=True)
os.makedirs(THUMBNAIL_CACHE, exist_ok=True)

# Default locations
DEFAULT_LOCATIONS = [
    {'name': 'Home', 'path': '/', 'icon': 'home', 'type': 'location'},
    {'name': 'Documents', 'path': '/documents', 'icon': 'file-text', 'type': 'location'},
    {'name': 'Downloads', 'path': '/downloads', 'icon': 'download', 'type': 'location'},
    {'name': 'Music', 'path': '/music', 'icon': 'music', 'type': 'location'},
    {'name': 'Pictures', 'path': '/pictures', 'icon': 'image', 'type': 'location'},
    {'name': 'Videos', 'path': '/videos', 'icon': 'film', 'type': 'location'},
]

def safe_path(path):
    base = get_base_path()
    if not path:
        return base
    abs_path = os.path.abspath(os.path.join(base, path.lstrip('/')))
    if not abs_path.startswith(base):
        return None
    return abs_path

def get_file_info(filepath):
    base = get_base_path()
    try:
        stat = os.stat(filepath)
        name = os.path.basename(filepath)
        is_dir = os.path.isdir(filepath)
        mime_type = 'directory' if is_dir else (mimetypes.guess_type(filepath)[0] or 'application/octet-stream')
        icon = get_icon_for_type(mime_type, name, is_dir)

        return {
            'name': name,
            'path': filepath.replace(base, '') or '/',
            'isDir': is_dir,
            'size': stat.st_size if not is_dir else get_dir_size(filepath),
            'modified': datetime.fromtimestamp(stat.st_mtime).isoformat(),
            'created': datetime.fromtimestamp(stat.st_ctime).isoformat(),
            'accessed': datetime.fromtimestamp(stat.st_atime).isoformat(),
            'mimeType': mime_type,
            'icon': icon,
            'permissions': oct(stat.st_mode)[-3:],
            'hidden': name.startswith('.'),
            'extension': os.path.splitext(name)[1].lower() if not is_dir else None,
            'uid': stat.st_uid,
            'gid': stat.st_gid,
        }
    except Exception as e:
        return None

def get_dir_size(path):
    total = 0
    try:
        for entry in os.scandir(path):
            if entry.is_file():
                total += entry.stat().st_size
            elif entry.is_dir():
                total += get_dir_size(entry.path)
    except:
        pass
    return total

def get_icon_for_type(mime_type, name, is_dir):
    if is_dir:
        folder_icons = {
            'documents': 'file-text', 'downloads': 'download', 'music': 'music',
            'pictures': 'image', 'photos': 'image', 'videos': 'film',
            'desktop': 'monitor', 'projects': 'briefcase', '.trash': 'trash-2',
            'home': 'home', '.git': 'git-branch', 'node_modules': 'package',
        }
        return folder_icons.get(name.lower(), 'folder')

    ext_icons = {
        '.pdf': 'file-text', '.doc': 'file-text', '.docx': 'file-text',
        '.xls': 'table', '.xlsx': 'table', '.csv': 'table',
        '.ppt': 'monitor', '.pptx': 'monitor',
        '.zip': 'archive', '.tar': 'archive', '.gz': 'archive', '.rar': 'archive', '.7z': 'archive',
        '.py': 'code', '.js': 'code', '.ts': 'code', '.jsx': 'code', '.tsx': 'code',
        '.html': 'code', '.css': 'code', '.json': 'code', '.xml': 'code',
        '.sh': 'terminal', '.bash': 'terminal',
        '.md': 'book-open', '.txt': 'file-text',
        '.mp3': 'music', '.wav': 'music', '.flac': 'music', '.m4a': 'music', '.ogg': 'music',
        '.mp4': 'film', '.mkv': 'film', '.avi': 'film', '.mov': 'film', '.webm': 'film',
        '.jpg': 'image', '.jpeg': 'image', '.png': 'image', '.gif': 'image', 
        '.svg': 'image', '.webp': 'image', '.bmp': 'image',
        '.iso': 'disc', '.dmg': 'disc',
        '.exe': 'box', '.app': 'box', '.deb': 'box', '.rpm': 'box',
    }

    ext = os.path.splitext(name)[1].lower()
    if ext in ext_icons:
        return ext_icons[ext]

    if mime_type.startswith('image/'):
        return 'image'
    elif mime_type.startswith('video/'):
        return 'film'
    elif mime_type.startswith('audio/'):
        return 'music'
    elif mime_type.startswith('text/'):
        return 'file-text'
    
    return 'file'

def add_to_recent(filepath):
    base = get_base_path()
    recent_file = os.path.join(base, '.recent.json')
    try:
        recent = []
        if os.path.exists(recent_file):
            with open(recent_file, 'r') as f:
                recent = json.load(f)

        recent = [r for r in recent if r['path'] != filepath]
        recent.insert(0, {
            'path': filepath.replace(base, ''),
            'timestamp': datetime.now().isoformat()
        })
        recent = recent[:100]

        with open(recent_file, 'w') as f:
            json.dump(recent, f)
    except:
        pass

def format_size(size):
    for unit in ['B', 'KB', 'MB', 'GB', 'TB']:
        if size < 1024:
            return f"{size:.1f} {unit}"
        size /= 1024
    return f"{size:.1f} PB"

@app.route('/health')
def health():
    return jsonify({'status': 'healthy', 'service': 'file-web-nautilus-v3'})

@app.route('/metrics')
def metrics():
    """Prometheus metrics endpoint"""
    # Update storage metrics
    try:
        base = get_base_path()
        stat = os.statvfs(base)
        total = stat.f_blocks * stat.f_frsize
        free = stat.f_bfree * stat.f_frsize
        used = total - free
        STORAGE_TOTAL.set(total)
        STORAGE_USED.set(used)
        STORAGE_FREE.set(free)

        # Count files and directories
        files = 0
        dirs = 0
        for root, dirnames, filenames in os.walk(base):
            files += len(filenames)
            dirs += len(dirnames)
            if files + dirs > 10000:  # Limit scan for performance
                break
        FILES_COUNT.set(files)
        DIRS_COUNT.set(dirs)
    except:
        pass

    return Response(generate_latest(), mimetype=CONTENT_TYPE_LATEST)

@app.route('/api/list')
def list_directory():
    path = request.args.get('path', '/')
    show_hidden = request.args.get('hidden', 'false').lower() == 'true'
    sort_by = request.args.get('sort', 'name')
    sort_order = request.args.get('order', 'asc')

    abs_path = safe_path(path)
    if not abs_path or not os.path.exists(abs_path):
        return jsonify({'error': 'Path not found'}), 404

    if not os.path.isdir(abs_path):
        return jsonify({'error': 'Not a directory'}), 400

    items = []
    try:
        for entry in os.scandir(abs_path):
            if not show_hidden and entry.name.startswith('.'):
                continue
            info = get_file_info(entry.path)
            if info:
                items.append(info)
    except PermissionError:
        return jsonify({'error': 'Permission denied'}), 403

    reverse = sort_order == 'desc'
    if sort_by == 'name':
        items.sort(key=lambda x: (not x['isDir'], x['name'].lower()), reverse=reverse)
    elif sort_by == 'size':
        items.sort(key=lambda x: (not x['isDir'], x['size']), reverse=reverse)
    elif sort_by == 'modified':
        items.sort(key=lambda x: (not x['isDir'], x['modified']), reverse=reverse)
    elif sort_by == 'type':
        items.sort(key=lambda x: (not x['isDir'], x['extension'] or ''), reverse=reverse)

    parent = os.path.dirname(abs_path)
    # Return '/' for root (no parent), never null
    if parent.startswith(BASE_PATH):
        parent_path = parent.replace(BASE_PATH, '') or '/'
    else:
        parent_path = '/'  # At root level, parent is self

    breadcrumbs = [{'name': 'Home', 'path': '/'}]
    current = ''
    for part in path.strip('/').split('/'):
        if part:
            current += '/' + part
            breadcrumbs.append({'name': part, 'path': current})

    return jsonify({
        'path': path,
        'parent': parent_path,
        'breadcrumbs': breadcrumbs,
        'items': items,
        'count': len(items),
        'dirInfo': get_file_info(abs_path)
    })

@app.route('/api/file')
def get_file():
    path = request.args.get('path', '')
    download = request.args.get('download', 'false').lower() == 'true'

    abs_path = safe_path(path)
    if not abs_path or not os.path.exists(abs_path):
        return jsonify({'error': 'File not found'}), 404

    if os.path.isdir(abs_path):
        return jsonify({'error': 'Is a directory'}), 400

    add_to_recent(abs_path)
    mime_type = mimetypes.guess_type(abs_path)[0] or 'application/octet-stream'

    if download:
        return send_file(abs_path, as_attachment=True, download_name=os.path.basename(abs_path))

    return send_file(abs_path, mimetype=mime_type)

@app.route('/api/upload', methods=['POST'])
def upload_file():
    path = request.form.get('path', '/')

    abs_path = safe_path(path)
    if not abs_path:
        return jsonify({'error': 'Invalid path'}), 400

    if not os.path.exists(abs_path):
        os.makedirs(abs_path, exist_ok=True)

    uploaded = []
    for key in request.files:
        file = request.files[key]
        if file.filename:
            filename = os.path.basename(file.filename)
            filepath = os.path.join(abs_path, filename)

            counter = 1
            base, ext = os.path.splitext(filename)
            while os.path.exists(filepath):
                filename = f"{base} ({counter}){ext}"
                filepath = os.path.join(abs_path, filename)
                counter += 1

            file.save(filepath)
            uploaded.append(get_file_info(filepath))

    return jsonify({'uploaded': uploaded, 'count': len(uploaded)})

@app.route('/api/mkdir', methods=['POST'])
def create_directory():
    data = request.get_json()
    path = data.get('path', '/')
    name = data.get('name', 'New Folder')

    abs_path = safe_path(path)
    if not abs_path:
        return jsonify({'error': 'Invalid path'}), 400

    new_dir = os.path.join(abs_path, name)

    counter = 1
    base_dir = new_dir
    while os.path.exists(new_dir):
        new_dir = f"{base_dir} ({counter})"
        counter += 1

    try:
        os.makedirs(new_dir)
        return jsonify(get_file_info(new_dir))
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/move', methods=['POST'])
def move_item():
    data = request.get_json()
    source = data.get('source')
    destination = data.get('destination')

    src_path = safe_path(source)
    dst_path = safe_path(destination)

    if not src_path or not dst_path:
        return jsonify({'error': 'Invalid path'}), 400

    if not os.path.exists(src_path):
        return jsonify({'error': 'Source not found'}), 404

    try:
        if os.path.isdir(dst_path):
            dst_path = os.path.join(dst_path, os.path.basename(src_path))

        shutil.move(src_path, dst_path)
        return jsonify(get_file_info(dst_path))
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/copy', methods=['POST'])
def copy_item():
    data = request.get_json()
    source = data.get('source')
    destination = data.get('destination')

    src_path = safe_path(source)
    dst_path = safe_path(destination)

    if not src_path or not dst_path:
        return jsonify({'error': 'Invalid path'}), 400

    if not os.path.exists(src_path):
        return jsonify({'error': 'Source not found'}), 404

    try:
        if os.path.isdir(dst_path):
            dst_path = os.path.join(dst_path, os.path.basename(src_path))

        counter = 1
        base_path = dst_path
        name, ext = os.path.splitext(base_path)
        while os.path.exists(dst_path):
            dst_path = f"{name} (copy {counter}){ext}"
            counter += 1

        if os.path.isdir(src_path):
            shutil.copytree(src_path, dst_path)
        else:
            shutil.copy2(src_path, dst_path)

        return jsonify(get_file_info(dst_path))
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/rename', methods=['POST'])
def rename_item():
    data = request.get_json()
    path = data.get('path')
    new_name = data.get('name')

    abs_path = safe_path(path)
    if not abs_path or not os.path.exists(abs_path):
        return jsonify({'error': 'Path not found'}), 404

    new_path = os.path.join(os.path.dirname(abs_path), new_name)

    if os.path.exists(new_path):
        return jsonify({'error': 'Name already exists'}), 400

    try:
        os.rename(abs_path, new_path)
        return jsonify(get_file_info(new_path))
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/delete', methods=['DELETE'])
def delete_item():
    path = request.args.get('path')
    permanent = request.args.get('permanent', 'false').lower() == 'true'

    abs_path = safe_path(path)
    if not abs_path or not os.path.exists(abs_path):
        return jsonify({'error': 'Path not found'}), 404

    try:
        if permanent:
            if os.path.isdir(abs_path):
                shutil.rmtree(abs_path)
            else:
                os.remove(abs_path)
        else:
            trash_name = f"{int(time.time())}_{os.path.basename(abs_path)}"
            trash_path = os.path.join(TRASH_PATH, trash_name)

            info_file = os.path.join(TRASH_PATH, f"{trash_name}.info")
            with open(info_file, 'w') as f:
                json.dump({
                    'originalPath': abs_path.replace(BASE_PATH, ''),
                    'deletedAt': datetime.now().isoformat()
                }, f)

            shutil.move(abs_path, trash_path)

        return jsonify({'deleted': path})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/delete-multiple', methods=['POST'])
def delete_multiple():
    data = request.get_json()
    paths = data.get('paths', [])
    permanent = data.get('permanent', False)
    
    deleted = []
    errors = []
    
    for path in paths:
        abs_path = safe_path(path)
        if not abs_path or not os.path.exists(abs_path):
            errors.append({'path': path, 'error': 'Not found'})
            continue
            
        try:
            if permanent:
                if os.path.isdir(abs_path):
                    shutil.rmtree(abs_path)
                else:
                    os.remove(abs_path)
            else:
                trash_name = f"{int(time.time())}_{os.path.basename(abs_path)}"
                trash_path = os.path.join(TRASH_PATH, trash_name)
                
                info_file = os.path.join(TRASH_PATH, f"{trash_name}.info")
                with open(info_file, 'w') as f:
                    json.dump({
                        'originalPath': abs_path.replace(BASE_PATH, ''),
                        'deletedAt': datetime.now().isoformat()
                    }, f)
                    
                shutil.move(abs_path, trash_path)
            deleted.append(path)
        except Exception as e:
            errors.append({'path': path, 'error': str(e)})
    
    return jsonify({'deleted': deleted, 'errors': errors})

@app.route('/api/search')
def search_files():
    query = request.args.get('q', '')
    path = request.args.get('path', '/')
    file_type = request.args.get('type', '')
    max_results = int(request.args.get('limit', 100))

    if not query:
        return jsonify({'error': 'Query required'}), 400

    abs_path = safe_path(path)
    if not abs_path:
        abs_path = BASE_PATH

    results = []
    query_lower = query.lower()

    def search_dir(dir_path, depth=0):
        if depth > 10 or len(results) >= max_results:
            return
        try:
            for entry in os.scandir(dir_path):
                if entry.name.startswith('.'):
                    continue
                if query_lower in entry.name.lower():
                    info = get_file_info(entry.path)
                    if info:
                        if file_type:
                            if file_type == 'folder' and not info['isDir']:
                                continue
                            elif file_type == 'image' and not info['mimeType'].startswith('image/'):
                                continue
                            elif file_type == 'document' and not any(t in info['mimeType'] for t in ['text/', 'pdf', 'document', 'word']):
                                continue
                            elif file_type == 'video' and not info['mimeType'].startswith('video/'):
                                continue
                            elif file_type == 'audio' and not info['mimeType'].startswith('audio/'):
                                continue
                        results.append(info)
                if entry.is_dir() and len(results) < max_results:
                    search_dir(entry.path, depth + 1)
        except:
            pass

    search_dir(abs_path)

    return jsonify({
        'query': query,
        'results': results,
        'count': len(results)
    })

@app.route('/api/recent')
def get_recent():
    try:
        if os.path.exists(RECENT_FILE):
            with open(RECENT_FILE, 'r') as f:
                recent = json.load(f)

            items = []
            for r in recent:
                abs_path = safe_path(r['path'])
                if abs_path and os.path.exists(abs_path):
                    info = get_file_info(abs_path)
                    if info:
                        info['accessedRecent'] = r['timestamp']
                        items.append(info)

            return jsonify({'items': items[:50], 'count': len(items)})
        return jsonify({'items': [], 'count': 0})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/locations')
def get_locations():
    locations = DEFAULT_LOCATIONS.copy()
    
    # Check which locations exist and add info
    for loc in locations:
        abs_path = safe_path(loc['path'])
        if abs_path and os.path.exists(abs_path):
            loc['exists'] = True
        else:
            loc['exists'] = False
    
    return jsonify({'locations': locations})

@app.route('/api/bookmarks', methods=['GET'])
def get_bookmarks():
    try:
        if os.path.exists(BOOKMARKS_FILE):
            with open(BOOKMARKS_FILE, 'r') as f:
                bookmarks = json.load(f)
                return jsonify({'bookmarks': bookmarks})
        return jsonify({'bookmarks': []})
    except:
        return jsonify({'bookmarks': []})

@app.route('/api/bookmarks', methods=['POST'])
def add_bookmark():
    data = request.get_json()
    path = data.get('path')
    name = data.get('name')

    abs_path = safe_path(path)
    if not abs_path or not os.path.isdir(abs_path):
        return jsonify({'error': 'Invalid directory'}), 400

    try:
        bookmarks = []
        if os.path.exists(BOOKMARKS_FILE):
            with open(BOOKMARKS_FILE, 'r') as f:
                bookmarks = json.load(f)

        if not any(b['path'] == path for b in bookmarks):
            bookmarks.append({
                'name': name or os.path.basename(path) or 'Home',
                'path': path,
                'icon': 'star'
            })

        with open(BOOKMARKS_FILE, 'w') as f:
            json.dump(bookmarks, f)

        return jsonify({'bookmarks': bookmarks})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/bookmarks', methods=['DELETE'])
def remove_bookmark():
    path = request.args.get('path')

    try:
        if os.path.exists(BOOKMARKS_FILE):
            with open(BOOKMARKS_FILE, 'r') as f:
                bookmarks = json.load(f)

            bookmarks = [b for b in bookmarks if b['path'] != path]

            with open(BOOKMARKS_FILE, 'w') as f:
                json.dump(bookmarks, f)

            return jsonify({'bookmarks': bookmarks})
        return jsonify({'bookmarks': []})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/trash')
def list_trash():
    items = []
    try:
        for entry in os.scandir(TRASH_PATH):
            if entry.name.endswith('.info'):
                continue

            info = get_file_info(entry.path)
            if info:
                info_file = entry.path + '.info'
                if os.path.exists(info_file):
                    with open(info_file, 'r') as f:
                        trash_info = json.load(f)
                        info['originalPath'] = trash_info.get('originalPath', '')
                        info['deletedAt'] = trash_info.get('deletedAt', '')

                items.append(info)
    except:
        pass

    items.sort(key=lambda x: x.get('deletedAt', ''), reverse=True)
    return jsonify({'items': items, 'count': len(items)})

@app.route('/api/trash/restore', methods=['POST'])
def restore_from_trash():
    data = request.get_json()
    path = data.get('path')

    trash_path = os.path.join(TRASH_PATH, os.path.basename(path))
    if not os.path.exists(trash_path):
        return jsonify({'error': 'Item not found in trash'}), 404

    info_file = trash_path + '.info'
    if os.path.exists(info_file):
        with open(info_file, 'r') as f:
            trash_info = json.load(f)
            original_path = safe_path(trash_info.get('originalPath', ''))
    else:
        name = os.path.basename(path).split('_', 1)[-1]
        original_path = os.path.join(BASE_PATH, name)

    if not original_path:
        return jsonify({'error': 'Cannot determine restore location'}), 400

    try:
        os.makedirs(os.path.dirname(original_path), exist_ok=True)

        counter = 1
        base_path = original_path
        name, ext = os.path.splitext(base_path)
        while os.path.exists(original_path):
            original_path = f"{name} (restored {counter}){ext}"
            counter += 1

        shutil.move(trash_path, original_path)

        if os.path.exists(info_file):
            os.remove(info_file)

        return jsonify(get_file_info(original_path))
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/trash/empty', methods=['DELETE'])
def empty_trash():
    try:
        for entry in os.scandir(TRASH_PATH):
            if entry.is_dir():
                shutil.rmtree(entry.path)
            else:
                os.remove(entry.path)
        return jsonify({'message': 'Trash emptied'})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/preview')
def get_preview():
    path = request.args.get('path', '')

    abs_path = safe_path(path)
    if not abs_path or not os.path.exists(abs_path):
        return jsonify({'error': 'File not found'}), 404

    info = get_file_info(abs_path)
    mime_type = info['mimeType']

    preview = {
        'info': info,
        'content': None,
        'contentType': None
    }

    if mime_type.startswith('text/') or mime_type in ['application/json', 'application/xml', 'application/javascript', 'application/x-python']:
        try:
            with open(abs_path, 'r', encoding='utf-8', errors='replace') as f:
                content = f.read(100000)
                preview['content'] = content
                preview['contentType'] = 'text'
        except:
            pass

    elif mime_type.startswith('image/'):
        preview['contentType'] = 'image'
        preview['content'] = f'/api/thumbnail?path={path}&size=400'

    elif mime_type.startswith('video/'):
        preview['contentType'] = 'video'

    elif 'pdf' in mime_type:
        preview['contentType'] = 'pdf'

    return jsonify(preview)

@app.route('/api/thumbnail')
def get_thumbnail():
    path = request.args.get('path', '')
    size = int(request.args.get('size', 200))

    abs_path = safe_path(path)
    if not abs_path or not os.path.exists(abs_path):
        return jsonify({'error': 'File not found'}), 404

    cache_key = hashlib.md5(f"{path}_{size}_{os.path.getmtime(abs_path)}".encode()).hexdigest()
    cache_path = os.path.join(THUMBNAIL_CACHE, f"{cache_key}.jpg")

    if os.path.exists(cache_path):
        return send_file(cache_path, mimetype='image/jpeg')

    try:
        from PIL import Image
        with Image.open(abs_path) as img:
            if img.mode in ('RGBA', 'LA', 'P'):
                img = img.convert('RGB')

            img.thumbnail((size, size), Image.Resampling.LANCZOS)
            img.save(cache_path, 'JPEG', quality=85)

            return send_file(cache_path, mimetype='image/jpeg')
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/info')
def get_item_info():
    path = request.args.get('path', '')

    abs_path = safe_path(path)
    if not abs_path or not os.path.exists(abs_path):
        return jsonify({'error': 'Path not found'}), 404

    info = get_file_info(abs_path)

    if info['isDir']:
        files = 0
        folders = 0
        try:
            for entry in os.scandir(abs_path):
                if entry.is_dir():
                    folders += 1
                else:
                    files += 1
        except:
            pass
        info['contents'] = {'files': files, 'folders': folders}

    info['sizeFormatted'] = format_size(info['size'])

    return jsonify(info)

@app.route('/api/compress', methods=['POST'])
def compress_files():
    data = request.get_json()
    paths = data.get('paths', [])
    archive_name = data.get('name', 'archive.zip')
    destination = data.get('destination', '/')

    abs_dest = safe_path(destination)
    if not abs_dest:
        return jsonify({'error': 'Invalid destination'}), 400

    archive_path = os.path.join(abs_dest, archive_name)

    try:
        import zipfile
        with zipfile.ZipFile(archive_path, 'w', zipfile.ZIP_DEFLATED) as zipf:
            for p in paths:
                abs_p = safe_path(p)
                if abs_p and os.path.exists(abs_p):
                    if os.path.isdir(abs_p):
                        for root, dirs, files in os.walk(abs_p):
                            for file in files:
                                file_path = os.path.join(root, file)
                                arcname = os.path.relpath(file_path, os.path.dirname(abs_p))
                                zipf.write(file_path, arcname)
                    else:
                        zipf.write(abs_p, os.path.basename(abs_p))

        return jsonify(get_file_info(archive_path))
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/decompress', methods=['POST'])
def decompress_file():
    data = request.get_json()
    path = data.get('path')
    destination = data.get('destination', '/')

    abs_path = safe_path(path)
    abs_dest = safe_path(destination)

    if not abs_path or not abs_dest:
        return jsonify({'error': 'Invalid path'}), 400

    try:
        import zipfile
        with zipfile.ZipFile(abs_path, 'r') as zipf:
            zipf.extractall(abs_dest)

        return jsonify({'message': 'Extracted successfully', 'destination': destination})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/services')
def get_services():
    status = {}
    for name, url in SERVICES.items():
        try:
            resp = requests.get(f"{url}/health", timeout=2)
            status[name] = resp.status_code == 200
        except:
            status[name] = False
    return jsonify(status)

@app.route('/api/stats')
def get_stats():
    """Get storage statistics"""
    try:
        base = get_base_path()
        stat = os.statvfs(base)
        total = stat.f_blocks * stat.f_frsize
        free = stat.f_bfree * stat.f_frsize
        used = total - free

        config = load_storage_config()

        return jsonify({
            'total': total,
            'used': used,
            'free': free,
            'totalFormatted': format_size(total),
            'usedFormatted': format_size(used),
            'freeFormatted': format_size(free),
            'percentUsed': round((used / total) * 100, 1) if total > 0 else 0,
            'basePath': base,
            'storageTarget': config.get('storageTarget', {}),
            'currentMount': config.get('storageTarget', {}).get('hostPath', base)
        })
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/storage/config', methods=['GET'])
def get_storage_config():
    """Get storage configuration"""
    try:
        config = load_storage_config()
        base = get_base_path()

        # Get storage stats for current path
        try:
            stat = os.statvfs(base)
            total = stat.f_blocks * stat.f_frsize
            free = stat.f_bfree * stat.f_frsize
            used = total - free
            stats = {
                'total': total,
                'used': used,
                'free': free,
                'totalFormatted': format_size(total),
                'usedFormatted': format_size(used),
                'freeFormatted': format_size(free),
                'percentUsed': round((used / total) * 100, 1) if total > 0 else 0
            }
        except:
            stats = None

        return jsonify({
            'config': config,
            'stats': stats,
            'basePath': base
        })
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/storage/config', methods=['POST'])
def update_storage_config():
    """Update storage configuration"""
    try:
        data = request.get_json()
        config = load_storage_config()

        # Update storage target
        if 'storageTarget' in data:
            config['storageTarget'] = data['storageTarget']

        # Update base path
        if 'basePath' in data:
            new_path = data['basePath']
            # Validate path exists or can be created
            if not os.path.exists(new_path):
                try:
                    os.makedirs(new_path, exist_ok=True)
                except Exception as e:
                    return jsonify({'error': f'Cannot create path: {e}'}), 400
            config['basePath'] = new_path

        # Update mount paths
        if 'mountPaths' in data:
            config['mountPaths'] = data['mountPaths']

        if save_storage_config(config):
            # Reinitialize paths
            global PATHS, TRASH_PATH, RECENT_FILE, FAVORITES_FILE, THUMBNAIL_CACHE, BOOKMARKS_FILE
            PATHS = init_paths()
            TRASH_PATH = PATHS['trash']
            RECENT_FILE = PATHS['recent']
            FAVORITES_FILE = PATHS['favorites']
            THUMBNAIL_CACHE = PATHS['thumbnails']
            BOOKMARKS_FILE = PATHS['bookmarks']

            # Ensure directories exist
            os.makedirs(TRASH_PATH, exist_ok=True)
            os.makedirs(THUMBNAIL_CACHE, exist_ok=True)

            return jsonify({'success': True, 'config': config})
        else:
            return jsonify({'error': 'Failed to save configuration'}), 500
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/storage/mounts', methods=['GET'])
def get_available_mounts():
    """Get available mount points"""
    try:
        config = load_storage_config()
        mounts = config.get('mountPaths', [])

        # Check which mounts are accessible
        for mount in mounts:
            path = mount.get('path', '')
            mount['accessible'] = os.path.exists(path) and os.access(path, os.R_OK)
            if mount['accessible']:
                try:
                    stat = os.statvfs(path)
                    total = stat.f_blocks * stat.f_frsize
                    free = stat.f_bfree * stat.f_frsize
                    used = total - free
                    mount['stats'] = {
                        'total': total,
                        'used': used,
                        'free': free,
                        'totalFormatted': format_size(total),
                        'usedFormatted': format_size(used),
                        'freeFormatted': format_size(free),
                        'percentUsed': round((used / total) * 100, 1) if total > 0 else 0
                    }
                except:
                    mount['stats'] = None
            else:
                mount['stats'] = None

        return jsonify({
            'mounts': mounts,
            'currentPath': get_base_path()
        })
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/storage/mounts', methods=['POST'])
def add_mount_path():
    """Add a new mount path"""
    try:
        data = request.get_json()
        name = data.get('name')
        path = data.get('path')
        node = data.get('node', '')

        if not name or not path:
            return jsonify({'error': 'Name and path are required'}), 400

        config = load_storage_config()
        mounts = config.get('mountPaths', [])

        # Check if path already exists
        if any(m['path'] == path for m in mounts):
            return jsonify({'error': 'Mount path already exists'}), 400

        mounts.append({
            'name': name,
            'path': path,
            'node': node,
            'default': False
        })

        config['mountPaths'] = mounts

        if save_storage_config(config):
            return jsonify({'success': True, 'mounts': mounts})
        else:
            return jsonify({'error': 'Failed to save configuration'}), 500
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/storage/mounts', methods=['DELETE'])
def remove_mount_path():
    """Remove a mount path"""
    try:
        path = request.args.get('path')
        if not path:
            return jsonify({'error': 'Path is required'}), 400

        config = load_storage_config()
        mounts = config.get('mountPaths', [])

        # Don't allow removing the default mount
        mount = next((m for m in mounts if m['path'] == path), None)
        if mount and mount.get('default'):
            return jsonify({'error': 'Cannot remove default mount path'}), 400

        config['mountPaths'] = [m for m in mounts if m['path'] != path]

        if save_storage_config(config):
            return jsonify({'success': True, 'mounts': config['mountPaths']})
        else:
            return jsonify({'error': 'Failed to save configuration'}), 500
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/api/storage/switch', methods=['POST'])
def switch_mount():
    """Switch to a different mount path"""
    try:
        data = request.get_json()
        path = data.get('path')

        if not path:
            return jsonify({'error': 'Path is required'}), 400

        # Verify path exists
        if not os.path.exists(path):
            return jsonify({'error': 'Path does not exist'}), 400

        config = load_storage_config()
        config['basePath'] = path

        # Update storage target
        config['storageTarget']['hostPath'] = path

        # Update default in mount paths
        for mount in config.get('mountPaths', []):
            mount['default'] = mount['path'] == path

        if save_storage_config(config):
            # Reinitialize paths
            global PATHS, TRASH_PATH, RECENT_FILE, FAVORITES_FILE, THUMBNAIL_CACHE, BOOKMARKS_FILE
            PATHS = init_paths()
            TRASH_PATH = PATHS['trash']
            RECENT_FILE = PATHS['recent']
            FAVORITES_FILE = PATHS['favorites']
            THUMBNAIL_CACHE = PATHS['thumbnails']
            BOOKMARKS_FILE = PATHS['bookmarks']

            # Ensure directories exist
            os.makedirs(TRASH_PATH, exist_ok=True)
            os.makedirs(THUMBNAIL_CACHE, exist_ok=True)

            return jsonify({'success': True, 'basePath': path})
        else:
            return jsonify({'error': 'Failed to save configuration'}), 500
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/')
def index():
    return send_file('static/index.html')

@app.route('/<path:path>')
def static_files(path):
    static_path = os.path.join('static', path)
    if os.path.exists(static_path):
        return send_file(static_path)
    return send_file('static/index.html')

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000, debug=False)
