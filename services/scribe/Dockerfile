FROM public.ecr.aws/docker/library/golang:1.21-alpine AS builder

WORKDIR /app

# Create go.mod with all dependencies
RUN go mod init scribe &&     go get k8s.io/client-go@v0.28.4 &&     go get k8s.io/api@v0.28.4 &&     go get k8s.io/apimachinery@v0.28.4

COPY main.go ./

RUN go mod tidy &&     CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -installsuffix cgo -o scribe .

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /app/scribe /scribe

ENV PORT=8080

EXPOSE 8080

ENTRYPOINT ["/scribe"]
