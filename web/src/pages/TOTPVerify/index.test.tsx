import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { message } from 'antd'
import TOTPVerify from './index'

// TOTPVerify：二次验证——验证码清洗 / 成功落 token / 失败留页 / 备份码模式
// （Router 须带 Routes：navigate('/') 后组件卸载，否则 useEffect 在空 query 上
//  重跑会误跳 /login——生产中路由切换即卸载，无此路径）

const apiMock = vi.hoisted(() => ({ verifyTOTP: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const renderAt = (query = '?tmp_token=tt1&username=admin') => {
  render(
    <MemoryRouter initialEntries={[`/totp-verify${query}`]}>
      <Routes>
        <Route path="/totp-verify" element={<TOTPVerify />} />
        <Route path="/" element={<span data-testid="home" />} />
        <Route path="/login" element={<span data-testid="login-page" />} />
      </Routes>
    </MemoryRouter>,
  )
}

const codeInput = () => screen.getByPlaceholderText('123456')
const verifyBtn = () => screen.getByRole('button', { name: /^验\s*证$/ }) as HTMLButtonElement

describe('TOTPVerify', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    apiMock.verifyTOTP.mockReset()
    apiMock.verifyTOTP.mockResolvedValue({ token: 'jwt', username: 'admin' })
  })

  it('缺 tmp_token：报错并跳回登录页', async () => {
    renderAt('')
    expect(await screen.findByTestId('login-page')).toBeInTheDocument()
    expect(msgError).toHaveBeenCalledWith('无效的验证链接')
  })

  it('渲染问候用户名；验证码输入剔除非数字并截断 6 位', async () => {
    renderAt()
    expect(await screen.findByText('你好，admin')).toBeInTheDocument()
    fireEvent.change(codeInput(), { target: { value: '12a34567b8' } })
    expect(codeInput()).toHaveValue('123456')
    fireEvent.change(codeInput(), { target: { value: '' } })
    expect(verifyBtn().disabled).toBe(true)
  })

  it('验证成功：token 落地并跳首页（Enter 提交通路）', async () => {
    renderAt()
    fireEvent.change(codeInput(), { target: { value: '123456' } })
    await act(async () => {
      fireEvent.keyDown(codeInput(), { key: 'Enter' })
      fireEvent.keyUp(codeInput(), { key: 'Enter' })
    })
    expect(await screen.findByTestId('home')).toBeInTheDocument()
    expect(apiMock.verifyTOTP).toHaveBeenCalledWith('123456', 'tt1')
    expect(localStorage.getItem('token')).toBe('jwt')
    expect(localStorage.getItem('username')).toBe('admin')
  })

  it('验证失败：错误文案展示并留页', async () => {
    apiMock.verifyTOTP.mockRejectedValue(new Error('bad code'))
    renderAt()
    fireEvent.change(codeInput(), { target: { value: '000000' } })
    await act(async () => {
      fireEvent.click(verifyBtn())
    })
    expect(await screen.findByText('验证失败，请重试')).toBeInTheDocument()
    expect(msgError).toHaveBeenCalledWith('验证失败，请重试')
    expect(screen.queryByTestId('home')).toBeNull()
    expect(localStorage.getItem('token')).toBeNull()
  })

  it('备份码模式：大写化清洗截断 12 位；空提交告警；切换回验证码', async () => {
    renderAt()
    fireEvent.click(screen.getByText('使用恢复码登录'))
    const backupInput = screen.getByPlaceholderText('xxxx-xxxx-xxxx')
    expect(screen.getByText('请输入一个恢复码以恢复访问权限')).toBeInTheDocument()
    fireEvent.change(backupInput, { target: { value: 'ab12-cd34efghij' } })
    expect(backupInput).toHaveValue('AB12CD34EFGH')
    // 清空后点验证：按备份码语境告警
    fireEvent.change(backupInput, { target: { value: '' } })
    // 按钮已 disabled（空码），空值告警走 Enter 提交通路
    await act(async () => {
      fireEvent.keyDown(backupInput, { key: 'Enter' })
      fireEvent.keyUp(backupInput, { key: 'Enter' })
    })
    expect(msgWarning).toHaveBeenCalledWith('请输入备份码')
    expect(apiMock.verifyTOTP).not.toHaveBeenCalled()
    // 切回验证码模式：输入框清空恢复数字清洗
    fireEvent.click(screen.getByText('使用验证码登录'))
    expect(codeInput()).toHaveValue('')
    expect(screen.getByText('请打开您的认证器应用，输入 6 位验证码')).toBeInTheDocument()
  })

  it('页脚返回登录链接', () => {
    renderAt()
    fireEvent.click(screen.getByText('返回登录'))
    expect(screen.getByTestId('login-page')).toBeInTheDocument()
  })
})
