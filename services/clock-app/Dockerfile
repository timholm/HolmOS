FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go .
COPY index.html .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o clock-app .

FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/clock-app .
COPY --from=builder /app/index.html .
EXPOSE 30007
CMD ["./clock-app"]
