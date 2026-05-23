#!/bin/bash
set -e

# Cockpit Agent 安装脚本
# 用法: sudo ./install-agent.sh

RELEASE_VERSION="${RELEASE_VERSION:-latest}"
BINARY_URL="${BINARY_URL:-https://github.com/cuihairu/cockpit/releases/download/${RELEASE_VERSION}}"
DOWNLOAD_URL="${DOWNLOAD_URL:-}"

echo "Cockpit Agent 安装脚本"
echo "======================"

# 检测架构
ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l)  ARCH="armv7" ;;
    *)
        echo "不支持的架构: $ARCH"
        exit 1
        ;;
esac

# 检测操作系统
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

BINARY_NAME="cockpit-agent-${OS}-${ARCH}"

if [ -z "$DOWNLOAD_URL" ]; then
    DOWNLOAD_URL="${BINARY_URL}/${BINARY_NAME}"
fi

echo "系统: $OS"
echo "架构: $ARCH"
echo "下载地址: $DOWNLOAD_URL"
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    echo "请使用 sudo 运行此脚本"
    exit 1
fi

# 创建用户和组
echo "创建 cockpit 用户..."
if ! id cockpit &>/dev/null; then
    useradd --system --user-group --home-dir /var/lib/cockpit-agent --shell /usr/sbin/nologin cockpit
fi

# 下载二进制文件
echo "下载 cockpit-agent..."
TMP_FILE=$(mktemp)
curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"

# 安装二进制文件
echo "安装到 /usr/local/bin..."
install -o root -g root -m 755 "$TMP_FILE" /usr/local/bin/cockpit-agent
rm -f "$TMP_FILE"

# 创建状态目录
echo "创建状态目录..."
mkdir -p /var/lib/cockpit-agent
chown cockpit:cockpit /var/lib/cockpit-agent

# 安装 systemd 服务
echo "安装 systemd 服务..."
cat > /etc/systemd/system/cockpit-agent.service << 'EOF'
[Unit]
Description=Cockpit Agent - Infrastructure Monitoring Agent
Documentation=https://github.com/cuihairu/cockpit
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cockpit
Group=cockpit
EnvironmentFile=/etc/default/cockpit-agent
ExecStart=/usr/local/bin/cockpit-agent start \
    -server "${SERVER_URL}" \
    -region "${REGION}" \
    -zone "${ZONE}" \
    -id "${AGENT_ID}"
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
EOF

# 创建环境配置文件
echo "创建环境配置文件..."
if [ ! -f /etc/default/cockpit-agent ]; then
    cat > /etc/default/cockpit-agent << 'EOF'
# Cockpit Agent 配置
# Server WebSocket 地址 (必需)
SERVER_URL=ws://localhost:8080

# 地域 (可选)
REGION=

# 可用区 (可选)
ZONE=

# Agent ID (可选)
# AGENT_ID=
EOF
    echo "配置文件已创建: /etc/default/cockpit-agent"
    echo "请编辑此文件设置 SERVER_URL"
fi

# 重载 systemd
echo "重载 systemd..."
systemctl daemon-reload

echo ""
echo "安装完成！"
echo ""
echo "下一步:"
echo "  1. 编辑配置: sudo vi /etc/default/cockpit-agent"
echo "  2. 设置 SERVER_URL 指向你的 Cockpit Server"
echo "  3. 启动服务: sudo systemctl enable --now cockpit-agent"
echo "  4. 查看状态: sudo systemctl status cockpit-agent"
echo "  5. 查看日志: sudo journalctl -u cockpit-agent -f"
