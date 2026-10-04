#!/usr/bin/env bash
# 停止 run-server.sh 起的双实例（不动 .acceptance 证据）
set -euo pipefail
SB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/.acceptance/server-backup"
for inst in a b; do
    pidfile="${SB_DIR}/instance-${inst}/server.pid"
    [[ -f "${pidfile}" ]] || continue
    pid=$(cat "${pidfile}")
    kill "${pid}" 2>/dev/null && echo "stopped instance-${inst} (${pid})" || true
    rm -f "${pidfile}"
done
