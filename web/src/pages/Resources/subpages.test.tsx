import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Resources from './index'
import ComputeDetail from './ComputeDetail'
import type { ComputeInstance, Domain, Gateway, Service, Storage } from '@/types'

// 资源页六子页：每类独立懒加载页的渲染、列渲染、空值兜底与展开行
// 参照 index.test.tsx 的 apiMock/settings/ProbeHeartbeatCell mock 模式

const apiMock = vi.hoisted(() => ({
  getComputeInstances: vi.fn(),
  getDomains: vi.fn(),
  getCertificates: vi.fn(),
  getServices: vi.fn(),
  getGateways: vi.fn(),
  getStorages: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

vi.mock('@/components/HeartbeatBar/ProbeHeartbeatCell', () => ({
  ProbeHeartbeatCell: ({ resourceType, resourceId }: { resourceType: string; resourceId: string }) => (
    <div data-testid="probe-cell">{`${resourceType}:${resourceId}`}</div>
  ),
}))

const settingsRef = { current: { showResourceCount: true, refreshInterval: 3600 } }
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsRef.current }),
}))

// === 计算实例子页 ComputePage fixture ===
const computeRows = [
  // 满字段 vm：配置列三段全出（|| 左侧全真）、操作列命中 vm 分支
  {
    id: 'c1', name: 'full-vm', type: 'vm', agentId: 'ag-1', region: 'cn', zone: 'z1', status: 'running', cpuCores: 4, memoryMb: 8192, diskGb: 100, ipv4: '10.0.0.1',
  },
  // container 只有内存：cpu/disk 走 || 0 右侧，!cpu 真而 !mem 假（链第二段断）
  {
    id: 'c2', name: 'thin-container', type: 'container', agentId: '', region: '', zone: '', status: 'stopped', memoryMb: 512, ipv4: '',
  },
  // vm 只有磁盘：链走满三段在 !disk 断（— 不出、磁盘段出）
  {
    id: 'c3', name: 'disk-only', type: 'vm', agentId: '', status: 'running', diskGb: 8, ipv4: '',
  },
  // baremetal 全缺省：链全真出「—」；操作列走 else（无动作按钮）
  {
    id: 'c4', name: 'bare', type: 'baremetal', agentId: '', status: 'error', ipv4: '',
  },
] as unknown as ComputeInstance[]

// === 域名子页 DomainsPage fixture ===
const domainRows = [
  { id: 'd1', domain: 'example.com', status: 'active', provider: 'cloudflare', autoRenew: true, expiresAt: '2026-06-01' },
  { id: 'd2', domain: 'old.io', status: 'expired', provider: '', autoRenew: false, expiresAt: '' },
] as unknown as Domain[]

// === 服务子页 ServicesPage fixture ===
const serviceRows = [
  { id: 's1', name: 'api', type: 'http', url: 'https://api.x.com', status: 'up', responseTimeMs: 120 },
  { id: 's2', name: 'db', type: 'tcp', url: '', status: 'degraded', responseTimeMs: 0 },
] as unknown as Service[]

// === 网关子页 GatewaysPage fixture ===
const gatewayRows = [
  { id: 'g1', name: 'gw', type: 'nginx', agentId: 'ag-1', ipv4: '1.2.3.4', upstream: '10.0.0.9', status: 'online' },
  { id: 'g2', name: 'gw2', type: '', agentId: '', ipv4: '', upstream: '', status: '' },
] as unknown as Gateway[]

// === 存储子页 StoragesPage fixture ===
const storageRows = [
  { id: 'st1', name: 'data', type: 'nfs', agentId: 'ag-1', path: '/data', totalGb: 100, usedGb: 40, status: 'online' },
  { id: 'st2', name: 'empty', type: '', agentId: '', path: '', totalGb: 0, usedGb: 0, status: '' },
  { id: 'st3', name: 'half', type: 'ssd', agentId: 'ag-1', path: '/mnt', totalGb: 50, usedGb: 0, status: 'online' },
  { id: 'st4', name: 'odd', type: 'cifs', agentId: '', path: '', totalGb: 0, usedGb: 40, status: '' },
] as unknown as Storage[]

type ApiKey = keyof typeof apiMock

// dataOverride 可替换单个 API 的 resolved 值（renderPage 内设置默认值，故经参数传入）
const renderPage = (dataOverride?: Partial<Record<ApiKey, unknown>>) => {
  const defaults: Record<ApiKey, unknown> = {
    getComputeInstances: { data: computeRows },
    getDomains: { data: domainRows },
    getCertificates: { data: [
      { id: 'x1', domain: 'a.com', status: 'valid', issuer: "Let's Encrypt", autoRenew: true, expiresAt: '' },
      { id: 'x2', domain: 'b.com', status: 'expired', issuer: 'DigiCert', autoRenew: false, expiresAt: '2027-01-15' },
    ] },
    getServices: { data: serviceRows },
    getGateways: { data: gatewayRows },
    getStorages: { data: storageRows },
  }
  for (const key of Object.keys(defaults) as ApiKey[]) {
    apiMock[key].mockResolvedValue(dataOverride?.[key] ?? defaults[key])
  }
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Resources />
    </QueryClientProvider>,
  )
}

// 切换 tab（通过文本匹配 antd TabBar 项，items prop 渲染的标签可能含计数后缀，用部分匹配）
const switchTab = async (label: string) => {
  fireEvent.click(screen.getByText(new RegExp(label)))
  // 等懒挂载的表格出现
  await screen.findAllByText(/.*/, { selector: 'tr.ant-table-row' })
}

// 异步查找表格行（文本可能在 td 内的链接等元素中，先找文本再找最近的 tr）
const rowOf = async (cell: string) => {
  const el = await screen.findByText(cell)
  return el.closest('tr') as HTMLTableRowElement
}

describe('Resources subpages', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    settingsRef.current = { showResourceCount: true, refreshInterval: 3600 }
  })

  // === ComputePage: 六子页 ===
  describe('ComputePage', () => {
    it('渲染计算实例列表', async () => {
      renderPage()
      expect(await screen.findByText(/计算实例/)).toBeInTheDocument()
    })

    it('配置列/操作列类型分支：满字段、部分字段、全缺省与 baremetal', async () => {
      renderPage()
      const r1 = await rowOf('full-vm')
      expect(within(r1).getByText('4 核')).toBeInTheDocument()
      expect(within(r1).getByText('8 GB')).toBeInTheDocument()
      expect(within(r1).getByText('100 GB')).toBeInTheDocument()

      const r2 = await rowOf('thin-container')
      expect(within(r2).getByText('512 MB')).toBeInTheDocument()

      const r3 = await rowOf('disk-only')
      expect(within(r3).getByText('8 GB')).toBeInTheDocument()

      const r4 = await rowOf('bare')
      // 全缺省 → 配置列出「—」；baremetal 走操作列 else 分支（云主机占位按钮）
      expect(within(r4).getByText('—')).toBeInTheDocument()
      // 行内还有展开行按钮
      const actionBtn = (row: HTMLElement) => row.querySelector('td:last-child button') as HTMLElement
      expect(actionBtn(r4)).toBeDisabled()
      // vm/container 行的动作按钮渲染（Tooltip 包装的禁用电源按钮）
      expect(actionBtn(r1)).toBeDisabled()
      expect(actionBtn(r2)).toBeDisabled()
    })

    it('刷新按钮触发 fetchAll（refetch 通道）', async () => {
      renderPage()
      await rowOf('full-vm')
      apiMock.getComputeInstances.mockClear()
      fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
      await waitFor(() => expect(apiMock.getComputeInstances).toHaveBeenCalled())
    })
  })

  // === DomainsPage ===
  describe('DomainsPage', () => {
    it('域名 tab：链接、自动续费与探活接线', async () => {
      renderPage()
      await switchTab('域名')
      expect(await screen.findByText('example.com')).toBeInTheDocument()
      const link = screen.getByText('example.com').closest('a')
      expect(link?.getAttribute('href')).toBe('https://example.com')
      expect(within(await rowOf('example.com')).getByText('是')).toBeInTheDocument()
      expect(within(await rowOf('old.io')).getByText('否')).toBeInTheDocument()
      expect(screen.getAllByTestId('probe-cell').some((el) => el.textContent === 'domain:d1')).toBe(true)
    })

    it('证书 tab：签发者、有效状态、过期时间格式化与自动续费否侧', async () => {
      renderPage()
      await switchTab('证书')
      expect(await screen.findByText('a.com')).toBeInTheDocument()
      expect(screen.getByText("Let's Encrypt")).toBeInTheDocument()
      expect(screen.getByText('valid')).toBeInTheDocument()
      // 非空过期时间 → toLocaleDateString（时区相关，用同一运行时表达式对期望值）
      expect(within(await rowOf('b.com')).getByText(new Date('2027-01-15').toLocaleDateString())).toBeInTheDocument()
      // autoRenew false → 「否」+ default 色
      expect(within(await rowOf('b.com')).getByText('否')).toBeInTheDocument()
    })
  })

  // === ServicesPage ===
  describe('ServicesPage', () => {
    it('服务 tab：URL 链接、响应时间空值兜底', async () => {
      renderPage()
      await switchTab('服务')
      expect(await screen.findByText('api')).toBeInTheDocument()
      expect(screen.getByText('https://api.x.com')).toBeInTheDocument()
      expect(within(await rowOf('db')).getAllByText('-').length).toBeGreaterThanOrEqual(2)
      expect(within(await rowOf('api')).getByText('120ms')).toBeInTheDocument()
    })
  })

  // === GatewaysPage ===
  describe('GatewaysPage', () => {
    it('网关 tab：地址/上游与空状态兜底', async () => {
      renderPage()
      await switchTab('网关')
      expect(await screen.findByText('gw')).toBeInTheDocument()
      expect(within(await rowOf('gw')).getByText('1.2.3.4')).toBeInTheDocument()
      expect(within(await rowOf('gw')).getByText('10.0.0.9')).toBeInTheDocument()
      expect(within(await rowOf('gw2')).getAllByText('-').length).toBeGreaterThanOrEqual(3)
    })
  })

  // === StoragesPage ===
  describe('StoragesPage', () => {
    it('存储 tab：容量拼接、单侧缺省兜底与无容量兜底', async () => {
      renderPage()
      await switchTab('存储')
      expect(await screen.findByText('data')).toBeInTheDocument()
      expect(within(await rowOf('data')).getByText('40 / 100 GB')).toBeInTheDocument()
      // usedGb 为 0 → 「0 / 50 GB」；totalGb 为 0 → 「40 / 0 GB」（|| 0 单侧兜底）
      expect(within(await rowOf('half')).getByText('0 / 50 GB')).toBeInTheDocument()
      expect(within(await rowOf('odd')).getByText('40 / 0 GB')).toBeInTheDocument()
      // 双 0 → '-' 提前返回
      expect(within(await rowOf('empty')).getAllByText('-').length).toBeGreaterThanOrEqual(2)
    })

    it('六个 API 返回 data 缺失：全部兜底空数组，页面空态不崩', async () => {
      renderPage({
        getComputeInstances: { data: null },
        getDomains: {}, // data 为 undefined，同走 || [] 兜底
        getCertificates: { data: null },
        getServices: {},
        getGateways: { data: null },
        getStorages: {},
      })
      expect(await screen.findByText('计算实例 (0)')).toBeInTheDocument()
      for (const label of ['域名 (0)', '证书 (0)', '服务 (0)', '网关 (0)', '存储 (0)']) {
        expect(screen.getByText(label)).toBeInTheDocument()
      }
      expect(document.querySelectorAll('tr.ant-table-row').length).toBe(0)
    })

    it('刷新按钮：六查询重新拉取', async () => {
      renderPage()
      await switchTab('存储')
      expect(await screen.findByText('data')).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
      await vi.waitFor(() => {
        expect(apiMock.getComputeInstances).toHaveBeenCalledTimes(2)
        expect(apiMock.getStorages).toHaveBeenCalledTimes(2)
      })
    })
  })

  // === ComputeDetail 三态 ===
  describe('ComputeDetail', () => {
    const base = {
      id: 'c1', name: 'web-vm', type: 'vm', agentId: 'ag-1', region: 'cn', zone: 'z1', status: 'running',
      cpuCores: 4, memoryMb: 8192, diskGb: 100, ipv4: '10.0.0.1',
    } as unknown as ComputeInstance

    it('满字段：内存 GB 显示、vm/running 配色、标签与时间行', () => {
      render(<ComputeDetail record={{ ...base, labels: { env: 'prod', tier: 'web' }, createdAt: '2026-01-02T03:04:05Z', updatedAt: '2026-02-03T04:05:06Z' } as never} />)
      expect(screen.getByText('c1')).toBeInTheDocument()
      expect(screen.getByText('web-vm')).toBeInTheDocument()
      expect(screen.getByText('VM')).toBeInTheDocument()
      expect(screen.getByText('running')).toBeInTheDocument()
      expect(screen.getByText('ag-1')).toBeInTheDocument()
      expect(screen.getByText('cn/z1')).toBeInTheDocument()
      expect(screen.getByText('4 核')).toBeInTheDocument()
      expect(screen.getByText('8 GB')).toBeInTheDocument()
      expect(screen.getByText('100 GB')).toBeInTheDocument()
      expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
      // 标签行渲染（Object.entries map）
      expect(screen.getByText(/env:/)).toBeInTheDocument()
      expect(screen.getByText(/tier:/)).toBeInTheDocument()
      expect(screen.getByText('创建时间')).toBeInTheDocument()
      expect(screen.getByText('更新时间')).toBeInTheDocument()
      expect(screen.getByText(new Date('2026-01-02T03:04:05Z').toLocaleString())).toBeInTheDocument()
    })

    it('内存 < 1024MB：MB 显示；container/stopped 配色', () => {
      render(<ComputeDetail record={{ ...base, type: 'container', status: 'stopped', memoryMb: 512 } as never} />)
      expect(screen.getByText('CONTAINER')).toBeInTheDocument()
      expect(screen.getByText('stopped')).toBeInTheDocument()
      expect(screen.getByText('512 MB')).toBeInTheDocument()
    })

    it('baremetal + 未知状态：orange/error 配色', () => {
      render(<ComputeDetail record={{ ...base, type: 'baremetal', status: 'unknown' } as never} />)
      expect(screen.getByText('BAREMETAL')).toBeInTheDocument()
      expect(screen.getByText('unknown')).toBeInTheDocument()
    })

    it('缺省兜底：labels 空不渲染标签行、无时间行、空字段显示 —', () => {
      render(<ComputeDetail record={{ ...base, labels: undefined, createdAt: undefined, updatedAt: undefined, type: undefined, status: undefined, agentId: '', region: '', zone: '', cpuCores: 0, memoryMb: 0, diskGb: 0, ipv4: '' } as never} />)
      // labels 空 → 无「标签」项
      expect(screen.queryByText('标签')).not.toBeInTheDocument()
      expect(screen.queryByText('创建时间')).not.toBeInTheDocument()
      expect(screen.queryByText('更新时间')).not.toBeInTheDocument()
      // 多个字段兜底为 —，region/zone -/-
      expect(document.body.textContent).toContain('-')
      expect(document.body.textContent).toContain('—')
      const body = document.body.textContent || ''
      expect(body.includes('-/-') || body.includes('—/—') || body.includes('- / -')).toBeTruthy()
    })
  })
})