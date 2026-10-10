import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ProbePanel from './ProbePanel'
import type { Agent } from '@/types'

// 目标探活面板（B6）：三态徽标/故障中窗口与持续时长/主机名映射/空态与失败提示

const apiMock = vi.hoisted(() => ({
  getProbeTargets: vi.fn(),
  getProbeWindows: vi.fn(),
  getAgents: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const agents = [
  { id: 'ag-1', hostname: 'edge-01', status: 'online' },
  { id: 'ag-2', hostname: 'nas-01', status: 'online' },
] as unknown as Agent[]

const renderPanel = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <ProbePanel />
    </QueryClientProvider>,
  )
}

describe('ProbePanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.getAgents.mockResolvedValue(agents)
  })

  it('渲染目标快照三态徽标与主机名映射、最近错误', async () => {
    apiMock.getProbeTargets.mockResolvedValue({
      targets: [
        {
          id: 't1', agentId: 'ag-1', target: 'blog-http', state: 'healthy',
          since: '2026-10-10T02:00:00Z', lastChecked: '2026-10-10T03:00:00Z', lastError: '',
        },
        {
          id: 't2', agentId: 'ag-2', target: 'pg-tcp', state: 'faulty',
          since: '2026-10-10T02:30:00Z', lastChecked: '2026-10-10T03:00:00Z',
          lastError: 'connection refused',
        },
        {
          id: 't3', agentId: 'ag-x', target: 'cold', state: 'unknown',
          since: '2026-10-10T02:00:00Z', lastChecked: '2026-10-10T02:00:00Z', lastError: '',
        },
      ],
    })
    apiMock.getProbeWindows.mockResolvedValue({ windows: [] })
    renderPanel()

    expect(await screen.findByText('blog-http')).toBeInTheDocument()
    expect(screen.getByText('正常')).toBeInTheDocument()
    expect(screen.getByText('故障')).toBeInTheDocument()
    expect(screen.getByText('冷区')).toBeInTheDocument()
    // 主机名映射：已知 agent 显示 hostname，未知回落 agentId
    expect(screen.getByText('nas-01')).toBeInTheDocument()
    expect(screen.getByText('ag-x')).toBeInTheDocument()
    expect(screen.getByText('connection refused')).toBeInTheDocument()
  })

  it('故障窗口：进行中打「故障中」标，已关窗算持续时长', async () => {
    apiMock.getProbeTargets.mockResolvedValue({ targets: [] })
    apiMock.getProbeWindows.mockResolvedValue({
      windows: [
        {
          id: 'w1', agentId: 'ag-1', target: 'pg-tcp',
          startedAt: '2026-10-10T02:00:00Z', endedAt: null, lastError: 'timeout',
        },
        {
          id: 'w2', agentId: 'ag-1', target: 'blog-http',
          startedAt: '2026-10-10T01:00:00Z', endedAt: '2026-10-10T01:04:30Z', lastError: 'x',
        },
      ],
    })
    renderPanel()

    expect((await screen.findAllByText('pg-tcp')).length).toBeGreaterThan(0)
    expect(screen.getByTestId('window-open')).toBeInTheDocument()
    expect(screen.getByText('4m30s')).toBeInTheDocument()
  })

  it('无目标：空态引导部署探针 agent，窗口表空文案', async () => {
    apiMock.getProbeTargets.mockResolvedValue({ targets: [] })
    apiMock.getProbeWindows.mockResolvedValue({ windows: [] })
    renderPanel()

    expect(
      await screen.findByText(/部署 cockpit-probe-agent 并配置 probe.yaml 后自动上报/),
    ).toBeInTheDocument()
    await waitFor(() => expect(screen.getByText('暂无故障窗口记录')).toBeInTheDocument())
    expect(apiMock.getProbeWindows).toHaveBeenCalledWith({ limit: 50 })
  })

  it('回查失败：警示条出现', async () => {
    apiMock.getProbeTargets.mockRejectedValue(new Error('boom'))
    apiMock.getProbeWindows.mockResolvedValue({ windows: [] })
    renderPanel()

    expect(await screen.findByText('探活数据加载失败')).toBeInTheDocument()
  })

  it('持续时长分档与退化输入：d/h/m/s 档、负差与空输入兜底', async () => {
    // ag-3 存在但无 hostname → 回落 agentId；state 未登记/空串走兜底文案
    apiMock.getAgents.mockResolvedValue([
      { id: 'ag-1', hostname: 'edge-01', status: 'online' },
      { id: 'ag-3', status: 'online' },
    ] as unknown as Agent[])
    apiMock.getProbeTargets.mockResolvedValue({
      targets: [
        {
          id: 't1', agentId: 'ag-3', target: 'no-host', state: '',
          since: '', lastChecked: '', lastError: '',
        },
        {
          id: 't2', agentId: 'ag-1', target: 'weird', state: 'degraded',
          since: '2026-10-10T02:00:00Z', lastChecked: '2026-10-10T03:00:00Z', lastError: '',
        },
      ],
    })
    apiMock.getProbeWindows.mockResolvedValue({
      windows: [
        {
          id: 'w1', agentId: 'ag-1', target: 'd-target',
          startedAt: '2026-10-01T00:00:00Z', endedAt: '2026-10-04T02:00:00Z', lastError: '',
        },
        {
          id: 'w2', agentId: 'ag-1', target: 'h-target',
          startedAt: '2026-10-01T00:00:00Z', endedAt: '2026-10-01T02:05:00Z', lastError: '',
        },
        {
          id: 'w3', agentId: 'ag-1', target: 's-target',
          startedAt: '2026-10-01T00:00:00Z', endedAt: '2026-10-01T00:00:45Z', lastError: '',
        },
        {
          id: 'w4', agentId: 'ag-1', target: 'neg-target',
          startedAt: '2026-10-01T02:00:00Z', endedAt: '2026-10-01T01:00:00Z', lastError: '',
        },
        {
          id: 'w5', agentId: 'ag-1', target: 'blank-target',
          startedAt: '', endedAt: null, lastError: '',
        },
      ],
    })
    renderPanel()

    expect(await screen.findByText('no-host')).toBeInTheDocument()
    expect(screen.getByText('未知')).toBeInTheDocument() // state 空串兜底
    expect(screen.getByText('degraded')).toBeInTheDocument() // 未登记 state 原样呈现
    expect(screen.getByText('3d2h')).toBeInTheDocument()
    expect(screen.getByText('2h5m')).toBeInTheDocument()
    expect(screen.getByText('45s')).toBeInTheDocument()
    // 负差（w4）/空开始（w5）/空时间字段（t1 since/lastChecked）兜底为 —
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(4)
  })

  it('刷新按钮触发目标与窗口两端重查', async () => {
    apiMock.getProbeTargets.mockResolvedValue({ targets: [] })
    apiMock.getProbeWindows.mockResolvedValue({ windows: [] })
    renderPanel()
    await screen.findByText(/部署 cockpit-probe-agent/)
    fireEvent.click(screen.getByLabelText('刷新探活数据'))
    await waitFor(() => {
      expect(apiMock.getProbeTargets.mock.calls.length).toBeGreaterThanOrEqual(2)
      expect(apiMock.getProbeWindows.mock.calls.length).toBeGreaterThanOrEqual(2)
    })
  })

  it('目标先于主机列表返回：Agent 列回落 agentId 不阻塞渲染', async () => {
    apiMock.getProbeTargets.mockResolvedValue({
      targets: [
        {
          id: 't1', agentId: 'ag-9', target: 'early', state: 'healthy',
          since: '2026-10-10T02:00:00Z', lastChecked: '2026-10-10T03:00:00Z', lastError: '',
        },
      ],
    })
    apiMock.getProbeWindows.mockResolvedValue({ windows: [] })
    apiMock.getAgents.mockReturnValue(new Promise(() => {})) // 永不 resolve
    renderPanel()

    expect(await screen.findByText('early')).toBeInTheDocument()
    expect(screen.getByText('ag-9')).toBeInTheDocument()
  })
})
