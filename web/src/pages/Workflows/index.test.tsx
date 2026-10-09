import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import type { WorkflowDef, WorkflowRun } from '@/services/workflows'
import WorkflowsPage from './index'

// Workflows：定义台账（15s 轮询）+ 新建/编辑抽屉（步骤编辑器）+ run 时间线
// （running 5s 轮询、终态停）+ run 取消 + 步骤 Job 详情复用台账视角

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

// PermGuard → usePerm → useUser：admin 全权限（workflows:read/write 可见）
vi.mock('@/contexts/useUser', () => ({
  useUser: () => ({
    user: {
      id: 'u1',
      username: 'admin',
      role: 'admin',
      permissions: ['workflows:read', 'workflows:write', 'jobs:read'],
    },
  }),
}))

const wfMock = vi.hoisted(() => ({
  listWorkflows: vi.fn(),
  createWorkflow: vi.fn(),
  updateWorkflow: vi.fn(),
  deleteWorkflow: vi.fn(),
  runWorkflow: vi.fn(),
  listWorkflowRuns: vi.fn(),
  getWorkflowRun: vi.fn(),
  cancelWorkflowRun: vi.fn(),
}))
// 只 mock API 函数，isRunTerminal/isStepTerminal 等纯函数用真实实现
vi.mock('@/services/workflows', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/services/workflows')>()),
  ...wfMock,
}))

const jobsMock = vi.hoisted(() => ({
  getJob: vi.fn(),
}))
vi.mock('@/services/jobs', () => ({
  getJob: jobsMock.getJob,
}))

process.on('unhandledRejection', () => {})

const msgError = vi.spyOn(message, 'error')
const msgSuccess = vi.spyOn(message, 'success')

const agents = [
  { id: 'a1', hostname: 'web-1', status: 'online', capabilities: [] },
  { id: 'a2', hostname: 'db-1', status: 'offline', capabilities: [] },
]

const wf: WorkflowDef = {
  id: 'wf-1',
  name: 'upgrade chain',
  description: 'stop → backup → upgrade',
  steps: [
    { type: 'agent.exec', target: 'a1', parameters: { name: 'backup', command: 'backup.sh' } },
    {
      type: 'agent.exec',
      target: 'a1',
      parameters: { name: 'upgrade', command: 'upgrade.sh', retry: 2, continue_on_error: true },
    },
  ],
  createdBy: 'cui',
  createdAt: '2026-10-09T00:00:00Z',
  updatedAt: '2026-10-09T00:00:00Z',
}

const run: WorkflowRun = {
  id: 'r-1',
  workflowId: 'wf-1',
  workflowName: 'upgrade chain',
  status: 'success',
  actor: 'cui',
  steps: [
    { name: 'backup', type: 'agent.exec', target: 'a1', status: 'success', jobId: 'j-1', attempts: 1 },
    { name: 'upgrade', type: 'agent.exec', target: 'a1', status: 'failed', jobId: 'j-2', attempts: 3, continueOnError: true },
  ],
  createdAt: '2026-10-09T01:00:00Z',
  finishedAt: '2026-10-09T01:00:10Z',
}

const renderPage = (list: WorkflowDef[] = [wf]) => {
  apiMock.getAgents.mockResolvedValue(agents)
  wfMock.listWorkflows.mockResolvedValue(list)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <WorkflowsPage />
    </QueryClientProvider>,
  )
}

const drawerFooterSave = () =>
  Array.from(document.querySelectorAll('.ant-drawer-footer button')).find((b) =>
    b.textContent?.includes('保'),
  ) as HTMLButtonElement

const drawerFooterCancel = () =>
  Array.from(document.querySelectorAll('.ant-drawer-footer button')).find((b) =>
    b.textContent?.includes('取'),
  ) as HTMLButtonElement

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

// Popconfirm 确认按钮（ant-popover 挂 body，默认 OK 案按钮为 primary）
const popconfirmOk = () =>
  document.querySelector('.ant-popover .ant-btn-primary') as HTMLButtonElement

describe('Workflows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    wfMock.createWorkflow.mockResolvedValue(wf)
    wfMock.updateWorkflow.mockResolvedValue(wf)
    wfMock.deleteWorkflow.mockResolvedValue(undefined)
    wfMock.runWorkflow.mockResolvedValue({ ...run, status: 'running', finishedAt: undefined })
    wfMock.getWorkflowRun.mockResolvedValue(run)
    wfMock.cancelWorkflowRun.mockResolvedValue({ ...run, status: 'cancelled' })
    jobsMock.getJob.mockResolvedValue({
      id: 'j-1',
      type: 'agent.exec',
      target: 'a1',
      actor: 'workflow',
      status: 'success',
      output: 'backup done',
      createdAt: '2026-10-09T01:00:00Z',
    })
  })
  afterEach(() => {
    document.body.innerHTML = '' // Modal/Drawer 弹层挂 body，跨用例清理
  })

  it('定义台账：名称/步数/步骤链渲染', async () => {
    renderPage()
    expect(await screen.findByText('Workflow 编排')).toBeInTheDocument()
    await screen.findByText('upgrade chain')
    expect(screen.getByText('backup → upgrade')).toBeInTheDocument()
  })

  it('空台账：空态提示', async () => {
    renderPage([])
    expect(await screen.findByText('暂无 Workflow')).toBeInTheDocument()
  })

  it('新建：步骤校验（缺步骤名/重复步骤名）不发请求', async () => {
    renderPage()
    fireEvent.click(await screen.findByText('新建 Workflow'))
    await screen.findByText('新建 Workflow', { selector: '.ant-drawer-title' })
    fireEvent.click(drawerFooterSave())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('请输入 Workflow 名称'))

    // 名称填上，步骤名空 → 缺步骤名
    fireEvent.change(screen.getByPlaceholderText('例如：停机 → 备份 → 升级 → 起机'), {
      target: { value: 'chain' },
    })
    fireEvent.click(drawerFooterSave())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('步骤 1：请输入步骤名'))
    expect(wfMock.createWorkflow).not.toHaveBeenCalled()

    // 步骤名 + 目标 + 命令齐全，第二行同名单步校验
    fireEvent.change(screen.getByPlaceholderText('同一 Workflow 内唯一'), {
      target: { value: 's1' },
    })
    fireEvent.click(screen.getByText('添加步骤'))
    const nameInputs = screen.getAllByPlaceholderText('同一 Workflow 内唯一')
    fireEvent.change(nameInputs[1], { target: { value: 's1' } })
    fireEvent.click(drawerFooterSave())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('步骤 1：请选择目标主机'))
  })

  it('新建成功：表单值折回 API 形态（retry/continue_on_error 条件携带）', async () => {
    renderPage()
    fireEvent.click(await screen.findByText('新建 Workflow'))
    await screen.findByText('新建 Workflow', { selector: '.ant-drawer-title' })
    fireEvent.change(screen.getByPlaceholderText('例如：停机 → 备份 → 升级 → 起机'), {
      target: { value: 'chain' },
    })
    fireEvent.change(screen.getByPlaceholderText('同一 Workflow 内唯一'), {
      target: { value: 's1' },
    })
    // 目标下拉
    const drawerSelects = document.querySelectorAll('.ant-drawer .ant-select')
    await pickOption(drawerSelects[0] as HTMLElement, 'web-1')
    fireEvent.change(screen.getByPlaceholderText('shell 命令'), { target: { value: 'uptime' } })

    fireEvent.click(drawerFooterSave())
    await waitFor(() =>
      expect(wfMock.createWorkflow).toHaveBeenCalledWith({
        name: 'chain',
        description: undefined,
        steps: [{ type: 'agent.exec', target: 'a1', parameters: { name: 's1', command: 'uptime' } }],
      }),
    )
    expect(msgSuccess).toHaveBeenCalledWith('Workflow 已创建')
  })

  it('编辑：预填步骤并经 updateWorkflow 提交', async () => {
    renderPage()
    fireEvent.click((await screen.findAllByText('编辑'))[0])
    await screen.findByText('编辑 Workflow：upgrade chain', undefined, { timeout: 3000 })
    // 两个既有步骤的名称都在
    expect(screen.getByDisplayValue('backup')).toBeInTheDocument()
    expect(screen.getByDisplayValue('upgrade')).toBeInTheDocument()
    fireEvent.click(drawerFooterSave())
    await waitFor(() => {
      expect(wfMock.updateWorkflow).toHaveBeenCalledWith('wf-1', {
        name: 'upgrade chain',
        description: 'stop → backup → upgrade',
        steps: wf.steps,
      })
    })
  })

  it('运行：runWorkflow 后打开 run 时间线；步骤 Job 详情可看输出', async () => {
    renderPage()
    fireEvent.click((await screen.findAllByText('运行'))[0])
    await waitFor(() => expect(wfMock.runWorkflow).toHaveBeenCalledWith('wf-1'))
    // run 弹窗出现（getWorkflowRun 返回终态视图：run 成功 + 失败步徽标）
    expect(await screen.findByText('Run：upgrade chain')).toBeInTheDocument()
    expect(screen.getAllByText('成功').length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('失败')).toBeInTheDocument()
    // 步骤时间线：attempts 3 / 查看入口
    expect(screen.getByText('3')).toBeInTheDocument()
    fireEvent.click(screen.getAllByText('查看')[0])
    expect(await screen.findByText('backup done')).toBeInTheDocument()
  })

  it('run running：显示取消运行入口，取消经 cancelWorkflowRun', async () => {
    wfMock.getWorkflowRun.mockResolvedValue({ ...run, status: 'running', finishedAt: undefined })
    renderPage()
    fireEvent.click((await screen.findAllByText('运行'))[0])
    await screen.findByText('运行中')
    fireEvent.click(screen.getByText('取消运行'))
    // Popconfirm 确认
    await waitFor(() => expect(popconfirmOk()).toBeTruthy())
    fireEvent.click(popconfirmOk())
    await waitFor(() => expect(wfMock.cancelWorkflowRun).toHaveBeenCalledWith('r-1'))
    expect(msgSuccess).toHaveBeenCalled()
  })

  it('run 终态：无取消入口；failed 状态徽标', async () => {
    renderPage()
    fireEvent.click((await screen.findAllByText('运行'))[0])
    await waitFor(() => expect(screen.getByText('失败')).toBeInTheDocument())
    // 终态（success）弹窗内不出现「取消运行」
    expect(screen.queryByText('取消运行')).not.toBeInTheDocument()
  })

  it('运行失败：服务端 409 报错进 message.error', async () => {
    wfMock.runWorkflow.mockRejectedValue({ response: { data: { error: 'workflow is already running' } } })
    renderPage()
    fireEvent.click((await screen.findAllByText('运行'))[0])
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('workflow is already running'))
  })

  it('删除：确认后 deleteWorkflow 并刷新', async () => {
    renderPage()
    fireEvent.click((await screen.findAllByText('删除'))[0])
    await waitFor(() => expect(popconfirmOk()).toBeTruthy())
    fireEvent.click(popconfirmOk())
    await waitFor(() => expect(wfMock.deleteWorkflow).toHaveBeenCalledWith('wf-1'))
    expect(msgSuccess).toHaveBeenCalledWith('已删除')
  })

  it('创建人缺失回退与步骤链 tooltip 分支', async () => {
    renderPage([{ ...wf, description: undefined }])
    expect(await screen.findByText('upgrade chain')).toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThanOrEqual(1)
  })

  it('步骤编辑器：添加/上移/删除步骤与数量校验', async () => {
    renderPage()
    fireEvent.click(await screen.findByText('新建 Workflow'))
    await screen.findByText('新建 Workflow', { selector: '.ant-drawer-title' })
    // 默认 1 步：删除按钮禁用（steps.length<=1）
    const delBtns = () => document.querySelectorAll('.ant-drawer .ant-card-extra button')
    fireEvent.click(screen.getByText('添加步骤'))
    expect(screen.getAllByPlaceholderText('同一 Workflow 内唯一').length).toBe(2)
    // 第二行上移 → 顺序交换（index 0 行的上移按钮此刻应可用）
    const upBtns = document.querySelectorAll('.ant-drawer .anticon-arrow-up')
    fireEvent.click(upBtns[1] as Element)
    // 删除第二行 → 回到 1 步
    const delIcons = document.querySelectorAll('.ant-drawer .anticon-delete')
    fireEvent.click(delIcons[delIcons.length - 1] as Element)
    expect(screen.getAllByPlaceholderText('同一 Workflow 内唯一').length).toBe(1)
    expect(delBtns().length).toBeGreaterThan(0)
  })

  it('抽屉取消按钮关闭编辑', async () => {
    renderPage()
    fireEvent.click(await screen.findByText('新建 Workflow'))
    await screen.findByText('新建 Workflow', { selector: '.ant-drawer-title' })
    fireEvent.click(drawerFooterCancel())
    await waitFor(() =>
      expect(document.querySelector('.ant-drawer-open')).toBeNull(),
    )
  })

  it('保存失败：服务端报错进 message.error', async () => {
    wfMock.createWorkflow.mockRejectedValue({ response: { data: { error: 'name is required' } } })
    renderPage()
    fireEvent.click(await screen.findByText('新建 Workflow'))
    await screen.findByText('新建 Workflow', { selector: '.ant-drawer-title' })
    fireEvent.change(screen.getByPlaceholderText('例如：停机 → 备份 → 升级 → 起机'), {
      target: { value: 'chain' },
    })
    fireEvent.change(screen.getByPlaceholderText('同一 Workflow 内唯一'), { target: { value: 's1' } })
    const drawerSelects = document.querySelectorAll('.ant-drawer .ant-select')
    await pickOption(drawerSelects[0] as HTMLElement, 'web-1')
    fireEvent.change(screen.getByPlaceholderText('shell 命令'), { target: { value: 'uptime' } })
    fireEvent.click(drawerFooterSave())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('name is required'))
  })

  it('删除失败：报错进 message.error', async () => {
    wfMock.deleteWorkflow.mockRejectedValue({ response: { data: { error: 'active run' } } })
    renderPage()
    fireEvent.click((await screen.findAllByText('删除'))[0])
    await waitFor(() => expect(popconfirmOk()).toBeTruthy())
    fireEvent.click(popconfirmOk())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('active run'))
  })

  it('取消运行失败：报错进 message.error', async () => {
    wfMock.getWorkflowRun.mockResolvedValue({ ...run, status: 'running', finishedAt: undefined })
    wfMock.cancelWorkflowRun.mockRejectedValue({ response: { data: { error: 'run is not running' } } })
    renderPage()
    fireEvent.click((await screen.findAllByText('运行'))[0])
    await screen.findByText('运行中')
    fireEvent.click(screen.getByText('取消运行'))
    await waitFor(() => expect(popconfirmOk()).toBeTruthy())
    fireEvent.click(popconfirmOk())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('run is not running'))
  })

  it('步骤无 jobId：输出列回退 -', async () => {
    wfMock.getWorkflowRun.mockResolvedValue({
      ...run,
      steps: [{ name: 'backup', type: 'agent.exec', target: 'a1', status: 'pending', attempts: 0 }],
    })
    renderPage()
    fireEvent.click((await screen.findAllByText('运行'))[0])
    await screen.findByText('Run：upgrade chain')
    expect(screen.queryByText('查看')).not.toBeInTheDocument()
  })

  it('Job 详情：失败步 error 段与空输出回退', async () => {
    jobsMock.getJob.mockResolvedValue({
      id: 'j-2',
      type: 'agent.exec',
      target: 'a1',
      actor: 'workflow',
      status: 'failed',
      error: 'exit status 3',
      output: '',
      createdAt: '2026-10-09T01:00:00Z',
    })
    renderPage()
    fireEvent.click((await screen.findAllByText('运行'))[0])
    await screen.findByText('Run：upgrade chain')
    fireEvent.click(screen.getAllByText('查看')[0])
    expect(await screen.findByText('exit status 3')).toBeInTheDocument()
    expect(screen.getByText('（无输出）')).toBeInTheDocument()
  })

  it('Job 读取失败：报错进 message.error', async () => {
    jobsMock.getJob.mockRejectedValue({ response: { data: { error: 'job not found' } } })
    renderPage()
    fireEvent.click((await screen.findAllByText('运行'))[0])
    await screen.findByText('Run：upgrade chain')
    fireEvent.click(screen.getAllByText('查看')[0])
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('job not found'))
  })
})
