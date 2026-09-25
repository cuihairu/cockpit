ARG NODE_VERSION=22-bookworm
ARG GO_VERSION=1.26-bookworm
# 镜像版本：CI（.github/workflows/docker.yml）传 tag/分支/sha，本地 build 缺省 dev。
# 注入二进制的 main.version（cmd/cockpit/main.go 的 var，cockpit version 可见）
ARG VERSION=dev

FROM node:${NODE_VERSION} AS web-builder
WORKDIR /src/web

# 仓库无 packageManager 字段，corepack 会挑「已知最新版」pnpm——钉死版本，
# 避免哪天上游发新版把 frozen install 掀了。11.19.0 = 本地/CI 实测通过
# lockfileVersion 9.0 的版本（corepack prepare 在新版 corepack 已废弃，
# 直接走 npm 全局装更稳）
RUN npm install -g pnpm@11.19.0 && pnpm --version
# pnpm-workspace.yaml 必须随 lockfile 一起进镜像：overrides 定义在其中，
# 缺了它会与 lockfile 记录不一致，frozen install 直接报 CONFIG_MISMATCH
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

COPY web ./
RUN pnpm build

FROM golang:${GO_VERSION} AS go-builder
ARG VERSION
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=1：SQLite 走 mattn/go-sqlite3（数据库层锁死在 cgo，见 docs）
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/cockpit ./cmd/cockpit

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl libsqlite3-0 tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && addgroup --system cockpit \
    && adduser --system --ingroup cockpit --home /data --no-create-home cockpit \
    && mkdir -p /app/web /data /etc/cockpit \
    && chown -R cockpit:cockpit /app /data

COPY --from=go-builder /out/cockpit /usr/local/bin/cockpit
COPY --from=web-builder /src/web/dist /app/web
COPY deployments/docker/config.yaml /etc/cockpit/config.yaml
COPY deployments/docker/entrypoint.sh /usr/local/bin/cockpit-entrypoint

RUN chmod 755 /usr/local/bin/cockpit /usr/local/bin/cockpit-entrypoint

ENV COCKPIT_CONFIG=/etc/cockpit/config.yaml
ENV STATIC_DIR=/app/web
ENV TZ=Asia/Shanghai

USER cockpit
WORKDIR /data
EXPOSE 9000

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -fsS http://127.0.0.1:9000/health >/dev/null || exit 1

ENTRYPOINT ["cockpit-entrypoint"]
CMD ["cockpit", "server", "-config", "/etc/cockpit/config.yaml"]
