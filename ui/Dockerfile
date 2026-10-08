# AzureTRE's UI (Microsoft, MIT licensed; see LICENSE), built once and configured at runtime
# through /config.js, which the deployment mounts from a ConfigMap.
FROM node:24-bookworm-slim AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund --loglevel=error
COPY . .
RUN cp src/config.source.json src/config.json && npx vite build

FROM nginxinc/nginx-unprivileged:1.29-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
# Placeholder; the deployment mounts the environment's config.js over it.
RUN echo 'window.__TRE_CONFIG__ = {};' > /usr/share/nginx/html/config.js
EXPOSE 8080
