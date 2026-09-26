import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

// 前端单测/覆盖率（vitest + jsdom）。与 vite.config.ts 共用 @ 别名。
// 覆盖率阈值随轮次推进逐步上调，目标 98%（见 docs/guide/testing.md）。
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    // 68 文件全并行在 16 核上抢 CPU，antd 插桩首渲染会被拖到超时；
    // 限半数 worker 换稳定性（总时长接近，flaky 归零）
    maxWorkers: 8,
    // coverage 插桩后 antd 组件首渲染显著变慢（默认 5s 会超时）；
    // 插桩 + 8 worker 下 Backups 重表单用例实测 63s+，放宽到 120s；
    // 2026-09-26 外部高负载（load 50-180/16 核）下同文件不同用例先后
    // 击穿 120s（单跑均绿，纯时序 flake），按 M7 先例再放宽到 300s——
    // 超时上限只影响失败判定等待，不影响断言语义
    testTimeout: 300000,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/main.tsx',
        'src/test/**',
        'src/**/*.d.ts',
        // 类型定义与纯数据常量无逻辑分支
        'src/types/**',
      ],
    },
  },
})
