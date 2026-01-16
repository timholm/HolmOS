FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY main.go .

# Build for ARM64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o auth-gateway .

# Runtime stage
FROM public.ecr.aws/docker/library/alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/auth-gateway .

EXPOSE 8080
CMD ["./auth-gateway"]
