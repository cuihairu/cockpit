import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

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
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules')) {
            if (id.includes('react-dom') || id.includes('react-router') || id.endsWith('react.js')) {
              return 'vendor-react'
            }
            if (id.includes('@ant-design') || id.includes('antd') || id.includes('@rc-component')) {
              return 'vendor-antd'
            }
            if (id.includes('@ant-design/charts') || id.includes('@antv')) {
              return 'vendor-chart'
            }
            if (id.includes('@xterm')) {
              return 'vendor-xterm'
            }
            if (id.includes('@novnc')) {
              return 'vendor-novnc'
            }
            if (id.includes('echarts') || id.includes('zrender')) {
              return 'vendor-echarts'
            }
          }
        },
      },
    },
  },
})
