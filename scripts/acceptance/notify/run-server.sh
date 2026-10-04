#!/usr/bin/env bash
# 通知渠道验收：本地 cockpit 实例（:19997）+ webhook 接收器（:9700）。
#   webhook 渠道 1  → http://127.0.0.1:9700/hook（secret 鉴权，活样本）
#   webhook 渠道 2  → http://127.0.0.1:9799/hook（死端口，投递失败样本）
# 「测试通知」= POST /api/notification/test（TestAll 绕过事件白名单，
# 逐渠道返回结果）→ 探针核对 API 结果面与接收器实际收包面。
# 产物（.acceptance/notify/instance/）：配置/db/logs/token/pid/收包 JSONL
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
NT_DIR="${REPO_ROOT}/.acceptance/notify"
WORK_DIR="${NT_DIR}/instance"
PORT=19997
RECEIVER_PORT=9700
SECRET="notify-accept-secret"

ADMIN_USER="${ADMIN_USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASSWORD:-e2e-strong-pass-1}"

mkdir -p "${WORK_DIR}"/{data,bin,logs}

# 复跑归一：清上一轮 DB 与收包记录——审计/收包断言都是本轮基线
rm -f "${WORK_DIR}/data/cockpit.db" "${WORK_DIR}/data/cockpit.db-"*
rm -f "${NT_DIR}/evidence/webhooks.jsonl"

cat > "${WORK_DIR}/cockpit.yaml" <<EOF
server:
  host: 127.0.0.1
  port: ${PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: notify-accept-jwt-secret
  expiration: 2h
notification:
  enabled: true
  webhook:
    - url: http://127.0.0.1:${RECEIVER_PORT}/hook
      secret: ${SECRET}
    - url: http://127.0.0.1:9799/hook
      secret: dead-port
EOF

echo "== 启动 webhook 接收器 (:${RECEIVER_PORT}) =="
python3 "${REPO_ROOT}/scripts/acceptance/webhook_receiver.py" \
    "${SECRET}" "${NT_DIR}/evidence/webhooks.jsonl" \
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
rm -f "${NT_DIR}/evidence/webhooks.jsonl"
echo "接收器 ready (pid=$(cat "${WORK_DIR}/receiver.pid"))"

echo "== 构建并启动 server (:${PORT}) =="
go build -o "${WORK_DIR}/bin/cockpit" ./cmd/cockpit
ADMIN_USERNAME="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASS}" \
    "${WORK_DIR}/bin/cockpit" server -config "${WORK_DIR}/cockpit.yaml" \
    > "${WORK_DIR}/logs/server.log" 2>&1 &
SERVER_PID=$!
echo "${SERVER_PID}" > "${WORK_DIR}/server.pid"

for i in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then break; fi
    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
        echo "server 启动失败："; tail -n 30 "${WORK_DIR}/logs/server.log"; exit 1
    fi
    sleep 1
done
curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null || { echo "health 超时"; exit 1; }

TOKEN=""
for i in $(seq 1 30); do
    TOKEN=$(curl -sf -X POST "http://127.0.0.1:${PORT}/api/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
        | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' 2>/dev/null) || TOKEN=""
    [[ -n "${TOKEN}" ]] && break
    sleep 1
done
[[ -n "${TOKEN}" ]] || { echo "登录失败"; tail -n 20 "${WORK_DIR}/logs/server.log"; exit 1; }
echo "${TOKEN}" > "${WORK_DIR}/token"
echo "server ready (pid=${SERVER_PID})，token 已存 ${WORK_DIR}/token"
