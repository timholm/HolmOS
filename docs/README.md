# HolmOS Documentation

> **🤖 CLAUDE/AI AGENTS:** To access the cluster, use `https://cmd.holm.chat/run`
>
> kubectl, SSH, and private IPs (192.168.x.x) are **BLOCKED**!
>
> See [CLAUDE.md](../CLAUDE.md) for instructions.

## Quick Cluster Access

```bash
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname"}'
```

## Documentation Index

- [REMOTE-ACCESS.md](REMOTE-ACCESS.md) - How to access the cluster remotely
- [OPERATIONS.md](OPERATIONS.md) - Operations runbook
- [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture
- [SERVICES.md](SERVICES.md) - Service catalog
- [SECRETS.md](SECRETS.md) - Secrets management
