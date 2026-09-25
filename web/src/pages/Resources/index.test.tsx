import { fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Resources, { activeKeyFallback } from './index'
import type { Certificate, ComputeInstance, Domain, Gateway, Service, Storage } from '@/types'
import { computeColumns } from './columns'

// Resources：六类资源并发拉取、tab 计数开关、各表列渲染与空值兜底、刷新

const apiMock = vi.hoisted(() => ({
  getComputeInstances: vi.fn(),
  getDomains: vi.fn(),
  getCertificates: vi.fn(),
  getServices: vi.fn(),
  getGateways: vi.fn(),
  getStorages: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

// 探活单元格已有独立测试，这里只验接线参数
vi.mock('@/components/HeartbeatBar/ProbeHeartbeatCell', () => ({
  ProbeHeartbeatCell: ({ resourceType, resourceId }: { resourceType: string; resourceId: string }) => (
    <div data-testid="probe-cell">{`${resourceType}:${resourceId}`}</div>
  ),
}))

const settingsRef = { current: { showResourceCount: true, refreshInterval: 3600 } }
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsRef.current }),
}))

const compute = [
  { id: 'c1', name: 'web-vm', type: 'vm', agentId: 'ag-1', region: 'cn', zone: 'z1', status: 'running', cpuCores: 4, memoryMb: 8192, diskGb: 100, ipv4: '10.0.0.1' },
  { id: 'c2', name: 'edge', type: 'container', agentId: '', region: '', zone: '', status: 'stopped', cpuCores: 1, memoryMb: 512, diskGb: 8, ipv4: '' },
] as unknown as ComputeInstance[]

const domains = [
  { id: 'd1', domain: 'example.com', status: 'active', provider: 'cloudflare', autoRenew: true, expiresAt: '2026-06-01' },
  { id: 'd2', domain: 'old.io', status: 'expired', provider: '', autoRenew: false, expiresAt: '' },
] as unknown as Domain[]

const certificates = [
  { id: 'x1', domain: 'a.com', status: 'valid', issuer: "Let's Encrypt", autoRenew: true, expiresAt: '' },
  // x2 覆盖反向分支：非空过期时间走格式化、autoRenew false 走「否/default」
  { id: 'x2', domain: 'b.com', status: 'expiring', issuer: 'ZeroSSL', autoRenew: false, expiresAt: '2027-01-15' },
] as unknown as Certificate[]

const services = [
  { id: 's1', name: 'api', type: 'http', url: 'https://api.x.com', status: 'up', responseTimeMs: 120 },
  { id: 's2', name: 'db', type: 'tcp', url: '', status: 'degraded', responseTimeMs: 0 },
] as unknown as Service[]

const gateways = [
  { id: 'g1', name: 'gw', type: 'nginx', agentId: 'ag-1', ipv4: '1.2.3.4', upstream: '10.0.0.9', status: 'online' },
  { id: 'g2', name: 'gw2', type: '', agentId: '', ipv4: '', upstream: '', status: '' },
] as unknown as Gateway[]

const storages = [
  { id: 'st1', name: 'data', type: 'nfs', agentId: 'ag-1', path: '/data', totalGb: 100, usedGb: 40, status: 'online' },
  { id: 'st2', name: 'empty', type: '', agentId: '', path: '', totalGb: 0, usedGb: 0, status: '' },
  // st3/st4 覆盖 `${used || 0}` 与 `${total || 0}` 的单侧兜底（双 0 走 '-' 提前返回）
  { id: 'st3', name: 'half', type: 'ssd', agentId: 'ag-1', path: '/mnt', totalGb: 50, usedGb: 0, status: 'online' },
  { id: 'st4', name: 'odd', type: 'cifs', agentId: '', path: '', totalGb: 0, usedGb: 40, status: '' },
] as unknown as Storage[]

type ApiKey = keyof typeof apiMock

// dataOverride 可替换单个 API 的 resolved 值（renderPage 内设置默认值，故经参数传入）
const renderPage = (dataOverride?: Partial<Record<ApiKey, unknown>>) => {
  const defaults: Record<ApiKey, unknown> = {
    getComputeInstances: { data: compute },
    getDomains: { data: domains },
    getCertificates: { data: certificates },
    getServices: { data: services },
    getGateways: { data: gateways },
    getStorages: { data: storages },
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

const switchTab = async (label: string) => {
  fireEvent.click(screen.getByText(new RegExp(`^${label}`)))
  // 等懒挂载的表格出现
  await screen.findAllByText(/.*/, { selector: 'tr.ant-table-row' })
}

const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(cell)) as HTMLTableRowElement

describe('Resources', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    settingsRef.current = { showResourceCount: true, refreshInterval: 3600 }
  })

  it('默认计算实例 tab：六查询并发、行渲染与空值兜底', async () => {
    renderPage()
    expect(await screen.findByText('web-vm')).toBeInTheDocument()
    for (const fn of [
      apiMock.getComputeInstances, apiMock.getDomains, apiMock.getCertificates,
      apiMock.getServices, apiMock.getGateways, apiMock.getStorages,
    ]) {
      expect(fn).toHaveBeenCalledTimes(1)
    }
    expect(screen.getByText('计算实例 (2)')).toBeInTheDocument()
    // 类型/状态 Tag、配置三行、位置与 IP 兜底
    expect(screen.getByText('VM')).toBeInTheDocument()
    expect(screen.getByText('CONTAINER')).toBeInTheDocument()
    const r1 = rowOf('web-vm')
    expect(within(r1).getByText('running')).toBeInTheDocument()
    expect(within(r1).getByText(/CPU: 4 核/)).toBeInTheDocument()
    expect(within(r1).getByText(/内存: 8192 MB/)).toBeInTheDocument()
    expect(within(r1).getByText(/磁盘: 100 GB/)).toBeInTheDocument()
    expect(within(r1).getByText('cn/z1')).toBeInTheDocument()
    const r2 = rowOf('edge')
    expect(within(r2).getByText('stopped')).toBeInTheDocument()
    expect(within(r2).getAllByText('-').length).toBeGreaterThanOrEqual(1)
  })

  it('关闭资源计数：tab 标签剥掉括号计数', async () => {
    settingsRef.current = { showResourceCount: false, refreshInterval: 3600 }
    renderPage()
    expect(await screen.findByText('web-vm')).toBeInTheDocument()
    expect(screen.getByText('计算实例')).toBeInTheDocument()
    expect(screen.queryByText(/计算实例 \(/)).not.toBeInTheDocument()
  })

  it('域名 tab：链接、自动续费与探活接线', async () => {
    renderPage()
    await switchTab('域名')
    expect(await screen.findByText('example.com')).toBeInTheDocument()
    const link = screen.getByText('example.com').closest('a')
    expect(link?.getAttribute('href')).toBe('https://example.com')
    expect(within(rowOf('example.com')).getByText('是')).toBeInTheDocument()
    expect(within(rowOf('old.io')).getByText('否')).toBeInTheDocument()
    expect(screen.getAllByTestId('probe-cell').some((el) => el.textContent === 'domain:d1')).toBe(true)
  })

  it('证书 tab：签发者、有效状态、过期时间格式化与自动续费否侧', async () => {
    renderPage()
    await switchTab('证书')
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    expect(screen.getByText("Let's Encrypt")).toBeInTheDocument()
    expect(screen.getByText('valid')).toBeInTheDocument()
    // 非空过期时间 → toLocaleDateString（时区相关，用同一运行时表达式对期望值）
    expect(within(rowOf('b.com')).getByText(new Date('2027-01-15').toLocaleDateString())).toBeInTheDocument()
    // autoRenew false → 「否」+ default 色
    expect(within(rowOf('b.com')).getByText('否')).toBeInTheDocument()
  })

  it('服务 tab：URL 链接、响应时间空值兜底', async () => {
    renderPage()
    await switchTab('服务')
    expect(await screen.findByText('api')).toBeInTheDocument()
    expect(screen.getByText('https://api.x.com')).toBeInTheDocument()
    expect(within(rowOf('db')).getAllByText('-').length).toBeGreaterThanOrEqual(2)
    expect(within(rowOf('api')).getByText('120ms')).toBeInTheDocument()
  })

  it('网关 tab：地址/上游与空状态兜底', async () => {
    renderPage()
    await switchTab('网关')
    expect(await screen.findByText('gw')).toBeInTheDocument()
    expect(within(rowOf('gw')).getByText('1.2.3.4')).toBeInTheDocument()
    expect(within(rowOf('gw')).getByText('10.0.0.9')).toBeInTheDocument()
    expect(within(rowOf('gw2')).getAllByText('-').length).toBeGreaterThanOrEqual(3)
  })

  it('存储 tab：容量拼接、单侧缺省兜底与无容量兜底', async () => {
    renderPage()
    await switchTab('存储')
    expect(await screen.findByText('data')).toBeInTheDocument()
    expect(within(rowOf('data')).getByText('40 / 100 GB')).toBeInTheDocument()
    // usedGb 为 0 → 「0 / 50 GB」；totalGb 为 0 → 「40 / 0 GB」（|| 0 单侧兜底）
    expect(within(rowOf('half')).getByText('0 / 50 GB')).toBeInTheDocument()
    expect(within(rowOf('odd')).getByText('40 / 0 GB')).toBeInTheDocument()
    // 双 0 → '-' 提前返回
    expect(within(rowOf('empty')).getAllByText('-').length).toBeGreaterThanOrEqual(2)
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
    expect(await screen.findByText('web-vm')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
    await vi.waitFor(() => {
      expect(apiMock.getComputeInstances).toHaveBeenCalledTimes(2)
      expect(apiMock.getStorages).toHaveBeenCalledTimes(2)
    })
  })

  it('名称列排序器：localeCompare 三态', () => {
    const sorter = computeColumns.find((c) => c.key === 'name')!
      .sorter as (a: ComputeInstance, b: ComputeInstance) => number
    expect(sorter({ name: 'a' } as ComputeInstance, { name: 'b' } as ComputeInstance)).toBeLessThan(0)
    expect(sorter({ name: 'b' } as ComputeInstance, { name: 'a' } as ComputeInstance)).toBeGreaterThan(0)
    expect(sorter({ name: 'a' } as ComputeInstance, { name: 'a' } as ComputeInstance)).toBe(0)
  })

  it('activeKeyFallback：命中、回退首个 tab、空列表兜底', () => {
    const items = [{ key: 'compute' }, { key: 'domains' }]
    expect(activeKeyFallback('domains', items)).toBe('domains')
    expect(activeKeyFallback('gone', items)).toBe('compute')
    expect(activeKeyFallback('gone', [])).toBe('gone')
  })
})
