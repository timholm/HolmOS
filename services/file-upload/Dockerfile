FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o file-upload .

FROM scratch
WORKDIR /app
COPY --from=builder /app/file-upload .
EXPOSE 8080
ENTRYPOINT ["./file-upload"]
