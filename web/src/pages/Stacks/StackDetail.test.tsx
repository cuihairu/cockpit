import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import StackDetail from './StackDetail'
import type { StackView } from '@/types'

// StackDetail：服务表兜底渲染、四类动作下发与 409/通用错误、任务轮询与终态
// （成功/失败弹窗/查询失败）、compose 保存三分支与 .env 草稿、日志条件查询、
// 历史时间线（空态/加载中/未知动作）、running/total 与 serviceOptions 三级回退

const apiMock = vi.hoisted(() => ({
  getStackDetail: vi.fn(),
  getStackTask: vi.fn(),
  getStackCompose: vi.fn(),
  getStackLogs: vi.fn(),
  getStackHistory: vi.fn(),
  stackAction: vi.fn(),
  saveStackCompose: vi.fn(),
  deleteStack: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => true }))

const msgSuccess = vi.spyOn(message, 'success')
const msgInfo = vi.spyOn(message, 'info')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const mkStack = (over: Partial<StackView> = {}): StackView =>
  ({
    agentId: 'ag-1',
    agentName: 'node-01',
    name: 'blog',
    running: 1,
    total: 2,
    lastAction: '',
    lastStatus: '',
    lastDeployedAt: 0,
    online: true,
    ...over,
  }) as unknown as StackView

const renderDetail = (stack: StackView | null = mkStack(), onClose = vi.fn()) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const view = render(
    <QueryClientProvider client={qc}>
      <StackDetail open stack={stack} onClose={onClose} />
    </QueryClientProvider>,
  )
  return { ...view, onClose }
}

// 模拟后端错误（axios.isAxiosError 依据 isAxiosError 标志）
const axiosErr = (status: number, data: unknown) =>
  Object.assign(new Error('Request failed'), { isAxiosError: true, response: { status, data } })

const openDetail = async () => {
  await screen.findByText('Stack 详情 — blog')
}

const drawer = () => document.querySelector('.ant-drawer') as HTMLElement

describe('StackDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.getStackDetail.mockResolvedValue({ running: 1, total: 2, services: [] })
    apiMock.getStackCompose.mockResolvedValue({
      name: 'blog', compose: 'services:\n', env: '', composeFile: '/data/blog/compose.yml', modifiedAt: 0,
    })
    apiMock.getStackLogs.mockResolvedValue({ logs: '' })
    apiMock.getStackHistory.mockResolvedValue({ deployments: [] })
  })

  it('离线 Stack：离线 Tag 且不拉取详情', async () => {
    renderDetail(mkStack({ online: false }))
    await openDetail()
    expect(within(drawer()).getByText('离线')).toBeInTheDocument()
    expect(apiMock.getStackDetail).not.toHaveBeenCalled()
  })

  it('服务表：未知 state 兜底 default 色、空 status 兜底 -', async () => {
    apiMock.getStackDetail.mockResolvedValue({
      running: 1, total: 2,
      services: [
        { name: 'nginx', image: 'nginx:latest', state: 'running', status: 'Up 2 hours', containerId: 'c1' },
        { name: 'weird', image: 'x', state: 'weird-state', status: '', containerId: 'c2' },
      ],
    })
    renderDetail()
    await openDetail()
    await screen.findByText('weird-state')
    const rows = Array.from(drawer().querySelectorAll<HTMLTableRowElement>('tr.ant-table-row'))
    const rWeird = rows.find((tr) => tr.textContent?.includes('weird'))!
    const rNginx = rows.find((tr) => tr.textContent?.includes('nginx'))!
    // 未知 state 无配色 → default；已知 state 用映射色
    expect(within(rWeird).getByText('weird-state').closest('.ant-tag')?.className).toContain('ant-tag-default')
    expect(within(rNginx).getByText('running').closest('.ant-tag')?.className).toContain('ant-tag-green')
    expect(within(rWeird).getAllByText('-').length).toBeGreaterThanOrEqual(1)
    expect(within(rNginx).getByText('Up 2 hours')).toBeInTheDocument()
  })

  it('停止/拉取镜像下发；重启在无运行服务时禁用', async () => {
    apiMock.getStackDetail.mockResolvedValue({ running: 0, total: 1, services: [] })
    apiMock.stackAction.mockResolvedValue({ taskId: 't-1' })
    apiMock.getStackTask.mockResolvedValue({ id: 't-1', status: 'success', log: '' })
    renderDetail()
    await openDetail()
    fireEvent.click(screen.getByRole('button', { name: /停止$/ }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('停止任务执行成功'))
    fireEvent.click(screen.getByRole('button', { name: /拉取镜像$/ }))
    await waitFor(() => expect(apiMock.stackAction).toHaveBeenCalledWith('ag-1', 'blog', 'pull'))
    const restartBtn = screen.getByRole('button', { name: /重启$/ }) as HTMLButtonElement
    expect(restartBtn.disabled).toBe(true)
  })

  it('重启在有运行服务时可下发', async () => {
    apiMock.getStackDetail.mockResolvedValue({ running: 1, total: 1, services: [] })
    apiMock.stackAction.mockResolvedValue({ taskId: 't-1' })
    apiMock.getStackTask.mockResolvedValue({ id: 't-1', status: 'success', log: '' })
    renderDetail()
    await openDetail()
    fireEvent.click(screen.getByRole('button', { name: /重启$/ }))
    await waitFor(() => expect(apiMock.stackAction).toHaveBeenCalledWith('ag-1', 'blog', 'restart'))
  })

  it('动作 409 冲突与通用失败分别提示', async () => {
    apiMock.stackAction.mockRejectedValueOnce(axiosErr(409, { error: 'busy' }))
    renderDetail()
    await openDetail()
    fireEvent.click(screen.getByRole('button', { name: /启动$/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('该 Stack 已有任务在执行，请稍后再试'))
    apiMock.stackAction.mockRejectedValueOnce(new Error('agent down'))
    fireEvent.click(screen.getByRole('button', { name: /启动$/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('操作失败: agent down'))
  })

  it('动作 pending：对应按钮 loading；任务下发后出执行中 Tag', async () => {
    let resolveAction: (v: { taskId: string }) => void = () => {}
    apiMock.stackAction.mockImplementation(
      () => new Promise((r) => { resolveAction = r }),
    )
    renderDetail()
    await openDetail()
    fireEvent.click(screen.getByRole('button', { name: /启动$/ }))
    await waitFor(() => expect(apiMock.stackAction).toHaveBeenCalled())
    // pending：up 按钮 loading（variables.action === 'up'），down 按钮不 loading
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /启动$/ }).className).toContain('ant-btn-loading'))
    expect(screen.getByRole('button', { name: /停止$/ }).className).not.toContain('ant-btn-loading')
    // 任务下发后等待首帧任务数据：执行中 Tag
    let resolveTask: (v: unknown) => void = () => {}
    apiMock.getStackTask.mockImplementation(
      () => new Promise((r) => { resolveTask = r }),
    )
    resolveAction({ taskId: 't-1' })
    await waitFor(() => expect(screen.getByText('任务执行中...')).toBeInTheDocument())
    resolveTask({ id: 't-1', status: 'success', log: '' })
    await waitFor(() => expect(msgInfo).toHaveBeenCalledWith('启动任务已下发，正在执行...'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('启动任务执行成功'))
  })

  it('任务失败：失败提示 + 日志弹窗可手动关闭；delete 失败不关抽屉', async () => {
    const onClose = vi.fn()
    apiMock.deleteStack.mockResolvedValue({ taskId: 't-9' })
    apiMock.getStackTask.mockResolvedValue({ id: 't-9', status: 'failed', log: 'boom: cannot stop' })
    renderDetail(mkStack(), onClose)
    await openDetail()
    fireEvent.click(screen.getByRole('button', { name: /删除 Stack/ }))
    await screen.findByText(/确定要删除「blog」吗/)
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('删除任务执行失败'))
    expect(await screen.findByText('删除任务日志 — blog')).toBeInTheDocument()
    expect(screen.getByText('boom: cannot stop')).toBeInTheDocument()
    // 失败不自动关抽屉
    expect(screen.getByText('Stack 详情 — blog')).toBeInTheDocument()
    // 手动关闭失败日志弹窗（onCancel → failLogDismissed）。
    // jsdom 无真实 CSS 过渡：等 leave-active 后补 transitionend 让 rc-motion 结束离场
    const logModal = screen.getByText('删除任务日志 — blog').closest('.ant-modal')!
    fireEvent.click(logModal.querySelector('.ant-modal-close')!)
    await waitFor(() => expect(logModal.className).toContain('ant-zoom-leave-active'))
    fireEvent.transitionEnd(logModal)
    await waitFor(() => expect(logModal).not.toBeVisible())
  })

  it('任务查询失败：提示且不重试', async () => {
    apiMock.stackAction.mockResolvedValue({ taskId: 't-1' })
    apiMock.getStackTask.mockRejectedValue(new Error('task not found'))
    renderDetail()
    await openDetail()
    fireEvent.click(screen.getByRole('button', { name: /启动$/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('查询任务状态失败: task not found'))
  })

  it('compose Tab：保存成功 created 提示、非 502 失败提示、.env 草稿编辑', async () => {
    apiMock.getStackCompose.mockResolvedValue({
      name: 'blog', compose: 'services:\n  web:\n', env: 'K=V', composeFile: '', modifiedAt: 0,
    })
    apiMock.saveStackCompose.mockResolvedValueOnce({ created: true })
    renderDetail()
    await openDetail()
    fireEvent.click(screen.getByText('compose.yml'))
    await screen.findByDisplayValue(/services:/)
    // composeFile 空 → - 兜底；modifiedAt 0 → 不显示修改时间
    await waitFor(() => {
      const texts = Array.from(drawer().querySelectorAll('.ant-typography')).map((e) => e.textContent ?? '')
      expect(texts.some((t) => t.includes('文件：-'))).toBe(true)
    })
    // .env 折叠面板展开并编辑 → env 草稿随保存提交
    fireEvent.click(screen.getByText('.env 环境变量'))
    const envArea = await screen.findByPlaceholderText('KEY=value（每行一条）')
    fireEvent.change(envArea, { target: { value: 'A=1' } })
    fireEvent.click(screen.getByRole('button', { name: /保存$/ }))
    await waitFor(() => expect(apiMock.saveStackCompose).toHaveBeenCalledWith(
      'ag-1', 'blog', { compose: 'services:\n  web:\n', env: 'A=1' }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('Stack 创建成功'))
    // 非 502 失败
    apiMock.saveStackCompose.mockRejectedValueOnce(new Error('disk full'))
    fireEvent.click(screen.getByRole('button', { name: /保存$/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败: disk full'))
  })

  it('compose 保存 502：YAML 校验失败终端弹窗可关闭', async () => {
    apiMock.saveStackCompose.mockRejectedValue(axiosErr(502, 'yaml: bad indent'))
    renderDetail()
    await openDetail()
    fireEvent.click(screen.getByText('compose.yml'))
    await screen.findByDisplayValue(/services:/)
    fireEvent.click(screen.getByRole('button', { name: /保存$/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('YAML 校验失败，请检查 compose.yml 内容'))
    expect(await screen.findByText('YAML 校验失败 — docker compose config 错误输出')).toBeInTheDocument()
    expect(screen.getByText('yaml: bad indent')).toBeInTheDocument()
    const termModal = screen.getByText('YAML 校验失败 — docker compose config 错误输出').closest('.ant-modal')!
    fireEvent.click(termModal.querySelector('.ant-modal-close')!)
    await waitFor(() => expect(termModal.className).toContain('ant-zoom-leave-active'))
    fireEvent.transitionEnd(termModal)
    await waitFor(() => expect(termModal).not.toBeVisible())
  })

  it('删除 Stack：409 冲突与通用失败提示', async () => {
    apiMock.deleteStack.mockRejectedValueOnce(axiosErr(409, { error: 'busy' }))
    renderDetail()
    await openDetail()
    fireEvent.click(screen.getByRole('button', { name: /删除 Stack/ }))
    await screen.findByText(/确定要删除「blog」吗/)
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('该 Stack 已有任务在执行，请稍后再试'))
    apiMock.deleteStack.mockRejectedValueOnce(new Error('agent gone'))
    fireEvent.click(screen.getByRole('button', { name: /删除 Stack/ }))
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('删除失败: agent gone'))
  })

  it('部署日志 Tab：按服务过滤、切 tail、查询与刷新', async () => {
    apiMock.getStackDetail.mockResolvedValue({
      running: 1, total: 2,
      services: [{ name: 'svc-a', image: 'nginx', state: 'running', status: 'Up', containerId: 'c' }],
    })
    apiMock.getStackLogs.mockResolvedValue({ logs: 'log-line-1' })
    renderDetail()
    await openDetail()
    await screen.findByText('svc-a')
    fireEvent.click(screen.getByText('部署日志'))
    // 服务过滤下拉：选项来自 detail.services
    fireEvent.mouseDown(screen.getByText('全部服务').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    fireEvent.click(await screen.findByText('svc-a', { selector: '.ant-select-item-option-content' }))
    // tail 下拉切 50 行
    fireEvent.mouseDown(screen.getByText('200 行').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    fireEvent.click(await screen.findByText('50 行', { selector: '.ant-select-item-option-content' }))
    fireEvent.click(screen.getByRole('button', { name: /查询$/ }))
    await waitFor(() => expect(apiMock.getStackLogs).toHaveBeenCalledWith(
      'ag-1', 'blog', { service: 'svc-a', tail: 50 }))
    // 等首查渲染完（loading 结束）再点刷新
    await screen.findByText('log-line-1')
    fireEvent.click(within(drawer()).getByRole('button', { name: /刷新$/ }))
    await waitFor(() => expect(apiMock.getStackLogs.mock.calls.length).toBeGreaterThanOrEqual(2))
  })

  it('历史 Tab：空态、加载中文案与未知动作标签兜底', async () => {
    renderDetail()
    await openDetail()
    fireEvent.click(screen.getByText('历史'))
    expect(await screen.findByText('暂无部署记录，启动或停止 Stack 后这里会显示时间线')).toBeInTheDocument()
    // 加载中描述
    let resolveHistory: (v: unknown) => void = () => {}
    apiMock.getStackHistory.mockImplementation(() => new Promise((r) => { resolveHistory = r }))
    fireEvent.click(screen.getByText('服务'))
    fireEvent.click(screen.getByText('历史'))
    expect(await screen.findByText('正在加载部署历史...')).toBeInTheDocument()
    resolveHistory({ deployments: [
      { action: 'custom-op', status: 'success', startedAt: 1759000000, finishedAt: 1759000001 },
    ] })
    // 未知动作直接透出原名
    expect(await screen.findByText('custom-op')).toBeInTheDocument()
    expect(screen.getByText('成功 · 1s')).toBeInTheDocument()
  })

  it('running/total 三级回退：detail → stack → 0/空', async () => {
    // detail 未返回时用 stack 的值（?? 中段）
    let resolveDetail: (v: unknown) => void = () => {}
    apiMock.getStackDetail.mockImplementation(() => new Promise((r) => { resolveDetail = r }))
    const first = renderDetail(mkStack({ running: 3, total: 4 }))
    await openDetail()
    expect(await screen.findByText('3/4 运行中')).toBeInTheDocument()
    // detail 返回后改用 detail 的值（?? 首段）
    resolveDetail({ running: 1, total: 2, services: [] })
    await waitFor(() => expect(screen.getByText('1/2 运行中')).toBeInTheDocument())
    expect(screen.queryByText('3/4 运行中')).not.toBeInTheDocument()
    first.unmount()

    // 双方都无值 → 0/0 不渲染 Tag（?? 末段）
    let resolveDetail2: (v: unknown) => void = () => {}
    apiMock.getStackDetail.mockImplementation(() => new Promise((r) => { resolveDetail2 = r }))
    renderDetail(mkStack({ running: undefined, total: undefined } as unknown as Partial<StackView>))
    await openDetail()
    resolveDetail2({ services: [] })
    await waitFor(() => expect(screen.queryByText(/运行中/)).not.toBeInTheDocument())
  })

  it('stack 与 detail 都无 services：过滤下拉无选项', async () => {
    apiMock.getStackDetail.mockImplementation(() => new Promise(() => {}))
    const { unmount } = renderDetail(mkStack({
      services: [{ name: 'redis', image: 'redis', state: 'running', status: 'Up', containerId: 'c' }],
    }))
    // 先证明 stack.services 会进入选项（?? 中段），再开无 services 的实例
    await openDetail()
    fireEvent.click(screen.getByText('部署日志'))
    fireEvent.mouseDown(screen.getByText('全部服务').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    expect(await screen.findByText('redis', { selector: '.ant-select-item-option-content' })).toBeInTheDocument()
    unmount()

    let resolveDetail2: (v: unknown) => void = () => {}
    apiMock.getStackDetail.mockImplementation(() => new Promise((r) => { resolveDetail2 = r }))
    renderDetail(mkStack({ services: undefined }))
    await openDetail()
    resolveDetail2({ running: 0, total: 0, services: [] })
    fireEvent.click(screen.getByText('部署日志'))
    fireEvent.mouseDown(screen.getByText('全部服务').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    await waitFor(() => expect(document.querySelector('.ant-select-item-option')).toBeNull())
  })

  it('服务 Tab 刷新按钮重查详情', async () => {
    apiMock.getStackDetail.mockResolvedValue({
      running: 1, total: 2,
      services: [{ name: 'svc-a', image: 'nginx', state: 'running', status: 'Up', containerId: 'c' }],
    })
    renderDetail()
    await openDetail()
    // 等详情首查渲染完（按钮 loading 结束）再点刷新
    await screen.findByText('svc-a')
    const before = apiMock.getStackDetail.mock.calls.length
    fireEvent.click(within(drawer()).getByRole('button', { name: /刷新$/ }))
    await waitFor(() => expect(apiMock.getStackDetail.mock.calls.length).toBeGreaterThan(before))
  })

  it('底部「关闭」按钮触发 onClose', async () => {
    const { onClose } = renderDetail()
    await openDetail()
    fireEvent.click(within(drawer()).getByRole('button', { name: /^关\s*闭$/ }))
    expect(onClose).toHaveBeenCalled()
  })
})
