FROM public.ecr.aws/docker/library/python:3.11-slim

WORKDIR /app

# Install Python dependencies
RUN pip install --no-cache-dir flask cryptography

# Copy application
COPY app.py .

# Create data directory
RUN mkdir -p /data

ENV PORT=8080
EXPOSE 8080

CMD ["python", "app.py"]
