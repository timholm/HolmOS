# HolmOS Scripts

> **🤖 CLAUDE/AI AGENTS:** To access the cluster, use `https://cmd.holm.chat/run`
>
> kubectl, SSH, and private IPs are **BLOCKED** from sandboxed environments!
>
> See [CLAUDE.md](../CLAUDE.md) for instructions.

## Available Scripts

- `remote-cmd.sh` - Run commands via cmd.holm.chat API
- `command-server.py` - HTTP server running on Mac Mini
- `smoke-test.sh` - Run smoke tests

## Quick Access

```bash
# Test connection
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname"}'

# Get pods (ask user for password)
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "sshpass -p PASSWORD ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 kubectl get pods -n holm"}'
```
