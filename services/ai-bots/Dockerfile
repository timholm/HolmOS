# AI Bots - Steve and Alice
FROM python:3.11-slim

# Install kubectl
RUN apt-get update && apt-get install -y curl && \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
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
COPY steve.py alice.py /app/

# Create data directory
RUN mkdir -p /data

# Default to Steve (override with command in k8s)
CMD ["python3", "/app/steve.py"]
