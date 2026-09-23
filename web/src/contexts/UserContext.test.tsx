import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { UserProvider } from './UserContext'
import { useUser } from './useUser'
import { TOTPRequiredError } from './userTypes'
import type { User } from './userTypes'
import type { UserInfo } from '@/types'

// api 模块全 mock：login/getCurrentUser 行为按用例设定
const apiMock = vi.hoisted(() => ({
  login: vi.fn(),
  getCurrentUser: vi.fn(),
  logout: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const me: UserInfo = {
  id: 'u1', username: 'admin', email: 'a@b.c', role: 'admin', permissions: ['users:admin'], totp_enabled: false,
}

// 探针：渲染 context 状态；login 失败信息落 err 标记（供断言异常分支）
const Probe = () => {
  const { user, token, permissionsReady, login, logout, updateUser } = useUser()
  const [err, setErr] = useState('')
  return (
    <div>
      <span data-testid="user">{user ? `${user.username}:${user.permissions?.length ?? 'undef'}` : 'null'}</span>
      <span data-testid="token">{token ?? 'null'}</span>
      <span data-testid="ready">{String(permissionsReady)}</span>
      <span data-testid="err">{err}</span>
      <button
        onClick={() => {
          login('admin', 'pw').catch((e: unknown) => {
            setErr(e instanceof TOTPRequiredError ? 'TOTP' : String((e as Error).message))
          })
        }}
      >
        login
      </button>
      <button onClick={logout}>logout</button>
      <button
        onClick={() =>
          updateUser({
            id: 'u2', username: 'admin', role: 'user',
            email: 'new@x.y', phone: '', department: '  ',
          } as User)
        }
      >
        update
      </button>
    </div>
  )
}

describe('UserContext', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('useUser 在 Provider 外抛错', () => {
    const Outside = () => {
      useUser()
      return null
    }
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<Outside />)).toThrow('useUser must be used within a UserProvider')
    spy.mockRestore()
  })

  it('无 token：user null、permissionsReady true（免拉取即绪）', () => {
    render(<UserProvider><Probe /></UserProvider>)
    expect(screen.getByTestId('user').textContent).toBe('null')
    expect(screen.getByTestId('ready').textContent).toBe('true')
  })

  it('getStoredUser：缺 username 视为未登录；缺 userId/role 走缺省', async () => {
    // 有 token 无 username：user 为 null，/api/me 走 prev 空分支展开完整 user
    localStorage.setItem('token', 'tk')
    apiMock.getCurrentUser.mockResolvedValue({ ...me, permissions: undefined })
    render(<UserProvider><Probe /></UserProvider>)
    await waitFor(() => {
      expect(screen.getByTestId('user').textContent).toBe('admin:0')
      expect(screen.getByTestId('ready').textContent).toBe('true')
    })
  })

  it('启动拉取：已有存量 user 时合并 /api/me，permissions 空回落 []', async () => {
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'admin')
    apiMock.getCurrentUser.mockResolvedValue({ ...me, permissions: undefined })
    render(<UserProvider><Probe /></UserProvider>)
    // prev 非空合并分支 + permissions ?? [] 右侧
    await waitFor(() => {
      expect(screen.getByTestId('user').textContent).toBe('admin:0')
    })
  })

  it('getStoredUser：可选字段有值与缺省（id/role/email/phone/department）', async () => {
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'bob')
    // 无 userId → id 空串；无 role → 'user'；无 email/phone/department → undefined
    apiMock.getCurrentUser.mockRejectedValue(new Error('down'))
    const { unmount } = render(<UserProvider><Probe /></UserProvider>)
    await waitFor(() => expect(screen.getByTestId('ready').textContent).toBe('true'))
    unmount()

    localStorage.clear()
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'bob')
    localStorage.setItem('userId', 'u9')
    localStorage.setItem('role', 'admin')
    localStorage.setItem('email', 'b@x.y')
    localStorage.setItem('phone', '123')
    localStorage.setItem('department', 'ops')
    apiMock.getCurrentUser.mockRejectedValue(new Error('down'))
    render(<UserProvider><Probe /></UserProvider>)
    await waitFor(() => expect(screen.getByTestId('ready').textContent).toBe('true'))
  })

  it('启动拉取中途卸载：cancelled 短路 then/finally', async () => {
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'admin')
    let resolveMe: (v: UserInfo) => void = () => {}
    apiMock.getCurrentUser.mockImplementation(() => new Promise((r) => { resolveMe = r }))
    const { unmount } = render(<UserProvider><Probe /></UserProvider>)
    unmount()
    resolveMe(me)
    await waitFor(() => expect(apiMock.getCurrentUser).toHaveBeenCalled())
  })

  it('有 token 启动拉 /api/me 展开 permissions', async () => {
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'admin')
    localStorage.setItem('role', 'admin')
    apiMock.getCurrentUser.mockResolvedValue(me)
    render(<UserProvider><Probe /></UserProvider>)
    await waitFor(() => {
      expect(screen.getByTestId('user').textContent).toBe('admin:1')
      expect(screen.getByTestId('ready').textContent).toBe('true')
    })
  })

  it('启动拉取失败静默：ready 置位、permissions 保持旧态', async () => {
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'admin')
    apiMock.getCurrentUser.mockRejectedValue(new Error('down'))
    render(<UserProvider><Probe /></UserProvider>)
    await waitFor(() => {
      expect(screen.getByTestId('ready').textContent).toBe('true')
    })
    // 存量 user（username 无 permissions）保留，fail-closed 显示 undef
    expect(screen.getByTestId('user').textContent).toBe('admin:undef')
  })

  it('login：TOTP 要求抛 TOTPRequiredError、不落 token', async () => {
    apiMock.login.mockResolvedValue({ requires_totp: true })
    render(<UserProvider><Probe /></UserProvider>)
    fireEvent.click(screen.getByText('login'))
    await waitFor(() => {
      expect(screen.getByTestId('err').textContent).toBe('TOTP')
    })
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('login：无 token 抛错；正常登录落五键并拉权限', async () => {
    apiMock.login.mockResolvedValueOnce({ requires_totp: false }) // 无 token 分支
    render(<UserProvider><Probe /></UserProvider>)
    fireEvent.click(screen.getByText('login'))
    await waitFor(() => {
      expect(screen.getByTestId('err').textContent).toBe('No token returned from login')
    })

    apiMock.login.mockResolvedValue({
      token: 't1', user_id: 'u1', username: 'admin', role: 'admin',
    })
    apiMock.getCurrentUser.mockResolvedValue(me)
    fireEvent.click(screen.getByText('login'))
    await waitFor(() => {
      expect(screen.getByTestId('user').textContent).toBe('admin:1')
      expect(screen.getByTestId('token').textContent).toBe('t1')
      expect(screen.getByTestId('ready').textContent).toBe('true')
    })
    expect(localStorage.getItem('token')).toBe('t1')
    expect(localStorage.getItem('userId')).toBe('u1')
    expect(localStorage.getItem('username')).toBe('admin')
  })

  it('login：缺 role 落 user、清理旧可选字段；/api/me 失败静默', async () => {
    localStorage.setItem('email', 'old@x.y')
    localStorage.setItem('phone', 'old')
    localStorage.setItem('department', 'old')
    apiMock.login.mockResolvedValue({
      token: 't2', user_id: 'u2', username: 'bob',
    })
    apiMock.getCurrentUser.mockRejectedValue(new Error('me down'))
    render(<UserProvider><Probe /></UserProvider>)
    fireEvent.click(screen.getByText('login'))
    await waitFor(() => expect(screen.getByTestId('token').textContent).toBe('t2'))
    expect(localStorage.getItem('role')).toBe('user')
    expect(localStorage.getItem('email')).toBeNull()
    expect(localStorage.getItem('phone')).toBeNull()
    expect(localStorage.getItem('department')).toBeNull()
  })

  it('login：/api/me 合并权限清单，permissions 空回落 []', async () => {
    apiMock.login.mockResolvedValue({
      token: 't3', user_id: 'u3', username: 'carl', role: 'user',
    })
    apiMock.getCurrentUser.mockResolvedValue({
      id: 'u3', username: 'carl', role: 'user', permissions: undefined, totp_enabled: false,
    })
    render(<UserProvider><Probe /></UserProvider>)
    fireEvent.click(screen.getByText('login'))
    await waitFor(() => {
      expect(screen.getByTestId('user').textContent).toBe('carl:0')
      expect(screen.getByTestId('token').textContent).toBe('t3')
    })
  })

  it('updateUser：有值写入、空/空白移除可选存储并更新 role', async () => {
    render(<UserProvider><Probe /></UserProvider>)
    fireEvent.click(screen.getByText('update'))
    await waitFor(() => expect(localStorage.getItem('email')).toBe('new@x.y'))
    expect(localStorage.getItem('phone')).toBeNull()
    expect(localStorage.getItem('department')).toBeNull()
    expect(localStorage.getItem('role')).toBe('user')
  })

  it('updateUser 回归：只更新资料字段时保留 permissions（否则菜单被 fail-closed 全裁）', async () => {
    // 复现线上「左侧菜单整个消失」：/api/me 载入 permissions 后，Profile 资料
    // 同步调 updateUser 只传 id/username/…/role，若 setUser 整体替换会把
    // permissions 抹成 undefined → filterMenuRoutes 全裁 → 侧栏导航项全没了
    localStorage.setItem('token', 'tk')
    localStorage.setItem('username', 'admin')
    localStorage.setItem('role', 'admin')
    apiMock.getCurrentUser.mockResolvedValue(me) // me.permissions = ['users:admin']
    render(<UserProvider><Probe /></UserProvider>)
    await waitFor(() => expect(screen.getByTestId('user').textContent).toBe('admin:1'))

    // 模拟 Profile 的 applyProfile：不带 permissions 的资料更新
    fireEvent.click(screen.getByText('update'))
    await waitFor(() => expect(localStorage.getItem('role')).toBe('user'))
    expect(screen.getByTestId('user').textContent).toBe('admin:1')
  })

  it('logout：清全部键、user/token 归 null、ready 保持 true', () => {
    localStorage.setItem('token', 't')
    localStorage.setItem('username', 'u')
    localStorage.setItem('role', 'admin')
    localStorage.setItem('email', 'a@b.c')
    render(<UserProvider><Probe /></UserProvider>)
    fireEvent.click(screen.getByText('logout'))
    expect(screen.getByTestId('user').textContent).toBe('null')
    expect(screen.getByTestId('token').textContent).toBe('null')
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('email')).toBeNull()
    expect(screen.getByTestId('ready').textContent).toBe('true')
  })
})
