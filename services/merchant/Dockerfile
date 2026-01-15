# Merchant - App Store AI Agent
# Dockerfile for ARM64 (Raspberry Pi cluster)

FROM docker.io/library/python:3.11-slim

# Set working directory
WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY app.py .

# Expose port
EXPOSE 30005

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:30005/health')" || exit 1

# Run as non-root user for security
RUN useradd -m -u 1000 merchant
USER merchant

# Start the application
CMD ["python", "app.py"]
