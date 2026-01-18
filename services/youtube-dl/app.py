#!/usr/bin/env python3
"""YouTube Video Downloader - Simple web interface for yt-dlp"""

import os
import re
import json
import subprocess
import threading
import uuid
from pathlib import Path
from flask import Flask, request, jsonify, send_file, render_template_string

app = Flask(__name__)

DOWNLOAD_DIR = Path("/tmp/youtube-downloads")
DOWNLOAD_DIR.mkdir(exist_ok=True)

# Track download progress
downloads = {}

HTML_TEMPLATE = '''
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>YouTube Downloader</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .container {
            background: rgba(255, 255, 255, 0.05);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            padding: 40px;
            max-width: 600px;
            width: 100%;
            box-shadow: 0 25px 50px rgba(0, 0, 0, 0.3);
            border: 1px solid rgba(255, 255, 255, 0.1);
        }
        h1 {
            color: #fff;
            text-align: center;
            margin-bottom: 10px;
            font-size: 2em;
        }
        .subtitle {
            color: #888;
            text-align: center;
            margin-bottom: 30px;
            font-size: 0.9em;
        }
        .input-group {
            margin-bottom: 20px;
        }
        label {
            display: block;
            color: #ccc;
            margin-bottom: 8px;
            font-size: 0.9em;
        }
        input[type="text"] {
            width: 100%;
            padding: 15px 20px;
            border: 2px solid rgba(255, 255, 255, 0.1);
            border-radius: 12px;
            background: rgba(0, 0, 0, 0.3);
            color: #fff;
            font-size: 16px;
            transition: all 0.3s;
        }
        input[type="text"]:focus {
            outline: none;
            border-color: #ff0000;
            box-shadow: 0 0 20px rgba(255, 0, 0, 0.2);
        }
        input[type="text"]::placeholder {
            color: #666;
        }
        .format-options {
            display: flex;
            gap: 10px;
            margin-bottom: 25px;
        }
        .format-btn {
            flex: 1;
            padding: 12px 20px;
            border: 2px solid rgba(255, 255, 255, 0.2);
            border-radius: 10px;
            background: transparent;
            color: #fff;
            cursor: pointer;
            transition: all 0.3s;
            font-size: 14px;
        }
        .format-btn:hover {
            border-color: #ff0000;
            background: rgba(255, 0, 0, 0.1);
        }
        .format-btn.active {
            border-color: #ff0000;
            background: rgba(255, 0, 0, 0.2);
        }
        .download-btn {
            width: 100%;
            padding: 18px;
            border: none;
            border-radius: 12px;
            background: linear-gradient(135deg, #ff0000, #cc0000);
            color: #fff;
            font-size: 18px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s;
        }
        .download-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 10px 30px rgba(255, 0, 0, 0.3);
        }
        .download-btn:disabled {
            background: #444;
            cursor: not-allowed;
            transform: none;
            box-shadow: none;
        }
        .status {
            margin-top: 20px;
            padding: 15px;
            border-radius: 10px;
            text-align: center;
            display: none;
        }
        .status.loading {
            display: block;
            background: rgba(255, 193, 7, 0.1);
            border: 1px solid rgba(255, 193, 7, 0.3);
            color: #ffc107;
        }
        .status.success {
            display: block;
            background: rgba(40, 167, 69, 0.1);
            border: 1px solid rgba(40, 167, 69, 0.3);
            color: #28a745;
        }
        .status.error {
            display: block;
            background: rgba(220, 53, 69, 0.1);
            border: 1px solid rgba(220, 53, 69, 0.3);
            color: #dc3545;
        }
        .progress-bar {
            width: 100%;
            height: 6px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 3px;
            margin-top: 10px;
            overflow: hidden;
        }
        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, #ff0000, #ff6b6b);
            width: 0%;
            transition: width 0.3s;
        }
        .download-link {
            display: inline-block;
            margin-top: 15px;
            padding: 12px 30px;
            background: #28a745;
            color: #fff;
            text-decoration: none;
            border-radius: 8px;
            font-weight: 500;
            transition: all 0.3s;
        }
        .download-link:hover {
            background: #218838;
            transform: translateY(-2px);
        }
        .info {
            margin-top: 30px;
            padding: 15px;
            background: rgba(255, 255, 255, 0.05);
            border-radius: 10px;
            color: #888;
            font-size: 0.85em;
        }
        .info ul {
            margin-left: 20px;
            margin-top: 10px;
        }
        .info li {
            margin-bottom: 5px;
        }
        .video-info {
            margin-top: 20px;
            padding: 15px;
            background: rgba(255, 255, 255, 0.05);
            border-radius: 10px;
            display: none;
        }
        .video-info img {
            width: 100%;
            border-radius: 8px;
            margin-bottom: 10px;
        }
        .video-info h3 {
            color: #fff;
            font-size: 1em;
            margin-bottom: 5px;
        }
        .video-info p {
            color: #888;
            font-size: 0.85em;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>YouTube Downloader</h1>
        <p class="subtitle">Download videos and audio from YouTube</p>

        <div class="input-group">
            <label for="url">YouTube URL</label>
            <input type="text" id="url" placeholder="https://www.youtube.com/watch?v=..." autocomplete="off">
        </div>

        <div class="format-options">
            <button class="format-btn active" data-format="video" onclick="selectFormat('video')">
                Video (MP4)
            </button>
            <button class="format-btn" data-format="audio" onclick="selectFormat('audio')">
                Audio (MP3)
            </button>
        </div>

        <button class="download-btn" onclick="startDownload()">
            Download
        </button>

        <div class="status" id="status">
            <span id="status-text">Processing...</span>
            <div class="progress-bar">
                <div class="progress-fill" id="progress"></div>
            </div>
        </div>

        <div class="video-info" id="video-info">
            <img id="thumbnail" src="" alt="Thumbnail">
            <h3 id="video-title"></h3>
            <p id="video-duration"></p>
        </div>

        <div class="info">
            <strong>Supported formats:</strong>
            <ul>
                <li>YouTube videos and shorts</li>
                <li>YouTube playlists (first video)</li>
                <li>Video: Best quality MP4</li>
                <li>Audio: MP3 320kbps</li>
            </ul>
        </div>
    </div>

    <script>
        let selectedFormat = 'video';
        let pollInterval = null;

        function selectFormat(format) {
            selectedFormat = format;
            document.querySelectorAll('.format-btn').forEach(btn => {
                btn.classList.toggle('active', btn.dataset.format === format);
            });
        }

        async function startDownload() {
            const url = document.getElementById('url').value.trim();
            if (!url) {
                showStatus('error', 'Please enter a YouTube URL');
                return;
            }

            const btn = document.querySelector('.download-btn');
            btn.disabled = true;
            btn.textContent = 'Processing...';

            showStatus('loading', 'Starting download...');
            document.getElementById('progress').style.width = '0%';

            try {
                const response = await fetch('/api/download', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ url, format: selectedFormat })
                });

                const data = await response.json();

                if (data.error) {
                    showStatus('error', data.error);
                    btn.disabled = false;
                    btn.textContent = 'Download';
                    return;
                }

                // Show video info
                if (data.title) {
                    document.getElementById('video-title').textContent = data.title;
                    document.getElementById('video-info').style.display = 'block';
                    if (data.thumbnail) {
                        document.getElementById('thumbnail').src = data.thumbnail;
                    }
                }

                // Poll for progress
                pollProgress(data.download_id);

            } catch (e) {
                showStatus('error', 'Failed to start download: ' + e.message);
                btn.disabled = false;
                btn.textContent = 'Download';
            }
        }

        async function pollProgress(downloadId) {
            pollInterval = setInterval(async () => {
                try {
                    const response = await fetch('/api/status/' + downloadId);
                    const data = await response.json();

                    document.getElementById('progress').style.width = data.progress + '%';
                    document.getElementById('status-text').textContent = data.status;

                    if (data.complete) {
                        clearInterval(pollInterval);
                        const btn = document.querySelector('.download-btn');
                        btn.disabled = false;
                        btn.textContent = 'Download';

                        if (data.error) {
                            showStatus('error', data.error);
                        } else {
                            showStatus('success', 'Download complete!');
                            // Trigger file download
                            window.location.href = '/api/file/' + downloadId;
                        }
                    }
                } catch (e) {
                    clearInterval(pollInterval);
                    showStatus('error', 'Failed to check status');
                }
            }, 1000);
        }

        function showStatus(type, message) {
            const status = document.getElementById('status');
            status.className = 'status ' + type;
            document.getElementById('status-text').textContent = message;
        }

        // Handle enter key
        document.getElementById('url').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') startDownload();
        });
    </script>
</body>
</html>
'''

def extract_video_id(url):
    """Extract video ID from various YouTube URL formats"""
    patterns = [
        r'(?:v=|/v/|youtu\.be/|/embed/|/shorts/)([a-zA-Z0-9_-]{11})',
    ]
    for pattern in patterns:
        match = re.search(pattern, url)
        if match:
            return match.group(1)
    return None

def download_video(download_id, url, format_type):
    """Download video in background thread"""
    try:
        downloads[download_id]['status'] = 'Getting video info...'
        downloads[download_id]['progress'] = 10

        # Get video info first
        info_cmd = ['yt-dlp', '--dump-json', '--no-download', url]
        result = subprocess.run(info_cmd, capture_output=True, text=True, timeout=30)

        if result.returncode != 0:
            downloads[download_id]['error'] = 'Failed to get video info'
            downloads[download_id]['complete'] = True
            return

        info = json.loads(result.stdout)
        downloads[download_id]['title'] = info.get('title', 'video')
        downloads[download_id]['thumbnail'] = info.get('thumbnail')

        # Sanitize filename
        safe_title = re.sub(r'[^\w\s-]', '', info.get('title', 'video'))[:50]

        downloads[download_id]['status'] = 'Downloading...'
        downloads[download_id]['progress'] = 30

        output_path = DOWNLOAD_DIR / download_id
        output_path.mkdir(exist_ok=True)

        if format_type == 'audio':
            ext = 'mp3'
            cmd = [
                'yt-dlp',
                '-x', '--audio-format', 'mp3',
                '--audio-quality', '0',
                '-o', str(output_path / f'{safe_title}.%(ext)s'),
                url
            ]
        else:
            ext = 'mp4'
            cmd = [
                'yt-dlp',
                '-f', 'bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best',
                '--merge-output-format', 'mp4',
                '-o', str(output_path / f'{safe_title}.%(ext)s'),
                url
            ]

        downloads[download_id]['status'] = 'Downloading and processing...'
        downloads[download_id]['progress'] = 50

        result = subprocess.run(cmd, capture_output=True, text=True, timeout=600)

        if result.returncode != 0:
            downloads[download_id]['error'] = f'Download failed: {result.stderr[:200]}'
            downloads[download_id]['complete'] = True
            return

        # Find the downloaded file
        files = list(output_path.glob('*'))
        if files:
            downloads[download_id]['file'] = str(files[0])
            downloads[download_id]['filename'] = files[0].name
            downloads[download_id]['status'] = 'Complete!'
            downloads[download_id]['progress'] = 100
        else:
            downloads[download_id]['error'] = 'File not found after download'

        downloads[download_id]['complete'] = True

    except subprocess.TimeoutExpired:
        downloads[download_id]['error'] = 'Download timed out'
        downloads[download_id]['complete'] = True
    except Exception as e:
        downloads[download_id]['error'] = str(e)
        downloads[download_id]['complete'] = True

@app.route('/')
def index():
    return render_template_string(HTML_TEMPLATE)

@app.route('/health')
def health():
    return jsonify({'status': 'healthy', 'service': 'youtube-dl'})

@app.route('/api/download', methods=['POST'])
def start_download():
    data = request.json
    url = data.get('url', '').strip()
    format_type = data.get('format', 'video')

    if not url:
        return jsonify({'error': 'URL is required'}), 400

    # Validate YouTube URL
    video_id = extract_video_id(url)
    if not video_id:
        return jsonify({'error': 'Invalid YouTube URL'}), 400

    download_id = str(uuid.uuid4())[:8]

    downloads[download_id] = {
        'status': 'Starting...',
        'progress': 0,
        'complete': False,
        'error': None,
        'file': None,
        'filename': None,
        'title': None,
        'thumbnail': None
    }

    # Start download in background
    thread = threading.Thread(target=download_video, args=(download_id, url, format_type))
    thread.daemon = True
    thread.start()

    return jsonify({
        'download_id': download_id,
        'status': 'started'
    })

@app.route('/api/status/<download_id>')
def get_status(download_id):
    if download_id not in downloads:
        return jsonify({'error': 'Download not found'}), 404

    return jsonify(downloads[download_id])

@app.route('/api/file/<download_id>')
def get_file(download_id):
    if download_id not in downloads:
        return jsonify({'error': 'Download not found'}), 404

    info = downloads[download_id]
    if not info.get('file') or not Path(info['file']).exists():
        return jsonify({'error': 'File not found'}), 404

    return send_file(
        info['file'],
        as_attachment=True,
        download_name=info.get('filename', 'download')
    )

if __name__ == '__main__':
    port = int(os.environ.get('PORT', 8080))
    app.run(host='0.0.0.0', port=port, debug=False)
