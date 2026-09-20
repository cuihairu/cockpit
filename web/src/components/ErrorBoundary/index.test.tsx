import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ErrorBoundary from './index'

// ErrorBoundary：捕获子树渲染错误显示降级 UI；恢复按钮重载页面

const Boom = ({ boom }: { boom?: boolean }) => {
  if (boom) throw new Error('kaboom')
  return <div>正常内容</div>
}

describe('ErrorBoundary', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('正常渲染 children', () => {
    render(<ErrorBoundary><Boom /></ErrorBoundary>)
    expect(screen.getByText('正常内容')).toBeInTheDocument()
  })

  it('子树抛错显示降级 UI 并记录日志', () => {
    const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    // jsdom location.reload 未实现，stub 掉
    vi.stubGlobal('location', { href: '', reload: vi.fn() })
    render(<ErrorBoundary><Boom boom /></ErrorBoundary>)
    expect(screen.getByText('页面出现错误')).toBeInTheDocument()
    expect(errSpy).toHaveBeenCalledWith(
      'Error Boundary caught an error:',
      expect.any(Error),
      expect.objectContaining({ componentStack: expect.any(String) }),
    )
  })

  it('点「刷新页面」调用 location.reload', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const reload = vi.fn()
    vi.stubGlobal('location', { href: '', reload })
    render(<ErrorBoundary><Boom boom /></ErrorBoundary>)
    screen.getByText('刷新页面').click()
    expect(reload).toHaveBeenCalledTimes(1)
  })
})
