FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /workspace

COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o file-encrypt .

FROM public.ecr.aws/docker/library/alpine:3.19

WORKDIR /app

COPY --from=builder /workspace/file-encrypt .

RUN mkdir -p /data

EXPOSE 8080

CMD ["./file-encrypt"]
