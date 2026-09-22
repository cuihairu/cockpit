import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import dayjs from 'dayjs'
import AuditLogs from './index'

// AuditLogs：统计卡片成败两态 / 行动与资源列映射 / 详情四分支
// （空、非法 JSON、普通 JSON、远控结构化）/ 快捷筛选 / 导出下载成败 / 搜索与分页

const apiMock = vi.hoisted(() => ({
  getAuditLogs: vi.fn(),
  getAuditStats: vi.fn(),
  exportAuditLogs: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const loggerMock = vi.hoisted(() => ({ logger: { error: vi.fn() } }))
vi.mock('@/utils/logger', () => loggerMock)

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')

const stats = { total_logs: 100, today_logs: 10, failed_logs: 5 }

// 时间列断言只看日期：created_at 取正午 12:00Z，常见时区下渲染同日
const logs = [
  { id: 1, created_at: '2026-01-01T12:00:00Z', username: 'admin', action: 'login', resource: 'user', status: 'success', ip: '10.0.0.1', details: '' },
  { id: 2, created_at: '2026-01-02T12:00:00Z', username: 'ops', action: 'sync', resource: 'widget', status: 'failure', ip: '10.0.0.2', details: 'not-json{' },
  { id: 3, created_at: '2026-01-03T12:00:00Z', username: 'admin', action: 'delete', resource: 'agent', status: 'success', ip: '10.0.0.3', details: '{"note":"extra"}' },
  {
    id: 4,
    created_at: '2026-01-04T12:00:00Z',
    username: 'ops',
    action: 'view',
    resource: 'remote_session',
    status: 'failure',
    ip: '10.0.0.4',
    details: '{"protocol":"ssh","agent_id":"ag-1","host":"web-01","port":22,"egress":"1.2.3.4","reason":"denied","duration":"5s"}',
  },
]

const renderPage = () => {
  apiMock.getAuditLogs.mockResolvedValue({ data: logs, pagination: { total: 42 } })
  apiMock.getAuditStats.mockResolvedValue(stats)
  apiMock.exportAuditLogs.mockResolvedValue(new Blob(['csv']))
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <AuditLogs />
    </QueryClientProvider>,
  )
}

const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(cell)) as HTMLTableRowElement

// RangePicker：聚焦输入展开面板，取可点日期格（title 为 YYYY-MM-DD）
const openRangePicker = async () => {
  const root = document.querySelector('.ant-picker-range') as HTMLElement
  const input = root.querySelector('input') as HTMLInputElement
  fireEvent.mouseDown(root)
  fireEvent.focus(input)
  fireEvent.click(root)
  return await waitFor(() => {
    const cells = Array.from(document.querySelectorAll('td.ant-picker-cell')).filter(
      (c) => c.getAttribute('title') && !c.classList.contains('ant-picker-cell-disabled'))
    if (cells.length < 20) throw new Error('range picker cells not ready')
    return cells as HTMLElement[]
  })
}

const cellAt = (idx: number) => {
  const cells = Array.from(document.querySelectorAll('td.ant-picker-cell')).filter(
    (c) => c.getAttribute('title') && !c.classList.contains('ant-picker-cell-disabled'))
  return cells[idx] as HTMLElement
}

describe('AuditLogs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    loggerMock.logger.error.mockClear()
  })

  it('首屏：统计卡片与表格行渲染，操作/资源列映射', async () => {
    renderPage()
    expect(await screen.findByText('总日志数')).toBeInTheDocument()
    expect(screen.getByText('100')).toBeInTheDocument()
    expect(screen.getByText('10')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
    // 成功率 (100-5)/100 = 95.0%（Statistic 拆 integer/decimal/suffix 节点，断容器文本）
    const rateCard = screen.getByText('成功率').closest('.ant-card')!
    expect(rateCard.textContent).toContain('95.0')
    expect(rateCard.textContent).toContain('%')
    expect(apiMock.getAuditLogs).toHaveBeenCalledWith({}, 1, 20)
    // 已知 action/resource 走映射，未知走原文
    expect(within(rowOf('admin')).getAllByText('登录').length).toBeGreaterThan(0)
    expect(within(rowOf('ops')).getAllByText('sync').length).toBeGreaterThan(0)
    expect(within(rowOf('admin')).getByText('用户')).toBeInTheDocument()
    expect(within(rowOf('ops')).getByText('widget')).toBeInTheDocument()
    expect(screen.getAllByText('成功').length).toBe(2)
    expect(screen.getAllByText('失败').length).toBe(2)
    expect(screen.getByText('共 42 条')).toBeInTheDocument()
  })

  it('统计查询失败：卡片不渲染并记日志', async () => {
    renderPage()
    apiMock.getAuditStats.mockRejectedValue(new Error('stats down'))
    // 触发 refetch 让 error 分支落地（retry:false 下首查失败同样适用）
    await screen.findAllByText('admin')
    fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
    await waitFor(() => expect(loggerMock.logger.error).toHaveBeenCalled())
  })

  it('详情列四分支：空、非法 JSON 原文、普通 JSON 展开、远控结构化', async () => {
    renderPage()
    expect((await screen.findAllByText('admin')).length).toBe(2)
    // 空详情 → '-'
    expect(within(rowOf('10.0.0.1')).getByText('-')).toBeInTheDocument()
    // 非法 JSON → 原文 span
    expect(screen.getByText('not-json{')).toBeInTheDocument()
    // 普通 JSON → pre 展开缩进
    expect(screen.getByText(/"note": "extra"/)).toBeInTheDocument()
    // 远控结构化：协议大写、目标拼端口
    expect(screen.getByText(/SSH/)).toBeInTheDocument()
    expect(screen.getByText(/ag-1/)).toBeInTheDocument()
    expect(screen.getByText(/web-01:22/)).toBeInTheDocument()
    expect(screen.getByText(/1\.2\.3\.4/)).toBeInTheDocument()
    expect(screen.getByText(/denied/)).toBeInTheDocument()
    expect(screen.getByText(/5s/)).toBeInTheDocument()
  })

  it('快捷筛选：远控会话 / 失败远控改写查询参数并重置页码', async () => {
    renderPage()
    expect((await screen.findAllByText('admin')).length).toBe(2)
    fireEvent.click(screen.getByRole('button', { name: '远控会话' }))
    await waitFor(() =>
      expect(apiMock.getAuditLogs).toHaveBeenLastCalledWith({ resource: 'remote_session' }, 1, 20))
    fireEvent.click(screen.getByRole('button', { name: '失败远控' }))
    await waitFor(() =>
      expect(apiMock.getAuditLogs).toHaveBeenLastCalledWith(
        { resource: 'remote_session', status: 'failure' }, 1, 20))
  })

  it('用户名搜索与操作类型下拉改写查询参数', async () => {
    renderPage()
    expect((await screen.findAllByText('admin')).length).toBe(2)
    const input = screen.getByPlaceholderText('搜索用户名')
    fireEvent.change(input, { target: { value: 'ops' } })
    await waitFor(() => expect(apiMock.getAuditLogs).toHaveBeenLastCalledWith({ username: 'ops' }, 1, 20))
    // 操作类型下拉选「登出」
    fireEvent.mouseDown(screen.getByText('操作类型'))
    const opt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent === '登出')
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(opt)
    await waitFor(() =>
      expect(apiMock.getAuditLogs).toHaveBeenLastCalledWith({ username: 'ops', action: 'logout' }, 1, 20))
  })

  it('导出成功：下载链路与成功提示', async () => {
    const createObjectURL = vi.fn(() => 'blob:mock')
    const revokeObjectURL = vi.fn()
    Object.defineProperty(URL, 'createObjectURL', { value: createObjectURL, configurable: true })
    Object.defineProperty(URL, 'revokeObjectURL', { value: revokeObjectURL, configurable: true })
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    renderPage()
    expect((await screen.findAllByText('admin')).length).toBe(2)
    fireEvent.click(screen.getByRole('button', { name: /导\s*出/ }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('导出成功'))
    expect(createObjectURL).toHaveBeenCalled()
    expect(clickSpy).toHaveBeenCalled()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:mock')
    clickSpy.mockRestore()
  })

  it('导出失败：记日志并提示失败', async () => {
    renderPage()
    expect((await screen.findAllByText('admin')).length).toBe(2)
    apiMock.exportAuditLogs.mockRejectedValue(new Error('boom'))
    fireEvent.click(screen.getByRole('button', { name: /导\s*出/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('导出失败'))
    expect(loggerMock.logger.error).toHaveBeenCalled()
  })

  it('分页翻页：第二页参数下发', async () => {
    renderPage()
    expect((await screen.findAllByText('admin')).length).toBe(2)
    fireEvent.click(screen.getByTitle('2'))
    await waitFor(() => expect(apiMock.getAuditLogs).toHaveBeenLastCalledWith({}, 2, 20))
  })

  it('资源类型下拉：改写 resource 并重置页码', async () => {
    renderPage()
    expect((await screen.findAllByText('admin')).length).toBe(2)
    fireEvent.mouseDown(screen.getByText('资源类型'))
    const opt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent === '存储')
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(opt)
    await waitFor(() =>
      expect(apiMock.getAuditLogs).toHaveBeenLastCalledWith({ resource: 'storage' }, 1, 20))
  })

  it('统计：total_logs 为 0 时成功率兑底 100%', async () => {
    apiMock.getAuditLogs.mockResolvedValue({ data: [], pagination: { total: 0 } })
    apiMock.getAuditStats.mockResolvedValue({ total_logs: 0, today_logs: 0, failed_logs: 0 })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <AuditLogs />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('总日志数')).toBeInTheDocument()
    const rateCard = screen.getByText('成功率').closest('.ant-card')!
    expect(rateCard.textContent).toContain('100')
  })

  it('远控详情：仅主机不拼端口，仅端口主机回退 -', async () => {
    apiMock.getAuditLogs.mockResolvedValue({
      data: [
        {
          id: 11, created_at: '2026-01-05T12:00:00Z', username: 'u1', action: 'view',
          resource: 'remote_session', status: 'success', ip: '10.0.0.5',
          details: '{"protocol":"telnet","host":"web-02"}',
        },
        {
          id: 12, created_at: '2026-01-06T12:00:00Z', username: 'u2', action: 'view',
          resource: 'remote_session', status: 'success', ip: '10.0.0.6',
          details: '{"protocol":"ssh","port":2222}',
        },
      ],
      pagination: { total: 2 },
    })
    apiMock.getAuditStats.mockResolvedValue(stats)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <AuditLogs />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('web-02')).toBeInTheDocument()
    // 有 host 无 port：不拼 :port（strong+文本节点拆分，断整行 textContent）
    const r1 = rowOf('web-02')
    expect(r1.textContent).toContain('目标:')
    expect(r1.textContent).toContain('web-02')
    expect(r1.textContent).not.toContain('web-02:')
    // 有 port 无 host：host 回退 '-' 并拼出 :2222
    const r2 = rowOf('10.0.0.6')
    expect(r2.textContent).toContain('目标:')
    expect(r2.textContent).toContain('-:2222')
  })

  it('日期范围：选中写入起止时间，清除还原', async () => {
    renderPage()
    expect((await screen.findAllByText('admin')).length).toBe(2)
    const cells = await openRangePicker()
    const startTitle = cells[5].getAttribute('title')!
    const endTitle = cells[10].getAttribute('title')!
    fireEvent.click(cells[5])
    fireEvent.click(cellAt(10))
    await waitFor(() =>
      expect(apiMock.getAuditLogs).toHaveBeenLastCalledWith({
        start_time: dayjs(startTitle).startOf('day').toISOString(),
        end_time: dayjs(endTitle).endOf('day').toISOString(),
      }, 1, 20))
    fireEvent.click(document.querySelector('.ant-picker-clear')!)
    await waitFor(() => expect(apiMock.getAuditLogs).toHaveBeenLastCalledWith({}, 1, 20))
  })
})
