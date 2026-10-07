#!/usr/bin/env bash
# Cockpit 防火墙 M1 只读观测真机验收探针（docs/guide/firewall-design.md）
#
# 目标：本机（需 nft/iptables 工具与免密 sudo）跑通
#   server 启动 → 三形态 agent 注册 → capability 上报 → firewall.status 读数
#
# 场景：
#   F0  server 启动 + admin 登录
#   F1  root agent（完整 PATH）：firewall capability 上报（nft+iptables 元数据）
#   F2  root agent 读数（正常态）：backend=nftables、真实表/链/规则计数
#   F3  root agent、PATH 摘除 nft（shim 只放行 iptables 三件）：backend=iptables
#       + variant 解析（本机 iptables v1.8.11 (nf_tables)）
#   F4  非 root agent（说明态）：capability 照报、available=false + 真实
#       「you must be root」原因（D8）
#   F5  权限位（D10）：viewer 内置角色可读 200；无 firewall:read 的自定义
#       角色用户 403
#
# 使用：
#   ./scripts/acceptance/firewall/probe.sh
#   FW_KEEP=1 ./scripts/acceptance/firewall/probe.sh   # 保留现场排查
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/firewall/probe.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-fw-$$"
SERVER_HOST="127.0.0.1"
SERVER_PORT="${FW_PORT:-19997}"
SERVER_URL="http://${SERVER_HOST}:${SERVER_PORT}"
WS_URL="ws://${SERVER_HOST}:${SERVER_PORT}/ws"
ADMIN_USER="admin"
ADMIN_PASS="fw-acc-strong-pass-1"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/firewall"

log() { printf '\033[1;34m[fw]\033[0m %s\n' "$*"; }
err() { printf '\033[1;31m[fw:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""
AGENT_PIDS=()

cleanup() {
  # root agent 经 sudo 启动，kill sudo PID 杀不到 root 子进程——
  # 按探针专属 ID 前缀清理（-fw-acc- 唯一命名，不误伤其他进程）
  sudo -n pkill -f "cockpit-agent start -server .* -id fw-acc-" 2>/dev/null || true
  for pid in "${AGENT_PIDS[@]:-}"; do
    [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
  done
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  if [[ "${FW_KEEP:-0}" == "1" ]]; then
    log "保留现场：${WORK_DIR}"
  else
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT INT TERM

[[ -d /usr/sbin ]] || { err "unexpected: no /usr/sbin"; exit 1; }
command -v nft >/dev/null || { err "本机无 nft，探针场景不完整"; exit 1; }
command -v iptables-save >/dev/null || { err "本机无 iptables-save"; exit 1; }
sudo -n true 2>/dev/null || { err "需要免密 sudo（root agent 场景）"; exit 1; }

mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$EVIDENCE_DIR"

# ---- F0: 启动 server + admin 登录 ----
cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: fw-acc-jwt-secret
  expiration: 1h
EOF

log "F0: building binaries..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit ./cmd/cockpit-agent)

log "F0: starting server on ${SERVER_URL}..."
ADMIN_USERNAME="$ADMIN_USER" \
ADMIN_PASSWORD="$ADMIN_PASS" \
"$WORK_DIR/bin/cockpit" server -config "$WORK_DIR/cockpit.yaml" >"$WORK_DIR/server.log" 2>&1 &
SERVER_PID=$!

for i in $(seq 1 30); do
  if curl -fsS "${SERVER_URL}/health" >/dev/null 2>&1; then break; fi
  sleep 1
  if [[ $i -eq 30 ]]; then
    err "server 未在 30s 内健康"; tail -n 50 "$WORK_DIR/server.log" >&2; exit 1
  fi
done

LOGIN_RESP=$(curl -fsS -X POST "${SERVER_URL}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}")
TOKEN=$(echo "$LOGIN_RESP" | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$TOKEN" ]] || { err "登录失败: $LOGIN_RESP"; exit 1; }
log "F0: server healthy + admin token acquired"

# ---- 启动三形态 agent ----
# PATH shim：只放行 iptables 三件（无 nft），验证 D2 后端选择真实回落
mkdir -p "$WORK_DIR/fwshim"
for b in iptables iptables-save ip6tables-save; do
  ln -sf "$(command -v "$b")" "$WORK_DIR/fwshim/$b"
done

log "Starting agents: fw-acc-root (root) / fw-acc-ipt (root, no-nft PATH) / fw-acc-user (non-root)..."
sudo -n "$WORK_DIR/bin/cockpit-agent" start -server "$WS_URL" -id fw-acc-root \
  >"$WORK_DIR/agent-root.log" 2>&1 &
AGENT_PIDS+=($!)

sudo -n env PATH="$WORK_DIR/fwshim:/usr/bin:/bin" \
  "$WORK_DIR/bin/cockpit-agent" start -server "$WS_URL" -id fw-acc-ipt \
  >"$WORK_DIR/agent-ipt.log" 2>&1 &
AGENT_PIDS+=($!)

"$WORK_DIR/bin/cockpit-agent" start -server "$WS_URL" -id fw-acc-user \
  >"$WORK_DIR/agent-user.log" 2>&1 &
AGENT_PIDS+=($!)

# 等待三台注册在线（root agent 的进程属主是 root，kill 需经 sudo）
wait_agent() {
  local id="$1"
  for i in $(seq 1 30); do
    local body
    body=$(curl -fsS "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" 2>/dev/null || echo '[]')
    if echo "$body" | python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
sys.exit(0 if any(a.get('id') == '$id' and a.get('status') == 'online' for a in agents) else 1)
" 2>/dev/null; then
      log "agent ${id} online"
      return 0
    fi
    sleep 1
  done
  err "agent ${id} 未在 30s 内注册在线"
  return 1
}
wait_agent fw-acc-root
wait_agent fw-acc-ipt
wait_agent fw-acc-user

status_of() {
  curl -fsS "${SERVER_URL}/api/agents/$1/firewall/status" -H "Authorization: Bearer $TOKEN"
}

# ---- F1: capability 上报（nft+iptables 元数据） ----
log "F1: checking firewall capability on fw-acc-root..."
curl -fsS "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
a = next((x for x in agents if x.get('id') == 'fw-acc-root'), None)
assert a, 'fw-acc-root missing'
caps = [c for c in (a.get('capabilities') or []) if c.get('type') == 'firewall']
assert caps, 'firewall capability missing: ' + json.dumps(a.get('capabilities'))
meta = caps[0].get('metadata') or {}
assert meta.get('nft') is True and meta.get('iptables') is True, 'metadata = ' + json.dumps(meta)
print('F1 PASS: capability firewall, metadata nft+iptables')
"

# ---- F2: 正常态读数（nftables 后端，真实规则集） ----
log "F2: reading firewall status from fw-acc-root (root, nft backend)..."
status_of fw-acc-root | python3 -c "
import sys, json
s = json.load(sys.stdin)
assert s.get('available') is True, 'available = ' + json.dumps(s.get('available'))
assert s.get('backend') == 'nftables', 'backend = ' + json.dumps(s.get('backend'))
assert 'nft' in (s.get('backendVersion') or ''), 'version = ' + json.dumps(s.get('backendVersion'))
assert s.get('totalRules', 0) >= 1, 'totalRules = ' + json.dumps(s.get('totalRules'))
tables = s.get('tables') or []
assert tables, 'tables empty'
chains = [c for t in tables for c in (t.get('chains') or [])]
assert chains, 'chains empty'
assert s.get('truncated') is False, 'truncated = ' + json.dumps(s.get('truncated'))
print('F2 PASS: nftables backend, %d tables, %d chains, %d rules' % (
    len(tables), len(chains), s.get('totalRules')))
"

# ---- F3: iptables 后端真读（PATH 摘除 nft） ----
log "F3: reading firewall status from fw-acc-ipt (root, iptables backend)..."
status_of fw-acc-ipt | python3 -c "
import sys, json
s = json.load(sys.stdin)
assert s.get('available') is True, 'available = ' + json.dumps(s)
assert s.get('backend') == 'iptables', 'backend = ' + json.dumps(s.get('backend'))
assert s.get('iptablesVariant') == 'nf_tables', 'variant = ' + json.dumps(s.get('iptablesVariant'))
assert s.get('totalRules', 0) >= 1, 'totalRules = ' + json.dumps(s.get('totalRules'))
tables = s.get('tables') or []
fams = sorted({t.get('family') for t in tables})
assert 'ipv4' in fams, 'families = ' + json.dumps(fams)
print('F3 PASS: iptables backend (nf_tables variant), %d tables, families %s, %d rules' % (
    len(tables), fams, s.get('totalRules')))
"

# ---- F4: 说明态（非 root：真实 Permission denied） ----
log "F4: reading firewall status from fw-acc-user (non-root)..."
status_of fw-acc-user | python3 -c "
import sys, json
s = json.load(sys.stdin)
assert s.get('available') is False, 'available = ' + json.dumps(s.get('available'))
assert 'must be root' in (s.get('error') or ''), 'error = ' + json.dumps(s.get('error'))
print('F4 PASS: available=false with real permission error (D8)')
"

# ---- F5: 权限位（viewer 200 / 无 firewall:read 角色 403） ----
log "F5: permission bit checks (D10)..."
curl -fsS -X POST "${SERVER_URL}/api/roles" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"fwless","permissions":["inventory:read"]}' >/dev/null
curl -fsS -X POST "${SERVER_URL}/api/users" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"username":"fw-acc-deny","password":"fw-acc-pass-1","role":"fwless"}' >/dev/null
curl -fsS -X POST "${SERVER_URL}/api/users" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"username":"fw-acc-view","password":"fw-acc-pass-1","role":"viewer"}' >/dev/null

DENY_TOKEN=$(curl -fsS -X POST "${SERVER_URL}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"fw-acc-deny","password":"fw-acc-pass-1"}' | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
VIEW_TOKEN=$(curl -fsS -X POST "${SERVER_URL}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"fw-acc-view","password":"fw-acc-pass-1"}' | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$DENY_TOKEN" && -n "$VIEW_TOKEN" ]] || { err "F5 用户登录失败"; exit 1; }

denied=$(curl -s -o /dev/null -w "%{http_code}" "${SERVER_URL}/api/agents/fw-acc-root/firewall/status" \
  -H "Authorization: Bearer $DENY_TOKEN")
[[ "$denied" == "403" ]] || { err "F5: 无 firewall:read 用户期望 403，实际 $denied"; exit 1; }
log "F5: custom role without firewall:read -> 403"

viewed=$(curl -s -o /dev/null -w "%{http_code}" "${SERVER_URL}/api/agents/fw-acc-root/firewall/status" \
  -H "Authorization: Bearer $VIEW_TOKEN")
[[ "$viewed" == "200" ]] || { err "F5: viewer 期望 200，实际 $viewed"; exit 1; }
log "F5: built-in viewer (firewall:read) -> 200"

log "All firewall M1 checks passed ✓ (F0-F5)"
