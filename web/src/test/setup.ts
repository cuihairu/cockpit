import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// matchMedia：ProLayout/antd 响应式组件在 jsdom 缺此 API
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
})

// antd message/notification 渲染挂在 body，避免跨用例残留
afterEach(() => {
  cleanup()
  document.body.innerHTML = ''
})
