FROM alpine:3.19
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY holm-git /app/
RUN chmod +x /app/holm-git && mkdir -p /data/repos
EXPOSE 8080
CMD ["/app/holm-git"]
