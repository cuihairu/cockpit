#!/usr/bin/env bash
# Agent 文件备份验收：本地 cockpit 实例（端口 19998，单在线 agent）+ webhook
# 接收器（:9701，收 backup.failed 失败通知）：
#   abk-acc-a1   备份执行目标（打包/恢复/下载全链在本机 agent）
# 产物（.acceptance/agent-backup/instance/）：二进制/配置/db/logs/token/各 pid
# 复跑归一：清上一轮 DB 与收包记录（调度/运行历史断言都是本轮基线）。
# 停机补跑场景由 restart-server.sh 只重启 server（保 DB/agent/接收器）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
AB_DIR="${REPO_ROOT}/.acceptance/agent-backup"
WORK_DIR="${AB_DIR}/instance"
PORT=19998
RECEIVER_PORT=9701
SECRET="abk-accept-secret"
A1_ID=abk-acc-a1

ADMIN_USER="${ADMIN_USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASSWORD:-e2e-strong-pass-1}"

mkdir -p "${WORK_DIR}"/{data,bin,logs}

# 复跑归一：清上一轮实例 DB——历史/调度断言以本轮为基线；接收器收包记录同理
rm -f "${WORK_DIR}/data/cockpit.db" "${WORK_DIR}/data/cockpit.db-"*
rm -f "${AB_DIR}/evidence/webhooks.jsonl"

cat > "${WORK_DIR}/cockpit.yaml" <<EOF
server:
  host: 127.0.0.1
  port: ${PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: abk-accept-jwt-secret
  expiration: 4h
notification:
  enabled: true
  webhook:
    - url: http://127.0.0.1:${RECEIVER_PORT}/hook
      secret: ${SECRET}
  events:
    backup.failed:
      type: backup.failed   # 白名单按 EventConfig.Type 字段匹配（service.IsEventEnabled）
      enabled: true
EOF

echo "== 启动 webhook 接收器 (:${RECEIVER_PORT}) =="
python3 "${REPO_ROOT}/scripts/acceptance/webhook_receiver.py" \
    "${SECRET}" "${AB_DIR}/evidence/webhooks.jsonl" "${RECEIVER_PORT}" \
    > "${WORK_DIR}/logs/receiver.log" 2>&1 &
echo $! > "${WORK_DIR}/receiver.pid"
sleep 1
if ! kill -0 "$(cat "${WORK_DIR}/receiver.pid")" 2>/dev/null; then
    echo "接收器启动失败："; cat "${WORK_DIR}/logs/receiver.log"; exit 1
fi
curl -sf -o /dev/null -X POST "http://127.0.0.1:${RECEIVER_PORT}/hook" \
    -H "X-Cockpit-Secret: ${SECRET}" -d 'ping' \
    || { echo "接收器健康检查失败"; exit 1; }
# 健康探针包不计入送达证据
rm -f "${AB_DIR}/evidence/webhooks.jsonl"
echo "接收器 ready (pid=$(cat "${WORK_DIR}/receiver.pid"))"

echo "== 构建二进制 =="
go build -o "${WORK_DIR}/bin/" ./cmd/cockpit ./cmd/cockpit-agent

echo "== 启动 server =="
ADMIN_USERNAME="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASS}" \
    "${WORK_DIR}/bin/cockpit" server -config "${WORK_DIR}/cockpit.yaml" \
    > "${WORK_DIR}/logs/server.log" 2>&1 &
SERVER_PID=$!
echo "${SERVER_PID}" > "${WORK_DIR}/server.pid"

for i in $(seq 1 30); do
    curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1 && break
    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
        echo "server 启动失败："; tail -n 30 "${WORK_DIR}/logs/server.log"; exit 1
    fi
    sleep 1
done
curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null || { echo "health 超时"; exit 1; }
echo "server ready (pid=${SERVER_PID}, :${PORT})"

echo "== 启动 agent（${A1_ID}）=="
"${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id "${A1_ID}" \
    > "${WORK_DIR}/logs/agent-a1.log" 2>&1 &
echo $! > "${WORK_DIR}/agent-a1.pid"

TOKEN=""
for i in $(seq 1 60); do
    TOKEN=$(curl -sf -X POST "http://127.0.0.1:${PORT}/api/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
        | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' 2>/dev/null) || TOKEN=""
    if [[ -n "${TOKEN}" ]]; then
        ONLINE=$(curl -sf "http://127.0.0.1:${PORT}/api/agents" -H "Authorization: Bearer ${TOKEN}" \
            | python3 -c 'import json,sys; d=json.load(sys.stdin); a=d.get("agents",d) if isinstance(d,dict) else d; print(sum(1 for x in a if x.get("status")=="online"))' 2>/dev/null) || ONLINE=0
        [[ "${ONLINE}" == "1" ]] && break
    fi
    sleep 1
done
[[ -n "${TOKEN}" ]] || { echo "登录失败"; tail -n 20 "${WORK_DIR}/logs/server.log"; exit 1; }
[[ "${ONLINE}" == "1" ]] || {
    echo "agent 未注册在线——检查 agent 日志："; tail -n 20 "${WORK_DIR}/logs/agent-a1.log"; exit 1
}
echo "${TOKEN}" > "${WORK_DIR}/token"
echo "agent ${A1_ID} 在线，token 已存 ${WORK_DIR}/token"
