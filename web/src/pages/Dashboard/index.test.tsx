import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Dashboard from './index'

// Dashboard：统计卡（showResourceCount 裁剪）+ 健康度分档 + Agent 摘要表
// （离线置底弱化 + 一键清理离线入口）

const apiMock = vi.hoisted(() => ({
  getStatus: vi.fn(),
  getAgents: vi.fn(),
  cleanupAgents: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

// AgentTable 的清理入口按 inventory:write 裁剪（PermGuard → usePerm）
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => true }))

const msgSuccess = vi.spyOn(message, 'success')

const settingsRef = vi.hoisted(() => ({ current: { refreshInterval: 30, showResourceCount: true } }))
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsRef.current }),
}))

const status = {
  services: { running: 5, down: 1, unknown: 0 },
  domains: { valid: 3, expiring: 1 },
  certificates: { valid: 2, expiring: 1 },
  infrastructure: { total: 4, online: 3 },
}

const agents = [
  {
    id: 'ag-1',
    hostname: 'web-01',
    ip: '10.0.0.1',
    region: 'cn-bj', zone: 'z1',
    status: 'online',
    lastSeen: '0',
    capabilities: [
      { type: 'files' },
      { type: 'terminal' },
      { type: 'docker' },
      { type: 'backup' },
    ],
  },
  {
    id: 'ag-2',
    hostname: 'db-01',
    ip: '10.0.0.2',
    region: 'us-la', zone: 'z2',
    status: 'offline',
    lastSeen: '0',
    capabilities: [],
  },
]

const renderPage = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Dashboard />
    </QueryClientProvider>,
  )
}

const rowOf = (name: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(name)) as HTMLTableRowElement

describe('Dashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    settingsRef.current = { refreshInterval: 30, showResourceCount: true }
    apiMock.getStatus.mockReset()
    apiMock.getStatus.mockResolvedValue(status)
    apiMock.getAgents.mockReset()
    apiMock.getAgents.mockResolvedValue(agents)
  })

  it('渲染六张统计卡与健康度', async () => {
    renderPage()
    expect(await screen.findByText('总览')).toBeInTheDocument()
    for (const title of ['运行中服务', '异常服务', '在线 Agent', '有效域名', '有效证书', '即将过期']) {
      expect(screen.getByText(title)).toBeInTheDocument()
    }
    // suffix 与 value 分元素，按卡内整块文本断言（等 status 数据落地）
    await waitFor(() =>
      expect(screen.getByText('在线 Agent').closest('div.stat-card')!.textContent).toContain('3/4'))
    expect(screen.getByText('75% 在线')).toBeInTheDocument()
    // 健康度 Progress 的成功/普通分档：75 → normal（50-79）
    expect(document.querySelector('.ant-progress-status-normal')).toBeInTheDocument()
  })

  it('showResourceCount 关闭时仅展示前四卡', async () => {
    settingsRef.current = { refreshInterval: 30, showResourceCount: false }
    renderPage()
    await screen.findByText('总览')
    await waitFor(() => expect(screen.getByText('运行中服务')).toBeInTheDocument())
    // slice(0,4)：第四卡「有效域名」保留，五六卡被裁
    expect(screen.getByText('有效域名')).toBeInTheDocument()
    expect(screen.queryByText('有效证书')).toBeNull()
    expect(screen.queryByText('即将过期')).toBeNull()
  })

  it('Agent 摘要表：能力截断到 3 并显示 +N', async () => {
    renderPage()
    await screen.findByText('web-01')
    const row = rowOf('web-01')
    expect(within(row).getByText('files')).toBeInTheDocument()
    expect(within(row).getByText('terminal')).toBeInTheDocument()
    expect(within(row).getByText('docker')).toBeInTheDocument()
    expect(within(row).getByText('+1')).toBeInTheDocument()
    expect(within(row).queryByText('backup')).toBeNull()
    expect(within(rowOf('db-01')).getByText('离线')).toBeInTheDocument()
  })

  it('在线率低于 50 时健康度异常分档', async () => {
    apiMock.getStatus.mockResolvedValue({
      ...status,
      infrastructure: { total: 4, online: 1 },
    })
    renderPage()
    expect(await screen.findByText('25% 在线')).toBeInTheDocument()
    expect(document.querySelector('.ant-progress-status-exception')).toBeInTheDocument()
  })

  it('在线率不低于 80 时健康度成功分档', async () => {
    apiMock.getStatus.mockResolvedValue({
      ...status,
      infrastructure: { total: 4, online: 4 },
    })
    renderPage()
    expect(await screen.findByText('100% 在线')).toBeInTheDocument()
    expect(document.querySelector('.ant-progress-status-success')).toBeInTheDocument()
  })

  it('Agent 区域为空：region/zone 回退 -', async () => {
    apiMock.getAgents.mockResolvedValue([
      { id: 'ag-9', hostname: 'bare-01', ip: '10.0.0.9', status: 'online', lastSeen: '0', capabilities: [] },
    ])
    renderPage()
    expect(await screen.findByText('bare-01')).toBeInTheDocument()
    expect(within(rowOf('bare-01')).getByText('-/-')).toBeInTheDocument()
  })

  it('离线置底弱化 + 离线时长标签；在线行不受影响', async () => {
    const ago = (hours: number) => Math.floor(Date.now() / 1000 - hours * 3600)
    apiMock.getAgents.mockResolvedValue([
      { id: 'off-old', hostname: 'stale-01', ip: '10.0.0.3', status: 'offline', lastSeen: ago(5), capabilities: [] },
      { id: 'on-1', hostname: 'live-01', ip: '10.0.0.4', status: 'online', lastSeen: ago(0.01), capabilities: [] },
      { id: 'off-fresh', hostname: 'stale-02', ip: '10.0.0.5', status: 'offline', lastSeen: ago(3), capabilities: [] },
      // 无 lastSeen（排序比较器 a/b 两侧 || 0 兜底分支）：最旧，垫在离线组末尾；
      // 造两台保证互比时两侧回退臂都命中（稳定排序下保持 fixture 顺序）
      { id: 'off-nolast', hostname: 'stale-03', ip: '10.0.0.6', status: 'offline', capabilities: [] },
      { id: 'off-nolast2', hostname: 'stale-04', ip: '10.0.0.7', status: 'offline', capabilities: [] },
    ])
    renderPage()
    expect(await screen.findByText('live-01')).toBeInTheDocument()

    // 在线优先置底：live-01 首行，离线行靠后（last_seen 新的在前，缺失垫底）
    const rows = Array.from(document.querySelectorAll('tr.ant-table-row'))
    expect(rows.findIndex((tr) => tr.textContent?.includes('live-01'))).toBe(0)
    expect(rows.findIndex((tr) => tr.textContent?.includes('stale-02'))).toBe(1)
    expect(rows.findIndex((tr) => tr.textContent?.includes('stale-01'))).toBe(2)
    expect(rows.findIndex((tr) => tr.textContent?.includes('stale-03'))).toBe(3)
    expect(rows.findIndex((tr) => tr.textContent?.includes('stale-04'))).toBe(4)

    // 离线行：时长标签 + 弱化行类；在线行不带弱化类；无 lastSeen 回退纯「离线」
    expect(within(rowOf('stale-03')).getByText('离线')).toBeInTheDocument()
    expect(within(rowOf('stale-01')).getByText('离线 5 小时')).toBeInTheDocument()
    expect(within(rowOf('stale-02')).getByText('离线 3 小时')).toBeInTheDocument()
    expect(rowOf('stale-01').className).toContain('agent-row-offline')
    expect(rowOf('live-01').className).not.toContain('agent-row-offline')
  })

  it('一键清理离线：确认后按档位调用并刷新列表', async () => {
    apiMock.cleanupAgents.mockResolvedValue({ status: 'ok', removed: ['a', 'b'], count: 2 })
    renderPage()
    await screen.findByText('web-01')

    fireEvent.click(screen.getByRole('button', { name: /清理离线/ }))
    expect(screen.getByText('一键清理离线 Agent')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('radio', { name: /离线超过 24 小时/ }))
    fireEvent.click(screen.getByRole('button', { name: /确认清理/ }))

    await waitFor(() =>
      expect(apiMock.cleanupAgents).toHaveBeenCalledWith({ thresholdHours: 24 }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已清理 2 台离线 Agent'))
    // 清理完成后拉回最新列表
    await waitFor(() => expect(apiMock.getAgents.mock.calls.length).toBeGreaterThanOrEqual(2))
  })

  it('刷新按钮触发 refetch', async () => {
    renderPage()
    await screen.findByText('web-01')
    const before = apiMock.getStatus.mock.calls.length
    fireEvent.click(screen.getByRole('button', { name: /刷新/ }))
    await waitFor(() => expect(apiMock.getStatus.mock.calls.length).toBeGreaterThan(before))
  })
})
