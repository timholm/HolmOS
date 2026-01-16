FROM golang:1.21-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o holm-git .

FROM alpine:3.19
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY --from=builder /app/holm-git .
RUN mkdir -p /data/repos
EXPOSE 8080
CMD ["./holm-git"]
