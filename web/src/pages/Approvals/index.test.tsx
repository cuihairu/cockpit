import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import type { Approval } from '@/services/approvals'
import ApprovalsPage from './index'

// 审批门：高危 Job 提交的审批台账（5s 轮询）+ 批准/驳回二次确认（M2d）

const userMock = vi.hoisted(() => ({ perms: ['approvals:read', 'approvals:write'] }))
vi.mock('@/contexts/useUser', () => ({
  useUser: () => ({
    user: {
      id: 'u1',
      username: 'admin',
      role: 'admin',
      permissions: userMock.perms,
    },
  }),
}))

const approvalsMock = vi.hoisted(() => ({
  listApprovals: vi.fn(),
  approveApproval: vi.fn(),
  rejectApproval: vi.fn(),
}))
vi.mock('@/services/approvals', () => ({
  listApprovals: approvalsMock.listApprovals,
  approveApproval: approvalsMock.approveApproval,
  rejectApproval: approvalsMock.rejectApproval,
}))

// 台账空态/加载骨架不触其他 api 面
vi.mock('@/services/api', () => ({ api: {} }))

process.on('unhandledRejection', () => {})

const msgError = vi.spyOn(message, 'error')
const msgSuccess = vi.spyOn(message, 'success')

const rows: Approval[] = [
  {
    id: 'ap-1',
    kind: 'job',
    target: 'a1',
    actor: 'cui',
    status: 'pending',
    summary: 'reboot-now',
    parameters: { command: 'reboot-now' },
    expiresAt: '2026-10-11T10:00:00Z',
    createdAt: '2026-10-11T09:50:00Z',
  },
  {
    id: 'ap-2',
    kind: 'job',
    target: 'a2',
    actor: 'cui',
    status: 'approved',
    summary: 'uptime',
    decidedBy: 'admin',
    decidedAt: '2026-10-11T09:55:00Z',
    jobId: 'job-9',
    createdAt: '2026-10-11T09:51:00Z',
  },
  {
    id: 'ap-3',
    kind: 'job',
    target: 'a3',
    actor: 'cui',
    status: 'expired',
    summary: 'cleanup',
    createdAt: '2026-10-11T09:40:00Z',
  },
]

const renderPage = (list: Approval[] = rows) => {
  approvalsMock.listApprovals.mockResolvedValue(list)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <ApprovalsPage />
    </QueryClientProvider>,
  )
}

// Popconfirm 确认按钮（ant-popover 挂 body，OK 案按钮为 primary）
const popconfirmOk = () =>
  document.querySelector('.ant-popover .ant-btn-primary') as HTMLButtonElement

// antd Select：点开下拉后按文本点选 body 挂载的 option
const pickOption = async (trigger: HTMLElement, label: string) => {
  fireEvent.mouseDown(trigger.querySelector('.ant-select-selector') as Element)
  const opt = await waitFor(() => {
    const found = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
      (o) => o.textContent === label,
    )
    if (!found) throw new Error('option not ready')
    return found
  })
  fireEvent.click(opt)
}

describe('Approvals', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    userMock.perms = ['approvals:read', 'approvals:write']
    approvalsMock.approveApproval.mockResolvedValue({ id: 'ap-1', status: 'approved' })
    approvalsMock.rejectApproval.mockResolvedValue({ id: 'ap-1', status: 'rejected' })
  })

  afterEach(() => {
    document.body.innerHTML = '' // Popconfirm 弹层挂 body，跨用例清理
  })

  it('台账渲染：状态徽标、待批行有批准/驳回，终态行无操作', async () => {
    renderPage()
    expect(await screen.findByText('reboot-now')).toBeInTheDocument()
    expect(screen.getByText('待批')).toBeInTheDocument()
    expect(screen.getByText('已批准')).toBeInTheDocument()
    expect(screen.getByText('已过期')).toBeInTheDocument()
    // pending 行两个操作按钮；approved 行显示 job id（copyable），expired 行 '-'
    expect(screen.getByRole('button', { name: /批\s*准/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /驳\s*回/ })).toBeInTheDocument()
    expect(screen.getByText(/job-9/)).toBeInTheDocument()
  })

  it('批准：Popconfirm 确认后经 approveApproval，成功提示并刷新', async () => {
    renderPage()
    fireEvent.click((await screen.findByRole('button', { name: /批\s*准/ })))
    await waitFor(() => expect(popconfirmOk()).toBeTruthy())
    fireEvent.click(popconfirmOk())
    await waitFor(() => expect(approvalsMock.approveApproval).toHaveBeenCalledWith('ap-1'))
    await waitFor(() =>
      expect(msgSuccess).toHaveBeenCalledWith('已批准，任务开始执行'),
    )
    // 决策后 invalidateQueries → 台账重取（5s 轮询叠加，慢机下次数只多不少）
    await waitFor(() =>
      expect(approvalsMock.listApprovals.mock.calls.length).toBeGreaterThanOrEqual(2),
    )
  })

  it('驳回：Popconfirm 确认后经 rejectApproval', async () => {
    renderPage()
    fireEvent.click((await screen.findByRole('button', { name: /驳\s*回/ })))
    await waitFor(() => expect(popconfirmOk()).toBeTruthy())
    fireEvent.click(popconfirmOk())
    await waitFor(() => expect(approvalsMock.rejectApproval).toHaveBeenCalledWith('ap-1'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已驳回'))
  })

  it('批准失败：服务端错误进 message.error', async () => {
    approvalsMock.approveApproval.mockRejectedValue({
      response: { data: { error: 'approval already decided or expired' } },
    })
    renderPage()
    fireEvent.click((await screen.findByRole('button', { name: /批\s*准/ })))
    await waitFor(() => expect(popconfirmOk()).toBeTruthy())
    fireEvent.click(popconfirmOk())
    await waitFor(() =>
      expect(msgError).toHaveBeenCalledWith('approval already decided or expired'),
    )
  })

  it('状态过滤：选择待批后 listApprovals 带 status 重取', async () => {
    renderPage([])
    // 空态等首取完成（antd 空态文案随 locale 变化，不断言文本）
    await waitFor(() => expect(approvalsMock.listApprovals).toHaveBeenCalledTimes(1))
    const trigger = screen.getByText('全部状态').closest('.ant-select') as HTMLElement
    await pickOption(trigger, '待批')
    await waitFor(() => expect(approvalsMock.listApprovals).toHaveBeenCalledWith('pending'))
  })

  it('无 approvals:write：操作列不出批准/驳回', async () => {
    userMock.perms = ['approvals:read']
    renderPage()
    expect(await screen.findByText('reboot-now')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /批\s*准/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /驳\s*回/ })).not.toBeInTheDocument()
  })
})
