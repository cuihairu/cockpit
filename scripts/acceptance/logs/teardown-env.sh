#!/usr/bin/env bash
# 日志验收：拆除环境（cockpit 实例由 stop-server.sh 清理）
# 证据目录 .acceptance/logs/evidence/ 保留，确认后手动清理
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LG_DIR="${REPO_ROOT}/.acceptance/logs"

# T2 循环日志容器（容器日志终卷入证据后删除）
if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -q '^cockpit-acc-log$'; then
    docker logs cockpit-acc-log > "${LG_DIR}/evidence/container-teardown.log" 2>&1 || true
    docker rm -f cockpit-acc-log >/dev/null 2>&1 && echo "removed cockpit-acc-log" || true
fi

# T1/T3 系统域 transient unit（--collect 在退出后自清；探针中断残留时在此兜底）
sudo -n systemctl stop cockpit-acc-hf.service cockpit-acc-idle.service >/dev/null 2>&1 \
    && echo "stopped transient units" || true

# 残留 journalctl -f / docker logs -f 跟随进程（探针异常中断时的兜底）
pgrep -f "journalctl -f -n .*cockpit-acc" >/dev/null 2>&1 \
    && { pkill -f "journalctl -f -n .*cockpit-acc"; echo "killed stray journalctl -f"; } || true

echo "done（.acceptance/logs 下的证据文件保留，确认后手动清理）"
