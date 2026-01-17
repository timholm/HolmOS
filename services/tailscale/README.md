# Tailscale Subnet Router for HolmOS

Provides secure remote access to the HolmOS cluster via Tailscale VPN.

## Features

- Access all cluster services from anywhere
- No port forwarding or public IPs needed
- Secure WireGuard-based encryption
- Works behind NAT/firewalls

## Setup

### 1. Generate Auth Key

1. Go to [Tailscale Admin Console](https://login.tailscale.com/admin/settings/keys)
2. Generate a new auth key with:
   - Reusable: Yes
   - Ephemeral: Yes (recommended for auto-cleanup)
   - Tags: `tag:k8s` (optional)

### 2. Update Secret

Edit `deployment.yaml` and replace `tskey-auth-REPLACE_ME` with your auth key:

```yaml
stringData:
  AUTH_KEY: "tskey-auth-xxxxx-xxxxxxxxx"
```

### 3. Deploy

```bash
kubectl apply -f deployment.yaml
```

### 4. Approve Routes

1. Go to [Tailscale Machines](https://login.tailscale.com/admin/machines)
2. Find `holmos-cluster`
3. Click "..." > "Edit route settings"
4. Enable the advertised routes:
   - `10.42.0.0/16` (Pod network)
   - `10.43.0.0/16` (Service network)
   - `192.168.8.0/24` (Local network)

## Usage

Once connected to Tailscale, you can access:

| Service | URL |
|---------|-----|
| HolmOS Shell | http://192.168.8.197:30000 |
| Nova Dashboard | http://192.168.8.197:30004 |
| Terminal | http://192.168.8.197:30800 |

Or use cluster DNS:
```bash
curl http://holmos-shell.holm.svc.cluster.local:3000
```

## Node Setup (Alternative)

For direct node access, run on each Pi:

```bash
curl -fsSL https://raw.githubusercontent.com/timholm/HolmOS/main/scripts/setup-tailscale.sh | sudo bash
```

Then authenticate:
```bash
sudo tailscale up --accept-routes
```

## Troubleshooting

Check pod status:
```bash
kubectl get pods -n tailscale
kubectl logs -n tailscale -l app=tailscale
```

Check Tailscale status:
```bash
kubectl exec -n tailscale deploy/tailscale-subnet-router -- tailscale status
```
