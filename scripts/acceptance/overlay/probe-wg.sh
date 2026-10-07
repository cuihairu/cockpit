#!/usr/bin/env bash
# Cockpit 组网观测 WireGuard 真机验收（acceptance-checklist「组网观测」节
# 138 的 wg 半：真实 peers/interfaces 快照 + 私钥/预共享密钥剥离）。
# 本机无 wg，NET_ADMIN 容器造形态：wireguard-tools 建真 wg0 接口
# （内核 wireguard 模块宿主可用，NET_ADMIN 下内核侧自动加载）+ 真 peer
# （含预共享密钥）——dump 全真，断言面板白名单不漏敏感列。
#
#   W1  真实快照：wireguard tool status=ok + version + wg0 接口
#       （listenPort=51820、peerCount=1、peer endpoint/allowed-ips 原样）
#   W2  密钥剥离：整份响应不含私钥与预共享密钥 hex（D6 白名单）
#   W3  数据保全：peer 公钥（公开数据）原样出现
#
# 边界注记：跨 agent 同节点合并 / ZT·TS 真实 peers 需真 zerotier/tailscale
# 网络（云端/daemon 登录），维持挂起——本探针只关 wg 半。
#
# 使用：
#   ./scripts/acceptance/overlay/probe-wg.sh
#   OVL_KEEP=1 ./scripts/acceptance/overlay/probe-wg.sh   # 保留容器现场
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/overlay/probe-wg.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SERVER_HOST="0.0.0.0"  # 容器 agent 经 host.docker.internal 拨入（nginx 探针同先例）
SERVER_PORT="${OVL_PORT:-19998}"
SERVER_URL="http://127.0.0.1:${SERVER_PORT}"
AGENT_ID="ovl-acc-wg"
CT="ovl-acc-wg"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/overlay"
LOG_FILE="$EVIDENCE_DIR/probe-wg.log"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-ovl-wg-$$"

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
log "W0: building server + agent (agent static——容器 glibc 与宿主不同)..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/cockpit-agent" ./cmd/cockpit-agent)

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: ovl-wg-jwt-secret
  expiration: 1h
EOF

log "W0: starting server on ${SERVER_URL}..."
ADMIN_USER="admin" ADMIN_PASSWORD="ovl-wg-strong-pass-1" \
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
  -d '{"username":"admin","password":"ovl-wg-strong-pass-1"}' \
  | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$TOKEN" ]] || { err "登录失败"; exit 1; }

# ---- W0: 容器编排——真 wg0 接口 + 真 peer（含 PSK）----
log "W0: starting container ${CT} (NET_ADMIN, debian:12)..."
docker rm -f "$CT" >/dev/null 2>&1 || true
docker run -d --name "$CT" --cap-add NET_ADMIN \
  --add-host host.docker.internal:host-gateway debian:12 \
  sleep infinity >/dev/null

log "W0: installing wireguard-tools + iproute2..."
docker exec "$CT" sh -c "apt-get update -qq && apt-get install -y -qq --no-install-recommends wireguard-tools iproute2 >/dev/null"

docker exec "$CT" sh -c '
set -e
wg genkey > /tmp/priv.key
wg pubkey < /tmp/priv.key > /tmp/pub.key
wg genkey > /tmp/peer_priv.key
wg pubkey < /tmp/peer_priv.key > /tmp/peer_pub.key
wg genpsk > /tmp/psk.key
ip link add dev wg0 type wireguard
wg set wg0 private-key /tmp/priv.key listen-port 51820
wg set wg0 peer "$(cat /tmp/peer_pub.key)" preshared-key /tmp/psk.key \
  endpoint 203.0.113.1:51820 allowed-ips 10.0.0.2/32,fd00::2/128 persistent-keepalive 25
ip addr add 10.0.0.1/24 dev wg0
ip link set wg0 up
echo "PRIV=$(cat /tmp/priv.key)"
echo "PSK=$(cat /tmp/psk.key)"
echo "PUB=$(cat /tmp/peer_pub.key)"
' > "$WORK_DIR/wg-keys.txt"
PRIV_KEY=$(grep '^PRIV=' "$WORK_DIR/wg-keys.txt" | cut -d= -f2)
PSK_KEY=$(grep '^PSK=' "$WORK_DIR/wg-keys.txt" | cut -d= -f2)
PUB_KEY=$(grep '^PUB=' "$WORK_DIR/wg-keys.txt" | cut -d= -f2)
[[ -n "$PRIV_KEY" && -n "$PSK_KEY" && -n "$PUB_KEY" ]] || { err "密钥生成失败"; exit 1; }
WG_OK=$(docker exec "$CT" sh -c 'ip link show wg0 >/dev/null && wg show wg0 >/dev/null && echo WG-UP || echo WG-DOWN')
[[ "$WG_OK" == "WG-UP" ]] || { err "wg0 未起来"; docker exec "$CT" ip link >&2 || true; exit 1; }
log "W0: wg0 up (listen 51820, 1 peer + PSK, endpoint 203.0.113.1:51820)"

# agent 入容器
docker cp "$WORK_DIR/bin/cockpit-agent" "$CT:/usr/local/bin/cockpit-agent"
docker exec "$CT" chmod +x /usr/local/bin/cockpit-agent
docker exec -d "$CT" env COCKPIT_DRIFT_BASELINE=/tmp/drift-baseline.json \
  /usr/local/bin/cockpit-agent start \
  -server "ws://host.docker.internal:${SERVER_PORT}/ws" -id "$AGENT_ID" \
  >>"$WORK_DIR/agent.log" 2>&1

for i in $(seq 1 60); do
  ONLINE=$(curl -fsSL "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" 2>/dev/null | \
    python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
print(any(a.get('id') == '${AGENT_ID}' and a.get('status') == 'online' for a in agents))
" || echo "False")
  [[ "$ONLINE" == "True" ]] && break
  sleep 1
  if [[ $i -eq 60 ]]; then
    err "agent 未在 60s 内注册在线"; tail -n 30 "$WORK_DIR/agent.log" >&2; exit 1
  fi
done
log "W0: agent online"

# ---- W1-W3 断言 ----
PASS=0; FAIL=0
check() {
  if [[ "$2" == "OK" ]]; then
    PASS=$((PASS+1)); log "[PASS] $1 — $3"
  else
    FAIL=$((FAIL+1)); log "[FAIL] $1 — $3"
  fi
}

API_CODE=""
API_BODY=""
api() { # api METHOD PATH -> API_CODE / API_BODY
  local resp
  resp=$(curl -sSL -X "$1" "${SERVER_URL}$2" -H "Authorization: Bearer $TOKEN" -w '\n%{http_code}')
  API_CODE=$(echo "$resp" | tail -n 1)
  API_BODY=$(echo "$resp" | sed '$d')
}
wg_tool() {
  api GET "/api/agents/${AGENT_ID}/overlay/status"
  echo "$API_BODY" | python3 -c "
import json, sys
d = json.load(sys.stdin)
for t in d.get('tools', []):
    if t.get('tool') == 'wireguard':
        print(json.dumps(t))
        break
"
}

WG_JSON=$(wg_tool)

# W1 真实快照
W1=$(WG_JSON="$WG_JSON" python3 -c "
import json, os, sys
t = json.loads(os.environ['WG_JSON'])
ifaces = t.get('interfaces') or []
wg0 = [i for i in ifaces if i.get('name') == 'wg0']
ok = (t.get('status') == 'ok' and bool(t.get('version')) and len(wg0) == 1)
if ok:
    i = wg0[0]
    ok = (i.get('listenPort') == '51820' and i.get('peerCount') == 1)
    peers = i.get('peers') or []
    if ok and len(peers) == 1:
        p = peers[0]
        ok = (p.get('endpoint') == '203.0.113.1:51820'
              and '10.0.0.2/32' in (p.get('virtualIps') or [])
              and 'fd00::2/128' in (p.get('virtualIps') or []))
print('OK' if ok else 'BAD')
" || echo BAD)
check "W1 真实快照：wg0 + listenPort=51820 + peer endpoint/allowed-ips 原样" \
  "$W1" "$(echo "$WG_JSON" | head -c 400)"

# W2 密钥剥离：响应不含私钥/PSK hex
W2=$(WG_JSON="$WG_JSON" PRIV_KEY="$PRIV_KEY" PSK_KEY="$PSK_KEY" python3 -c "
import os, sys
body = os.environ['WG_JSON']
leak = os.environ['PRIV_KEY'].strip() in body or os.environ['PSK_KEY'].strip() in body
print('BAD' if leak else 'OK')
" || echo BAD)
check "W2 密钥剥离：私钥与预共享密钥 hex 不在响应（D6 白名单）" \
  "$W2" "priv/psk hex $(echo "$WG_JSON" | grep -q "$PRIV_KEY" && echo LEAKED || echo absent)/$(echo "$WG_JSON" | grep -q "$PSK_KEY" && echo LEAKED || echo absent)"

# W3 数据保全：peer 公钥原样出现
W3=$(WG_JSON="$WG_JSON" PUB_KEY="$PUB_KEY" python3 -c "
import os, sys
print('OK' if os.environ['PUB_KEY'].strip() in os.environ['WG_JSON'] else 'BAD')
" || echo BAD)
check "W3 数据保全：peer 公钥（公开数据）原样出现" \
  "$W3" "pubkey=$(echo "$PUB_KEY" | head -c 12)..."

log "overlay wg acceptance: PASS=${PASS} FAIL=${FAIL}"
[[ "$FAIL" -eq 0 ]] || exit 1
log "All OVL-WG checks passed ✓ (W1-W3)"
