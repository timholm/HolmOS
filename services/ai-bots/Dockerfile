# AI Bots - Steve and Karen (the moody beta tester)
FROM python:3.11-slim

# Install kubectl and chromium for browser testing (detect architecture)
RUN apt-get update && apt-get install -y \
    curl \
    chromium \
    chromium-driver \
    && ARCH=$(dpkg --print-architecture) && \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/${ARCH}/kubectl" && \
    install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl && \
    rm kubectl && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
WORKDIR /app

RUN pip install --no-cache-dir \
    flask \
    flask-sock \
    aiohttp \
    simple-websocket

# Copy bot scripts
COPY steve.py karen.py /app/

# Create data and screenshots directories
RUN mkdir -p /data /data/screenshots

# Default to Steve (override with command in k8s)
CMD ["python3", "/app/steve.py"]
