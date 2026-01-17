FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o pxe-server .

FROM alpine:latest
RUN apk --no-cache add \
    dnsmasq \
    tftp-hpa \
    syslinux \
    nginx \
    curl \
    wget \
    ca-certificates

# Create directories
RUN mkdir -p /tftpboot/pxelinux.cfg /var/www/html/autoinstall /iso

# Copy PXELINUX files
RUN cp /usr/share/syslinux/pxelinux.0 /tftpboot/ && \
    cp /usr/share/syslinux/ldlinux.c32 /tftpboot/ && \
    cp /usr/share/syslinux/libutil.c32 /tftpboot/ && \
    cp /usr/share/syslinux/menu.c32 /tftpboot/ && \
    cp /usr/share/syslinux/libcom32.c32 /tftpboot/

WORKDIR /app
COPY --from=builder /app/pxe-server .
COPY dnsmasq.conf /etc/dnsmasq.conf
COPY start.sh /start.sh
RUN chmod +x /start.sh

EXPOSE 67/udp 69/udp 8080 80

CMD ["/start.sh"]
