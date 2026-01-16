FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o deploy-controller .

FROM public.ecr.aws/docker/library/alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/deploy-controller .
COPY ui.html .
EXPOSE 8080
CMD ["./deploy-controller"]
