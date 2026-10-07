#!/usr/bin/env bash
# Cockpit 磁盘健康（SMART）真机验收探针（docs/guide/disk-health-design.md）。
#
# 本机带物理盘（/dev/sda）但 smartctl 读数需 root（非 root 只回空壳 JSON），
# agent 以 sudo -n 拉起（probe-hosts modprobe 同先例）；本机为 VM 虚拟盘，
# smartctl 只有 health=passed、无温度传感器无 attr 表——断言面相应为：
#
#   S1  盘发现：available=true + lsblk 物理盘齐（/dev/sda）
#   S2  健康态：health 与 smartctl -H 一致（本机健康盘 = passed）
#   S3  温度通道一致：有传感器时 0-100 且相等；本机无传感器 → 双侧同缺
#   S4  attr 缺失不伪造：重映射/待定扇区在 ground truth 无 attr 表时，
#       面板同样缺省（不造 0 值冒充真读）
#
# 未清项注记：attr 数值面（重映射/待定扇区/NVMe media_errors 的非零真值）
# 需带真 attr 表的主机（NAS/物理机），与本机无关；146（FAILED 盘告警）
# 需真坏盘不可造，去重逻辑单测覆盖；147（非 root sudo 提权读取）需改宿主
# sudoers，不做系统级变更——均维持挂起。
#
# 使用：
#   ./scripts/acceptance/smart/probe.sh
#   SMART_KEEP=1 ./scripts/acceptance/smart/probe.sh   # 保留现场排查
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/smart/probe.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-smart-$$"
SERVER_HOST="127.0.0.1"
SERVER_PORT="${SMART_PORT:-19995}"
SERVER_URL="http://${SERVER_HOST}:${SERVER_PORT}"
AGENT_ID="smart-acc-local"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/smart"
LOG_FILE="$EVIDENCE_DIR/probe.log"

log() { printf '\033[1;36m[smt]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[smt:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""
AGENT_PID=""

cleanup() {
  # root agent 杀不掉就用端口唯一模式兜底（$AGENT_PID 是 sudo 的 pid，
  # cui 的信号到不了里面的 root 进程）
  if [[ -n "$AGENT_PID" ]]; then
    sudo -n pkill -f "cockpit-agent start -server ws://${SERVER_HOST}:${SERVER_PORT}" 2>/dev/null || true
    kill "$AGENT_PID" 2>/dev/null || true
  fi
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  if [[ "${SMART_KEEP:-0}" == "1" ]]; then
    log "保留现场：${WORK_DIR}"
  else
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT INT TERM

command -v smartctl >/dev/null || { err "本机无 smartctl，前置不满足"; exit 1; }
command -v python3 >/dev/null || { err "需要 python3"; exit 1; }

mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$EVIDENCE_DIR"
: > "$LOG_FILE"

# ---- 构建 + server + 本机 agent ----
log "S0: building server + agent..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit ./cmd/cockpit-agent)

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: smart-acc-jwt-secret
  expiration: 1h
EOF

log "S0: starting server on ${SERVER_URL}..."
ADMIN_USER="admin" ADMIN_PASSWORD="smart-acc-strong-pass-1" \
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
  -d '{"username":"admin","password":"smart-acc-strong-pass-1"}' \
  | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$TOKEN" ]] || { err "登录失败"; exit 1; }

log "S0: starting local agent (${AGENT_ID}, root——smartctl 读数需 root)..."
sudo -n env COCKPIT_DRIFT_BASELINE="$WORK_DIR/drift-baseline.json" \
  "$WORK_DIR/bin/cockpit-agent" start \
  -server "ws://${SERVER_HOST}:${SERVER_PORT}/ws" -id "$AGENT_ID" \
  >>"$WORK_DIR/agent.log" 2>&1 &
AGENT_PID=$!

for i in $(seq 1 60); do
  if curl -fsS "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" 2>/dev/null | \
    grep -q "\"id\":\"${AGENT_ID}\""; then
    ONLINE=$(curl -fsS "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" | \
      python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
print(any(a.get('id') == '${AGENT_ID}' and a.get('status') == 'online' for a in agents))
")
    [[ "$ONLINE" == "True" ]] && break
  fi
  sleep 1
  if [[ $i -eq 60 ]]; then
    err "agent 未在 60s 内注册在线"; tail -n 30 "$WORK_DIR/agent.log" >&2; exit 1
  fi
done
log "S0: agent online"

# ---- ground truth：smartctl 直读（root——非 root 只回空壳 JSON）----
GT=$(sudo -n smartctl --json -H -A /dev/sda 2>/dev/null || true)
GT_HEALTH=$(python3 -c "
import json,sys
d = json.loads(sys.stdin.read() or '{}')
passed = d.get('smart_status', {}).get('passed')
print('passed' if passed is True else ('failed' if passed is False else 'N/A'))
" <<<"$GT")
GT_TEMP=$(python3 -c "
import json,sys
d = json.loads(sys.stdin.read() or '{}')
t = d.get('temperature', {})
print(t.get('current', 'MISSING'))
" <<<"$GT")
GT_HAS_ATTR=$(python3 -c "
import json,sys
d = json.loads(sys.stdin.read() or '{}')
print('YES' if d.get('ata_smart_attributes', {}).get('table') else 'NO')
" <<<"$GT")
GT_REALLOC=$(python3 -c "
import json,sys
d = json.loads(sys.stdin.read() or '{}')
for a in d.get('ata_smart_attributes', {}).get('table', []):
    if a.get('id') == 5:
        print(a.get('raw', {}).get('value', 'MISSING')); break
else:
    print('MISSING')
" <<<"$GT")
GT_PENDING=$(python3 -c "
import json,sys
d = json.loads(sys.stdin.read() or '{}')
for a in d.get('ata_smart_attributes', {}).get('table', []):
    if a.get('id') == 197:
        print(a.get('raw', {}).get('value', 'MISSING')); break
else:
    print('MISSING')
" <<<"$GT")
log "S0: ground truth health=${GT_HEALTH} temp=${GT_TEMP} attr_table=${GT_HAS_ATTR} realloc=${GT_REALLOC} pending=${GT_PENDING}"

# ---- S1-S4: REST 快照断言 ----
PASS=0; FAIL=0
check() {
  if [[ "$2" == "OK" ]]; then
    PASS=$((PASS+1)); log "[PASS] $1 — $3"
  else
    FAIL=$((FAIL+1)); log "[FAIL] $1 — $3"
  fi
}

DEVJSON="$WORK_DIR/dev.json"
SNAP=$(curl -fsS "${SERVER_URL}/api/agents/${AGENT_ID}/smart/status" \
  -H "Authorization: Bearer $TOKEN")
python3 -c "
import json, sys
d = json.loads(sys.stdin.read())
assert d.get('available') is True, 'available != true'
devs = [x for x in d.get('devices', []) if x.get('name') == '/dev/sda']
assert len(devs) == 1, '/dev/sda missing'
open(sys.argv[1], 'w').write(json.dumps(devs[0]))
" "$DEVJSON" <<<"$SNAP"

HEALTH=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1])).get('health',''))" "$DEVJSON")
MODEL=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1])).get('model','')[:32])" "$DEVJSON")
check "S1 盘发现（available + /dev/sda）" \
  "$( [[ "$HEALTH" != "" && "$MODEL" != "" ]] && echo OK || echo BAD )" \
  "model=${MODEL} sizeBytes=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1])).get('sizeBytes',0))" "$DEVJSON")"

check "S2 健康态与 smartctl -H 一致（passed）" \
  "$( [[ "$HEALTH" == "$GT_HEALTH" && "$HEALTH" == "passed" ]] && echo OK || echo BAD )" \
  "panel=${HEALTH} ground_truth=${GT_HEALTH}"

TEMP=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1])).get('temperatureC','MISSING'))" "$DEVJSON")
check "S3 温度通道一致（本机 VM 盘无传感器 → 双侧同缺）" \
  "$( python3 -c "
import sys
panel, gt = sys.argv[1], sys.argv[2]
if panel == 'MISSING' and gt in ('MISSING', '0'):
    sys.exit(0)
if panel != 'MISSING' and gt != 'MISSING' and panel == gt and 0 <= int(panel) <= 100:
    sys.exit(0)
sys.exit(1)
" "$TEMP" "$GT_TEMP" && echo OK || echo BAD )" \
  "panel=${TEMP} ground_truth=${GT_TEMP}（omitempty：0 值不渲染）"

REALLOC=$(python3 -c "import json,sys;v=json.load(open(sys.argv[1])).get('reallocatedSectors');print('MISSING' if v is None else v)" "$DEVJSON")
PENDING=$(python3 -c "import json,sys;v=json.load(open(sys.argv[1])).get('pendingSectors');print('MISSING' if v is None else v)" "$DEVJSON")
check "S4 attr 缺失不伪造（ground truth 无 attr 表 → 面板同缺）" \
  "$( python3 -c "
import sys
has_attr, realloc, pending, gt_realloc, gt_pending = sys.argv[1:6]
if has_attr == 'NO':
    sys.exit(0 if realloc == 'MISSING' and pending == 'MISSING' else 1)
sys.exit(0 if realloc == gt_realloc and pending == gt_pending else 1)
" "$GT_HAS_ATTR" "$REALLOC" "$PENDING" "$GT_REALLOC" "$GT_PENDING" && echo OK || echo BAD )" \
  "attr_table=${GT_HAS_ATTR} panel realloc=${REALLOC} pending=${PENDING}"

log "smart acceptance: PASS=${PASS} FAIL=${FAIL}"
[[ "$FAIL" -eq 0 ]] || exit 1
log "All SMART checks passed ✓ (S1-S4)"
