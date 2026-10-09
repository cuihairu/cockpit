#!/usr/bin/env bash
# Workflow 编排验收：拆除环境（cockpit 实例由 stop-server.sh 清理）。
# 无 unit/容器/临时夹具；仅兜底清理探针取消场景可能的残留 sleep
# （正常路径在途 sleep 步骤会自然结束，此处是探针异常中断时的保险）
# 证据目录 .acceptance/workflows/evidence/ 保留，确认后手动清理
set -euo pipefail

if pgrep -f "sleep 12" >/dev/null 2>&1; then
    pkill -f "sleep 12" && echo "killed stray sleep 12" || true
fi
pkill -f "cockpit-agent start -server ws://127.0.0.1:20010" >/dev/null 2>&1 \
    && echo "killed stray agent" || true

echo "done（.acceptance/workflows 下的证据文件保留，确认后手动清理）"
