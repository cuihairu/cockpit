#!/usr/bin/env bash
# 远控三协议（Guacamole SSH/RDP/VNC）真机验收：环境搭建
#
# 前置（见 docs/guide/acceptance-checklist.md 远控三协议节）：
#   - docker daemon 在线（无 compose CLI 亦可，全部 docker run）
#   - 本机 guacamole/guacd:1.5.5 镜像（docker pull 自动带回）
#
# 产物（.acceptance/guac/，已 gitignore）：
#   rec/    录制共享目录（GUACD_RECORDING_PATH，guacd 容器与 server 同路径挂载）
#   keys/   验收密钥对（ed25519 OpenSSH 格式 + RSA PEM 格式，测 guacd 私钥格式兼容面）
#   logs/   容器日志
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
GUAC_DIR="${REPO_ROOT}/.acceptance/guac"
SRC_DIR="${REPO_ROOT}/scripts/acceptance/guac"

SSH_PORT=2222   # 本机 22 被真 sshd 占用（publickey-only），验收目标起在 2222
REC_DIR="${GUAC_DIR}/rec"

mkdir -p "${GUAC_DIR}"/{rec,keys,logs}
chmod 0777 "${REC_DIR}"   # guacd 容器内以 guacd 用户落录制文件

# ---- 验收密钥对 ----
if [[ ! -f "${GUAC_DIR}/keys/ed25519" ]]; then
    ssh-keygen -q -t ed25519 -N "" -C guac-accept-ed25519 -f "${GUAC_DIR}/keys/ed25519"
fi
if [[ ! -f "${GUAC_DIR}/keys/rsa_pem" ]]; then
    # -m PEM：经典 PEM 头格式，对照 OpenSSH 新格式验证 guacd/libssh2 私钥兼容面
    ssh-keygen -q -t rsa -b 3072 -m PEM -N "" -C guac-accept-rsa -f "${GUAC_DIR}/keys/rsa_pem"
fi

cd "${SRC_DIR}"

# ---- 构建目标镜像 ----
docker build -q -t cockpit-acc-sshd -f Dockerfile.sshd . >/dev/null
docker build -q -t cockpit-acc-vnc -f Dockerfile.vnc . >/dev/null
docker build -q -t cockpit-acc-rdp -f Dockerfile.rdp . >/dev/null

# ---- 清理旧容器 ----
for name in cockpit-acc-sshd cockpit-acc-vnc cockpit-acc-rdp cockpit-acc-guacd cockpit-acc-chrome; do
    docker rm -f "${name}" >/dev/null 2>&1 || true
done

# ---- SSH 目标：真 OpenSSH，口令 + 公钥认证 ----
docker run -d --name cockpit-acc-sshd \
    -p 127.0.0.1:${SSH_PORT}:22 \
    -v "${GUAC_DIR}/keys:/pubkeys:ro" \
    cockpit-acc-sshd >/dev/null

# ---- VNC 目标：Xvnc 真服务器（agent 探测 5900 端口 → Workbench VNC 入口可见） ----
docker run -d --name cockpit-acc-vnc \
    -p 127.0.0.1:5900:5900 \
    cockpit-acc-vnc >/dev/null

# ---- RDP 目标：xrdp（尽力而为，3389 同理被 agent 探测） ----
docker run -d --name cockpit-acc-rdp \
    -p 127.0.0.1:3389:3389 \
    cockpit-acc-rdp >/dev/null

# ---- guacd：host 网络（4822 即本机回环可达），录制目录同路径双挂 ----
docker run -d --name cockpit-acc-guacd \
    --network host \
    -e GUACD_LOG_LEVEL=info \
    -v "${REC_DIR}:${REC_DIR}" \
    guacamole/guacd:1.5.5 >/dev/null

# ---- 就绪探测 ----
wait_port() {
    local port="$1" name="$2" i
    for i in $(seq 1 30); do
        if timeout 1 bash -c "</dev/tcp/127.0.0.1/${port}" 2>/dev/null; then
            echo "${name} ready on 127.0.0.1:${port}"
            return 0
        fi
        sleep 1
    done
    echo "ERROR: ${name} not ready on 127.0.0.1:${port}" >&2
    return 1
}
wait_port ${SSH_PORT} "sshd"
wait_port 4822 "guacd"
wait_port 5900 "vnc"
wait_port 3389 "rdp"

docker logs cockpit-acc-guacd > "${GUAC_DIR}/logs/guacd.log" 2>&1 || true
echo
echo "== 验收环境就绪 =="
docker ps --filter name=cockpit-acc- --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
