# PXE Server

## Purpose

The PXE (Preboot Execution Environment) Server enables network-based Linux installation for bare-metal provisioning in the HolmOS cluster. It serves boot files over TFTP/HTTP, manages ISO images, generates automated installation configurations (autoinstall/preseed), and provides a complete solution for deploying new cluster nodes.

## How It Works

### Network Boot Flow
1. New machine boots via PXE and receives DHCP response with TFTP server address
2. Machine downloads bootloader (pxelinux) from TFTP server
3. Bootloader fetches menu configuration and displays available installation options
4. User selects distribution; kernel and initrd are loaded via TFTP
5. Installer boots and fetches autoinstall/preseed configuration via HTTP
6. Automated installation proceeds without user interaction

### ISO Management
- Downloads distribution ISOs from official mirrors
- Extracts kernel, initrd, and installation files from ISOs
- Copies netboot files to TFTP root for PXE booting
- Tracks available ISOs and their readiness status

### Configuration Generation
- **Ubuntu Autoinstall**: Cloud-init based configuration for Ubuntu 20.04+
- **Debian Preseed**: Traditional preseed configuration for Debian
- Generates PXE boot entries for each configured host
- Supports custom hostname, username, password, SSH keys, disk, timezone, and locale

### Supported Distributions
- Ubuntu 22.04 LTS
- Ubuntu 24.04 LTS
- Debian 12
- Rocky Linux 9
- AlmaLinux (detection supported)

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `TFTP_ROOT` | `/tftpboot` | TFTP root directory for boot files |
| `HTTP_ROOT` | `/var/www/html` | HTTP root for installation files |
| `ISO_DIR` | `/iso` | Directory for ISO storage |
| `SERVER_IP` | `192.168.8.197` | Server IP for client configuration |

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Server status with ISO list |
| `/health` | GET | Health check |
| `/api/status` | GET | Server status and ISOs |
| `/api/distros` | GET | List available distributions |
| `/api/isos` | GET | List downloaded ISOs |
| `/api/download` | POST | Download ISO from URL |
| `/api/provision` | POST | Generate PXE config for host |
| `/files/*` | GET | Static file server for installations |

### Provision Request

```json
POST /api/provision
{
  "distro": "ubuntu",
  "version": "22.04",
  "hostname": "node-01",
  "username": "admin",
  "password": "secure-password",
  "sshKey": "ssh-rsa AAAA...",
  "disk": "/dev/sda",
  "timezone": "America/New_York",
  "locale": "en_US.UTF-8"
}
```

### Provision Response

```json
{
  "status": "configured",
  "configPath": "/var/www/html/autoinstall/node-01.yaml",
  "pxePath": "/tftpboot/pxelinux.cfg/01-node-01"
}
```

### Download ISO Request

```json
POST /api/download
{
  "url": "https://releases.ubuntu.com/22.04/ubuntu-22.04.3-live-server-amd64.iso",
  "name": "ubuntu-22.04.iso"
}
```

### Directory Structure

```
/tftpboot/
  pxelinux.cfg/
    default           # Default PXE menu
    01-{hostname}     # Per-host configurations
  ubuntu-22.04/
    vmlinuz           # Linux kernel
    initrd            # Initial ramdisk
  debian-12/
    vmlinuz
    initrd

/var/www/html/
  autoinstall/
    {hostname}.yaml   # Ubuntu autoinstall configs
  ubuntu-22.04/       # Extracted ISO contents
  debian-12/
  preseed.cfg         # Debian preseed template

/iso/
  ubuntu-22.04.iso
  debian-12.iso
```

### Default PXE Menu

The server generates a default PXE menu with options for:
- Ubuntu 22.04 LTS installation
- Debian 12 installation
- Boot from local disk

### Autoinstall Configuration

Generated Ubuntu autoinstall includes:
- LVM-based storage layout
- SSH server installation with root login enabled
- Common packages: openssh-server, curl, vim, htop, net-tools
- Custom user with sudo access
- SSH key deployment

## Dependencies

### System Requirements
- TFTP server (separate from this service) pointed at `TFTP_ROOT`
- DHCP server configured to point to TFTP server
- Sufficient disk space for ISOs (typically 1-5GB each)
- Network access to distribution mirrors for ISO downloads

### Required Directories
The service creates these directories on startup:
- `TFTP_ROOT` - TFTP boot files
- `HTTP_ROOT` - HTTP installation files
- `ISO_DIR` - ISO storage
- `HTTP_ROOT/autoinstall` - Generated configurations

### External Dependencies
- Standard Go HTTP client for ISO downloads
- System `mount` and `cp` commands for ISO extraction
- PXE-capable clients on the same network

### Network Requirements
- Port 69/UDP: TFTP (served by external TFTP server)
- Port 80/HTTP or configured port: HTTP file serving
- DHCP must provide next-server and filename options
