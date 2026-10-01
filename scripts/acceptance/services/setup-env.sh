#!/usr/bin/env bash
# 服务管理真实环境验收（acceptance-checklist「服务管理（三后端）」systemd 行、
# todo.md systemd 服务管理条目剩余项）：环境搭建。
#
# 形态（全部本机，端口 19993，避开 guac 19990 / traefik 19991 / logs 19992）：
#   - 测试 unit ×2（绝不碰用户业务 unit）：
#       cockpit-acc-svc.service    装在 /usr/lib/systemd/system——mask 只能
#                                  盖 /etc 位置的同名 symlink，装 /etc 实体
#                                  文件会被 systemd 拒（File already exists，
#                                  实测）；enable symlink 仍在 /etc wants/
#       cockpit-acc-ghost.service  装 /etc 但永不 enable/start（daemon-reload
#                                  后无引用不 load）→ 「未加载安装项」样本
#   - 双 agent 对照（run-server.sh 起）：
#       a1 svc-acc-a1  root（sudo -n 起）→ restart/enable/mask 成功组
#       a2 svc-acc-a2  非 root（cui）   → polkit 拒（Access denied ...）→
#                                         报错透传组（实测：带会话/脱会话均拒）
# 前置：systemd 主机 + 免密 sudo。产物（.acceptance/services/，已 gitignore）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SV_DIR="${REPO_ROOT}/.acceptance/services"
UNIT=cockpit-acc-svc.service
GHOST=cockpit-acc-ghost.service
# R3 journal 样本：装 /usr/lib 永不 enable——探针先 PUT 换带 marker 的
# ExecStart（dogfood 覆盖位 + 捆绑 daemon-reload），start 产 journal 历史，
# stop + daemon-reload 后成「未加载安装项」，日志跳转按 unit 查历史
JLOG=cockpit-acc-jlog.service

command -v systemctl >/dev/null || { echo "ERROR: 需要 systemctl" >&2; exit 1; }
[ -d /run/systemd/system ] || { echo "ERROR: 非 systemd 主机（/run/systemd/system 不存在）" >&2; exit 1; }
if ! sudo -n true 2>/dev/null; then
    echo "ERROR: 需要免密 sudo（安装测试 unit 与 root agent）" >&2; exit 1
fi

mkdir -p "${SV_DIR}"/{logs,evidence} "${SV_DIR}"/instance/{data,bin,logs}

# 清残留（可复跑）：先卸可能的 mask 残链，再删双位 unit 文件、拆 wants 链
sudo -n systemctl unmask "${UNIT}" >/dev/null 2>&1 || true
sudo -n systemctl disable --now "${UNIT}" >/dev/null 2>&1 || true
sudo -n systemctl stop "${UNIT}" >/dev/null 2>&1 || true
sudo -n systemctl stop "${JLOG}" >/dev/null 2>&1 || true
sudo -n rm -f "/etc/systemd/system/${UNIT}" "/etc/systemd/system/${GHOST}" \
    "/usr/lib/systemd/system/${UNIT}" \
    "/etc/systemd/system/${JLOG}" "/usr/lib/systemd/system/${JLOG}" \
    "/etc/systemd/system/multi-user.target.wants/${UNIT}" \
    "/etc/systemd/system/multi-user.target.wants/${JLOG}"
sudo -n systemctl daemon-reload

# 未加载样本：装文件不 enable 不 start（reload 后无引用 → list-units 不含）
sudo -n tee "/etc/systemd/system/${GHOST}" >/dev/null <<'EOF'
[Unit]
Description=cockpit acceptance ghost unit (installed, never enabled, never started)
[Service]
Type=simple
ExecStart=/bin/true
[Install]
WantedBy=multi-user.target
EOF

# 操作面样本：装 /usr/lib（mask/unmask 链路需要），enable --now 立基准
# （active + enabled），后续全部动词由探针经 REST 驱动
cat > /tmp/cockpit-acc-svc.service <<'EOF'
[Unit]
Description=cockpit acceptance test unit (throwaway)
[Service]
Type=simple
ExecStart=/bin/sleep infinity
[Install]
WantedBy=multi-user.target
EOF
sudo -n cp /tmp/cockpit-acc-svc.service "/usr/lib/systemd/system/${UNIT}"

# journal 样本：装 /usr/lib（PUT 覆盖位链路需要）永不 enable/start——基线
# ExecStart=/bin/true 无 marker，探针 PUT 换带 marker 版后 start 才有输出
cat > /tmp/cockpit-acc-jlog.service <<'EOF'
[Unit]
Description=cockpit acceptance journal sample (throwaway)
[Service]
Type=oneshot
ExecStart=/bin/true
[Install]
WantedBy=multi-user.target
EOF
sudo -n cp /tmp/cockpit-acc-jlog.service "/usr/lib/systemd/system/${JLOG}"
sudo -n systemctl daemon-reload
sudo -n systemctl enable --now "${UNIT}"

# 基准与 ghost 语义落证据（真机对照口径：运行表 0 / 安装表 disabled）
{
    echo "date: $(date -Is)"
    echo "sudo passwordless: OK"
    echo "systemd: $(systemctl --version | head -1)"
    echo "svc baseline: active=$(systemctl is-active "${UNIT}") enabled=$(systemctl is-enabled "${UNIT}")"
    echo "ghost in list-units (want 0): $(systemctl list-units --type=service --no-legend --no-pager | grep -c "${GHOST}" || true)"
    echo "ghost in list-unit-files: $(systemctl list-unit-files --type=service --no-legend --no-pager | grep "${GHOST}" || echo MISSING)"
    echo "jlog baseline: active=$(systemctl is-active "${JLOG}") enabled=$(systemctl is-enabled "${JLOG}") path=$(systemctl show -p FragmentPath --value "${JLOG}")"
    echo "unit path: $(systemctl show -p FragmentPath --value "${UNIT}")"
} > "${SV_DIR}/evidence/setup.log"

echo "== 验收环境就绪 =="
cat "${SV_DIR}/evidence/setup.log"
