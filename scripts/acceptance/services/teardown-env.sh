#!/usr/bin/env bash
# systemd 服务管理验收：拆除环境（cockpit 实例由 stop-server.sh 清理）
# 测试 unit 停用→停→删文件→daemon-reload，还原系统原状；
# 证据目录 .acceptance/services/evidence/ 保留，确认后手动清理
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SV_DIR="${REPO_ROOT}/.acceptance/services"
UNIT="cockpit-acc-svc.service"

# 测试 unit 收尾（只动自己名下；每步都可失败——探针 T4 已做 stop/disable 断言，
# 这里是中断/复跑兜底）
sudo -n systemctl disable --now "${UNIT}" >/dev/null 2>&1 \
    && echo "disabled+stopped ${UNIT}" || true
if [[ -f "/etc/systemd/system/${UNIT}" ]]; then
    sudo -n rm -f "/etc/systemd/system/${UNIT}" && echo "removed ${UNIT}" || true
    sudo -n systemctl daemon-reload && echo "daemon-reloaded" || true
fi

echo "done（.acceptance/services 下的证据文件保留，确认后手动清理）"
