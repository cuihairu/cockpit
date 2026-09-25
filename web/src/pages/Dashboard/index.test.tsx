import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Dashboard from './index'

// Dashboard：统计卡（showResourceCount 裁剪）+ 健康度分档 + Agent 摘要表

const apiMock = vi.hoisted(() => ({ getStatus: vi.fn(), getAgents: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

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

  it('刷新按钮触发 refetch', async () => {
    renderPage()
    await screen.findByText('web-01')
    const before = apiMock.getStatus.mock.calls.length
    fireEvent.click(screen.getByRole('button', { name: /刷新/ }))
    await waitFor(() => expect(apiMock.getStatus.mock.calls.length).toBeGreaterThan(before))
  })
})
