# 方案设计：多账号与细粒度权限控制（RBAC）

> 2026-09-19。多账号权限控制首期设计。需求原文：「账号权限控制，支持多账号」。
> 本文只做设计决策与接口定义，实现按 P0/P1 分期。

## 现状与痛点

多账号基础已存在：`User` 表（用户名/密码/TOTP/部门）、用户 CRUD API、JWT 携带
`Role`。缺的是权限模型——判定全靠散落的字符串比较：

| 痛点 | 现状 | 后果 |
|------|------|------|
| 只有两级角色 | `Role` 字段硬编码 `admin` / `user` 两种取值 | 想给运维同事「能操作但不能管用户」做不到 |
| 判定散落 | 10+ 处 handler 手写 `user.Role != "admin"`（`api.go` 394/703/802/844/850/893/911…） | 新增模块容易漏判；审计「谁能干什么」要人肉 grep |
| 无权限语义 | 「admin」同时意味着用户管理、证书签发、文件读写、终端接入 | 无法回答「这个账号能碰生产文件吗」 |
| user 角色无定义 | 非 admin 除改自己资料外几乎什么都做不了，但这是隐式约定 | 前端菜单与后端判定各自为政 |

## 架构总览

RBAC-lite：**用户 → 角色 → 权限点**。权限判定收敛为 server 侧一个中间件，
agent 与 websocket 通道零改动（权限在 server 收口，agent 只接受 server 单向调用）。

```
┌─ web ─────────────────┐      ┌─ server ──────────────────────────────────┐
│ 路由守卫 + v-perm 指令 │─JWT─▶│ auth middleware：验签 → UserInfo{Role}     │
│                       │      │   ↓                                        │
│ /api/me 返回          │      │ requirePermission(perm)：按 Role 查角色表   │
│ permissions[]         │      │   ↓（SQLite 本地读，微秒级，不做缓存）      │
│ 菜单/按钮按权限裁剪    │      │ handler：删除散落的 Role != "admin"         │
└───────────────────────┘      └────────────────────────────────────────────┘
```

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | 权限模型 | RBAC（角色→权限点集合），不做 ABAC/ACL | 管理面用户量个位数，按岗位授权足够；ABAC 的属性条件无处取值 |
| D2 | 权限点粒度 | `<resource>:<action>`，action ∈ `read` / `write` / `admin` | 模块级足够（如 `files:write`、`acme:admin`）；按钮级授权成本高收益低 |
| D3 | resource 清单 | 与现有 API 模块一一对应：`inventory` `files` `logs` `terminal` `docker` `stack` `cron` `backup` `acme` `dns` `ddns` `proxy` `overlay` `drift` `nas` `alerts` `audit` `users` `roles` `settings`，实现时对账补全 `services` `smart` `recordings` | 后端已有路由前缀就是天然 resource，不发明新分类 |
| D4 | 角色存储 | 新增 `Role` 表（`name` 主键、`permissions` JSON、`builtin` 标记）；`User.role` 沿用字符串存角色名 | GORM AutoMigrate 一张表搞定；User 不需要外键约束，角色名即软引用 |
| D5 | 权限校验位置 | auth middleware 之后加 `requirePermission(perm)`；`admin` 权限点隐含该 resource 的 read/write | 中间件链一处收口；「admin 隐含读写」让内置 admin 角色不用枚举全部权限点 |
| D6 | 每请求查库 | 校验时直接查 `roles` 表，不做内存缓存 | 管理面 QPS 个位数；缓存失效逻辑（角色改了要踢在线用户）比省的那点查询贵得多 |
| D7 | JWT 内容 | 沿用现状：token 只带角色名 | 角色定义变更对已发 token **立即生效**（好性质）；把权限点塞进 token 反而要处理「token 未过期但权限已收窄」 |
| D8 | 内置角色 | `admin`（全量含 `users:admin`/`roles:admin`）、`operator`（全部模块 read+write，无 users/roles/settings）、`viewer`（全模块 read） | 覆盖 90% 场景；自定义角色兜剩余 10% |
| D9 | 存量迁移 | `role=admin` → admin；`role=user` → viewer，首次启动 seed 内置角色 | 保持现状语义（现在 user 就没有写权限），要放宽管理员手工升 operator |
| D10 | agent 侧 | 零改动 | agent 无用户概念；RPC 由 server 发起，权限在 REST 层拦截即等效拦截 RPC |
| D11 | websocket/终端 | 连接建立时校验 `terminal:write`，会话期内不复查 | 终端会话中途断权易产生半截命令；重连即按新权限判定 |
| D12 | 角色管理 API | 仅 `roles:admin`（内置 admin）：CRUD 自定义角色；内置角色不可改删 | 防止自锁；自定义角色权限点必须落在 D3 清单内（后端校验） |
| D13 | 自我保护 | 不可删除/降级最后一个有效 admin；不可修改自己的角色 | 常规多租户系统事故教训，一条校验省一次数据卷救援 |
| D14 | 审计 | 角色增改、授权变更、越权拒绝（403）全部入审计日志 | 「谁给谁提了权」必须可追溯；403 记录用于发现权限配置不足 |

## 权限点清单（D3 展开）

```
inventory:read/write    files:read/write       logs:read           terminal:write
docker:read/write       stack:read/write       cron:read/write     backup:read/write
acme:read/write/admin   dns:read/write         ddns:read/write     proxy:read/write
overlay:read/write      drift:read/write       nas:read/write      alerts:read/write
audit:read              users:admin            roles:admin         settings:admin
```

约定：`<r>:write` 隐含 `<r>:read`；`<r>:admin` 隐含 write（如 acme 的签发/吊销/
CA 目录切换归 `acme:admin`，查看证书列表 `acme:read`）。

## 分期

| 期 | 内容 | 验收 |
|----|------|------|
| P0（后端） | Role 表 + 内置角色 seed + `requirePermission` 中间件 + 存量 handler 判定收敛 + 权限点清单常量 + D13/D14 校验 | 全部 API 在三内置角色下的 200/403 矩阵有测试覆盖 |
| P1（前端+管理） | 用户/角色管理页、`/api/me` 返回 permissions、路由守卫与菜单裁剪、v-perm 按钮指令 | viewer 登录看不到任何写操作入口 |
| P2（按需） | 自定义角色 UI 完善、权限模板、部门级授权（Department 字段已有，暂不参与判定） | 按实际需求排期 |

## 实现状态（P0 拆笔）

| 笔 | 内容 | 状态 |
|----|------|------|
| 1 | storage：`Role` 表（name 主键 / permissions JSON / builtin）+ 权限点常量与内置角色定义（admin/operator/viewer）+ seed（幂等，内置角色 permissions 以代码为准覆盖更新）+ CRUD（自定义角色权限点白名单校验；删除时拒内置、拒被引用）+ D9 存量迁移 `role=user → viewer` | ✅ |
| 2 | server：路由权限表（path 前缀 × method → 权限点，支持 AND 语义，domain-binding 增删改要求 `dns:write`+`proxy:write`）+ `authorize` 判定层（auth 后、serveAPI 分发前；每请求查库，角色缺失 fail-closed 403）+ 隐含规则（write⊇read、admin⊇write）+ 三角色矩阵测试。**不删存量判定**——authorize 拦在前，散落 `Role != "admin"` 暂成双保险 | ✅ |
| 3 | 收敛：删 10 处散落 `Role != "admin"`（api.go ×7、api_proxy.go ×3），users/settings 端点改走权限表。落地时追加两项：创建/更新用户时校验角色名在角色表存在（幽灵角色 fail-closed 会锁号，`errors.Is(ErrNotFound)` 才 400、库故障放行 500）；创建用户默认角色 user→viewer（D9 迁移后 user 已不存在）。改密免验旧密码条件从 admin 字符串改为 `userHasPerm(users:admin)` | ✅ |
| 4 | 角色/用户管理 API：`/api/roles` CRUD（仅 `roles:admin`）+ D13 自我保护（最后一个有效 admin 不可删/降级、不可改自己角色）+ D14 403 入审计。落地时追加三项：**「有效 admin」= 角色在角色表存在且覆盖 `users:admin`**（幽灵角色不算）；D13 计数出错时同样拒绝（fail-safe，库故障不做保护性变更）；RBAC 403 短路不经过内层 auth 挂点，审计层在请求前做可选认证（有效 Bearer 先塞 ctx）保证能记到「谁」被拒——内层 auth 幂等重复解析无害 | ✅ |
| P1 | web：用户/角色管理页、`/api/me` permissions、菜单裁剪（拆笔见下表） | 拆 4 笔 |

## 实现状态（P1 拆笔）

验收标准（设计阶段 P1 行）：viewer 登录看不到任何写操作入口。

| 笔 | 内容 | 状态 |
|----|------|------|
| 5 | server：`/api/me` 返回 `permissions`（查角色表展开；幽灵角色 → 空清单，与判定层 fail-closed 一致） | ✅ |
| 6 | web 权限基础设施：UserContext 启动拉 `/api/me` 存 permissions（D7 语义：动态拉取而非 login 固化，角色变更刷新即生效）+ `usePerm`/`PermGuard`（前端复刻 write⊇read、admin⊇write 隐含规则）+ 菜单按权限标注与裁剪 + 路由守卫（无 read 权限 → 403 页；后端 RBAC 仍是权威，守卫纯 UX）。落地决策：permissions 不落 localStorage（刷新即重拉，避免陈旧判定）；未加载/加载失败一律 fail-closed 全裁剪；菜单项 perm 与后端 requiredPerms 的 GET 语义一一对齐（monitor→inventory、disk→smart、域名绑定→dns:read）；设置菜单全员可见（含个人 TOTP），Tab 级裁剪笔 8 | ✅ |
| 7 | web 用户/角色管理页：「访问控制」菜单组（用户管理 users:admin、角色管理 roles:admin），用户 CRUD/改角色/改密，角色 CRUD/权限点矩阵编辑（内置角色只读展示） | 未开工 |
| 8 | web 页面写操作入口逐页裁剪：写按钮/危险操作包 `PermGuard`（v-perm 的 React 等价物），满足 viewer 无写入口验收 | 未开工 |

两个实现决策（设计阶段未写死，落地时定）：

- **内置角色 seed 覆盖策略**：已存在的 builtin 角色 permissions 以代码为准覆盖更新
  （内置角色用户不可改——D12，因此版本升级调整内置清单必须能生效）；自定义角色不动。
  稳态（角色已一致、无存量 user）零写——只读库 reopen（server 测试的 DB 错误注入
  手法）不触发写。
- **自定义角色删除**：被任何用户引用时拒绝删除（软引用删除会让这些用户 fail-closed
  全拒，等价于锁号）。
- **D13 用户侧分支的可达性**：到达用户管理 handler 的操作者持 `users:admin`、自身即
  有效 admin，「删/降最后一个有效 admin」排除目标后至少剩操作者——生产不可达。该分支
  保留为判定层变化时的防御纵深（测试直调覆盖）。角色侧（削自定义角色的 `users:admin`）
  真实可达：`roles:admin` 操作者可以不持 `users:admin`。

笔 2 追加的实现决策：

- **判定层形态**：不是逐 handler 包 requirePermission，而是全局 `RBACMiddleware`
  挂在 AuditMiddleware 内层（403 也进审计链）、各 auth 挂点外层。自身解析 Bearer
  取角色；无/坏 token 放行给内层 auth 出 401（认证优先于鉴权）。一处挂载，
  serveAPI 与十个独立注册点（docker/stacks/backups/probe/proxies/metrics/
  audit/remote/desktop/vnc）零改动，`/ws`（agent 通道，非用户请求）不在 `/api/`
  前缀自动豁免。
- **子路径归并**：probe/notification → `alerts`（拨测配置含告警阈值）、metrics →
  `inventory`、proxies → `proxy`、server-backups → `backup`、remote/desktop/vnc
  → `terminal`。
- **固定 action**：audit/logs 的动作全是读（导出/检索也是 POST，恒映射 `:read`）；
  terminal 只有 write（GET 列表同样按 `terminal:write` 门禁，D11 语义）。
- **acme 签发/部署**（POST `.../issue`、`.../deploy`）恒 `acme:admin`。
- **未登记 /api/ 路径放行**（handler 404 兜底）——缺省拒绝与缺省 404 无安全差，
  新端点忘登记不会把 404 变 403 暴露路径存在性。

## 不做的事

## 不做的事

- 不引入 casban/ory 类权限框架——依赖重量与本项目体量不匹配
- 不做行级权限（某个 agent 只对某部门可见）——现有部署是个人/小团队自托管，
  行级隔离等真实需求出现再议
- 不改 agent 协议——权限是 server 职责，agent 保持「被调用即执行」的信任模型
