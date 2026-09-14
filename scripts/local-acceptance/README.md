# Compose Stack 本地验收（无真实 Docker）

在不装 Docker 的机器上对 Compose Stack 功能（P1/M1）做端到端验收：
真实 `cockpit` server 二进制 + 真实 `cockpit-agent` 二进制，Docker 侧由两个
替身顶上——unix socket 上的最小 Docker Engine API（`fake-docker-daemon.go`）
与 PATH 内的 compose CLI 替身（`fake-docker`）。

```bash
bash scripts/local-acceptance/run-acceptance.sh
```

脚本自建临时目录（数据库、stacks 目录、socket、日志），跑完自动清理；
端口默认 18099，可用 `ACCEPTANCE_PORT=xxx` 覆盖。

## 替身行为

| 组件 | 行为 |
|------|------|
| `fake-docker-daemon` | 监听 `$FAKE_DOCKER_SOCK`，实现 `/_ping`、`/version`、`/info`、`/containers/json`（汇总状态目录里的 `<project>.json`） |
| `fake-docker` | 拦截 `docker compose version/config -q/up -d/down/logs`；`up` 先睡 `FAKE_COMPOSE_UP_DELAY`（默认 2s，制造并发 409 窗口）再解析 compose 顶层 services 写状态文件，`down` 删除状态文件——`stack.status` 与 compose 动作真实联动 |

## 断言范围（32 项）

登录 → agent 注册（`docker-api` capability）→ 创建 stack（`created=true`、
compose/.env 回读一致）→ 非法 compose 校验失败不落盘 → 列表 → `up` 返回
taskId → 同名并发 `up` 409 → 任务轮询 success → status 联动（running/total）
→ logs → `down` 归零 → `remove` 删目录 → agent 断连后缓存灰态
（`online=false`）→ 审计事件齐全且 `.env` 内容不泄漏。

## 与真实 Docker 验收的边界

替身不覆盖：镜像真实拉取、容器健康/端口/重启策略、compose 的
build/pull/profiles/healthcheck 语义、`COCKPIT_STACKS_DIR` 权限实践
（建议 0700）。这些留给有 Docker 的测试机，清单见
`docs/guide/stack-deploy-design.md` 的 M1 验收小节。
