FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY auth-gateway .
RUN chmod +x auth-gateway
EXPOSE 8080
CMD ["./auth-gateway"]
