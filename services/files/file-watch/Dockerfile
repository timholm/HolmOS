# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY main.go .

RUN go mod init file-watch && \
    go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o file-watch .

# Runtime stage - use scratch for minimal image
FROM scratch

WORKDIR /app

COPY --from=builder /app/file-watch .

EXPOSE 8080

ENTRYPOINT ["./file-watch"]
