import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import type { Job } from '@/services/jobs'
import JobsPage from './index'

// Jobs：全机执行台账（15s 轮询列表）+ 创建即执行弹窗 + 终态详情弹窗

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

// PermGuard → usePerm → useUser：admin 全权限（jobs:read/write 可见）
vi.mock('@/contexts/useUser', () => ({
  useUser: () => ({
    user: {
      id: 'u1',
      username: 'admin',
      role: 'admin',
      permissions: ['jobs:read', 'jobs:write'],
    },
  }),
}))

const jobsMock = vi.hoisted(() => ({
  listJobs: vi.fn(),
  createJob: vi.fn(),
  cancelJob: vi.fn(),
}))
vi.mock('@/services/jobs', () => ({
  listJobs: jobsMock.listJobs,
  createJob: jobsMock.createJob,
  cancelJob: jobsMock.cancelJob,
}))

process.on('unhandledRejection', () => {})

const msgError = vi.spyOn(message, 'error')
const msgSuccess = vi.spyOn(message, 'success')

const agents = [
  { id: 'a1', hostname: 'web-1', status: 'online', capabilities: [] },
  { id: 'a2', hostname: 'db-1', status: 'offline', capabilities: [] },
  // 无 hostname：盖 agentOptions 里 a.hostname || a.id 兜底分支
  { id: 'a3', status: 'online', capabilities: [] },
]

const jobs: Job[] = [
  {
    id: 'job-1',
    type: 'agent.exec',
    target: 'a1',
    actor: 'cui',
    status: 'success',
    parameters: { command: 'uptime' },
    output: 'load average: 0.10',
    exitCode: 0,
    createdAt: '2026-10-04T12:00:00Z',
    startedAt: '2026-10-04T12:00:00Z',
    finishedAt: '2026-10-04T12:00:01Z',
  },
  {
    id: 'job-2',
    type: 'agent.exec',
    target: 'a1',
    actor: 'cui',
    status: 'failed',
    parameters: { command: 'false' },
    exitCode: 3,
    error: 'agent rejected',
    createdAt: '2026-10-04T12:01:00Z',
    finishedAt: '2026-10-04T12:01:00Z',
  },
]

const renderPage = (list: Job[] = jobs) => {
  apiMock.getAgents.mockResolvedValue(agents)
  jobsMock.listJobs.mockResolvedValue(list)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <JobsPage />
    </QueryClientProvider>,
  )
}

const modalOk = () => document.querySelector('.ant-modal-footer .ant-btn-primary') as HTMLButtonElement

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

describe('Jobs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    jobsMock.createJob.mockResolvedValue({ ...jobs[0], id: 'job-new' })
    jobsMock.cancelJob.mockResolvedValue({ ...jobs[0], status: 'cancelled' })
  })
  afterEach(() => {
    document.body.innerHTML = '' // Modal/下拉弹层挂 body，跨用例清理
  })

  it('台账渲染：状态标签、命令、结果入口', async () => {
    renderPage()
    expect(await screen.findByText('Job 执行')).toBeInTheDocument()
    await screen.findByText('uptime')
    // 成功/失败标签都在
    expect(screen.getByText('成功')).toBeInTheDocument()
    expect(screen.getByText('失败')).toBeInTheDocument()
    // 离线 agent 的执行记录目标照常展示
    expect(screen.getAllByText('a1').length).toBeGreaterThan(0)
  })

  it('空台账：空态提示', async () => {
    renderPage([])
    expect(await screen.findByText('暂无执行记录')).toBeInTheDocument()
  })

  it('创建执行：选主机 + 命令 → createJob 收到 type/target/parameters', async () => {
    renderPage()
    fireEvent.click(await screen.findByText('执行命令'))
    await screen.findByText('执行命令', { selector: '.ant-modal-title' })

    // 目标下拉：离线 agent 禁用
    const selects = document.querySelectorAll('.ant-modal .ant-select')
    await pickOption(selects[0] as HTMLElement, 'web-1')

    fireEvent.change(document.querySelector('.ant-modal textarea') as HTMLTextAreaElement, {
      target: { value: 'uptime' },
    })
    fireEvent.click(modalOk())

    await waitFor(() =>
      expect(jobsMock.createJob).toHaveBeenCalledWith({
        type: 'agent.exec',
        target: 'a1',
        parameters: { command: 'uptime', timeout_s: 60 },
      }),
    )
    expect(msgSuccess).toHaveBeenCalled()
  })

  it('创建校验：空命令不发起请求', async () => {
    renderPage()
    fireEvent.click(await screen.findByText('执行命令'))
    await screen.findByText('执行命令', { selector: '.ant-modal-title' })
    fireEvent.click(modalOk())
    await waitFor(() => expect(msgError).not.toHaveBeenCalled())
    expect(jobsMock.createJob).not.toHaveBeenCalled()
  })

  it('创建失败：服务端报错进 message.error', async () => {
    jobsMock.createJob.mockRejectedValue({ response: { data: { error: 'agent offline' } } })
    renderPage()
    fireEvent.click(await screen.findByText('执行命令'))
    const selects = document.querySelectorAll('.ant-modal .ant-select')
    await pickOption(selects[0] as HTMLElement, 'web-1')
    fireEvent.change(document.querySelector('.ant-modal textarea') as HTMLTextAreaElement, {
      target: { value: 'uptime' },
    })
    fireEvent.click(modalOk())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('agent offline'))
  })

  it('结果详情：查看弹窗带状态/退出码/输出', async () => {
    renderPage()
    fireEvent.click((await screen.findAllByText('查看'))[0])
    expect(await screen.findByText('load average: 0.10')).toBeInTheDocument()
    expect(screen.getByText('退出码')).toBeInTheDocument()
  })

  it('台账边角：running 行结果列回退、无参数命令回退、无完成时间耗时回退', async () => {
    renderPage([
      {
        id: 'job-3',
        type: 'agent.exec',
        target: 'a1',
        actor: 'cui',
        status: 'running',
        createdAt: '2026-10-04T12:02:00Z',
        startedAt: '2026-10-04T12:02:00Z', // 有开始无结束 → 耗时 '-'（|| 右值侧）
      },
    ])
    expect(await screen.findByText('执行中')).toBeInTheDocument()
    // 命令列（无 parameters → '-'）、结果列（running → '-'）、耗时列各一个回退
    await waitFor(() => expect(screen.getAllByText('-').length).toBeGreaterThanOrEqual(3))
    // running 非终态无「查看」入口
    expect(screen.queryByText('查看')).not.toBeInTheDocument()
  })

  it('创建执行：清空超时则 parameters 不带 timeout_s', async () => {
    renderPage()
    fireEvent.click(await screen.findByText('执行命令'))
    await screen.findByText('执行命令', { selector: '.ant-modal-title' })
    const selects = document.querySelectorAll('.ant-modal .ant-select')
    await pickOption(selects[0] as HTMLElement, 'web-1')
    fireEvent.change(document.querySelector('.ant-modal textarea') as HTMLTextAreaElement, {
      target: { value: 'uptime' },
    })
    // InputNumber 清空 → form 值为 undefined → if (raw.timeout_s) 假侧
    fireEvent.change(
      document.querySelector('.ant-modal input[role="spinbutton"]') as HTMLInputElement,
      { target: { value: '' } },
    )
    fireEvent.click(modalOk())
    await waitFor(() =>
      expect(jobsMock.createJob).toHaveBeenCalledWith({
        type: 'agent.exec',
        target: 'a1',
        parameters: { command: 'uptime' },
      }),
    )
  })

  it('失败详情：error 段落渲染与无输出回退（（无输出））', async () => {
    renderPage()
    fireEvent.click((await screen.findAllByText('查看'))[1]) // job-2：error + 无 output
    expect(await screen.findByText('agent rejected')).toBeInTheDocument()
    expect(screen.getByText('（无输出）')).toBeInTheDocument()
  })

  it('稀疏终态详情：无退出码回退横杠 + 亚秒耗时 ms', async () => {
    renderPage([
      {
        id: 'job-4',
        type: 'agent.exec',
        target: 'a1',
        actor: 'cui',
        status: 'failed',
        parameters: { command: 'df' },
        createdAt: '2026-10-04T12:03:00Z',
        startedAt: '2026-10-04T12:03:00.9Z', // 5ms 亚秒 → 耗时走 ms 分支
        finishedAt: '2026-10-04T12:03:00.905Z',
      },
    ])
    expect(await screen.findByText('5ms')).toBeInTheDocument()
    fireEvent.click(screen.getByText('查看'))
    // 详情弹窗：exitCode undefined → 退出码回退 '-'
    await waitFor(() => expect(screen.getAllByText('-').length).toBeGreaterThanOrEqual(1))
    expect(screen.getByText('退出码')).toBeInTheDocument()
  })

  it('pending 行取消按钮：cancelJob 调用并刷新（W3 派发前可撤）', async () => {
    renderPage([
      {
        id: 'job-p',
        type: 'agent.exec',
        target: 'a1',
        actor: 'cui',
        status: 'pending',
        createdAt: '2026-10-04T12:05:00Z',
      },
    ])
    expect(await screen.findByText('排队中')).toBeInTheDocument()
    fireEvent.click(screen.getByText('取消'))
    await waitFor(() => expect(jobsMock.cancelJob).toHaveBeenCalledWith('job-p'))
    expect(msgSuccess).toHaveBeenCalled()
  })

  it('状态过滤：选择「失败」后 listJobs 收到 status 过滤', async () => {
    renderPage()
    expect(await screen.findByText('uptime')).toBeInTheDocument()
    const filter = document.querySelector('.ant-card .ant-select') as HTMLElement
    await pickOption(filter, '失败')
    await waitFor(() => expect(jobsMock.listJobs).toHaveBeenCalledWith({ status: 'failed' }))
  })

  it('取消失败：服务端报错进 message.error', async () => {
    jobsMock.cancelJob.mockRejectedValue({ response: { data: { error: 'only pending jobs can be cancelled' } } })
    renderPage([
      {
        id: 'job-p',
        type: 'agent.exec',
        target: 'a1',
        actor: 'cui',
        status: 'pending',
        createdAt: '2026-10-04T12:05:00Z',
      },
    ])
    expect(await screen.findByText('排队中')).toBeInTheDocument()
    fireEvent.click(screen.getByText('取消'))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('only pending jobs can be cancelled'))
  })

  it('弹窗关闭：创建取消与详情关闭都能退出', async () => {
    renderPage()
    // 详情弹窗：footer 关闭（onCancel 与 footer 分支）——先做，避免两个
    // modal root 并存（antd 关闭动画在 jsdom 不完成）干扰文本查询
    fireEvent.click((await screen.findAllByText('查看'))[0])
    await screen.findByText('load average: 0.10')
    // antd 对双汉字按钮自动插空格（关 闭），用正则兜住
    fireEvent.click(screen.getByText(/关\s*闭/))
    expect(await screen.findByText('Job 执行')).toBeInTheDocument()

    // 再次打开，走右上角 X（onCancel 分支）
    fireEvent.click((await screen.findAllByText('查看'))[0])
    await screen.findByText('load average: 0.10')
    fireEvent.click(document.querySelector('.ant-modal-close') as HTMLButtonElement)

    // 创建弹窗：取消（onCancel 分支）
    fireEvent.click(await screen.findByText('执行命令'))
    await screen.findByText('执行命令', { selector: '.ant-modal-title' })
    const cancelBtn = document.querySelector(
      '.ant-modal-footer .ant-btn:not(.ant-btn-primary)',
    ) as HTMLButtonElement
    fireEvent.click(cancelBtn)
  })
})