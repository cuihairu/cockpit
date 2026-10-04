#!/usr/bin/env bash
# 执行 Job 验收：拆除环境（cockpit 实例由 stop-server.sh 清理）。
# 无 unit/容器/临时夹具；仅兜底清理探针超时场景可能的残留 sleep
# （正常路径 agent 已按进程组 SIGKILL，此处是探针异常中断时的保险）
# 证据目录 .acceptance/jobs/evidence/ 保留，确认后手动清理
set -euo pipefail

if pgrep -f "sleep 297" >/dev/null 2>&1; then
    pkill -f "sleep 297" && echo "killed stray sleep 297" || true
fi
pkill -f "cockpit-agent start -server ws://127.0.0.1:19994" >/dev/null 2>&1 \
    && echo "killed stray agent" || true

echo "done（.acceptance/jobs 下的证据文件保留，确认后手动清理）"
