#!/usr/bin/env bash
# 面板数据库备份验收：本地双 cockpit 实例（server-only，无 agent——备份是
# server 本体链路）：
#   a  :19995  retention_days=1（tick 按天清理实证样本）
#   b  :19996  retention_days=0（0=永久 样本）
# interval_hours/retention_days 由探针启动后即刻 PUT（缺省 24h/7d 太慢）。
# 生产 tick 固定 1h（serverBackupLoop 每小时醒一次），定时产物最早出现在
# T+60min——探针负责轮询取证。token 有效期给 4h（探针全程 ≈65min）。
# 产物（.acceptance/server-backup/instance-{a,b}/）：配置/db/logs/token/pid
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
SB_DIR="${REPO_ROOT}/.acceptance/server-backup"

ADMIN_USER="${ADMIN_USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASSWORD:-e2e-strong-pass-1}"

echo "== 构建二进制 =="
mkdir -p "${SB_DIR}/bin"
go build -o "${SB_DIR}/bin/cockpit" ./cmd/cockpit

for inst in a b; do
    case "${inst}" in
        a) PORT=19995 ;;
        b) PORT=19996 ;;
    esac
    INST_DIR="${SB_DIR}/instance-${inst}"
    mkdir -p "${INST_DIR}/data" "${INST_DIR}/logs"

    # 复跑归一：清上一轮 DB 与备份目录——config Setting、备份列表全部回到
    # 新鲜基线（B0 断言缺省 24h/7d 依赖 DB 干净）
    rm -rf "${INST_DIR}/data/server-backups"
    rm -f "${INST_DIR}/data/cockpit.db" "${INST_DIR}/data/cockpit.db-"*

    cat > "${INST_DIR}/cockpit.yaml" <<EOF
server:
  host: 127.0.0.1
  port: ${PORT}
database:
  path: ${INST_DIR}/data/cockpit.db
jwt:
  secret: sb-accept-jwt-secret
  expiration: 4h
EOF

    echo "== 启动实例 ${inst} (:${PORT}) =="
    ADMIN_USERNAME="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASS}" \
        "${SB_DIR}/bin/cockpit" server -config "${INST_DIR}/cockpit.yaml" \
        > "${INST_DIR}/logs/server.log" 2>&1 &
    SERVER_PID=$!
    echo "${SERVER_PID}" > "${INST_DIR}/server.pid"

    for i in $(seq 1 30); do
        if curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then break; fi
        if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
            echo "server ${inst} 启动失败："; tail -n 30 "${INST_DIR}/logs/server.log"; exit 1
        fi
        sleep 1
    done
    curl -sf "http://127.0.0.1:${PORT}/health" >/dev/null || { echo "health 超时 (${inst})"; exit 1; }

    TOKEN=""
    for i in $(seq 1 30); do
        TOKEN=$(curl -sf -X POST "http://127.0.0.1:${PORT}/api/auth/login" \
            -H 'Content-Type: application/json' \
            -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
            | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])' 2>/dev/null) || TOKEN=""
        [[ -n "${TOKEN}" ]] && break
        sleep 1
    done
    [[ -n "${TOKEN}" ]] || { echo "登录失败 (${inst})"; tail -n 20 "${INST_DIR}/logs/server.log"; exit 1; }
    echo "${TOKEN}" > "${INST_DIR}/token"
    echo "实例 ${inst} ready (pid=${SERVER_PID}, :${PORT})"
done

echo "== 双实例就绪：定时 tick 预计 T+60min 触发，探针轮询取证 =="
