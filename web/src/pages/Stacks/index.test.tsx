import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Stacks from './index'
import type { Agent, StackView } from '@/types'

// Stacks：列表（服务比例/离线/目录告警）、新建（模板填充/名称校验/502 终端弹窗）、
// 详情抽屉（动作下发+任务终态、compose 草稿保存、日志过滤、历史时间线）、RBAC

const apiMock = vi.hoisted(() => ({
  getStacks: vi.fn(),
  getAgents: vi.fn(),
  saveStackCompose: vi.fn(),
  getStackDetail: vi.fn(),
  getStackTask: vi.fn(),
  getStackCompose: vi.fn(),
  getStackLogs: vi.fn(),
  getStackHistory: vi.fn(),
  stackAction: vi.fn(),
  deleteStack: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => canWrite }))

const msgSuccess = vi.spyOn(message, 'success')
const msgInfo = vi.spyOn(message, 'info')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const mkAgent = (id: string, hostname: string, caps: string[], status = 'online'): Agent =>
  ({ id, hostname, ip: '10.0.0.1', location: { region: 'cn', zone: 'z1' }, status, lastSeen: '0',
     capabilities: caps.map((type) => ({ type })) }) as unknown as Agent

const agents = [
  mkAgent('ag-1111111111112', 'node-01', ['docker']),
  mkAgent('ag-2222222222223', 'off-02', ['docker'], 'offline'),
  mkAgent('ag-3', 'plain', ['files']),
]

const stacks = [
  { agentId: 'ag-1111111111112', agentName: 'node-01', name: 'blog', running: 2, total: 2, online: true,
    lastAction: 'up', lastStatus: 'success', lastDeployedAt: 1759000000 },
  { agentId: 'ag-2222222222223', agentName: 'off-02', name: 'kuma', running: 0, total: 1, online: false,
    lastAction: '', lastStatus: '', lastDeployedAt: 0 },
  { agentId: 'ag-1111111111112', agentName: 'node-01', name: 'dev', running: 1, total: 3, online: true,
    lastAction: 'restart', lastStatus: 'failed', lastDeployedAt: 1759000000 },
] as unknown as StackView[]

const renderPage = (stackList = stacks) => {
  apiMock.getAgents.mockResolvedValue(agents)
  apiMock.getStacks.mockResolvedValue({
    stacks: stackList,
    agentInfo: {
      'ag-2222222222223': { dir: '/data/stacks', dirWritable: false, dirError: 'permission denied' },
      'ag-1111111111112': { dir: '/data/stacks', dirWritable: true },
    },
  })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Stacks />
    </QueryClientProvider>,
  )
}

const stackRow = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row'))
    .find((tr) => tr.textContent?.includes(cell)) as HTMLTableRowElement

// 模拟后端 502（axios.isAxiosError 依据 isAxiosError 标志）
const axios502 = (data: string) =>
  Object.assign(new Error('Request failed'), { isAxiosError: true, response: { status: 502, data } })

describe('Stacks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    canWrite = true
  })

  it('列表：服务比例配色、离线置灰行、目录告警与未部署占位', async () => {
    renderPage()
    expect(await screen.findByText('blog')).toBeInTheDocument()
    expect(within(stackRow('blog')).getByText('2/2')).toBeInTheDocument()
    expect(within(stackRow('kuma')).getByText('0/1')).toBeInTheDocument()
    expect(within(stackRow('dev')).getByText('1/3')).toBeInTheDocument()
    expect(within(stackRow('kuma')).getByText('离线')).toBeInTheDocument()
    expect(within(stackRow('kuma')).getByText('未部署')).toBeInTheDocument()
    expect(within(stackRow('blog')).getByText('up')).toBeInTheDocument()
    expect(screen.getByText('部分 Agent 的 stacks 目录异常')).toBeInTheDocument()
    expect(screen.getByText(/permission denied/)).toBeInTheDocument()
    expect(screen.getByText('共 3 个 Stack')).toBeInTheDocument()
    // 离线 Stack 的详情按钮禁用
    const detailBtn = within(stackRow('kuma')).getByRole('button', { name: /详\s*情/ }) as HTMLButtonElement
    expect(detailBtn.disabled).toBe(true)
  })

  it('空态：无 Stack 出引导文案', async () => {
    renderPage([])
    expect(await screen.findByText('暂无 Stack，点击右上角「新建 Stack」开始部署应用')).toBeInTheDocument()
  })

  it('新建：名称校验、模板填充与合法提交', async () => {
    apiMock.saveStackCompose.mockResolvedValue({ created: true })
    renderPage()
    expect(await screen.findByText('blog')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建 Stack/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal-title')?.textContent).toBe('新建 Stack'))
    // 空提交 → 三条必填
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    expect(await screen.findByText('请选择 Agent')).toBeInTheDocument()
    expect(screen.getByText('请输入 Stack 名称')).toBeInTheDocument()
    // 名称 pattern
    fireEvent.change(screen.getByLabelText('Stack 名称'), { target: { value: 'Bad Name!' } })
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    expect(await screen.findByText(/仅支持小写字母、数字/)).toBeInTheDocument()
    expect(apiMock.saveStackCompose).not.toHaveBeenCalled()
    // 选 agent + 模板填充 compose
    // rc-select 监听在 .ant-select-selector 上（父容器事件不冒泡给子），mouseDown 须打 selector
    const openSelect = (ph: string) =>
      fireEvent.mouseDown(screen.getByText(ph).closest('.ant-select')!.querySelector('.ant-select-selector')!)
    openSelect('选择在线的 Docker Agent')
    // option 文本在 .ant-select-item-option-content 子元素内（getByText 只看直接文本子节点）
    fireEvent.click(await screen.findByText(/node-01 \(ag-/, { selector: '.ant-select-item-option-content' }))
    openSelect('选择模板或粘贴已有 compose.yml')
    fireEvent.click(await screen.findByText('WordPress 博客', { selector: '.ant-select-item-option-content' }))
    const composeArea = document.querySelector('.ant-modal textarea') as HTMLTextAreaElement
    await waitFor(() => expect(composeArea.value).toContain('wordpress:latest'))
    fireEvent.change(screen.getByLabelText('Stack 名称'), { target: { value: 'my-blog' } })
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.saveStackCompose).toHaveBeenCalledWith('ag-1111111111112', 'my-blog', {
      compose: expect.stringContaining('wordpress:latest') }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('Stack「my-blog」创建成功，可在详情中启动部署'))
  })

  it('新建 502：YAML 校验失败出终端弹窗', async () => {
    apiMock.saveStackCompose.mockRejectedValue(axios502('yaml: line 3: did not find expected key'))
    renderPage()
    expect(await screen.findByText('blog')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建 Stack/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal-title')?.textContent).toBe('新建 Stack'))
    fireEvent.mouseDown(screen.getByText('选择在线的 Docker Agent').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    fireEvent.click(await screen.findByText(/node-01 \(ag-/, { selector: '.ant-select-item-option-content' }))
    fireEvent.change(screen.getByLabelText('Stack 名称'), { target: { value: 'my-blog' } })
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('YAML 校验失败，请检查 compose.yml 内容'))
    expect(await screen.findByText('YAML 校验失败 — docker compose config 错误输出')).toBeInTheDocument()
    expect(screen.getByText(/yaml: line 3/)).toBeInTheDocument()
  })

  it('详情服务 Tab：状态比例、服务表与日志跳转接线', async () => {
    apiMock.getStackDetail.mockResolvedValue({
      running: 2, total: 2,
      services: [
        { name: 'nginx', image: 'nginx:latest', state: 'running', status: 'Up 2 hours' },
        { name: 'redis', image: 'redis:7', state: 'exited', status: '' },
      ],
    })
    apiMock.getStackLogs.mockResolvedValue({ logs: 'line1\nline2' })
    renderPage()
    expect(await screen.findByText('blog')).toBeInTheDocument()
    fireEvent.click(within(stackRow('blog')).getByRole('button', { name: /详\s*情/ }))
    expect(await screen.findByText('Stack 详情 — blog')).toBeInTheDocument()
    expect(screen.getByText('在线')).toBeInTheDocument()
    expect(screen.getByText('2/2 运行中')).toBeInTheDocument()
    // 服务表：state Tag 与 status 空值兜底（等 detail 数据渲染出行）
    await screen.findByText('redis')
    const svcRow = Array.from(document.querySelectorAll('.ant-drawer tr.ant-table-row'))
      .find((tr) => tr.textContent?.includes('redis')) as HTMLTableRowElement
    expect(within(svcRow).getByText('exited')).toBeInTheDocument()
    expect(within(svcRow).getAllByText('-').length).toBeGreaterThanOrEqual(1)
    // 日志跳转：带 service 的查询 + 终端内容
    fireEvent.click(within(svcRow).getByRole('button', { name: /日\s*志/ }))
    await waitFor(() => expect(apiMock.getStackLogs).toHaveBeenCalledWith(
      'ag-1111111111112', 'blog', { service: 'redis', tail: 200 }))
    expect(await screen.findByText(/line1/)).toBeInTheDocument()
  })

  it('动作下发与任务终态：启动任务成功提示并刷新服务', async () => {
    apiMock.getStackDetail.mockResolvedValue({ running: 2, total: 2, services: [] })
    apiMock.stackAction.mockResolvedValue({ taskId: 't-1' })
    apiMock.getStackTask.mockResolvedValue({ id: 't-1', status: 'success', log: '' })
    renderPage()
    expect(await screen.findByText('blog')).toBeInTheDocument()
    fireEvent.click(within(stackRow('blog')).getByRole('button', { name: /详\s*情/ }))
    await screen.findByText('2/2 运行中')
    // icon 的 aria-label 拼进 accessible name（如 play-circle启动），按后缀匹配
    fireEvent.click(screen.getByRole('button', { name: /启动$/ }))
    await waitFor(() => expect(apiMock.stackAction).toHaveBeenCalledWith('ag-1111111111112', 'blog', 'up'))
    await waitFor(() => expect(msgInfo).toHaveBeenCalledWith('启动任务已下发，正在执行...'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('启动任务执行成功'))
    // 任务结束刷新服务详情（初始 + refetch，具体次数受任务轮询时序影响）
    await waitFor(() => expect(apiMock.getStackDetail.mock.calls.length).toBeGreaterThanOrEqual(2))
  })

  it('删除任务成功后自动关闭抽屉', async () => {
    apiMock.getStackDetail.mockResolvedValue({ running: 0, total: 0, services: [] })
    apiMock.deleteStack.mockResolvedValue({ taskId: 't-2' })
    apiMock.getStackTask.mockResolvedValue({ id: 't-2', status: 'success', log: '' })
    renderPage()
    expect(await screen.findByText('blog')).toBeInTheDocument()
    fireEvent.click(within(stackRow('blog')).getByRole('button', { name: /详\s*情/ }))
    expect(await screen.findByText('Stack 详情 — blog')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /删除 Stack/ }))
    expect(await screen.findByText(/确定要删除「blog」吗/)).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.deleteStack).toHaveBeenCalledWith('ag-1111111111112', 'blog'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('删除任务执行成功'))
    // 抽屉自动关闭
    await waitFor(() => expect(screen.queryByText('Stack 详情 — blog')).not.toBeInTheDocument())
  })

  it('compose Tab：草稿编辑保存与空内容拦截', async () => {
    apiMock.getStackDetail.mockResolvedValue({ running: 0, total: 0, services: [] })
    apiMock.getStackCompose.mockResolvedValue({
      compose: 'services:\n  web:\n    image: nginx\n', env: 'K=V',
      composeFile: '/data/stacks/blog/compose.yml', modifiedAt: 1759000000,
    })
    apiMock.saveStackCompose.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('blog')).toBeInTheDocument()
    fireEvent.click(within(stackRow('blog')).getByRole('button', { name: /详\s*情/ }))
    expect(await screen.findByText('Stack 详情 — blog')).toBeInTheDocument()
    fireEvent.click(screen.getByText('compose.yml'))
    const composeArea = await screen.findByDisplayValue(/image: nginx/)
    // 路径可能拆多个文本节点，遍历抽屉内全部 typography 断言存在
    await waitFor(() => {
      const texts = Array.from(document.querySelectorAll('.ant-drawer .ant-typography'))
        .map((e) => e.textContent ?? '')
      expect(texts.some((t) => t.includes('/data/stacks/blog/compose.yml'))).toBe(true)
    })
    // 清空 → warning 拦截；填回并改 → 保存带 env
    fireEvent.change(composeArea, { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: /保存$/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('compose.yml 内容不能为空'))
    expect(apiMock.saveStackCompose).not.toHaveBeenCalled()
    fireEvent.change(composeArea, { target: { value: 'services:\n  web:\n    image: nginx:1.27\n' } })
    fireEvent.click(screen.getByRole('button', { name: /保存$/ }))
    await waitFor(() => expect(apiMock.saveStackCompose).toHaveBeenCalledWith(
      'ag-1111111111112', 'blog',
      { compose: 'services:\n  web:\n    image: nginx:1.27\n', env: 'K=V' }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('保存成功'))
  })

  it('历史 Tab：时间线三态', async () => {
    apiMock.getStackDetail.mockResolvedValue({ running: 0, total: 0, services: [] })
    apiMock.getStackHistory.mockResolvedValue({ deployments: [
      { action: 'up', status: 'success', startedAt: 1759000000, finishedAt: 1759000060 },
      { action: 'down', status: 'failed', startedAt: 1758000000, finishedAt: 1758000030 },
      { action: 'pull', status: 'running', startedAt: 1757000000, finishedAt: 0 },
    ] })
    renderPage()
    expect(await screen.findByText('blog')).toBeInTheDocument()
    fireEvent.click(within(stackRow('blog')).getByRole('button', { name: /详\s*情/ }))
    expect(await screen.findByText('Stack 详情 — blog')).toBeInTheDocument()
    fireEvent.click(screen.getByText('历史'))
    expect(await screen.findByText('成功 · 60s')).toBeInTheDocument()
    expect(screen.getByText('失败 · 30s')).toBeInTheDocument()
    expect(screen.getByText('执行中 · 进行中')).toBeInTheDocument()
  })

  it('RBAC 无写权限：新建与抽屉内全部写操作隐藏', async () => {
    canWrite = false
    apiMock.getStackDetail.mockResolvedValue({ running: 1, total: 1, services: [] })
    renderPage()
    expect(await screen.findByText('blog')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /新建 Stack/ })).not.toBeInTheDocument()
    fireEvent.click(within(stackRow('blog')).getByRole('button', { name: /详\s*情/ }))
    expect(await screen.findByText('Stack 详情 — blog')).toBeInTheDocument()
    for (const label of [/启动$/, /停止$/, /重启$/, /拉取镜像/]) {
      expect(screen.queryByRole('button', { name: label })).not.toBeInTheDocument()
    }
    expect(screen.queryByRole('button', { name: /删除 Stack/ })).not.toBeInTheDocument()
    // 读操作保留：刷新（列表与抽屉内均可能出现，只断存在）
    expect(screen.getAllByRole('button', { name: /刷新$/ }).length).toBeGreaterThanOrEqual(1)
    fireEvent.click(screen.getByText('compose.yml'))
    await waitFor(() => expect(screen.queryByRole('button', { name: /保存$/ })).not.toBeInTheDocument())
  })
})
