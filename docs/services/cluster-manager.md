# Cluster Manager

## Purpose

The Cluster Manager provides centralized management and monitoring for all physical nodes in the HolmOS Raspberry Pi cluster. It enables administrators to view node status, execute remote commands, manage system updates, perform reboots, and monitor cluster health from a single dashboard.

## How It Works

### Node Discovery and Monitoring
- Maintains a static list of known cluster nodes with their IP addresses
- Uses TCP socket connections to SSH port (22) for fast node availability checks
- Caches node status with a 10-second TTL to reduce network overhead
- Performs parallel pings using a thread pool for quick status updates

### SSH Command Execution
- Executes commands on remote nodes via SSH using `sshpass`
- Supports custom command execution with configurable timeouts
- Includes safety filters to block dangerous commands (e.g., `rm -rf /`, `mkfs`)
- Maximum command timeout of 5 minutes

### Node Operations
- **Ping**: Check individual node connectivity and latency
- **Update**: Run `apt update && apt upgrade -y` on nodes
- **Reboot**: Safely reboot individual nodes or groups of workers
- **Shutdown**: Power off nodes (requires confirmation for control plane)
- **Execute**: Run arbitrary commands on nodes

### Metrics Collection
- Gathers CPU, memory, and disk usage via SSH commands
- Collects system load averages (1, 5, 15 minutes)
- Reports uptime for each node

### Cluster Health
- Calculates overall cluster health percentage
- Distinguishes between control plane, NAS, and worker nodes
- Reports average network latency across online nodes
- Provides cluster status: healthy, degraded, critical, or offline

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SSH_USER` | `rpi1` | SSH username for remote access |
| `SSH_PASSWORD` | `19209746` | SSH password for remote access |

### Known Nodes

The cluster manager monitors these nodes by default:

| Hostname | IP Address | Role |
|----------|------------|------|
| rpi-1 | 192.168.8.197 | Control Plane |
| rpi-2 | 192.168.8.196 | Worker |
| rpi-3 | 192.168.8.195 | Worker |
| rpi-4 | 192.168.8.194 | Worker |
| rpi-5 | 192.168.8.108 | Worker |
| rpi-6 | 192.168.8.235 | Worker |
| rpi-7 | 192.168.8.209 | Worker |
| rpi-8 | 192.168.8.202 | Worker |
| rpi-9 | 192.168.8.187 | Worker |
| rpi-10 | 192.168.8.210 | Worker |
| rpi-11 | 192.168.8.231 | Worker |
| rpi-12 | 192.168.8.105 | Worker |
| openmediavault | 192.168.8.199 | NAS |

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/nodes` | GET | List all nodes with status |
| `/api/v1/nodes/refresh` | POST | Force refresh node cache |
| `/api/v1/nodes/online` | GET | List online nodes only |
| `/api/v1/nodes/offline` | GET | List offline nodes only |
| `/api/v1/nodes/{hostname}/ping` | GET | Ping specific node |
| `/api/v1/nodes/{hostname}/details` | GET | Get node details |
| `/api/v1/nodes/{hostname}/metrics` | GET | Get node system metrics |
| `/api/v1/nodes/{hostname}/update` | POST | Run apt update on node |
| `/api/v1/nodes/{hostname}/reboot` | POST | Reboot specific node |
| `/api/v1/nodes/{hostname}/shutdown` | POST | Shutdown specific node |
| `/api/v1/nodes/{hostname}/exec` | POST | Execute command on node |
| `/api/v1/health` | GET | Cluster health overview |
| `/api/v1/cluster/summary` | GET | Comprehensive cluster summary |
| `/api/v1/update-all` | POST | Update all nodes sequentially |
| `/api/v1/reboot-workers` | POST | Reboot all worker nodes |
| `/api/v1/reboot-all` | POST | Reboot entire cluster (requires confirmation) |
| `/api/v1/kubeconfig` | GET | Download cluster kubeconfig |
| `/api/v1/terminal/url/{hostname}` | GET | Get SSH terminal URL for node |

## Dependencies

### System Requirements
- `sshpass` must be installed for SSH authentication
- Network access to all cluster nodes on port 22 (SSH)

### Internal Services
- **Terminal Web**: Provides SSH terminal access for nodes

### Kubernetes Resources
- Optional: kubeconfig access from mounted ConfigMap or direct k3s path
- Paths checked: `/etc/kubeconfig/kubeconfig.yaml`, `/etc/rancher/k3s/k3s.yaml`, `~/.kube/config`

### External Dependencies
- Flask web framework
- Flask-CORS for cross-origin requests
- Waitress WSGI server
- concurrent.futures for parallel operations
