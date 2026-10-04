#!/usr/bin/env bash
# 通知渠道真机验收（acceptance-checklist「通知渠道」行：至少配置一个渠道，
# 用「测试通知」按钮核对送达）：环境搭建。
#
# 形态（全部本机）：server :19997（避开 guac 19990 / traefik 19991 /
# logs 19992 / services 19993 / jobs 19994 / server-backup 19995/19996）+
# webhook 接收器 127.0.0.1:9700/hook（scripts/acceptance/webhook_receiver.py，
# 校验 X-Cockpit-Secret）。另配一个指向死端口 9799 的 webhook 渠道作为
# 「投递失败逐渠道呈现」样本（无需杀进程即得失败面）。无 systemd、无
# 容器、无 sudo、无云凭据（ntfy/telegram 云端不验，行内注明）。
# 产物（.acceptance/notify/，已 gitignore）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
NT_DIR="${REPO_ROOT}/.acceptance/notify"

command -v go >/dev/null || { echo "ERROR: 需要 go 工具链" >&2; exit 1; }
command -v python3 >/dev/null || { echo "ERROR: 需要 python3（webhook 接收器）" >&2; exit 1; }
for port in 19997 9700 9799; do
    if ss -ltn "( sport = :${port} )" 2>/dev/null | grep -q "${port}"; then
        echo "ERROR: 端口 ${port} 已被占用（9700=上轮接收器未停先跑 stop-server.sh；" \
            "9799 须保持空闲=失败渠道样本）" >&2
        exit 1
    fi
done

mkdir -p "${NT_DIR}"/{logs,evidence}

{
    echo "date: $(date -Is)"
    echo "go: $(go version)"
    echo "python3: $(python3 --version 2>&1)"
    echo "ports 19997/9700/9799: free（9799 保持空闲=失败渠道样本）"
} > "${NT_DIR}/evidence/setup.log"

echo "== 验收环境就绪 =="
cat "${NT_DIR}/evidence/setup.log"
