import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, useLocation } from 'react-router-dom'
import Login from './index'
import { TOTPRequiredError } from '@/contexts/userTypes'

// Login：登录表单——成功跳首页 / TOTP 跳二次验证 / 失败报错

const loginMock = vi.hoisted(() => vi.fn())
vi.mock('@/contexts/useUser', () => ({ useUser: () => ({ login: loginMock }) }))

const renderAt = (initial = '/login') => {
  const Probe = () => {
    const loc = useLocation()
    return <span data-testid="loc">{loc.pathname}{loc.search}</span>
  }
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <Login />
      <Probe />
    </MemoryRouter>,
  )
}

const loc = () => document.querySelector('[data-testid="loc"]')!.textContent

describe('Login', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    loginMock.mockReset()
    loginMock.mockResolvedValue(undefined)
  })

  it('渲染品牌区与表单；记住账号默认勾选；忘记密码链接', () => {
    renderAt()
    expect(screen.getByText('Cockpit')).toBeInTheDocument()
    expect(screen.getByText('个人混合基础设施控制台')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('用户名')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('密码')).toBeInTheDocument()
    expect(screen.getByText('记住账号')).toBeVisible()
    expect(screen.getByRole('link', { name: '忘记密码？' })).toHaveAttribute('href', '/forgot-password')
  })

  it('空提交触发必填校验，不发登录请求', async () => {
    renderAt()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /登\s*录/ }))
    })
    expect(await screen.findByText('请输入用户名')).toBeInTheDocument()
    expect(screen.getByText('请输入密码')).toBeInTheDocument()
    expect(loginMock).not.toHaveBeenCalled()
  })

  it('登录成功跳首页', async () => {
    renderAt()
    fireEvent.change(screen.getByPlaceholderText('用户名'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByPlaceholderText('密码'), { target: { value: 'pw' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /登\s*录/ }))
    })
    await waitFor(() => expect(loc()).toBe('/'))
    expect(loginMock).toHaveBeenCalledWith('admin', 'pw')
  })

  it('需要 TOTP：带 tmp_token 与用户名跳二次验证页', async () => {
    loginMock.mockRejectedValue(new TOTPRequiredError({
      user_id: 'u1', username: 'admin', requires_totp: true, tmp_token: 'tt9',
    }))
    renderAt()
    fireEvent.change(screen.getByPlaceholderText('用户名'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByPlaceholderText('密码'), { target: { value: 'pw' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /登\s*录/ }))
    })
    await waitFor(() =>
      expect(loc()).toBe('/totp-verify?tmp_token=tt9&username=admin'))
  })

  it('登录失败：报错并留在登录页', async () => {
    loginMock.mockRejectedValue(new Error('bad credentials'))
    renderAt()
    fireEvent.change(screen.getByPlaceholderText('用户名'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByPlaceholderText('密码'), { target: { value: 'wrong' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /登\s*录/ }))
    })
    // App.useApp 的 message 渲染真实 notice
    expect(await screen.findByText('用户名或密码错误')).toBeInTheDocument()
    expect(loc()).toBe('/login')
  })
})
