FROM --platform=linux/arm64 docker.io/library/python:3.11-slim

WORKDIR /app

RUN useradd -m -s /bin/bash appuser

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

RUN chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

CMD ["python", "app.py"]
