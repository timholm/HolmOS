FROM golang:1.21-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod ./
COPY main.go .
COPY index.html .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o terminal-web .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/terminal-web .
COPY --from=builder /app/index.html .
EXPOSE 8080
CMD ["./terminal-web"]
