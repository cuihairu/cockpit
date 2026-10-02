#!/usr/bin/env bash
# 停止 run-server.sh 起的本地实例（不动 .acceptance 证据）
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WORK_DIR="${GUAC_WORK_DIR:-${REPO_ROOT}/.acceptance/guac/instance}"
for pidfile in "${WORK_DIR}/agent.pid" "${WORK_DIR}/server.pid"; do
    if [[ -f "${pidfile}" ]]; then
        pid=$(cat "${pidfile}")
        kill "${pid}" 2>/dev/null && echo "stopped ${pidfile##*/} (${pid})" || true
        rm -f "${pidfile}"
    fi
done
