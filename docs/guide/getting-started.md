# 快速开始

## 环境要求

### Server
- Go 1.23+
- 8080 端口（可配置）

### Agent
- Go 1.23+（或直接下载预编译二进制）
- 能访问 Server 的网络连接

## 安装 Server

### 1. 下载

```bash
git clone https://github.com/cuihairu/cockpit.git
cd cockpit
go build -o cockpit ./cmd/cockpit
```

或直接下载预编译版本（TODO）

### 2. 初始化

```bash
./cockpit init
```

这将创建：
```
~/.cockpit/
├── config.yaml       # 主配置
└── inventory/        # 资产配置目录
    ├── cockpit.yaml
    ├── regions/
    ├── zones/
    └── resources/
```

### 3. 启动

```bash
./cockpit server start
```

默认监听 `http://localhost:8080`

## 部署 Agent

### 1. 下载

在目标机器上：

```bash
git clone https://github.com/cuihairu/cockpit.git
cd cockpit
go build -o cockpit-agent ./cmd/cockpit-agent
```

### 2. 启动

```bash
./cockpit-agent start --server ws://your-server-ip:8080
```

Agent 会：
1. 连接到 Server
2. 自动注册（报告位置和能力）
3. 开始心跳（每 30 秒）

### 3. 验证

在 Server 上：

```bash
./cockpit agent list
```

应该看到：
```
AGENT_ID              LOCATION           STATUS
agent-huainan-dc-a    jiangsu-huaian/dc-a online
```

## 添加第一个资产

### 1. 创建地域

```bash
# 或者直接编辑 YAML
cat > ~/.cockpit/inventory/regions/local.yaml <<EOF
apiVersion: cockpit.dev/v1alpha1
kind: Region
metadata:
  name: local
  displayName: 本地
spec:
  description: 家庭/本地机房
  location: 江苏淮安
  timezone: Asia/Shanghai
EOF
```

### 2. 创建计算实例

```bash
cat > ~/.cockpit/inventory/resources/compute-instances/pve-host-01.yaml <<EOF
apiVersion: cockpit.dev/v1alpha1
kind: ComputeInstance
metadata:
  id: compute-pve-host-01
  name: pve-host-01
  displayName: PVE 主机 01
spec:
  region: local
  zone: home-lab
  type: bare-metal
  platform: proxmox
  platformUrl: https://192.168.1.10:8006
  access:
    web:
      url: https://192.168.1.10:8006
  monitoring:
    enabled: true
EOF
```

### 3. 同步到数据库

```bash
./cockpit sync
```

### 4. 查看状态

```bash
./cockpit status
```

## 下一步

- [部署指南](/guide/deploy-server) —— 公网部署、内网部署
- [资产定义](/guide/inventory) —— 添加更多资产类型
- [监控配置](/guide/monitoring) —— 配置告警和健康检查
