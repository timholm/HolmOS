FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /build
COPY main.go .

RUN go mod init audiobook-web && \
    go get github.com/lib/pq && \
    go mod tidy && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o audiobook-web .

FROM public.ecr.aws/docker/library/alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /build/audiobook-web .

EXPOSE 8080

ENTRYPOINT ["./audiobook-web"]
