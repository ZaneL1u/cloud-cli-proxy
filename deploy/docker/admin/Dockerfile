FROM node:24-alpine AS build

WORKDIR /app
COPY web/admin/package.json web/admin/package-lock.json* ./
RUN npm install
COPY web/admin/ .
RUN npm run build

FROM nginx:alpine
COPY deploy/docker/admin/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
