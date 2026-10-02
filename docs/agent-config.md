# Agent 配置参考

## 启动参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-server` | Server WebSocket 地址（必需） | — |
| `-id` | Agent ID（可选，自动从 machine-id 生成） | `agent-{hostname}-{machineId[:8]}` |
| `-secret` | Agent 认证密钥 | — |
| `-region` | 地域 | — |
| `-zone` | 可用区 | — |
| `-labels` | 标签，格式 `key1=v1,key2=v2` | — |
| `-bias` | 同机器多 Agent 偏移量 | `0` |
| `-ssh-keys` | SSH 私钥目录 | `~/.ssh/` |

示例：

```bash
cockpit-agent start \
  -server wss://cockpit.cuihairu.site/ws \
  -region shanghai -zone home \
  -secret MY_SECRET \
  -ssh-keys /home/user/.ssh
```

## 环境变量

通过 `/etc/default/cockpit-agent`（systemd）或 Docker `-e` 设置。

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `COCKPIT_SSH_KEYS` | SSH 私钥目录（覆盖 `-ssh-keys` 和自动检测） | 自动检测 |
| `SERVER_URL` | Server 地址（systemd wrapper 用） | `wss://cockpit.cuihairu.site/ws` |
| `REGION` | 地域 | — |
| `ZONE` | 可用区 | — |
| `AGENT_ID` | Agent ID | — |
| `SECRET` | 认证密钥 | — |
| `LABELS` | 标签 | — |
| `BIAS` | 偏移量 | — |
| `COCKPIT_AGENT_OPTS` | 额外启动参数（追加到命令行） | — |

## SSH 密钥路径

Agent 建立远程 SSH 连接时，按以下顺序查找私钥：

```
1. 前端显式传入的密钥（通过 Web 终端的认证表单）
2. COCKPIT_SSH_KEYS 环境变量指定的目录
3. SUDO_USER 的 ~/.ssh/（systemd/sudo 场景，如 /home/cui/.ssh/）
4. 当前用户的 ~/.ssh/（兜底，如 /root/.ssh/）
```

在每个目录中，按优先级扫描：

| 文件 | 算法 |
|------|------|
| `id_ed25519` | Ed25519（推荐） |
| `id_rsa` | RSA |
| `id_ecdsa` | ECDSA |
| `id_dsa` | DSA（不推荐） |

### systemd 部署

Agent 以 root 运行，但自动读取 `SUDO_USER`（实际用户）的密钥：

```bash
# 密钥放在实际用户目录即可
/home/cui/.ssh/id_ed25519

# 或在 /etc/default/cockpit-agent 中指定
COCKPIT_SSH_KEYS=/home/cui/.ssh
```

### Docker 部署

```bash
# 方式 1：挂载宿主机密钥（只读）
docker run -v /home/user/.ssh:/root/.ssh:ro cockpit-agent

# 方式 2：指定自定义路径
docker run \
  -e COCKPIT_SSH_KEYS=/app/keys \
  -v /path/to/keys:/app/keys:ro \
  cockpit-agent

# 方式 3：不指定，默认读容器内 /root/.ssh/
docker run cockpit-agent
```

### 生成密钥

```bash
# 生成 Ed25519 密钥（推荐）
ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -C "cockpit-agent@$(hostname)"

# 将公钥添加到目标机器
ssh-copy-id -i ~/.ssh/id_ed25519.pub user@target-host
```

## systemd 服务管理

```bash
# 查看状态
sudo systemctl status cockpit-agent

# 查看日志
sudo journalctl -u cockpit-agent -f

# 重启
sudo systemctl restart cockpit-agent

# 配置文件
sudo vim /etc/default/cockpit-agent
```

配置文件 `/etc/default/cockpit-agent` 格式：

```bash
SERVER_URL=wss://cockpit.cuihairu.site/ws
REGION=shanghai
ZONE=home
SECRET=your-secret-here
COCKPIT_AGENT_OPTS="-labels env=prod,role=web"
```