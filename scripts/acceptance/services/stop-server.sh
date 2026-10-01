#!/usr/bin/env bash
# 停止 run-server.sh 起的本地实例（不动 .acceptance 证据）
# root agent 进程属 root，kill 需 sudo 兜底
set -euo pipefail
WORK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/.acceptance/services/instance"
if [[ -f "${WORK_DIR}/agent-root.pid" ]]; then
    pid=$(cat "${WORK_DIR}/agent-root.pid")
    kill "${pid}" 2>/dev/null || sudo -n kill "${pid}" 2>/dev/null || true
    echo "stopped agent-root.pid (${pid})"
    rm -f "${WORK_DIR}/agent-root.pid"
fi
for pidfile in "${WORK_DIR}"/agent-noroot.pid "${WORK_DIR}/server.pid"; do
    [[ -f "${pidfile}" ]] || continue
    pid=$(cat "${pidfile}")
    kill "${pid}" 2>/dev/null && echo "stopped ${pidfile##*/} (${pid})" || true
    rm -f "${pidfile}"
done
