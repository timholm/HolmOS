#!/usr/bin/env python3
"""
command-server.py - Simple HTTP server for running commands remotely

Run this on your Mac Mini, then expose via ngrok:
    python3 command-server.py &
    ngrok http 8080

Security: This server executes arbitrary commands! Only expose via ngrok
with proper authentication or on trusted networks.
"""

from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import subprocess
import sys

PORT = 8080

class CommandHandler(BaseHTTPRequestHandler):
    def _send_response(self, status, data):
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Access-Control-Allow-Origin', '*')
        self.end_headers()
        self.wfile.write(json.dumps(data).encode())

    def do_OPTIONS(self):
        self.send_response(200)
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'POST, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type')
        self.end_headers()

    def do_POST(self):
        if self.path != '/run':
            self._send_response(404, {'error': 'Not found'})
            return

        try:
            content_length = int(self.headers.get('Content-Length', 0))
            body = self.rfile.read(content_length).decode('utf-8')
            data = json.loads(body)
            cmd = data.get('cmd', '')

            if not cmd:
                self._send_response(400, {'error': 'No command provided'})
                return

            # Execute command
            result = subprocess.run(
                cmd,
                shell=True,
                capture_output=True,
                text=True,
                timeout=300  # 5 minute timeout
            )

            self._send_response(200, {
                'stdout': result.stdout,
                'stderr': result.stderr,
                'code': result.returncode
            })

        except json.JSONDecodeError as e:
            self._send_response(400, {'error': f'Invalid JSON: {e}'})
        except subprocess.TimeoutExpired:
            self._send_response(408, {'error': 'Command timed out'})
        except Exception as e:
            self._send_response(500, {'error': str(e)})

    def log_message(self, format, *args):
        print(f"[{self.log_date_time_string()}] {args[0]}")

def main():
    server = HTTPServer(('0.0.0.0', PORT), CommandHandler)
    print(f"Command server running on port {PORT}")
    print(f"Expose with: ngrok http {PORT}")
    print("Press Ctrl+C to stop")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down...")
        server.shutdown()

if __name__ == '__main__':
    main()
