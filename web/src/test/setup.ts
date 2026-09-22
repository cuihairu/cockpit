import '@testing-library/jest-dom/vitest'
import { afterEach } from 'vitest'
import { cleanup, configure } from '@testing-library/react'

// antd 表单校验消息经 async-validator + 过渡动画后才进 DOM，
// 默认 1s 异步超时在高并发下不够，统一放宽到 15s。
configure({ asyncUtilTimeout: 15000 })

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

// jsdom 不支持带伪元素的 getComputedStyle（antd TextArea resize 探测会调），
// 回退到无伪元素签名，避免 virtual console 刷屏。
const nativeGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = ((el: Element, pseudo?: string | null) =>
  pseudo ? { resize: 'none' } as CSSStyleDeclaration : nativeGetComputedStyle(el)) as typeof window.getComputedStyle

// antd message/notification 渲染挂在 body，避免跨用例残留
afterEach(async () => {
  cleanup()
  document.body.innerHTML = ''
  // React 19 scheduler 的 setImmediate 在 jsdom teardown 后仍可能触发一次
  // 调度（window is not defined）；让出一轮宏任务让 pending 工作先落定。
  await new Promise((r) => setTimeout(r, 0))
})
