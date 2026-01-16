# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod ./

# Download dependencies - allow network retry
RUN go mod download 2>/dev/null || go mod tidy

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o file-convert .

# Runtime stage - Ubuntu base for ImageMagick
FROM ubuntu:22.04

# Avoid prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive

# Install ImageMagick and clean up
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    imagemagick \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Configure ImageMagick policy to allow conversions
RUN sed -i 's/<policy domain="coder" rights="none"/<policy domain="coder" rights="read|write"/g' /etc/ImageMagick-6/policy.xml || true

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/file-convert .

# Create directory for temporary files
RUN mkdir -p /tmp/conversions && chmod 777 /tmp/conversions

# Set user
RUN useradd -r -u 1000 appuser
USER appuser

EXPOSE 8080

CMD ["./file-convert"]
