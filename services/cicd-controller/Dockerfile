FROM quay.io/projectquay/golang:1.21 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o cicd-controller .

FROM quay.io/fedora/fedora-minimal:40
RUN microdnf install -y ca-certificates && microdnf clean all
WORKDIR /app
COPY --from=builder /app/cicd-controller .
COPY ui.html .
EXPOSE 8080
CMD ["./cicd-controller"]
