#!/usr/bin/env bash
# 远控三协议验收：本地 cockpit 实例（scripts/e2e-smoke.sh 同路数）
#   - server + agent 本地起（端口 19990）
#   - GUACD_ADDR/GUACD_RECORDING_PATH 指向 setup-env.sh 起的 guacd
#   - 出口策略用 allow-list（127.0.0.1）而非 allow_arbitrary——顺带验拒绝分支
# 产物（.acceptance/guac/instance/）：二进制/配置/db/logs/token
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
GUAC_DIR="${REPO_ROOT}/.acceptance/guac"
WORK_DIR="${GUAC_DIR}/instance"
REC_DIR="${GUAC_DIR}/rec"
PORT=19990

ADMIN_USER="${ADMIN_USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASSWORD:-e2e-strong-pass-1}"

mkdir -p "${WORK_DIR}"/{data,bin,logs}

cat > "${WORK_DIR}/cockpit.yaml" <<EOF
server:
  host: 127.0.0.1
  port: ${PORT}
  static_dir: ${REPO_ROOT}/web/dist
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: guac-accept-jwt-secret
  expiration: 2h
remote_control:
  allow_arbitrary_target: false
  allowed_targets:
    - 127.0.0.1
EOF

echo "== 构建二进制 =="
go build -o "${WORK_DIR}/bin/" ./cmd/cockpit ./cmd/cockpit-agent

echo "== 启动 server =="
ADMIN_USERNAME="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASS}" \
GUACD_ADDR=127.0.0.1:4822 \
GUACD_RECORDING_PATH="${REC_DIR}" \
"${WORK_DIR}/bin/cockpit" server -config "${WORK_DIR}/cockpit.yaml" \
    > "${WORK_DIR}/logs/server.log" 2>&1 &
SERVER_PID=$!
echo "${SERVER_PID}" > "${WORK_DIR}/server.pid"

for i in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then break; fi
    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
        echo "server 启动失败："; tail -30 "${WORK_DIR}/logs/server.log"; exit 1
    fi
    sleep 1
done
curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null || { echo "health 超时"; exit 1; }
echo "server ready (pid=${SERVER_PID}, :${PORT})"

echo "== 启动 agent =="
"${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id guac-acc-agent \
    > "${WORK_DIR}/logs/agent.log" 2>&1 &
AGENT_PID=$!
echo "${AGENT_PID}" > "${WORK_DIR}/agent.pid"

# 等 agent 注册上线
for i in $(seq 1 30); do
    TOKEN=$(curl -sf -X POST "http://127.0.0.1:${PORT}/api/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
        | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' 2>/dev/null) || TOKEN=""
    if [[ -n "${TOKEN}" ]]; then
        COUNT=$(curl -sf "http://127.0.0.1:${PORT}/api/agents" -H "Authorization: Bearer ${TOKEN}" \
            | python3 -c 'import json,sys; d=json.load(sys.stdin); print(len(d.get("agents", d)) if isinstance(d, dict) else len(d))' 2>/dev/null) || COUNT=0
        [[ "${COUNT}" != "0" ]] && break
    fi
    sleep 1
done
[[ -n "${TOKEN}" ]] || { echo "登录失败"; tail -20 "${WORK_DIR}/logs/server.log"; exit 1; }
echo "${TOKEN}" > "${WORK_DIR}/token"
echo "agent 在线（/api/agents=${COUNT}），token 已存 ${WORK_DIR}/token"
