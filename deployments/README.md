# Cockpit 部署文件

此目录包含 Cockpit Server 和 Agent 的部署脚本和配置文件，支持 Linux (systemd) 和 Windows 服务。

## 目录

- [Linux 部署](#linux-部署)
  - [Server 部署](#server-部署)
  - [Agent 部署](#agent-部署)
- [Windows 部署](#windows-部署)
  - [Server 部署](#server-部署-1)
  - [Agent 部署](#agent-部署-1)
- [配置说明](#配置说明)

---

## Linux 部署

### Server 部署

Server 是中央管理服务器，提供 Web UI 和 API 接口。

**快速安装：**

```bash
curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/deployments/install-server.sh | sudo bash
```

**手动安装：**

```bash
# 1. 下载二进制文件到 /usr/local/bin/cockpit

# 2. 复制服务文件
sudo cp cockpit-server.service /etc/systemd/system/cockpit.service

# 3. 创建用户
sudo useradd --system --user-group --home-dir /var/lib/cockpit --shell /usr/sbin/nologin cockpit

# 4. 启动服务
sudo systemctl daemon-reload
sudo systemctl enable --now cockpit
```

**访问：** `http://<server-ip>:8080`

### Agent 部署

Agent 运行在被管理节点上，通过 WebSocket 连接到 Server。

**快速安装：**

```bash
curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/deployments/install-agent.sh | sudo bash
```

**手动安装：**

```bash
# 1. 下载二进制文件到 /usr/local/bin/cockpit-agent

# 2. 复制服务文件
sudo cp cockpit-agent.service /etc/systemd/system/cockpit-agent.service
sudo cp cockpit-agent.env /etc/default/cockpit-agent

# 3. 编辑配置
sudo vi /etc/default/cockpit-agent

# 4. 启动服务
sudo systemctl daemon-reload
sudo systemctl enable --now cockpit-agent
```

---

## Windows 部署

### Server 部署

**前置要求：** PowerShell 管理员权限

**快速安装：**

```powershell
# 以管理员身份运行 PowerShell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
.\install-server.ps1
```

**手动安装：**

```powershell
# 1. 创建安装目录
New-Item -ItemType Directory -Path "C:\Program Files\Cockpit" -Force

# 2. 下载 cockpit.exe 到安装目录

# 3. 创建数据目录
New-Item -ItemType Directory -Path "C:\ProgramData\Cockpit" -Force

# 4. 注册为 Windows 服务
New-Service -Name "CockpitServer" `
    -BinaryPathName "C:\Program Files\Cockpit\cockpit.exe server start" `
    -DisplayName "Cockpit Infrastructure Management Server" `
    -StartupType Automatic

# 5. 启动服务
Start-Service -Name "CockpitServer"
```

**访问：** `http://localhost:8080`

### Agent 部署

**前置要求：** PowerShell 管理员权限

**快速安装：**

```powershell
# 以管理员身份运行 PowerShell
.\install-agent.ps1 -ServerUrl "ws://your-server:8080" -Region "jiangsu-huaian" -Zone "datacenter-a"
```

**手动安装：**

```powershell
# 1. 创建安装目录
New-Item -ItemType Directory -Path "C:\Program Files\CockpitAgent" -Force

# 2. 下载 cockpit-agent.exe 到安装目录

# 3. 注册为 Windows 服务
New-Service -Name "CockpitAgent" `
    -BinaryPathName '"C:\Program Files\CockpitAgent\cockpit-agent.exe" start -server "ws://your-server:8080"' `
    -DisplayName "Cockpit Infrastructure Monitoring Agent" `
    -StartupType Automatic

# 4. 启动服务
Start-Service -Name "CockpitAgent"
```

**卸载：**

```powershell
.\uninstall-windows.ps1 -Component "All"  # Server + Agent
# 或
.\uninstall-windows.ps1 -Component "Server"  # 仅 Server
# 或
.\uninstall-windows.ps1 -Component "Agent"   # 仅 Agent
```

---

## 配置说明

### Agent 配置参数

| 参数 | Linux 环境变量 | Windows 参数 | 必需 | 说明 | 示例 |
|------|---------------|-------------|------|------|------|
| Server 地址 | `SERVER_URL` | `-ServerUrl` | 是 | Server WebSocket 地址 | `ws://192.168.1.10:8080` |
| 地域 | `REGION` | `-Region` | 否 | 地域标识 | `jiangsu-huaian` |
| 可用区 | `ZONE` | `-Zone` | 否 | 可用区标识 | `datacenter-a` |
| Agent ID | `AGENT_ID` | `-AgentId` | 否 | 自定义 Agent ID | 默认自动生成 |

### 连接安全

对于公网部署，建议使用加密连接：

```bash
# Linux
SERVER_URL=wss://cockpit.example.com:8080

# Windows
.\install-agent.ps1 -ServerUrl "wss://cockpit.example.com:8080"
```

---

## 管理命令

### Linux

**Server 管理：**

```bash
sudo systemctl status cockpit
sudo systemctl restart cockpit
sudo systemctl stop cockpit
sudo journalctl -u cockpit -f
```

**Agent 管理：**

```bash
sudo systemctl status cockpit-agent
sudo systemctl restart cockpit-agent
sudo systemctl stop cockpit
sudo journalctl -u cockpit-agent -f
```

### Windows

**Server 管理：**

```powershell
Get-Service -Name CockpitServer
Restart-Service -Name CockpitServer
Stop-Service -Name CockpitServer
Get-EventLog -LogName Application -Source CockpitServer -Newest 50
```

**Agent 管理：**

```powershell
Get-Service -Name CockpitAgent
Restart-Service -Name CockpitAgent
Stop-Service -Name CockpitAgent
Get-EventLog -LogName Application -Source CockpitAgent -Newest 50
```

---

## 防火墙配置

### Linux

```bash
# firewall-cmd
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# ufw
sudo ufw allow 8080/tcp
```

### Windows

```powershell
New-NetFirewallRule -DisplayName "Cockpit Server" `
    -Direction Inbound `
    -LocalPort 8080 `
    -Protocol TCP `
    -Action Allow
```

**注意：** Agent 主动连接 Server，无需开放入站端口。

---

## 故障排查

### Agent 无法连接

1. 检查 Server URL 是否正确
2. 检查网络连通性：`curl -v ws://server:8080` (Linux)
3. 检查 Server 防火墙
4. 查看日志：
   - Linux: `sudo journalctl -u cockpit-agent -n 50`
   - Windows: `Get-EventLog -LogName Application -Newest 50`

### Server 无法访问

1. 检查服务状态
2. 检查端口监听：`sudo netstat -tlnp | grep 8080`
3. 检查防火墙规则
