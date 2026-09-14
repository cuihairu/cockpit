import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

function packageNameFromId(id: string) {
  const normalized = normalizeModuleId(id)
  const marker = '/node_modules/.pnpm/'
  const pnpmIndex = normalized.lastIndexOf(marker)

  if (pnpmIndex >= 0) {
    const afterStore = normalized.slice(pnpmIndex + marker.length)
    const nestedModules = '/node_modules/'
    const nestedIndex = afterStore.indexOf(nestedModules)
    if (nestedIndex >= 0) {
      const segments = afterStore.slice(nestedIndex + nestedModules.length).split('/')
      return segments[0].startsWith('@') ? `${segments[0]}/${segments[1]}` : segments[0]
    }
  }

  const nodeModulesIndex = normalized.lastIndexOf('/node_modules/')
  if (nodeModulesIndex < 0) {
    return ''
  }

  const segments = normalized.slice(nodeModulesIndex + '/node_modules/'.length).split('/')
  return segments[0].startsWith('@') ? `${segments[0]}/${segments[1]}` : segments[0]
}

function normalizeModuleId(id: string) {
  return id.split(path.sep).join('/')
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:9000',
        changeOrigin: true,
        ws: true,
      },
      '/ws': {
        target: 'ws://localhost:9000',
        ws: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1000,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            const normalized = normalizeModuleId(id)
            const packageName = packageNameFromId(id)
            if (packageName === 'react' || packageName === 'react-dom') {
              return 'vendor-react'
            }
            if (packageName.startsWith('react-router') || packageName === '@tanstack/react-query') {
              return 'vendor-router-query'
            }
            if (packageName === '@ant-design/icons') {
              return 'vendor-icons'
            }
            if (packageName.startsWith('@ant-design/pro') || packageName === '@umijs/route-utils') {
              return 'vendor-pro'
            }
            if (packageName.startsWith('@rc-component') || packageName.startsWith('rc-')) {
              return 'vendor-rc'
            }
            if (packageName.startsWith('@ant-design') || packageName === 'antd') {
              if (normalized.includes('/antd/es/table') || normalized.includes('/antd/lib/table')) {
                return 'vendor-antd-table'
              }
              if (normalized.includes('/antd/es/date-picker') || normalized.includes('/antd/lib/date-picker') || normalized.includes('/antd/es/calendar') || normalized.includes('/antd/lib/calendar')) {
                return 'vendor-antd-date'
              }
              if (normalized.includes('/antd/es/select') || normalized.includes('/antd/lib/select') || normalized.includes('/antd/es/tree-select') || normalized.includes('/antd/lib/tree-select')) {
                return 'vendor-antd-select'
              }
              if (normalized.includes('/antd/es/form') || normalized.includes('/antd/lib/form') || normalized.includes('/antd/es/input') || normalized.includes('/antd/lib/input')) {
                return 'vendor-antd-form'
              }
              if (normalized.includes('/antd/es/modal') || normalized.includes('/antd/lib/modal') || normalized.includes('/antd/es/dropdown') || normalized.includes('/antd/lib/dropdown')) {
                return 'vendor-antd-overlay'
              }
              return 'vendor-antd'
            }
            if (packageName.startsWith('@ant-design/charts') || packageName.startsWith('@antv')) {
              return 'vendor-chart'
            }
            if (packageName.startsWith('@xterm')) {
              return 'vendor-xterm'
            }
            if (packageName.startsWith('@novnc')) {
              return 'vendor-novnc'
            }
            if (packageName === 'zrender') {
              return 'vendor-zrender'
            }
            if (packageName === 'echarts') {
              if (normalized.includes('/echarts/lib/chart') || normalized.endsWith('/echarts/charts.js')) {
                return 'vendor-echarts-charts'
              }
              if (normalized.includes('/echarts/lib/component') || normalized.endsWith('/echarts/components.js')) {
                return 'vendor-echarts-components'
              }
              if (normalized.includes('/echarts/lib/coord')) {
                return 'vendor-echarts-coord'
              }
              return 'vendor-echarts-core'
            }
          }
        },
      },
    },
  },
})
