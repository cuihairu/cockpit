#!/usr/bin/env bash
# 日志验收：本地 cockpit 实例（scripts/e2e-smoke.sh 同路数，端口 19992）。
#   - server + 3 个 agent：
#       a1 全量 PATH（logs capability、systemd/docker 查询都成功）
#       a2 空 PATH（无 logs capability → 联邦检索 no-logs 归因样本）
#       a4 journalctl 失败 shim 遮蔽（有 capability、查询失败 → 降级样本）
#     a2/a4 的 capability 差异正是联邦检索筛选的输入面（D12 目标=在线且带
#     logs capability）；restricted PATH 可行的前提是 agent 启动期无无条件
#     exec（detector 全部 LookPath/文件守卫先行），已在实现侧核实
#   - COCKPIT_DRIFT_BASELINE 挪出 /var/lib/cockpit（本验收以普通用户跑）
# 产物（.acceptance/logs/instance/）：二进制/配置/db/logs/token/各 pid
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LG_DIR="${REPO_ROOT}/.acceptance/logs"
WORK_DIR="${LG_DIR}/instance"
PORT=19992
A1_ID=logs-acc-a1
A2_ID=logs-acc-a2
A4_ID=logs-acc-a4

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
  secret: logs-accept-jwt-secret
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

echo "== 启动 agent ×3 =="
# a1：全量 PATH（systemd + docker 真实查询）
COCKPIT_DRIFT_BASELINE="${LG_DIR}/drift-baseline-a1.json" \
"${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id "${A1_ID}" \
    > "${WORK_DIR}/logs/agent-a1.log" 2>&1 &
echo $! > "${WORK_DIR}/agent-a1.pid"

# a2：PATH 只含空目录 → LookPath 全失败 → 无 logs capability
# （agent 以绝对路径启动，PATH 只影响探测，不影响自身运行）
PATH="${LG_DIR}/fakebin-empty" \
COCKPIT_DRIFT_BASELINE="${LG_DIR}/drift-baseline-a2.json" \
"${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id "${A2_ID}" \
    > "${WORK_DIR}/logs/agent-a2.log" 2>&1 &
echo $! > "${WORK_DIR}/agent-a2.pid"

# a4：journalctl 被 shim 遮蔽（其余命令正常）→ 有 logs capability、
# systemd 查询失败 → 联邦检索单机降级样本
PATH="${LG_DIR}/fakebin-journalctl-fail:${PATH}" \
COCKPIT_DRIFT_BASELINE="${LG_DIR}/drift-baseline-a4.json" \
"${WORK_DIR}/bin/cockpit-agent" start -server "ws://127.0.0.1:${PORT}/ws" -id "${A4_ID}" \
    > "${WORK_DIR}/logs/agent-a4.log" 2>&1 &
echo $! > "${WORK_DIR}/agent-a4.pid"

# 等 3 个 agent 全部注册 + 登录拿 token
TOKEN=""
for i in $(seq 1 60); do
    TOKEN=$(curl -sf -X POST "http://127.0.0.1:${PORT}/api/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
        | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' 2>/dev/null) || TOKEN=""
    if [[ -n "${TOKEN}" ]]; then
        COUNT=$(curl -sf "http://127.0.0.1:${PORT}/api/agents" -H "Authorization: Bearer ${TOKEN}" \
            | python3 -c 'import json,sys; d=json.load(sys.stdin); print(len(d.get("agents", d)) if isinstance(d, dict) else len(d))' 2>/dev/null) || COUNT=0
        [[ "${COUNT}" == "3" ]] && break
    fi
    sleep 1
done
[[ -n "${TOKEN}" ]] || { echo "登录失败"; tail -20 "${WORK_DIR}/logs/server.log"; exit 1; }
[[ "${COUNT}" == "3" ]] || {
    echo "agent 注册不全（${COUNT}/3）——检查各 agent 日志："
    tail -5 "${WORK_DIR}/logs/agent-a1.log" "${WORK_DIR}/logs/agent-a2.log" "${WORK_DIR}/logs/agent-a4.log"
    exit 1
}
echo "${TOKEN}" > "${WORK_DIR}/token"
echo "3 agents 在线（/api/agents=${COUNT}），token 已存 ${WORK_DIR}/token"
