#!/usr/bin/env bash
# Traefik 后端热加载验收：拆除环境（cockpit 实例由 stop-server.sh 清理）
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TF_DIR="${REPO_ROOT}/.acceptance/traefik"

# Traefik 日志终卷入证据（探针运行期间之后的部分）
if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -q '^cockpit-acc-traefik$'; then
    docker logs cockpit-acc-traefik > "${TF_DIR}/evidence/traefik-teardown.log" 2>&1 || true
fi
docker rm -f cockpit-acc-traefik >/dev/null 2>&1 && echo "removed cockpit-acc-traefik" || true

# 宿主上游后端
for name in backend-a backend-b; do
    if [[ -f "${TF_DIR}/${name}.pid" ]]; then
        pid=$(cat "${TF_DIR}/${name}.pid")
        kill "${pid}" 2>/dev/null && echo "stopped ${name} (${pid})" || true
        rm -f "${TF_DIR}/${name}.pid"
    fi
done
echo "done（.acceptance/traefik 下的证据文件保留，确认后手动清理）"
