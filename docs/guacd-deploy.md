# guacd 部署指南

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

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `GUACD_LOG_LEVEL` | 日志级别（别开 debug，会打印密码） | `info` |
| `GUACD_BIND` | 绑定地址（compose 文件用） | `127.0.0.1` |
| `GUACD_PORT` | 端口（compose 文件用） | `4822` |
| `TZ` | 时区 | `Asia/Shanghai` |

### Cockpit Server 端

Cockpit Server 通过 `GUACD_ADDR` 环境变量连接 guacd：

```bash
# 默认值（同机部署）
GUACD_ADDR=127.0.0.1:4822

# 跨机部署时指向 guacd 主机内网地址
GUACD_ADDR=192.168.5.10:4822
```

在 `/etc/cockpit/config.yaml` 同机部署不需要额外配置。

## SSH 密钥认证

guacd 本身不存储密钥。SSH 密钥由 **Cockpit Agent** 管理：

1. Agent 启动时自动检查 `~/.ssh/`，无密钥则生成 Ed25519
2. 用户通过 Web 发起 SSH 连接时，Server 调用 Agent 的 `ssh.getDefaultKey` RPC 获取私钥
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

## 跨机部署

当 guacd 与 Cockpit Server 不在同一台机器时：

1. 修改 compose 文件绑定内网地址：
   ```yaml
   ports:
     - "192.168.5.10:4822:4822"
   ```

2. Cockpit Server 设置环境变量：
   ```bash
   GUACD_ADDR=192.168.5.10:4822
   ```

3. 确保防火墙放行 4822 端口（仅内网，不暴露公网）。

## 支持的协议

| 协议 | 端口 | 说明 |
|------|------|------|
| SSH | 22 | 密钥/密码认证，字符终端 |
| VNC | 5900+ | 桌面远程控制 |
| RDP | 3389 | Windows 远程桌面 |

三种协议均通过 guacd 统一处理，Cockpit Server 只做双向字节管道转发。