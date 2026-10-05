#!/usr/bin/env bash
# 停机补跑场景专用：只重启 server（保留 DB/agent/webhook 接收器与全部产物）。
# 用法：restart-server.sh [down-secs]
#   down-secs  kill 后到重新拉起前的停机时长（默认 0）。补跑场景传 ~110，
#              让配置的 daily@ 到点时刻落在停机窗口内。
# server 回来后 agent 经既有重连循环自动再注册；JWT secret 不变，探针 token 仍有效。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK_DIR="${REPO_ROOT}/.acceptance/agent-backup/instance"
PORT=19998
DOWN_SECS="${1:-0}"

ADMIN_USER="${ADMIN_USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASSWORD:-e2e-strong-pass-1}"

OLD_PID="$(cat "${WORK_DIR}/server.pid")"
kill "${OLD_PID}"
echo "server ${OLD_PID} stopped，停机 ${DOWN_SECS}s"
sleep "${DOWN_SECS}"

ADMIN_USERNAME="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASS}" \
    "${WORK_DIR}/bin/cockpit" server -config "${WORK_DIR}/cockpit.yaml" \
    >> "${WORK_DIR}/logs/server.log" 2>&1 &
NEW_PID=$!
echo "${NEW_PID}" > "${WORK_DIR}/server.pid"

for i in $(seq 1 30); do
    curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1 && break
    if ! kill -0 "${NEW_PID}" 2>/dev/null; then
        echo "server 重启失败："; tail -n 30 "${WORK_DIR}/logs/server.log"; exit 1
    fi
    sleep 1
done
curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null || { echo "health 超时"; exit 1; }
echo "server 重启完成 (pid=${NEW_PID})"
