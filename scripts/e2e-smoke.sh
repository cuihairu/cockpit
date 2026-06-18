#!/usr/bin/env bash
# Cockpit 端到端最小冒烟脚本
#
# 目标：用临时目录 + 临时 SQLite 跑通
#   server 启动 → agent 注册 → inventory 同步 → /api/resources/* 可读
#
# 使用：
#   ./scripts/e2e-smoke.sh
#
# 退出码：
#   0  全部步骤通过
#   1  任一关键步骤失败
#
# 依赖：go、curl、jq（可选）、Python 3（仅用于解析 JSON 兜底）

set -euo pipefail

# ---- 配置 ----
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-e2e-$$"
SERVER_HOST="127.0.0.1"
SERVER_PORT="${E2E_PORT:-19990}"
SERVER_URL="http://${SERVER_HOST}:${SERVER_PORT}"
WS_URL="ws://${SERVER_HOST}:${SERVER_PORT}/ws"
ADMIN_USER="admin"
ADMIN_PASS="e2e-strong-pass-1"

log() { printf '\033[1;34m[e2e]\033[0m %s\n' "$*"; }
err() { printf '\033[1;31m[e2e:err]\033[0m %s\n' "$*" >&2; }

cleanup() {
  rc=$?
  log "Cleaning up (exit code=$rc)"
  if [[ $rc -ne 0 ]] && [[ -f "$WORK_DIR/server.log" ]]; then
    err "--- server.log ---"
    tail -n 60 "$WORK_DIR/server.log" >&2 || true
  fi
  [[ -n "${SERVER_PID:-}" ]] && kill "$SERVER_PID" 2>/dev/null || true
  [[ -n "${AGENT_PID:-}" ]] && kill "$AGENT_PID" 2>/dev/null || true
  KEEP_LOGS="${E2E_KEEP_LOGS:-0}"
  if [[ "$KEEP_LOGS" == "1" ]]; then
    log "Keeping logs at $WORK_DIR"
  else
    rm -rf "$WORK_DIR"
  fi
  exit $rc
}
trap cleanup EXIT INT TERM

mkdir -p "$WORK_DIR/data"

# ---- 准备配置 ----
cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: e2e-jwt-secret
  expiration: 1h
EOF

cat > "$WORK_DIR/inventory.yaml" <<EOF
version: "v1"
metadata:
  name: e2e-smoke
  description: End-to-end smoke inventory
domains:
  example.com:
    id: example-com
    domain: example.com
    provider: manual
certificates:
  example-com-cert:
    id: example-com-cert
    domain: example.com
    provider: manual
EOF

cd "$ROOT_DIR"

# ---- 构建二进制 ----
log "Building binaries..."
GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit ./cmd/cockpit-agent

# ---- 启动 server ----
log "Starting server on ${SERVER_URL}..."
# 注意：e2e 场景不设置 ALLOWED_ORIGINS，让 WebSocket 接受任意 Origin（开发模式默认）
# 设置白名单会导致 Agent 拨号时 Origin 不匹配而握手失败
ADMIN_USERNAME="$ADMIN_USER" \
ADMIN_PASSWORD="$ADMIN_PASS" \
"$WORK_DIR/bin/cockpit" server -config "$WORK_DIR/cockpit.yaml" >"$WORK_DIR/server.log" 2>&1 &
SERVER_PID=$!

# 等待 /health
log "Waiting for server health..."
for i in $(seq 1 30); do
  if curl -fsS "${SERVER_URL}/health" >"$WORK_DIR/health.json" 2>/dev/null; then
    log "Server healthy: $(cat "$WORK_DIR/health.json")"
    break
  fi
  sleep 1
  if [[ $i -eq 30 ]]; then
    err "Server did not become healthy within 30s"
    err "Server log tail:"
    tail -n 50 "$WORK_DIR/server.log" >&2 || true
    exit 1
  fi
done

# ---- 启动 agent ----
log "Starting agent..."
"$WORK_DIR/bin/cockpit-agent" start -server "$WS_URL" >"$WORK_DIR/agent.log" 2>&1 &
AGENT_PID=$!

# 等待 agent 注册（最多 15s），通过未授权访问 /api/agents 验证 401 而非 404
log "Waiting for agent registration..."
for i in $(seq 1 15); do
  code=$(curl -s -o /dev/null -w "%{http_code}" "${SERVER_URL}/api/agents" || true)
  if [[ "$code" == "401" ]]; then
    log "API reachable (401 = auth required, route exists)"
    break
  fi
  sleep 1
  if [[ $i -eq 15 ]]; then
    err "Server API not reachable after 15s"
    tail -n 50 "$WORK_DIR/server.log" >&2 || true
    exit 1
  fi
done

# ---- 登录获取 JWT ----
log "Logging in as ${ADMIN_USER}..."
LOGIN_RESP=$(curl -fsS -X POST "${SERVER_URL}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" || echo '{}')

TOKEN=$(echo "$LOGIN_RESP" | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))' 2>/dev/null || true)
if [[ -z "$TOKEN" ]]; then
  err "Login failed or no token in response: $LOGIN_RESP"
  exit 1
fi
log "Got JWT token"

# ---- 验证 agent 在线 ----
log "Verifying agent registration via /api/agents..."
for i in $(seq 1 30); do
  AGENTS=$(curl -fsS "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN")
  COUNT=$(echo "$AGENTS" | python3 -c 'import sys, json; print(len(json.load(sys.stdin) or []))' 2>/dev/null || echo 0)
  if [[ "$COUNT" -gt 0 ]]; then
    log "Agent online. Agents count: ${COUNT}"
    break
  fi
  sleep 1
  if [[ $i -eq 30 ]]; then
    err "Agent failed to register within 30s"
    err "Agent log tail:"
    tail -n 50 "$WORK_DIR/agent.log" >&2 || true
    exit 1
  fi
done

# ---- 触发 inventory sync ----
log "Syncing inventory..."
"$WORK_DIR/bin/cockpit" sync \
  -config "$WORK_DIR/cockpit.yaml" \
  -inventory "$WORK_DIR/inventory.yaml" || {
    err "Inventory sync failed"
    exit 1
  }
log "Inventory sync done"

# ---- 验证 resources API ----
verify_resource() {
  local kind="$1"
  local code
  code=$(curl -s -o /dev/null -w "%{http_code}" "${SERVER_URL}/api/resources/${kind}" \
    -H "Authorization: Bearer $TOKEN")
  if [[ "$code" != "200" ]]; then
    err "GET /api/resources/${kind} returned ${code}"
    exit 1
  fi
  log "GET /api/resources/${kind} -> 200"
}

verify_resource "domains"
verify_resource "certificates"

log "All smoke checks passed ✓"
exit 0
