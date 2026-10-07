#!/usr/bin/env bash
# Cockpit Stacks 目录自检真机验收（acceptance-checklist「应用部署」节剩余项：
# 无权限场景）。本机 agent 即非 root（cui），无需容器造形态：
#
#   K1  无权限（单 agent）：stacks 目录 root:root 0700 → stack.list 失败，
#       GET /api/stacks/agents/{id} 502，错误体附 info.dirError（permission denied）
#       且 dirWritable=false——目录类故障 dirError 随错误体下发到列表页的链路实证
#   K2  无权限（聚合）：GET /api/stacks 的 agentInfo[denied].dirError 同样带出
#       （web 目录告警联动的实际消费面）
#   K3  对照（单 agent）：目录不存在 → agent MkdirAll 0700 自动创建，
#       dirWritable=true、composeVersion 出现
#   K4  对照（聚合）：agentInfo[ok].dirWritable=true
#
# 使用：
#   ./scripts/acceptance/stacks/probe-dir.sh
#   STACKS_KEEP=1 ./scripts/acceptance/stacks/probe-dir.sh   # 保留现场排查
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/stacks/probe-dir.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-stacks-dir-$$"
SERVER_HOST="127.0.0.1"
SERVER_PORT="${STACKS_PORT:-19996}"
SERVER_URL="http://${SERVER_HOST}:${SERVER_PORT}"
DENIED_DIR="/tmp/cockpit-stacks-denied-$$"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/stacks"
LOG_FILE="$EVIDENCE_DIR/probe-dir.log"

log() { printf '\033[1;36m[stk]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[stk:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""
AGENT_PIDS=()

cleanup() {
  for pid in "${AGENT_PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null || true
  done
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  sudo -n rm -rf "$DENIED_DIR" 2>/dev/null || true
  if [[ "${STACKS_KEEP:-0}" == "1" ]]; then
    log "保留现场：${WORK_DIR}"
  else
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT INT TERM

command -v docker >/dev/null || { err "需要 docker（stacks capability 前提）"; exit 1; }
docker compose version >/dev/null 2>&1 || { err "需要 docker compose CLI"; exit 1; }

mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$EVIDENCE_DIR"
: > "$LOG_FILE"

# ---- S0: build + server + 双 agent（ok / denied）----
log "S0: building server + agent..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit ./cmd/cockpit-agent)

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: stacks-acc-jwt-secret
  expiration: 1h
EOF

log "S0: starting server on ${SERVER_URL}..."
ADMIN_USER="admin" ADMIN_PASSWORD="stacks-acc-strong-pass-1" \
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
  -d '{"username":"admin","password":"stacks-acc-strong-pass-1"}' \
  | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$TOKEN" ]] || { err "登录失败"; exit 1; }

# 无权限目录：root 属主 0700，agent（cui）可 stat 不可写——CreateTemp 探针 EACCES
sudo -n mkdir -p "$DENIED_DIR"
sudo -n chown root:root "$DENIED_DIR"
sudo -n chmod 0700 "$DENIED_DIR"

start_agent() {
  local id="$1" dir="$2"
  COCKPIT_STACKS_DIR="$dir" COCKPIT_DRIFT_BASELINE="$WORK_DIR/drift-baseline.json" \
  "$WORK_DIR/bin/cockpit-agent" start \
    -server "ws://${SERVER_HOST}:${SERVER_PORT}/ws" -id "$id" \
    >>"$WORK_DIR/agent-${id}.log" 2>&1 &
  AGENT_PIDS+=($!)
}

log "S0: starting agents (stacks-acc-ok / stacks-acc-denied)..."
start_agent "stacks-acc-ok" "$WORK_DIR/stacks-ok"
start_agent "stacks-acc-denied" "$DENIED_DIR"

for id in stacks-acc-ok stacks-acc-denied; do
  for i in $(seq 1 60); do
    ONLINE=$(curl -fsS "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" 2>/dev/null | \
      python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
print(any(a.get('id') == '${id}' and a.get('status') == 'online' for a in agents))
" || echo "False")
    [[ "$ONLINE" == "True" ]] && break
    sleep 1
    if [[ $i -eq 60 ]]; then
      err "agent ${id} 未在 60s 内注册在线"; tail -n 30 "$WORK_DIR/agent-${id}.log" >&2; exit 1
    fi
  done
done
log "S0: both agents online"

# ---- K1-K4 断言 ----
PASS=0; FAIL=0
check() {
  if [[ "$2" == "OK" ]]; then
    PASS=$((PASS+1)); log "[PASS] $1 — $3"
  else
    FAIL=$((FAIL+1)); log "[FAIL] $1 — $3"
  fi
}

api() { # api METHOD PATH -> API_CODE / API_BODY（-L：Go mux 对 /api/stacks → /api/stacks/ 有临时重定向）
  local resp
  resp=$(curl -sSL -X "$1" "${SERVER_URL}$2" -H "Authorization: Bearer $TOKEN" -w '\n%{http_code}')
  API_CODE=$(echo "$resp" | tail -n 1)
  API_BODY=$(echo "$resp" | sed '$d')
}

# K1 无权限单 agent：list 失败 → 502 + info.dirError 随错误体下发
api GET "/api/stacks/agents/stacks-acc-denied"
K1=$(API_CODE="$API_CODE" API_BODY="$API_BODY" python3 -c "
import json, os, sys
try:
    d = json.loads(os.environ['API_BODY'])
except Exception:
    sys.exit(1)
info = d.get('info') or {}
ok = (os.environ['API_CODE'] == '502'
      and info.get('dirWritable') is False
      and 'permission denied' in (info.get('dirError') or '')
      and bool(d.get('error')))
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "K1 无权限目录（单 agent）：502 + info.dirError=permission denied + dirWritable=false" \
  "$( [[ "$K1" == "OK" ]] && echo OK || echo BAD )" \
  "code=${API_CODE} body=$(echo "$API_BODY" | head -c 300)"

# K2 无权限聚合：agentInfo[denied].dirError 带出（web 目录告警消费面）
api GET "/api/stacks"
K2=$(API_BODY="$API_BODY" python3 -c "
import json, os, sys
try:
    d = json.loads(os.environ['API_BODY'])
except Exception:
    sys.exit(1)
info = (d.get('agentInfo') or {}).get('stacks-acc-denied') or {}
sys.exit(0 if (info.get('dirWritable') is False
               and 'permission denied' in (info.get('dirError') or '')) else 1)" && echo OK || echo BAD)
check "K2 无权限目录（聚合）：agentInfo[stacks-acc-denied].dirError 带出" \
  "$( [[ "$K2" == "OK" ]] && echo OK || echo BAD )" \
  "body=$(echo "$API_BODY" | python3 -c "import json,sys;d=json.load(sys.stdin);print(json.dumps(d.get('agentInfo',{}),ensure_ascii=False)[:300])" 2>/dev/null || echo "$API_BODY" | head -c 300)"

# K3 对照单 agent：目录自动创建 0700 → dirWritable=true + composeVersion
api GET "/api/stacks/agents/stacks-acc-ok"
K3=$(API_CODE="$API_CODE" API_BODY="$API_BODY" python3 -c "
import json, os, sys
try:
    d = json.loads(os.environ['API_BODY'])
except Exception:
    sys.exit(1)
info = d.get('info') or {}
sys.exit(0 if (os.environ['API_CODE'] == '200'
               and info.get('dirWritable') is True
               and not info.get('dirError')
               and info.get('composeVersion')) else 1)" && echo OK || echo BAD)
check "K3 对照（单 agent）：目录自动创建 → dirWritable=true + composeVersion" \
  "$( [[ "$K3" == "OK" ]] && echo OK || echo BAD )" \
  "code=${API_CODE} info=$(echo "$API_BODY" | python3 -c "import json,sys;print(json.dumps(json.load(sys.stdin).get('info',{}),ensure_ascii=False))" 2>/dev/null || echo parse-fail)"

# K4 对照聚合：agentInfo[ok].dirWritable=true
api GET "/api/stacks"
K4=$(API_BODY="$API_BODY" python3 -c "
import json, os, sys
try:
    d = json.loads(os.environ['API_BODY'])
except Exception:
    sys.exit(1)
info = (d.get('agentInfo') or {}).get('stacks-acc-ok') or {}
sys.exit(0 if info.get('dirWritable') is True else 1)" && echo OK || echo BAD)
check "K4 对照（聚合）：agentInfo[stacks-acc-ok].dirWritable=true" \
  "$( [[ "$K4" == "OK" ]] && echo OK || echo BAD )" \
  "body=$(echo "$API_BODY" | python3 -c "import json,sys;d=json.load(sys.stdin);print(json.dumps(d.get('agentInfo',{}),ensure_ascii=False)[:300])" 2>/dev/null || echo "$API_BODY" | head -c 300)"

log "stacks dir acceptance: PASS=${PASS} FAIL=${FAIL}"
[[ "$FAIL" -eq 0 ]] || exit 1
log "All STACKS-DIR checks passed ✓ (K1-K4)"
