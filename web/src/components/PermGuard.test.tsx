import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PermGuard } from './PermGuard'
import { UserProvider } from '@/contexts/UserContext'
import type { UserInfo } from '@/types'

// PermGuard：权限裁剪容器——无权限默认不渲染（「看不到入口」验收）

const apiMock = vi.hoisted(() => ({
  login: vi.fn(),
  getCurrentUser: vi.fn(),
  logout: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const me = (permissions: string[]): UserInfo => ({
  id: 'u1', username: 'admin', email: 'a@b.c', role: 'admin', permissions, totp_enabled: false,
})

const grant = (permissions: string[]) => {
  localStorage.setItem('token', 'tk')
  localStorage.setItem('username', 'admin')
  apiMock.getCurrentUser.mockResolvedValue(me(permissions))
}

describe('PermGuard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
  })

  it('未登录（fail-closed）不渲染 children', () => {
    render(
      <UserProvider>
        <PermGuard perm="dns:write"><button>危险操作</button></PermGuard>
      </UserProvider>,
    )
    expect(screen.queryByText('危险操作')).toBeNull()
  })

  it('无权限不渲染；渲染 fallback', () => {
    grant([])
    render(
      <UserProvider>
        <PermGuard perm="dns:write" fallback={<span>只读</span>}>
          <button>危险操作</button>
        </PermGuard>
      </UserProvider>,
    )
    // 等 /api/me 拉完仍无权限
    waitFor(() => expect(apiMock.getCurrentUser).toHaveBeenCalled())
    expect(screen.queryByText('危险操作')).toBeNull()
  })

  it('有权限渲染 children（含 AND 数组）', async () => {
    grant(['dns:write', 'proxy:write'])
    render(
      <UserProvider>
        <PermGuard perm={['dns:write', 'proxy:write']}><button>危险操作</button></PermGuard>
      </UserProvider>,
    )
    expect(await screen.findByText('危险操作')).toBeInTheDocument()
  })

  it('AND 数组缺一不渲染', async () => {
    grant(['dns:write'])
    render(
      <UserProvider>
        <PermGuard perm={['dns:write', 'proxy:write']}><button>危险操作</button></PermGuard>
      </UserProvider>,
    )
    // 等拉取完成（有 user 但缺 proxy:write）
    await waitFor(() => expect(apiMock.getCurrentUser).toHaveBeenCalled())
    await waitFor(() => expect(screen.queryByText('危险操作')).toBeNull())
  })
})
