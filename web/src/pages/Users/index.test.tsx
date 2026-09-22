import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import UsersPage from './index'

// Users：列表（自己 Tag/角色 Select 禁改）+ 新建 + 改角色 + 重置密码 + 删除
// （Modal onOk 走 form.submit()——校验 reject 是 antd 常态，兜底吞掉）

const apiMock = vi.hoisted(() => ({
  listUsers: vi.fn(),
  listRoles: vi.fn(),
  updateUser: vi.fn(),
  createUser: vi.fn(),
  changeUserPassword: vi.fn(),
  deleteUser: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/contexts/useUser', () => ({
  useUser: () => ({ user: { id: 'u1', username: 'admin', role: 'admin' } }),
}))

process.on('unhandledRejection', () => {})

const msgError = vi.spyOn(message, 'error')
const msgSuccess = vi.spyOn(message, 'success')

const users = [
  { id: 'u1', username: 'admin', role: 'admin', totp_enabled: true },
  { id: 'u2', username: 'alice', email: 'a@x.y', role: 'viewer', totp_enabled: false, created_at: '2026-01-02T03:04:05Z' },
]
const roles = [
  { name: 'admin', permissions: ['users:admin'], builtin: true },
  { name: 'viewer', permissions: [], builtin: true },
  { name: 'operator', permissions: ['files:read'] },
]

const renderPage = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <UsersPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

// 首列 textContent 含用户名（Space 裸文本被拆分，精确 getByText 不可靠）
const rowOf = (name: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find(
    (tr) => tr.cells[0]?.textContent?.includes(name)) as HTMLTableRowElement

const modalOk = () =>
  document.querySelector('.ant-modal-footer .ant-btn-primary') as HTMLButtonElement

const popconfirmOk = () =>
  document.querySelector('.ant-popover .ant-btn-primary') as HTMLButtonElement

// rc-select option 无 title 属性，按 .ant-select-item-option 文本匹配
const pickOption = async (label: string) => {
  const opt = await waitFor(() => {
    const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
      (o) => o.textContent === label)
    if (!el) throw new Error(`option not found: ${label}`)
    return el as HTMLElement
  })
  fireEvent.click(opt)
}

// 操作列：第 1 个是重置密码（icon-only 无 accessible name），第 2 个是删除
const resetPwdBtn = (name: string) =>
  within(rowOf(name)).getAllByRole('button')[0] as HTMLButtonElement
const selfDeleteBtn = (name: string) =>
  within(rowOf(name)).getAllByRole('button')[1] as HTMLButtonElement

describe('Users', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.listUsers.mockReset()
    apiMock.listUsers.mockResolvedValue(users)
    apiMock.listRoles.mockReset()
    apiMock.listRoles.mockResolvedValue(roles)
    apiMock.updateUser.mockReset()
    apiMock.updateUser.mockResolvedValue({})
    apiMock.createUser.mockReset()
    apiMock.createUser.mockResolvedValue({})
    apiMock.changeUserPassword.mockReset()
    apiMock.changeUserPassword.mockResolvedValue({})
    apiMock.deleteUser.mockReset()
    apiMock.deleteUser.mockResolvedValue({})
  })
  afterEach(() => {
    document.body.innerHTML = '' // Modal/Popconfirm 挂 body 的弹层跨用例清理
  })

  it('列表渲染：自己标记、TOTP 状态、创建时间', async () => {
    renderPage()
    expect(await screen.findByText('用户管理')).toBeInTheDocument()
    await screen.findByText('alice') // 数据行渲染完成
    const self = rowOf('admin')
    expect(within(self).getByText('自己')).toBeInTheDocument()
    expect(within(self).getByText('已启用')).toBeInTheDocument()
    const other = rowOf('alice')
    expect(within(other).getByText('未启用')).toBeInTheDocument()
    expect(within(other).getByText(/2026-01-02/)).toBeInTheDocument()
  })

  it('自己行角色 Select 与删除禁用；他人可用', async () => {
    renderPage()
    await screen.findByText('alice')
    expect(rowOf('admin').querySelector('.ant-select')!.classList).toContain('ant-select-disabled')
    expect(selfDeleteBtn('admin').disabled).toBe(true)
    expect(rowOf('alice').querySelector('.ant-select')!.classList).not.toContain('ant-select-disabled')
    expect(selfDeleteBtn('alice').disabled).toBe(false)
  })

  it('Select 改角色：updateUser + 成功提示', async () => {
    renderPage()
    await screen.findByText('alice')
    fireEvent.mouseDown(rowOf('alice').querySelector('.ant-select-selector')!)
    await pickOption('operator')
    await waitFor(() => expect(apiMock.updateUser).toHaveBeenCalledWith('u2', { role: 'operator' }))
    expect(msgSuccess).toHaveBeenCalledWith('已将 alice 的角色改为 operator')
  })

  it('新建用户：空提交校验 + 提交成功', async () => {
    renderPage()
    await screen.findByText('alice')
    fireEvent.click(screen.getByRole('button', { name: /新建用户/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal')).toBeInTheDocument())
    await act(async () => {
      fireEvent.click(modalOk()) // 空提交
    })
    expect(await screen.findByText('请输入用户名')).toBeInTheDocument()
    const textboxes = await screen.findAllByRole('textbox')
    fireEvent.change(textboxes[0], { target: { value: 'bob' } })
    fireEvent.change(document.querySelector('.ant-modal input[type="password"]')!, { target: { value: 'pass12345' } })
    await act(async () => {
      fireEvent.click(modalOk())
    })
    await waitFor(() =>
      expect(apiMock.createUser).toHaveBeenCalledWith({
        username: 'bob',
        password: 'pass12345',
        role: 'viewer',
        email: undefined,
      }))
    expect(msgSuccess).toHaveBeenCalledWith('用户 bob 已创建')
  })

  it('重置密码：短密码校验 + 成功重置', async () => {
    renderPage()
    await screen.findByText('alice')
    fireEvent.click(resetPwdBtn('alice'))
    expect(await screen.findByText('重置 alice 的密码')).toBeInTheDocument()
    fireEvent.change(document.querySelector('.ant-modal input[type="password"]')!, { target: { value: 'short' } })
    await act(async () => {
      fireEvent.click(modalOk())
    })
    expect(await screen.findByText('至少 8 位')).toBeInTheDocument()
    fireEvent.change(document.querySelector('.ant-modal input[type="password"]')!, { target: { value: 'newpass123' } })
    await act(async () => {
      fireEvent.click(modalOk())
    })
    await waitFor(() => expect(apiMock.changeUserPassword).toHaveBeenCalledWith('u2', 'newpass123'))
    expect(msgSuccess).toHaveBeenCalledWith('已重置 alice 的密码')
  })

  it('删除用户：Popconfirm 确认后调用', async () => {
    renderPage()
    await screen.findByText('alice')
    fireEvent.click(selfDeleteBtn('alice'))
    expect(await screen.findByText('删除用户 alice?')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(popconfirmOk())
    })
    await waitFor(() => expect(apiMock.deleteUser).toHaveBeenCalledWith('u2'))
    expect(msgSuccess).toHaveBeenCalledWith('用户 alice 已删除')
  })

  it('mutation 失败：fallback 错误文案', async () => {
    apiMock.deleteUser.mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByText('alice')
    fireEvent.click(selfDeleteBtn('alice'))
    await screen.findByText('删除用户 alice?')
    await act(async () => {
      fireEvent.click(popconfirmOk())
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('操作失败'))
  })

  it('改角色失败：fallback 错误文案', async () => {
    apiMock.updateUser.mockRejectedValue(new Error('role boom'))
    renderPage()
    await screen.findByText('alice')
    fireEvent.mouseDown(rowOf('alice').querySelector('.ant-select-selector')!)
    await pickOption('operator')
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('操作失败'))
  })

  it('新建用户失败：fallback 错误文案', async () => {
    apiMock.createUser.mockRejectedValue(new Error('create boom'))
    renderPage()
    await screen.findByText('alice')
    fireEvent.click(screen.getByRole('button', { name: /新建用户/ }))
    await waitFor(() => expect(screen.getByText('邮箱（可选）')).toBeInTheDocument())
    const textboxes = await screen.findAllByRole('textbox')
    fireEvent.change(textboxes[0], { target: { value: 'bob' } })
    fireEvent.change(document.querySelector('.ant-modal input[type="password"]')!, { target: { value: 'pass12345' } })
    await act(async () => {
      fireEvent.click(modalOk())
    })
    await waitFor(() => expect(apiMock.createUser).toHaveBeenCalledWith(expect.objectContaining({ username: 'bob' })))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('操作失败'))
  })

  it('改密失败：fallback 错误文案', async () => {
    apiMock.changeUserPassword.mockRejectedValue(new Error('pwd boom'))
    renderPage()
    await screen.findByText('alice')
    fireEvent.click(resetPwdBtn('alice'))
    expect(await screen.findByText('重置 alice 的密码')).toBeInTheDocument()
    fireEvent.change(document.querySelector('.ant-modal input[type="password"]')!, { target: { value: 'newpass123' } })
    await act(async () => {
      fireEvent.click(modalOk())
    })
    await waitFor(() => expect(apiMock.changeUserPassword).toHaveBeenCalledWith('u2', 'newpass123'))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('操作失败'))
  })

  it('刷新按钮 invalidate 用户列表；两个弹窗取消触发 onCancel', async () => {
    renderPage()
    await screen.findByText('alice')
    const before = apiMock.listUsers.mock.calls.length
    fireEvent.click(document.querySelector('.anticon-reload')!.closest('button')!)
    await waitFor(() => expect(apiMock.listUsers.mock.calls.length).toBeGreaterThan(before))
    // 新建弹窗取消（footer Cancel；测试未包 ConfigProvider 为英文文案）
    fireEvent.click(screen.getByRole('button', { name: /新建用户/ }))
    await waitFor(() => expect(screen.getByText('邮箱（可选）')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: /^Cancel$/ }))
  })

  it('重置密码弹窗取消（X）触发 onCancel', async () => {
    renderPage()
    await screen.findByText('alice')
    fireEvent.click(resetPwdBtn('alice'))
    expect(await screen.findByText('重置 alice 的密码')).toBeInTheDocument()
    // jsdom 下关弹窗动画不结束、标题被冻结，无法断言关闭——点 X 覆盖 onCancel 即可
    fireEvent.click(document.querySelector('.ant-modal-close')!)
  })
})
