#!/usr/bin/env bash
# 面板数据库备份验收：拆除环境（实例由 stop-server.sh 清理）。
# 无 unit/容器/定时夹具；仅兜底清理探针中断后可能残留的实例进程
# （按验收实例配置路径精确匹配，不碰其它验收域或用户进程）
# 证据目录 .acceptance/server-backup/evidence/ 保留，确认后手动清理
set -euo pipefail
for inst in a b; do
    if pkill -f "cockpit server -config .*/instance-${inst}/cockpit.yaml" >/dev/null 2>&1; then
        echo "killed stray instance-${inst}"
    fi
done
echo "done（.acceptance/server-backup 下的证据文件保留，确认后手动清理）"
