FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o file-search .

FROM scratch
WORKDIR /app
COPY --from=builder /app/file-search .
EXPOSE 8080
ENTRYPOINT ["./file-search"]
