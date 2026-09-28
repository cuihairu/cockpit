# 真机验收辅助资产

真实 Docker 主机 / 真实通知渠道做验收时的可复用工具与夹具。
一次性产物（server 二进制、数据库、token、secrets、运行日志）不入库——
统一放 `.acceptance/`（已 gitignore）作为暂存区，验收完即清；
结论与关键证据摘录进 `todo.md` 对应条目与各设计文档「验收记录」。

- `webhook_receiver.py`：通知渠道验收用 webhook 接收器。用法
  `python3 scripts/acceptance/webhook_receiver.py <期望secret> <输出jsonl>`，
  监听 `127.0.0.1:9700/hook`，逐条记录时间戳/`X-Cockpit-Secret` 校验结果/正文，
  secret 不符回 401。服务健康探针真机验收（2026-09-28）即用它核对
  `service_health.*` 六类事件送达。
- `fixtures/`：stacks 真机验收 compose 夹具（均为 2026-09-28 实测通过的原文件）：
  - `demo-compose.yaml` + `demo.env`：端口/healthcheck/`unless-stopped`/env 替换/`profiles` 排除
  - `voldemo-compose.yaml`：命名卷跨容器共享 + `depends_on` 启动顺序
  - `netdemo-compose.yaml`：自定义 bridge 网络 + 服务间 DNS

背景：[stack-deploy-design](../docs/guide/stack-deploy-design.md)、
[service-health-design](../docs/guide/service-health-design.md)、
[acceptance-checklist](../docs/guide/acceptance-checklist.md)。
