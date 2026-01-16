FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o user-preferences .

# Final stage - use scratch for minimal image
FROM scratch

# Copy the binary from builder
COPY --from=builder /app/user-preferences /user-preferences

EXPOSE 8080

ENTRYPOINT ["/user-preferences"]
