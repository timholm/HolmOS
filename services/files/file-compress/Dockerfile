# Build stage - using registry mirror to avoid Docker Hub rate limits
FROM --platform=linux/arm64 golang:1.22-alpine AS builder

WORKDIR /app

COPY main.go .

RUN go mod init file-compress && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o file-compress main.go

# Runtime stage - using gcr.io/distroless for smaller image and no Docker Hub dependency
FROM --platform=linux/arm64 gcr.io/distroless/static:nonroot

WORKDIR /app

COPY --from=builder /app/file-compress .

USER nonroot:nonroot

EXPOSE 8080

CMD ["./file-compress"]
