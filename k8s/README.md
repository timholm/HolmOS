# Kubernetes Configurations

> **🤖 CLAUDE/AI AGENTS:** To access the cluster, use `https://cmd.holm.chat/run`
>
> kubectl, SSH, and private IPs (192.168.x.x) are **BLOCKED** from sandboxed environments!
>
> See [CLAUDE.md](../CLAUDE.md) for full instructions.

## Contents

- `hpa/` - Horizontal Pod Autoscalers
- `network-policies/` - Network policies
- `pdb/` - Pod Disruption Budgets
- `secrets/` - Secret templates (no real values)
- `tekton/` - CI/CD pipelines

## Accessing the Cluster

```bash
# Use cmd.holm.chat (NOT kubectl directly!)
curl -X POST https://cmd.holm.chat/run -H "Content-Type: application/json" \
  -d '{"cmd": "sshpass -p PASSWORD ssh -o StrictHostKeyChecking=no rpi1@192.168.8.197 kubectl get pods -n holm"}'
```
