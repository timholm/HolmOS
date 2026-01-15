FROM python:3.11-slim

WORKDIR /app

# Install Python dependencies
RUN pip install --no-cache-dir flask kubernetes

# Copy application
COPY app.py .

EXPOSE 80

CMD ["python", "app.py"]
