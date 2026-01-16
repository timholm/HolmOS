FROM quay.io/projectquay/golang:1.21 AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o backup-storage .

# Final stage
FROM quay.io/prometheus/busybox:latest

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/backup-storage .

# Expose port
EXPOSE 8080

# Run the application
CMD ["./backup-storage"]
