#!/usr/bin/env bash
# 停止 run-server.sh 起的本地实例（不动 .acceptance 证据与测试 unit）
# a1 是 sudo 起的 root 进程：cui 直接 kill 会 EPERM，回退 sudo -n kill
set -euo pipefail
WORK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/.acceptance/services/instance"
for pidfile in "${WORK_DIR}"/agent-a*.pid "${WORK_DIR}/server.pid"; do
    [[ -f "${pidfile}" ]] || continue
    pid=$(cat "${pidfile}")
    if kill "${pid}" 2>/dev/null || sudo -n kill "${pid}" 2>/dev/null; then
        echo "stopped ${pidfile##*/} (${pid})"
    fi
    rm -f "${pidfile}"
done
