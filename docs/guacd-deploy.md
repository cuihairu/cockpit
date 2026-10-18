# guacd 部署指南

> 语言：**中文** ｜ [English](./guacd-deploy.en.md)

## 概述

guacd（Apache Guacamole Daemon）是 Cockpit 远程桌面网关的协议翻译层。浏览器通过 Cockpit Server（Go 网关）与 guacd 通信，guacd 负责 SSH/VNC/RDP 协议终结。

```
浏览器 ↔ Cockpit Server (Go) ↔ guacd (TCP 4822) ↔ 目标机器
```

## 快速部署

### Docker Compose（推荐）

```bash
# 创建目录
mkdir -p /opt/cockpit/guacd

# 下载 compose 文件
curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/deployments/guacd/docker-compose.yml \
  -o /opt/cockpit/guacd/docker-compose.yml

# 启动
cd /opt/cockpit/guacd
docker compose up -d

# 验证
docker compose ps        # 状态应为 healthy
nc -z 127.0.0.1 4822     # 端口应可达
```

### Docker 命令

```bash
docker run -d \
  --name cockpit-guacd \
  --restart unless-stopped \
  -p 127.0.0.1:4822:4822 \
  -e GUACD_LOG_LEVEL=info \
  -e TZ=Asia/Shanghai \
  -v guacd-recordings:/var/lib/guacamole \
  guacamole/guacd:1.5.5
```

## 配置

### guacd 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `GUACD_LOG_LEVEL` | 日志级别（别开 debug，会打印密码） | `info` |
| `GUACD_BIND` | 绑定地址（compose 文件用） | `127.0.0.1` |
| `GUACD_PORT` | 端口（compose 文件用） | `4822` |
| `TZ` | 时区 | `Asia/Shanghai` |

### Cockpit Server 中继环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `GUAC_RELAY_BIND_ADDR` | Server 中继监听的具体本地 IP；留空时自动使用 Server 连接 guacd 的路由接口 | 自动 |
| `GUAC_RELAY_HOST` | 写入 Guacamole connect 指令、供 guacd 回拨的 Server 主机名/IP；留空时使用绑定 IP | 自动 |

### Cockpit Server 端

Cockpit Server 通过 `GUACD_ADDR` 环境变量连接 guacd：

```bash
# 同一 Docker 网络：使用服务 DNS 名（推荐）
GUACD_ADDR=guacd:4822

# 非容器、同机部署
GUACD_ADDR=127.0.0.1:4822

# 跨机部署时指向 guacd 主机内网地址；Server 必须可被 guacd 回连
GUACD_ADDR=192.168.5.10:4822
```

在 `/etc/cockpit/config.yaml` 同机部署不需要额外配置。

## SSH 密钥认证

guacd 本身不存储密钥。SSH 密钥由 **Cockpit Agent** 管理：

1. Agent 启动时自动检查 `~/.ssh/`，无密钥则生成 Ed25519
2. 用户在 Web 发起 SSH 连接且**既未填私钥也未填口令**时，Server 才调用 Agent 的 `ssh.getDefaultKey` RPC 获取默认私钥（显式凭据一律原样透传）
3. Server 将私钥 PEM 传给 guacd 的 `private-key` 参数
4. guacd 用该密钥认证连接目标机器

用户只需将 Agent 的公钥添加到目标机器的 `authorized_keys`：

```bash
# 查看 Agent 公钥
cat /home/cui/.ssh/id_ed25519.pub

# 添加到目标机器
ssh-copy-id -i /home/cui/.ssh/id_ed25519.pub user@target-host
```

## 数据卷

| 卷 | 用途 |
|------|------|
| `guacd-recordings` | 桌面会话录制文件（`.guac`） |
| `guacd-drive` | RDP 驱动器重定向（阶段二预留） |

录制文件由 Cockpit Server 在会话结束后自动收走归档到数据卷的
`/data/recordings/` 目录（元数据回填数据库；`collectGuacRecording`）。

## 健康检查

```bash
# 容器健康状态
docker inspect cockpit-guacd --format='{{.State.Health.Status}}'

# TCP 探活
nc -z 127.0.0.1 4822

# 日志
docker logs cockpit-guacd --tail 20
```

## 升级

```bash
cd /opt/cockpit/guacd
docker compose pull
docker compose up -d
```

## 分离网络 / 跨机部署

首选将 Cockpit Server 与 guacd 放在同一个受限 Docker 网络，设置
`GUACD_ADDR=guacd:4822`。Server 会用连接 guacd 的 TCP 路由接口 IP 创建
**绑定到具体接口**的临时中继，并把该 IP/端口交给 guacd；中继不是 loopback
服务，也不会绑定 `0.0.0.0`。

若 guacd 位于另一主机或两者不共享可路由网络，除 Server 可访问 guacd 的
`GUACD_ADDR` 外，还必须设置 Server 侧 `GUAC_RELAY_BIND_ADDR` 为 Server 上
可被 guacd 回连的具体本地 IP，并设置 `GUAC_RELAY_HOST` 为 guacd 可解析、可达
的 Server 地址。防火墙须仅允许 guacd 到该 Server 地址的中继端口范围；不要将
中继端口暴露到公网。中继使用系统临时端口，需结合主机/容器网络 ACL 收口。
仅映射 guacd 的 4822 端口或仅设置 `GUACD_ADDR` 不会自动打通反向回连。

## 支持的协议

| 协议 | 端口 | 说明 |
|------|------|------|
| SSH | 22 | 密钥/密码认证，字符终端 |
| VNC | 5900+ | 桌面远程控制 |
| RDP | 3389 | Windows 远程桌面 |

三种协议均通过 guacd 统一处理，Cockpit Server 只做双向字节管道转发。
