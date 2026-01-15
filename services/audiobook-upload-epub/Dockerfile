FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /app
COPY main.go .
RUN go mod init audiobook-upload-epub && \
    go get github.com/lib/pq && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o audiobook-upload-epub .

FROM public.ecr.aws/docker/library/alpine:3.19
WORKDIR /app
COPY --from=builder /app/audiobook-upload-epub .
EXPOSE 8080
CMD ["./audiobook-upload-epub"]
