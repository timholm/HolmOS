#!/bin/sh
set -e

echo "=== PXE Provisioning Server Starting ==="

# Get server IP if not set
if [ -z "$SERVER_IP" ]; then
    SERVER_IP=$(hostname -i | awk '{print $1}')
fi
echo "Server IP: $SERVER_IP"

# Configure dnsmasq with the server IP
cat >> /etc/dnsmasq.conf << EOF

# Dynamic configuration
dhcp-range=${SERVER_IP},proxy
tftp-secure
EOF

# Start dnsmasq for TFTP and DHCP proxy
echo "Starting dnsmasq (TFTP + DHCP proxy)..."
dnsmasq --no-daemon --log-queries &
DNSMASQ_PID=$!

# Start nginx for HTTP file serving
echo "Starting nginx for HTTP file serving..."
cat > /etc/nginx/http.d/default.conf << EOF
server {
    listen 80;
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
exec /app/pxe-server
