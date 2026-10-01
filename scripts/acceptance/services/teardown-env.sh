#!/usr/bin/env bash
# 服务管理验收：拆除环境（cockpit 实例由 stop-server.sh 清理）
# 测试 unit 彻底移除：unmask 残链 → disable --now → 双位 unit 文件删除 →
# daemon-reload；root agent 产物（drift-baseline-a1.json）sudo 清
# 证据目录 .acceptance/services/evidence/ 保留，确认后手动清理
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SV_DIR="${REPO_ROOT}/.acceptance/services"
UNIT=cockpit-acc-svc.service
GHOST=cockpit-acc-ghost.service
JLOG=cockpit-acc-jlog.service

# 测试 unit（mask 可能残链 / enable symlink / 两处 unit 文件 / PUT 产生的
# /etc 覆盖位副本 / jlog 的双位与 wants 链）
sudo -n systemctl unmask "${UNIT}" >/dev/null 2>&1 || true
sudo -n systemctl disable --now "${UNIT}" >/dev/null 2>&1 || true
sudo -n systemctl stop "${UNIT}" "${JLOG}" >/dev/null 2>&1 || true
sudo -n rm -f "/usr/lib/systemd/system/${UNIT}" "/etc/systemd/system/${UNIT}" \
    "/etc/systemd/system/${GHOST}" \
    "/usr/lib/systemd/system/${JLOG}" "/etc/systemd/system/${JLOG}" \
    "/etc/systemd/system/multi-user.target.wants/${UNIT}" \
    "/etc/systemd/system/multi-user.target.wants/${JLOG}"
sudo -n systemctl daemon-reload

# root agent 产物
sudo -n rm -f "${SV_DIR}/drift-baseline-a1.json"

# 残留验收实例进程（pid 文件已失效时的兜底；root agent 需 sudo pkill）
pkill -f "cockpit-agent start -server ws://127.0.0.1:19993" >/dev/null 2>&1 \
    && echo "killed stray agent" || true
sudo -n pkill -f "cockpit-agent start -server ws://127.0.0.1:19993" >/dev/null 2>&1 \
    && echo "killed stray root agent" || true

echo "done（.acceptance/services 下的证据文件保留，确认后手动清理）"
