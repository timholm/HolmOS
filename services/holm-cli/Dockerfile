FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o holm .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/holm .
ENTRYPOINT ["./holm"]
