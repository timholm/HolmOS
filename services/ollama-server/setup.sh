#!/bin/bash
# Ollama Server Setup Script for Debian x86_64 with NVIDIA GPU
# Target: Lenovo laptop with RTX 2070 Mobile
# Usage: curl -fsSL <url>/setup.sh | sudo bash

set -e

echo "=== Ollama Server Setup ==="

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root: sudo bash setup.sh"
    exit 1
fi

# Get the actual user (not root)
ACTUAL_USER=${SUDO_USER:-$USER}

echo "[1/7] Installing dependencies..."
apt update
apt install -y curl wget gnupg2 software-properties-common

echo "[2/7] Installing NVIDIA drivers..."
# Add contrib and non-free repos
cat > /etc/apt/sources.list.d/nvidia.list << 'EOF'
deb http://deb.debian.org/debian bookworm main contrib non-free non-free-firmware
deb http://deb.debian.org/debian bookworm-updates main contrib non-free non-free-firmware
deb http://security.debian.org/debian-security bookworm-security main contrib non-free non-free-firmware
EOF

apt update
apt install -y linux-headers-$(uname -r) nvidia-driver nvidia-kernel-dkms

echo "[3/7] Installing Ollama..."
curl -fsSL https://ollama.com/install.sh | sh

echo "[4/7] Configuring Ollama to listen on all interfaces..."
mkdir -p /etc/systemd/system/ollama.service.d
cat > /etc/systemd/system/ollama.service.d/override.conf << 'EOF'
[Service]
Environment="OLLAMA_HOST=0.0.0.0"
EOF

systemctl daemon-reload
systemctl enable ollama
systemctl restart ollama

echo "[5/7] Disabling sleep on lid close..."
sed -i 's/#HandleLidSwitch=.*/HandleLidSwitch=ignore/' /etc/systemd/logind.conf
sed -i 's/#HandleLidSwitchExternalPower=.*/HandleLidSwitchExternalPower=ignore/' /etc/systemd/logind.conf
sed -i 's/#HandleLidSwitchDocked=.*/HandleLidSwitchDocked=ignore/' /etc/systemd/logind.conf
sed -i 's/HandleLidSwitch=.*/HandleLidSwitch=ignore/' /etc/systemd/logind.conf
sed -i 's/HandleLidSwitchExternalPower=.*/HandleLidSwitchExternalPower=ignore/' /etc/systemd/logind.conf
sed -i 's/HandleLidSwitchDocked=.*/HandleLidSwitchDocked=ignore/' /etc/systemd/logind.conf

systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target
systemctl restart systemd-logind

echo "[6/7] Adding user to ollama group..."
usermod -aG ollama "$ACTUAL_USER"

echo "[7/7] Pulling default models..."
sleep 5  # Wait for ollama to start
sudo -u "$ACTUAL_USER" ollama pull llama3.2:3b
sudo -u "$ACTUAL_USER" ollama pull qwen2.5-coder:7b

# Get IP address
IP=$(hostname -I | awk '{print $1}')

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Ollama Server: http://${IP}:11434"
echo ""
echo "Models installed:"
ollama list
echo ""
echo "Test with:"
echo "  curl http://${IP}:11434/api/generate -d '{\"model\":\"qwen2.5-coder:7b\",\"prompt\":\"Hello\"}'"
echo ""
echo "NOTE: Reboot required for NVIDIA drivers to load!"
echo "Run: sudo reboot"
