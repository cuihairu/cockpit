#!/usr/bin/env bash
# Cockpit NAS 跳板探测真机验收（acceptance-checklist「NAS 系统对接」节 202 的
# 机制半：无本地存储工具的主机可经 COCKPIT_NAS_TARGETS 作跳板观测）。
# debian:12-slim 裸容器 + targets 环境变量——targets 观测路径全真
# （真 DSM/TrueNAS/OMV 远端需真实 NAS，维持挂起，见清单注记）。
#
# 形态注记（真机实证）：①docker 会把宿主 /etc/hosts 等以 bind mount 进容器，
# df 视角下是宿主真实块设备（≥1GB 计入 mounts）——「裸容器」对 df 不裸，
# available 随 mounts 走是正确产品行为；②/proc 为宿主共享，mdstat 恒存在
# → DetectNas 的 mdstat 分支在 Linux 上恒真，nas capability 恒上报——
# env 注册分支只在无 mdstat 的平台（Windows/macOS 跳板）可见，本探针断言
# 观测路径而非 capability 位。
#
#   J1  跳板观测路径：合法 targets（一条 dsm 条目指死端口）→ nas/status 200、
#       target 失联只 log 跳过：source=linux（不掺 dsm）、pools/shares 空、
#       available 与 mounts 自洽、整体不崩不超时
#   J2  非法 env 不崩：COCKPIT_NAS_TARGETS=非 JSON → 整体忽略，
#       nas/status 200 且 source=linux
#
# 使用：
#   ./scripts/acceptance/nas/probe-jump.sh
#   NAS_KEEP=1 ./scripts/acceptance/nas/probe-jump.sh   # 保留容器现场
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/nas/probe-jump.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SERVER_HOST="0.0.0.0"  # 容器 agent 经 host.docker.internal 拨入（nginx 探针同先例）
SERVER_PORT="${NAS_PORT:-19999}"
SERVER_URL="http://127.0.0.1:${SERVER_PORT}"
CT="nas-acc-jump"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/nas"
LOG_FILE="$EVIDENCE_DIR/probe-jump.log"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-nas-jump-$$"

log() { printf '\033[1;36m[nas]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[nas:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""

cleanup() {
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  if [[ "${NAS_KEEP:-0}" == "1" ]]; then
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
log "J0: building server + agent (agent static——容器 glibc 与宿主不同)..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/cockpit-agent" ./cmd/cockpit-agent)

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: nas-jump-jwt-secret
  expiration: 1h
EOF

log "J0: starting server on ${SERVER_URL}..."
ADMIN_USER="admin" ADMIN_PASSWORD="nas-jump-strong-pass-1" \
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
  -d '{"username":"admin","password":"nas-jump-strong-pass-1"}' \
  | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$TOKEN" ]] || { err "登录失败"; exit 1; }

# ---- J0: 裸容器（debian:12-slim——零存储工具零多余依赖）----
log "J0: starting bare container ${CT} (debian:12-slim)..."
docker rm -f "$CT" >/dev/null 2>&1 || true
docker run -d --name "$CT" --add-host host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
docker cp "$WORK_DIR/bin/cockpit-agent" "$CT:/usr/local/bin/cockpit-agent"
docker exec "$CT" chmod +x /usr/local/bin/cockpit-agent

start_agent() { # start_agent ID "ENV"（ENV 为空格分隔的 K=V）
  docker exec -d "$CT" env $2 COCKPIT_DRIFT_BASELINE=/tmp/drift-baseline.json \
    /usr/local/bin/cockpit-agent start \
    -server "ws://host.docker.internal:${SERVER_PORT}/ws" -id "$1" \
    >>"$WORK_DIR/agent-$1.log" 2>&1
}
kill_agent() {
  docker exec "$CT" pkill -f "cockpit-agent start" >/dev/null 2>&1 || true
  sleep 1
}
wait_agent() { # wait_agent ID
  for i in $(seq 1 60); do
    ONLINE=$(curl -fsSL "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" 2>/dev/null | \
      python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
print(any(a.get('id') == '$1' and a.get('status') == 'online' for a in agents))
" || echo "False")
    [[ "$ONLINE" == "True" ]] && return 0
    sleep 1
  done
  err "agent $1 未在 60s 内注册在线"; return 1
}
agent_capabilities() { # agent_capabilities ID → 空格分隔 type 列表
  curl -fsSL "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
for a in agents:
    if a.get('id') == '$1':
        print(' '.join(sorted(c.get('type','') for c in a.get('capabilities') or [])))
        break
"
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

# ---- J1 跳板观测路径：合法 targets（死端口 dsm 条目）→ 失联降级 ----
VALID_TARGETS='[{"name":"jump-dsm","type":"dsm","addr":"http://127.0.0.1:59999","username":"acc","password":"acc-pass","insecureTls":true}]'
log "J1: agent nas-acc-jump with valid targets (dead-port dsm)..."
start_agent "nas-acc-jump" "COCKPIT_NAS_TARGETS=$VALID_TARGETS"
wait_agent "nas-acc-jump"
log "J1: caps=[$(agent_capabilities nas-acc-jump)]（mdstat 恒在 → nas 恒上报，见头注）"
api GET "/api/agents/nas-acc-jump/nas/status"
J1=$(API_CODE="$API_CODE" API_BODY="$API_BODY" python3 -c "
import json, os, sys
if os.environ['API_CODE'] != '200':
    sys.exit(1)
d = json.loads(os.environ['API_BODY'])
mounts = d.get('mounts')
ok = (d.get('source') == 'linux'           # 失联 target 不掺进参与源
      and d.get('pools') == [] and d.get('shares') == []
      and isinstance(mounts, list)          # df 段独立工作（docker bind mount 形态）
      and d.get('available') == (len(mounts) > 0))
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "J1 跳板观测：失联 target 只跳过——source=linux 不掺 dsm + pools/shares 空 + available 与 mounts 自洽" \
  "$J1" "code=${API_CODE} body=$(echo "$API_BODY" | head -c 260)"

# ---- J2 非法 env 不崩：非 JSON → 整体忽略，观测照常 ----
log "J2: agent nas-acc-badenv with invalid targets JSON..."
kill_agent
start_agent "nas-acc-badenv" "COCKPIT_NAS_TARGETS=not-json-at-all"
wait_agent "nas-acc-badenv"
api GET "/api/agents/nas-acc-badenv/nas/status"
J2=$(API_CODE="$API_CODE" API_BODY="$API_BODY" python3 -c "
import json, os, sys
if os.environ['API_CODE'] != '200':
    sys.exit(1)
d = json.loads(os.environ['API_BODY'])
sys.exit(0 if d.get('source') == 'linux' and d.get('pools') == [] else 1)" && echo OK || echo BAD)
check "J2 非法 env：整体忽略不崩——nas/status 200 + source=linux" \
  "$J2" "code=${API_CODE} body=$(echo "$API_BODY" | head -c 200)"

log "nas jump acceptance: PASS=${PASS} FAIL=${FAIL}"
[[ "$FAIL" -eq 0 ]] || exit 1
log "All NAS-JUMP checks passed ✓ (J1-J2)"
