FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY go.mod .
COPY main.go .
COPY static/ static/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o test-dashboard .

FROM scratch
WORKDIR /app
COPY --from=builder /build/test-dashboard .
COPY --from=builder /build/static/ static/
EXPOSE 8080
ENTRYPOINT ["/app/test-dashboard"]
