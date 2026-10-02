# Agent ID 生成策略

## 概述

Agent ID 是 Cockpit Agent 在 Server 端的唯一标识。ID 在 Agent 生命周期内保持稳定——**重启不变化、重连复用同一 ID**，Server 将其视为同一台机器。

## 生成规则

### 自动模式（默认，无 `-id` 参数）

基于操作系统原生的 **机器标识** 生成，格式：

```
agent-{hostname}-{machineId前8位}
```

示例：`agent-coding-edb1f8ab`

各平台机器标识来源：

| 平台 | 来源 | 说明 |
|------|------|------|
| Linux | `/etc/machine-id` | systemd 首次启动时生成，32 位 hex，持久化不变 |
| macOS | `IOPlatformUUID` | 硬件级 UUID，绑定主板，重装系统不变 |
| Windows | `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` | 安装系统时生成，持久化不变 |

> 不支持的平台 fallback 为 `agent-{hostname}-{random}`（每次重启变化，应显式指定 `-id`）。

### 手动模式（`-id` 参数）

直接使用指定值作为 Agent ID：

```bash
cockpit-agent start -server wss://example.com/ws -id my-custom-agent
```

## 偏移量（`-bias`）

同一台机器部署多个 Agent 时，用 `-bias` 区分（默认 0，不追加后缀）：

```bash
# Agent 0: agent-coding-edb1f8ab
cockpit-agent start -server wss://example.com/ws

# Agent 1: agent-coding-edb1f8ab-1
cockpit-agent start -server wss://example.com/ws -bias 1

# Agent 2: agent-coding-edb1f8ab-2
cockpit-agent start -server wss://example.com/ws -bias 2
```

手动 `-id` 同样支持 bias：

```bash
# my-agent-1
cockpit-agent start -server wss://example.com/ws -id my-agent -bias 1
```

## 完整 ID 生成流程

```
是否指定 -id ?
├── 是 → ID = {-id} + (bias > 0 ? "-{bias}" : "")
└── 否 → 读取 machineID()
         ├── 成功 → ID = "agent-{hostname}-{machineId[:8]}" + (bias > 0 ? "-{bias}" : "")
         └── 失败 → ID = "agent-{hostname}-{random}" + (bias > 0 ? "-{bias}" : "")
```

## 注意事项

### 克隆虚拟机

从同一镜像批量创建的 VM 会共享 `/etc/machine-id`，需要重新生成：

```bash
# 重新生成 machine-id（systemd）
sudo systemd-machine-id-setup
# 或手动
sudo rm /etc/machine-id
sudo systemd-machine-id-setup
sudo systemctl restart cockpit-agent
```

### Docker 容器

容器默认继承宿主机的 machine-id。隔离方式：

```bash
# 方式 1：挂载独立 machine-id
echo "$(uuidgen | tr -d '-')" > /etc/cockpit-agent/machine-id
docker run -v /etc/cockpit-agent/machine-id:/etc/machine-id:ro ...

# 方式 2：显式指定 ID
cockpit-agent start -server wss://example.com/ws -id container-01
```

### macOS 首次运行

macOS 读取 `IOPlatformUUID` 不需要 root 权限，无需额外配置。

### Windows 首次运行

读取注册表 `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid`，普通用户权限即可。