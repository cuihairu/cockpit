#!/usr/bin/env bash
# Cockpit 备份异地保留真机验收——agent 侧机制半（acceptance-checklist「备份
# 与恢复（agent 侧）」节 47：真 rclone 推送成功；推送失败不改任务终态且
# backup.remote-failed 独立通知；「补传」成功）。
# 容器形态：debian:12 装 rclone，建 local 后端远端（bklocal:/backups）——
# 真 rclone exec 链路全真（argv/正则/RemoteStatus 跟踪），零云凭据。
# 真实 S3/B2 云端点维持挂起（需云凭据，见清单注记）。
#
#   R0  基线：agent 在线 + 容器内 rclone 可用 + remote_dest 配置期受理
#       （requireAgentRclone 探测 capability metadata 通过）
#   R1  推送成功：remote_dest=bklocal:/backups → run success +
#       remoteStatus=ok + 远端目录实收归档
#   R2  推送失败不改终态：ghost remote（config 无 section）→ run 仍
#       success（D21）+ remoteStatus=failed + remoteError 摘要 +
#       backup.remote-failed webhook 实收 + 本地档完整可下载
#   R3  补传：删远端产物后 sync-remote → 远端文件恢复
#
# 使用：
#   ./scripts/acceptance/agent-backup/probe-remote.sh
#   BKR_KEEP=1 ./scripts/acceptance/agent-backup/probe-remote.sh   # 保留现场
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/agent-backup/probe-remote.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SERVER_HOST="0.0.0.0"  # 容器 agent 经 host.docker.internal 拨入（nginx 探针同先例）
SERVER_PORT="${BKR_PORT:-20001}"
SERVER_URL="http://127.0.0.1:${SERVER_PORT}"
CT="bk-acc-remote"
RECEIVER_PORT="${BKR_HOOK_PORT:-19702}"
WH_SECRET="bkr-webhook-secret"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/agent-backup"
LOG_FILE="$EVIDENCE_DIR/probe-remote.log"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-bkr-$$"

log() { printf '\033[1;36m[bkr]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[bkr:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""
RECEIVER_PID=""

cleanup() {
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  [[ -n "$RECEIVER_PID" ]] && kill "$RECEIVER_PID" 2>/dev/null || true
  if [[ "${BKR_KEEP:-0}" == "1" ]]; then
    log "保留现场：容器 ${CT} 与 ${WORK_DIR}"
  else
    docker rm -f "$CT" >/dev/null 2>&1 || true
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT INT TERM

command -v docker >/dev/null || { err "需要 docker"; exit 1; }
mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$WORK_DIR/logs" "$EVIDENCE_DIR"
: > "$LOG_FILE"

# ---- R0: build + server + webhook 接收器 ----
log "R0: building server + agent (agent static——容器 glibc 与宿主不同)..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/cockpit-agent" ./cmd/cockpit-agent)

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: bkr-jwt-secret
  expiration: 1h
notification:
  enabled: true
  webhook:
    - url: http://127.0.0.1:${RECEIVER_PORT}/hook
      secret: ${WH_SECRET}
  events:
    backup.remote-failed:
      type: backup.remote-failed   # 白名单按 EventConfig.Type 字段匹配
      enabled: true
EOF

log "R0: starting webhook receiver (:${RECEIVER_PORT})..."
WH_LOG="$WORK_DIR/webhooks.jsonl"
python3 "$ROOT_DIR/scripts/acceptance/webhook_receiver.py" \
  "$WH_SECRET" "$WH_LOG" "$RECEIVER_PORT" >>"$WORK_DIR/logs/receiver.log" 2>&1 &
RECEIVER_PID=$!
sleep 1
kill -0 "$RECEIVER_PID" 2>/dev/null || { err "接收器启动失败"; cat "$WORK_DIR/logs/receiver.log" >&2; exit 1; }

log "R0: starting server on ${SERVER_URL}..."
ADMIN_USER="admin" ADMIN_PASSWORD="bkr-strong-pass-1" \
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
  -d '{"username":"admin","password":"bkr-strong-pass-1"}' \
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
parse_run() { # parse_run（读 API_BODY 最新一条）-> RUN_STATUS/RUN_FILE/RUN_RSTATUS/RUN_RERR
  # rclone 错误文案含单引号（didn't find section…），repr+eval 会撞碎——
  # 改 shlex.quote 落文件再 source（探针批 15 实证坑）
  RUNS_JSON="$API_BODY" OUT="$WORK_DIR/runvars.env" python3 -c "
import json, os, shlex
runs = json.loads(os.environ['RUNS_JSON']).get('runs') or []
r = runs[0] if runs else {}
lines = ['RUN_STATUS=' + shlex.quote(r.get('status') or ''),
         'RUN_FILE=' + shlex.quote(r.get('file') or ''),
         'RUN_RSTATUS=' + shlex.quote(r.get('remoteStatus') or ''),
         'RUN_RERR=' + shlex.quote(r.get('remoteError') or '')]
open(os.environ['OUT'], 'w').write('\n'.join(lines) + '\n')
"
  . "$WORK_DIR/runvars.env"
}
wait_run() { # wait_run CFG_ID DEADLINE_SEC（终态后 RUN_* 就绪；返回 0=有终态）
  local deadline=$(( $(date +%s) + $2 ))
  while [[ $(date +%s) -lt $deadline ]]; do
    api GET "/api/backups/configs/$1/runs"
    if [[ "$API_CODE" == "200" ]]; then
      parse_run
      if [[ -n "$RUN_STATUS" && "$RUN_STATUS" != "running" ]]; then return 0; fi
    fi
    sleep 2
  done
  RUN_STATUS=""; RUN_FILE=""; RUN_RSTATUS=""; RUN_RERR="poll-timeout"
  return 1
}
new_cfg() { # new_cfg NAME REMOTE_DEST -> CFG_ID（201 否则空）
  local body
  body=$(curl -sSL -X POST "${SERVER_URL}/api/backups/configs" \
    -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    -d "{\"agent_id\":\"${CT}\",\"name\":\"$1\",\"sources\":[\"/data/site\"],\"dest_dir\":\"/data/dest\",\"schedule\":\"manual\",\"retention\":0,\"remote_dest\":\"$2\"}")
  echo "$body" | python3 -c 'import sys, json; print(json.load(sys.stdin).get("id", ""))' 2>/dev/null || echo ""
}

# ---- R0: 容器编排——rclone + local 后端远端 ----
log "R0: starting container ${CT} (debian:12 + rclone)..."
docker rm -f "$CT" >/dev/null 2>&1 || true
docker run -d --name "$CT" --add-host host.docker.internal:host-gateway \
  debian:12 sleep infinity >/dev/null
docker exec "$CT" sh -c \
  "apt-get update -qq && apt-get install -y -qq --no-install-recommends rclone >/dev/null"
docker exec "$CT" rclone config create bklocal local >/dev/null
docker exec "$CT" sh -c 'mkdir -p /data/site /data/dest /backups && echo "cockpit-remote fixture" > /data/site/readme.txt'

docker cp "$WORK_DIR/bin/cockpit-agent" "$CT:/usr/local/bin/cockpit-agent"
docker exec "$CT" chmod +x /usr/local/bin/cockpit-agent
docker exec -d "$CT" env COCKPIT_DRIFT_BASELINE=/tmp/drift-baseline.json \
  /usr/local/bin/cockpit-agent start \
  -server "ws://host.docker.internal:${SERVER_PORT}/ws" -id "$CT" \
  >>"$WORK_DIR/agent.log" 2>&1
wait_agent "$CT"
RCLONE_VER=$(docker exec "$CT" rclone version 2>/dev/null | head -n1 || echo MISSING)
CFG_R1=""
CFG_R1=$(new_cfg "remoterun" "bklocal:/backups")
R0=$(V1="$RCLONE_VER" V2="$CFG_R1" python3 -c "
import os, sys
ok = (os.environ['V1'] != 'MISSING' and os.environ['V2'] != '')
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "R0 基线：agent 在线 + rclone 可用 + remote_dest 配置期受理（rclone capability 探测过）" \
  "$R0" "rclone=${RCLONE_VER} cfg_id=${CFG_R1}"
[[ -n "$CFG_R1" ]] || { err "R1 配置创建失败（remote_dest 被拒？）"; exit 1; }

# ---- R1 推送成功：remoteStatus=ok + 远端实收 ----
log "R1: running config remoterun (bklocal:/backups)..."
api POST "/api/backups/configs/${CFG_R1}/run"
R1_RUN=""
if wait_run "$CFG_R1" 120; then R1_RUN="done"; fi
REMOTE_HAS=""
R1_FILE_REMOTE=""
if [[ "$RUN_STATUS" == "success" && -n "$RUN_FILE" ]]; then
  R1_FILE_REMOTE=$(docker exec "$CT" sh -c "ls /backups/${RUN_FILE} 2>/dev/null && echo YES || echo NO")
fi
R1=$(S="$RUN_STATUS" RS="$RUN_RSTATUS" RM="$R1_FILE_REMOTE" python3 -c "
import os, sys
ok = (os.environ['S'] == 'success'
      and os.environ['RS'] == 'ok'
      and os.environ['RM'].endswith('YES'))
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "R1 推送成功：run success + remoteStatus=ok + 远端目录实收归档" \
  "$R1" "run=${RUN_STATUS} remoteStatus=${RUN_RSTATUS} remote=${R1_FILE_REMOTE:0:60}"

# ---- R2 推送失败不改终态：ghost remote → failed 通知 + 本地档在 ----
log "R2: running config remotefail (ghost:/backups)..."
CFG_R2=$(new_cfg "remotefail" "ghost:/backups")
[[ -n "$CFG_R2" ]] || { err "R2 配置创建失败"; exit 1; }
api POST "/api/backups/configs/${CFG_R2}/run"
R2_RUN=""
if wait_run "$CFG_R2" 120; then R2_RUN="done"; fi
DL_OK=""
if [[ -n "$RUN_FILE" ]]; then
  NAME_ENC=$(python3 -c 'import urllib.parse, sys; print(urllib.parse.quote(sys.argv[1], safe=""))' "$RUN_FILE")
  DL_CODE=$(curl -sS -o /dev/null -w '%{http_code}' \
    "${SERVER_URL}/api/backups/configs/${CFG_R2}/files/download?name=${NAME_ENC}" \
    -H "Authorization: Bearer $TOKEN")
  [[ "$DL_CODE" == "200" ]] && DL_OK="yes"
fi
GOT_HOOK=""
HOOK_DETAIL=""
deadline=$(( $(date +%s) + 45 ))
while [[ $(date +%s) -lt $deadline ]]; do
  HOOK_DETAIL=$(WH_LOG="$WH_LOG" CFG_R2="$CFG_R2" python3 -c "
import json, os, sys
want_res = '\"resource_id\":\"%s\"' % os.environ['CFG_R2']
try:
    with open(os.environ['WH_LOG']) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            h = json.loads(line)
            b = h.get('body', '')
            if ('\"event_type\":\"backup.remote-failed\"' in b and want_res in b
                    and '\"level\":\"error\"' in b):
                print(b)
                sys.exit(0)
except FileNotFoundError:
    pass
sys.exit(1)
" ) || HOOK_DETAIL=""
  [[ -n "$HOOK_DETAIL" ]] && { GOT_HOOK="yes"; break; }
  sleep 2
done
R2=$(S="$RUN_STATUS" RS="$RUN_RSTATUS" RE="$RUN_RERR" DL="$DL_OK" HK="$GOT_HOOK" python3 -c "
import os, sys
ok = (os.environ['S'] == 'success'            # D21：推送失败不改任务终态
      and os.environ['RS'] == 'failed'
      and len(os.environ['RE']) > 0           # rclone stderr 摘要透传
      and os.environ['DL'] == 'yes'           # 本地档完整可下载
      and os.environ['HK'] == 'yes')          # backup.remote-failed 实收
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "R2 推送失败不改终态：run success + remoteStatus=failed + remoteError 摘要 + webhook 实收 + 本地档在（D21）" \
  "$R2" "run=${RUN_STATUS} remoteStatus=${RUN_RSTATUS} remoteError=${RUN_RERR:0:80} dl=${DL_OK} hook=${HOOK_DETAIL:0:80}"

# ---- R3 补传：删远端产物后 sync-remote 恢复 ----
log "R3: removing remote artifact then sync-remote..."
docker exec "$CT" sh -c "rm -f /backups/*" >/dev/null 2>&1 || true
R1_LATEST=$(api GET "/api/backups/configs/${CFG_R1}/runs" && parse_run && echo "$RUN_FILE")
[[ -n "$R1_LATEST" ]] || { err "R3 取 R1 产物名失败"; exit 1; }
api POST "/api/backups/configs/${CFG_R1}/files/sync-remote" "{\"name\":\"${R1_LATEST}\"}"
SYNC_CODE="$API_CODE"
BACK_HAS=$(docker exec "$CT" sh -c "test -f /backups/${R1_LATEST} && echo YES || echo NO")
R3=$(C="$SYNC_CODE" B="$BACK_HAS" python3 -c "
import os, sys
sys.exit(0 if os.environ['C'] == '200' and os.environ['B'] == 'YES' else 1)" && echo OK || echo BAD)
check "R3 补传：sync-remote 200 + 远端产物恢复" \
  "$R3" "code=${SYNC_CODE} remote=/backups/${R1_LATEST} ${BACK_HAS}"

log "backup remote acceptance: PASS=${PASS} FAIL=${FAIL}"
[[ "$FAIL" -eq 0 ]] || exit 1
log "All BKR checks passed ✓ (R0-R3)"
