#!/usr/bin/env bash
# Workflow 编排验收：本地 cockpit 实例（端口 20010，单在线 agent）：
#   a1 workflows-acc-a1  在线执行目标（成功链/失败停/重试/取消场景）
#   离线样本用从未注册的 ghost id（workflows-acc-ghost）——registry 缺席即
#   503，与「曾在线后掉线」走同一 registry.Get 分支（jobs 探针同口径注明）
# 产物（.acceptance/workflows/instance/）：二进制/配置/db/logs/token/各 pid
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WF_DIR="${REPO_ROOT}/.acceptance/workflows"
WORK_DIR="${WF_DIR}/instance"
PORT=20010
A1_ID=workflows-acc-a1

ADMIN_USER="${ADMIN_USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASSWORD:-e2e-strong-pass-1}"

mkdir -p "${WORK_DIR}"/{data,bin,logs}

# 复跑归一：清上一轮实例 DB——本轮断言基线是空台账（W2 run 台账恰 N 条等
# 按计数对照），旧 Workflow/Job 行会污染；用户与角色由启动 env 重建
rm -f "${WORK_DIR}/data/cockpit.db" "${WORK_DIR}/data/cockpit.db-"*

cat > "${WORK_DIR}/cockpit.yaml" <<EOF
server:
  host: 127.0.0.1
  port: ${PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: workflows-accept-jwt-secret
  expiration: 2h
EOF

echo "== 构建二进制 =="
go build -o "${WORK_DIR}/bin/" ./cmd/cockpit ./cmd/cockpit-agent

echo "== 启动 server =="
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
echo "server ready (pid=${SERVER_PID}, :${PORT})"

echo "== 启动 agent（${A1_ID}）=="
"${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id "${A1_ID}" \
    > "${WORK_DIR}/logs/agent-a1.log" 2>&1 &
echo $! > "${WORK_DIR}/agent-a1.pid"

# 等 agent 注册 + 登录拿 token
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
    echo "agent 未注册在线——检查 agent 日志："; tail -n 5 "${WORK_DIR}/logs/agent-a1.log"; exit 1
}
echo "${TOKEN}" > "${WORK_DIR}/token"
echo "agent ${A1_ID} 在线，token 已存 ${WORK_DIR}/token"
