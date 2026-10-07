#!/usr/bin/env bash
# Cockpit 反向代理 nginx 后端真机验收探针（docs/guide/proxy-design.md）。
#
# 本机无 nginx（宿主装系统服务不可取），容器造宿主形态——firewall
# probe-hosts.sh 同手法，nginx 与 agent 同容器（nginx provider 的 conf.d
# 与 reload 都发生在 nginx 真正所在的主机）：
#
#   N0  四台 agent 上线（nginx / systemd / dual / bare）
#   N1  新建站点全链：apply http 站点（websocket）→ 片段落盘（meta 注释 +
#       proxy_pass + Upgrade 头）→ sites/site.get 回读 → curl 真实可达；
#       https 站点（自签证书）→ https 可达 + 80→443 301
#   N2  语法/证书错误同步拦截 + 回滚（拦截者是 reload 的前置解析而非
#       pre-write 的 -t）：bogus extra / 证书路径不存在两种，502 透传 +
#       片段不落盘
#   N3  无 systemd 环境（signal reload 路径前提）
#   N4  端口冲突的真实语义：nginx -s reload 不 bind socket，bind 失败
#       异步吞错（apply 200 + 片段留盘 + 旧配置继续服役 + master error.log
#       emerg）——nginx 固有语义、数据面安全；另验同步失败回滚（master
#       已死 → 502 + rolled back + 重启恢复）
#   N5  systemd 容器（--privileged /sbin/init）：reloadMode=systemctl，
#       apply 走 systemctl reload nginx，unit 保持 active
#   N6  双后端分流：nginx + /etc/traefik/dynamic 并存 → capability 双报，
#       apply 落 nginx conf.d（proxyRPCPrefix nginx 优先）
#   N7  旧 agent 形态（无后端 capability）：回退 nginx. 前缀 → agent
#       unknown provider 502（兼容路径真的发得出去）
#
# 使用：
#   ./scripts/acceptance/nginx/probe.sh
#   NG_KEEP=1 ./scripts/acceptance/nginx/probe.sh   # 保留现场排查
#
# 退出码：0 全部场景通过；1 任一失败
# 证据：.acceptance/nginx/probe.log

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/cockpit-ngx-$$"
SERVER_HOST="0.0.0.0"
SERVER_PORT="${NG_PORT:-19996}"
REACH_HOST="127.0.0.1"
SERVER_URL="http://${REACH_HOST}:${SERVER_PORT}"
AGENT_WS="ws://host.docker.internal:${SERVER_PORT}/ws"
ADMIN_USER="admin"
ADMIN_PASS="ngx-acc-strong-pass-1"
EVIDENCE_DIR="$ROOT_DIR/.acceptance/nginx"
LOG_FILE="$EVIDENCE_DIR/probe.log"
NGINX_HTTP_PORT="${NG_HTTP_PORT:-28080}"   # 宿主侧 → 容器 80（18080 被宿主常驻服务占用）
NGINX_TLS_PORT="${NG_TLS_PORT:-28443}"     # 宿主侧 → 容器 443

log() { printf '\033[1;36m[ngx]\033[0m %s\n' "$*" | tee -a "$LOG_FILE"; }
err() { printf '\033[1;31m[ngx:err]\033[0m %s\n' "$*" >&2; }

SERVER_PID=""
CONTAINERS=(ngx-acc-nginx ngx-acc-systemd ngx-acc-dual ngx-acc-bare)

cleanup() {
  if [[ "${NG_KEEP:-0}" == "1" ]]; then
    log "保留现场：容器 ${CONTAINERS[*]} + ${WORK_DIR}"
  else
    for c in "${CONTAINERS[@]}"; do
      docker rm -f "$c" >/dev/null 2>&1 || true
    done
    rm -rf "$WORK_DIR"
  fi
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

command -v openssl >/dev/null || { err "需要 openssl（自签证书）"; exit 1; }
docker image inspect debian:12 >/dev/null 2>&1 || docker pull -q debian:12 >/dev/null
docker image inspect debian:12-slim >/dev/null 2>&1 || docker pull -q debian:12-slim >/dev/null

mkdir -p "$WORK_DIR/data" "$WORK_DIR/bin" "$WORK_DIR/certs" "$EVIDENCE_DIR"
: > "$LOG_FILE"

# ---- N0 前置：构建 + server + 证书 ----
log "N0: building server + static agent..."
(cd "$ROOT_DIR" && GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/" ./cmd/cockpit)
(cd "$ROOT_DIR" && CGO_ENABLED=0 GOBIN="$WORK_DIR/bin" go build -o "$WORK_DIR/bin/cockpit-agent" ./cmd/cockpit-agent)

cat > "$WORK_DIR/cockpit.yaml" <<EOF
server:
  host: ${SERVER_HOST}
  port: ${SERVER_PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: ngx-acc-jwt-secret
  expiration: 1h
EOF

log "N0: starting server on ${SERVER_URL}..."
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

# 自签证书（SAN tls.accept.test；curl --cacert 验证证书引用真被加载）
openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
  -keyout "$WORK_DIR/certs/acc.key" -out "$WORK_DIR/certs/acc.pem" \
  -subj "/CN=tls.accept.test" \
  -addext "subjectAltName=DNS:tls.accept.test" >/dev/null 2>&1

# REST 帮手：api METHOD PATH [JSON] → API_CODE / API_BODY
api() {
  local m="$1" p="$2" data="${3:-}" out
  if [[ -n "$data" ]]; then
    out=$(curl -sS -w $'\n%{http_code}' -X "$m" "${SERVER_URL}/api${p}" \
      -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "$data")
  else
    out=$(curl -sS -w $'\n%{http_code}' -X "$m" "${SERVER_URL}/api${p}" \
      -H "Authorization: Bearer $TOKEN")
  fi
  API_CODE="${out##*$'\n'}"
  API_BODY="${out%$'\n'*}"
}

# 断言帮手
PASS=0; FAIL=0
check() { # check 名称 表达式结果(OK/BAD) 详情
  if [[ "$2" == "OK" ]]; then
    PASS=$((PASS+1)); log "[PASS] $1 — $3"
  else
    FAIL=$((FAIL+1)); log "[FAIL] $1 — $3"
  fi
}
has() { [[ "$1" == *"$2"* ]] && echo OK || echo BAD; }

start_container_agent() {
  local name="$1"
  docker cp "$WORK_DIR/bin/cockpit-agent" "$name:/usr/local/bin/cockpit-agent" >/dev/null
  docker exec -d "$name" sh -c \
    "/usr/local/bin/cockpit-agent start -server ${AGENT_WS} -id ${name} >/tmp/agent.log 2>&1"
}

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

agent_capabilities() { # agent_capabilities ID → 空格分隔 type 列表
  curl -fsS "${SERVER_URL}/api/agents" -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
agents = json.load(sys.stdin) or []
for a in agents:
    if a.get('id') == '$1':
        print(' '.join(sorted(c.get('type','') for c in a.get('capabilities') or [])))
        break
"
}

# ---- N0: 四台容器 ----
log "N0: starting nginx container (signal form)..."
docker run -d --name ngx-acc-nginx --add-host=host.docker.internal:host-gateway \
  -p "127.0.0.1:${NGINX_HTTP_PORT}:80" -p "127.0.0.1:${NGINX_TLS_PORT}:443" \
  debian:12-slim sleep infinity >/dev/null
docker exec ngx-acc-nginx sh -c \
  'apt-get update -qq >/dev/null 2>&1 && \
   apt-get install -y -qq --no-install-recommends nginx python3 >/dev/null 2>&1 && \
   mkdir -p /etc/nginx/certs /srv/a /srv/b && \
   echo NGX-ACC-BACKEND-A > /srv/a/index.html && \
   echo NGX-ACC-BACKEND-B > /srv/b/index.html && \
   nginx' || { err "nginx 容器初始化失败"; docker logs ngx-acc-nginx >&2 || true; exit 1; }
docker cp "$WORK_DIR/certs/acc.pem" ngx-acc-nginx:/etc/nginx/certs/acc.pem >/dev/null
docker cp "$WORK_DIR/certs/acc.key" ngx-acc-nginx:/etc/nginx/certs/acc.key >/dev/null
# 上游后端 ×2 + 端口冲突占位监听（N4 用）
docker exec -d ngx-acc-nginx sh -c 'cd /srv/a && python3 -m http.server 19091 >/dev/null 2>&1'
docker exec -d ngx-acc-nginx sh -c 'cd /srv/b && python3 -m http.server 19092 >/dev/null 2>&1'
docker exec -d ngx-acc-nginx sh -c 'python3 -m http.server 18081 >/dev/null 2>&1'
docker exec -d ngx-acc-nginx sh -c 'python3 -m http.server 18444 >/dev/null 2>&1'
start_container_agent ngx-acc-nginx
wait_agent ngx-acc-nginx

log "N0: starting systemd container..."
# debian:12 基础镜像不带 systemd（/sbin/init 不存在）——启动命令内先装
# systemd + nginx，装完 exec /lib/systemd/systemd 接管 PID 1
docker run -d --privileged --name ngx-acc-systemd \
  --add-host=host.docker.internal:host-gateway \
  debian:12 sh -c \
  'apt-get update -qq >/dev/null 2>&1 && \
   apt-get install -y -qq systemd nginx >/dev/null 2>&1 && \
   exec /lib/systemd/systemd' >/dev/null
for i in $(seq 1 90); do
  STATE=$(docker exec ngx-acc-systemd systemctl is-system-running 2>/dev/null || true)
  [[ "$STATE" == "running" || "$STATE" == "degraded" ]] && break
  if [[ $i -eq 90 ]]; then
    err "systemd 容器未在 90s 内就绪（is-system-running=${STATE}）"
    docker logs ngx-acc-systemd 2>&1 | tail -10 >&2
    exit 1
  fi
  sleep 1
done
docker exec ngx-acc-systemd sh -c \
  'systemctl start nginx 2>/dev/null || systemctl restart nginx'
for i in $(seq 1 10); do
  ACTIVE=$(docker exec ngx-acc-systemd systemctl is-active nginx 2>/dev/null || true)
  [[ "$ACTIVE" == "active" ]] && break
  docker exec ngx-acc-systemd systemctl start nginx 2>/dev/null || true
  sleep 1
done
[[ "$ACTIVE" == "active" ]] || { err "systemd 容器 nginx 未 active"; exit 1; }
start_container_agent ngx-acc-systemd
wait_agent ngx-acc-systemd

log "N0: starting dual-backend container (nginx + traefik dir)..."
docker run -d --name ngx-acc-dual --add-host=host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
docker exec ngx-acc-dual sh -c \
  'apt-get update -qq >/dev/null 2>&1 && \
   apt-get install -y -qq --no-install-recommends nginx >/dev/null 2>&1 && \
   mkdir -p /etc/traefik/dynamic && nginx' \
  || { err "dual 容器初始化失败"; exit 1; }
start_container_agent ngx-acc-dual
wait_agent ngx-acc-dual

log "N0: starting bare container (no backend)..."
docker run -d --name ngx-acc-bare --add-host=host.docker.internal:host-gateway \
  debian:12-slim sleep infinity >/dev/null
start_container_agent ngx-acc-bare
wait_agent ngx-acc-bare
log "N0: four agents online"

# ---- N1: 新建站点全链 ----
api GET "/agents/ngx-acc-nginx/proxy/status"
MODE=$(python3 -c "import json,sys;print(json.load(sys.stdin).get('reloadMode',''))" <<<"$API_BODY")
BACKEND=$(python3 -c "import json,sys;print(json.load(sys.stdin).get('backend',''))" <<<"$API_BODY")
SCOUNT=$(python3 -c "import json,sys;print(json.load(sys.stdin).get('siteCount',-1))" <<<"$API_BODY")
check "N1a status 概览（backend=nginx + confDir + siteCount=0）" \
  "$( [[ "$BACKEND" == "nginx" && -n "$MODE" && "$SCOUNT" == "0" && "$API_BODY" == *'"confDir"'* ]] && echo OK || echo BAD )" \
  "backend=${BACKEND} reloadMode=${MODE} siteCount=${SCOUNT}"

api PUT "/agents/ngx-acc-nginx/proxy/sites/web-a" '{
  "name":"web-a","serverNames":["a.accept.test"],
  "upstream":"127.0.0.1:19091","scheme":"http","websocket":true
}'
check "N1b apply http 站点（websocket）" \
  "$( [[ "$API_CODE" == 200 ]] && echo OK || echo BAD )" \
  "code=${API_CODE} body=${API_BODY:0:120}"

if docker exec ngx-acc-nginx test -f /etc/nginx/conf.d/cockpit-site-web-a.conf; then
  CONF=$(docker exec ngx-acc-nginx cat /etc/nginx/conf.d/cockpit-site-web-a.conf)
  check "N1c 片段落盘（meta 注释 + proxy_pass + Upgrade 头）" \
    "$( [[ "$CONF" == *'# cockpit:meta '* && "$CONF" == *'proxy_pass http://127.0.0.1:19091;'* && "$CONF" == *'Upgrade'* ]] && echo OK || echo BAD )" \
    "meta+proxy_pass+websocket 头齐"
else
  check "N1c 片段落盘（meta 注释 + proxy_pass + Upgrade 头）" "BAD" "片段文件不存在"
fi

MARK=$(curl -fsS -H 'Host: a.accept.test' "http://${REACH_HOST}:${NGINX_HTTP_PORT}/" 2>/dev/null || true)
check "N1d 站点真实可达（marker A）" \
  "$(has "$MARK" NGX-ACC-BACKEND-A)" \
  "body=${MARK:0:40}"

api GET "/agents/ngx-acc-nginx/proxy/sites"
check "N1e sites 列表回读（meta 解析）" \
  "$(has "$API_BODY" '"web-a"')" \
  "body=${API_BODY:0:160}"

api PUT "/agents/ngx-acc-nginx/proxy/sites/web-tls" '{
  "name":"web-tls","serverNames":["tls.accept.test"],
  "upstream":"127.0.0.1:19092","scheme":"https",
  "tlsCert":"/etc/nginx/certs/acc.pem","tlsKey":"/etc/nginx/certs/acc.key"
}'
check "N1f apply https 站点" \
  "$( [[ "$API_CODE" == 200 ]] && echo OK || echo BAD )" \
  "code=${API_CODE} body=${API_BODY:0:120}"

TLSMARK=$(curl -fsS --cacert "$WORK_DIR/certs/acc.pem" \
  --resolve "tls.accept.test:${NGINX_TLS_PORT}:127.0.0.1" \
  "https://tls.accept.test:${NGINX_TLS_PORT}/" 2>/dev/null || true)
check "N1g https 可达且证书真被加载（--cacert 验证）" \
  "$(has "$TLSMARK" NGX-ACC-BACKEND-B)" \
  "body=${TLSMARK:0:40}"

HTTPCODE=""
for i in $(seq 1 8); do
  HTTPCODE=$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: tls.accept.test' \
    "http://${REACH_HOST}:${NGINX_HTTP_PORT}/" 2>/dev/null || true)
  [[ "$HTTPCODE" == 301 ]] && break
  sleep 0.5  # reload 异步窗口期请求可能落进排水中的旧 worker（旧配置无此块）
done
check "N1h 80→443 301 跳转" \
  "$( [[ "$HTTPCODE" == 301 ]] && echo OK || echo BAD )" \
  "http_code=${HTTPCODE}"

# ---- N2: 语法/证书错误同步拦截 + 回滚 ----
# 拦截者其实是 reload：nginx -s reload 的 -s 进程发 SIGHUP 前先自解析全量
# 配置，新片段的语法错/证书缺失在 -s 进程即 emerg、退出非零 → agent 走
# D4 回滚 → 502 透传（pre-write 的 nginx -t 只护存量配置）。bind 冲突
# 不在此列（-s 进程不 bind，见 N4 的异步吞错）。
api PUT "/agents/ngx-acc-nginx/proxy/sites/web-bad" '{
  "name":"web-bad","serverNames":["bad.accept.test"],
  "upstream":"127.0.0.1:19091","scheme":"http",
  "extra":"totally_bogus_directive yes;"
}'
check "N2a bogus extra → 502 同步拦截" \
  "$( [[ "$API_CODE" == 502 ]] && echo OK || echo BAD )" \
  "code=${API_CODE} msg=$(echo "$API_BODY" | head -c 110)"

check "N2b 错误消息带回滚声明（rolled back）" \
  "$(has "$API_BODY" 'rolled back')" ""

check "N2c 拦截后不落盘" \
  "$( docker exec ngx-acc-nginx sh -c 'test ! -f /etc/nginx/conf.d/cockpit-site-web-bad.conf' 2>/dev/null && echo OK || echo BAD )" \
  "cockpit-site-web-bad.conf 应不存在"

api PUT "/agents/ngx-acc-nginx/proxy/sites/web-badcert" '{
  "name":"web-badcert","serverNames":["bc.accept.test"],
  "upstream":"127.0.0.1:19092","scheme":"https",
  "tlsCert":"/nonexistent/acc.pem","tlsKey":"/nonexistent/acc.key"
}'
check "N2d 证书路径不存在 → 502 同步拦截 + 回滚" \
  "$( [[ "$API_CODE" == 502 && "$API_BODY" == *'rolled back'* ]] && echo OK || echo BAD )" \
  "code=${API_CODE} msg=$(echo "$API_BODY" | head -c 100)"

# ---- N3: 无 systemd 环境（signal reload 路径前提） ----
check "N3 容器无 systemctl 且无 systemd 运行（reloadMode=${MODE}）" \
  "$( docker exec ngx-acc-nginx sh -c 'command -v systemctl >/dev/null || test ! -d /run/systemd/system' 2>/dev/null && echo OK || echo BAD )" \
  "reload 走 nginx -s reload signal 分支的直接前提"

# ---- N4: 端口冲突真实语义（nginx reload 异步吞错实证）+ 同步失败回滚 ----
# nginx -s reload 只确认「信号已发」即返回 0：master 侧 bind 失败异步落在
# error.log（emerg），旧配置继续服役；systemctl 模式同构（debian
# nginx.service 的 ExecReload 就是 nginx -s reload）。这是 nginx 固有
# 语义：数据面安全（D4「失败站点照旧」仍成立），上报层无法同步感知。
port_alive() { # 容器内 connect 探测（slim 无 ss/curl）
  docker exec ngx-acc-nginx python3 -c \
    "import socket;s=socket.socket();s.settimeout(2);s.connect(('127.0.0.1',$1));print('OK')" \
    2>/dev/null || echo BAD
}
check "N4a 冲突占位监听在位（python 18081/18444）" \
  "$( [[ "$(port_alive 18081)" == OK && "$(port_alive 18444)" == OK ]] && echo OK || echo BAD )" \
  "18081=$(port_alive 18081) 18444=$(port_alive 18444)"

api PUT "/agents/ngx-acc-nginx/proxy/sites/web-conf80" '{
  "name":"web-conf80","serverNames":["c80.accept.test"],
  "upstream":"127.0.0.1:19091","scheme":"http",
  "extra":"listen 18081;"
}'
check "N4b 80 族冲突 apply 返回 200（异步吞错语义）" \
  "$( [[ "$API_CODE" == 200 ]] && echo OK || echo BAD )" \
  "code=${API_CODE}（nginx -s reload 只确认信号，bind 失败异步）"

check "N4c 冲突站点片段已落盘（上报层失真实证）" \
  "$( docker exec ngx-acc-nginx test -f /etc/nginx/conf.d/cockpit-site-web-conf80.conf 2>/dev/null && echo OK || echo BAD )" \
  "cockpit-site-web-conf80.conf 存在"

EMERG=$(docker exec ngx-acc-nginx sh -c \
  'grep -c "Address already in use" /var/log/nginx/error.log' 2>/dev/null || echo 0)
check "N4d master error.log 留 emerg bind 实证（agent 侧不可见）" \
  "$( [[ "${EMERG:-0}" -ge 1 ]] && echo OK || echo BAD )" \
  "error.log Address already in use ×${EMERG}"

check "N4e 冲突端口仍归 python（nginx 未抢占）" "$(port_alive 18081)" ""

MARK3=$(curl -fsS -H 'Host: a.accept.test' "http://${REACH_HOST}:${NGINX_HTTP_PORT}/" 2>/dev/null || true)
check "N4f 旧站点继续服务（旧配置服役 = 数据面安全）" \
  "$(has "$MARK3" NGX-ACC-BACKEND-A)" ""

api PUT "/agents/ngx-acc-nginx/proxy/sites/web-conf443" '{
  "name":"web-conf443","serverNames":["c443.accept.test"],
  "upstream":"127.0.0.1:19092","scheme":"http",
  "extra":"listen 18444 ssl;\nssl_certificate /etc/nginx/certs/acc.pem;\nssl_certificate_key /etc/nginx/certs/acc.key;"
}'
check "N4g 443 族冲突同语义（200 + 片段落盘 + 端口未动）" \
  "$( [[ "$API_CODE" == 200 ]] && docker exec ngx-acc-nginx test -f /etc/nginx/conf.d/cockpit-site-web-conf443.conf 2>/dev/null && [[ "$(port_alive 18444)" == OK ]] && echo OK || echo BAD )" \
  "code=${API_CODE}"

# 同步失败路径：master 已死时 nginx -s reload 真报非零退出 → D4 回滚可证
docker exec ngx-acc-nginx nginx -s quit 2>/dev/null || true
for i in $(seq 1 10); do
  docker exec ngx-acc-nginx sh -c 'test ! -f /run/nginx.pid' 2>/dev/null && break
  sleep 0.5
done
api PUT "/agents/ngx-acc-nginx/proxy/sites/web-quit" '{
  "name":"web-quit","serverNames":["q.accept.test"],
  "upstream":"127.0.0.1:19091","scheme":"http"
}'
check "N4h master 已死 apply → 502 同步报错 + rolled back" \
  "$( [[ "$API_CODE" == 502 && "$API_BODY" == *'rolled back'* ]] && echo OK || echo BAD )" \
  "code=${API_CODE} msg=$(echo "$API_BODY" | head -c 110)"
check "N4i 回滚后片段未留（原本不存在 → 删除新文件）" \
  "$( docker exec ngx-acc-nginx sh -c 'test ! -f /etc/nginx/conf.d/cockpit-site-web-quit.conf' 2>/dev/null && echo OK || echo BAD )" \
  "cockpit-site-web-quit.conf 应不存在"

# 清掉两个冲突片段后重启 nginx（不然重启时 bind 冲突整个起不来）
docker exec ngx-acc-nginx sh -c \
  'rm -f /etc/nginx/conf.d/cockpit-site-web-conf80.conf /etc/nginx/conf.d/cockpit-site-web-conf443.conf'
docker exec ngx-acc-nginx nginx 2>/dev/null || true
sleep 1
MARK4=$(curl -fsS -H 'Host: a.accept.test' "http://${REACH_HOST}:${NGINX_HTTP_PORT}/" 2>/dev/null || true)
check "N4j nginx 重启后 web-a 恢复服务（片段在盘即配置恢复）" \
  "$(has "$MARK4" NGX-ACC-BACKEND-A)" ""

# ---- N5: systemd 路径 ----
api GET "/agents/ngx-acc-systemd/proxy/status"
SMODE=$(python3 -c "import json,sys;print(json.load(sys.stdin).get('reloadMode',''))" <<<"$API_BODY")
check "N5a systemd 容器 reloadMode=systemctl" \
  "$( [[ "$SMODE" == "systemctl" ]] && echo OK || echo BAD )" \
  "reloadMode=${SMODE}"

api PUT "/agents/ngx-acc-systemd/proxy/sites/web-sys" '{
  "name":"web-sys","serverNames":["sys.accept.test"],
  "upstream":"127.0.0.1:19091","scheme":"http"
}'
check "N5b apply 走 systemctl reload nginx" \
  "$( [[ "$API_CODE" == 200 ]] && echo OK || echo BAD )" \
  "code=${API_CODE}"

ACTIVE=$(docker exec ngx-acc-systemd systemctl is-active nginx 2>/dev/null || true)
check "N5c unit 保持 active + 片段落盘" \
  "$( [[ "$ACTIVE" == "active" ]] && docker exec ngx-acc-systemd test -f /etc/nginx/conf.d/cockpit-site-web-sys.conf 2>/dev/null && echo OK || echo BAD )" \
  "is-active=${ACTIVE} 片段应存在"

# ---- N6: 双后端分流（nginx 优先） ----
CAPS=$(agent_capabilities ngx-acc-dual)
check "N6a 双 capability 上报（nginx-proxy + traefik-proxy）" \
  "$( [[ "$CAPS" == *nginx-proxy* && "$CAPS" == *traefik-proxy* ]] && echo OK || echo BAD )" \
  "caps=${CAPS}"

api PUT "/agents/ngx-acc-dual/proxy/sites/web-dual" '{
  "name":"web-dual","serverNames":["dual.accept.test"],
  "upstream":"127.0.0.1:19091","scheme":"http"
}'
check "N6b 分流 nginx 优先（apply 落 conf.d 而非 traefik 目录）" \
  "$( [[ "$API_CODE" == 200 ]] && docker exec ngx-acc-dual test -f /etc/nginx/conf.d/cockpit-site-web-dual.conf 2>/dev/null && echo OK || echo BAD )" \
  "code=${API_CODE} conf.d/cockpit-site-web-dual.conf 应存在"

# ---- N7: 旧 agent 回退路径 ----
api PUT "/agents/ngx-acc-bare/proxy/sites/web-x" '{
  "name":"web-x","serverNames":["x.accept.test"],
  "upstream":"127.0.0.1:19091","scheme":"http"
}'
check "N7 无后端 agent：回退 nginx. 前缀 → unknown provider 502" \
  "$( [[ "$API_CODE" == 502 && "$API_BODY" == *'unknown provider'* ]] && echo OK || echo BAD )" \
  "code=${API_CODE} msg=$(echo "$API_BODY" | head -c 80)"

# ---- 收尾：删除站点（回滚恢复路径的反向） ----
api DELETE "/agents/ngx-acc-nginx/proxy/sites/web-a"
check "N8 delete 站点 200 + 片段移除" \
  "$( [[ "$API_CODE" == 200 ]] && docker exec ngx-acc-nginx sh -c 'test ! -f /etc/nginx/conf.d/cockpit-site-web-a.conf' 2>/dev/null && echo OK || echo BAD )" \
  "code=${API_CODE} 片段应已移除"

log "nginx acceptance: PASS=${PASS} FAIL=${FAIL}"
[[ "$FAIL" -eq 0 ]] || exit 1
log "All nginx proxy checks passed ✓ (N0-N8)"
