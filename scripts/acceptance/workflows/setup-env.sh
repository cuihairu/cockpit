#!/usr/bin/env bash
# Workflow 编排真机验收（workflow-design.md M1，acceptance-checklist「Workflow
# 编排」节）：环境搭建。
#
# 形态（全部本机，端口 20010，避开既有各套 1999x/2000x 口径）：Workflow 链路
# 只依赖 server + 在线 agent + agent 侧 sh——无 systemd unit、无容器、无 sudo。
# 环境核验仅构建工具与端口占用。
# 产物（.acceptance/workflows/，已 gitignore）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WF_DIR="${REPO_ROOT}/.acceptance/workflows"

command -v go >/dev/null || { echo "ERROR: 需要 go 工具链" >&2; exit 1; }
if ss -ltn "( sport = :20010 )" 2>/dev/null | grep -q 20010; then
    echo "ERROR: 端口 20010 已被占用（上一轮实例未停？先跑 stop-server.sh）" >&2
    exit 1
fi

mkdir -p "${WF_DIR}"/{logs,evidence} "${WF_DIR}"/instance/{data,bin,logs}

command -v sh >/dev/null || { echo "ERROR: 需要 sh（agent.exec 经 sh -c 执行）" >&2; exit 1; }

{
    echo "date: $(date -Is)"
    echo "go: $(go version)"
    echo "sh: $(command -v sh)"
    echo "port 20010: free"
} > "${WF_DIR}/evidence/setup.log"

echo "== 验收环境就绪 =="
cat "${WF_DIR}/evidence/setup.log"
