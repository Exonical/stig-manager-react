# syntax=docker/dockerfile:1.7
FROM node:22-alpine AS build
ENV PNPM_HOME=/root/.local/share/pnpm \
    PATH=/root/.local/share/pnpm:$PATH
RUN corepack enable && corepack prepare pnpm@9 --activate

WORKDIR /repo
COPY pnpm-workspace.yaml pnpm-lock.yaml package.json* ./
COPY docs/package.json docs/
COPY web/package.json web/
RUN pnpm install --frozen-lockfile --filter @stig-manager-react/web...

COPY web ./web
RUN pnpm --filter @stig-manager-react/web build

FROM nginx:1.27-alpine AS run
COPY --from=build /repo/web/dist /usr/share/nginx/html
COPY web/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
