#!/usr/bin/env bash
# 执行 Job 真机验收（jobs-design.md，acceptance-checklist「统一 Job 执行」节）：
# 环境搭建。
#
# 形态（全部本机，端口 19994，避开 guac 19990 / traefik 19991 / logs 19992 /
# services 19993）：Job 链路只依赖 server + 在线 agent + agent 侧 sh——
# 无 systemd unit、无容器、无 sudo。环境核验仅构建工具与端口占用。
# 产物（.acceptance/jobs/，已 gitignore）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
JB_DIR="${REPO_ROOT}/.acceptance/jobs"

command -v go >/dev/null || { echo "ERROR: 需要 go 工具链" >&2; exit 1; }
if ss -ltn "( sport = :19994 )" 2>/dev/null | grep -q 19994; then
    echo "ERROR: 端口 19994 已被占用（上一轮实例未停？先跑 stop-server.sh）" >&2
    exit 1
fi

mkdir -p "${JB_DIR}"/{logs,evidence} "${JB_DIR}"/instance/{data,bin,logs}

command -v sh >/dev/null || { echo "ERROR: 需要 sh（agent.exec 经 sh -c 执行）" >&2; exit 1; }
command -v pgrep >/dev/null || { echo "ERROR: 需要 pgrep（超时孤儿进程复核）" >&2; exit 1; }

{
    echo "date: $(date -Is)"
    echo "go: $(go version)"
    echo "sh: $(command -v sh)"
    echo "pgrep: $(command -v pgrep)"
    echo "port 19994: free"
} > "${JB_DIR}/evidence/setup.log"

echo "== 验收环境就绪 =="
cat "${JB_DIR}/evidence/setup.log"
