# Build stage
FROM --platform=$BUILDPLATFORM public.ecr.aws/docker/library/golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod ./

# Download dependencies
RUN go mod download

# Copy source code
COPY *.go ./

# Build for ARM64
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-arm64} go build -ldflags="-w -s" -o gateway .

# Final stage - minimal image
FROM --platform=$TARGETPLATFORM public.ecr.aws/docker/library/alpine:3.19

WORKDIR /app

# Add ca-certificates for HTTPS calls
RUN apk --no-cache add ca-certificates

# Copy binary
COPY --from=builder /app/gateway .

# Make executable
RUN chmod +x gateway

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run as non-root user
RUN adduser -D -u 1000 appuser
USER appuser

# Run the gateway
CMD ["./gateway"]
