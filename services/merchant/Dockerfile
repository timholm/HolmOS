# Claude Terminal - SSH Terminal for HolmOS
# Dockerfile for ARM64 (Raspberry Pi cluster)

FROM --platform=linux/arm64 docker.io/library/python:3.11-slim

# Set working directory
WORKDIR /app

# Install system dependencies for paramiko (SSH)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libffi-dev \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY app.py .

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')" || exit 1

# Run as non-root user for security
RUN useradd -m -u 1000 claude
USER claude

# Start the application
CMD ["python", "app.py"]
