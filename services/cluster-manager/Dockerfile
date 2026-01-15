FROM public.ecr.aws/docker/library/python:3.11-slim

# Install system dependencies including SSH tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    iputils-ping \
    sshpass \
    openssh-client \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

# Environment variables for SSH access (override in deployment)
ENV SSH_USER=rpi1
ENV SSH_PASSWORD=19209746

EXPOSE 8080

CMD ["python", "app.py"]
