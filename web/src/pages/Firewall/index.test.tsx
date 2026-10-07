import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Firewall from './index'
import type { Agent, FirewallChain, FirewallStatus, FirewallTable } from '@/types'

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
    // 总览行出现（异步查询完成后）；web-01 同时在总览行与面板头
    expect(await screen.findByText('nftables')).toBeInTheDocument()
    expect((await screen.findAllByText('web-01')).length).toBeGreaterThan(0)
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
    // Permission denied 同时出现在总览状态列与面板说明（两处）
    expect((await screen.findAllByText(/Permission denied/)).length).toBeGreaterThan(0)
  })

  it('truncated=true：页顶 Alert 提示 4MB 截断', async () => {
    renderPage(() => Promise.resolve({ ...nftStatus, truncated: true }))
    expect(await screen.findByText('web-01 规则集过大，仅显示前 4MB')).toBeInTheDocument()
  })

  it('截断 meta-only（tables 空 + error）：面板显示 error 说明而非误报规则集为空', async () => {
    renderPage(() =>
      Promise.resolve({
        available: true,
        backend: 'nftables',
        backendVersion: 'nftables 1.0.6',
        totalRules: 20000,
        tables: [],
        truncated: true,
        error: 'ruleset output 7698112 bytes exceeds 4194304 limit, summary only',
      }),
    )
    expect((await screen.findAllByText('web-01')).length).toBeGreaterThan(0)
    fireEvent.click(panelHeader('web-01'))
    expect(
      await screen.findByText(/ruleset output 7698112 bytes exceeds 4194304 limit/),
    ).toBeInTheDocument()
    expect(screen.queryByText('规则集为空（未配置任何规则）')).not.toBeInTheDocument()
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
    // 等面板头渲染出来再展开
    expect((await screen.findAllByText('web-01')).length).toBeGreaterThan(0)
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText('cockpit')).toBeInTheDocument()
    expect(await screen.findByText('tcp dport 9000 accept')).toBeInTheDocument()
  })

  it('策略去重（跨表同名基础链取首个）/计数列/非常规策略/无基础链占位', async () => {
    // ag-a：inet/ip 两表都有 input——去重只显首个；output queue 走默认徽标；
    // 规则带 counters 验证「包/字节」列
    const dedupStatus = (): FirewallStatus => ({
      available: true,
      backend: 'nftables',
      totalRules: 1,
      tables: [
        {
          family: 'inet',
          name: 'filter',
          chains: [
            {
              name: 'input',
              policy: 'accept',
              rules: [{ text: 'tcp dport 22 accept', packets: 5, bytes: 10, ownedByCockpit: false }],
            },
            { name: 'output', policy: 'queue', rules: [] },
            { name: 'sanction', rules: [] },
          ],
        },
        { family: 'ip', name: 'filter', chains: [{ name: 'input', policy: 'drop', rules: [] }] },
      ],
      truncated: false,
    })
    // ag-b：只有自定义链（prerouting 无策略）→ 默认策略列占位 —
    const noBaseStatus: FirewallStatus = {
      available: true,
      backend: 'nftables',
      totalRules: 0,
      tables: [{ family: 'inet', name: 'raw', chains: [{ name: 'prerouting', rules: [] }] }],
      truncated: false,
    }
    renderPage((id) => Promise.resolve(id === 'ag-1' ? dedupStatus() : noBaseStatus), [
      mkAgent('ag-1', 'web-01'),
      mkAgent('ag-2', 'db-01'),
    ])
    // 总览先行：去重 / 非常规策略 / 无基础链占位
    expect(await screen.findByText('input accept')).toBeInTheDocument()
    expect(screen.getAllByText('input accept')).toHaveLength(1) // ip/filter input drop 被去重
    expect(screen.getByText('output queue')).toBeInTheDocument()
    expect(screen.getAllByText('—').length).toBeGreaterThan(0) // ag-2 无基础链占位
    // 计数列在面板内：展开 ag-1
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText('5 / 10')).toBeInTheDocument()
  })

  it('面板获取失败提示 + available=false 无后端时的回退文案', async () => {
    renderPage(
      (id) =>
        id === 'ag-1'
          ? Promise.reject(new Error('connection refused'))
          : Promise.resolve({
              available: false,
              backend: '',
              totalRules: 0,
              tables: [],
              truncated: false,
            }),
      [mkAgent('ag-1', 'web-01'), mkAgent('ag-2', 'db-01')],
    )
    // 等面板头渲染出来再展开
    expect((await screen.findAllByText('web-01')).length).toBeGreaterThan(0)
    // 面板查询失败 → 单主机提示
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText('该主机防火墙状态获取失败')).toBeInTheDocument()
    // available=false 且无 backend/error → 通用回退说明
    fireEvent.click(panelHeader('db-01'))
    expect(await screen.findByText('防火墙规则集不可读')).toBeInTheDocument()
    expect(
      await screen.findByText('Agent 需以 root 运行才能读取防火墙规则集'),
    ).toBeInTheDocument()
  })

  it('空规则集/缺 chains/rules 字段回退/空规则文本占位/离线与无主机名 agent', async () => {
    // ag-1：available 但 tables 空 → 面板空态；含缺 chains 的表、缺 rules 的链、空 text 规则
    const sparse: FirewallStatus = {
      available: true,
      backend: 'iptables',
      totalRules: 1,
      tables: [
        { family: 'ipv4', name: 'mangle', chains: undefined as unknown as FirewallTable['chains'] },
        {
          family: 'ipv4',
          name: 'filter',
          chains: [{ name: 'INPUT', rules: undefined as unknown as FirewallChain['rules'] }, { name: 'FORWARD', rules: [{ text: '', ownedByCockpit: false }] }],
        },
      ],
      truncated: false,
    }
    // ag-2：离线（capability 在也不进清单）；ag-3：无 hostname（回退 id 展示）
    const offlineAgent = { ...mkAgent('ag-2', 'gone-01'), status: 'offline' } as Agent
    const noHostAgent = { ...mkAgent('ag-3', ''), status: 'online' } as Agent
    renderPage(
      (id) =>
        Promise.resolve(id === 'ag-1' ? sparse : { ...nftStatus, tables: [] }),
      [mkAgent('ag-1', 'web-01'), offlineAgent, noHostAgent],
    )
    // 离线 agent 不入面板与总览
    expect(await screen.findByText('nftables')).toBeInTheDocument()
    expect(screen.queryByText('gone-01')).toBeNull()
    // 无 hostname 的 agent 回退用 id
    expect(screen.getAllByText('ag-3').length).toBeGreaterThan(0)
    // 展开面板：空链占位与空规则文本占位
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText('0 条规则')).toBeInTheDocument() // rules 缺省 → 0
    expect(await screen.findByText('（无表达式）')).toBeInTheDocument()
    expect(screen.getAllByText('空链').length).toBeGreaterThan(0)
    // 空规则集面板（tables 空 → Empty 引导）
    fireEvent.click(panelHeader('ag-3'))
    expect(await screen.findByText('规则集为空（未配置任何规则）')).toBeInTheDocument()
  })
})
