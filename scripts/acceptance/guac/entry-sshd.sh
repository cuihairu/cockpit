#!/bin/sh
# 装载验收密钥对（setup-env.sh 生成，只读挂载 /pubkeys）后启动 sshd
set -e
if [ -d /pubkeys ]; then
    cat /pubkeys/*.pub > /home/accept/.ssh/authorized_keys
    chown accept:accept /home/accept/.ssh/authorized_keys
    chmod 600 /home/accept/.ssh/authorized_keys
fi
# sshd StrictModes 要求 .ssh 目录归用户所有且 700（镜像里 /home/accept/.ssh
# 是 root:700 → authorized_keys 被拒 → 私钥认证恒失败；真机验收发现 2026-09-30）
chown accept:accept /home/accept/.ssh
chmod 700 /home/accept/.ssh
exec /usr/sbin/sshd -D -e
