FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /app

COPY main.go .

RUN go build -o file-decompress main.go

FROM public.ecr.aws/docker/library/alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/file-decompress .

EXPOSE 8080

CMD ["./file-decompress"]
