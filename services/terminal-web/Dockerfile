FROM golang:1.21-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod ./
COPY main.go .
COPY index.html .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o terminal-web .

FROM alpine:latest
RUN apk add --no-cache ca-certificates curl bash
# Install kubectl for ARM64
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/arm64/kubectl" \
    && chmod +x kubectl \
    && mv kubectl /usr/local/bin/
WORKDIR /app
ENV SHELL=/bin/bash
COPY --from=builder /app/terminal-web .
COPY --from=builder /app/index.html .
EXPOSE 8080
CMD ["./terminal-web"]
