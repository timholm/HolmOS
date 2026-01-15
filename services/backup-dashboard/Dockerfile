FROM quay.io/projectquay/golang:1.21 AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY *.go ./

RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o backup-dashboard .

FROM quay.io/prometheus/busybox:latest

WORKDIR /app
COPY --from=builder /app/backup-dashboard .

EXPOSE 8080

CMD ["./backup-dashboard"]
