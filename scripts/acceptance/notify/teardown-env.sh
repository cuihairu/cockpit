#!/usr/bin/env bash
# 通知渠道验收：拆除环境（实例与接收器由 stop-server.sh 清理）。
# 仅兜底清理探针中断后可能残留的进程（按验收实例专属路径/端口精确匹配，
# 不碰其它验收域或用户进程）
# 证据目录 .acceptance/notify/evidence/ 保留，确认后手动清理
set -euo pipefail
if pkill -f "cockpit server -config .*/notify/instance/cockpit.yaml" >/dev/null 2>&1; then
    echo "killed stray notify server"
fi
if pkill -f "webhook_receiver.py notify-accept-secret" >/dev/null 2>&1; then
    echo "killed stray webhook receiver"
fi
echo "done（.acceptance/notify 下的证据文件保留，确认后手动清理）"
