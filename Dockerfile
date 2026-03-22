FROM node:25.8.1-alpine AS build

WORKDIR /app

ARG APP_VERSION=dev
ENV APP_VERSION=${APP_VERSION}
ENV VITE_APP_VERSION=${APP_VERSION}

COPY package.json package-lock.json ./
RUN npm install -g npm@11.11.0 && npm ci

COPY . .
RUN npm run build

FROM caddy:2.10-alpine

COPY docker/Caddyfile /etc/caddy/Caddyfile
COPY --from=build /app/dist /srv
COPY --from=build /app/mobile/manifest.json /srv/mobile/manifest.json

EXPOSE 80 443

CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
