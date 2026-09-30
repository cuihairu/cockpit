#!/usr/bin/env bash
# Traefik 后端热加载真机验收（proxy-design.md M2 真机项）：环境搭建。
#
# 形态（对齐生产惯例「Traefik 容器 + 宿主目录挂载」）：
#   - Traefik 容器（traefik:v3.5）：file provider watch 宿主动态目录，
#     entrypoints web(:80→18180) / websecure(:443→18453)，api.insecure(:8080→18181)
#     ——API 只绑 127.0.0.1，供探针核对路由注册面（热加载的直接证据）
#   - 上游后端 ×2：宿主 python3 http.server（19091/19092，marker index.html），
#     Traefik 经 host.docker.internal:host-gateway 回连宿主
#   - 自签证书（SAN s2.accept.test）：验证 https 站点的证书文件引用真被加载
#
# 前置：docker daemon 在线（无 compose CLI 亦可，docker run 即可）+ openssl。
# 产物（.acceptance/traefik/，已 gitignore）：
#   dynamic/ 宿主动态目录（agent 写、Traefik 读，两侧同看这个目录）
#   certs/   自签证书；backends/{a,b}/ marker 首页；logs/ evidence/
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TF_DIR="${REPO_ROOT}/.acceptance/traefik"
DYN_DIR="${TF_DIR}/dynamic"
CERT_DIR="${TF_DIR}/certs"
BACKEND_DIR="${TF_DIR}/backends"

TRAEFIK_HTTP_PORT=18180   # Traefik web entrypoint（宿主侧）
TRAEFIK_API_PORT=18181    # api.insecure（/ping /api/http/routers /api/version）
TRAEFIK_TLS_PORT=18453    # websecure entrypoint（宿主侧）
BACKEND_A_PORT=19091
BACKEND_B_PORT=19092

command -v openssl >/dev/null || { echo "需要 openssl（生成自签证书）"; exit 1; }

mkdir -p "${DYN_DIR}" "${CERT_DIR}" "${TF_DIR}"/{logs,evidence} "${BACKEND_DIR}"/{a,b}

# ---- 上游后端 marker 首页（curl 落到哪个后端，body 一眼可辨）----
echo "TRAEFIK-ACC-BACKEND-A" > "${BACKEND_DIR}/a/index.html"
echo "TRAEFIK-ACC-BACKEND-B" > "${BACKEND_DIR}/b/index.html"

# ---- 自签证书（s2.accept.test，自己就是 CA：curl --cacert 即可验证
#      Traefik 真的加载了动态配置里引用的那份证书文件）----
if [[ ! -f "${CERT_DIR}/accept.pem" ]]; then
    openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
        -keyout "${CERT_DIR}/accept.key" -out "${CERT_DIR}/accept.pem" \
        -subj "/CN=s2.accept.test" \
        -addext "subjectAltName=DNS:s2.accept.test" >/dev/null 2>&1
fi

# ---- Traefik 容器（宿主动态目录挂进容器作 file provider 目录）----
docker rm -f cockpit-acc-traefik >/dev/null 2>&1 || true
docker pull -q traefik:v3.5 >/dev/null
docker run -d --name cockpit-acc-traefik \
    -p 127.0.0.1:${TRAEFIK_HTTP_PORT}:80 \
    -p 127.0.0.1:${TRAEFIK_API_PORT}:8080 \
    -p 127.0.0.1:${TRAEFIK_TLS_PORT}:443 \
    -v "${DYN_DIR}:/etc/traefik/dynamic" \
    -v "${CERT_DIR}:/etc/traefik/certs:ro" \
    --add-host=host.docker.internal:host-gateway \
    traefik:v3.5 \
    --providers.file.directory=/etc/traefik/dynamic \
    --providers.file.watch=true \
    --entrypoints.web.address=:80 \
    --entrypoints.websecure.address=:443 \
    --api.insecure=true \
    --log.level=INFO >/dev/null

# 版本与镜像摘要入证据（浮动 tag v3.5，实际解析到哪版以这里为准）
{
    echo "traefik image: $(docker inspect --format '{{.Config.Image}} {{.Image}}' cockpit-acc-traefik)"
    curl -sf "http://127.0.0.1:${TRAEFIK_API_PORT}/api/version" 2>/dev/null || true
    echo
} > "${TF_DIR}/evidence/setup.log"

# ---- 上游后端（宿主进程，pidfile 交 teardown 清理）----
start_backend() { # port dir name
    local port="$1" dir="$2" name="$3"
    if [[ -f "${TF_DIR}/${name}.pid" ]] && kill -0 "$(cat "${TF_DIR}/${name}.pid")" 2>/dev/null; then
        return 0
    fi
    nohup python3 -m http.server "${port}" --directory "${dir}" \
        > "${TF_DIR}/logs/${name}.log" 2>&1 &
    echo $! > "${TF_DIR}/${name}.pid"
}
start_backend ${BACKEND_A_PORT} "${BACKEND_DIR}/a" backend-a
start_backend ${BACKEND_B_PORT} "${BACKEND_DIR}/b" backend-b

# ---- 就绪探测 ----
wait_url() { # url name
    local i
    for i in $(seq 1 30); do
        if curl -sf -o /dev/null "$1"; then echo "$2 ready"; return 0; fi
        sleep 1
    done
    echo "ERROR: $2 未就绪（$1）" >&2
    docker logs cockpit-acc-traefik 2>&1 | tail -20 >&2 || true
    return 1
}
# v3 默认不开 --ping，就绪探测用 api.insecure 的 /api/version
wait_url "http://127.0.0.1:${TRAEFIK_API_PORT}/api/version" "traefik /api/version"
wait_url "http://127.0.0.1:${BACKEND_A_PORT}/" "backend-a"
wait_url "http://127.0.0.1:${BACKEND_B_PORT}/" "backend-b"

docker logs cockpit-acc-traefik > "${TF_DIR}/logs/traefik-startup.log" 2>&1 || true
echo
echo "== 验收环境就绪 =="
docker ps --filter name=cockpit-acc-traefik --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
cat "${TF_DIR}/evidence/setup.log"
