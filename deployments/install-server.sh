#!/bin/bash
set -e

# Cockpit Server 安装脚本
# 用法: sudo ./install-server.sh

RELEASE_VERSION="${RELEASE_VERSION:-latest}"
BINARY_URL="${BINARY_URL:-https://github.com/cuihairu/cockpit/releases/download/${RELEASE_VERSION}}"
DOWNLOAD_URL="${DOWNLOAD_URL:-}"

echo "Cockpit Server 安装脚本"
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

BINARY_NAME="cockpit-${OS}-${ARCH}"

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
    useradd --system --user-group --home-dir /var/lib/cockpit --shell /usr/sbin/nologin cockpit
fi

# 下载二进制文件
echo "下载 cockpit..."
TMP_FILE=$(mktemp)
curl -fsSL "$DOWNLOAD_URL" -o "$TMP_FILE"

# 安装二进制文件
echo "安装到 /usr/local/bin..."
install -o root -g root -m 755 "$TMP_FILE" /usr/local/bin/cockpit
rm -f "$TMP_FILE"

# 创建状态目录
echo "创建状态目录..."
mkdir -p /var/lib/cockpit
chown cockpit:cockpit /var/lib/cockpit

# 安装 systemd 服务
echo "安装 systemd 服务..."
cat > /etc/systemd/system/cockpit.service << 'EOF'
[Unit]
Description=Cockpit Server - Infrastructure Management Server
Documentation=https://github.com/cuihairu/cockpit
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cockpit
Group=cockpit
ExecStart=/usr/local/bin/cockpit server start
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=cockpit
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/cockpit
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

# 重载 systemd
echo "重载 systemd..."
systemctl daemon-reload

echo ""
echo "安装完成！"
echo ""
echo "下一步:"
echo "  启动服务: sudo systemctl enable --now cockpit"
echo "  查看状态: sudo systemctl status cockpit"
echo "  查看日志: sudo journalctl -u cockpit -f"
echo ""
echo "  Web UI: http://$(hostname -I | awk '{print $1}'):8080"
