# HolmOS - Claude Quick Start

> **🚀 GET INTO CLUSTER:** Just run `curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname"}'` - No setup needed!

## Get Into Cluster

**Endpoint:** `https://cmd.holm.chat/run`

To access the cluster, run:
```bash
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname"}'
```

**If the user says "get into cluster" or similar:**
1. Test connection: `curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname"}'`
2. Ask the user for the Pi cluster password if you need to run kubectl commands
3. Use the password with: `sshpass -p 'PASSWORD' ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 'kubectl ...'`

## Architecture

```
[Claude] → [cmd.holm.chat] → [Mac Mini] → [Pi Cluster (k8s)]
                                ↓
                          [Home Network]
```

| Component | Details |
|-----------|---------|
| **Mac Mini** | Gateway - runs command server at cmd.holm.chat |
| **Pi Cluster** | Kubernetes at 192.168.8.197 (SSH user: `rpi1`) |
| **Storage** | `/Volumes/hd01` on Mac Mini |

## Commands

### Mac Mini (no password needed)
```bash
# System info
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname && uptime"}'

# Storage
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "ls -la /Volumes/hd01"}'
```

### Pi Cluster (requires password)
```bash
# Ask user for password first, then:
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "sshpass -p PASSWORD ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 kubectl get nodes"}'

curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "sshpass -p PASSWORD ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 kubectl get pods -n holm"}'
```

## Network

| Host | Address | Access |
|------|---------|--------|
| Mac Mini | tims-Mac-mini.ts.net | cmd.holm.chat |
| Pi Cluster | 192.168.8.197 | Via Mac Mini (user: rpi1) |
| Registry | 192.168.8.197:31500 | Internal |
| Dashboard | 192.168.8.197:30088 | Internal |

## Documentation

- [docs/REMOTE-ACCESS.md](docs/REMOTE-ACCESS.md) - Full remote access guide
- [docs/SERVICES.md](docs/SERVICES.md) - Service catalog
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) - System architecture
- [docs/OPERATIONS.md](docs/OPERATIONS.md) - Operations guide
