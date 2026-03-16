FROM node:25.8.1-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm install -g npm@11.11.0 && npm ci

COPY . .
RUN npm run build

FROM nginx:1.27-alpine

COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 8080

CMD ["nginx", "-g", "daemon off;"]
