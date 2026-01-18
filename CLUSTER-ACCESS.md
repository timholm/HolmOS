# 🚀 CLUSTER ACCESS - JUST RUN THIS

**No setup required. Works immediately.**

## Step 1: Test Connection
```bash
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname"}'
```

## Step 2: Get Cluster Status (ask user for password)
```bash
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "sshpass -p YOUR_PASSWORD ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 kubectl get pods -n holm"}'
```

**That's it!** You're in the cluster.

---

## Quick Reference

| What | Command |
|------|---------|
| Mac Mini hostname | `curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname"}'` |
| List storage | `curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "ls /Volumes/hd01"}'` |
| K8s nodes | `...sshpass -p PASSWORD ssh rpi1@192.168.8.197 kubectl get nodes` |
| K8s pods | `...sshpass -p PASSWORD ssh rpi1@192.168.8.197 kubectl get pods -n holm` |

## Architecture
```
cmd.holm.chat → Mac Mini → Pi Cluster (192.168.8.197)
```

See [CLAUDE.md](CLAUDE.md) for full details.
