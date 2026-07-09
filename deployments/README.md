# Cockpit 部署文件

此目录提供 Linux systemd 和 Windows Service 的安装脚本、服务单元和环境变量模板。

## 当前约定

| 项目 | 默认值 |
| --- | --- |
| Server CLI | `cockpit server -config <config.yaml>` |
| Agent CLI | `cockpit-agent start -server <ws-url>` |
| Server 端口 | `9000` |
| Agent WebSocket | `ws://server:9000/ws` 或 `wss://server/ws` |
| Server 配置 | `/etc/cockpit/config.yaml` 或 `C:\ProgramData\Cockpit\config.yaml` |
| Server 环境 | `/etc/default/cockpit-server` 或机器级环境变量 |
| Agent 环境 | `/etc/default/cockpit-agent` 或安装脚本参数 |

## Linux Server

快速安装：

```bash
curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/deployments/install-server.sh | sudo bash
```

安装脚本会创建：

- `/usr/local/bin/cockpit`
- `/etc/systemd/system/cockpit.service`
- `/etc/cockpit/config.yaml`
- `/etc/default/cockpit-server`
- `/var/lib/cockpit`

启动前至少编辑管理员密码：

```bash
sudo vi /etc/default/cockpit-server
sudo vi /etc/cockpit/config.yaml
sudo systemctl daemon-reload
sudo systemctl enable --now cockpit
```

常用命令：

```bash
sudo systemctl status cockpit
sudo systemctl restart cockpit
sudo journalctl -u cockpit -f
```

访问：

```text
http://<server-ip>:9000
```

## Docker Server

仓库内置 Dockerfile 与 Docker Compose 配置，适合直接在服务器上部署 Server：

```bash
cp deployments/docker/.env.example .env
vi .env
docker compose up -d --build
```

详细说明见 [docker/README.md](docker/README.md)。

## Linux Agent

快速安装：

```bash
curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/deployments/install-agent.sh | sudo bash
```

编辑 Agent 连接信息：

```bash
sudo vi /etc/default/cockpit-agent
sudo systemctl daemon-reload
sudo systemctl enable --now cockpit-agent
```

示例：

```dotenv
SERVER_URL=wss://cockpit.example.com/ws
REGION=home
ZONE=datacenter
AGENT_ID=server01
SECRET=optional-agent-secret
```

常用命令：

```bash
sudo systemctl status cockpit-agent
sudo systemctl restart cockpit-agent
sudo journalctl -u cockpit-agent -f
```

## Windows Server

以管理员身份运行 PowerShell：

```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
.\install-server.ps1 -AdminUsername "admin" -AdminPassword "change-this-password"
```

安装脚本会：

- 安装 `cockpit.exe`
- 创建 `C:\ProgramData\Cockpit\config.yaml`
- 设置机器级 `ADMIN_USERNAME` / `ADMIN_PASSWORD`
- 注册 `CockpitServer` 服务
- 添加 9000 端口防火墙规则

管理命令：

```powershell
Get-Service -Name CockpitServer
Restart-Service -Name CockpitServer
Stop-Service -Name CockpitServer
```

## Windows Agent

以管理员身份运行 PowerShell：

```powershell
.\install-agent.ps1 `
  -ServerUrl "wss://cockpit.example.com/ws" `
  -AgentId "server01" `
  -Region "home" `
  -Zone "datacenter" `
  -Secret "optional-agent-secret"
```

管理命令：

```powershell
Get-Service -Name CockpitAgent
Restart-Service -Name CockpitAgent
Stop-Service -Name CockpitAgent
```

卸载：

```powershell
.\uninstall-windows.ps1 -Component "All"
```

## 生产环境注意

- 必须设置强密码 `ADMIN_PASSWORD`。
- 设置 `PRODUCTION=true` 时必须提供强随机 `TOTP_ENCRYPTION_KEY`。
- 建议设置 `ALLOWED_ORIGINS=https://cockpit.example.com`。
- 对外访问建议由 Nginx、Caddy、Traefik 等反向代理提供 HTTPS/WSS。
- Agent 主动连接 Server，不需要在 Agent 节点开放入站端口。

## 防火墙

Server 需要开放 9000/TCP，或只开放反向代理端口：

```bash
sudo ufw allow 9000/tcp
```

Windows：

```powershell
New-NetFirewallRule -DisplayName "Cockpit Server" `
  -Direction Inbound `
  -LocalPort 9000 `
  -Protocol TCP `
  -Action Allow
```

## 故障排查

Agent 无法连接：

- 确认 `SERVER_URL` 以 `/ws` 结尾。
- 确认 Server 可访问且 WebSocket 路径未被反向代理拦截。
- 确认 Agent secret 与 Server 中记录一致。
- 查看 `journalctl -u cockpit-agent -f` 或 Windows 服务日志。

Server 无法启动：

- 确认设置了 `ADMIN_PASSWORD` 且至少 8 位。
- 确认配置文件字段是 `server.host` 和 `server.port`，不是旧的 `server.addr`。
- 确认数据库目录可由服务用户写入。
