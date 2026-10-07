#!/usr/bin/env bash
# Cockpit 备份前置命令钩子真机验收（acceptance-checklist「备份与恢复（agent
# 侧）」节 50：mysqldump / pg_dump 真库导出产物在备份内 + 失败短路不打包）。
# 双容器形态：debian:12 装 mariadb-server（真库 + 真客户端工具）、
# postgres:16-alpine 官方镜像（自带 pg_dump）——hook 命令全真执行，
# 归档内容逐字节断言。超时分支由单测盖（backupHookTimeout 注入 +
# killHookGroup 整组杀），5min 真等不在探针复刻。
#
#   B0  基线：双 agent 在线 + 容器内 mysqldump / pg_dump 真实可用
#   B1  mysqldump 热备：pre_hook 真库导出 → run success → 归档含
#       site/db-acc.sql（INSERT + 哨兵行）与夹具
#   B2  失败短路：pre_hook 非零退出 → run failed，Error 含退出码与
#       stderr 摘要（D28），File 空 + 产物列表 0 件（不打包）
#   B3  pg_dump 热备：同 B1 形态（COPY + 哨兵行）
#
# 使用：
#   ./scripts/acceptance/agent-backup/probe-hook.sh
#   BKH_KEEP=1 ./scripts/acceptance/agent-backup/probe-hook.sh   # 保留容器现场
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/agent-backup/probe-hook.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SERVER_HOST="0.0.0.0"  # 容器 agent 经 host.docker.internal 拨入（nginx 探针同先例）
SERVER_PORT="${BKH_PORT:-20000}"
SERVER_URL="http://127.0.0.1:${SERVER_PORT}"
CT_MY="bk-acc-my"
CT_PG="bk-acc-pg"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/agent-backup"
LOG_FILE="$EVIDENCE_DIR/probe-hook.log"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-bkh-$$"

log() { printf '\033[1;36m[bkh]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[bkh:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""

cleanup() {
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  if [[ "${BKH_KEEP:-0}" == "1" ]]; then
    log "保留现场：容器 ${CT_MY}/${CT_PG} 与 ${WORK_DIR}"
  else
    docker rm -f "$CT_MY" >/dev/null 2>&1 || true
    docker rm -f "$CT_PG" >/dev/null 2>&1 || true
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT INT TERM

command -v docker >/dev/null || { err "需要 docker"; exit 1; }
mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$EVIDENCE_DIR"
: > "$LOG_FILE"

# ---- B0: build + server ----
log "B0: building server + agent (agent static——容器 glibc 与宿主不同)..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/cockpit-agent" ./cmd/cockpit-agent)

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: bkh-jwt-secret
  expiration: 1h
EOF

log "B0: starting server on ${SERVER_URL}..."
ADMIN_USER="admin" ADMIN_PASSWORD="bkh-strong-pass-1" \
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
  -d '{"username":"admin","password":"bkh-strong-pass-1"}' \
  | python3 -c 'import sys, json; print(json.load(sys.stdin).get("token", ""))')
[[ -n "$TOKEN" ]] || { err "登录失败"; exit 1; }

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
run_cfg() { # run_cfg CFG_ID
  api POST "/api/backups/configs/$1/run"
}
parse_run() { # parse_run（读 API_BODY 最新一条）-> RUN_STATUS/RUN_ERR/RUN_FILE
  eval "$(echo "$API_BODY" | RUNS_JSON="$API_BODY" python3 -c "
import json, os, sys
runs = json.loads(os.environ['RUNS_JSON']).get('runs') or []
r = runs[0] if runs else {}
print('RUN_STATUS=%s' % repr(r.get('status', '')))
print('RUN_ERR=%s' % repr(r.get('error', '')))
print('RUN_FILE=%s' % repr(r.get('file', '')))
")"
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
  RUN_STATUS=""; RUN_ERR="poll-timeout"; RUN_FILE=""
  return 1
}
download_file() { # download_file CFG_ID NAME -> DL 路径（非 200 置空）
  local name_enc
  name_enc=$(python3 -c 'import urllib.parse, sys; print(urllib.parse.quote(sys.argv[1], safe=""))' "$2")
  DL="$WORK_DIR/dl-$1.tar.gz"
  local code
  code=$(curl -sS -o "$DL" -w '%{http_code}' \
    "${SERVER_URL}/api/backups/configs/$1/files/download?name=${name_enc}" \
    -H "Authorization: Bearer $TOKEN")
  [[ "$code" == "200" ]] || DL=""
}

# ---- B0: 容器编排——真库双容器 ----
log "B0: starting mariadb container ${CT_MY} (debian:12)..."
docker rm -f "$CT_MY" >/dev/null 2>&1 || true
docker run -d --name "$CT_MY" --add-host host.docker.internal:host-gateway \
  debian:12 sleep infinity >/dev/null

log "B0: installing mariadb-server + mariadb-client（真库 + 真导出工具）..."
docker exec "$CT_MY" sh -c \
  "apt-get update -qq && apt-get install -y -qq --no-install-recommends mariadb-server mariadb-client >/dev/null"

log "B0: starting mariadb + seeding accdb..."
docker exec "$CT_MY" service mariadb start >/dev/null
for i in $(seq 1 60); do
  if docker exec "$CT_MY" mariadb-admin ping >/dev/null 2>&1; then break; fi
  sleep 1
  if [[ $i -eq 60 ]]; then err "mariadb 未在 60s 内就绪"; exit 1; fi
done
docker exec "$CT_MY" mariadb -e \
  "CREATE DATABASE accdb; USE accdb; CREATE TABLE t1(id INT PRIMARY KEY, note VARCHAR(64)); \
   INSERT INTO t1 VALUES (1,'hook-my-sentinel-42');"
docker exec "$CT_MY" sh -c 'mkdir -p /data/site /data/dest /data/dest-fail && echo "cockpit-hook fixture" > /data/site/readme.txt'

log "B0: starting postgres container ${CT_PG} (postgres:16-alpine)..."
docker rm -f "$CT_PG" >/dev/null 2>&1 || true
docker run -d --name "$CT_PG" -e POSTGRES_PASSWORD=accpass \
  --add-host host.docker.internal:host-gateway postgres:16-alpine >/dev/null
for i in $(seq 1 90); do
  if docker exec "$CT_PG" pg_isready -U postgres >/dev/null 2>&1; then break; fi
  sleep 1
  if [[ $i -eq 90 ]]; then err "postgres 未在 90s 内就绪"; docker logs "$CT_PG" >&2 || true; exit 1; fi
done
docker exec "$CT_PG" psql -U postgres -q \
  -c "CREATE TABLE pgacc_t1(id INT PRIMARY KEY, note VARCHAR(64));" \
  -c "INSERT INTO pgacc_t1 VALUES (1,'hook-pg-sentinel-42');"
docker exec "$CT_PG" sh -c 'mkdir -p /data/site /data/dest && echo "cockpit-hook fixture" > /data/site/readme.txt'

# agent 入双容器
for CT in "$CT_MY" "$CT_PG"; do
  docker cp "$WORK_DIR/bin/cockpit-agent" "$CT:/usr/local/bin/cockpit-agent"
  docker exec "$CT" chmod +x /usr/local/bin/cockpit-agent
  docker exec -d "$CT" env COCKPIT_DRIFT_BASELINE=/tmp/drift-baseline.json \
    /usr/local/bin/cockpit-agent start \
    -server "ws://host.docker.internal:${SERVER_PORT}/ws" -id "$CT" \
    >>"$WORK_DIR/agent-$CT.log" 2>&1
done
wait_agent "$CT_MY"
wait_agent "$CT_PG"
MY_DUMP_VER=$(docker exec "$CT_MY" sh -c 'mysqldump --version 2>/dev/null || echo MISSING')
PG_DUMP_VER=$(docker exec "$CT_PG" pg_dump --version 2>/dev/null || echo MISSING)
B0=$(V1="$MY_DUMP_VER" V2="$PG_DUMP_VER" python3 -c "
import os, sys
ok = (os.environ['V1'] != 'MISSING' and 'MISSING' not in os.environ['V2'])
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "B0 基线：双 agent 在线 + mysqldump/pg_dump 真实可用" \
  "$B0" "my=${MY_DUMP_VER} pg=${PG_DUMP_VER}"

# ---- B1 mysqldump 热备：真库导出产物在备份内 ----
log "B1: config hookmy with pre_hook mysqldump..."
NEW_BODY=$(curl -sSL -X POST "${SERVER_URL}/api/backups/configs" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"${CT_MY}\",\"name\":\"hookmy\",\"sources\":[\"/data/site\"],\"dest_dir\":\"/data/dest\",\"schedule\":\"manual\",\"retention\":0,\"pre_hook\":\"mysqldump accdb > /data/site/db-acc.sql\"}")
CFG_MY=$(echo "$NEW_BODY" | python3 -c 'import sys, json; print(json.load(sys.stdin).get("id", ""))')
[[ -n "$CFG_MY" ]] || { err "B1 配置创建失败: $NEW_BODY"; exit 1; }
run_cfg "$CFG_MY"
B1_RUN=""
if wait_run "$CFG_MY" 120; then B1_RUN="done"; fi
FILES_MY=$(curl -fsSL "${SERVER_URL}/api/backups/configs/${CFG_MY}/files" -H "Authorization: Bearer $TOKEN" 2>/dev/null | \
  python3 -c 'import sys, json; files = json.load(sys.stdin).get("files") or []; print(len(files), files[0]["name"] if files else "")' || echo "0 ")
FNAME_MY=$(echo "$FILES_MY" | cut -d' ' -f2)
DL_MY=""
ARCH_OK=""
if [[ "$RUN_STATUS" == "success" && -n "$FNAME_MY" ]]; then
  download_file "$CFG_MY" "$FNAME_MY"
  if [[ -n "$DL" ]]; then
    ARCH_OK=$(ARCH="$DL" python3 -c "
import tarfile, os, sys
try:
    t = tarfile.open(os.environ['ARCH'], 'r:gz')
    names = t.getnames()
    if 'site/db-acc.sql' not in names or 'site/readme.txt' not in names:
        print('BAD'); sys.exit()
    sql = t.extractfile('site/db-acc.sql').read().decode('utf-8', 'replace')
    ok = 'INSERT INTO' in sql and 'hook-my-sentinel-42' in sql
    print('OK' if ok else 'BAD')
except Exception:
    print('BAD')
" )
  fi
fi
B1=$(S="$RUN_STATUS" A="$ARCH_OK" python3 -c "
import os, sys
sys.exit(0 if os.environ['S'] == 'success' and os.environ['A'] == 'OK' else 1)" && echo OK || echo BAD)
check "B1 mysqldump 热备：真库导出产物在备份内（run success + 归档含 INSERT/哨兵行）" \
  "$B1" "run=${RUN_STATUS} err=${RUN_ERR:0:80} file=${FNAME_MY} archive=${ARCH_OK}"

# ---- B2 失败短路：非零退出 → failed，不打包 ----
log "B2: config hookfail with failing pre_hook..."
NEW_BODY=$(curl -sSL -X POST "${SERVER_URL}/api/backups/configs" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"${CT_MY}\",\"name\":\"hookfail\",\"sources\":[\"/data/site\"],\"dest_dir\":\"/data/dest-fail\",\"schedule\":\"manual\",\"retention\":0,\"pre_hook\":\"echo hook-boom-stderr >&2; exit 3\"}")
CFG_FAIL=$(echo "$NEW_BODY" | python3 -c 'import sys, json; print(json.load(sys.stdin).get("id", ""))')
[[ -n "$CFG_FAIL" ]] || { err "B2 配置创建失败: $NEW_BODY"; exit 1; }
run_cfg "$CFG_FAIL"
B2_RUN=""
if wait_run "$CFG_FAIL" 120; then B2_RUN="done"; fi
FILES_FAIL=$(curl -fsSL "${SERVER_URL}/api/backups/configs/${CFG_FAIL}/files" -H "Authorization: Bearer $TOKEN" 2>/dev/null | \
  python3 -c 'import sys, json; print(len(json.load(sys.stdin).get("files") or []))' || echo "-1")
B2=$(S="$RUN_STATUS" E="$RUN_ERR" N="$FILES_FAIL" F="$RUN_FILE" python3 -c "
import os, sys
e = os.environ['E']
ok = (os.environ['S'] == 'failed'
      and 'pre-hook failed' in e
      and 'exit status 3' in e
      and 'hook-boom-stderr' in e   # stderr 摘要透传（D28）
      and os.environ['N'] == '0'    # 不打包：产物列表 0 件
      and os.environ['F'] == '')    # File 空
sys.exit(0 if ok else 1)" && echo OK || echo BAD)
check "B2 失败短路：failed + 退出码/stderr 透传 + 不打包（D28）" \
  "$B2" "run=${RUN_STATUS} err=${RUN_ERR:0:120} files=${FILES_FAIL} file=${RUN_FILE}"

# ---- B3 pg_dump 热备：真库导出产物在备份内 ----
log "B3: config hookpg with pre_hook pg_dump..."
NEW_BODY=$(curl -sSL -X POST "${SERVER_URL}/api/backups/configs" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"${CT_PG}\",\"name\":\"hookpg\",\"sources\":[\"/data/site\"],\"dest_dir\":\"/data/dest\",\"schedule\":\"manual\",\"retention\":0,\"pre_hook\":\"PGPASSWORD=accpass pg_dump -h 127.0.0.1 -U postgres postgres > /data/site/db-pg.sql\"}")
CFG_PG=$(echo "$NEW_BODY" | python3 -c 'import sys, json; print(json.load(sys.stdin).get("id", ""))')
[[ -n "$CFG_PG" ]] || { err "B3 配置创建失败: $NEW_BODY"; exit 1; }
run_cfg "$CFG_PG"
B3_RUN=""
if wait_run "$CFG_PG" 120; then B3_RUN="done"; fi
FILES_PG=$(curl -fsSL "${SERVER_URL}/api/backups/configs/${CFG_PG}/files" -H "Authorization: Bearer $TOKEN" 2>/dev/null | \
  python3 -c 'import sys, json; files = json.load(sys.stdin).get("files") or []; print(files[0]["name"] if files else "")' || echo "")
DL_PG=""
ARCH_OK_PG=""
if [[ "$RUN_STATUS" == "success" && -n "$FILES_PG" ]]; then
  download_file "$CFG_PG" "$FILES_PG"
  DL_PG="$DL"
  ARCH_OK_PG=$(ARCH="$DL_PG" python3 -c "
import tarfile, os, sys
try:
    t = tarfile.open(os.environ['ARCH'], 'r:gz')
    names = t.getnames()
    if 'site/db-pg.sql' not in names:
        print('BAD'); sys.exit()
    sql = t.extractfile('site/db-pg.sql').read().decode('utf-8', 'replace')
    ok = 'COPY' in sql and 'hook-pg-sentinel-42' in sql
    print('OK' if ok else 'BAD')
except Exception:
    print('BAD')
" )
fi
B3=$(S="$RUN_STATUS" A="$ARCH_OK_PG" python3 -c "
import os, sys
sys.exit(0 if os.environ['S'] == 'success' and os.environ['A'] == 'OK' else 1)" && echo OK || echo BAD)
check "B3 pg_dump 热备：真库导出产物在备份内（run success + 归档含 COPY/哨兵行）" \
  "$B3" "run=${RUN_STATUS} err=${RUN_ERR:0:80} file=${FILES_PG} archive=${ARCH_OK_PG}"

log "backup hook acceptance: PASS=${PASS} FAIL=${FAIL}"
[[ "$FAIL" -eq 0 ]] || exit 1
log "All BKH checks passed ✓ (B0-B3)"
