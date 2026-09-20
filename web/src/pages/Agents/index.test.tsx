import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Agents from './index'
import type { Agent } from '@/types'

// Agents：搜索/三维筛选（地域/状态/类型）+ 详情链路 + 远程连接三协议分流
// （Terminal/Desktop/VNC Modal 有独立测试，mock 成轻量桩）

const apiMock = vi.hoisted(() => ({ getAgents: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const modalStubs = vi.hoisted(() => ({
  opened: [] as string[],
  TerminalModal: vi.fn(),
  DesktopModal: vi.fn(),
  VNCModal: vi.fn(),
}))
vi.mock('@/components/TerminalModal', () => ({
  default: (props: { title?: string; agentId?: string }) => {
    modalStubs.opened.push(`terminal:${props.title}`)
    return <div data-testid="terminal-modal" />
  },
}))
vi.mock('@/components/DesktopModal', () => ({
  default: (props: { title?: string }) => {
    modalStubs.opened.push(`desktop:${props.title}`)
    return <div data-testid="desktop-modal" />
  },
}))
vi.mock('@/components/VNCModal', () => ({
  default: (props: { title?: string }) => {
    modalStubs.opened.push(`vnc:${props.title}`)
    return <div data-testid="vnc-modal" />
  },
}))
vi.mock('@/components/RemoteServices', () => ({
  default: ({
    services,
    onConnect,
  }: {
    services: Array<{ protocol: string; host: string; port: number }>
    onConnect: (p: string, h: string, port: number) => void
  }) => (
    <div>
      {services.map((s) => (
        <button key={s.protocol} onClick={() => onConnect(s.protocol, s.host, s.port)}>
          conn-{s.protocol}
        </button>
      ))}
    </div>
  ),
}))

const agents = [
  {
    id: 'ag-1',
    hostname: 'web-01',
    ip: '10.0.0.1',
    location: { region: 'cn-bj', zone: 'z1' },
    status: 'online',
    lastSeen: '0',
    virtType: 'kvm',
    virtRole: 'guest',
    capabilities: [
      {
        type: 'remote-services',
        metadata: { ssh: { host: '10.0.0.1', port: 22, name: 'SSH', running: true } },
      },
      { type: 'files' },
    ],
  },
  {
    id: 'ag-2',
    hostname: 'db-01',
    ip: '192.168.1.9',
    location: { region: 'us-la', zone: 'z2' },
    status: 'offline',
    lastSeen: '0',
    virtType: 'none',
    virtRole: 'host',
    capabilities: [],
  },
] as unknown as Agent[]

const renderPage = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Agents />
    </QueryClientProvider>,
  )
}

const rowOf = (id: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(id)) as HTMLTableRowElement

const pickOption = async (label: string) => {
  const opt = await waitFor(() => {
    const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
      (o) => o.textContent === label)
    if (!el) throw new Error(`option not found: ${label}`)
    return el as HTMLElement
  })
  fireEvent.click(opt)
}

describe('Agents', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    modalStubs.opened = []
    apiMock.getAgents.mockReset()
    apiMock.getAgents.mockResolvedValue(agents)
  })

  it('列表渲染：类型/状态/能力列', async () => {
    renderPage()
    await screen.findByText('web-01') // 数据行渲染完成
    const kvm = rowOf('ag-1')
    expect(within(kvm).getByText('KVM')).toBeInTheDocument()
    expect(within(kvm).getByText('在线')).toBeInTheDocument()
    expect(within(kvm).getByText('remote-services')).toBeInTheDocument()
    expect(within(kvm).getByText('files')).toBeInTheDocument()
    const physical = rowOf('ag-2')
    expect(within(physical).getByText('物理机')).toBeInTheDocument()
    expect(within(physical).getByText('离线')).toBeInTheDocument()
  })

  it('搜索按主机名/IP/Agent ID 过滤', async () => {
    renderPage()
    await screen.findByText('web-01')
    fireEvent.change(screen.getByPlaceholderText('搜索主机名、IP 或 Agent ID'), { target: { value: 'db' } })
    expect(rowOf('ag-2')).toBeInTheDocument()
    expect(document.querySelectorAll('tr.ant-table-row').length).toBe(1)
    fireEvent.change(screen.getByPlaceholderText('搜索主机名、IP 或 Agent ID'), { target: { value: '10.0.0.1' } })
    expect(rowOf('ag-1')).toBeInTheDocument()
    expect(document.querySelectorAll('tr.ant-table-row').length).toBe(1)
    fireEvent.change(screen.getByPlaceholderText('搜索主机名、IP 或 Agent ID'), { target: { value: 'ag-2' } })
    expect(rowOf('ag-2')).toBeInTheDocument()
  })

  it('状态筛选：仅剩离线行', async () => {
    renderPage()
    await screen.findByText('web-01')
    fireEvent.mouseDown(screen.getAllByText('筛选状态')[0].closest('.ant-select')!.querySelector('.ant-select-selector')!)
    await pickOption('离线')
    expect(rowOf('ag-2')).toBeInTheDocument()
    expect(document.querySelectorAll('tr.ant-table-row').length).toBe(1)
  })

  it('类型筛选：physical 命中 host 角色与 none 类型', async () => {
    renderPage()
    await screen.findByText('web-01')
    fireEvent.mouseDown(screen.getAllByText('筛选类型')[0].closest('.ant-select')!.querySelector('.ant-select-selector')!)
    await pickOption('物理机')
    expect(rowOf('ag-2')).toBeInTheDocument()
    expect(document.querySelectorAll('tr.ant-table-row').length).toBe(1)
  })

  it('地域筛选 options 由数据推导并生效', async () => {
    renderPage()
    await screen.findByText('web-01')
    fireEvent.mouseDown(screen.getAllByText('筛选地域')[0].closest('.ant-select')!.querySelector('.ant-select-selector')!)
    await pickOption('cn-bj')
    expect(rowOf('ag-1')).toBeInTheDocument()
    expect(document.querySelectorAll('tr.ant-table-row').length).toBe(1)
  })

  it('详情链路：打开详情并从远程服务发起 ssh 连接分流 Terminal', async () => {
    renderPage()
    await screen.findByText('web-01')
    fireEvent.click(within(rowOf('ag-1')).getByRole('button', { name: '详情' }))
    expect(await screen.findByText('Agent 详情')).toBeInTheDocument()
    fireEvent.click(screen.getByText('conn-ssh'))
    await screen.findByTestId('terminal-modal')
    expect(modalStubs.opened).toContain('terminal:SSH - 10.0.0.1:22')
  })

  it('刷新按钮触发 refetch', async () => {
    renderPage()
    await screen.findByText('web-01')
    const before = apiMock.getAgents.mock.calls.length
    fireEvent.click(screen.getByRole('button', { name: /刷新/ }))
    await waitFor(() => expect(apiMock.getAgents.mock.calls.length).toBeGreaterThan(before))
  })
})
