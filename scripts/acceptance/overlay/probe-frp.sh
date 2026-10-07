#!/usr/bin/env bash
# Cockpit 组网观测 frp 分级真机验收（acceptance-checklist「组网观测」节：
# 零配置只出版本+进程；配 admin 地址后隧道计数出现）。本机无 frp，容器造形态：
# debian:12 容器内跑真 frps + frpc（GitHub release 二进制），agent 同容器——
# frp 就运行在 agent 所在主机，观测面全部真实。
#
#   O0  容器编排：frps（bind 7100 + admin 7400）+ frpc（一条 tcp proxy +
#       admin 7500）真实起进程、真实注册
#   O1  零配置分级：agent 不配 COCKPIT_FRPC_ADMIN/FRPS_ADMIN → frp tool
#       status=ok + version + frpc/frps running=true，且无 tunnels/proxies 键
#   O2  admin 计数：配双 admin 地址 → frpc.tunnels ≥1、frps.proxies ≥1
#   O3  降级分级：COCKPIT_FRPC_ADMIN 指死端口 → status=degraded +
#       frpc.adminError 有值（frps 侧不受影响 proxies 照出）
#
# 使用：
#   ./scripts/acceptance/overlay/probe-frp.sh
#   OVL_KEEP=1 ./scripts/acceptance/overlay/probe-frp.sh   # 保留容器现场
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/overlay/probe-frp.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SERVER_HOST="0.0.0.0"  # 容器 agent 经 host.docker.internal 拨入，须听 0.0.0.0（nginx 探针同先例）
SERVER_PORT="${OVL_PORT:-19997}"
SERVER_URL="http://${SERVER_HOST}:${SERVER_PORT}"
AGENT_ID="ovl-acc-frp"
CT="ovl-acc-frp"
FRP_VER="0.61.2"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/overlay"
LOG_FILE="$EVIDENCE_DIR/probe-frp.log"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-ovl-frp-$$"

log() { printf '\033[1;36m[ovl]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[ovl:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""

cleanup() {
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  if [[ "${OVL_KEEP:-0}" == "1" ]]; then
    log "保留现场：容器 ${CT} 与 ${WORK_DIR}"
  else
    docker rm -f "$CT" >/dev/null 2>&1 || true
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT INT TERM

command -v docker >/dev/null || { err "需要 docker"; exit 1; }
mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$EVIDENCE_DIR"
: > "$LOG_FILE"

# ---- S0: build + server ----
log "O0: building server + agent (agent static——容器 glibc 与宿主不同)..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/cockpit-agent" ./cmd/cockpit-agent)

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: ovl-acc-jwt-secret
  expiration: 1h
EOF

log "O0: starting server on ${SERVER_URL}..."
ADMIN_USER="admin" ADMIN_PASSWORD="ovl-acc-strong-pass-1" \
"$WORK_DIR/bin/cockpit" server -config "$WORK_DIR/cockpit.yaml" >>"$WORK_DIR/server.log" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 30); do
  if curl -fsS "${SERVER_URL}/health" >/dev/null 2>&1; then break; fi
  sleep 1
  if [[ $i -eq 30 ]]; then
    err "server 未在 30s 内健康"; tail -n 50 "$WORK_DIR/server.log" >&2; exit 1
  fi
done

TOKEN=$(curl -fsSL -X POST "${SERVER_URL}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"ovl-acc-strong-pass-1"}' \
  | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$TOKEN" ]] || { err "登录失败"; exit 1; }

# ---- O0: 容器编排——真 frps + 真 frpc ----
log "O0: starting container ${CT} (debian:12)..."
docker rm -f "$CT" >/dev/null 2>&1 || true
docker run -d --name "$CT" --add-host host.docker.internal:host-gateway debian:12 \
  sleep infinity >/dev/null

log "O0: installing deps + frp ${FRP_VER}..."
docker exec "$CT" sh -c "apt-get update -qq && apt-get install -y -qq --no-install-recommends wget ca-certificates tar procps >/dev/null"
docker exec "$CT" sh -c "wget -q https://github.com/fatedier/frp/releases/download/v${FRP_VER}/frp_${FRP_VER}_linux_amd64.tar.gz -O /tmp/frp.tgz && tar -C /tmp -xzf /tmp/frp.tgz && install /tmp/frp_${FRP_VER}_linux_amd64/frpc /tmp/frp_${FRP_VER}_linux_amd64/frps /usr/local/bin/ && frpc -v && frps -v"

docker exec "$CT" sh -c 'cat > /etc/frps.toml <<EOF
bindAddr = "127.0.0.1"
bindPort = 7100
webServer.addr = "127.0.0.1"
webServer.port = 7400
EOF'
docker exec "$CT" sh -c 'cat > /etc/frpc.toml <<EOF
serverAddr = "127.0.0.1"
serverPort = 7100
loginFailExit = false
webServer.addr = "127.0.0.1"
webServer.port = 7500

[[proxies]]
name = "web"
type = "tcp"
localIP = "127.0.0.1"
localPort = 80
remotePort = 7200
EOF'

# frps 先起（frpc 首连撞上未监听会 loginFailExit 永久退出）
docker exec -d "$CT" sh -c 'frps -c /etc/frps.toml >/var/log/frps.log 2>&1'
sleep 1
docker exec -d "$CT" sh -c 'frpc -c /etc/frpc.toml >/var/log/frpc.log 2>&1'
sleep 3
FRP_UP=$(docker exec "$CT" sh -c 'pgrep -x frps >/dev/null && pgrep -x frpc >/dev/null && echo FRP-UP || echo FRP-DOWN')
[[ "$FRP_UP" == "FRP-UP" ]] || { err "frps/frpc 未起来"; docker exec "$CT" sh -c 'cat /var/log/frps.log /var/log/frpc.log' >&2; exit 1; }
log "O0: frps + frpc running (bind 7100 / admin 7400+7500 / proxy web->7200)"

# agent 入容器（admin 环境变量按阶段在 docker run 时注入）
copy_agent() {
  docker cp "$WORK_DIR/bin/cockpit-agent" "$CT:/usr/local/bin/cockpit-agent"
  docker exec "$CT" chmod +x /usr/local/bin/cockpit-agent
}
start_agent() { # start_agent "ENV_K=V ENV_K2=V2"
  docker exec -d "$CT" env $1 COCKPIT_DRIFT_BASELINE=/tmp/drift-baseline.json \
    /usr/local/bin/cockpit-agent start \
    -server "ws://host.docker.internal:${SERVER_PORT}/ws" -id "$AGENT_ID" \
    >>"$WORK_DIR/agent.log" 2>&1
}
kill_agent() {
  docker exec "$CT" pkill -f "cockpit-agent start" >/dev/null 2>&1 || true
  sleep 1
}
wait_agent() {
  for i in $(seq 1 60); do
    ONLINE=$(curl -fsSL "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" 2>/dev/null | \
      python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
print(any(a.get('id') == '${AGENT_ID}' and a.get('status') == 'online' for a in agents))
" || echo "False")
    [[ "$ONLINE" == "True" ]] && return 0
    sleep 1
  done
  err "agent 未在 60s 内注册在线"; tail -n 30 "$WORK_DIR/agent.log" >&2; return 1
}

api() { # api METHOD PATH -> API_CODE / API_BODY
  local resp
  resp=$(curl -sSL -X "$1" "${SERVER_URL}$2" -H "Authorization: Bearer $TOKEN" -w '\n%{http_code}')
  API_CODE=$(echo "$resp" | tail -n 1)
  API_BODY=$(echo "$resp" | sed '$d')
}
PASS=0; FAIL=0
check() {
  if [[ "$2" == "OK" ]]; then
    PASS=$((PASS+1)); log "[PASS] $1 — $3"
  else
    FAIL=$((FAIL+1)); log "[FAIL] $1 — $3"
  fi
}
# frp_tool：从 overlay.status 里摘 frp tool 的 JSON
frp_tool() {
  api GET "/api/agents/${AGENT_ID}/overlay/status"
  echo "$API_BODY" | python3 -c "
import json, sys
d = json.load(sys.stdin)
for t in d.get('tools', []):
    if t.get('tool') == 'frp':
        print(json.dumps(t))
        break
"
}

copy_agent

# ---- O1 零配置分级：无 admin env → 版本+进程，无 tunnels/proxies ----
log "O1: agent without admin env..."
kill_agent
start_agent ""
wait_agent
O1=$(frp_tool | python3 -c "
import json, sys
t = json.loads(sys.stdin.read() or '{}')
extra = t.get('extra') or {}
frpc = extra.get('frpc') or {}
frps = extra.get('frps') or {}
ok = (t.get('status') == 'ok'
      and bool(t.get('version'))
      and frpc.get('running') is True
      and frps.get('running') is True
      and 'tunnels' not in frpc and 'proxies' not in frps
      and 'adminError' not in frpc and 'adminError' not in frps)
print('OK' if ok else 'BAD')
" || echo BAD)
check "O1 零配置：status=ok + version + 双进程 running + 无 tunnels/proxies 键" \
  "$O1" "$(frp_tool | head -c 300)"

# ---- O2 admin 计数：双 admin 地址 → tunnels/proxies ≥1 ----
log "O2: agent with live admin env..."
kill_agent
start_agent "COCKPIT_FRPC_ADMIN=127.0.0.1:7500 COCKPIT_FRPS_ADMIN=127.0.0.1:7400"
wait_agent
O2=$(frp_tool | python3 -c "
import json, sys
t = json.loads(sys.stdin.read() or '{}')
extra = t.get('extra') or {}
frpc = extra.get('frpc') or {}
frps = extra.get('frps') or {}
ok = (t.get('status') == 'ok'
      and isinstance(frpc.get('tunnels'), int) and frpc.get('tunnels') >= 1
      and isinstance(frps.get('proxies'), int) and frps.get('proxies') >= 1)
print('OK' if ok else 'BAD')
" || echo BAD)
check "O2 admin 计数：frpc.tunnels >=1 + frps.proxies >=1" \
  "$O2" "$(frp_tool | head -c 300)"

# ---- O3 降级分级：frpc admin 死端口 → degraded + adminError，frps 侧照常 ----
log "O3: agent with dead frpc admin..."
kill_agent
start_agent "COCKPIT_FRPC_ADMIN=127.0.0.1:7499 COCKPIT_FRPS_ADMIN=127.0.0.1:7400"
wait_agent
O3=$(frp_tool | python3 -c "
import json, sys
t = json.loads(sys.stdin.read() or '{}')
extra = t.get('extra') or {}
frpc = extra.get('frpc') or {}
frps = extra.get('frps') or {}
ok = (t.get('status') == 'degraded'
      and bool(frpc.get('adminError'))
      and isinstance(frps.get('proxies'), int) and frps.get('proxies') >= 1)
print('OK' if ok else 'BAD')
" || echo BAD)
check "O3 降级：死端口 → status=degraded + frpc.adminError（frps.proxies 不受影响）" \
  "$O3" "$(frp_tool | head -c 300)"

log "overlay frp acceptance: PASS=${PASS} FAIL=${FAIL}"
[[ "$FAIL" -eq 0 ]] || exit 1
log "All OVL-FRP checks passed ✓ (O1-O3)"
