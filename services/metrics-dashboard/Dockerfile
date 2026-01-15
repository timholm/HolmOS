FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod file
COPY go.mod ./

# Copy source files
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o metrics-dashboard .

# Final image
FROM public.ecr.aws/docker/library/alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/metrics-dashboard .

EXPOSE 8080

CMD ["./metrics-dashboard"]
