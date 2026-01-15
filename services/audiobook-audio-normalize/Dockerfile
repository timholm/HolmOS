# Build stage
FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /app
COPY main.go .
RUN go mod init audiobook-audio-normalize && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o audiobook-audio-normalize .

# Runtime stage - using ubuntu:22.04 for ffmpeg support
FROM public.ecr.aws/ubuntu/ubuntu:22.04

# Install ffmpeg and ca-certificates
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ffmpeg \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/audiobook-audio-normalize .

# Create data directory
RUN mkdir -p /data/audiobooks

EXPOSE 8080

CMD ["./audiobook-audio-normalize"]
