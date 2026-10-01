#!/usr/bin/env bash
# systemd 服务管理验收：本地 cockpit 实例（scripts/acceptance/logs/run-server.sh
# 同路数，端口 19993）。
#   - server + 2 个 agent（同机双身份，topo 即输入面）：
#       root    全量环境 sudo -n 起（systemctl 动词成功样本）
#       noroot  cui 本人起（读正常 + 动词 502 报错透传样本）
#     两者 DetectSystemd 同为真（systemctl + /run/systemd/system 与 uid 无关），
#     capability 都是 service/backend=systemd——权限差异只来自 uid 本身
#   - COCKPIT_DRIFT_BASELINE 挪出 /var/lib/cockpit（普通用户/root 双写都无权限问题）
# 产物（.acceptance/services/instance/）：二进制/配置/db/logs/token/各 pid
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SV_DIR="${REPO_ROOT}/.acceptance/services"
WORK_DIR="${SV_DIR}/instance"
PORT=19993
ROOT_ID=svc-acc-root
USER_ID=svc-acc-noroot

ADMIN_USER="${ADMIN_USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASSWORD:-e2e-strong-pass-1}"

mkdir -p "${WORK_DIR}"/{data,bin,logs}

cat > "${WORK_DIR}/cockpit.yaml" <<EOF
server:
  host: 127.0.0.1
  port: ${PORT}
database:
  path: ${WORK_DIR}/data/cockpit.db
jwt:
  secret: svc-accept-jwt-secret
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
        echo "server 启动失败："; tail -30 "${WORK_DIR}/logs/server.log"; exit 1
    fi
    sleep 1
done
curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null || { echo "health 超时"; exit 1; }
echo "server ready (pid=${SERVER_PID}, :${PORT})"

echo "== 启动 agent ×2（root + noroot） =="
# root：sudo -n 起，动词成功样本（sudo 行首 VAR= 赋值透环境）
sudo -n COCKPIT_DRIFT_BASELINE="${SV_DIR}/drift-baseline-root.json" \
"${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id "${ROOT_ID}" \
    > "${WORK_DIR}/logs/agent-root.log" 2>&1 &
echo $! > "${WORK_DIR}/agent-root.pid"

# noroot：cui 本人起，报错透传样本
COCKPIT_DRIFT_BASELINE="${SV_DIR}/drift-baseline-noroot.json" \
"${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id "${USER_ID}" \
    > "${WORK_DIR}/logs/agent-noroot.log" 2>&1 &
echo $! > "${WORK_DIR}/agent-noroot.pid"

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
[[ -n "${TOKEN}" ]] || { echo "登录失败"; tail -20 "${WORK_DIR}/logs/server.log"; exit 1; }
[[ "${COUNT}" == "2" ]] || {
    echo "agent 注册不全（${COUNT}/2）——检查各 agent 日志："
    tail -5 "${WORK_DIR}/logs/agent-root.log" "${WORK_DIR}/logs/agent-noroot.log"
    exit 1
}
echo "${TOKEN}" > "${WORK_DIR}/token"
echo "2 agents 在线（/api/agents=${COUNT}），token 已存 ${WORK_DIR}/token"
