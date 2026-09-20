import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { UserProvider } from './UserContext'
import { useUser } from './useUser'
import { TOTPRequiredError } from './userTypes'
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
  const { user, token, permissionsReady, login, logout } = useUser()
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
    </div>
  )
}

describe('UserContext', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('无 token：user null、permissionsReady true（免拉取即绪）', () => {
    render(<UserProvider><Probe /></UserProvider>)
    expect(screen.getByTestId('user').textContent).toBe('null')
    expect(screen.getByTestId('ready').textContent).toBe('true')
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
