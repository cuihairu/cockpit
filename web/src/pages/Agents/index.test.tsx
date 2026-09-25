import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Agents from './index'
import { buildAgentColumns } from './columns'
import type { ReactElement, ReactNode } from 'react'
import type { Agent } from '@/types'

// Agents：搜索/三维筛选（地域/状态/类型）+ 详情链路 + 远程连接分流
// （RDP/VNC/SSH 走 Guacamole Modal、telnet 走 TerminalModal；两者有独立测试，
// 这里 mock 成轻量桩）

const apiMock = vi.hoisted(() => ({ getAgents: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const modalStubs = vi.hoisted(() => ({
  opened: [] as string[],
  closed: [] as string[],
  TerminalModal: vi.fn(),
  GuacamoleModal: vi.fn(),
}))
vi.mock('@/components/TerminalModal', () => ({
  default: (props: { title?: string; agentId?: string; onClose?: () => void }) => {
    const key = `terminal:${props.title}`
    if (!modalStubs.opened.includes(key)) modalStubs.opened.push(key)
    return (
      <div
        data-testid="terminal-modal"
        onClick={() => {
          modalStubs.closed.push('terminal')
          props.onClose?.()
        }}
      />
    )
  },
}))
vi.mock('@/components/GuacamoleModal', () => ({
  default: (props: { title?: string; protocol?: string; onClose?: () => void }) => {
    const key = `guac:${props.protocol}:${props.title}`
    if (!modalStubs.opened.includes(key)) modalStubs.opened.push(key)
    return (
      <div
        data-testid="guac-modal"
        onClick={() => {
          modalStubs.closed.push('guac')
          props.onClose?.()
        }}
      />
    )
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
    region: 'cn-bj', zone: 'z1',
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
    region: 'us-la', zone: 'z2',
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

  it('详情链路：打开详情并从远程服务发起 ssh 连接分流 Guacamole（三协议统一栈）', async () => {
    renderPage()
    await screen.findByText('web-01')
    fireEvent.click(within(rowOf('ag-1')).getByRole('button', { name: '详情' }))
    expect(await screen.findByText('Agent 详情')).toBeInTheDocument()
    fireEvent.click(screen.getByText('conn-ssh'))
    await screen.findByTestId('guac-modal')
    expect(modalStubs.opened).toContain('guac:ssh:SSH - 10.0.0.1:22')
    expect(screen.queryByTestId('terminal-modal')).toBeNull()
  })

  it('刷新按钮触发 refetch', async () => {
    renderPage()
    await screen.findByText('web-01')
    const before = apiMock.getAgents.mock.calls.length
    fireEvent.click(screen.getByRole('button', { name: /刷新/ }))
    await waitFor(() => expect(apiMock.getAgents.mock.calls.length).toBeGreaterThan(before))
  })

  it('类型筛选非 physical 分支按 virtType 命中', async () => {
    renderPage()
    await screen.findByText('web-01')
    fireEvent.mouseDown(screen.getAllByText('筛选类型')[0].closest('.ant-select')!.querySelector('.ant-select-selector')!)
    await pickOption('KVM')
    expect(rowOf('ag-1')).toBeInTheDocument()
    expect(document.querySelectorAll('tr.ant-table-row').length).toBe(1)
  })

  it('地域缺省归入 unknown 选项', async () => {
    apiMock.getAgents.mockResolvedValue([
      { id: 'ag-x', hostname: 'solo', ip: '5.5.5.5', status: 'online', lastSeen: '0', capabilities: [] } as unknown as Agent,
    ])
    renderPage()
    await screen.findByText('solo')
    fireEvent.mouseDown(screen.getAllByText('筛选地域')[0].closest('.ant-select')!.querySelector('.ant-select-selector')!)
    // region 缺省的主机归入 unknown 选项（过滤比较的是原始 region，选项仅展示）
    await pickOption('unknown')
  })

  it('三协议分流打开对应 Modal 且各自 onClose 可关；详情 X 触发 onClose', async () => {
    apiMock.getAgents.mockResolvedValue([
      {
        // 无 id → handleConnect 的 selectedAgent?.id || '' 走兜底空串
        hostname: 'triple',
        ip: '7.7.7.7',
        region: 'cn',
        status: 'online',
        lastSeen: '0',
        virtType: 'kvm',
        virtRole: 'guest',
        capabilities: [
          {
            type: 'remote-services',
            metadata: {
              ssh: { host: '7.7.7.7', port: 22, name: 'SSH', running: true },
              rdp: { host: '7.7.7.7', port: 3389, name: 'RDP', running: true },
              vnc: { host: '7.7.7.7', port: 5900, name: 'VNC', running: true },
            },
          },
        ],
      } as unknown as Agent,
    ])
    renderPage()
    await screen.findByText('triple')
    fireEvent.click(within(rowOf('triple')).getByRole('button', { name: '详情' }))
    expect(await screen.findByText('Agent 详情')).toBeInTheDocument()
    fireEvent.click(screen.getByText('conn-ssh'))
    fireEvent.click(screen.getByText('conn-rdp'))
    fireEvent.click(screen.getByText('conn-vnc'))
    await screen.findByTestId('guac-modal')
    expect(modalStubs.opened).toEqual([
      'guac:ssh:SSH - 7.7.7.7:22',
      'guac:rdp:RDP - 7.7.7.7:3389',
      'guac:vnc:VNC - 7.7.7.7:5900',
    ])
    // 三个 Modal 的 onClose 各自触发（visible 置 false 的 setter）
    fireEvent.click(screen.getAllByTestId('guac-modal')[0])
    fireEvent.click(screen.getAllByTestId('guac-modal')[0])
    fireEvent.click(screen.getAllByTestId('guac-modal')[0])
    expect(modalStubs.closed).toEqual(['guac', 'guac', 'guac'])
    // 详情 Modal 的 X → onClose
    fireEvent.click(document.querySelector('.ant-modal-close')!)
  })

  it('telnet 仍走 TerminalModal（D1：telnet 不进 guacd 栈）', async () => {
    apiMock.getAgents.mockResolvedValue([
      {
        id: 'ag-telnet',
        hostname: 'legacy',
        ip: '7.7.7.8',
        region: 'cn',
        status: 'online',
        lastSeen: '0',
        virtType: 'kvm',
        virtRole: 'guest',
        capabilities: [
          {
            type: 'remote-services',
            metadata: {
              telnet: { host: '7.7.7.8', port: 23, name: 'TELNET', running: true },
            },
          },
        ],
      } as unknown as Agent,
    ])
    renderPage()
    await screen.findByText('legacy')
    fireEvent.click(within(rowOf('legacy')).getByRole('button', { name: '详情' }))
    expect(await screen.findByText('Agent 详情')).toBeInTheDocument()
    fireEvent.click(screen.getByText('conn-telnet'))
    await screen.findByTestId('terminal-modal')
    expect(modalStubs.opened).toContain('terminal:TELNET - 7.7.7.8:23')
    expect(screen.queryByTestId('guac-modal')).toBeNull()
    // onClose 回落（关掉 TerminalModal）
    fireEvent.click(screen.getByTestId('terminal-modal'))
    expect(modalStubs.closed).toContain('terminal')
  })

  it('列定义：三个 sorter 三态（含空值兜底）与能力 +N tooltip 分支', () => {
    const cols = buildAgentColumns({ onShowDetail: vi.fn() })
    const byTitle = (title: string) => cols.find((c) => (c as { title: string }).title === title)!
    const sorterOf = (title: string) => byTitle(title).sorter as (a: Agent, b: Agent) => number
    const host = sorterOf('主机名')
    expect(host({ hostname: 'a' } as Agent, { hostname: 'b' } as Agent)).toBeLessThan(0)
    expect(host({ hostname: '' } as Agent, { hostname: 'b' } as Agent)).toBeLessThan(0)
    const virt = sorterOf('类型')
    expect(virt({ virtType: 'kvm' } as Agent, { virtType: 'none' } as Agent)).toBeLessThan(0)
    expect(virt({ virtType: '' } as Agent, { virtType: 'kvm' } as Agent)).toBeLessThan(0)
    const status = sorterOf('状态')
    expect(status({ status: 'offline' } as Agent, { status: 'online' } as Agent)).toBeLessThan(0)
    expect(status({ status: 'online' } as Agent, { status: 'offline' } as Agent)).toBeGreaterThan(0)
    expect(status({ status: 'online' } as Agent, { status: 'online' } as Agent)).toBe(0)

    // 能力列：>3 渲染 +N 溢出 Tag（Tooltip 文案为余下能力逗号拼接）
    const capRender = byTitle('能力').render as (caps: Agent['capabilities']) => ReactNode
    const four = render(capRender([
      { type: 'file' }, { type: 'exec' }, { type: 'docker' }, { type: 'nas' },
    ] as Agent['capabilities']) as ReactElement)
    expect(four.container.textContent).toContain('+1')
    const three = render(capRender([
      { type: 'file' }, { type: 'exec' }, { type: 'docker' },
    ] as Agent['capabilities']) as ReactElement)
    expect(three.container.textContent).not.toContain('+')
  })

  it('列定义 sorter 空值另一侧：b 侧缺省与 a.status 缺省短路', () => {
    const cols = buildAgentColumns({ onShowDetail: vi.fn() })
    const sorterOf = (title: string) =>
      cols.find((c) => (c as { title: string }).title === title)!.sorter as (a: Agent, b: Agent) => number
    expect(sorterOf('主机名')({ hostname: 'b' } as Agent, { hostname: undefined } as unknown as Agent)).toBeGreaterThan(0)
    expect(sorterOf('类型')({ virtType: 'kvm' } as Agent, { virtType: undefined } as unknown as Agent)).toBeGreaterThan(0)
    // a.status 缺省走可选链短路，返回 undefined
    expect(sorterOf('状态')({ status: undefined } as unknown as Agent, { status: 'online' } as Agent)).toBeUndefined()
    expect(sorterOf('状态')({ status: 'online' } as Agent, { status: undefined } as unknown as Agent)).toBeGreaterThan(0)
  })

  it('渲染缺口侧：空字段占位、未知状态回退、相对时间分支与能力 +N', async () => {
    const now = Math.floor(Date.now() / 1000)
    apiMock.getAgents.mockResolvedValue([
      {
        id: 'ag-e', hostname: '', ip: '', region: '', zone: '',
        status: 'weird', lastSeen: '', capabilities: undefined,
      },
      {
        id: 'ag-t', hostname: 'just-now', ip: '1.1.1.4', region: 'r', zone: 'z',
        status: 'online', lastSeen: String(now),
        virtType: 'kvm', virtRole: 'guest', capabilities: [],
      },
      {
        id: 'ag-m', hostname: 'half-hour', ip: '1.1.1.5', region: 'r', zone: 'z',
        status: 'offline', lastSeen: String(now - 1800),
        virtType: 'kvm', virtRole: 'guest', capabilities: [],
      },
      {
        id: 'ag-h', hostname: 'five-hours', ip: '1.1.1.6', region: 'r', zone: 'z',
        status: 'offline', lastSeen: String(now - 18000),
        virtType: 'kvm', virtRole: 'guest', capabilities: [],
      },
      {
        id: 'ag-c', hostname: 'cap-five', ip: '1.1.1.7', region: 'r', zone: 'z',
        status: 'online', lastSeen: String(now),
        virtRole: 'host',
        capabilities: [
          { type: 'remote-services' }, { type: 'files' }, { type: 'exec' },
          { type: 'docker' }, { type: 'nas' },
        ],
      },
    ] as unknown as Agent[])
    renderPage()
    await screen.findByText('just-now')

    // 空 hostname/ip/zone/lastSeen 各渲染占位 '-'
    const emptyRow = rowOf('ag-e')
    expect(within(emptyRow).getAllByText('-')).toHaveLength(4)
    // 未知 status 回退 offline 配置文案
    expect(within(emptyRow).getByText('离线')).toBeInTheDocument()
    // capabilities 为 undefined：能力列无 Tag；virtType/virtRole 缺省类型列走「未知」
    expect(within(emptyRow).queryByText('remote-services')).not.toBeInTheDocument()
    expect(within(emptyRow).getByText('未知')).toBeInTheDocument()

    expect(within(rowOf('ag-t')).getByText('刚刚')).toBeInTheDocument()
    expect(within(rowOf('ag-m')).getByText('30 分钟前')).toBeInTheDocument()
    expect(within(rowOf('ag-h')).getByText('5 小时前')).toBeInTheDocument()

    // >3 capabilities 渲染 +N 溢出 Tag；virtRole host 显示物理机
    const capRow = rowOf('ag-c')
    expect(within(capRow).getByText('+2')).toBeInTheDocument()
    expect(within(capRow).getByText('物理机')).toBeInTheDocument()
  })
})
