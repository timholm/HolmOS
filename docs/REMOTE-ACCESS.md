# Remote Access to HolmOS Cluster

Access the Mac Mini and Pi cluster remotely via the HTTP command API.

## Quick Start

**Endpoint:** `https://cmd.holm.chat/run`

```bash
curl -X POST https://cmd.holm.chat/run \
  -H "Content-Type: application/json" \
  -d '{"cmd": "hostname"}'
```

Or use the script:
```bash
./scripts/remote-cmd.sh "hostname"
```

## Architecture

```
[Remote Client] → [cmd.holm.chat] → [Cloudflare Tunnel] → [Mac Mini] → [Pi Cluster]
```

- **cmd.holm.chat** - Public endpoint (Cloudflare tunnel to Mac Mini)
- **Mac Mini** - Runs command-server.py, gateway to home network
- **Pi Cluster** - Kubernetes cluster at 192.168.8.197

## API Reference

### POST /run

Execute a command on the Mac Mini.

**Request:**
```json
{"cmd": "your command here"}
```

**Response:**
```json
{"stdout": "output", "stderr": "", "code": 0}
```

**Examples:**
```bash
# Mac Mini info
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "hostname && uptime"}'

# List storage
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "ls -la /Volumes/hd01"}'

# Pi cluster pods (replace PASSWORD with actual password)
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "sshpass -p PASSWORD ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 kubectl get pods -n holm"}'
```

## Using the Script

The `remote-cmd.sh` script defaults to cmd.holm.chat:

```bash
# No setup needed - just run
./scripts/remote-cmd.sh "hostname"
./scripts/remote-cmd.sh "ls /Volumes/hd01"
./scripts/remote-cmd.sh "kubectl get nodes"  # Runs on Mac Mini
```

Override endpoint if needed:
```bash
REMOTE_API_URL=https://other.url ./scripts/remote-cmd.sh "hostname"
```

## Server Setup (Mac Mini)

The command server should already be running. If you need to restart:

```bash
# Start the command server
python3 ~/HolmOS/scripts/command-server.py &

# Verify it's running
curl http://localhost:8080/run -X POST -H "Content-Type: application/json" -d '{"cmd": "echo ok"}'
```

### Cloudflare Tunnel

The tunnel exposes port 8080 as cmd.holm.chat. Managed via cloudflared:

```bash
# Check tunnel status
cloudflared tunnel list
```

### Persistent Service (launchd)

`~/Library/LaunchAgents/com.holmos.command-server.plist`:
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

## Common Operations

### Cluster Status
```bash
./scripts/remote-cmd.sh "sshpass -p PASSWORD ssh rpi1@192.168.8.197 kubectl get nodes"
./scripts/remote-cmd.sh "sshpass -p PASSWORD ssh rpi1@192.168.8.197 kubectl get pods -n holm"
```

### Deploy a Service
```bash
./scripts/remote-cmd.sh "sshpass -p PASSWORD ssh rpi1@192.168.8.197 kubectl rollout restart deployment/SERVICE -n holm"
```

### View Logs
```bash
./scripts/remote-cmd.sh "sshpass -p PASSWORD ssh rpi1@192.168.8.197 kubectl logs -n holm deployment/steve-bot --tail=50"
```

### Storage Access
```bash
./scripts/remote-cmd.sh "ls -la /Volumes/hd01/Movies"
./scripts/remote-cmd.sh "df -h /Volumes/hd01"
```

## Network Reference

| Host | Address | Access |
|------|---------|--------|
| Mac Mini | tims-Mac-mini.ts.net | cmd.holm.chat |
| Pi Cluster | 192.168.8.197 | Via Mac Mini |
| Registry | 192.168.8.197:31500 | Internal |
| Dashboard | 192.168.8.197:30088 | Internal |

## Troubleshooting

### Connection refused
```bash
# Check if server is running on Mac Mini
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "ps aux | grep command-server"}'
```

### Timeout errors
Commands timeout after 5 minutes. For long operations, run in background:
```bash
./scripts/remote-cmd.sh "nohup long-command &"
```

### Cloudflare tunnel down
SSH directly to Mac Mini if tunnel is down, then restart cloudflared.

## Related Documentation

- [CLAUDE.md](../CLAUDE.md) - Quick start for Claude
- [OPERATIONS.md](./OPERATIONS.md) - Cluster operations guide
- [ARCHITECTURE.md](./ARCHITECTURE.md) - System architecture
