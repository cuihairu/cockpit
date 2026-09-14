#!/usr/bin/env bash
# Cockpit Compose Stack（P1/M1）本地端到端验收。
#
# 本机无需真实 Docker：以「真实 server 二进制 + 真实 agent 二进制 +
# fake Docker Engine（fake-docker-daemon）+ fake compose CLI（fake-docker）」
# 跑通 Web API → JWT → 审计 → WebSocket RPC → stack provider → compose 全链路。
# 真实 Docker 主机上的验收项见 docs/guide/stack-deploy-design.md 的 M1 清单。
#
# 覆盖断言：
#   A1  agent 注册且带 docker-api capability
#   A2  创建 stack（compose 校验通过，created=true，审计 stack.create）
#   A3  compose/.env 回读一致
#   A4  非法 compose 校验失败 → 非 2xx 且不落盘
#   A5  列表包含新 stack
#   A6  up 返回 taskId（异步任务）
#   A7  同名并发 up → 409
#   A8  任务轮询到 success
#   A9  status 联动 fake engine：running/total 正确
#   A10 compose logs 返回日志文本
#   A11 down → 任务 success → running 归零
#   A12 remove → 目录删除、列表移除
#   A13 agent 断连 → 列表显示缓存灰态 online=false
#   A14 审计含 stack up/down/remove/restart/pull 事件；.env 内容不泄漏进审计
#   A15 restart / pull 动作（M1.5）→ 任务 success
#   A16 部署历史：server 记录 + 后台回填终态（含 up/restart/pull）
#   A17 目录自检信息（stack.info → agentInfo.dirWritable）
set -u

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HERE="$(cd "$(dirname "$0")" && pwd)"
WORK="$(mktemp -d /tmp/cockpit-accept.XXXXXX)"
PORT="${ACCEPTANCE_PORT:-18099}"
BASE="http://127.0.0.1:$PORT"
API="$BASE/api"
ADMIN_PASS='Accept-Test-2026'
AGENT_ID='accept-agent'
ENV_SECRET='accept-secret-9f3a'

PASS=0
FAIL=0
SERVER_PID=''
AGENT_PID=''
DAEMON_PID=''

cleanup() {
  for pid in "$AGENT_PID" "$SERVER_PID" "$DAEMON_PID"; do
    [ -n "$pid" ] && kill "$pid" 2>/dev/null
  done
  wait 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT

step()  { printf '\n\033[1m== %s ==\033[0m\n' "$*"; }
pass()  { printf 'PASS: %s\n' "$*"; PASS=$((PASS+1)); }
fail()  { printf 'FAIL: %s\n' "$*"; FAIL=$((FAIL+1)); }

# check_eq <名称> <实际> <期望>
check_eq() {
  if [ "$2" = "$3" ]; then pass "$1 ($2)"; else fail "$1 — got [$2] want [$3]"; fi
}
# check_ne <名称> <实际> <不期望>
check_ne() {
  if [ "$2" != "$3" ]; then pass "$1 ($2)"; else fail "$1 — unexpectedly [$2]"; fi
}
retry() { # retry <次数> <命令...>：命令成功（exit 0）即返回
  local n="$1"; shift
  for _ in $(seq "$n"); do
    if "$@" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  return 1
}

step "0. 构建 server / agent"
( cd "$ROOT" && go build -o "$WORK/cockpit" ./cmd/cockpit ) || { fail "build server"; exit 1; }
( cd "$ROOT" && go build -o "$WORK/cockpit-agent" ./cmd/cockpit-agent ) || { fail "build agent"; exit 1; }
pass "build ok"

step "1. 启动 fake Docker Engine + fake compose CLI"
mkdir -p "$WORK/bin" "$WORK/state" "$WORK/stacks"
cp "$HERE/fake-docker" "$WORK/bin/docker"
chmod +x "$WORK/bin/docker"
FAKE_DOCKER_SOCK="$WORK/docker.sock" FAKE_COMPOSE_STATE_DIR="$WORK/state" \
  go run "$HERE/fake-docker-daemon.go" >"$WORK/daemon.log" 2>&1 &
DAEMON_PID=$!
retry 10 bash -c "[ -S '$WORK/docker.sock' ]" || { fail "fake daemon socket"; exit 1; }
pass "fake engine on $WORK/docker.sock"

step "2. 启动 server"
cat > "$WORK/config.yaml" <<EOF
server:
  host: 127.0.0.1
  port: $PORT
database:
  path: $WORK/cockpit.db
jwt:
  secret: acceptance-test-jwt-secret-0123456789
  expiration: 1h
EOF
ADMIN_USERNAME=admin ADMIN_PASSWORD="$ADMIN_PASS" \
  "$WORK/cockpit" server -config "$WORK/config.yaml" >"$WORK/server.log" 2>&1 &
SERVER_PID=$!
retry 30 curl -s -o /dev/null "$API/auth/login" || { fail "server did not start"; tail -20 "$WORK/server.log"; exit 1; }
pass "server on :$PORT"

TOKEN="$(curl -s -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$ADMIN_PASS\"}" | jq -r '.token // empty')"
check_eq "登录获取 token" "$([ -n "$TOKEN" ] && echo yes || echo no)" "yes"
AUTH=(-H "Authorization: Bearer $TOKEN")

step "3. 启动 agent（fake engine + fake compose CLI 在 PATH）"
DOCKER_HOST="unix://$WORK/docker.sock" \
COCKPIT_STACKS_DIR="$WORK/stacks" \
FAKE_COMPOSE_STATE_DIR="$WORK/state" \
PATH="$WORK/bin:$PATH" \
  "$WORK/cockpit-agent" start -server "ws://127.0.0.1:$PORT/ws" -id "$AGENT_ID" >"$WORK/agent.log" 2>&1 &
AGENT_PID=$!

ag_online() {
  curl -s -H "Authorization: Bearer $TOKEN" "$API/agents" | jq -e \
    --arg id "$AGENT_ID" '(.agents? // .)[] | select(.id == $id and .status == "online")' >/dev/null
}
retry 30 ag_online || { fail "agent 上线"; tail -20 "$WORK/agent.log"; exit 1; }
pass "agent $AGENT_ID 在线"

cap_ok() {
  curl -s -H "Authorization: Bearer $TOKEN" "$API/agents" | jq -e \
    --arg id "$AGENT_ID" '(.agents? // .)[] | select(.id == $id)
      | (.capabilities | index("docker-api"))' >/dev/null
}
check_eq "A1 agent 带 docker-api capability" "$(cap_ok && echo yes || echo no)" "yes"

COMPOSE_OK='services:
  nginx:
    image: nginx:alpine
    ports:
      - "8080:80"
  redis:
    image: redis:7-alpine
'
COMPOSE_BAD='services:
  web:
    image: nginx:alpine
    invalid-something: true
'

step "A2. 创建 stack demo（compose + .env）"
resp="$(curl -s -w '\n%{http_code}' -X PUT "$API/stacks/agents/$AGENT_ID/demo/compose" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "$(jq -n --arg c "$COMPOSE_OK" --arg e "SECRET_VALUE=$ENV_SECRET" '{compose:$c,env:$e}')")"
code="$(echo "$resp" | tail -1)"; body="$(echo "$resp" | head -n -1)"
check_eq "A2 保存返回 200" "$code" "200"
check_eq "A2 created=true（审计 stack.create）" "$(echo "$body" | jq -r '.created // empty')" "true"

step "A3. 回读 compose / .env"
# 内容含换行，用 md5 对比（jq -j 与 printf '%s' 都不追加尾换行）
want_md5="$(printf '%s' "$COMPOSE_OK" | md5sum | cut -d' ' -f1)"
readback="$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/demo/compose")"
check_eq "A3 compose 内容一致" "$(echo "$readback" | jq -j '.compose' | md5sum | cut -d' ' -f1)" "$want_md5"
check_eq "A3 env 内容一致" "$(echo "$readback" | jq -r '.env')" "SECRET_VALUE=$ENV_SECRET"

step "A4. 非法 compose 校验失败不落盘"
code="$(curl -s -o /dev/null -w '%{http_code}' -X PUT "$API/stacks/agents/$AGENT_ID/demo/compose" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "$(jq -n --arg c "$COMPOSE_BAD" '{compose:$c}')")"
check_ne "A4 非法 compose 被拒绝" "$code" "200"
after="$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/demo/compose")"
check_eq "A4 原文件未被覆盖" "$(echo "$after" | jq -j '.compose' | md5sum | cut -d' ' -f1)" "$want_md5"

step "A5. 列表包含 demo"
listed() {
  # -L：/api/stacks 无尾斜杠会被 ServeMux 301（浏览器 XHR 自动跟随）
  curl -sL "${AUTH[@]}" "$API/stacks" | jq -e --arg id "$AGENT_ID" \
    '(.stacks? // .)[] | select(.agentId == $id and .name == "demo")' >/dev/null
}
retry 10 listed
check_eq "A5 列表含 demo" "$(listed && echo yes || echo no)" "yes"

step "A6. up → taskId（异步任务）"
resp="$(curl -s -w '\n%{http_code}' -X POST "$API/stacks/agents/$AGENT_ID/demo/up" "${AUTH[@]}")"
code="$(echo "$resp" | tail -1)"; body="$(echo "$resp" | head -n -1)"
check_eq "A6 up 返回 200" "$code" "200"
TASK_ID="$(echo "$body" | jq -r '.taskId // empty')"
check_ne "A6 返回 taskId" "$TASK_ID" ""

step "A7. 同名并发 up → 409"
code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/stacks/agents/$AGENT_ID/demo/up" "${AUTH[@]}")"
check_eq "A7 并发 up 得 409" "$code" "409"

step "A8. 任务轮询到 success"
task_ok() {
  curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/tasks/$TASK_ID" | jq -e '.status == "success"' >/dev/null
}
retry 30 task_ok
check_eq "A8 任务 success" "$(task_ok && echo yes || echo no)" "yes"

step "A9. status 联动 fake engine（2 服务 running）"
st="$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/demo")"
check_eq "A9 running=2" "$(echo "$st" | jq -r '.running // empty')" "2"
check_eq "A9 total=2" "$(echo "$st" | jq -r '.total // empty')" "2"

step "A10. compose logs"
logs="$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/demo/logs")"
check_eq "A10 日志含标记行" "$(echo "$logs" | grep -c 'accept-logs-ok' || true)" "1"

step "A11. down → running 归零"
resp="$(curl -s -X POST "$API/stacks/agents/$AGENT_ID/demo/down" "${AUTH[@]}")"
DOWN_TASK="$(echo "$resp" | jq -r '.taskId // empty')"
check_ne "A11 down 返回 taskId" "$DOWN_TASK" ""
retry 30 bash -c "curl -s '${AUTH[@]}' '$API/stacks/agents/$AGENT_ID/tasks/$DOWN_TASK' | jq -e '.status == \"success\"' >/dev/null"
check_eq "A11 down 任务 success" "$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/tasks/$DOWN_TASK" | jq -r '.status')" "success"

step "A12. remove → 目录删除"
# 先建第二个 stack 供 A13 灰态使用
curl -s -o /dev/null -X PUT "$API/stacks/agents/$AGENT_ID/cache-demo/compose" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "$(jq -n --arg c "$COMPOSE_OK" '{compose:$c}')"
resp="$(curl -s -X DELETE "$API/stacks/agents/$AGENT_ID/demo" "${AUTH[@]}")"
RM_TASK="$(echo "$resp" | jq -r '.taskId // empty')"
check_ne "A12 remove 返回 taskId" "$RM_TASK" ""
retry 30 bash -c "curl -s '${AUTH[@]}' '$API/stacks/agents/$AGENT_ID/tasks/$RM_TASK' | jq -e '.status == \"success\"' >/dev/null"
check_eq "A12 remove 任务 success" "$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/tasks/$RM_TASK" | jq -r '.status')" "success"
check_eq "A12 磁盘目录已删" "$([ -d "$WORK/stacks/demo" ] && echo exists || echo gone)" "gone"
gone() {
  curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID" | jq -e '(.stacks? // .)[] | select(.name == "demo")' >/dev/null 2>&1 && echo no || echo yes
}
retry 10 bash -c "true"
check_eq "A12 列表已移除 demo" "$(gone)" "yes"

step "A15. cache-demo 重建 + restart / pull（M1.5 新动作）"
resp="$(curl -s -X POST "$API/stacks/agents/$AGENT_ID/cache-demo/up" "${AUTH[@]}")"
UP2_TASK="$(echo "$resp" | jq -r '.taskId // empty')"
retry 30 bash -c "curl -s '${AUTH[@]}' '$API/stacks/agents/$AGENT_ID/tasks/$UP2_TASK' | jq -e '.status == \"success\"' >/dev/null"
check_eq "A15.1 cache-demo up success" "$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/tasks/$UP2_TASK" | jq -r '.status')" "success"
resp="$(curl -s -w '\n%{http_code}' -X POST "$API/stacks/agents/$AGENT_ID/cache-demo/restart" "${AUTH[@]}")"
code="$(echo "$resp" | tail -1)"; body="$(echo "$resp" | head -n -1)"
check_eq "A15.2 restart 返回 200" "$code" "200"
RESTART_TASK="$(echo "$body" | jq -r '.taskId // empty')"
retry 30 bash -c "curl -s '${AUTH[@]}' '$API/stacks/agents/$AGENT_ID/tasks/$RESTART_TASK' | jq -e '.status == \"success\"' >/dev/null"
check_eq "A15.3 restart 任务 success" "$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/tasks/$RESTART_TASK" | jq -r '.status')" "success"
resp="$(curl -s -X POST "$API/stacks/agents/$AGENT_ID/cache-demo/pull" "${AUTH[@]}")"
PULL_TASK="$(echo "$resp" | jq -r '.taskId // empty')"
retry 30 bash -c "curl -s '${AUTH[@]}' '$API/stacks/agents/$AGENT_ID/tasks/$PULL_TASK' | jq -e '.status == \"success\"' >/dev/null"
check_eq "A15.4 pull 任务 success" "$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/tasks/$PULL_TASK" | jq -r '.status')" "success"

step "A16. 部署历史（server 侧记录 + 后台回填终态）"
hist_ok() {
  curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/cache-demo/history" | jq -e \
    '[.deployments[] | select(.action == "up" and .status == "success" and .finishedAt > 0)] | length > 0' >/dev/null
}
retry 30 hist_ok
check_eq "A16.1 up 历史带终态" "$(hist_ok && echo yes || echo no)" "yes"
actions="$(curl -s "${AUTH[@]}" "$API/stacks/agents/$AGENT_ID/cache-demo/history" | jq -r '[.deployments[].action] | unique | sort | join(",")')"
check_eq "A16.2 历史含 up/restart/pull" "$actions" "pull,restart,up"

step "A17. 目录自检信息（stack.info）"
agg="$(curl -sL "${AUTH[@]}" "$API/stacks")"
check_eq "A17.1 聚合响应含 agentInfo" "$(echo "$agg" | jq -r --arg id "$AGENT_ID" '.agentInfo[$id].dirWritable // empty')" "true"
check_eq "A17.2 自检带 stacks 目录路径" "$(echo "$agg" | jq -r --arg id "$AGENT_ID" '.agentInfo[$id].dir // empty' | grep -c 'stacks' || true)" "1"

step "A13. agent 断连 → 缓存灰态"
kill "$AGENT_PID" 2>/dev/null; wait "$AGENT_PID" 2>/dev/null; AGENT_PID=''
gray() {
  curl -sL "${AUTH[@]}" "$API/stacks" | jq -e --arg id "$AGENT_ID" \
    '(.stacks? // .)[] | select(.agentId == $id and .name == "cache-demo" and .online == false)' >/dev/null
}
retry 30 gray
check_eq "A13 cache-demo 灰态 online=false" "$(gray && echo yes || echo no)" "yes"

step "A14. 审计事件 + .env 不泄漏"
audit="$(curl -s "${AUTH[@]}" "$API/admin/audit/logs?page=1&page_size=100&resource=stack")"
for ev in stack_create stack_up stack_down stack_remove stack_restart stack_pull; do
  check_eq "A14 审计含 $ev" "$(echo "$audit" | grep -c "$ev" || true)" "1"
done
check_eq "A14 .env 值不进审计" "$(echo "$audit" | grep -c "$ENV_SECRET" || true)" "0"

step "汇总"
printf 'PASS: %d  FAIL: %d\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
