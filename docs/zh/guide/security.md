# 安全

本页汇总控制面的安全边界。远控票据流程与目标白名单限制见[运行时状态](/zh/guide/runtime)。

## 安全概念

| 边界 | 当前实现 |
| --- | --- |
| 用户认证 | JWT Bearer token |
| 管理员初始化 | `ADMIN_USERNAME` 可选，`ADMIN_PASSWORD` 必需 |
| 二次验证 | TOTP，密钥落盘加密，密钥来源 `COCKPIT_VAULT_KEY`（回退 `TOTP_ENCRYPTION_KEY`） |
| Agent 认证 | 已配置 secret 的 Agent 必须在注册时提供匹配 secret |
| WebSocket Origin | `ALLOWED_ORIGINS` 控制；未设置时适合开发但不适合生产 |
| 远程连接 | 5 分钟、单次使用 ticket |
