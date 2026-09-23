import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Workbench from './index'
import type { Agent } from '@/types'

// Workbench：Agent 选择与过滤 / openConnection 三协议分流 /
// 无 agent 与无服务的 warning / Tab 面板挂载
// （AgentSidebar/FileBrowser/LogsPanel/OverviewPanel/三 Modal 均有独立测试，mock 成桩）

const apiMock = vi.hoisted(() => ({ getAgents: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => true }))

const opened = vi.hoisted(() => ({ titles: [] as string[] }))
vi.mock('@/components/TerminalModal', () => ({
  default: (props: { title?: string; onClose?: () => void }) => {
    opened.titles.push(`terminal:${props.title}`)
    return (
      <div data-testid="terminal-modal">
        <button onClick={props.onClose}>close-terminal</button>
      </div>
    )
  },
}))
vi.mock('@/components/DesktopModal', () => ({
  default: (props: { title?: string; onClose?: () => void }) => {
    opened.titles.push(`desktop:${props.title}`)
    return (
      <div data-testid="desktop-modal">
        <button onClick={props.onClose}>close-desktop</button>
      </div>
    )
  },
}))
vi.mock('@/components/VNCModal', () => ({
  default: (props: { title?: string; onClose?: () => void }) => {
    opened.titles.push(`vnc:${props.title}`)
    return (
      <div data-testid="vnc-modal">
        <button onClick={props.onClose}>close-vnc</button>
      </div>
    )
  },
}))
vi.mock('@/components/FileBrowser', () => ({
  default: ({ agentId }: { agentId: string }) => <div data-testid="file-browser">{agentId}</div>,
}))
vi.mock('@/workbench/AgentSidebar', () => ({
  default: ({
    agents,
    selectedAgentId,
    onQueryChange,
    onRefresh,
    onSelect,
  }: {
    agents: Agent[]
    selectedAgentId: string
    onQueryChange: (q: string) => void
    onRefresh: () => void
    onSelect: (id: string) => void
  }) => (
    <div>
      <div data-testid="sidebar-count">{agents.length}</div>
      <div data-testid="sidebar-selected">{selectedAgentId}</div>
      <input
        aria-label="sidebar-query"
        onChange={(e) => onQueryChange(e.target.value)}
      />
      <button onClick={onRefresh}>refresh-agents</button>
      {agents.map((a) => (
        <button key={a.id} onClick={() => onSelect(a.id)}>
          pick-{a.hostname}
        </button>
      ))}
    </div>
  ),
}))
vi.mock('@/workbench/ConnectionPanel', () => ({
  default: ({
    protocol,
    onConnect,
  }: {
    protocol: string
    onConnect: () => void
  }) => (
    <button onClick={onConnect} data-testid={`panel-${protocol}`}>
      connect-{protocol}
    </button>
  ),
}))
vi.mock('@/workbench/OverviewPanel', () => ({
  default: ({ agent }: { agent: Agent | null }) => (
    <div data-testid="overview">{agent?.id ?? 'none'}</div>
  ),
}))
vi.mock('@/workbench/LogsPanel', () => ({
  default: ({ agentId }: { agentId: string }) => <div data-testid="logs-panel">{agentId}</div>,
}))

const msgWarning = vi.spyOn(message, 'warning')

const agents = [
  {
    id: 'ag-1',
    hostname: 'web-01',
    ip: '10.0.0.1',
    region: 'cn-bj',
    status: 'online',
    lastSeen: '0',
    capabilities: [
      {
        type: 'remote-services',
        metadata: {
          ssh: { host: '10.0.0.1', port: 22, name: 'SSH', running: true },
          vnc: { host: '10.0.0.1', port: 5900, name: 'VNC', running: true },
        },
      },
    ],
  },
  {
    id: 'ag-2',
    hostname: 'db-01',
    ip: '10.0.0.2',
    region: 'us-la',
    status: 'online',
    lastSeen: '0',
    capabilities: [],
  },
] as unknown as Agent[]

const renderPage = (data: Agent[] = agents) => {
  apiMock.getAgents.mockResolvedValue(data)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Workbench />
    </QueryClientProvider>,
  )
}

describe('Workbench', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    opened.titles = []
    apiMock.getAgents.mockReset()
  })

  it('无 agent：卡片回退标题、内嵌面板显示 Empty 并提示先选服务器', async () => {
    renderPage([])
    expect(await screen.findByText('工作台')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /SSH/ }))
    })
    expect(msgWarning).toHaveBeenCalledWith('请先选择一台服务器')
    // files/logs Tab 内嵌面板走 Empty 分支（Tabs 懒挂载，激活后才渲染）
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /文\s*件/ }))
    })
    expect(await screen.findByText('暂无可用 Agent')).toBeInTheDocument()
  })

  it('默认选中首个 Agent：概览渲染；切文件/日志 Tab 挂载对应面板', async () => {
    renderPage()
    expect(await screen.findByText('web-01')).toBeInTheDocument() // Card 标题
    expect(screen.getByTestId('overview')).toHaveTextContent('ag-1')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /文\s*件/ }))
    })
    expect(await screen.findByTestId('file-browser')).toHaveTextContent('ag-1')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /日\s*志/ }))
    })
    expect(await screen.findByTestId('logs-panel')).toHaveTextContent('ag-1')
  })

  it('侧栏搜索过滤并支持切换选中', async () => {
    renderPage()
    await screen.findByText('web-01')
    fireEvent.change(screen.getByLabelText('sidebar-query'), { target: { value: 'db' } })
    expect(screen.getByTestId('sidebar-count')).toHaveTextContent('1')
    fireEvent.click(screen.getByText('pick-db-01'))
    expect(screen.getByTestId('sidebar-selected')).toHaveTextContent('ag-2')
    expect(screen.getByTestId('overview')).toHaveTextContent('ag-2')
  })

  it('SSH/VNC 有服务时分流对应 Modal；RDP 无服务告警', async () => {
    renderPage()
    await screen.findByText('web-01')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /SSH/ }))
    })
    await screen.findByTestId('terminal-modal')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /VNC/ }))
    })
    await screen.findByTestId('vnc-modal')
    expect(opened.titles).toEqual(expect.arrayContaining([
      'terminal:SSH - web-01',
      'vnc:VNC - web-01',
    ]))
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /RDP/ }))
    })
    expect(msgWarning).toHaveBeenCalledWith('未检测到可用的 RDP 服务')
    expect(opened.titles.filter((t) => t.startsWith('desktop:'))).toEqual([])
  })

  it('概览/文件/日志按钮只切 Tab 不弹窗', async () => {
    renderPage()
    await screen.findByText('web-01')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /概\s*览/ }))
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /日\s*志/ }))
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /文\s*件/ }))
    })
    expect(msgWarning).not.toHaveBeenCalled()
    expect(document.querySelector('.ant-tabs-tab-active')?.textContent).toContain('文件')
  })

  it('侧栏刷新重查 Agent；Tabs 栏切换走 onChange', async () => {
    renderPage()
    await screen.findByText('web-01')
    const before = apiMock.getAgents.mock.calls.length
    fireEvent.click(screen.getByText('refresh-agents'))
    await waitFor(() => expect(apiMock.getAgents.mock.calls.length).toBeGreaterThan(before))
    // 直接点 Tabs 栏（onChange → setTab）
    fireEvent.click(screen.getByRole('tab', { name: '日志' }))
    expect(document.querySelector('.ant-tabs-tab-active')?.textContent).toContain('日志')
  })

  it('搜索按 id/ip/region 命中；缺 hostname/ip 的 agent 不炸', async () => {
    renderPage([
      {
        id: 'ag-1', hostname: 'web-01', ip: '10.0.0.1', region: 'cn-bj', status: 'online', lastSeen: '0',
        capabilities: [],
      },
      {
        id: 'special-id', status: 'online', lastSeen: '0', capabilities: [],
      },
    ] as unknown as Agent[])
    await screen.findByText('web-01')
    // 按 id 命中（大小写归一）
    fireEvent.change(screen.getByLabelText('sidebar-query'), { target: { value: 'SPECIAL' } })
    expect(screen.getByTestId('sidebar-count')).toHaveTextContent('1')
    // 按 ip 命中
    fireEvent.change(screen.getByLabelText('sidebar-query'), { target: { value: '10.0.0' } })
    expect(screen.getByTestId('sidebar-count')).toHaveTextContent('1')
    // 按 region 命中
    fireEvent.change(screen.getByLabelText('sidebar-query'), { target: { value: 'cn-bj' } })
    expect(screen.getByTestId('sidebar-count')).toHaveTextContent('1')
    // 无命中
    fireEvent.change(screen.getByLabelText('sidebar-query'), { target: { value: 'zzz' } })
    expect(screen.getByTestId('sidebar-count')).toHaveTextContent('0')
  })

  it('RDP 有服务时开 DesktopModal；面板 connect 按钮分流三协议；关闭各自 Modal', async () => {
    renderPage([
      {
        id: 'ag-1',
        hostname: '', // 空 hostname → 标题/会话名回落 id
        ip: '10.0.0.1',
        region: 'cn-bj',
        status: 'online',
        lastSeen: '0',
        capabilities: [
          {
            type: 'remote-services',
            metadata: {
              ssh: { host: '10.0.0.1', port: 22, name: 'SSH', running: true },
              rdp: { host: '10.0.0.1', port: 3389, name: 'RDP', running: true },
              vnc: { host: '10.0.0.1', port: 5900, name: 'VNC', running: true },
            },
          },
          // RDP 入口前置拦截：无 rdp-client capability（stub 构建）时禁用
          { type: 'rdp-client' },
        ],
      },
    ] as unknown as Agent[])
    // 空 hostname 回退 agent id 作卡片标题
    await waitFor(() =>
      expect(document.querySelector('.ant-card-head-title')?.textContent).toContain('ag-1'))
    await screen.findByTestId('overview')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /RDP/ }))
    })
    await screen.findByTestId('desktop-modal')
    expect(opened.titles).toEqual(expect.arrayContaining(['desktop:RDP - ag-1']))
    fireEvent.click(screen.getByText('close-desktop'))
    await waitFor(() => expect(screen.queryByTestId('desktop-modal')).toBeNull())
    // RDP Tab 面板的连接按钮（ConnectionPanel onConnect → openConnection('rdp')）
    await act(async () => {
      fireEvent.click(screen.getByTestId('panel-rdp'))
    })
    await screen.findByTestId('desktop-modal')
    fireEvent.click(screen.getByText('close-desktop'))
    await waitFor(() => expect(screen.queryByTestId('desktop-modal')).toBeNull())
    // SSH/VNC 面板连接按钮（切 Tab 后挂载）
    fireEvent.click(screen.getByRole('tab', { name: 'SSH' }))
    await act(async () => {
      fireEvent.click(await screen.findByTestId('panel-ssh'))
    })
    await screen.findByTestId('terminal-modal')
    fireEvent.click(screen.getByText('close-terminal'))
    await waitFor(() => expect(screen.queryByTestId('terminal-modal')).toBeNull())
    fireEvent.click(screen.getByRole('tab', { name: 'VNC' }))
    await act(async () => {
      fireEvent.click(await screen.findByTestId('panel-vnc'))
    })
    await screen.findByTestId('vnc-modal')
    fireEvent.click(screen.getByText('close-vnc'))
    await waitFor(() => expect(screen.queryByTestId('vnc-modal')).toBeNull())
    expect(msgWarning).not.toHaveBeenCalled()
  })

  it('RDP 无 rdp-client capability：前置拦截提示，不开 DesktopModal', async () => {
    msgWarning.mockClear()
    renderPage([
      {
        id: 'ag-stub',
        hostname: 'stub-host',
        ip: '10.0.0.1',
        region: 'cn-bj',
        status: 'online',
        lastSeen: '0',
        capabilities: [
          {
            type: 'remote-services',
            metadata: {
              rdp: { host: '10.0.0.1', port: 3389, name: 'RDP', running: true },
            },
          },
          // 无 rdp-client：agent 为 stub 构建（未开 -tags rdp）
        ],
      },
    ] as unknown as Agent[])
    // 等 agent 列表就绪（否则 selectedAgent 为空，走「请先选择服务器」分支）
    await waitFor(() => expect(screen.getByTestId('overview')).toHaveTextContent('ag-stub'))
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /RDP/ }))
    })
    expect(msgWarning).toHaveBeenCalledWith(
      expect.stringContaining('RDP 客户端'),
    )
    expect(screen.queryByTestId('desktop-modal')).toBeNull()
  })
})
