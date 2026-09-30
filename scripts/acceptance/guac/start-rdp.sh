#!/bin/sh
# 无 systemd 容器内跑 xrdp：sesman 先行，xrdp 前台
set -e
mkdir -p /var/run/xrdp
rm -f /var/run/xrdp/*.pid 2>/dev/null || true
xrdp-sesman --nodaemon &
sleep 1
exec xrdp -n
