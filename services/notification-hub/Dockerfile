# Build stage
FROM --platform=linux/arm64 golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o /notification-hub main.go

# Runtime stage
FROM --platform=linux/arm64 alpine:3.19

WORKDIR /app

COPY --from=builder /notification-hub .

EXPOSE 8080

USER nobody:nobody

ENTRYPOINT ["./notification-hub"]
