import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { useConnectionTimeout } from './useConnectionTimeout'
import { usePerm } from './usePerm'
import { UserProvider } from '@/contexts/UserContext'
import type { UserInfo } from '@/types'

// usePerm：基于 UserContext 的 permissions 判定（fail-closed——
// undefined 恒 false）。useConnectionTimeout：计时器启停语义。

const apiMock = vi.hoisted(() => ({
  login: vi.fn(),
  getCurrentUser: vi.fn(),
  logout: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const me = (permissions: string[]): UserInfo => ({
  id: 'u1', username: 'admin', email: 'a@b.c', role: 'admin', permissions, totp_enabled: false,
})

describe('usePerm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  const Probe = ({ need }: { need: string | string[] }) => {
    const ok = usePerm(need)
    return <span data-testid="perm">{String(ok)}</span>
  }

  it('未登录 / permissions 未加载恒 false（fail-closed）', () => {
    render(
      <UserProvider>
        <Probe need="dns:write" />
      </UserProvider>,
    )
    expect(screen.getByTestId('perm').textContent).toBe('false')
  })

  it('拉取失败 permissions 缺失保持 false', async () => {
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'admin')
    apiMock.getCurrentUser.mockRejectedValue(new Error('down'))
    render(
      <UserProvider>
        <Probe need="dns:write" />
      </UserProvider>,
    )
    // 等启动拉取结束（ready 置位后仍无 permissions）
    await waitFor(() => expect(apiMock.getCurrentUser).toHaveBeenCalled())
    expect(screen.getByTestId('perm').textContent).toBe('false')
  })

  it('有权限 true；write 隐含 read', async () => {
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'admin')
    apiMock.getCurrentUser.mockResolvedValue(me(['dns:write', 'proxy:write']))
    render(
      <UserProvider>
        <Probe need="dns:write" />
      </UserProvider>,
    )
    await waitFor(() => {
      expect(screen.getByTestId('perm').textContent).toBe('true')
    })
  })

  it('AND 数组缺一 false；无权限 false', async () => {
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'admin')
    apiMock.getCurrentUser.mockResolvedValue(me(['dns:write']))
    render(
      <UserProvider>
        <Probe need={['dns:write', 'proxy:write']} />
      </UserProvider>,
    )
    await waitFor(() => {
      expect(screen.getByTestId('perm').textContent).toBe('false')
    })
  })
})

describe('useConnectionTimeout', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  const onTimeout = vi.fn()
  const Probe = ({ enabled, timeout }: { enabled: boolean; timeout: number }) => {
    const { start, clear, reset } = useConnectionTimeout({ timeout, onTimeout, enabled })
    return (
      <div>
        <button onClick={start}>start</button>
        <button onClick={clear}>clear</button>
        <button onClick={reset}>reset</button>
      </div>
    )
  }

  it('start 到时触发 onTimeout；clear 中途取消', () => {
    onTimeout.mockClear()
    render(<Probe enabled timeout={1000} />)
    act(() => {
      fireEvent.click(screen.getByText('start'))
    })
    act(() => {
      vi.advanceTimersByTime(999)
    })
    expect(onTimeout).not.toHaveBeenCalled()
    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(onTimeout).toHaveBeenCalledTimes(1)

    // 再次 start 重计时，clear 中途取消
    act(() => {
      fireEvent.click(screen.getByText('start'))
      vi.advanceTimersByTime(500)
      fireEvent.click(screen.getByText('clear'))
      vi.advanceTimersByTime(5000)
    })
    expect(onTimeout).toHaveBeenCalledTimes(1)
  })

  it('start 重复调用重置计时，不叠加触发', () => {
    onTimeout.mockClear()
    render(<Probe enabled timeout={1000} />)
    act(() => {
      fireEvent.click(screen.getByText('start'))
      vi.advanceTimersByTime(600)
      fireEvent.click(screen.getByText('start'))
      vi.advanceTimersByTime(600)
    })
    // 第二次 start 清掉首个计时器，1000ms 内不应触发
    expect(onTimeout).not.toHaveBeenCalled()
    act(() => {
      vi.advanceTimersByTime(400)
    })
    expect(onTimeout).toHaveBeenCalledTimes(1)
  })

  it('enabled=false 时 reset 只清不启', () => {
    onTimeout.mockClear()
    render(<Probe enabled={false} timeout={100} />)
    act(() => {
      fireEvent.click(screen.getByText('reset'))
      vi.advanceTimersByTime(5000)
    })
    expect(onTimeout).not.toHaveBeenCalled()
  })

  it('enabled 时 reset 等价重启到时触发', () => {
    onTimeout.mockClear()
    render(<Probe enabled timeout={100} />)
    act(() => {
      fireEvent.click(screen.getByText('reset'))
      vi.advanceTimersByTime(150)
    })
    expect(onTimeout).toHaveBeenCalledTimes(1)
  })

  it('卸载清理计时器', () => {
    onTimeout.mockClear()
    const { unmount } = render(<Probe enabled timeout={100} />)
    act(() => {
      fireEvent.click(screen.getByText('start'))
    })
    unmount()
    act(() => {
      vi.advanceTimersByTime(5000)
    })
    expect(onTimeout).not.toHaveBeenCalled()
  })
})
