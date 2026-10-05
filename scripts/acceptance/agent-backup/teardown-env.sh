#!/usr/bin/env bash
# Agent 文件备份验收：拆除环境（实例由 stop-server.sh 清理）。
# 无 unit/容器；夹具大文件（256MB 随机样本）在 .acceptance/agent-backup/work，
# 连同实例一并保留证据后由人工清理。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

# 兜底：探针异常中断可能留下 server/agent 接收器进程（按本轮配置路径精确匹配）
pkill -f "cockpit server -config ${REPO_ROOT}/.acceptance/agent-backup/instance/cockpit.yaml" >/dev/null 2>&1 \
    && echo "killed stray server" || true
pkill -f "cockpit-agent start -server ws://127.0.0.1:19998" >/dev/null 2>&1 \
    && echo "killed stray agent" || true

echo "done（.acceptance/agent-backup 下的证据保留，确认后手动清理）"
