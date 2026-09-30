#!/usr/bin/env bash
# 日志尾随与跨机联邦检索真机验收（logs-design.md M2/M3 真机项，
# acceptance-checklist「日志检索」节）：环境搭建。
#
# 形态（全部本机，端口 19992，避开 guac 19990 / traefik 19991）：
#   - journalctl 高频源/空闲源：系统域 transient unit（sudo -n systemd-run）
#     ——本机实验结论：journalctl -u 不匹配用户域瞬态单元（记录只带
#     _SYSTEMD_USER_UNIT），系统域才能被 -u 命中；-f 对已结束的 unit 常驻
#     不退（超时兜底是唯一出口，正是 L143 要验的语义）
#   - docker 源：alpine 循环输出容器（docker stop → docker logs -f 退出）
#   - 联邦检索拓扑（run-server.sh 起 3 agent）：
#       a1 全量 PATH   → logs capability，查询成功（结果样本）
#       a2 空 PATH     → 无 logs capability（skipped: no-logs 样本）
#       a4 journalctl 被失败 shim 遮蔽 → 有 capability 但查询失败
#                       （单 agent 失败降级 ok=false 样本）
#     ghost id → skipped: offline 样本（缺席即离线，不区分 not-found）
#
# 前置：免密 sudo（系统域 systemd-run）+ journalctl + docker（daemon 在线，
# 无 compose CLI 亦可）+ alpine 镜像。产物（.acceptance/logs/，已 gitignore）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
LG_DIR="${REPO_ROOT}/.acceptance/logs"

command -v journalctl >/dev/null || { echo "ERROR: 需要 journalctl" >&2; exit 1; }
command -v systemd-run >/dev/null || { echo "ERROR: 需要 systemd-run" >&2; exit 1; }
command -v docker >/dev/null || { echo "ERROR: 需要 docker CLI" >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo "ERROR: docker daemon 不在线" >&2; exit 1; }
# 免密 sudo：T1 高频源与 T3 空闲源都要在系统域建 transient unit
# （非 sudo 的 systemd-run 被 polkit 拒：interactive authentication）
if ! sudo -n true 2>/dev/null; then
    echo "ERROR: 需要免密 sudo（systemd-run 系统域建 transient unit）" >&2
    exit 1
fi

mkdir -p "${LG_DIR}"/{logs,evidence} "${LG_DIR}"/instance/{data,bin,logs} \
    "${LG_DIR}/fakebin-empty" "${LG_DIR}/fakebin-journalctl-fail"

# a2 夹具：空 PATH——DetectLogs 是纯 LookPath（探测不执行命令），两类日志
# 二进制都找不到 → 无 logs capability（.keep 不可执行，不构成探测命中）
: > "${LG_DIR}/fakebin-empty/.keep"

# a4 夹具：journalctl 失败 shim——LookPath 命中（capability 成立）、
# 实际执行必失败（exit 1 无输出）→ 联邦检索的「单机失败降级」样本
cat > "${LG_DIR}/fakebin-journalctl-fail/journalctl" <<'EOF'
#!/bin/sh
# 验收夹具：遮蔽真 journalctl——探测（LookPath）通过、查询（exec）失败，
# 制造「带 logs capability 但单机查询失败」的降级样本（L144）
echo "journalctl: acceptance injected failure (shim)" >&2
exit 1
EOF
chmod +x "${LG_DIR}/fakebin-journalctl-fail/journalctl"

# T2 循环日志容器镜像
docker image inspect alpine:latest >/dev/null 2>&1 || docker pull -q alpine:latest

{
    echo "date: $(date -Is)"
    echo "sudo passwordless: OK"
    echo "systemd: $(systemctl --version | head -1)"
    echo "journalctl: $(command -v journalctl)"
    echo "docker: $(docker --version)"
    echo "alpine image: $(docker image inspect --format '{{.Id}}' alpine:latest)"
    echo "fixtures: fakebin-empty(PATH 全空) fakebin-journalctl-fail(shim 遮蔽)"
} > "${LG_DIR}/evidence/setup.log"

# 残留同名容器/单元先清（可复跑）
docker rm -f cockpit-acc-log >/dev/null 2>&1 || true
sudo -n systemctl stop cockpit-acc-hf.service cockpit-acc-idle.service >/dev/null 2>&1 || true

echo "== 验收环境就绪 =="
cat "${LG_DIR}/evidence/setup.log"
