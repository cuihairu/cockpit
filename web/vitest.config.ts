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
    // coverage 插桩后 antd 组件首渲染显著变慢（默认 5s 会超时）
    testTimeout: 20000,
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
