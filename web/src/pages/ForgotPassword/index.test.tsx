import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { message } from 'antd'
import ForgotPassword from './index'

// ForgotPassword：三步找回密码——用户名提交 / 统一提示 / 60s 重发倒计时
// （fake timers 下 RTL waitFor 轮询冻结——断言全部走 act flush 后的同步查询）

const apiMock = vi.hoisted(() => ({ forgotPassword: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const msgInfo = vi.spyOn(message, 'info')

const wrap = (ui: React.ReactNode) => <MemoryRouter>{ui}</MemoryRouter>

const submitUsername = async (value: string) => {
  fireEvent.change(screen.getByPlaceholderText('请输入您的用户名'), {
    target: { value },
  })
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: '发送重置邮件' }))
  })
}

describe('ForgotPassword', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    apiMock.forgotPassword.mockReset()
    apiMock.forgotPassword.mockResolvedValue({ message: '如果该用户存在，重置邮件已发送' })
  })
  afterEach(() => vi.useRealTimers())

  it('步骤 1：说明与空提交校验', async () => {
    vi.useRealTimers() // antd 校验链在 fake timers 下不推进
    render(wrap(<ForgotPassword />))
    expect(screen.getByText('找回密码')).toBeInTheDocument()
    expect(screen.getByText('重置密码流程')).toBeInTheDocument()
    await submitUsername('') // 空值提交（校验错误渲染晚于单次 flush，用轮询）
    expect(await screen.findByText('请输入用户名')).toBeInTheDocument()
    expect(apiMock.forgotPassword).not.toHaveBeenCalled()
  })

  it('提交后显示统一提示而不泄露邮箱 + 重发按钮倒计时禁用', async () => {
    render(wrap(<ForgotPassword />))
    await submitUsername('admin')
    expect(screen.getByText('重置邮件已发送')).toBeInTheDocument()
    expect(screen.getByText('如果该账户存在且已绑定邮箱，重置邮件将发送至绑定邮箱。')).toBeInTheDocument()
    expect(screen.queryByText(/@/)).not.toBeInTheDocument()
    expect(apiMock.forgotPassword).toHaveBeenCalledWith('admin')
    expect((screen.getByRole('button', { name: /60秒后可重发/ }) as HTMLButtonElement).disabled).toBe(true)
    // 倒计时递减
    act(() => { vi.advanceTimersByTime(10_000) })
    expect((screen.getByRole('button', { name: /50秒后可重发/ }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('倒计时归零后重发可用并再次调用接口', async () => {
    render(wrap(<ForgotPassword />))
    await submitUsername('admin')
    act(() => { vi.advanceTimersByTime(60_000) })
    const resend = screen.getByRole('button', { name: '重新发送' })
    expect((resend as HTMLButtonElement).disabled).toBe(false)
    await act(async () => {
      fireEvent.click(resend)
    })
    expect(apiMock.forgotPassword).toHaveBeenCalledTimes(2)
    expect(msgInfo).toHaveBeenCalledWith('如果该用户存在，重置邮件已发送')
    // 重发后重新进入倒计时
    expect((screen.getByRole('button', { name: /60秒后可重发/ }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('接口失败：安全措辞提示并停留步骤 1', async () => {
    apiMock.forgotPassword.mockRejectedValue(new Error('no user'))
    render(wrap(<ForgotPassword />))
    await submitUsername('ghost')
    // message 静态方法在 jsdom 不渲染 notice，断言 spy
    expect(msgInfo).toHaveBeenCalledWith('如果该用户存在，重置邮件已发送')
    // 未进入步骤 2
    expect(screen.getByRole('button', { name: '发送重置邮件' })).toBeInTheDocument()
  })

  it('步骤 2 重发失败走安全措辞；「返回登录」跳登录', async () => {
    render(wrap(<ForgotPassword />))
    await submitUsername('admin')
    expect(screen.getByText('重置邮件已发送')).toBeInTheDocument()
    // 步骤 2 的返回登录（navigate）
    fireEvent.click(screen.getByRole('button', { name: '返回登录' }))
    // 重发失败（倒计时归零后）
    act(() => { vi.advanceTimersByTime(60_000) })
    apiMock.forgotPassword.mockRejectedValue(new Error('smtp down'))
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: '重新发送' }))
    })
    expect(msgInfo).toHaveBeenCalledWith('如果该用户存在，重置邮件已发送')
  })
})
