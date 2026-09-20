import { act, fireEvent, render, screen } from '@testing-library/react'
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
  default: (props: { title?: string }) => {
    opened.titles.push(`terminal:${props.title}`)
    return <div data-testid="terminal-modal" />
  },
}))
vi.mock('@/components/DesktopModal', () => ({
  default: (props: { title?: string }) => {
    opened.titles.push(`desktop:${props.title}`)
    return <div data-testid="desktop-modal" />
  },
}))
vi.mock('@/components/VNCModal', () => ({
  default: (props: { title?: string }) => {
    opened.titles.push(`vnc:${props.title}`)
    return <div data-testid="vnc-modal" />
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
    onSelect,
  }: {
    agents: Agent[]
    selectedAgentId: string
    onQueryChange: (q: string) => void
    onSelect: (id: string) => void
  }) => (
    <div>
      <div data-testid="sidebar-count">{agents.length}</div>
      <div data-testid="sidebar-selected">{selectedAgentId}</div>
      <input
        aria-label="sidebar-query"
        onChange={(e) => onQueryChange(e.target.value)}
      />
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
    location: { region: 'cn-bj' },
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
    location: { region: 'us-la' },
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
      fireEvent.click(screen.getByRole('button', { name: /文\s*件/ }))
    })
    expect(msgWarning).not.toHaveBeenCalled()
    expect(document.querySelector('.ant-tabs-tab-active')?.textContent).toContain('文件')
  })
})
