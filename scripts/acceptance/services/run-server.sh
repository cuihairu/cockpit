#!/usr/bin/env bash
# 服务管理验收：本地 cockpit 实例（端口 19993，双 agent 对照）：
#   a1 svc-acc-a1  root（sudo -n 起）→ restart/enable/disable/mask/unmask
#                  成功组（系统级 unit 管理特权）
#   a2 svc-acc-a2  非 root（cui，继承会话 env）→ polkit 拒非交互授权
#                  （实测「Access denied as the requested operation requires
#                  interactive authentication」）→ 报错透传组
# 产物（.acceptance/services/instance/）：二进制/配置/db/logs/token/各 pid
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SV_DIR="${REPO_ROOT}/.acceptance/services"
WORK_DIR="${SV_DIR}/instance"
PORT=19993
A1_ID=svc-acc-a1
A2_ID=svc-acc-a2

ADMIN_USER="${ADMIN_USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASSWORD:-e2e-strong-pass-1}"

mkdir -p "${WORK_DIR}"/{data,bin,logs}

# 复跑归一：清上一轮实例 DB——/api/agents 是 DB 视图，旧 agent 行（status
# 恒 online，server 未感知其被杀）会污染注册计数；admin 由启动 env 重建
rm -f "${WORK_DIR}/data/cockpit.db" "${WORK_DIR}/data/cockpit.db-"*

cat > "${WORK_DIR}/cockpit.yaml" <<EOF
server:
  host: 127.0.0.1
  port: ${PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: services-accept-jwt-secret
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

echo "== 启动 agent ×2（root 对照 + 非 root 报错组）=="
# a1：root——sudo 起，env 显式传递；日志重定向在外层 shell（cui 建文件），
# pid 为 sudo 进程，stop 时转发信号给 agent
sudo -n env COCKPIT_DRIFT_BASELINE="${SV_DIR}/drift-baseline-a1.json" \
    "${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id "${A1_ID}" \
    > "${WORK_DIR}/logs/agent-a1.log" 2>&1 &
echo $! > "${WORK_DIR}/agent-a1.pid"

# a2：非 root（cui 后台进程，继承会话 env——polkit subject 与直跑实验同源）
COCKPIT_DRIFT_BASELINE="${SV_DIR}/drift-baseline-a2.json" \
    "${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id "${A2_ID}" \
    > "${WORK_DIR}/logs/agent-a2.log" 2>&1 &
echo $! > "${WORK_DIR}/agent-a2.pid"

# 等 2 个 agent 全部注册 + 登录拿 token
TOKEN=""
for i in $(seq 1 60); do
    TOKEN=$(curl -sf -X POST "http://127.0.0.1:${PORT}/api/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
        | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' 2>/dev/null) || TOKEN=""
    if [[ -n "${TOKEN}" ]]; then
        COUNT=$(curl -sf "http://127.0.0.1:${PORT}/api/agents" -H "Authorization: Bearer ${TOKEN}" \
            | python3 -c 'import json,sys; d=json.load(sys.stdin); print(len(d.get("agents", d)) if isinstance(d, dict) else len(d))' 2>/dev/null) || COUNT=0
        [[ "${COUNT}" == "2" ]] && break
    fi
    sleep 1
done
[[ -n "${TOKEN}" ]] || { echo "登录失败"; tail -n 20 "${WORK_DIR}/logs/server.log"; exit 1; }
[[ "${COUNT}" == "2" ]] || {
    echo "agent 注册不全（${COUNT}/2）——检查各 agent 日志："
    tail -n 5 "${WORK_DIR}/logs/agent-a1.log" "${WORK_DIR}/logs/agent-a2.log"
    exit 1
}
echo "${TOKEN}" > "${WORK_DIR}/token"
echo "2 agents 在线（/api/agents=${COUNT}），token 已存 ${WORK_DIR}/token"
