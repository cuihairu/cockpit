# Cockpit Web UI

基于 React、TypeScript、Vite、Ant Design 和 ProComponents 的前端应用。

## 开发

```bash
pnpm install
pnpm dev
```

开发服务器默认监听 `http://localhost:3000`。Vite 会把以下路径代理到本地 Server：

- `/api` -> `http://localhost:9000`
- `/ws` -> `ws://localhost:9000`

因此本地开发时需要先启动后端：

```bash
export ADMIN_PASSWORD='change-this-password'
./cockpit server -config config/cockpit.yaml
```

Windows PowerShell：

```powershell
$env:ADMIN_PASSWORD = 'change-this-password'
.\cockpit.exe server -config config\cockpit.yaml
```

## 构建

```bash
pnpm build
```

构建产物输出到 `web/dist`。Server 可通过以下任一方式提供静态文件：

- 配置 `server.static_dir: ./web/dist`
- 设置环境变量 `STATIC_DIR=./web/dist`

## 技术栈

- React 19
- TypeScript
- Vite
- Ant Design 5
- ProComponents / ProLayout
- React Router
- TanStack Query
- Axios
- xterm.js / noVNC / ECharts

## 目录结构

```text
src/
  assets/
  components/
  contexts/
  hooks/
  pages/
  services/
  types/
  utils/
  workbench/
  App.tsx
  main.tsx
```

前端只通过 Server API 通信，不直接访问 Agent 或内网目标。
