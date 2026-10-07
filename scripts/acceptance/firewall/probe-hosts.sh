#!/usr/bin/env bash
# Cockpit 防火墙 M1 宿主形态探针（docs/guide/firewall-design.md）
#
# 与 probe.sh（本机三形态 agent）互补：本脚本用 docker 容器造出本机没有的
# 宿主形态，覆盖验收清单「防火墙观测（M1）」节剩余三场景——
#
#   H0  server 启动 + admin 登录（端口默认 19998，静态 agent 构建）
#   H1  裸容器（debian:12-slim，无 nft/iptables）：无 firewall capability，
#       firewall/status → 502 unknown provider（页面空态的数据面事实）
#   H2  iptables-legacy 容器（NET_ADMIN + update-alternatives 切 legacy）：
#       backend=iptables + variant=legacy + 真实种子规则读数
#   H3  超大 nft 规则集（>4MB，2 万条长注释规则）：truncated=true +
#       meta-only（tables 空 + error 带 exceeds，D4 nft 语义）
#   H4  超大 iptables-restore 规则集（>4MB，2 万条长注释规则）：
#       truncated=true + 行界截断（tables 非空部分解析，D4 iptables 语义）
#
# 容器内 agent 需静态构建（宿主 glibc 2.43 ≠ debian:12 的 2.36）。
# 容器名 fw-acc-{bare,legacy,big} 探针专属，cleanup 精确删除不碰他人容器。
#
# 使用：
#   ./scripts/acceptance/firewall/probe-hosts.sh
#   FW_KEEP=1 ./scripts/acceptance/firewall/probe-hosts.sh   # 保留现场排查
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/firewall/probe-hosts.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-fw-hosts-$$"
SERVER_HOST="0.0.0.0"
SERVER_PORT="${FW_PORT:-19998}"
REACH_HOST="127.0.0.1"
SERVER_URL="http://${REACH_HOST}:${SERVER_PORT}"
AGENT_WS="ws://host.docker.internal:${SERVER_PORT}/ws"
ADMIN_USER="admin"
ADMIN_PASS="fw-acc-strong-pass-1"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/firewall"
LOG_FILE="$EVIDENCE_DIR/probe-hosts.log"

log() { printf '\033[1;35m[fwh]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[fwh:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""
CONTAINERS=(fw-acc-bare fw-acc-legacy fw-acc-big)

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

mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$EVIDENCE_DIR"
: > "$LOG_FILE"

# ---- H0: 构建静态 agent + 启动 server + admin 登录 ----
log "H0: building server + static agent (CGO_ENABLED=0)..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/cockpit-agent" ./cmd/cockpit-agent)
file "$WORK_DIR/bin/cockpit-agent" | grep -q "statically linked" || {
  err "agent 非静态链接，容器内跑不起来"; exit 1; }

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

log "H0: starting server on ${SERVER_URL}..."
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
log "H0: server healthy + admin token acquired"

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

# 容器 agent 启动模板：拷静态二进制 + 后台启动（日志落容器内 /tmp 便于失败排查）
start_container_agent() {
  local name="$1"
  docker cp "$WORK_DIR/bin/cockpit-agent" "$name:/usr/local/bin/cockpit-agent" >/dev/null
  docker exec -d "$name" sh -c \
    "/usr/local/bin/cockpit-agent start -server ${AGENT_WS} -id ${name} >/tmp/agent.log 2>&1"
}

status_code_of() {
  curl -s -o /dev/null -w "%{http_code}" "${SERVER_URL}/api/agents/$1/firewall/status" \
    -H "Authorization: Bearer $TOKEN"
}

# ---- H1: 裸容器（无防火墙工具） ----
log "H1: bare container (no firewall tools)..."
docker run -d --name fw-acc-bare --add-host=host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
start_container_agent fw-acc-bare
wait_agent fw-acc-bare

curl -fsS "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
a = next((x for x in agents if x.get('id') == 'fw-acc-bare'), None)
assert a, 'fw-acc-bare missing'
caps = [c.get('type') for c in (a.get('capabilities') or [])]
assert 'firewall' not in caps, 'unexpected firewall capability: ' + json.dumps(caps)
print('H1 PASS: bare agent has no firewall capability')
"

code=$(status_code_of fw-acc-bare)
[[ "$code" == "502" ]] || { err "H1: 期望 502（unknown provider），实际 $code"; exit 1; }
log "H1 PASS: firewall/status -> 502 (unknown provider: firewall)"

# ---- H2: iptables-legacy 容器（NET_ADMIN + 切 legacy + 种子规则） ----
log "H2: iptables-legacy container (apt install + update-alternatives)..."
docker run -d --name fw-acc-legacy --cap-add NET_ADMIN \
  --add-host=host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
docker exec fw-acc-legacy sh -c \
  'apt-get update -qq >/dev/null 2>&1 && \
   apt-get install -y -qq --no-install-recommends iptables >/dev/null 2>&1 && \
   ! command -v nft && \
   for a in iptables iptables-save iptables-restore; do \
     update-alternatives --set "$a" "/usr/sbin/${a}-legacy" >/dev/null 2>&1 || true; \
   done; iptables --version' | tee -a "$LOG_FILE"
docker exec fw-acc-legacy iptables --version 2>/dev/null | grep -q "(legacy)" || {
  err "H2: iptables 未切到 legacy"; exit 1; }

# 种子规则（ipv4 三条；ipv6 尽力而为——内核模块缺失不算失败，D2 v6 静默省略语义）；
# iptables-save 必须看得见种子规则（否则 save 仍走 nft 后端，读数对不上 legacy variant）
docker exec fw-acc-legacy sh -c '
  iptables -A INPUT -i lo -j ACCEPT &&
  iptables -A INPUT -p tcp --dport 22 -j ACCEPT &&
  iptables -A INPUT -p tcp --dport 443 -j DROP &&
  (for a in ip6tables ip6tables-save ip6tables-restore; do \
     update-alternatives --set "$a" "/usr/sbin/${a}-legacy" >/dev/null 2>&1 || true; \
   done; ip6tables -A INPUT -p icmpv6 -j ACCEPT 2>/dev/null || true) &&
  iptables-save | grep -q -- "--dport 22"' || {
  err "H2: iptables-save 看不见种子规则（alternatives 未全切？）"; exit 1; }
start_container_agent fw-acc-legacy
wait_agent fw-acc-legacy

status_of() {
  curl -fsS "${SERVER_URL}/api/agents/$1/firewall/status" -H "Authorization: Bearer $TOKEN"
}

status_of fw-acc-legacy | python3 -c "
import sys, json
s = json.load(sys.stdin)
assert s.get('available') is True, 'available = ' + json.dumps(s.get('available'))
assert s.get('backend') == 'iptables', 'backend = ' + json.dumps(s.get('backend'))
assert s.get('iptablesVariant') == 'legacy', 'variant = ' + json.dumps(s.get('iptablesVariant'))
assert s.get('totalRules', 0) >= 3, 'totalRules = ' + json.dumps(s.get('totalRules'))
assert s.get('truncated') is False, 'truncated = ' + json.dumps(s.get('truncated'))
tables = s.get('tables') or []
fams = sorted({t.get('family') for t in tables})
assert 'ipv4' in fams, 'families = ' + json.dumps(fams)
flt = next((t for t in tables if t.get('family') == 'ipv4' and t.get('name') == 'filter'), None)
assert flt, 'ipv4 filter table missing'
inp = next((c for c in flt.get('chains') or [] if c.get('name') == 'INPUT'), None)
assert inp, 'INPUT chain missing'
rules = inp.get('rules') or []
joined = json.dumps(rules)
assert '--dport 22' in joined, 'seed rule dport 22 missing: ' + joined[:400]
assert '--dport 443' in joined, 'seed rule dport 443 missing'
print('H2 PASS: iptables backend, legacy variant, %d rules, families %s' % (
    s.get('totalRules'), fams))
"

# ---- H3: 超大 nft 规则集（>4MB，meta-only 截断） ----
log "H3: big nft ruleset container (2w long-comment rules)..."
docker run -d --name fw-acc-big --cap-add NET_ADMIN \
  --add-host=host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
docker exec fw-acc-big sh -c \
  'apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq nftables >/dev/null 2>&1 && nft --version'

# 2 万条规则，每条带 160 字符注释——JSON 序列化后体积 ~8-10MB，稳过 4MiB
docker exec fw-acc-big sh -c '
  PAD=$(printf "fw-acc-big-padding-%090d" 0)   # nft comment 上限 128 字符
  {
    echo "table inet fwaccbig {"
    echo "  chain input { type filter hook input priority 0; policy accept;"
    i=0
    while [ $i -lt 20000 ]; do
      echo "    tcp dport $((10000+i)) counter accept comment \"${PAD}-${i}\""
      i=$((i+1))
    done
    echo "  }"
    echo "}"
  } > /tmp/big.nft
  nft -f /tmp/big.nft'
docker exec fw-acc-big sh -c 'nft -j list ruleset | wc -c' | tee -a "$LOG_FILE"
BYTES=$(docker exec fw-acc-big sh -c 'nft -j list ruleset | wc -c')
[[ "$BYTES" -gt 4194304 ]] || { err "H3: ruleset 仅 ${BYTES} 字节，未过 4MiB，注释垫料不够"; exit 1; }
start_container_agent fw-acc-big
wait_agent fw-acc-big

status_of fw-acc-big | python3 -c "
import sys, json
s = json.load(sys.stdin)
assert s.get('available') is True, 'available = ' + json.dumps(s.get('available'))
assert s.get('backend') == 'nftables', 'backend = ' + json.dumps(s.get('backend'))
assert s.get('truncated') is True, 'truncated = ' + json.dumps(s.get('truncated'))
assert 'exceeds' in (s.get('error') or ''), 'error = ' + json.dumps(s.get('error'))
assert (s.get('tables') or []) == [], 'tables 应为空（meta-only）: %d tables' % len(s.get('tables') or [])
print('H3 PASS: nftables >4MiB -> truncated=true, meta-only, error=%r' % (
    (s.get('error') or '')[:70]))
"

# ---- H4: 超大 iptables-restore 规则集（>4MB，行界截断） ----
log "H4: flooding fw-acc-legacy with >4MiB iptables-restore..."
docker exec fw-acc-legacy sh -c '
  PAD=$(printf "fw-acc-legacy-padding-%0160d" 0)
  {
    echo "*filter"
    i=0
    while [ $i -lt 20000 ]; do
      echo "-A INPUT -p tcp --dport $((20000+i)) -m comment --comment \"${PAD}-${i}\" -j ACCEPT"
      i=$((i+1))
    done
    echo "COMMIT"
  } > /tmp/big.restore
  iptables-restore < /tmp/big.restore'
docker exec fw-acc-legacy sh -c 'iptables-save | wc -c' | tee -a "$LOG_FILE"
BYTES=$(docker exec fw-acc-legacy sh -c 'iptables-save | wc -c')
[[ "$BYTES" -gt 4194304 ]] || { err "H4: save 仅 ${BYTES} 字节，未过 4MiB"; exit 1; }

status_of fw-acc-legacy | python3 -c "
import sys, json
s = json.load(sys.stdin)
assert s.get('available') is True, 'available = ' + json.dumps(s.get('available'))
assert s.get('backend') == 'iptables', 'backend = ' + json.dumps(s.get('backend'))
assert s.get('truncated') is True, 'truncated = ' + json.dumps(s.get('truncated'))
tables = s.get('tables') or []
assert tables, 'tables 应非空（行界截断仍部分解析）'
flt = next((t for t in tables if t.get('family') == 'ipv4' and t.get('name') == 'filter'), None)
assert flt, 'ipv4 filter missing'
inp = next((c for c in flt.get('chains') or [] if c.get('name') == 'INPUT'), None)
assert inp, 'INPUT missing'
n = len(inp.get('rules') or [])
assert n >= 15000, '截断后 INPUT 规则仅 %d 条，行界截断语义不符' % n
# 原始 3 条种子规则仍在（前 4MB 覆盖不到的行界截断不丢头部）
print('H4 PASS: iptables >4MiB -> truncated=true, line-bound cut kept %d INPUT rules' % n)
"

log "All firewall host-form checks passed ✓ (H0-H4)"
