import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import Profile from './index'

// Profile：useUser + getCurrentUser 双源同步（失败回退本地 user），
// 资料/改密两个 Form——保存、校验、resetFields

const localUser = {
  id: 'u1',
  username: 'admin',
  email: 'local@example.com',
  phone: '100',
  department: 'ops',
  role: 'admin',
}
const serverUser = {
  id: 'u1',
  username: 'admin',
  email: 'server@example.com',
  phone: '200',
  department: 'dev',
  role: 'admin',
}

const apiMock = vi.hoisted(() => ({
  getCurrentUser: vi.fn(),
  updateProfile: vi.fn(),
  changePassword: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))
const updateUser = vi.hoisted(() => vi.fn())
let mockUser: typeof localUser | null = localUser
vi.mock('@/contexts/useUser', () => ({
  useUser: () => ({ user: mockUser, updateUser }),
}))

const msgError = vi.spyOn(message, 'error')
const msgSuccess = vi.spyOn(message, 'success')

describe('Profile', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUser = localUser
    apiMock.getCurrentUser.mockReset()
    apiMock.getCurrentUser.mockResolvedValue(serverUser)
    apiMock.updateProfile.mockReset()
    apiMock.updateProfile.mockResolvedValue({ ...serverUser, email: 'saved@example.com' })
    apiMock.changePassword.mockReset()
    apiMock.changePassword.mockResolvedValue({})
  })

  it('加载成功：服务器数据填表并同步 updateUser', async () => {
    render(<Profile />)
    expect(screen.getByText('个人信息')).toBeInTheDocument()
    expect(screen.getByText('管理员')).toBeInTheDocument()
    await waitFor(() =>
      expect(updateUser).toHaveBeenCalledWith(
        expect.objectContaining({ email: 'server@example.com', phone: '200', department: 'dev' }),
      ))
    expect((screen.getByPlaceholderText('请输入邮箱') as HTMLInputElement).value).toBe('server@example.com')
    expect((screen.getByPlaceholderText('请输入手机号') as HTMLInputElement).value).toBe('200')
    expect((screen.getByPlaceholderText('请输入部门') as HTMLInputElement).value).toBe('dev')
  })

  it('加载失败：报错并回退本地 user 数据', async () => {
    apiMock.getCurrentUser.mockRejectedValue(new Error('down'))
    render(<Profile />)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('加载个人信息失败'))
    expect((screen.getByPlaceholderText('请输入邮箱') as HTMLInputElement).value).toBe('local@example.com')
    expect((screen.getByPlaceholderText('请输入部门') as HTMLInputElement).value).toBe('ops')
  })

  it('邮箱格式校验', async () => {
    render(<Profile />)
    await waitFor(() =>
      expect((screen.getByPlaceholderText('请输入手机号') as HTMLInputElement).value).toBe('200'))
    fireEvent.change(screen.getByPlaceholderText('请输入邮箱'), { target: { value: 'bad' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保存信息/ }))
    })
    expect(await screen.findByText('请输入有效的邮箱地址')).toBeInTheDocument()
    expect(apiMock.updateProfile).not.toHaveBeenCalled()
  })

  it('保存资料成功：updateProfile + updateUser + 成功提示', async () => {
    render(<Profile />)
    // 等 getCurrentUser 填充完成再改值提交
    await waitFor(() =>
      expect((screen.getByPlaceholderText('请输入手机号') as HTMLInputElement).value).toBe('200'))
    fireEvent.change(screen.getByPlaceholderText('请输入邮箱'), { target: { value: 'new@example.com' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保存信息/ }))
    })
    await waitFor(() =>
      expect(apiMock.updateProfile).toHaveBeenCalledWith({
        email: 'new@example.com',
        phone: '200',
        department: 'dev',
      }))
    expect(msgSuccess).toHaveBeenCalledWith('个人信息已更新')
    await waitFor(() =>
      expect(updateUser).toHaveBeenCalledWith(
        expect.objectContaining({ email: 'saved@example.com' }),
      ))
  })

  it('保存资料失败：报错文案透出', async () => {
    apiMock.updateProfile.mockRejectedValue(new Error('boom'))
    render(<Profile />)
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保存信息/ }))
    })
    expect(msgError).toHaveBeenCalledWith('更新个人信息失败')
  })

  it('改密成功：changePassword 调用后表单清空', async () => {
    render(<Profile />)
    fireEvent.change(screen.getByPlaceholderText('请输入当前密码'), { target: { value: 'old123' } })
    fireEvent.change(screen.getByPlaceholderText('请输入新密码（至少 6 位）'), { target: { value: 'newpass123' } })
    fireEvent.change(screen.getByPlaceholderText('请再次输入新密码'), { target: { value: 'newpass123' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /修改密码/ }))
    })
    expect(apiMock.changePassword).toHaveBeenCalledWith('old123', 'newpass123')
    expect(msgSuccess).toHaveBeenCalledWith('密码已修改')
    await waitFor(() =>
      expect((screen.getByPlaceholderText('请输入当前密码') as HTMLInputElement).value).toBe(''))
  })

  it('改密确认不一致校验', async () => {
    render(<Profile />)
    fireEvent.change(screen.getByPlaceholderText('请输入当前密码'), { target: { value: 'old123' } })
    fireEvent.change(screen.getByPlaceholderText('请输入新密码（至少 6 位）'), { target: { value: 'newpass123' } })
    fireEvent.change(screen.getByPlaceholderText('请再次输入新密码'), { target: { value: 'mismatch9' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /修改密码/ }))
    })
    expect(await screen.findByText('两次输入的密码不一致')).toBeInTheDocument()
    expect(apiMock.changePassword).not.toHaveBeenCalled()
  })

  it('改密失败：报错文案透出', async () => {
    apiMock.changePassword.mockRejectedValue(new Error('old wrong'))
    render(<Profile />)
    fireEvent.change(screen.getByPlaceholderText('请输入当前密码'), { target: { value: 'old123' } })
    fireEvent.change(screen.getByPlaceholderText('请输入新密码（至少 6 位）'), { target: { value: 'newpass123' } })
    fireEvent.change(screen.getByPlaceholderText('请再次输入新密码'), { target: { value: 'newpass123' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /修改密码/ }))
    })
    expect(msgError).toHaveBeenCalledWith('修改密码失败')
  })

  it('无本地 user：描述兜底 Admin/用户 角色文案，保存回落空 id/角色 user', async () => {
    mockUser = null
    apiMock.getCurrentUser.mockRejectedValue(new Error('down'))
    apiMock.updateProfile.mockResolvedValue({})
    render(<Profile />)
    expect(screen.getByText('Admin')).toBeInTheDocument()
    expect(screen.getByText('用户')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保存信息/ }))
    })
    await waitFor(() => expect(updateUser).toHaveBeenCalledWith(
      expect.objectContaining({ id: '', username: '', role: 'user' })))
  })

  it('服务器资料缺可选字段：表单回落空串', async () => {
    apiMock.getCurrentUser.mockResolvedValue({
      id: 'u1', username: 'admin', role: 'admin', permissions: [], totp_enabled: false,
    })
    render(<Profile />)
    await waitFor(() =>
      expect((screen.getByPlaceholderText('请输入邮箱') as HTMLInputElement).value).toBe(''))
    expect((screen.getByPlaceholderText('请输入手机号') as HTMLInputElement).value).toBe('')
    expect((screen.getByPlaceholderText('请输入部门') as HTMLInputElement).value).toBe('')
  })

  it('加载中卸载：ignore 短路成功与失败回填', async () => {
    let resolveMe: (v: unknown) => void = () => {}
    apiMock.getCurrentUser.mockImplementation(() => new Promise((r) => { resolveMe = r }))
    const first = render(<Profile />)
    first.unmount()
    resolveMe(serverUser)
    await waitFor(() => expect(apiMock.getCurrentUser).toHaveBeenCalled())

    let rejectMe: (e: unknown) => void = () => {}
    apiMock.getCurrentUser.mockImplementation(() => new Promise((_r, rej) => { rejectMe = rej }))
    const second = render(<Profile />)
    second.unmount()
    rejectMe(new Error('down'))
    await waitFor(() => expect(apiMock.getCurrentUser).toHaveBeenCalledTimes(2))
  })
})
