# Cockpit Dashboard

基于 React + Ant Design + ProComponents 的 Web UI。

## 开发

```bash
# 安装依赖
pnpm install

# 启动开发服务器 (http://localhost:3000)
pnpm dev

# 构建生产版本
pnpm build
```

## 构建集成

构建后的文件会被 Go Server 嵌入并提供服务：

```bash
# 在项目根目录
cd web
pnpm build
cd ..
go build ./cmd/cockpit
./cockpit server start
# 访问 http://localhost:8080
```

## 技术栈

- **React 18** + **TypeScript**
- **Vite** - 构建工具
- **Ant Design 5** - UI 组件库
- **ProComponents** - 高级组件（ProTable、ProForm 等）
- **React Router** - 路由
- **TanStack Query** - 数据获取
- **Axios** - HTTP 客户端

## 目录结构

```
src/
├── components/     # 共享组件
├── pages/          # 页面组件
│   └── Dashboard/
├── services/       # API 服务
│   └── api.ts
├── types/          # TypeScript 类型定义
│   └── index.ts
├── App.tsx         # 根组件
├── main.tsx        # 入口
└── index.css       # 全局样式
```
