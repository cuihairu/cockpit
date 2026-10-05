#!/usr/bin/env bash
# 停止 run-server.sh 起的本地实例（不动 .acceptance 证据）
set -euo pipefail
AB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/.acceptance/agent-backup"
WORK_DIR="${AB_DIR}/instance"
for pidfile in "${WORK_DIR}/agent-a1.pid" "${WORK_DIR}/server.pid" "${WORK_DIR}/receiver.pid"; do
    [[ -f "${pidfile}" ]] || continue
    pid=$(cat "${pidfile}")
    kill "${pid}" 2>/dev/null && echo "stopped ${pidfile##*/} (${pid})" || true
    rm -f "${pidfile}"
done
