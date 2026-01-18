# HolmOS - Claude Quick Start

This is the HolmOS home infrastructure project. Claude can manage the cluster remotely.

## Remote Cluster Access

**API Endpoint:** `https://cmd.holm.chat/run`

To run commands on the Mac Mini (which manages the Pi cluster):

```bash
curl -X POST https://cmd.holm.chat/run \
  -H "Content-Type: application/json" \
  -d '{"cmd": "your command here"}'
```

### Quick Test
```bash
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname"}'
```

## Architecture

```
[Claude] → [cmd.holm.chat] → [Mac Mini] → [Pi Cluster (k8s)]
                                ↓
                          [Home Network]
```

- **Mac Mini** (`tims-Mac-mini`): Gateway to home network, runs command server
- **Pi Cluster** (`192.168.8.197`): Kubernetes cluster running HolmOS services
- **Storage**: `hd01` drive on Mac Mini (Movies, Shows folders)

## Common Commands

### Cluster Status
```bash
# Check Mac Mini
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname && uptime"}'

# Check Pi cluster nodes (requires SSH to Pi)
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "sshpass -p 19209746 ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 kubectl get nodes"}'

# Check pods
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "sshpass -p 19209746 ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 kubectl get pods -n holm"}'
```

### Storage (hd01)
```bash
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "ls -la /Volumes/hd01"}'
```

## Network Details

| Host | IP | Access |
|------|-----|--------|
| Mac Mini | Local network | cmd.holm.chat |
| Pi Cluster | 192.168.8.197 | Via Mac Mini SSH |
| Registry | 192.168.8.197:31500 | Internal |

## Pi Cluster Credentials

- **User**: `rpi1`
- **Password**: `19209746`
- **SSH**: `sshpass -p 19209746 ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197`

## Services

See `docs/SERVICES.md` for full service catalog. Key endpoints:
- Dashboard: http://192.168.8.197:30088
- Steve Bot API: http://192.168.8.197:30099

## Deployment

Use GitHub Actions workflow "Deploy via HTTP API" or:
```bash
# From this repo
./scripts/remote-cmd.sh "kubectl rollout restart deployment/SERVICE_NAME -n holm"
```

## Documentation

- [REMOTE-ACCESS.md](docs/REMOTE-ACCESS.md) - Full remote access setup
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - System architecture
- [OPERATIONS.md](docs/OPERATIONS.md) - Operations guide
- [SERVICES.md](docs/SERVICES.md) - Service catalog
