import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { message } from 'antd'
import ResetPassword from './index'

// ResetPassword：三步重置——验证码校验 / 密码强度分档 / 重置完成
// （token 缺失 Navigate /login，Router 须带 Routes 让组件切换可观测）

const apiMock = vi.hoisted(() => ({ verifyResetCode: vi.fn(), resetPassword: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const msgError = vi.spyOn(message, 'error')
const msgSuccess = vi.spyOn(message, 'success')

const renderAt = (query = '?token=tk1') => {
  render(
    <MemoryRouter initialEntries={[`/reset-password${query}`]}>
      <Routes>
        <Route path="/reset-password" element={<ResetPassword />} />
        <Route path="/login" element={<span data-testid="login-page" />} />
      </Routes>
    </MemoryRouter>,
  )
}

const submitCode = async (code: string) => {
  fireEvent.change(screen.getByPlaceholderText('请输入6位验证码'), { target: { value: code } })
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: /^验\s*证$/ }))
  })
}

describe('ResetPassword', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.verifyResetCode.mockReset()
    apiMock.verifyResetCode.mockResolvedValue({ valid: true })
    apiMock.resetPassword.mockReset()
    apiMock.resetPassword.mockResolvedValue({})
  })

  it('缺 token：直接跳回登录页', () => {
    renderAt('')
    expect(screen.getByTestId('login-page')).toBeInTheDocument()
  })

  it('步骤 1：空提交与格式校验', async () => {
    renderAt()
    expect(screen.getByText('请输入邮件中发送的6位验证码以验证您的身份。')).toBeInTheDocument()
    await submitCode('')
    expect(await screen.findByText('请输入验证码')).toBeInTheDocument()
    await submitCode('abc')
    expect(await screen.findByText('验证码只能包含数字')).toBeInTheDocument()
    await submitCode('1234')
    expect(await screen.findByText('验证码为6位数字')).toBeInTheDocument()
    expect(apiMock.verifyResetCode).not.toHaveBeenCalled()
  })

  it('验证码无效：报错并停留步骤 1', async () => {
    apiMock.verifyResetCode.mockResolvedValue({ valid: false })
    renderAt()
    await submitCode('123456')
    expect(msgError).toHaveBeenCalledWith('验证码无效或已过期')
    expect(screen.getByRole('button', { name: /^验\s*证$/ })).toBeInTheDocument()
  })

  it('验证失败：接口异常透错并停留步骤 1', async () => {
    apiMock.verifyResetCode.mockRejectedValue(new Error('svc down'))
    renderAt()
    await submitCode('123456')
    expect(msgError).toHaveBeenCalledWith('验证失败')
    expect(screen.getByRole('button', { name: /^验\s*证$/ })).toBeInTheDocument()
  })

  it('验证成功进步骤 2：密码强度弱/中/强三档', async () => {
    renderAt()
    await submitCode('654321')
    expect(apiMock.verifyResetCode).toHaveBeenCalledWith('tk1', '654321')
    expect(msgSuccess).toHaveBeenCalledWith('验证码验证成功')
    expect(await screen.findByText('设置新密码')).toBeInTheDocument()

    const pw = screen.getByPlaceholderText('请输入新密码（至少6位）')
    // 未输入时不渲染强度区块
    expect(screen.queryByText(/密码强度/)).toBeNull()
    fireEvent.change(pw, { target: { value: '12345678' } }) // 25+10=35 → 弱
    expect(screen.getByText('密码强度：')).toBeInTheDocument()
    expect(screen.getByText('弱')).toBeInTheDocument()
    fireEvent.change(pw, { target: { value: 'abcd12345' } }) // 25+20+10=55 → 中
    expect(screen.getByText('中')).toBeInTheDocument()
    fireEvent.change(pw, { target: { value: 'Abcdefgh1234' } }) // 90 → 强
    expect(screen.getByText('强')).toBeInTheDocument()
  })

  it('密码强度：短长度/无数字/含特殊字符各分支', async () => {
    renderAt()
    await submitCode('654321')
    const pw = screen.getByPlaceholderText('请输入新密码（至少6位）')
    // 短于 8 位（长度加分分支 false）+ 无数字 + 含特殊字符 → 20+20+10=50 中
    fireEvent.change(pw, { target: { value: 'Ab!' } })
    expect(screen.getByText('中')).toBeInTheDocument()
    // 无数字、含特殊字符、≥8 位 → 25+20+20+10=75 强
    fireEvent.change(pw, { target: { value: 'Abcdefgh!' } })
    expect(screen.getByText('强')).toBeInTheDocument()
  })

  it('步骤 2：确认密码为空走校验通过分支（required 拦截）', async () => {
    renderAt()
    await submitCode('654321')
    fireEvent.change(screen.getByPlaceholderText('请输入新密码（至少6位）'), { target: { value: 'newpass123' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: '重置密码' }))
    })
    expect(await screen.findByText('请确认新密码')).toBeInTheDocument()
    expect(apiMock.resetPassword).not.toHaveBeenCalled()
  })

  it('步骤 2：确认密码不一致校验', async () => {
    renderAt()
    await submitCode('654321')
    fireEvent.change(screen.getByPlaceholderText('请输入新密码（至少6位）'), { target: { value: 'newpass123' } })
    fireEvent.change(screen.getByPlaceholderText('请再次输入新密码'), { target: { value: 'different' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: '重置密码' }))
    })
    expect(await screen.findByText('两次输入的密码不一致')).toBeInTheDocument()
    expect(apiMock.resetPassword).not.toHaveBeenCalled()
  })

  it('重置成功：完成页前往登录', async () => {
    renderAt()
    await submitCode('654321')
    fireEvent.change(screen.getByPlaceholderText('请输入新密码（至少6位）'), { target: { value: 'newpass123' } })
    fireEvent.change(screen.getByPlaceholderText('请再次输入新密码'), { target: { value: 'newpass123' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: '重置密码' }))
    })
    expect(await screen.findByText('密码重置成功')).toBeInTheDocument()
    expect(apiMock.resetPassword).toHaveBeenCalledWith('tk1', '654321', 'newpass123')
    expect(msgSuccess).toHaveBeenCalledWith('密码已成功重置')
    fireEvent.click(screen.getByRole('button', { name: '前往登录' }))
    expect(screen.getByTestId('login-page')).toBeInTheDocument()
  })

  it('重置接口失败：报错停留步骤 2', async () => {
    apiMock.resetPassword.mockRejectedValue(new Error('boom'))
    renderAt()
    await submitCode('654321')
    fireEvent.change(screen.getByPlaceholderText('请输入新密码（至少6位）'), { target: { value: 'newpass123' } })
    fireEvent.change(screen.getByPlaceholderText('请再次输入新密码'), { target: { value: 'newpass123' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: '重置密码' }))
    })
    // Error 无 response → fallback 文案
    expect(msgError).toHaveBeenCalledWith('重置密码失败')
    expect(screen.getByText('设置新密码')).toBeInTheDocument()
  })
})
