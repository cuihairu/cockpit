#!/usr/bin/env bash
# Cockpit DDNS 巡检间隔语义真机验收（acceptance-checklist「DDNS」节 124：
# 巡检间隔修改即时生效；`0=关闭` 形态）。
# 零外部依赖形态：配置指 ghost agent + 不配 Cloudflare token——检查在
# provider/agent 前置校验处快速失败，但 CheckedAt/LastStatus/LastError
# 回写在任何错误路径都走（runDDNSCheck），用 CheckedAt 推进做扫描观测。
# 真 CF 记录比对维持挂起（需真 token，见清单 120-123）。
#
#   D1  校验面与默认值：GET 默认 300；59/86401 拒 400；非法 JSON 同拒
#   D2  interval=60 到点扫描：CheckedAt 落地 + failed 回写（LastError 非空）
#   D3  连续推进：第二个扫描窗 CheckedAt 前移（≈60s 节奏，即时生效①）
#   D4  0=关闭：PUT 0 后跨 ≥1 个原扫描窗 CheckedAt 冻结
#   D5  改回再生效：PUT 60 后 CheckedAt 恢复推进（即时生效②）
#
# 使用：
#   ./scripts/acceptance/ddns/probe-scan.sh
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/ddns/probe-scan.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SERVER_HOST="127.0.0.1"
SERVER_PORT="${DDN_PORT:-20002}"
SERVER_URL="http://127.0.0.1:${SERVER_PORT}"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/ddns"
LOG_FILE="$EVIDENCE_DIR/probe-scan.log"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-ddn-$$"

log() { printf '\033[1;36m[ddn]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[ddn:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""

cleanup() {
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  if [[ "${DDN_KEEP:-0}" == "1" ]]; then
    log "保留现场：${WORK_DIR}"
  else
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT INT TERM

mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$EVIDENCE_DIR"
: > "$LOG_FILE"

# ---- build + server ----
log "D0: building server (无需 agent——巡检循环纯 server 侧)..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: ddn-jwt-secret
  expiration: 1h
EOF

log "D0: starting server on ${SERVER_URL}..."
ADMIN_USER="admin" ADMIN_PASSWORD="ddn-strong-pass-1" \
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
  -d '{"username":"admin","password":"ddn-strong-pass-1"}' \
  | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$TOKEN" ]] || { err "登录失败"; exit 1; }

api() { # api METHOD PATH [BODY] -> API_CODE / API_BODY
  local resp
  resp=$(curl -sSL -X "$1" "${SERVER_URL}$2" -H "Authorization: Bearer $TOKEN" \
    ${3:+-H "Content-Type: application/json" -d "$3"} -w '\n%{http_code}')
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
cfg_field() { # cfg_field ID FIELD -> 值（ddns 配置 JSON 单字段）
  api GET "/api/ddns"
  echo "$API_BODY" | FIELD="$2" ID="$1" python3 -c "
import json, os, sys
cfgs = json.loads(sys.stdin.read())
if isinstance(cfgs, dict):
    cfgs = cfgs.get('configs') or cfgs.get('items') or []
for c in cfgs:
    if c.get('id') == int(os.environ['ID']):
        v = c.get(os.environ['FIELD'])
        print('' if v is None else v)
        break
"
}
wait_checked_at_gt() { # wait_checked_at_gt ID BASE DEADLINE_SEC -> 0=推进
  local deadline=$(( $(date +%s) + $3 ))
  while [[ $(date +%s) -lt $deadline ]]; do
    local cur
    cur=$(cfg_field "$1" checkedAt)
    if [[ -n "$cur" && "$cur" -gt "$2" ]] 2>/dev/null; then
      echo "$cur"; return 0
    fi
    sleep 5
  done
  return 1
}

# ---- D1 校验面与默认值 ----
api GET "/api/ddns/config"
D1=$(C="$API_CODE" B="$API_BODY" python3 -c "
import json, os, sys
if os.environ['C'] != '200':
    sys.exit(1)
d = json.loads(os.environ['B'])
sys.exit(0 if d.get('scan_interval_seconds') == 300 else 1)" && echo OK || echo BAD)
check "D1 默认间隔 300（未配置回默认）" "$D1" "code=${API_CODE} body=${API_BODY:0:80}"
api PUT "/api/ddns/config" '{"scan_interval_seconds": 59}'
R59="$API_CODE"
api PUT "/api/ddns/config" '{"scan_interval_seconds": 86401}'
RMAX="$API_CODE"
api PUT "/api/ddns/config" '{"scan_interval_seconds": "abc"}'
RBAD="$API_CODE"
api GET "/api/ddns/config"
D1V=$(C1="$R59" C2="$RMAX" C3="$RBAD" C4="$API_CODE" B="$API_BODY" python3 -c "
import json, os, sys
ok = (os.environ['C1'] == '400' and os.environ['C2'] == '400'
      and os.environ['C3'] == '400' and os.environ['C4'] == '200'
      and json.loads(os.environ['B']).get('scan_interval_seconds') == 300)
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "D1 校验面：59/86401/非数字 拒 400，合法值不被污染" \
  "$D1V" "59→${R59} 86401→${RMAX} abc→${RBAD} after=${API_BODY:0:60}"

# ---- D2 interval=60 到点扫描 ----
api PUT "/api/ddns/config" '{"scan_interval_seconds": 60}'
[[ "$API_CODE" == "200" ]] || { err "PUT interval=60 失败: $API_BODY"; exit 1; }
api POST "/api/ddns" '{"agentId":"ddns-ghost","zoneId":"zone-d2","zoneName":"example.com","recordName":"d2.example.com","type":"A","enabled":true}'
[[ "$API_CODE" == "200" ]] || { err "配置创建失败: $API_BODY"; exit 1; }
CFG_ID=$(echo "$API_BODY" | python3 -c 'import sys, json; print(json.load(sys.stdin)["id"])')
log "D2: config id=${CFG_ID} (ghost agent, 无 CF token——前置校验快速失败路径)"
C1=""
if C1=$(wait_checked_at_gt "$CFG_ID" 0 150); then :; else C1=""; fi
ERR1=$(cfg_field "$CFG_ID" lastError)
STATUS1=$(cfg_field "$CFG_ID" lastStatus)
D2=$(A="$C1" S="$STATUS1" E="$ERR1" python3 -c "
import os, sys
ok = (os.environ['A'] != ''           # CheckedAt 落地（扫描发生过）
      and os.environ['S'] == 'failed' # ghost/provider 快速失败回写
      and len(os.environ['E']) > 0)
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "D2 interval=60 到点扫描：CheckedAt 落地 + failed/LastError 回写" \
  "$D2" "checkedAt=${C1:-none} status=${STATUS1} err=${ERR1:0:60}"
[[ -n "$C1" ]] || { err "D2 首扫未发生，后续无意义"; exit 1; }

# ---- D3 连续推进（≈60s 节奏） ----
if C2=$(wait_checked_at_gt "$CFG_ID" "$C1" 150); then :; else C2=""; fi
GAP=$(( ${C2:-0} - C1 ))
D3=$(A="$C2" G="$GAP" python3 -c "
import os, sys
ok = (os.environ['A'] != ''
      and 30 <= int(os.environ['G']) <= 150)   # 60s 节奏 ± 容差
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "D3 连续推进：第二扫描窗 CheckedAt 前移（间隔修改生效①）" \
  "$D3" "c1=${C1} c2=${C2:-none} gap=${GAP}s"
[[ -n "$C2" ]] || { err "D3 第二扫未发生"; exit 1; }

# ---- D4 0=关闭 ----
api PUT "/api/ddns/config" '{"scan_interval_seconds": 0}'
[[ "$API_CODE" == "200" ]] || { err "PUT interval=0 失败: $API_BODY"; exit 1; }
sleep 100   # 跨 ≥1 个原扫描窗（tick=60s）
C2_AFTER=$(cfg_field "$CFG_ID" checkedAt)
D4=$(A="$C2_AFTER" B2="$C2" python3 -c "
import os, sys
sys.exit(0 if os.environ['A'] == os.environ['B2'] else 1)" && echo OK || echo BAD)
check "D4 0=关闭：跨 ≥1 个原扫描窗 CheckedAt 冻结（100s）" \
  "$D4" "c2=${C2} after100s=${C2_AFTER}"

# ---- D5 改回再生效 ----
api PUT "/api/ddns/config" '{"scan_interval_seconds": 60}'
[[ "$API_CODE" == "200" ]] || { err "PUT interval=60 失败: $API_BODY"; exit 1; }
if C3=$(wait_checked_at_gt "$CFG_ID" "$C2" 150); then :; else C3=""; fi
D5=$(A="$C3" B2="$C2" python3 -c "
import os, sys
sys.exit(0 if os.environ['A'] != '' and int(os.environ['A']) > int(os.environ['B2']) else 1)" && echo OK || echo BAD)
check "D5 改回 60 再生效：CheckedAt 恢复推进（间隔修改生效②）" \
  "$D5" "c2=${C2} c3=${C3:-none}"

log "ddns scan acceptance: PASS=${PASS} FAIL=${FAIL}"
[[ "$FAIL" -eq 0 ]] || exit 1
log "All DDN checks passed ✓ (D1-D5)"
