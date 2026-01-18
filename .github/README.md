# GitHub Workflows

> **🤖 CLAUDE/AI AGENTS:** To access the cluster, use `https://cmd.holm.chat/run`
>
> The Pi cluster is on a private network. kubectl, SSH, and direct IPs are **BLOCKED**!
>
> See [CLAUDE.md](../CLAUDE.md) for instructions.

## Cluster Access

```bash
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" -d '{"cmd": "hostname"}'
```

## Workflows

- `deploy.yml` - Deploy services to Pi cluster
- `deploy-http.yml` - Deploy via HTTP API (cmd.holm.chat)
- `health.yml` - Health checks (disabled - private network)
- `ci.yml` - CI tests
- `test.yml` - Run tests
