#!/usr/bin/env bash
# 停止 run-server.sh 起的本地实例（不动 .acceptance 证据）
set -euo pipefail
WORK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/.acceptance/jobs/instance"
for pidfile in "${WORK_DIR}"/agent-a*.pid "${WORK_DIR}/server.pid"; do
    [[ -f "${pidfile}" ]] || continue
    pid=$(cat "${pidfile}")
    kill "${pid}" 2>/dev/null && echo "stopped ${pidfile##*/} (${pid})" || true
    rm -f "${pidfile}"
done
