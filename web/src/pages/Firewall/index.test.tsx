import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Firewall from './index'
import type { Agent, FirewallStatus } from '@/types'

// Firewall（防火墙观测）：三态渲染（无 capability 空态 / available=false 说明态 /
// 正常总览+规则明细）、truncated Alert、默认策略徽标、cockpit 规则标注

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getFirewallStatus: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const mkAgent = (id: string, hostname: string, fw = true): Agent =>
  ({
    id,
    hostname,
    ip: '1.2.3.4',
    status: 'online',
    lastSeen: '0',
    capabilities: fw ? [{ type: 'firewall' }] : [{ type: 'files' }],
  }) as unknown as Agent

const nftStatus: FirewallStatus = {
  available: true,
  backend: 'nftables',
  backendVersion: 'nft 1.0.9',
  totalRules: 2,
  tables: [
    {
      family: 'inet',
      name: 'filter',
      chains: [
        {
          name: 'input',
          policy: 'accept',
          rules: [
            { handle: 12, text: 'tcp dport 9000 accept', packets: 123, bytes: 4567, ownedByCockpit: false },
            { handle: 13, text: 'tcp dport 22 accept # cockpit:allow-ssh', ownedByCockpit: true },
          ],
        },
        { name: 'forward', policy: 'drop', rules: [] },
      ],
    },
  ],
  truncated: false,
}

const renderPage = (statusImpl?: (id: string) => Promise<FirewallStatus>, agents?: Agent[]) => {
  apiMock.getAgents.mockResolvedValue(agents ?? [mkAgent('ag-1', 'web-01')])
  apiMock.getFirewallStatus.mockImplementation(
    statusImpl ?? (() => Promise.resolve(nftStatus)),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Firewall />
    </QueryClientProvider>,
  )
}

// 总览行是异步拉取后渲染：先等行出现再做同步断言
const panelHeader = (name: string) =>
  Array.from(document.querySelectorAll('.ant-collapse-header')).find(
    (h) => h.textContent === name) as HTMLElement

describe('Firewall', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('正常态：总览行（后端/版本/规则数/默认策略徽标），truncated=false 无告警', async () => {
    renderPage()
    // 总览行出现（异步查询完成后）
    expect(await screen.findByText('nftables')).toBeInTheDocument()
    expect(await screen.findByText('web-01')).toBeInTheDocument()
    expect(screen.getByText('nft 1.0.9')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument() // 规则总数
    expect(screen.getByText('input accept')).toBeInTheDocument()
    expect(screen.getByText('forward drop')).toBeInTheDocument()
    expect(screen.getByText('正常')).toBeInTheDocument()
    // truncated=false 不出页顶告警
    expect(screen.queryByText(/规则集过大/)).toBeNull()
  })

  it('iptables 后端：变体标注 + 部分读取提示（error 非空）+ 面板规则明细', async () => {
    renderPage(() =>
      Promise.resolve({
        available: true,
        backend: 'iptables',
        iptablesVariant: 'nf_tables',
        totalRules: 1,
        tables: [
          {
            family: 'ipv4',
            name: 'filter',
            chains: [
              {
                name: 'INPUT',
                policy: 'ACCEPT',
                rules: [{ text: '-p tcp -m tcp --dport 9000 -j ACCEPT', ownedByCockpit: false }],
              },
            ],
          },
        ],
        truncated: false,
        error: 'ip6tables: exit 4',
      }),
    )
    expect(await screen.findByText('iptables')).toBeInTheDocument()
    expect(screen.getByText('nf_tables')).toBeInTheDocument()
    expect(screen.getByText('input accept')).toBeInTheDocument() // 大写归一
    expect(screen.getByText('部分读取')).toBeInTheDocument()
    // 展开面板：error 说明 + 规则明细
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText('-p tcp -m tcp --dport 9000 -j ACCEPT')).toBeInTheDocument()
    expect(await screen.findByText('ip6tables: exit 4')).toBeInTheDocument()
  })

  it('available=false：说明态（error 原因），总览状态列提示不可读', async () => {
    renderPage(() =>
      Promise.resolve({
        available: false,
        backend: 'iptables',
        totalRules: 0,
        tables: [],
        truncated: false,
        error: 'iptables-save: Permission denied (you must be root)',
      }),
    )
    // 总览行状态列（异步出现）
    expect(await screen.findByText(/不可读/)).toBeInTheDocument()
    // 面板说明态
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText(/防火墙规则集不可读（iptables）/)).toBeInTheDocument()
    expect(await screen.findByText(/Permission denied/)).toBeInTheDocument()
  })

  it('truncated=true：页顶 Alert 提示 4MB 截断', async () => {
    renderPage(() => Promise.resolve({ ...nftStatus, truncated: true }))
    expect(await screen.findByText('web-01 规则集过大，仅显示前 4MB')).toBeInTheDocument()
  })

  it('无防火墙主机：空态且不发状态查询', async () => {
    renderPage(undefined, [mkAgent('ag-9', 'win-01', false)])
    expect(
      await screen.findByText('暂无带防火墙工具的在线主机——装有 nftables 或 iptables 的 Linux 主机运行 Agent 即可'),
    ).toBeInTheDocument()
    expect(apiMock.getFirewallStatus).not.toHaveBeenCalled()
  })

  it('cockpit 名下规则带标注', async () => {
    renderPage()
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText('cockpit')).toBeInTheDocument()
    expect(await screen.findByText('tcp dport 9000 accept')).toBeInTheDocument()
  })
})
