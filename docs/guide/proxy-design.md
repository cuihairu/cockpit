# P1 方案设计：反向代理管理（Nginx 站点可视化下发）

> 2026-09-15。P1「反向代理/路由管理」首期设计。todo.md 原文：「Nginx/Caddy/Traefik
> 配置可视化与下发，把『网关』资源从记录升级为可管理对象」。本文只做设计决策与接口定义。

## 现状与痛点

| 痛点 | 现状 | 后果 |
|------|------|------|
| 加一个站点要 SSH + 手写 server 块 | 复制旧配置改域名，`nginx -t` + reload | 格式/转义错一次，全站 502 |
| 站点现状无处可查 | 域名→上游映射只在运维者脑子里 | 半年后不知道哪个端口是什么服务 |
| 改错配置影响面大 | 直接改生产 conf，坏了手忙脚乱回滚 | reload 失败期间用户报障 |
| Gateway 资源只是记录 | inventory 的 Gateway 是网络设备档案，与 Nginx 反代无关 | 「可管理」无从谈起 |

## 架构总览

与文件管理器同一通道模型：**同步 RPC + server 纯转发**。Nginx 配置片段是 KB 级文本，
`nginx -t` + reload 秒级完成，30s RPC 超时绰绰有余，不需要异步任务模型。

```
┌─ server ─────────────────────┐        ┌─ agent（nginx 所在主机）────────┐
│ REST /api/agents/{id}/proxy  │ ─RPC─▶ │ nginx provider（新）             │
│ 站点 CRUD + 审计             │ ◀─RPC─ │ 渲染 → nginx -t → 写文件 → reload│
│ 不落库：文件即状态           │        │ 只管 cockpit-*.conf，其余零接触  │
└──────────────────────────────┘        └─────────────────────────────────┘
```

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | M1 后端范围 | 仅 **Nginx**（宿主机裸装/systemd）；Caddy/Traefik 后续独立扩展 | 个人云最普遍；`nginx -t` 校验与平滑 reload 成熟；容器化 nginx 的配置挂载路径复杂，后补 |
| D2 | 管理边界 | cockpit 只管理 `/etc/nginx/conf.d/cockpit-*.conf` 自己名下的片段，**用户已有配置零接触** | 不解析任意 nginx 语法（正则解析 server 块是出了名的脆弱）；从根上不可能弄坏既有站点 |
| D3 | 站点元数据 | 文件头 `# cockpit:meta {json}` 内嵌，读取时只解析自己写的文件 | 回读自己渲染的产物无需真正解析 nginx 语法，roundtrip 可靠 |
| D4 | 应用流程 | 渲染 → `nginx -t` → 写文件 → reload；**`nginx -t` 失败不落盘**；reload 失败自动回滚文件再试一次 | 语法错误绝不落盘；「apply 失败站点照旧」的强保证 |
| D5 | reload 方式 | 优先 `systemctl reload nginx`，失败 fallback `nginx -s reload` | systemd 环境 service 语义正确；裸装/容器内无 systemd 走信号 |
| D6 | 执行模型 | 全同步 RPC | 配置片段小、命令秒级，异步任务模型是过度设计 |
| D7 | 站点模型 | `{name, serverNames[], upstream, scheme(http/https), tlsCert, tlsKey, websocket, extra}` | 覆盖个人云 90% 场景（反代 + WS + 证书）；非常规指令经 `extra` 直通，`nginx -t` 兜底 |
| D8 | capability | `nginx-proxy`，`exec.LookPath("nginx")` + `nginx -v` 探测；仅检测通过的 agent 注册 provider 并上报 capability | 无 nginx 的主机不出现假能力 |
| D9 | REST 挂载 | `/api/agents/{id}/proxy/...`，复用文件管理器的 serveAPI `/agents/` 分发模式 | 按 agent 作用域，一机一 nginx |
| D10 | 持久化 | **不落库**——站点状态以 agent 侧文件为唯一事实源，server 纯转发 + 审计 | 无 server/agent 状态同步问题；apply/delete 记审计即可追溯 |

## Agent 侧设计

### 探测与注册

- `detector` 不动——`internal/agent/agent.go` 探测函数：`exec.LookPath("nginx")` 通过
  后跑 `nginx -v 2>&1` 取版本，成功则追加 `nginx-proxy` capability
  （metadata: `{version, confDir}`）；
- `providers.go`：按检测到的 `nginx-proxy` capability 注册 `NewNginxProvider`；
  conf 目录可用 `COCKPIT_NGINX_CONF_DIR` 覆盖（默认 `/etc/nginx/conf.d`，
  兼容 sites-enabled 风格的发行版自行配置）。

### RPC 方法

| 方法 | 参数 | 返回 | 说明 |
|------|------|------|------|
| `proxy.status` | `{}` | `{installed, version, confDir, siteCount, reloadMode}` | 概览；reloadMode = systemctl / signal |
| `proxy.sites` | `{}` | `{sites: [{name, serverNames, upstream, scheme, websocket, updatedAt}]}` | 列 cockpit 名下站点（解析 meta） |
| `proxy.site.get` | `{name}` | `{name, meta, content}` | 查看渲染后的配置全文 |
| `proxy.site.apply` | `{site}` | `{name, file}` | 校验 → `nginx -t` → 写 → reload（失败回滚） |
| `proxy.site.delete` | `{name}` | `{}` | 删片段文件 → reload；reload 失败恢复文件 |

### 站点参数与校验（双端同规则）

| 字段 | 规则 |
|------|------|
| `name` | `^[a-z0-9][a-z0-9_-]{0,63}$`（backupNameRe 同款），文件名 `cockpit-site-<name>.conf` 的一部分 |
| `serverNames` | 1–16 个，每个匹配 `^[A-Za-z0-9*.\-]{1,253}$`（域名/通配符） |
| `upstream` | `^[A-Za-z0-9.:\-]{1,253}$`，形如 `127.0.0.1:3000` |
| `scheme` | `http` / `https` |
| `tlsCert` / `tlsKey` | scheme=https 时必填，绝对路径；**上传证书走文件管理器**（file.write），职责不重复 |
| `websocket` | bool；true 时渲染 Upgrade/Connection 头 + `proxy_http_version 1.1` |
| `extra` | 0–4KB 自由文本，原样插入 server 块尾部（高级用户直通口） |

### 渲染与应用流程

```
render(site) → 片段文本（首行 meta 注释）
  ↓
nginx -t                      失败 → 返回 stderr 摘要，不落盘
  ↓
写 cockpit-site-<name>.conf   旧文件内容先读入内存
  ↓
reload                        失败 → 回滚旧内容（或删除新文件）→ 再 reload → 返回错误
```

- `nginx -t`：`exec.CommandContext(ctx, "nginx", "-t")`，10s 超时，stderr 全文截断
  2KB 返回给 UI；
- reload：`systemctl reload nginx`（5s 超时）失败 → `nginx -s reload`；两者皆失败按
  reload 失败处理（回滚）；
- meta 注释：单行压缩 JSON，`# cockpit:meta {...}`；不含敏感信息（证书是路径引用）。

### 渲染模板（http 站点示例）

```nginx
# cockpit:meta {"name":"blog","serverNames":["blog.example.com"],...}
server {
    listen 80;
    server_name blog.example.com;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

- https：`listen 443 ssl` + `ssl_certificate/-key`，并渲染第二个 server 块
  做 80 → https 301 跳转；
- websocket：location 内追加 `proxy_http_version 1.1` + Upgrade/Connection 头；
- `extra` 原样追加在 location 块之后、server 块内。

## Server 侧设计

### REST API（server/api_proxy_sites.go，挂 serveAPI `/agents/` 分支，JWT）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/api/agents/{id}/proxy/status` | 转发 proxy.status |
| GET  | `/api/agents/{id}/proxy/sites` | 转发 proxy.sites |
| GET  | `/api/agents/{id}/proxy/sites/{name}` | 转发 proxy.site.get（含配置全文） |
| PUT  | `/api/agents/{id}/proxy/sites/{name}` | body 为站点参数 → proxy.site.apply + 审计 proxy_apply |
| DELETE | `/api/agents/{id}/proxy/sites/{name}` | proxy.site.delete + 审计 proxy_delete |

- 参数校验 server 侧执行与 agent 相同规则（双端防御）；`nginx -t` 的 stderr 摘要
  原样回给 UI（这是用户修正输入的关键信息）；
- 审计：`ResourceProxySite = "proxy_site"`，动作 proxy_apply / proxy_delete；
  detail 记 name/domain/upstream，**不含证书内容**（只有路径）。

## Web UI 设计

- 新页面 `/proxy`「反向代理」（菜单，ArrowRightOutlined）：
  - **Agent 选择**：下拉，标注 nginx 版本（无 capability 的 agent 禁选并提示）；
  - **状态卡**：nginx 版本 / 配置目录 / 站点数 / reload 方式；
  - **站点列表** Table：名称 / 域名（Tag）/ 上游 / 协议（http / https Tag）/
    WebSocket / 更新时间 / 操作（配置、编辑、删除 Popconfirm）；
  - **新建/编辑 Modal**：名称、域名（tags 多值）、上游、协议 Radio、
    证书路径/私钥路径（https 时出现，提示「证书文件可经工作台-文件上传」）、
    WebSocket 开关、高级指令 TextArea；保存失败展示 nginx -t 错误摘要；
  - **配置查看 Modal**：只读 pre 展示渲染后的完整片段。

## M2：Traefik 后端（设计，2026-09-18）

D1 承诺的后端扩展。**M2 仅 Traefik，且只走 file provider 动态目录**——
`providers.file.directory` 指向的目录里每个 YAML 文件是一段独立动态配置，
与 nginx `conf.d` 片段模式同构，D2「自己名下片段、用户配置零接触」原样
成立。Caddy 暂缓：Caddyfile 是整文件语义，按站点分片必须改用户主文件加
`import` 行，与零接触原则冲突（见不做清单）。

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D11 | 后端范围 | 仅 Traefik file provider；渲染/校验/应用按 backend 抽象（`SiteRenderer`/`SiteApplier`），RPC 方法名 `proxy.*` 不变，provider 持有 backend 实现分发 | 方法表不动 server 与 web 路由；nginx 行为零变化 |
| D12 | 目录探测与 capability | 动态目录取值：静态配置 `/etc/traefik/traefik.yml`（或 `.yaml`）解析 `providers.file.directory` → 缺省 `/etc/traefik/dynamic`；capability `traefik-proxy` = 目录存在且可写。**不依赖 LookPath("traefik")** | Traefik 大多容器化跑，宿主机常无二进制；目录（含 docker 挂载的宿主侧路径）才是事实源；版本探测失败仅置 version 空 |
| D13 | 校验与应用 | 无 `nginx -t`/reload 等价物：渲染后 `yaml.Unmarshal` 语法自检 + router→service 引用一致性校验，**校验失败不落盘**；file provider 热加载（watch），写坏文件由 Traefik 拒载该文件、其余片段照常（局部隔离） | 比 nginx 弱在无全局预检（坏文件只影响本站点）、强在无 reload 失败回滚分支；apply 流程退化为 渲染→自检→原子写 |
| D14 | 渲染映射 | 同站点模型（D7 字段不变）：`serverNames` → router rule 的 `Host(...)` 多值；`upstream` → service `loadBalancer.servers[].url`；https → 443 router `tls=true` + 文件级 `tls.certificates`（certFile/keyFile 路径引用，ACME 推送文件直引）+ 80 router 挂 `redirectScheme` 中间件永久跳转；**`websocket` 字段 no-op**（Traefik 原生透传 WS，字段保留兼容面板）；**`extra` 不支持**——非空时校验直接拒绝 | 跳转按站点双 router 而非静态 entrypoint 配置（不碰静态配置=零接触）；拒绝 extra 避免渲染任意 YAML 片段的注入面 |
| D15 | 元数据与 drift | meta 首行 YAML 注释 `# cockpit:meta {json}` 与 nginx 同构；drift kind 白名单扩 `traefik`（`dynamicDir/cockpit-site-<name>.yml`，读文件即 current），drift 页 KIND_LABEL 加标签 | BaselineRecorder/diff 链路通用，仅扩 snapshot 分支与白名单 |
| D16 | server / web | server 纯转发不感知 backend，零改动；web 状态卡显示 backend 名称与版本（无二进制则「-」），编辑 Modal 的 extra 控件在 traefik 后端禁用并提示不支持，其余 UI 复用 | 后端差异收敛在 agent 渲染层与两处 UI 提示 |

渲染片段示例（https 站点）：

```yaml
# cockpit:meta {"name":"blog","serverNames":["blog.example.com"],...}
http:
  routers:
    cockpit-blog-web:
      rule: "Host(`blog.example.com`)"
      entryPoints: ["web"]
      middlewares: ["cockpit-blog-redirect"]
      service: cockpit-blog
    cockpit-blog-websecure:
      rule: "Host(`blog.example.com`)"
      entryPoints: ["websecure"]
      service: cockpit-blog
      tls: {}
  middlewares:
    cockpit-blog-redirect:
      redirectScheme:
        scheme: https
        permanent: true
  services:
    cockpit-blog:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:3000"
tls:
  certificates:
    - certFile: /path/fullchain.pem
      keyFile: /path/privkey.pem
```

router/service 命名 `cockpit-<site>`（冲突域在 Traefik 全局命名空间，
加前缀避免与用户动态文件撞名）。

### M2 清单（未实施）

- [ ] agent：`traefik_provider.go`（探测/渲染/yaml 自检/原子写）+ backend 抽象
      + capability 追加；drift 白名单扩 traefik
- [ ] server：零改动（验证现有 5 端点对 traefik agent 透传）
- [ ] web：状态卡 backend 显示 + extra 禁用
- [ ] 测试：渲染模板各形态（http/https+跳转/证书引用）、yaml 自检失败不落盘、
      meta roundtrip、extra 拒绝、目录探测（自定义/缺省/不可写）、drift 分支
- [ ] 真机验收（列入 todo.md）：Traefik 容器挂载宿主目录实测热加载与
      证书文件引用

## 不做（后续项）

- Caddy（Caddyfile 渲染）：Caddyfile 整文件语义，按站点分片必须改用户主文件加
  import 行，与 D2 零接触冲突；除非有真实需求且接受一次性主文件协作，暂缓；
- 流量统计 / 访问日志分析：需要日志采集管道，独立立项；
- 证书签发与续期联动：P1 证书资源与 acme 集成后一起做；
- upstream 健康检查 / 负载均衡多后端：个人场景单上游先够用；
- 容器化 nginx：配置挂载路径发现复杂，等真实需求。

## M1 清单

- [x] agent：`internal/agent/rpc/nginx_provider.go`（探测/渲染/nginx -t/写/reload/回滚）
      + capability 追加 + 按能力注册
- [x] server：`api_proxy_sites.go`（5 端点 + 双端参数校验 + 审计）+ serveAPI 接入
      + audit 常量
- [x] web：`/proxy` 页面（agent 选择/状态卡/站点列表/编辑 Modal/配置查看）+ 路由菜单
- [x] 测试：渲染模板各形态（http/https+跳转/ws/extra）、参数校验、meta roundtrip、
      nginx -t 失败不落盘、reload 失败回滚、server 转发与审计
- [x] 文档收尾 + todo.md 同步

## 参考

- 内部：[file-manager-design.md](./file-manager-design.md)（/agents/ 分发与证书上传通道）、
  [stack-deploy-design.md](./stack-deploy-design.md)（命令探测与 provider 注册）、
  `todo.md` P1 反向代理条目
- 外部：Nginx 官方 docs（proxy_set_header 惯例组合）、
  [nginxconfig.io](https://nginxconfig.io/)（渲染模板的参数化思路）
