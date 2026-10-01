#!/usr/bin/env bash
# systemd 服务管理真机验收（service-design.md M1 真机项，
# acceptance-checklist「服务管理」节 systemd 首项部分）：环境搭建。
#
# 形态（全部本机，端口 19993，避开 guac 19990 / traefik 19991 / logs 19992）：
#   - 测试 unit：/etc/systemd/system/cockpit-acc-svc.service（/bin/sleep infinity，
#     本机实验结论：list-units --all（213 行）与 list-unit-files（299 行）天然
#     有差集——模板/autovt 类与 apt-news/bolt 类为 files-only 未加载项，
#     mergeServiceUnits 保留正是 T1 要验的语义；自建测试 unit 只作 restart/
#     enable 动词样本，绝不动用户业务 unit）
#   - 双 agent（run-server.sh 起）：root（sudo -n 起，动词成功样本）+
#     cui 非 root（动词 502 报错透传样本；读操作正常，证明是纯权限失败）
#   - 非 root systemctl restart 真实报错（本机实测）："Access denied as the
#     requested operation requires interactive authentication..."——provider
#     包成 "systemctl restart <unit>: <首行>"，server 502 原样透传
#
# 前置：systemctl + /run/systemd/system（DetectSystemd 同款）+ 免密 sudo
# （建测试 unit 文件/daemon-reload/root agent 都要）。产物（.acceptance/services/，
# 已 gitignore——沿用 logs 验收惯例）。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SV_DIR="${REPO_ROOT}/.acceptance/services"
UNIT="cockpit-acc-svc.service"
UNIT_PATH="/etc/systemd/system/${UNIT}"

command -v systemctl >/dev/null || { echo "ERROR: 需要 systemctl" >&2; exit 1; }
[[ -d /run/systemd/system ]] || { echo "ERROR: 非 systemd 主机（/run/systemd/system 缺席）" >&2; exit 1; }
if ! sudo -n true 2>/dev/null; then
    echo "ERROR: 需要免密 sudo（写测试 unit 文件/daemon-reload/root agent）" >&2
    exit 1
fi

mkdir -p "${SV_DIR}"/{evidence,instance/{data,bin,logs}}

# 测试 unit 文件（只建自己名下；可复跑：已存在则覆盖写回同一内容）
sudo -n tee "${UNIT_PATH}" >/dev/null <<'EOF'
[Unit]
Description=Cockpit acceptance test service (safe to remove)
After=network.target

[Service]
Type=simple
ExecStart=/bin/sleep infinity

[Install]
WantedBy=multi-user.target
EOF
sudo -n systemctl daemon-reload
# 归一初始态：停用 + 不自启（复跑时还原； fresh unit 本就如此，忽略报错）
sudo -n systemctl disable --now "${UNIT}" >/dev/null 2>&1 || true

# 归因口径快照：两表行数 + files-only 差集样本（T1「列表含未加载 unit」的输入面）
UNITS_N=$(systemctl list-units --type=service --all --no-legend --no-pager | wc -l)
FILES_N=$(systemctl list-unit-files --type=service --no-legend --no-pager | wc -l)
systemctl list-unit-files --type=service --no-legend --no-pager | awk '$1~/\.service$/ {print $1}' | sort -u > /tmp/svc-acc-files.txt
systemctl list-units --type=service --all --no-legend --no-pager | awk '$1~/\.service$/ {print $1}' | sort -u > /tmp/svc-acc-units.txt
comm -23 /tmp/svc-acc-files.txt /tmp/svc-acc-units.txt | head -5 > /tmp/svc-acc-notloaded.txt || true
rm -f /tmp/svc-acc-files.txt /tmp/svc-acc-units.txt

{
    echo "date: $(date -Is)"
    echo "sudo passwordless: OK"
    echo "systemd: $(systemctl --version | head -1)"
    echo "list-units: ${UNITS_N} 行 / list-unit-files: ${FILES_N} 行"
    echo "files-only 未加载样本:"
    sed 's/^/  /' /tmp/svc-acc-notloaded.txt
    echo "test unit: ${UNIT_PATH}"
    echo "test unit file-state: $(systemctl is-enabled ${UNIT} 2>&1 || true)"
    echo "test unit active-state: $(systemctl is-active ${UNIT} 2>&1 || true)"
} > "${SV_DIR}/evidence/setup.log"
rm -f /tmp/svc-acc-notloaded.txt

echo "== 验收环境就绪 =="
cat "${SV_DIR}/evidence/setup.log"
