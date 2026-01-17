FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY *.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o agents .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/agents .
EXPOSE 8080
CMD ["./agents"]
