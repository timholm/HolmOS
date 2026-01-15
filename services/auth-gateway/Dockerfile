FROM public.ecr.aws/docker/library/busybox:latest
WORKDIR /app
COPY auth-gateway .
RUN chmod +x auth-gateway
EXPOSE 8080
CMD ["./auth-gateway"]
