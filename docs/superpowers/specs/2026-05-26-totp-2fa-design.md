# TOTP 二次验证功能设计文档

**日期**: 2026-05-26
**状态**: 设计阶段
**作者**: AI Assistant

## 1. 概述

为 Cockpit 添加 TOTP (Time-based One-Time Password) 二次验证功能，提升账号安全性。支持 Google Authenticator、Microsoft Authenticator 等标准 TOTP 应用。

### 核心策略
- **可选启用**: 用户可自主选择启用
- **首次登录引导**: 类似 GitHub 的引导流程
- **两页面登录**: 登录页 → TOTP 验证页
- **备份恢复**: 10 个一次性恢复码

---

## 2. 数据库设计

### 2.1 User 表扩展

```go
type User struct {
    // ... 现有字段
    TOTPSecret   string     `gorm:"column:totp_secret" json:"-"`
    TOTPEnabled  bool       `gorm:"column:totp_enabled;default:false" json:"totp_enabled"`
    BackupCodes  string     `gorm:"column:backup_codes" json:"-"`
    TOTPSetupAt  *time.Time `gorm:"column:totp_setup_at" json:"totp_setup_at,omitempty"`
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `totp_secret` | string | Base32 TOTP 密钥，AES-256 加密存储 |
| `totp_enabled` | boolean | 是否已启用 TOTP |
| `backup_codes` | string | 10 个恢复码的 SHA256 哈希，JSON 数组 |
| `totp_setup_at` | timestamp | TOTP 启用时间 |

### 2.2 迁移策略
- 新字段可为空，向后兼容
- 现有用户不受影响
- 自动迁移在启动时执行

---

## 3. 后端 API 设计

### 3.1 新增端点

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/totp/generate` | 生成新密钥和 QR 码 |
| POST | `/api/auth/totp/enable` | 验证后启用 TOTP |
| POST | `/api/auth/totp/disable` | 禁用 TOTP（需验证） |
| GET | `/api/auth/totp/backup-codes` | 获取备份码（仅一次） |
| POST | `/api/auth/totp/verify` | 验证 TOTP 代码 |

### 3.2 登录流程修改

**请求/响应变更：**

```go
// LoginResponse 新增字段
type LoginResponse struct {
    Token       string `json:"token,omitempty"`
    RequiresTOTP bool   `json:"requires_totp"`
    TmpToken    string `json:"tmp_token,omitempty"`
    // ... 其他字段
}
```

**登录流程：**
1. 用户提交用户名/密码
2. 验证成功后检查 `totp_enabled`
3. 若已启用：返回 `requires_totp: true` + `tmp_token`
4. 前端跳转到 TOTP 验证页
5. 提交 TOTP 代码 + `tmp_token`
6. 验证成功返回正式 `token`

### 3.3 临时 Token 规则
- 有效期：5 分钟
- 单次使用，验证后立即失效
- 存储格式：`tmp:{userID}:{timestamp}`

---

## 4. 前端设计

### 4.1 目录结构

```
web/src/
├── pages/
│   ├── Login/
│   │   └── index.tsx          (修改：处理 TOTP 流程)
│   ├── TOTPVerify/            (新增)
│   │   ├── index.tsx
│   │   └── index.less
│   └── SetupTOTP/             (新增)
│       ├── index.tsx
│       └── index.less
├── components/
│   ├── QRCodeDisplay/         (新增)
│   │   └── index.tsx
│   └── BackupCodesDisplay/    (新增)
│       └── index.tsx
└── services/
    └── api.ts                 (修改：新增 TOTP API)
```

### 4.2 首次登录引导流程

```
登录成功 → 检查 totp_enabled
           ↓
        false? → 显示"安全设置建议"模态框
           ↓           ↓
        立即设置    稍后提醒(可跳过)
           ↓
    SetupTOTP 页面
    1. 显示 QR 码
    2. 扫码后输入验证码确认
    3. 显示备份码（要求保存）
    4. 完成设置
```

### 4.3 页面组件

**SetupTOTP (设置引导页)**
- 步骤条：扫码 → 验证 → 备份码
- QR 码展示（支持重新生成）
- 验证码输入（6 位）
- 备份码展示（只显示一次）

**TOTPVerify (验证页)**
- 6 位数字输入
- 备份码输入（切换模式）
- "丢失设备？"帮助链接

---

## 5. 安全设计

### 5.1 加密方案
- **密钥存储**: AES-256-GCM
- **密钥来源**: 环境变量 `TOTP_ENCRYPTION_KEY`
- **备份码**: SHA256 哈希存储

### 5.2 防暴力破解
- TOTP 验证：5 次/10分钟（IP + 用户 ID）
- 临时 token：5 分钟过期
- 失败记录：审计日志

### 5.3 时间窗口容错
- 允许 ±1 个时间步长（30秒）
- 防止服务器时间漂移

### 5.4 备份码规则
- 格式：`xxxx-xxxx-xxxx`
- 使用后失效，不可重用
- 全部用完可重新生成

---

## 6. 兼容的认证器

以下应用均支持，因为遵循 RFC 6238 标准：
- Google Authenticator
- Microsoft Authenticator
- Authy
- 1Password
- Bitwarden
- 其他标准 TOTP 应用

---

## 7. 依赖库

### 后端
```go
require (
    github.com/pquerna/otp v1.4.0
)
```

### 前端
```bash
npm install qrcode @types/qrcode
```

---

## 8. 实施顺序

1. **数据库层** - User 表扩展、加密工具
2. **后端核心** - TOTP 逻辑、API 端点、登录流程
3. **前端服务层** - API 客户端、类型定义
4. **前端页面** - TOTP 验证页、设置引导页
5. **测试** - 单元测试、集成测试、手动测试

---

## 9. 预计工作量

- 后端：~400 行代码
- 前端：~350 行代码
- 测试：~200 行代码
