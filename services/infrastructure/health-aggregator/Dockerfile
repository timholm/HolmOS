# Build stage
FROM --platform=linux/arm64 golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .

RUN if [ ! -f go.mod ]; then go mod init health-aggregator; fi
RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o /health-aggregator main.go

# Runtime stage
FROM --platform=linux/arm64 alpine:3.19

WORKDIR /app

COPY --from=builder /health-aggregator .

EXPOSE 8080

USER nobody:nobody

ENTRYPOINT ["./health-aggregator"]
