#!/usr/bin/env bash
# 远控三协议验收：拆除目标容器（cockpit 实例由 run-server.sh 自行清理）
set -euo pipefail
for name in cockpit-acc-sshd cockpit-acc-vnc cockpit-acc-rdp cockpit-acc-guacd cockpit-acc-chrome; do
    docker rm -f "${name}" >/dev/null 2>&1 && echo "removed ${name}" || true
done
echo "done（.acceptance/guac 下的证据文件保留，确认后手动清理）"
