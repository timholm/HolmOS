FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -o pxe-server .

FROM alpine:latest
RUN apk --no-cache add \
    dnsmasq \
    tftp-hpa \
    nginx \
    curl \
    wget \
    ca-certificates \
    xz

# Create directories
RUN mkdir -p /tftpboot/pxelinux.cfg /var/www/html/autoinstall /iso

# Download x86_64 SYSLINUX for PXE boot (these are architecture-independent boot files for x86 clients)
RUN cd /tmp && \
    wget -q https://mirrors.edge.kernel.org/pub/linux/utils/boot/syslinux/syslinux-6.03.tar.xz && \
    tar -xf syslinux-6.03.tar.xz && \
    cp syslinux-6.03/bios/core/pxelinux.0 /tftpboot/ && \
    cp syslinux-6.03/bios/com32/elflink/ldlinux/ldlinux.c32 /tftpboot/ && \
    cp syslinux-6.03/bios/com32/libutil/libutil.c32 /tftpboot/ && \
    cp syslinux-6.03/bios/com32/menu/menu.c32 /tftpboot/ && \
    cp syslinux-6.03/bios/com32/lib/libcom32.c32 /tftpboot/ && \
    rm -rf /tmp/syslinux*

WORKDIR /app
COPY --from=builder /app/pxe-server .
COPY dnsmasq.conf /etc/dnsmasq.conf
COPY start.sh /start.sh
RUN chmod +x /start.sh

EXPOSE 67/udp 69/udp 8080 80

CMD ["/start.sh"]
