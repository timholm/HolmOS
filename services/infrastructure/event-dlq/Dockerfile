# Build stage
FROM --platform=linux/arm64 golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .

RUN if [ ! -f go.mod ]; then go mod init event-dlq; fi
RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o /event-dlq main.go

# Runtime stage
FROM --platform=linux/arm64 alpine:3.19

WORKDIR /app

COPY --from=builder /event-dlq .

EXPOSE 8080

USER nobody:nobody

ENTRYPOINT ["./event-dlq"]
