#!/usr/bin/env bash
# package-ipk.sh 组装 OpenWrt .ipk 安装包（ar 归档：debian-binary +
# control.tar.gz + data.tar.gz），并附 tar.gz 手动分发包。
#
# 用法: package-ipk.sh <binary> <openwrt-arch> <version> <out-dir>
#   binary       已交叉编译好的 cockpit-agent 二进制
#   openwrt-arch opkg 架构名（mipsel/mips/aarch64_generic/...）
#                与 `opkg print-architecture` 输出一致
#   version      包版本（nightly 用日期，如 0.1.0-20260920）
#   out-dir      产物目录
#
# 产物:
#   <out-dir>/cockpit-agent_<version>_<arch>.ipk
#   <out-dir>/cockpit-agent-openwrt-<arch>.tar.gz

set -euo pipefail

binary=$1
arch=$2
version=$3
out_dir=$4

if [ ! -x "$binary" ]; then
    echo "error: binary not found or not executable: $binary" >&2
    exit 1
fi

mkdir -p "$out_dir"
pkg_root=$(mktemp -d)
trap 'rm -rf "$pkg_root"' EXIT

# ---- data.tar.gz：目标机文件树（根 = /）----
install -D -m 0755 "$binary" "$pkg_root/data/usr/bin/cockpit-agent"
install -D -m 0755 "$(dirname "$0")/openwrt/cockpit-agent.init" \
    "$pkg_root/data/etc/init.d/cockpit-agent"
install -D -m 0644 "$(dirname "$0")/openwrt/cockpit-agent.conf" \
    "$pkg_root/data/etc/config/cockpit-agent"

tar -C "$pkg_root/data" --owner=0 --group=0 --mtime='@0' -czf "$pkg_root/data.tar.gz" .

# ---- control.tar.gz ----
cat > "$pkg_root/control" <<EOF
Package: cockpit-agent
Version: $version
Architecture: $arch
Section: net
Priority: optional
Maintainer: cuihairu <cuihairu@users.noreply.github.com>
Depends:
Provides: cockpit-agent
Description: Cockpit Agent - personal hybrid infrastructure agent.
 Personal infrastructure monitoring agent; connects to a Cockpit
 server over WebSocket. Pure static Go binary, no runtime deps.
EOF

# conffiles：升级时保留用户配置
echo /etc/config/cockpit-agent > "$pkg_root/conffiles"

# postinst：真机安装（非 chroot 构建环境）时自启用并提示配置
cat > "$pkg_root/postinst" <<'EOF'
#!/bin/sh
[ -n "${IPKG_INSTROOT:-}" ] && exit 0
/etc/init.d/cockpit-agent enable
echo "cockpit-agent installed. Configure then start:"
echo "  uci set cockpit-agent.main.server='wss://<your-server>/ws'"
echo "  uci set cockpit-agent.main.secret='<secret>'"
echo "  uci commit cockpit-agent && /etc/init.d/cockpit-agent start"
exit 0
EOF
chmod 0755 "$pkg_root/postinst"

tar -C "$pkg_root" --owner=0 --group=0 --mtime='@0' -czf "$pkg_root/control.tar.gz" \
    control conffiles postinst

# ---- ipk：ar 归档，debian-binary 必须是第一个成员 ----
echo "2.0" > "$pkg_root/debian-binary"
ipk="$out_dir/cockpit-agent_${version}_${arch}.ipk"
ar rc "$ipk" "$pkg_root/debian-binary" "$pkg_root/control.tar.gz" "$pkg_root/data.tar.gz"

# ---- tar.gz 手动分发包（scp 到机器解开即用）----
tgz="$out_dir/cockpit-agent-openwrt-${arch}.tar.gz"
tar -C "$pkg_root/data" --owner=0 --group=0 --mtime='@0' -czf "$tgz" .

echo "OK: $ipk"
echo "OK: $tgz"
