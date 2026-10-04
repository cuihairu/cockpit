#!/usr/bin/env bash
# 面板数据库备份（server 侧）真机验收（server-backup-design.md，
# acceptance-checklist「面板数据库备份」节）：环境搭建。
#
# 形态（全部本机，端口 19995/19996，避开 guac 19990 / traefik 19991 /
# logs 19992 / services 19993 / jobs 19994）：备份链路只依赖 server 本体
# （VACUUM INTO + 目录扫描 + 定时循环），无 agent、无 systemd、无容器、
# 无 sudo。额外依赖 sqlite3（验收要求产物用 sqlite3 重新打开抽查表数据）。
# 定时路径证据须等真二进制的 1h tick（serverBackupTick=time.Hour 不可
# 注入）；双实例并行（retention=1 / retention=0）一次等待同证两行。
# 产物（.acceptance/server-backup/，已 gitignore）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SB_DIR="${REPO_ROOT}/.acceptance/server-backup"

command -v go >/dev/null || { echo "ERROR: 需要 go 工具链" >&2; exit 1; }
command -v sqlite3 >/dev/null || { echo "ERROR: 需要 sqlite3（产物完整性抽查）" >&2; exit 1; }
for port in 19995 19996; do
    if ss -ltn "( sport = :${port} )" 2>/dev/null | grep -q "${port}"; then
        echo "ERROR: 端口 ${port} 已被占用（上一轮实例未停？先跑 stop-server.sh）" >&2
        exit 1
    fi
done

mkdir -p "${SB_DIR}"/{logs,evidence}

{
    echo "date: $(date -Is)"
    echo "go: $(go version)"
    echo "sqlite3: $(sqlite3 --version | awk '{print $1}')"
    echo "ports 19995/19996: free"
} > "${SB_DIR}/evidence/setup.log"

echo "== 验收环境就绪 =="
cat "${SB_DIR}/evidence/setup.log"
