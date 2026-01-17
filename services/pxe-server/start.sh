#!/bin/sh
set -e

echo "=== PXE Provisioning Server Starting ==="

# Get server IP if not set
if [ -z "$SERVER_IP" ]; then
    SERVER_IP=$(hostname -i | awk '{print $1}')
fi
echo "Server IP: $SERVER_IP"

# Configure dnsmasq for TFTP only (no DHCP - configure your router instead)
cat > /etc/dnsmasq.conf << EOF
# TFTP Server Configuration
port=0
enable-tftp
tftp-root=/tftpboot
log-queries
log-facility=/var/log/dnsmasq.log
EOF

# Start dnsmasq for TFTP only
echo "Starting dnsmasq (TFTP server on port 69)..."
dnsmasq --no-daemon --log-queries &
DNSMASQ_PID=$!

# Start nginx for HTTP file serving on port 8088
echo "Starting nginx for HTTP file serving on port 8088..."
cat > /etc/nginx/http.d/default.conf << EOF
server {
    listen 8088;
    server_name _;

    root /var/www/html;
    autoindex on;

    location / {
        try_files \$uri \$uri/ =404;
    }

    location /autoinstall/ {
        alias /var/www/html/autoinstall/;
        autoindex on;
    }
}
EOF
nginx &
NGINX_PID=$!

# Start the Go API server
echo "Starting PXE API server on port ${PORT:-8080}..."
echo ""
echo "=== PXE Server Ready ==="
echo "TFTP Server: ${SERVER_IP}:69"
echo "HTTP Files:  http://${SERVER_IP}:8088/"
echo "API Server:  http://${SERVER_IP}:${PORT:-8080}/"
echo ""
echo "Configure your router DHCP with:"
echo "  Option 66 (TFTP Server): ${SERVER_IP}"
echo "  Option 67 (Boot File):   pxelinux.0"
echo ""
exec /app/pxe-server
