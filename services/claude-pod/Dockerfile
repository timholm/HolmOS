FROM --platform=linux/arm64 public.ecr.aws/docker/library/node:20-alpine

WORKDIR /app

COPY package*.json ./

RUN npm install --production

COPY server.js .

USER node

EXPOSE 30001

ENV PORT=30001

CMD ["node", "server.js"]
