import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import RolesPage from './index'

// Roles：内置角色只读 / 权限 Popover / 权限矩阵编辑（name 编辑态锁死）+ 删除
// （Modal onOk 走 form.submit()——校验 reject 是 antd 常态，兜底吞掉）

const apiMock = vi.hoisted(() => ({
  listRoles: vi.fn(),
  createRole: vi.fn(),
  updateRole: vi.fn(),
  deleteRole: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

process.on('unhandledRejection', () => {})

const msgError = vi.spyOn(message, 'error')
const msgSuccess = vi.spyOn(message, 'success')

const roles = [
  {
    name: 'admin',
    builtin: true,
    permissions: ['files:read', 'files:write', 'files:admin', 'dns:read', 'dns:admin'],
  },
  { name: 'viewer', builtin: true, permissions: ['files:read'] },
  { name: 'operator', permissions: ['dns:read', 'dns:write'] },
]

const renderPage = (data = roles) => {
  apiMock.listRoles.mockResolvedValue(data)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <RolesPage />
    </QueryClientProvider>,
  )
}

// 首列 textContent 含角色名（Space 裸文本被拆分，精确 getByText 不可靠）
const rowOf = (name: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find(
    (tr) => tr.cells[0]?.textContent?.includes(name)) as HTMLTableRowElement

const modalOk = () =>
  document.querySelector('.ant-modal-footer .ant-btn-primary') as HTMLButtonElement

const popconfirmOk = () =>
  document.querySelector('.ant-popover .ant-btn-primary') as HTMLButtonElement

// 权限矩阵：定位「文件管理」行的第 n 个复选框（0=read 1=write 2=admin）
const matrixCheckbox = (resourceLabel: string, index: number) => {
  const row = Array.from(document.querySelectorAll<HTMLDivElement>('.ant-checkbox-group > div')).find((d) =>
    d.textContent?.startsWith(resourceLabel)) as HTMLDivElement
  return within(row).getAllByRole('checkbox')[index]
}

describe('Roles', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.listRoles.mockReset()
    apiMock.createRole.mockReset()
    apiMock.createRole.mockResolvedValue({})
    apiMock.updateRole.mockReset()
    apiMock.updateRole.mockResolvedValue({})
    apiMock.deleteRole.mockReset()
    apiMock.deleteRole.mockResolvedValue({})
  })
  afterEach(() => {
    document.body.innerHTML = '' // Modal/Popover 弹层挂 body，跨用例清理
  })

  it('列表渲染：内置标记与只读、自定义角色可操作', async () => {
    renderPage()
    expect(await screen.findByText('角色管理')).toBeInTheDocument()
    await screen.findByText('operator') // 数据行渲染完成
    expect(within(rowOf('admin')).getByText('内置')).toBeInTheDocument()
    expect(within(rowOf('admin')).getByText('内置角色不可修改')).toBeInTheDocument()
    expect(within(rowOf('viewer')).getByText('内置角色不可修改')).toBeInTheDocument()
    expect(within(rowOf('operator')).queryByText('内置角色不可修改')).toBeNull()
    expect(within(rowOf('operator')).getByRole('button', { name: /编\s*辑/ })).toBeInTheDocument()
    expect(within(rowOf('operator')).getByRole('button', { name: /删\s*除/ })).toBeInTheDocument()
  })

  it('权限点数与 Popover 明细', async () => {
    renderPage()
    await screen.findByText('operator')
    // Popover 默认 hover 触发（mouseEnterDelay 0.1s，findByText 轮询可等）
    fireEvent.mouseEnter(within(rowOf('admin')).getByRole('button', { name: '5 项' }))
    fireEvent.mouseMove(within(rowOf('admin')).getByRole('button', { name: '5 项' }))
    expect(await screen.findByText('files:read')).toBeInTheDocument()
    expect(screen.getByText('files:admin')).toBeInTheDocument()
    // operator 两项
    expect(within(rowOf('operator')).getByRole('button', { name: '2 项' })).toBeInTheDocument()
    fireEvent.mouseLeave(document.body)
  })

  it('新建角色：矩阵勾选提交 createRole', async () => {
    renderPage()
    await screen.findByText('operator')
    fireEvent.click(screen.getByRole('button', { name: /新建角色/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal')).toBeInTheDocument())
    // 权限全集从 admin 内置角色推导：dns / files 两资源
    expect(screen.getByText('文件管理')).toBeInTheDocument()
    expect(screen.getByText('DNS 管理')).toBeInTheDocument()
    fireEvent.change(document.querySelector('.ant-modal input')!, { target: { value: 'dev' } })
    fireEvent.click(matrixCheckbox('文件管理', 1)) // files:write
    fireEvent.click(matrixCheckbox('DNS 管理', 0)) // dns:read
    await act(async () => {
      fireEvent.click(modalOk())
    })
    await waitFor(() =>
      expect(apiMock.createRole).toHaveBeenCalledWith({
        name: 'dev',
        permissions: ['files:write', 'dns:read'],
      }))
    expect(msgSuccess).toHaveBeenCalledWith('角色 dev 已创建')
  })

  it('编辑角色：名字锁死 + updateRole 提交勾选集', async () => {
    renderPage()
    await screen.findByText('operator')
    fireEvent.click(within(rowOf('operator')).getByRole('button', { name: /编\s*辑/ }))
    expect(await screen.findByText('编辑角色 operator')).toBeInTheDocument()
    const nameInput = document.querySelector('.ant-modal input') as HTMLInputElement
    expect(nameInput.disabled).toBe(true)
    expect(nameInput.value).toBe('operator')
    // 初值回填 dns:read + dns:write
    expect((matrixCheckbox('DNS 管理', 0) as HTMLInputElement).checked).toBe(true)
    expect((matrixCheckbox('DNS 管理', 1) as HTMLInputElement).checked).toBe(true)
    fireEvent.click(matrixCheckbox('DNS 管理', 2)) // 追加 dns:admin
    await act(async () => {
      fireEvent.click(modalOk())
    })
    await waitFor(() =>
      expect(apiMock.updateRole).toHaveBeenCalledWith('operator', [
        'dns:read',
        'dns:write',
        'dns:admin',
      ]))
    expect(msgSuccess).toHaveBeenCalledWith('角色 operator 已更新')
  })

  it('删除角色：Popconfirm 确认后调用', async () => {
    renderPage()
    await screen.findByText('operator')
    fireEvent.click(within(rowOf('operator')).getByRole('button', { name: /删\s*除/ }))
    expect(await screen.findByText('删除角色 operator?')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(popconfirmOk())
    })
    await waitFor(() => expect(apiMock.deleteRole).toHaveBeenCalledWith('operator'))
    expect(msgSuccess).toHaveBeenCalledWith('角色 operator 已删除')
  })

  it('空角色列表：表格空态且矩阵无资源行', async () => {
    renderPage([])
    expect(await screen.findByText('角色管理')).toBeInTheDocument()
    await waitFor(() => expect(document.querySelector('tr.ant-table-row')).toBeNull())
    // 测试未包 ConfigProvider locale，空态文案是默认英文——锚定 placeholder 结构
    expect(document.querySelector('.ant-table-placeholder')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建角色/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal')).toBeInTheDocument())
    expect(document.querySelectorAll('.ant-checkbox-group > div').length).toBe(0)
  })

  it('保存失败：fallback 错误文案', async () => {
    apiMock.createRole.mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByText('operator')
    fireEvent.click(screen.getByRole('button', { name: /新建角色/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal')).toBeInTheDocument())
    fireEvent.change(document.querySelector('.ant-modal input')!, { target: { value: 'dev' } })
    await act(async () => {
      fireEvent.click(modalOk())
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('操作失败'))
  })
})
