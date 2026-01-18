# Remote Access to HolmOS Cluster

This document describes how to access the Mac Mini and Pi cluster remotely via HTTP API over ngrok.

## Overview

The cluster can be accessed remotely using an HTTP command API that runs on the Mac Mini and is exposed via ngrok. This approach:
- Bypasses SSH protocol issues with firewalls/proxies
- Works through corporate networks and sandboxed environments
- Provides simple REST API for command execution

## Architecture

```
[Remote Client] → [ngrok] → [Mac Mini Command Server] → [Pi Cluster]
                    ↓
              HTTPS tunnel
```

## Setup (Mac Mini)

### 1. Start the Command Server

```bash
# Clone the repo if needed
cd ~/HolmOS

# Run the command server
python3 scripts/command-server.py &
```

The server runs on port 8080 by default.

### 2. Expose via ngrok

```bash
# Start ngrok tunnel
ngrok http 8080
```

Note the generated URL (e.g., `https://xxxx.ngrok-free.app`)

### 3. Keep it Running (Optional)

For persistent access, use a systemd service or launchd:

**macOS launchd** (`~/Library/LaunchAgents/com.holmos.command-server.plist`):
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.holmos.command-server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/bin/python3</string>
        <string>/Users/tim/HolmOS/scripts/command-server.py</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

Load with: `launchctl load ~/Library/LaunchAgents/com.holmos.command-server.plist`

## Usage

### Using the Remote Command Script

```bash
# Set the ngrok URL
export REMOTE_API_URL="https://xxxx.ngrok-free.app"

# Run commands
./scripts/remote-cmd.sh "hostname"
./scripts/remote-cmd.sh "kubectl get pods -n holm"
./scripts/remote-cmd.sh "docker ps"
```

### Direct curl Usage

```bash
# Run a command
curl -X POST https://xxxx.ngrok-free.app/run \
  -H "Content-Type: application/json" \
  -d '{"cmd": "kubectl get nodes"}'

# Response format:
# {"stdout": "...", "stderr": "...", "code": 0}
```

### From CI/CD (GitHub Actions)

```yaml
- name: Run remote command
  env:
    REMOTE_API_URL: ${{ secrets.REMOTE_API_URL }}
  run: |
    curl -X POST $REMOTE_API_URL/run \
      -H "Content-Type: application/json" \
      -d '{"cmd": "kubectl rollout status deployment/my-app -n holm"}'
```

## Common Commands

### Cluster Status
```bash
./scripts/remote-cmd.sh "kubectl get nodes"
./scripts/remote-cmd.sh "kubectl get pods -n holm"
./scripts/remote-cmd.sh "kubectl top nodes"
```

### Deploy a Service
```bash
./scripts/remote-cmd.sh "kubectl set image deployment/steve-bot steve-bot=192.168.8.197:31500/steve-bot:latest -n holm"
./scripts/remote-cmd.sh "kubectl rollout status deployment/steve-bot -n holm"
```

### View Logs
```bash
./scripts/remote-cmd.sh "kubectl logs -n holm deployment/steve-bot --tail=50"
```

### Access Pi Cluster from Mac Mini
```bash
./scripts/remote-cmd.sh "sshpass -p '19209746' ssh rpi1@192.168.8.197 'kubectl get pods -n holm'"
```

## Security Considerations

1. **ngrok URL rotation** - Free ngrok URLs change on restart. Use ngrok paid plan for stable URLs, or update `REMOTE_API_URL` secret when URL changes.

2. **Authentication** - The basic command server has no authentication. For production:
   - Use ngrok's built-in auth: `ngrok http 8080 --basic-auth="user:password"`
   - Add API key validation to command-server.py
   - Use ngrok's IP restrictions

3. **Command injection** - The server executes arbitrary commands. Only expose to trusted clients.

## Troubleshooting

### Connection refused
- Ensure command-server.py is running: `ps aux | grep command-server`
- Check ngrok is running: `ngrok status`

### Timeout errors
- Commands have a 5-minute timeout by default
- For long-running commands, increase timeout in command-server.py

### ngrok URL changed
- Restart ngrok: `ngrok http 8080`
- Update `REMOTE_API_URL` environment variable or GitHub secret

## GitHub Secrets Required

| Secret | Description |
|--------|-------------|
| `REMOTE_API_URL` | ngrok URL (e.g., `https://xxxx.ngrok-free.app`) |

## Related Documentation

- [OPERATIONS.md](./OPERATIONS.md) - Cluster operations guide
- [ARCHITECTURE.md](./ARCHITECTURE.md) - System architecture
