#!/usr/bin/env bash
# Agent 文件备份真机验收（backup-design.md，acceptance-checklist「备份与恢复
# （agent 侧）」节）：环境搭建。
#
# 形态（全部本机，端口 19998，避开 guac 19990 / traefik 19991 / logs 19992 /
# services 19993 / jobs 19994 / server-backup 19995-19996 / notify 19997）：
# 备份链路只依赖 server + 在线 agent + agent 侧文件系统——无 systemd unit、
# 无容器、无 sudo。webhook 接收器占 9701（9700 归 notify）。
# 产物（.acceptance/agent-backup/，已 gitignore）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
AB_DIR="${REPO_ROOT}/.acceptance/agent-backup"

command -v go >/dev/null || { echo "ERROR: 需要 go 工具链" >&2; exit 1; }
for port in 19998 9701; do
    if ss -ltn "( sport = :${port} )" 2>/dev/null | grep -q "${port}"; then
        echo "ERROR: 端口 ${port} 已被占用（上一轮实例未停？先跑 stop-server.sh）" >&2
        exit 1
    fi
done

mkdir -p "${AB_DIR}"/{logs,evidence} "${AB_DIR}"/instance/{data,bin,logs}

command -v tar >/dev/null || { echo "ERROR: 需要 tar（证据留档复核）" >&2; exit 1; }
command -v python3 >/dev/null || { echo "ERROR: 需要 python3（webhook 接收器）" >&2; exit 1; }

{
    echo "date: $(date -Is)"
    echo "go: $(go version)"
    echo "tar: $(command -v tar)"
    echo "python3: $(command -v python3)"
    echo "ports 19998/9701: free"
} > "${AB_DIR}/evidence/setup.log"

echo "== 验收环境就绪 =="
cat "${AB_DIR}/evidence/setup.log"
