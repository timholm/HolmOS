FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o alice-bot .

FROM alpine:latest
RUN apk --no-cache add ca-certificates git
WORKDIR /app
COPY --from=builder /app/alice-bot .
RUN mkdir -p /repo
EXPOSE 8080
CMD ["./alice-bot"]
