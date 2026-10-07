#!/usr/bin/env bash
# Cockpit 防火墙 M1 浏览器侧验收探针（docs/guide/firewall-design.md）。
#
# 与 probe.sh（数据面）/probe-hosts.sh（容器宿主形态）互补：本脚本起真实
# Chrome（headless-shell 容器）驱动 Web 三态页，覆盖验收清单「防火墙观测
# （M1）」节剩余四项的页面呈现——
#
#   W0  server（静态托管 web/dist）+ 无防火墙工具裸容器 + Chrome 容器
#   W1  登录 → W2 空态（仅有无工具主机在线 → 「暂无带防火墙工具的在线主机」）
#   W3  起三台防火墙 agent（legacy/非 root/超大规则集）→ 总览表 + 页顶 4MB 截断告警
#   W4  legacy 面板：规则明细 + cockpit 标注（cockpit: 注释种子规则）
#   W5  非 root 面板：防火墙规则集不可读说明态
#   W6  超大规则集面板：meta-only 错误说明（不误报「规则集为空」）
#
# 浏览器驱动 node scripts/acceptance/firewall/webprobe.cjs（Node ≥22 内置
# WebSocket 手写 CDP，零依赖），分 empty/full 两段调用——空态必须先于
# 防火墙 agent 存在。
#
# 使用：
#   ./scripts/acceptance/firewall/probe-web.sh
#   FW_KEEP=1 ./scripts/acceptance/firewall/probe-web.sh   # 保留现场排查
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/firewall/probe-web.log + fw-*.png

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-fw-web-$$"
SERVER_HOST="0.0.0.0"
SERVER_PORT="${FW_PORT:-19999}"
REACH_HOST="127.0.0.1"
SERVER_URL="http://${REACH_HOST}:${SERVER_PORT}"
AGENT_WS="ws://host.docker.internal:${SERVER_PORT}/ws"
CDP_PORT="${FW_CDP_PORT:-9225}"
ADMIN_USER="admin"
ADMIN_PASS="fw-acc-strong-pass-1"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/firewall"
LOG_FILE="$EVIDENCE_DIR/probe-web.log"
WEBPROBE="$ROOT_DIR/scripts/acceptance/firewall/webprobe.cjs"

log() { printf '\033[1;36m[fww]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[fww:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""
CONTAINERS=(fw-web-bare fw-web-legacy fw-web-noroot fw-web-big fw-web-chrome)

cleanup() {
  for c in "${CONTAINERS[@]}"; do
    docker rm -f "$c" >/dev/null 2>&1 || true
  done
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  if [[ "${FW_KEEP:-0}" == "1" ]]; then
    log "保留现场：${WORK_DIR}"
  else
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT INT TERM

command -v node >/dev/null || { err "需要 node ≥22（内置 WebSocket）"; exit 1; }
docker image inspect chromedp/headless-shell:stable >/dev/null 2>&1 || {
  err "缺少 chromedp/headless-shell:stable 镜像"; exit 1; }

mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$EVIDENCE_DIR"
: > "$LOG_FILE"

# ---- W0: 构建 + server（静态托管 web/dist）+ 登录 ----
log "W0: building server + static agent..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/cockpit-agent" ./cmd/cockpit-agent)

if [[ ! -f "$ROOT_DIR/web/dist/index.html" ]]; then
  log "W0: web/dist 缺失，构建中（1-2 分钟）..."
  (cd "$ROOT_DIR/web" && CI=true pnpm run build >/dev/null)
fi

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
  static_dir: ${ROOT_DIR}/web/dist
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: fw-acc-jwt-secret
  expiration: 1h
EOF

log "W0: starting server on ${SERVER_URL}..."
ADMIN_USERNAME="$ADMIN_USER" \
ADMIN_PASSWORD="$ADMIN_PASS" \
"$WORK_DIR/bin/cockpit" server -config "$WORK_DIR/cockpit.yaml" >>"$WORK_DIR/server.log" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 30); do
  if curl -fsS "${SERVER_URL}/health" >/dev/null 2>&1; then break; fi
  sleep 1
  if [[ $i -eq 30 ]]; then
    err "server 未在 30s 内健康"; tail -n 50 "$WORK_DIR/server.log" >&2; exit 1
  fi
done
TOKEN=$(curl -fsS -X POST "${SERVER_URL}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
  | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$TOKEN" ]] || { err "登录失败"; exit 1; }
log "W0: server healthy（静态托管 web/dist）"

start_container_agent() {
  local name="$1" user="${2:-}"
  docker cp "$WORK_DIR/bin/cockpit-agent" "$name:/usr/local/bin/cockpit-agent" >/dev/null
  if [[ -n "$user" ]]; then
    docker exec -d -u "$user" "$name" sh -c \
      "/usr/local/bin/cockpit-agent start -server ${AGENT_WS} -id ${name} >/tmp/agent.log 2>&1"
  else
    docker exec -d "$name" sh -c \
      "/usr/local/bin/cockpit-agent start -server ${AGENT_WS} -id ${name} >/tmp/agent.log 2>&1"
  fi
}

wait_agent() {
  local id="$1"
  for i in $(seq 1 60); do
    if curl -fsS "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" 2>/dev/null | python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
sys.exit(0 if any(a.get('id') == '$id' and a.get('status') == 'online' for a in agents) else 1)
" 2>/dev/null; then
      log "agent ${id} online"
      return 0
    fi
    sleep 1
  done
  err "agent ${id} 未在 60s 内注册在线"
  return 1
}

# ---- Chrome 容器（CDP；socat 占 9222 的坑 → 显式 9225） ----
log "W0: starting headless-shell Chrome (CDP :${CDP_PORT})..."
docker run -d --name fw-web-chrome --network host \
  chromedp/headless-shell:stable --remote-debugging-port=${CDP_PORT} >/dev/null
for i in $(seq 1 30); do
  if curl -fsS "http://${REACH_HOST}:${CDP_PORT}/json/version" >/dev/null 2>&1; then break; fi
  sleep 1
  if [[ $i -eq 30 ]]; then
    err "Chrome CDP 未在 30s 内就绪"; docker logs fw-web-chrome 2>&1 | tail -20 >&2; exit 1
  fi
done
log "W0: Chrome CDP ready"

# ---- 第一段：空态（仅裸容器在线） ----
log "W2: bare container online (no firewall tools)..."
docker run -d --name fw-web-bare --add-host=host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
start_container_agent fw-web-bare
wait_agent fw-web-bare

if ! node "$WEBPROBE" --web "$SERVER_URL" --cdp "http://${REACH_HOST}:${CDP_PORT}" \
  --user "$ADMIN_USER" --pass "$ADMIN_PASS" --phase empty --ev "$EVIDENCE_DIR" 2>&1 | tee -a "$LOG_FILE"; then
  err "浏览器探针 empty 段失败"; exit 1
fi

# ---- 第二段：三台防火墙 agent ----
log "W3: starting legacy / non-root / big-ruleset containers..."

# legacy：--no-install-recommends（iptables 的 Recommends 会拉 nftables，
# D2 按 nft 优先如实选后端——场景失真）+ alternatives 全家切 legacy
docker run -d --name fw-web-legacy --cap-add NET_ADMIN \
  --add-host=host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
docker exec fw-web-legacy sh -c \
  'apt-get update -qq >/dev/null 2>&1 && \
   apt-get install -y -qq --no-install-recommends iptables >/dev/null 2>&1 && \
   for a in iptables iptables-save iptables-restore; do \
     update-alternatives --set "$a" "/usr/sbin/${a}-legacy" >/dev/null 2>&1 || true; \
   done && \
   iptables -A INPUT -i lo -j ACCEPT && \
   iptables -A INPUT -p tcp --dport 22 -j ACCEPT && \
   iptables -A INPUT -p tcp --dport 80 -m comment --comment "cockpit:browse-accept" -j ACCEPT && \
   iptables -A INPUT -p tcp --dport 443 -j DROP'
start_container_agent fw-web-legacy
wait_agent fw-web-legacy

# 非 root：工具在但 agent 以 nobody 跑——capability 照报、读数失败（说明态）
docker run -d --name fw-web-noroot --add-host=host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
docker exec fw-web-noroot sh -c \
  'apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq --no-install-recommends iptables >/dev/null 2>&1'
start_container_agent fw-web-noroot nobody
wait_agent fw-web-noroot

# 超大规则集：2 万条 109 字符注释规则（nft comment 上限 128），JSON ~7.7MB > 4MiB
docker run -d --name fw-web-big --cap-add NET_ADMIN \
  --add-host=host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
docker exec fw-web-big sh -c \
  'apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq nftables >/dev/null 2>&1 && \
   PAD=$(printf "fw-acc-big-padding-%090d" 0) && \
   { echo "table inet fwaccbig {"; \
     echo "  chain input { type filter hook input priority 0; policy accept;"; \
     i=0; while [ $i -lt 20000 ]; do \
       echo "    tcp dport $((10000+i)) counter accept comment \"${PAD}-${i}\""; \
       i=$((i+1)); done; \
     echo "  }"; echo "}"; } > /tmp/big.nft && nft -f /tmp/big.nft'
start_container_agent fw-web-big
wait_agent fw-web-big

LEGACY_HOST=$(docker exec fw-web-legacy hostname)
NOROOT_HOST=$(docker exec fw-web-noroot hostname)
BIG_HOST=$(docker exec fw-web-big hostname)

if ! node "$WEBPROBE" --web "$SERVER_URL" --cdp "http://${REACH_HOST}:${CDP_PORT}" \
  --user "$ADMIN_USER" --pass "$ADMIN_PASS" --phase full --ev "$EVIDENCE_DIR" \
  --hosts "legacy=${LEGACY_HOST},noroot=${NOROOT_HOST},big=${BIG_HOST}" 2>&1 | tee -a "$LOG_FILE"; then
  err "浏览器探针 full 段失败"; exit 1
fi

log "All firewall browser checks passed ✓ (W0-W6)"
