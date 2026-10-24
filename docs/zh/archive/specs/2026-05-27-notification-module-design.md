# 通知模块设计规格

> **目标:** 为 Cockpit 添加外部通知能力，支持企业微信、飞书、钉钉等平台，通过集成 Herald 服务实现。

**日期:** 2026-05-27

---

## 概述

将 [Herald](https://github.com/cuihairu/herald) 事件驱动通知基础设施集成到 Cockpit，当系统产生警告时自动推送消息到配置的渠道。

**设计原则:**
- HTTP First - 通过 HTTP API 与 Herald 通信
- 非阻塞 - 通知发送失败不影响主业务
- 配置驱动 - 使用 YAML 配置文件
- 智能格式化 - 针对不同平台优化消息格式

---

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                         Cockpit                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐                                           │
│  │ Alert Module │                                           │
│  │              │                                           │
│  │ - Generator  │ ────> HTTP POST ─────────┐                │
│  │ - Storage    │                          │                │
│  └──────────────┘                          │                │
│        ▲                                   │                │
│        │                                   ▼                │
│  ┌─────────────────────────────────┐  ┌─────────────────┐  │
│  │  Herald Client (封装层)          │  │   Herald        │  │
│  │  internal/notification/         │  │   独立服务       │  │
│  │  - 事件类型映射                  │  │   :8080         │  │
│  │  - HTTP 封装                     │  │                 │  │
│  │  - 错误处理                      │  │  ┌───────────┐  │  │
│  └─────────────────────────────────┘  │  │ Dashboard │  │  │
│                                        │  └───────────┘  │  │
│                                        │  ┌───────────┐  │  │
│                                        │  │  Providers│  │  │
│                                        │  │ - 企业微信 │  │  │
│                                        │  │ - 飞书     │  │  │
│                                        │  │ - 钉钉     │  │  │
│                                        │  └───────────┘  │  │
│                                        └─────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

---

## 事件类型映射

| Cockpit 警告 | Herald 事件类型 | Level | 标题示例 |
|-------------|-----------------|-------|---------|
| 证书已过期 | `certificate.expired` | error | 证书已过期 |
| 证书即将过期(7天内) | `certificate.expiring` | error | 证书即将过期（7天内）|
| 证书即将过期(30天内) | `certificate.warning` | warning | 证书即将过期（30天内）|
| 服务宕机 | `service.down` | error | 服务宕机 |
| Agent 离线 | `agent.offline` | warning | Agent 离线 |
| 域名已过期 | `domain.expired` | error | 域名已过期 |

**Herald 事件 Payload 格式:**
```json
{
  "type": "certificate.expiring",
  "labels": {
    "level": "error",
    "domain": "example.com",
    "days_until_expiry": "5",
    "certificate_id": "xxx"
  }
}
```

---

## 配置系统

### 配置文件结构

**文件路径:** `config/cockpit.yaml`

```yaml
# 服务器配置
server:
  host: "0.0.0.0"
  port: 9000

# 数据库配置
database:
  path: "./data/cockpit.db"

# JWT 配置
jwt:
  secret: "your-secret-key"
  expiration: 24h

# 邮件配置（密码重置用）
email:
  enabled: true
  smtp:
    host: "smtp.gmail.com"
    port: 587
    username: "${SMTP_USER}"
    password: "${SMTP_PASS}"
    from: "noreply@example.com"
    from_name: "Cockpit"
  base_url: "http://localhost:9000"

# 通知配置（Herald 集成）
notification:
  enabled: true
  herald:
    base_url: "http://localhost:8080"
    timeout: 5s
  events:
    certificate_expired:
      type: "certificate.expired"
      enabled: true
    certificate_expiring:
      type: "certificate.expiring"
      enabled: true
    certificate_warning:
      type: "certificate.warning"
      enabled: true
    service_down:
      type: "service.down"
      enabled: true
    agent_offline:
      type: "agent.offline"
      enabled: true
    domain_expired:
      type: "domain.expired"
      enabled: true

# Agent 配置
agent:
  api_key_header: "X-API-Key"
```

### 配置结构定义

```go
// internal/config/config.go
package config

import (
    "os"
    "time"
    "gopkg.in/yaml.v3"
)

type Config struct {
    Server       *ServerConfig       `yaml:"server"`
    Database     *DatabaseConfig     `yaml:"database"`
    JWT          *JWTConfig          `yaml:"jwt"`
    Email        *EmailConfig        `yaml:"email"`
    Notification *NotificationConfig `yaml:"notification"`
    Agent        *AgentConfig        `yaml:"agent"`
}

type ServerConfig struct {
    Host string `yaml:"host"`
    Port int    `yaml:"port"`
}

type DatabaseConfig struct {
    Path string `yaml:"path"`
}

type JWTConfig struct {
    Secret     string        `yaml:"secret"`
    Expiration time.Duration `yaml:"expiration"`
}

type EmailConfig struct {
    Enabled bool     `yaml:"enabled"`
    SMTP    *SMTPConfig `yaml:"smtp"`
    BaseURL string   `yaml:"base_url"`
}

type SMTPConfig struct {
    Host     string `yaml:"host"`
    Port     int    `yaml:"port"`
    Username string `yaml:"username"`
    Password string `yaml:"password"`
    From     string `yaml:"from"`
    FromName string `yaml:"from_name"`
}

type NotificationConfig struct {
    Enabled bool                      `yaml:"enabled"`
    Herald  *HeraldConfig             `yaml:"herald"`
    Events  map[string]*EventConfig   `yaml:"events"`
}

type HeraldConfig struct {
    BaseURL string        `yaml:"base_url"`
    Timeout time.Duration `yaml:"timeout"`
}

type EventConfig struct {
    Type    string `yaml:"type"`
    Enabled bool   `yaml:"enabled"`
}

type AgentConfig struct {
    APIKeyHeader string `yaml:"api_key_header"`
}

// Load 从文件加载配置
func Load(path string) (*Config, error)

// expandEnv 扩展环境变量引用 ${VAR}
func expandEnv(s string) string
```

---

## 通知客户端

### 文件结构

```
internal/notification/
├── client.go           // Herald HTTP 客户端
├── events.go           // 事件类型定义和转换
└── config.go           // 配置接口
```

### 核心接口

```go
// internal/notification/client.go
package notification

import (
    "context"
    "time"
    "github.com/cuihairu/cockpit/internal/config"
    "github.com/cuihairu/cockpit/internal/storage"
)

// Client Herald 通知客户端
type Client struct {
    baseURL    string
    timeout    time.Duration
    httpClient *http.Client
    cfg        *config.NotificationConfig
}

// NewClient 创建通知客户端
func NewClient(cfg *config.NotificationConfig) *Client

// SendEvent 发送事件到 Herald
func (c *Client) SendEvent(ctx context.Context, event *Event) error

// SendAlert 从 Alert 模型发送通知
func (c *Client) SendAlert(ctx context.Context, alert *storage.Alert) error

// IsEnabled 检查通知是否启用
func (c *Client) IsEnabled() bool
```

### 事件定义

```go
// internal/notification/events.go
package notification

// Event Herald 事件
type Event struct {
    Type   string            `json:"type"`
    Labels map[string]string `json:"labels"`
}

// EventType 事件类型
type EventType string

const (
    CertificateExpired  EventType = "certificate.expired"
    CertificateExpiring EventType = "certificate.expiring"
    CertificateWarning  EventType = "certificate.warning"
    ServiceDown         EventType = "service.down"
    AgentOffline        EventType = "agent.offline"
    DomainExpired       EventType = "domain.expired"
)

// AlertToEvent 将 Alert 转换为 Herald Event
func AlertToEvent(alert *storage.Alert, cfg *config.NotificationConfig) *Event
```

---

## 错误处理与降级

| 场景 | 处理方式 |
|-----|---------|
| Herald 服务不可达 | 记录日志，不影响主业务 |
| 发送超时 | 配置的超时时间到期后放弃 |
| Herald 返回错误 | 记录日志，不重试（Herald 内部已处理）|
| 配置禁用 | 直接跳过通知逻辑 |

**非阻塞发送模式:**
```go
// 在 alert generator 中使用
go func() {
    if notificationClient.IsEnabled() {
        if err := notificationClient.SendAlert(context.Background(), alert); err != nil {
            log.Printf("Failed to send notification: %v", err)
        }
    }
}()
```

---

## 实施清单

### 新建文件

- `internal/config/config.go` - 统一配置加载
- `config/cockpit.yaml` - 配置文件（带示例）
- `internal/notification/client.go` - Herald HTTP 客户端
- `internal/notification/events.go` - 事件类型映射

### 修改文件

- `internal/alert/generator.go` - 集成通知发送
- `internal/server/server.go` - 加载并传递配置
- `internal/auth/password_reset.go` - 从配置读取 SMTP 设置
- `go.mod` - 添加 yaml 配置依赖

### 不变

- 数据库结构（无新增表）
- 前端代码（纯后端集成）

---

## Herald 服务要求

Herald 服务需独立部署，参考：https://github.com/cuihairu/herald

**Docker 部署示例:**
```bash
git clone https://github.com/cuihairu/herald.git
cd herald
cp .env.example .env
# 编辑 .env 配置企业微信/飞书/钉钉 Webhook
docker-compose up -d
```

**Herald 配置示例 (飞书):**
```yaml
providers:
  feishu:
    type: builtin
    config:
      webhook_url: "${FEISHU_WEBHOOK_URL}"
```

---

## 测试计划

1. **单元测试** - 配置加载、事件转换
2. **集成测试** - Mock Herald 服务，验证 HTTP 调用
3. **端到端测试** - 启动真实 Herald，验证消息送达

---

## 未来扩展

- [ ] 用户订阅模式 - 每个用户配置自己的通知渠道
- [ ] 通知历史记录 - 记录发送状态到数据库
- [ ] 前端配置页面 - 可视化配置通知渠道
- [ ] 更多平台 - Discord、Slack 等
