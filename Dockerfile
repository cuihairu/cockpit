ARG NODE_VERSION=22-bookworm
ARG GO_VERSION=1.26-bookworm

FROM node:${NODE_VERSION} AS web-builder
WORKDIR /src/web

RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web ./
RUN pnpm build

FROM golang:${GO_VERSION} AS go-builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cockpit ./cmd/cockpit

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
