import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Monitor from './index'

// Monitor：Agent 下拉（默认首个）+ SystemInfoCard/MetricsChart 数据透传 + 刷新

const metricsMock = vi.hoisted(() => ({
  getSystemSnapshots: vi.fn(),
  getSystemSnapshot: vi.fn(),
  getMetricsHistory: vi.fn(),
}))
vi.mock('@/services/metrics', () => ({
  getSystemSnapshots: metricsMock.getSystemSnapshots,
  getSystemSnapshot: metricsMock.getSystemSnapshot,
  getMetricsHistory: metricsMock.getMetricsHistory,
}))
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: { refreshInterval: 30 } }),
}))
// pro-components 与图表/信息卡较重，mock 成透传桩
vi.mock('@ant-design/pro-components', () => ({
  PageContainer: ({ extra, children }: { extra?: React.ReactNode; children?: React.ReactNode }) => (
    <div>
      <div data-testid="page-extra">{extra}</div>
      {children}
    </div>
  ),
  ProCard: ({ title, children }: { title?: React.ReactNode; children?: React.ReactNode }) => (
    <div>
      <div>{title}</div>
      {children}
    </div>
  ),
}))
vi.mock('@/components/SystemInfoCard', () => ({
  default: ({ systemInfo }: { systemInfo?: { agentId: string } }) => (
    <div data-testid="sys-card">{systemInfo?.agentId}</div>
  ),
}))
vi.mock('@/components/MetricsChart', () => ({
  default: ({ title, data }: { title?: string; data: unknown[] }) => (
    <div data-testid="chart">{title}:{data?.length}</div>
  ),
}))

const snapshots = [
  { agentId: 'ag-1', hostname: 'web-01', osName: 'linux', arch: 'amd64' },
  { agentId: 'ag-2', hostname: 'db-01', osName: 'linux', arch: 'arm64' },
]
const history = {
  data: [
    { timestamp: 1, cpuUsage: 10, memUsagePercent: 20, diskUsagePercent: 30, load1: 0.5 },
    { timestamp: 2, cpuUsage: 12, memUsagePercent: 22, diskUsagePercent: 31, load1: 0.6 },
  ],
}

const renderPage = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Monitor />
    </QueryClientProvider>,
  )
}

describe('Monitor', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    metricsMock.getSystemSnapshots.mockReset()
    metricsMock.getSystemSnapshots.mockResolvedValue(snapshots)
    metricsMock.getSystemSnapshot.mockReset()
    metricsMock.getSystemSnapshot.mockImplementation(
      (id: string) => Promise.resolve({ agentId: id, hostname: 'x' }))
    metricsMock.getMetricsHistory.mockReset()
    metricsMock.getMetricsHistory.mockResolvedValue(history)
  })

  it('无在线 Agent：提示且不发详情查询', async () => {
    metricsMock.getSystemSnapshots.mockResolvedValue([])
    renderPage()
    expect(await screen.findByText('暂无在线 Agent')).toBeInTheDocument()
    expect(metricsMock.getSystemSnapshot).not.toHaveBeenCalled()
    expect(metricsMock.getMetricsHistory).not.toHaveBeenCalled()
  })

  it('默认选中首个 Agent：信息卡与四图表透传数据', async () => {
    renderPage()
    expect(await screen.findByTestId('sys-card')).toHaveTextContent('ag-1')
    await waitFor(() => expect(metricsMock.getSystemSnapshot).toHaveBeenCalledWith('ag-1'))
    const charts = await screen.findAllByTestId('chart')
    expect(charts.map((c) => c.textContent)).toEqual([
      'CPU 使用率:2', '内存使用率:2', '磁盘使用率:2', '系统负载 (1分钟):2',
    ])
    expect(await screen.findByText('web-01 (linux amd64)')).toBeInTheDocument()
  })

  it('切换 Agent 后详情查询换目标', async () => {
    renderPage()
    await screen.findByTestId('sys-card')
    fireEvent.mouseDown(document.querySelector('.ant-select-selector')!)
    const opt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent === 'db-01 (linux arm64)')
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(opt)
    await waitFor(() => expect(metricsMock.getSystemSnapshot).toHaveBeenCalledWith('ag-2'))
    expect(await screen.findByTestId('sys-card')).toHaveTextContent('ag-2')
  })

  it('刷新图标触发重拉', async () => {
    renderPage()
    await screen.findByTestId('sys-card')
    const before = metricsMock.getSystemSnapshots.mock.calls.length
    fireEvent.click(document.querySelector('.anticon-reload')!)
    await waitFor(() =>
      expect(metricsMock.getSystemSnapshots.mock.calls.length).toBeGreaterThan(before))
  })
})
