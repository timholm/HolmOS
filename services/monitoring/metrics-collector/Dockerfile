FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod ./

# Download dependencies (none in this case, but good practice)
RUN go mod download

# Copy source code
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o metrics-collector .

# Final stage - scratch image
FROM scratch

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/metrics-collector .

# Expose port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/app/metrics-collector"]
