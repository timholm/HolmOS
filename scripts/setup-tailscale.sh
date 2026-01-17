#!/bin/bash
# Tailscale Setup Script for HolmOS Cluster Nodes
# Supports: Raspberry Pi (arm64), Debian/Ubuntu (x86_64)
# Usage: curl -fsSL <url>/setup-tailscale.sh | sudo bash
#
# Options:
#   SUBNET_ROUTER=1  - Enable subnet routing for cluster access
#   AUTH_KEY=xxx     - Use auth key for unattended setup
#   ADVERTISE_ROUTES="10.42.0.0/16,10.43.0.0/16" - K8s pod/service CIDRs

set -e

echo "=== Tailscale Setup for HolmOS ==="

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root: sudo bash setup-tailscale.sh"
    exit 1
fi

# Detect architecture
ARCH=$(uname -m)
echo "Detected architecture: $ARCH"

# Detect OS
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
    VERSION=$VERSION_CODENAME
else
    echo "Cannot detect OS"
    exit 1
fi
echo "Detected OS: $OS $VERSION"

echo "[1/4] Installing Tailscale..."

# Install based on OS
case $OS in
    debian|ubuntu|raspbian)
        # Add Tailscale's package signing key and repository
        curl -fsSL https://pkgs.tailscale.com/stable/$OS/$VERSION.noarmor.gpg | tee /usr/share/keyrings/tailscale-archive-keyring.gpg >/dev/null
        curl -fsSL https://pkgs.tailscale.com/stable/$OS/$VERSION.tailscale-keyring.list | tee /etc/apt/sources.list.d/tailscale.list

        apt-get update
        apt-get install -y tailscale
        ;;
    *)
        echo "Unsupported OS: $OS"
        echo "Trying generic install script..."
        curl -fsSL https://tailscale.com/install.sh | sh
        ;;
esac

echo "[2/4] Enabling IP forwarding (for subnet routing)..."
# Enable IP forwarding for subnet routing
cat > /etc/sysctl.d/99-tailscale.conf << 'EOF'
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
EOF
sysctl -p /etc/sysctl.d/99-tailscale.conf

echo "[3/4] Starting Tailscale service..."
systemctl enable --now tailscaled

echo "[4/4] Configuring Tailscale..."

# Build tailscale up command
UP_CMD="tailscale up"

# Add auth key if provided
if [ -n "$AUTH_KEY" ]; then
    UP_CMD="$UP_CMD --authkey=$AUTH_KEY"
fi

# Configure as subnet router if requested
if [ "$SUBNET_ROUTER" = "1" ]; then
    # Default to K8s pod and service CIDRs if not specified
    ROUTES=${ADVERTISE_ROUTES:-"10.42.0.0/16,10.43.0.0/16"}
    UP_CMD="$UP_CMD --advertise-routes=$ROUTES --accept-routes"
    echo "Subnet router mode enabled for: $ROUTES"
fi

# Accept routes from other nodes
UP_CMD="$UP_CMD --accept-routes"

# Run tailscale up
if [ -n "$AUTH_KEY" ]; then
    echo "Authenticating with auth key..."
    eval $UP_CMD
else
    echo ""
    echo "=== Manual Authentication Required ==="
    echo ""
    echo "Run the following command to authenticate:"
    echo ""
    echo "  sudo tailscale up --accept-routes"
    echo ""
    if [ "$SUBNET_ROUTER" = "1" ]; then
        echo "For subnet router mode, run:"
        echo ""
        echo "  sudo tailscale up --advertise-routes=${ROUTES:-10.42.0.0/16,10.43.0.0/16} --accept-routes"
        echo ""
        echo "Then approve the routes in Tailscale admin console:"
        echo "  https://login.tailscale.com/admin/machines"
        echo ""
    fi
fi

# Get status
echo ""
echo "=== Setup Complete ==="
echo ""
tailscale status || echo "(Not yet authenticated)"
echo ""
echo "Useful commands:"
echo "  tailscale status    - Show connection status"
echo "  tailscale ip        - Show Tailscale IP address"
echo "  tailscale ping <ip> - Ping another Tailscale node"
echo "  tailscale netcheck  - Check network connectivity"
echo ""
echo "Tailscale admin: https://login.tailscale.com/admin"
echo ""
