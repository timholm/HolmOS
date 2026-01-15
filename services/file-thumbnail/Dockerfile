# Build stage - try gcr.io mirror
FROM --platform=linux/arm64 gcr.io/distroless/static-debian12 AS scratch-base

FROM --platform=linux/arm64 golang:1.23-alpine AS builder

WORKDIR /app

COPY main.go .

RUN go mod init file-thumbnail && \
    go get golang.org/x/image/draw@v0.23.0 && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o file-thumbnail main.go

# Runtime stage - use scratch for minimal size
FROM scratch

COPY --from=builder /app/file-thumbnail /file-thumbnail

EXPOSE 8080

ENTRYPOINT ["/file-thumbnail"]
