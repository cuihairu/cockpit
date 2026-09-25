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

// ResizeObserver：jsdom 无实现，GuacamoleModal 的 SSH 尺寸同步（窗口变形 →
// size 指令 → guacd 换算列/行）依赖它。注册可观测桩：observe 的目标与回调记
// 入 __resizeObservers，测试用 window.__triggerResize(w, h) 手动触发。
interface RoStub {
  callback: ResizeObserverCallback
  targets: Set<Element>
  disconnected: boolean
}
const resizeObservers: RoStub[] = []
class ResizeObserverStub {
  private entry: RoStub
  constructor(callback: ResizeObserverCallback) {
    this.entry = { callback, targets: new Set(), disconnected: false }
    resizeObservers.push(this.entry)
  }
  observe(el: Element) {
    this.entry.targets.add(el)
  }
  unobserve(el: Element) {
    this.entry.targets.delete(el)
  }
  disconnect() {
    this.entry.targets.clear()
    this.entry.disconnected = true
  }
}
// configurable：TerminalModal 等测试文件自己 vi.stubGlobal('ResizeObserver')，
// 桩若不可配置会把它们顶掉（Cannot redefine property）
Object.defineProperty(window, 'ResizeObserver', {
  writable: true,
  configurable: true,
  value: ResizeObserverStub,
})
Object.defineProperty(window, '__resizeObservers', {
  writable: true,
  configurable: true,
  value: resizeObservers,
})
Object.defineProperty(window, '__triggerResize', {
  writable: true,
  configurable: true,
  value: (width: number, height: number) => {
    let fired = 0
    for (const ro of resizeObservers) {
      if (ro.disconnected) continue
      for (const target of ro.targets) {
        ro.callback(
          [{ target, contentRect: { width, height } } as unknown as ResizeObserverEntry],
          {} as ResizeObserver,
        )
        fired += 1
      }
    }
    return fired
  },
})
afterEach(() => {
  resizeObservers.length = 0
})

// antd message/notification 渲染挂在 body，避免跨用例残留
afterEach(async () => {
  cleanup()
  document.body.innerHTML = ''
  // React 19 scheduler 的 setImmediate 在 jsdom teardown 后仍可能触发一次
  // 调度（window is not defined）；让出一轮宏任务让 pending 工作先落定。
  await new Promise((r) => setTimeout(r, 0))
})
