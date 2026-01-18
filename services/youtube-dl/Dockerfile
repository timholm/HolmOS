FROM python:3.11-slim

# Install yt-dlp and ffmpeg
RUN apt-get update && apt-get install -y \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir yt-dlp flask

WORKDIR /app
COPY app.py .

# Create download directory
RUN mkdir -p /tmp/youtube-downloads

EXPOSE 8080

CMD ["python3", "app.py"]
