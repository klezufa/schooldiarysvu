FROM node:24.14.0-alpine AS build
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
COPY api/openapi.yaml /api/openapi.yaml
RUN npm run generate && npm run build

FROM nginxinc/nginx-unprivileged:1.29-alpine
# Upstream-образ выходит реже, чем исправления Alpine (Trivy: 47 HIGH/CRITICAL в brotli, curl, openssl, expat на момент
# сборки). Пакеты обновляются при каждой сборке; curl, нужный только для отладки, удаляется.
USER root
RUN apk upgrade --no-cache && apk del --no-cache curl
USER 101
COPY --from=build /app/dist /usr/share/nginx/html
COPY deploy/nginx/security-headers.conf deploy/nginx/locations.conf /etc/nginx/snippets/
# По умолчанию профиль local-http; профиль https подменяет default.conf и profile-headers.conf (docker-compose.https.yml).
COPY deploy/nginx/profile-headers-local.conf /etc/nginx/snippets/profile-headers.conf
COPY deploy/nginx/local-http.conf /etc/nginx/conf.d/default.conf
EXPOSE 8080
