FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /app

# Install ca-certificates for HTTPS calls
RUN apk add --no-cache ca-certificates

# Copy source
COPY main.go .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o settings-web main.go

# Final stage
FROM public.ecr.aws/docker/library/alpine:3.19

WORKDIR /app

# Install kubectl for system info queries
RUN apk add --no-cache ca-certificates curl && \
    curl -LO "https://dl.k8s.io/release/v1.29.0/bin/linux/arm64/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/

COPY --from=builder /app/settings-web .

EXPOSE 8080

CMD ["./settings-web"]
