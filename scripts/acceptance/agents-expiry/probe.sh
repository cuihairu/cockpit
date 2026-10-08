#!/usr/bin/env bash
# 过期 agent 自动判定验收探针（D-2026-10-08-3，用户「清理过期 agent」闭环）。
#
# 场景（全本机，端口 19989，CDP 9226；避开既有的 19990-19998 分配）：
#   E1  agent 注册在线（/api/agents status=online，lastSeen 新鲜）
#   E2  幽灵在线制造：SIGKILL agent + SIGKILL server（双双无优雅退出路径，
#       readLoop defer 没机会写 offline）→ 重启 server → /api/agents 仍是
#       status=online（假在线幽灵，last_seen 冻结在 kill 时刻）
#   E3  过期自动判定：等待 last_seen 龄 > 阈值(60s) + cleanupLoop tick(30s)
#       → 状态自动翻转 offline（服务端 sweep 干的，agent 已死不可能自己改）
#       + server 日志出现 "Expired 1 agent(s)"（清理动作有日志）
#   E4  阈值配置可见：/api/status 下发 agentExpireMinutes=1
#   E5  web 列表：过期行默认隐藏 + Alert「已隐藏 1 台」+ 角标「过期阈值」
#       （CDP 断言 + 截图，webprobe.cjs）
#   E6  web 显示切换 → 行回来标「离线」；手动「清理离线 agent」钮 → 物理
#       删除（/api/agents 变空）
#
# 使用：
#   ./scripts/acceptance/agents-expiry/probe.sh
#   AGEX_KEEP=1 ./scripts/acceptance/agents-expiry/probe.sh   # 保留现场排查
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/agents-expiry/probe.log + exp-*.png

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-agex-$$"
SERVER_HOST="127.0.0.1"
SERVER_PORT="${AGEX_PORT:-19989}"
SERVER_URL="http://${SERVER_HOST}:${SERVER_PORT}"
CDP_PORT="${AGEX_CDP_PORT:-9226}"
ADMIN_USER="admin"
ADMIN_PASS="agex-acc-strong-pass-1"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/agents-expiry"
LOG_FILE="$EVIDENCE_DIR/probe.log"
WEBPROBE="$ROOT_DIR/scripts/acceptance/agents-expiry/webprobe.cjs"
THRESHOLD_MIN=1   # 加速验收：生产默认 5

mkdir -p "$EVIDENCE_DIR" "$WORK_DIR"/{data,bin,logs}

log() { echo "[$(date +%H:%M:%S)] $*" | tee -a "$LOG_FILE"; }
cleanup() {
    docker rm -f agex-web-chrome >/dev/null 2>&1 || true
    if [[ "${AGEX_KEEP:-0}" != "1" ]]; then
        [[ -f "$WORK_DIR/server.pid" ]] && kill "$(cat "$WORK_DIR/server.pid")" >/dev/null 2>&1 || true
        rm -rf "$WORK_DIR"
    else
        log "AGEX_KEEP=1：现场保留在 $WORK_DIR"
    fi
}
trap cleanup EXIT

exec > >(tee -a "$LOG_FILE") 2>&1

command -v docker >/dev/null || { echo "ERROR: 需要 docker（headless Chrome）"; exit 1; }
if ss -ltn "( sport = :${SERVER_PORT} )" 2>/dev/null | grep -q "${SERVER_PORT}"; then
    echo "ERROR: 端口 ${SERVER_PORT} 已被占用"; exit 1
fi
docker image inspect chromedp/headless-shell:stable >/dev/null 2>&1 || {
    echo "ERROR: 缺少 chromedp/headless-shell:stable 镜像"; exit 1; }

log "E0: building server + agent..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/cockpit-agent" ./cmd/cockpit-agent)
if [[ "${AGEX_SKIP_WEB_BUILD:-0}" == "1" && -f "$ROOT_DIR/web/dist/index.html" ]]; then
    log "E0: AGEX_SKIP_WEB_BUILD=1，沿用既有 web/dist"
else
    log "E0: building web/dist（强制重建，验收的是当前源码而非陈旧产物，1-2 分钟）..."
    (cd "$ROOT_DIR/web" && CI=true pnpm run build >/dev/null)
fi

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: agex-accept-jwt-secret
  expiration: 4h
EOF

start_server() {
    ADMIN_USERNAME="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASS}" \
    AGENT_EXPIRE_MINUTES="${THRESHOLD_MIN}" \
    STATIC_DIR="$ROOT_DIR/web/dist" \
        "$WORK_DIR/bin/cockpit" server -config "$WORK_DIR/cockpit.yaml" \
        >> "$WORK_DIR/logs/server.log" 2>&1 &
    echo $! > "$WORK_DIR/server.pid"
    for _ in $(seq 1 30); do
        curl -sf "$SERVER_URL/health" >/dev/null 2>&1 && return 0
        kill -0 "$(cat "$WORK_DIR/server.pid")" 2>/dev/null || {
            log "FAIL: server 启动失败"; tail -n 30 "$WORK_DIR/logs/server.log"; exit 1; }
        sleep 1
    done
    log "FAIL: health 超时"; exit 1
}

login_token() {
    curl -sf -X POST "$SERVER_URL/api/auth/login" -H 'Content-Type: application/json' \
        -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
        | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])'
}

agents_json() {
    curl -sf "$SERVER_URL/api/agents" -H "Authorization: Bearer $(cat "$WORK_DIR/token")"
}

AGENT_ID="agex-acc-a1"

# ---- E1: agent 注册在线 ----
log "E1: starting server + agent..."
: > "$WORK_DIR/logs/server.log"
start_server
"$WORK_DIR/bin/cockpit-agent" start -server "ws://${SERVER_HOST}:${SERVER_PORT}/ws" -id "$AGENT_ID" \
    > "$WORK_DIR/logs/agent.log" 2>&1 &
echo $! > "$WORK_DIR/agent.pid"

TOKEN=""
for _ in $(seq 1 60); do
    TOKEN=$(login_token 2>/dev/null) || TOKEN=""
    [[ -n "$TOKEN" ]] && echo "$TOKEN" > "$WORK_DIR/token" && break
    sleep 1
done
[[ -n "$TOKEN" ]] || { log "FAIL: 登录失败"; exit 1; }

ONLINE=""
for _ in $(seq 1 30); do
    ONLINE=$(agents_json | python3 -c "
import json,sys
d=json.load(sys.stdin); a=d if isinstance(d,list) else d.get('agents',[])
m=[x for x in a if x.get('id')=='$AGENT_ID']
print(m[0]['status'] if m else 'missing')" 2>/dev/null) || ONLINE=""
    [[ "$ONLINE" == "online" ]] && break
    sleep 1
done
if [[ "$ONLINE" == "online" ]]; then
    log "PASS E1: agent $AGENT_ID 注册在线"
else
    log "FAIL E1: agent 状态=$ONLINE"; tail -n 20 "$WORK_DIR/logs/agent.log"; exit 1
fi

# ---- E2: 制造幽灵在线（双 SIGKILL，绕过一切优雅路径）----
# 先杀 server（死进程写不了库），再杀 agent——顺序反了会给 server readLoop
# 几毫秒把 offline 写进库，幽灵行就造不出来（首轮实测踩过）
log "E2: SIGKILL server + agent（制造假在线幽灵行）..."
KILL_AT=$(date +%s)
kill -9 "$(cat "$WORK_DIR/server.pid")" 2>/dev/null || true
kill -9 "$(cat "$WORK_DIR/agent.pid")" 2>/dev/null || true
wait "$(cat "$WORK_DIR/server.pid")" 2>/dev/null || true
sleep 2
: > "$WORK_DIR/logs/server.log"   # 只保留重启后的日志（E3 要 grep sweep 日志）
start_server

STATUS=""
for _ in $(seq 1 10); do
    STATUS=$(agents_json | python3 -c "
import json,sys
d=json.load(sys.stdin); a=d if isinstance(d,list) else d.get('agents',[])
m=[x for x in a if x.get('id')=='$AGENT_ID']
print(m[0]['status'] if m else 'missing')" 2>/dev/null) || STATUS=""
    [[ "$STATUS" == "online" ]] && break
    sleep 1
done
if [[ "$STATUS" == "online" ]]; then
    log "PASS E2: 重启后幽灵行 status=online（假在线，last_seen 冻结于 kill 时刻）"
else
    log "FAIL E2: 期望幽灵 online，实际 $STATUS"; exit 1
fi

# ---- E3: 过期自动判定（sweep 标离线 + 日志）----
log "E3: 等待过期（阈值 ${THRESHOLD_MIN}min + tick 30s，约 100s）..."
FLIP_AT=""
for _ in $(seq 1 40); do
    STATUS=$(agents_json | python3 -c "
import json,sys
d=json.load(sys.stdin); a=d if isinstance(d,list) else d.get('agents',[])
m=[x for x in a if x.get('id')=='$AGENT_ID']
print(m[0]['status'] if m else 'missing')" 2>/dev/null) || STATUS=""
    if [[ "$STATUS" == "offline" ]]; then FLIP_AT=$(date +%s); break; fi
    sleep 5
done
AGE=$(( ${FLIP_AT:-0} - KILL_AT ))
if [[ -n "$FLIP_AT" ]] && (( AGE >= THRESHOLD_MIN * 60 )); then
    log "PASS E3: ${AGE}s 后自动翻转 offline（> 阈值 ${THRESHOLD_MIN}min，判定来自 sweep）"
else
    log "FAIL E3: 翻转状态 STATUS=$STATUS 翻转龄=${AGE}s"; exit 1
fi
if grep -q "Expired 1 agent(s)" "$WORK_DIR/logs/server.log"; then
    log "PASS E3b: sweep 日志在场（清理动作有日志）"
    grep "Expired 1 agent(s)" "$WORK_DIR/logs/server.log" | head -1
else
    log "FAIL E3b: 未找到 sweep 日志"; exit 1
fi

# ---- E4: 阈值配置可见 ----
EXPOSED=$(curl -sf "$SERVER_URL/api/status" -H "Authorization: Bearer $TOKEN" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("agentExpireMinutes",""))')
if [[ "$EXPOSED" == "$THRESHOLD_MIN" ]]; then
    log "PASS E4: /api/status agentExpireMinutes=$EXPOSED"
else
    log "FAIL E4: agentExpireMinutes=$EXPOSED, want $THRESHOLD_MIN"; exit 1
fi

# ---- E5/E6: web 列表（隐藏/标注/手动清理）----
log "E5/E6: starting headless Chrome (CDP :${CDP_PORT})..."
# 挂宿主字体：headless-shell 无 CJK 字体，截图中文全变豆腐块（DOM 断言不受
# 影响，但验收截图要给人看）
FONT_ARGS=""
[[ -d /usr/share/fonts ]] && FONT_ARGS="-v /usr/share/fonts:/usr/share/fonts:ro"
docker run -d --name agex-web-chrome --network host $FONT_ARGS \
    chromedp/headless-shell:stable --remote-debugging-port=${CDP_PORT} >/dev/null
for _ in $(seq 1 20); do
    curl -sf "http://127.0.0.1:${CDP_PORT}/json/version" >/dev/null 2>&1 && break
    sleep 1
done

if node "$WEBPROBE" --web="$SERVER_URL" --cdp="http://127.0.0.1:${CDP_PORT}" \
    --user="$ADMIN_USER" --pass="$ADMIN_PASS" --ev="$EVIDENCE_DIR"; then
    log "PASS E5/E6: web 三态（隐藏/显示标注/手动清理）全过"
else
    log "FAIL E5/E6: webprobe 失败"; exit 1
fi

# E6 终判：手动清理后 DB 行物理消失
LEFT=$(agents_json | python3 -c "
import json,sys
d=json.load(sys.stdin); a=d if isinstance(d,list) else d.get('agents',[])
print(len(a))")
if [[ "$LEFT" == "0" ]]; then
    log "PASS E6b: 手动清理后 /api/agents 清空（物理删除生效）"
else
    log "FAIL E6b: 清理后仍剩 $LEFT 行"; exit 1
fi

log "=== 全部场景 PASS（E1-E6）==="
