# syntax=docker/dockerfile:1

FROM node:24-alpine AS build
WORKDIR /src

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npx ng build --configuration production

FROM nginx:1.27-alpine AS final
COPY --from=build /src/dist/frontend/browser /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

# 127.0.0.1 e não localhost: dentro do container "localhost" resolve também
# para ::1, o wget do busybox tenta o IPv6 primeiro e o nginx escuta apenas
# em IPv4 — o container ficava eternamente unhealthy servindo normalmente.
HEALTHCHECK --interval=5s --timeout=3s --retries=5 \
  CMD wget -qO- http://127.0.0.1:80/health || exit 1
