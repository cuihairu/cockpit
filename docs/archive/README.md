# 归档设计稿

本目录保存 Cockpit 早期阶段的设计文档与实施计划，**仅供历史参考**，不代表当前运行架构。

- 当前的架构与边界请看 [`../guide/architecture.md`](../guide/architecture.md)。
- 当前协议与 API 请看 [`../guide/protocol.md`](../guide/protocol.md)。
- 想了解功能现状请看仓库根目录的 `README.md` 和 `todo.md`。

## 目录结构

```
specs/   设计规格：描述当时设想的目标、数据模型和接口
plans/   实施计划：分步骤的执行清单，包含大量 agent 工作流元信息
```

## 收录内容

| 日期 | 主题 | 类型 | 当前状态 |
| --- | --- | --- | --- |
| 2026-05-26 | TOTP 二次验证 | spec + plan | 已落地，参见 `internal/auth/totp.go` 与前端 Settings 安全设置 |
| 2026-05-27 | 通知模块（Herald 集成） | spec + plan | 设计稿状态，未完整落地；当前告警走 `internal/notification` 的内置渠道 |

## 维护约定

- 这些文档不再随代码演进同步，若与现行实现冲突以代码和 `guide/` 为准。
- 新功能设计稿请直接写在 `guide/` 或 PR 描述中，不要再放进本目录。
- 如需删除某份归档，请先在 PR 中说明并保留 git 历史。
