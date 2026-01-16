# Build stage - using public.ecr.aws mirror to avoid Docker Hub rate limits
FROM --platform=linux/arm64 public.ecr.aws/docker/library/golang:1.22-alpine AS builder

WORKDIR /app

COPY main.go .

RUN go mod init file-preview && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o file-preview main.go

# Runtime stage - using scratch for minimal image
FROM scratch

WORKDIR /app

COPY --from=builder /app/file-preview .

EXPOSE 8080

ENTRYPOINT ["./file-preview"]
