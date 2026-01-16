# Build stage
FROM --platform=linux/arm64 golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum* ./
RUN go mod download || true

# Copy source code
COPY . .

# Initialize module if needed
RUN if [ ! -f go.mod ]; then go mod init event-broker; fi
RUN go mod tidy

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o /event-broker main.go

# Runtime stage
FROM --platform=linux/arm64 alpine:3.19

WORKDIR /app

COPY --from=builder /event-broker .

EXPOSE 8080 4222

USER nobody:nobody

ENTRYPOINT ["./event-broker"]
