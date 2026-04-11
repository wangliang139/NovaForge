FROM golang:1.25-bookworm AS backend-build

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends \
    build-essential gcc git ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /workspace/server

ENV GOPROXY=https://goproxy.cn,https://goproxy.io,https://proxy.golang.org,direct
ENV GOSUMDB=sum.golang.google.cn

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server ./
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o /out/app ./cmd/app/main.go


# 使用 Debian/glibc 基础镜像：esbuild 官方预编译为 glibc；Alpine(musl) 或 linux/amd64+QEMU 下易出现
# Go 运行时崩溃（如 lfstack.push invalid packing）。若在 Apple Silicon 上仍失败，可加：
#   docker build --platform linux/arm64 ...
FROM node:20-bookworm AS frontend-build

WORKDIR /workspace/frontend
RUN corepack enable && corepack prepare pnpm@latest --activate

ENV CI=true
ENV NODE_OPTIONS=--max-old-space-size=8192

COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY frontend ./
RUN pnpm run build


FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    nginx ca-certificates curl && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=backend-build /out/app /app/app
COPY --from=frontend-build /workspace/frontend/dist /var/www/html
COPY docker/nginx.conf /etc/nginx/nginx.conf
COPY docker/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

# default ports: frontend=8000, server=3000, healthcheck=8080, metrics=4014
EXPOSE 8000 3000 8080 4014

ENTRYPOINT ["/entrypoint.sh"]
