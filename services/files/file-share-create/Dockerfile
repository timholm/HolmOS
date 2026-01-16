# Build stage
FROM --platform=linux/arm64 golang:1.22-alpine AS builder

WORKDIR /app

COPY main.go .

RUN go mod init file-share-create && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o file-share-create main.go

# Runtime stage
FROM --platform=linux/arm64 alpine:3.19

RUN apk --no-cache add ca-certificates tzdata && \
    adduser -D -s /bin/sh appuser

WORKDIR /app

COPY --from=builder /app/file-share-create .

RUN mkdir -p /data && chown appuser:appuser /data

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./file-share-create"]
