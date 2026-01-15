# Test Dashboard - HolmOS Comprehensive Testing Hub
# Multi-stage build for ARM64 (Raspberry Pi)

FROM --platform=linux/arm64 golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod file first for better caching
COPY go.mod .

# Download dependencies (if any)
RUN go mod download 2>/dev/null || true

# Copy source files
COPY main.go .
COPY static/ static/

# Build the binary
# CGO_ENABLED=0 for static binary
# GOOS=linux GOARCH=arm64 for Raspberry Pi
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
    -ldflags="-w -s -X main.version=2.0.0" \
    -o test-dashboard .

# Final stage - minimal runtime image
FROM --platform=linux/arm64 alpine:3.19

# Install CA certificates for HTTPS requests (GitHub API)
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for security
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/test-dashboard .

# Copy static files (embedded, but keeping for reference)
COPY --from=builder /build/static/ static/

# Set ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Environment variables
ENV PORT=8080
ENV CLUSTER_HOST=holm.svc.cluster.local
# GITHUB_REPO and GITHUB_TOKEN should be set via Kubernetes secrets

# Run the application
ENTRYPOINT ["/app/test-dashboard"]
