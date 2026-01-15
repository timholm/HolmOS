FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go .
COPY index.html .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o calculator-app .

FROM public.ecr.aws/docker/library/alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/calculator-app .
EXPOSE 8080
CMD ["./calculator-app"]
