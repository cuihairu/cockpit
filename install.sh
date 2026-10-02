#!/usr/bin/env bash
# Cockpit Agent 一键安装（Linux / macOS）
#
# 从每日构建（nightly release，匿名可直链下载）拉取与本机 OS/架构匹配的
# cockpit-agent 并安装，可选注册开机自启服务（systemd / launchd）。
#
# 用法:
#   # Linux / macOS 一键（自动检测 OS 与架构）:
#   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
#
#   # 带参数（注册开机自启服务）:
#   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | \
#     bash -s -- --with-service --server wss://cockpit.cuihairu.site/ws
#
#   # 静默安装（零交互；不带 --server 则装完打印手动配置指引）:
#   ./install.sh --silent --server=wss://cockpit.cuihairu.site/ws
#
#   # 本地执行:
#   ./install.sh [--with-service] [--server URL] [选项]
#   （不带 --server 且终端可交互时，会提示输入服务端地址）
#
# 幂等：重跑即升级（覆盖二进制；已注册服务则自动重启加载新二进制）。
# 兼容 macOS 自带 bash 3.2（无关联数组/大小写展开等 4.0 特性）。
set -euo pipefail

REPO_DEFAULT="cuihairu/cockpit"
RELEASE_TAG_DEFAULT="nightly"
LABEL="com.cuihairu.cockpit-agent"
UNIT_NAME="cockpit-agent.service"

info() { printf '%s\n' "$*"; }
warn() { printf '[警告] %s\n' "$*" >&2; }
die()  { printf '错误: %s\n' "$*" >&2; exit 1; }

usage() {
	cat <<'EOF'
Cockpit Agent 一键安装（Linux / macOS）

选项:
  --install-dir DIR   安装目录（默认 /usr/local/bin，无写权限时 ~/.local/bin）
  --with-service      注册开机自启服务（Linux systemd / macOS launchd）
  --server URL        Server WebSocket 地址（注册服务时必需，路径指向 /ws）;
                      支持 --server URL 与 --server=URL 两种写法
  --silent            静默安装：跳过一切交互提示；不带 --server 时装完打印手动配置指引
  --id ID             Agent ID（可选，默认自动生成）
  --secret S          Agent 认证密钥（可选，推荐）
  --region R          地域（可选）
  --zone Z            可用区（可选）
  --labels L          标签，格式 key1=v1,key2=v2（可选）
  -h, --help          显示本帮助

交互: 不带 --server 且终端可交互（curl | bash 亦会经 /dev/tty 提示）时，
安装完成后提示输入服务端地址（域名/IP 或完整 wss:// 地址，回车跳过）。

环境变量（curl | bash 管道形态无法传参时使用）:
  COCKPIT_WITH_SERVICE=1  等价 --with-service
  COCKPIT_SILENT=1        等价 --silent
  COCKPIT_SERVER / COCKPIT_AGENT_ID / COCKPIT_SECRET / COCKPIT_REGION /
  COCKPIT_ZONE / COCKPIT_LABELS / COCKPIT_INSTALL_DIR
  COCKPIT_REPO（默认 cuihairu/cockpit）/ COCKPIT_RELEASE（默认 nightly）

重跑即升级（幂等）。安装完成后自动执行 cockpit-agent --version 验证。
EOF
}

# ---------- 参数与环境变量 ----------
INSTALL_DIR="${COCKPIT_INSTALL_DIR:-}"
WITH_SERVICE="${COCKPIT_WITH_SERVICE:-0}"
SILENT="${COCKPIT_SILENT:-0}"
SERVER="${COCKPIT_SERVER:-}"
AGENT_ID="${COCKPIT_AGENT_ID:-}"
SECRET="${COCKPIT_SECRET:-}"
REGION="${COCKPIT_REGION:-}"
ZONE="${COCKPIT_ZONE:-}"
LABELS="${COCKPIT_LABELS:-}"
REPO="${COCKPIT_REPO:-$REPO_DEFAULT}"
RELEASE_TAG="${COCKPIT_RELEASE:-$RELEASE_TAG_DEFAULT}"

while [ $# -gt 0 ]; do
	case "$1" in
	--install-dir)
		[ $# -ge 2 ] || die "--install-dir 需要一个目录参数"
		INSTALL_DIR="$2"
		shift 2
		;;
	--with-service) WITH_SERVICE=1; shift ;;
	--silent) SILENT=1; shift ;;
	--server=*)
		SERVER="${1#--server=}"
		[ -n "$SERVER" ] || die "--server= 需要一个地址参数"
		shift
		;;
	--server)
		[ $# -ge 2 ] || die "--server 需要一个 URL 参数"
		SERVER="$2"
		shift 2
		;;
	--id)
		[ $# -ge 2 ] || die "--id 需要一个参数"
		AGENT_ID="$2"
		shift 2
		;;
	--secret)
		[ $# -ge 2 ] || die "--secret 需要一个参数"
		SECRET="$2"
		shift 2
		;;
	--region)
		[ $# -ge 2 ] || die "--region 需要一个参数"
		REGION="$2"
		shift 2
		;;
	--zone)
		[ $# -ge 2 ] || die "--zone 需要一个参数"
		ZONE="$2"
		shift 2
		;;
	--labels)
		[ $# -ge 2 ] || die "--labels 需要一个参数"
		LABELS="$2"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		die "未知参数: $1（-h 查看用法）"
		;;
	esac
done

# ---------- OS / 架构检测 ----------
OS_RAW="$(uname -s)"
ARCH_RAW="$(uname -m)"

case "$OS_RAW" in
Linux) OS="linux" ;;
Darwin) OS="darwin" ;;
*)
	die "不支持的操作系统: $OS_RAW —— 本脚本支持 Linux 与 macOS；Windows 请用 install.ps1"
	;;
esac

case "$ARCH_RAW" in
x86_64 | amd64) ARCH="amd64" ;;
aarch64 | arm64) ARCH="arm64" ;;
armv7l | armv8l | armhf | arm) ARCH="arm" ;;
armv6l)
	die "不支持的架构: $ARCH_RAW（armv6，如树莓派 Zero/1）—— nightly 最低支持 armv7"
	;;
i386 | i486 | i586 | i686 | x86)
	die "不支持的架构: $ARCH_RAW（32 位 x86）—— nightly 未提供 386 产物"
	;;
mips | mipsel | mips64*)
	die "检测到 MIPS 架构（$ARCH_RAW）：通用 nightly 未提供 MIPS 产物；OpenWrt 设备请用每日构建的 ipk 包安装（opkg install cockpit-agent_*.ipk，见 deployments/openwrt/）"
	;;
*)
	die "不支持的架构: $ARCH_RAW —— 已支持: x86_64/amd64、aarch64/arm64、armv7（arm）；如确需此架构请到 ${REPO}/actions 手动构建"
	;;
esac

TARGET="${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/cockpit-agent-${TARGET}-nightly.tar.gz"

info "Cockpit Agent 一键安装"
info "  系统: ${OS} (${OS_RAW})"
info "  架构: ${ARCH} (${ARCH_RAW})"
info "  来源: ${URL}"
info ""

# ---------- root 提权辅助（非交互：只接受免密 sudo） ----------
as_root() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
		sudo -n "$@"
	else
		return 127
	fi
}

# ---------- 下载工具 ----------
fetch() {
	# fetch <url> <输出文件>
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --retry 3 --connect-timeout 15 -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q --tries=3 -O "$2" "$1"
	else
		die "需要 curl 或 wget 之一来下载产物，请先安装"
	fi
}

TMPDIR_DL="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_DL"' EXIT

info "下载 nightly 产物..."
if ! fetch "$URL" "$TMPDIR_DL/cockpit-agent.tar.gz"; then
	die "下载失败: $URL
  - 404：该平台（${TARGET}）的 nightly 产物可能尚未生成——每日构建在近 24h 有提交时于 00:00 UTC 重建；
  - 网络问题：请检查代理或稍后重试。"
fi

tar xzf "$TMPDIR_DL/cockpit-agent.tar.gz" -C "$TMPDIR_DL"
[ -f "$TMPDIR_DL/cockpit-agent" ] || die "解包异常：包内未找到 cockpit-agent 二进制"

# ---------- 安装目录决策 ----------
if [ -z "$INSTALL_DIR" ]; then
	if [ "$(id -u)" -eq 0 ]; then
		INSTALL_DIR="/usr/local/bin"
	elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
		INSTALL_DIR="/usr/local/bin"
	else
		INSTALL_DIR="${HOME}/.local/bin"
	fi
fi
mkdir -p "$INSTALL_DIR" 2>/dev/null || as_root mkdir -p "$INSTALL_DIR" ||
	die "无法创建安装目录: $INSTALL_DIR"

if [ -w "$INSTALL_DIR" ]; then
	install -m 0755 "$TMPDIR_DL/cockpit-agent" "$INSTALL_DIR/cockpit-agent"
else
	as_root install -m 0755 "$TMPDIR_DL/cockpit-agent" "$INSTALL_DIR/cockpit-agent" ||
		die "安装目录不可写且无可用的免密 sudo: $INSTALL_DIR（可用 --install-dir 指定其他目录）"
fi

BIN_PATH="$INSTALL_DIR/cockpit-agent"

# ---------- 安装后验证 ----------
VER_OUTPUT="$("$BIN_PATH" --version 2>/dev/null)" ||
	die "安装后验证失败：无法执行 $BIN_PATH --version（挂载点是否 noexec？）"
case "$VER_OUTPUT" in
"Cockpit Agent v"*) info "已安装: $VER_OUTPUT -> $BIN_PATH" ;;
*) die "版本输出异常: $VER_OUTPUT" ;;
esac

case ":$PATH:" in
*":$INSTALL_DIR:"*) ;;
*)
	warn "$INSTALL_DIR 不在当前 PATH 中——请加入 PATH 后再直接使用 cockpit-agent 命令"
	;;
esac

# ---------- 服务端地址：规范化 / 交互提示 / 校验 ----------
normalize_server_url() {
	# $1 = 用户输入（域名/IP[:端口] 或完整 ws(s):// URL）→ SERVER_URL_NORM
	# 纯地址默认按 wss:// 构造（服务端前置 nginx 80 端口 301 到 https，
	# ws:// 握手不跟随重定向会 bad handshake）；要走 ws:// 请输完整 URL
	local s port
	s="$(printf '%s' "$1" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
	[ -n "$s" ] || return 1
	case "$s" in
	ws://* | wss://*)
		case "$s" in
		*"/ws") ;;
		*"/ws/") s="${s%/}" ;;
		*) s="${s%/}/ws" ;;
		esac
		SERVER_URL_NORM="$s"
		return 0
		;;
	*)
		if printf '%s' "$s" | grep -qE '^[A-Za-z0-9._-]+(:[0-9]{1,5})?$'; then
			: # 域名 / IPv4[:端口]
		elif printf '%s' "$s" | grep -qE '^\[[0-9A-Fa-f:]+\](:[0-9]{1,5})?$'; then
			: # [IPv6][:端口]
		else
			return 1
		fi
		case "$s" in
		*:*)
			port="${s##*:}"
			case "$port" in
			'' | *[!0-9]*) ;;
			*) [ "$port" -ge 1 ] && [ "$port" -le 65535 ] || return 1 ;;
			esac
			;;
		esac
		SERVER_URL_NORM="wss://${s}/ws"
		return 0
		;;
	esac
}

prompt_server_interactive() {
	# 交互输入服务端地址。curl | bash 下 stdin 是脚本本身，必须读 /dev/tty；
	# 输入有效 → SERVER 置规范化 URL 返回 0；跳过 / 非交互 / 重试耗尽 → 返回 1
	# 注意：exec 的重定向持久生效，吞错须用组内重定向（否则 stderr 被永久
	# 指向 /dev/null，后续提示与 xtrace 全部静默——本节走查踩过的坑）
	[ "$SILENT" = "1" ] && return 1
	local input tries=3
	if ! { exec 3</dev/tty; } 2>/dev/null; then
		return 1
	fi
	while [ "$tries" -gt 0 ]; do
		printf '请输入 Cockpit 服务端地址（域名或 IP，例: cockpit.cuihairu.site 或 10.0.0.5:9000；完整 wss:// 地址亦可；直接回车跳过）: ' >&2
		IFS= read -r input <&3 || {
			{ exec 3<&-; } 2>/dev/null || true
			return 1
		}
		input="$(printf '%s' "$input" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
		if [ -z "$input" ]; then
			{ exec 3<&-; } 2>/dev/null || true
			return 1
		fi
		if normalize_server_url "$input"; then
			SERVER="$SERVER_URL_NORM"
			{ exec 3<&-; } 2>/dev/null || true
			return 0
		fi
		printf '[警告] 地址格式无效: %s（应为 域名[:端口] 或完整 ws(s):// URL）\n' "$input" >&2
		tries=$((tries - 1))
	done
	{ exec 3<&-; } 2>/dev/null || true
	return 1
}

verify_server_in_env() {
	# $1 = env 文件，$2 = 期望 SERVER_URL（整行精确匹配）
	# 系统级 env 为 root:600，当前用户不可读时经免密 sudo 读
	if [ -r "$1" ]; then
		grep -qx "SERVER_URL=$2" "$1" 2>/dev/null
	else
		as_root grep -qx "SERVER_URL=$2" "$1" 2>/dev/null
	fi
}

# 服务端地址解析：显式参数 > 交互提示；--with-service 拿不到地址则失败即停
if [ -n "$SERVER" ]; then
	normalize_server_url "$SERVER" ||
		die "无效的服务端地址: $SERVER（应为 域名[:端口] 或完整 ws(s):// URL）"
	SERVER="$SERVER_URL_NORM"
	info "服务端地址: $SERVER"
else
	if prompt_server_interactive; then
		info "服务端地址: $SERVER"
	elif [ "$WITH_SERVICE" = "1" ]; then
		die "注册服务需要服务端地址：传 --server=<域名或IP>，或以交互方式输入"
	else
		SERVER=""
	fi
fi

# ---------- 服务注册（可选，--with-service） ----------
SYSTEMD_UNIT_SYSTEM="/etc/systemd/system/${UNIT_NAME}"
SYSTEMD_UNIT_USER="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/${UNIT_NAME}"
ENV_FILE_SYSTEM="/etc/default/cockpit-agent"
ENV_FILE_USER="${XDG_CONFIG_HOME:-$HOME/.config}/cockpit-agent/env"
LAUNCHD_PLIST_SYSTEM="/Library/LaunchDaemons/${LABEL}.plist"
LAUNCHD_PLIST_USER="$HOME/Library/LaunchAgents/${LABEL}.plist"

# 连接信息解析：--server / COCKPIT_SERVER 优先（显式更新配置文件）；
# 未传参则复用既有配置文件；两者皆无 → 明确报错（失败即停，不注册半残服务）。
write_env_file() {
	# $1 = 目标 env 文件（systemd EnvironmentFile，KEY=VALUE 原样）
	[ -n "$SERVER" ] || die "注册服务需要连接信息：传 --server，或已存在配置文件 $1"
	local tmp
	tmp="$(mktemp)"
	{
		printf '# cockpit-agent 服务配置（install.sh 生成/更新）\n'
		printf 'SERVER_URL=%s\n' "$SERVER"
		printf 'REGION=%s\n' "$REGION"
		printf 'ZONE=%s\n' "$ZONE"
		printf 'AGENT_ID=%s\n' "$AGENT_ID"
		printf 'SECRET=%s\n' "$SECRET"
		printf 'LABELS=%s\n' "$LABELS"
	} >"$tmp"
	chmod 600 "$tmp"
	mkdir -p "$(dirname "$1")" 2>/dev/null || as_root mkdir -p "$(dirname "$1")" ||
		die "无法创建配置目录: $(dirname "$1")"
	if [ -w "$(dirname "$1")" ]; then
		mv "$tmp" "$1"
	else
		as_root mv "$tmp" "$1" || die "无法写入配置文件: $1"
	fi
	# 系统级 env 归 root（sudo mv 跨文件系统会保留源文件属主）；用户级 chown 无权限静默跳过
	as_root chown root:root "$1" 2>/dev/null || true
}

xml_escape() {
	printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g'
}

write_systemd_unit() {
	# $1 = system|user, $2 = unit 路径
	if [ "$1" = "system" ]; then
		as_root tee "$2" >/dev/null <<UNIT
[Unit]
Description=Cockpit Agent - Infrastructure Monitoring Agent
Documentation=https://github.com/cuihairu/cockpit
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cockpit
Group=cockpit
EnvironmentFile=${ENV_FILE_SYSTEM}
ExecStart=${BIN_PATH} start \\
    -server "\${SERVER_URL}" \\
    -region "\${REGION}" \\
    -zone "\${ZONE}" \\
    -id "\${AGENT_ID}" \\
    -secret "\${SECRET}" \\
    -labels "\${LABELS}"
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=cockpit-agent
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/cockpit-agent
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT
	else
		mkdir -p "$(dirname "$2")"
		tee "$2" >/dev/null <<UNIT
[Unit]
Description=Cockpit Agent - Infrastructure Monitoring Agent
Documentation=https://github.com/cuihairu/cockpit

[Service]
Type=simple
EnvironmentFile=${ENV_FILE_USER}
ExecStart=${BIN_PATH} start \\
    -server "\${SERVER_URL}" \\
    -region "\${REGION}" \\
    -zone "\${ZONE}" \\
    -id "\${AGENT_ID}" \\
    -secret "\${SECRET}" \\
    -labels "\${LABELS}"
Restart=always
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=default.target
UNIT
	fi
}

register_systemd() {
	if [ "$(id -u)" -eq 0 ] || as_root true 2>/dev/null; then
		# ---- 系统级：cockpit 专用用户 + /etc/default env + 系统单元 ----
		if ! id cockpit >/dev/null 2>&1; then
			NOLOGIN="$(command -v nologin || echo /usr/sbin/nologin)"
			as_root useradd --system --user-group --home-dir /var/lib/cockpit-agent \
				--shell "$NOLOGIN" cockpit ||
				die "创建系统用户 cockpit 失败"
		fi
		as_root mkdir -p /var/lib/cockpit-agent
		as_root chown cockpit:cockpit /var/lib/cockpit-agent
		if [ -n "$SERVER" ] || [ ! -f "$ENV_FILE_SYSTEM" ]; then
			write_env_file "$ENV_FILE_SYSTEM"
		fi
		write_systemd_unit system "$SYSTEMD_UNIT_SYSTEM"
		as_root systemctl daemon-reload ||
			die "systemctl daemon-reload 失败"
		as_root systemctl enable --now cockpit-agent ||
			die "服务启动失败——查看日志: journalctl -u cockpit-agent -n 20 --no-pager"
		sleep 1
		svc_state="$(systemctl is-active cockpit-agent 2>/dev/null || true)"
		if [ "$svc_state" = "active" ]; then
			info "服务已注册并启动: ${SYSTEMD_UNIT_SYSTEM}（active，开机自启）"
		else
			warn "服务已注册但状态为 ${svc_state:-unknown}——连接信息可能未通，查看: journalctl -u cockpit-agent -n 20 --no-pager"
		fi
	else
		# ---- 用户级：~/.config systemd user 单元 ----
		command -v systemctl >/dev/null 2>&1 ||
			die "未找到 systemd，无法注册服务"
		systemctl --user show >/dev/null 2>&1 ||
			die "无法访问用户级 systemd 会话（非登录环境？）——请改用 sudo 运行本脚本注册系统级服务"
		if [ -n "$SERVER" ] || [ ! -f "$ENV_FILE_USER" ]; then
			write_env_file "$ENV_FILE_USER"
		fi
		write_systemd_unit user "$SYSTEMD_UNIT_USER"
		systemctl --user daemon-reload || die "systemctl --user daemon-reload 失败"
		systemctl --user enable --now cockpit-agent ||
			die "用户级服务启动失败——查看: journalctl --user -u cockpit-agent -n 20 --no-pager"
		sleep 1
		svc_state="$(systemctl --user is-active cockpit-agent 2>/dev/null || true)"
		if [ "$svc_state" = "active" ]; then
			info "用户级服务已注册并启动: ${SYSTEMD_UNIT_USER}（active）"
			warn "如需未登录时也随开机运行: loginctl enable-linger $USER"
		else
			warn "用户级服务已注册但状态为 ${svc_state:-unknown}——查看: journalctl --user -u cockpit-agent -n 20 --no-pager"
		fi
	fi
}

write_launchd_plist() {
	# $1 = daemon|agent, $2 = plist 路径（launchd 无 EnvironmentFile 等价物，
	# 连接信息固化在 ProgramArguments 内；用户值经 XML 转义）
	[ -n "$SERVER" ] || die "注册 launchd 服务需要 --server"
	local args log_dir
	args="<string>${BIN_PATH}</string>
      <string>start</string>
      <string>-server</string><string>$(xml_escape "$SERVER")</string>"
	[ -n "$REGION" ] && args="${args}
      <string>-region</string><string>$(xml_escape "$REGION")</string>"
	[ -n "$ZONE" ] && args="${args}
      <string>-zone</string><string>$(xml_escape "$ZONE")</string>"
	[ -n "$AGENT_ID" ] && args="${args}
      <string>-id</string><string>$(xml_escape "$AGENT_ID")</string>"
	[ -n "$SECRET" ] && args="${args}
      <string>-secret</string><string>$(xml_escape "$SECRET")</string>"
	[ -n "$LABELS" ] && args="${args}
      <string>-labels</string><string>$(xml_escape "$LABELS")</string>"

	if [ "$1" = "daemon" ]; then
		log_dir="/var/log"
	else
		log_dir="${HOME}/.cockpit-agent"
		mkdir -p "$log_dir"
	fi
	{
		printf '<?xml version="1.0" encoding="UTF-8"?>\n'
		printf '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n'
		printf '<plist version="1.0">\n<dict>\n'
		printf '  <key>Label</key><string>%s</string>\n' "$LABEL"
		printf '  <key>ProgramArguments</key>\n  <array>\n      %s\n  </array>\n' "$args"
		printf '  <key>RunAtLoad</key><true/>\n'
		printf '  <key>KeepAlive</key><true/>\n'
		printf '  <key>ThrottleInterval</key><integer>10</integer>\n'
		printf '  <key>StandardOutPath</key><string>%s/cockpit-agent.log</string>\n' "$log_dir"
		printf '  <key>StandardErrorPath</key><string>%s/cockpit-agent.err.log</string>\n' "$log_dir"
		printf '</dict>\n</plist>\n'
	} >"$2"
}

register_launchd() {
	if [ "$(id -u)" -eq 0 ]; then
		PLIST="$LAUNCHD_PLIST_SYSTEM"
		DOMAIN="system"
		KIND="daemon"
	else
		PLIST="$LAUNCHD_PLIST_USER"
		DOMAIN="gui/$(id -u)"
		KIND="agent"
	fi
	command -v launchctl >/dev/null 2>&1 || die "未找到 launchctl，无法注册服务"
	mkdir -p "$(dirname "$PLIST")"
	# 已有 plist 且未传 --server → 复用既有配置（升级重跑路径）
	if [ ! -f "$PLIST" ] || [ -n "$SERVER" ]; then
		write_launchd_plist "$KIND" "$PLIST"
	fi
	chmod 644 "$PLIST" 2>/dev/null || true
	launchctl bootout "$DOMAIN/$LABEL" >/dev/null 2>&1 || true
	launchctl bootstrap "$DOMAIN" "$PLIST" ||
		die "launchctl bootstrap 失败: $PLIST"
	launchctl enable "$DOMAIN/$LABEL" || true
	launchctl kickstart -k "$DOMAIN/$LABEL" >/dev/null 2>&1 ||
		warn "launchctl kickstart 未成功——查看日志: $PLIST 同目录对应 err.log"
	info "launchd 服务已注册: $PLIST（RunAtLoad + KeepAlive）"
}

if [ "$WITH_SERVICE" = "1" ]; then
	case "$OS" in
	linux)
		command -v systemctl >/dev/null 2>&1 ||
			die "Linux 上注册服务需要 systemd（未找到 systemctl）"
		register_systemd
		;;
	darwin)
		register_launchd
		;;
	*)
		die "内部错误: 未知 OS $OS（服务注册未执行）"
		;;
	esac
else
	# 未要求注册服务：显式传了 --server 则更新既有服务的连接配置，随后重启加载新二进制
	# （重跑=升级/改配置，幂等）
	if [ -f "$SYSTEMD_UNIT_SYSTEM" ] && as_root systemctl is-enabled cockpit-agent >/dev/null 2>&1; then
		if [ -n "$SERVER" ]; then
			write_env_file "$ENV_FILE_SYSTEM"
			verify_server_in_env "$ENV_FILE_SYSTEM" "$SERVER" ||
				die "配置写入校验失败: $ENV_FILE_SYSTEM 中未找到 SERVER_URL=$SERVER"
			info "已更新服务连接配置: $ENV_FILE_SYSTEM（SERVER_URL=$SERVER）"
		fi
		if as_root systemctl restart cockpit-agent; then
			info "已检测到既有系统服务，已重启加载新版本"
		else
			warn "既有系统服务重启失败——查看: journalctl -u cockpit-agent -n 20 --no-pager"
		fi
	elif [ -f "$SYSTEMD_UNIT_USER" ] && systemctl --user is-enabled cockpit-agent >/dev/null 2>&1; then
		if [ -n "$SERVER" ]; then
			write_env_file "$ENV_FILE_USER"
			verify_server_in_env "$ENV_FILE_USER" "$SERVER" ||
				die "配置写入校验失败: $ENV_FILE_USER 中未找到 SERVER_URL=$SERVER"
			info "已更新服务连接配置: $ENV_FILE_USER（SERVER_URL=$SERVER）"
		fi
		if systemctl --user restart cockpit-agent; then
			info "已检测到既有用户级服务，已重启加载新版本"
		else
			warn "既有用户级服务重启失败"
		fi
	elif [ -f "$LAUNCHD_PLIST_SYSTEM" ] && [ "$(id -u)" -eq 0 ] && command -v launchctl >/dev/null 2>&1; then
		if [ -n "$SERVER" ]; then
			register_launchd
		else
			launchctl kickstart -k "system/$LABEL" >/dev/null 2>&1 &&
				info "已检测到既有 launchd 服务，已重启加载新版本"
		fi
	elif [ -f "$LAUNCHD_PLIST_USER" ] && command -v launchctl >/dev/null 2>&1; then
		if [ -n "$SERVER" ]; then
			register_launchd
		else
			launchctl kickstart -k "gui/$(id -u)/$LABEL" >/dev/null 2>&1 &&
				info "已检测到既有 launchd 服务，已重启加载新版本"
		fi
	elif [ -n "$SERVER" ]; then
		# 无任何既有服务：服务端地址写入用户级配置（装机时落盘）
		write_env_file "$ENV_FILE_USER"
		verify_server_in_env "$ENV_FILE_USER" "$SERVER" ||
			die "配置写入校验失败: $ENV_FILE_USER 中未找到 SERVER_URL=$SERVER"
		info "连接配置已写入: $ENV_FILE_USER（SERVER_URL=$SERVER）"
	fi
fi

info ""
info "完成。下一步:"
if [ "$WITH_SERVICE" != "1" ]; then
	info "  注册开机自启服务: ./install.sh --with-service --server wss://cockpit.cuihairu.site/ws"
	if [ -n "$SERVER" ]; then
		info "  或手动启动: cockpit-agent start -server $SERVER"
	else
		info "  未配置服务端地址——之后自己手动执行配置:"
		info "    cockpit-agent start -server wss://<你的服务端>/ws（例: wss://cockpit.cuihairu.site/ws）"
		info "    或重跑本脚本: ./install.sh --server=<域名或IP>（自动写入连接配置）"
	fi
fi
info "  验证版本: cockpit-agent --version"
