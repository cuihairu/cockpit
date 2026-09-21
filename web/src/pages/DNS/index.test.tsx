import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import DNS from './index'
import type { Agent } from '@/types'

// DNS：未配置引导按 provider 分流、记录管理（zone/类型过滤/CF 代理列/新建
// 编辑 proxied 联动/删除）、DDNS（状态三态/巡检开关/新建编辑 pattern/检查
// 三分支/RBAC 只读降级）

const apiMock = vi.hoisted(() => ({
  getDNSStatus: vi.fn(),
  getDNSZones: vi.fn(),
  getDNSRecords: vi.fn(),
  createDNSRecord: vi.fn(),
  updateDNSRecord: vi.fn(),
  deleteDNSRecord: vi.fn(),
  getDDNSConfigs: vi.fn(),
  getAgents: vi.fn(),
  getDDNSScanConfig: vi.fn(),
  putDDNSScanConfig: vi.fn(),
  checkDDNSConfig: vi.fn(),
  createDDNSConfig: vi.fn(),
  updateDDNSConfig: vi.fn(),
  deleteDDNSConfig: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

// dns:write 与 ddns:write 分别控记录管理操作列与 DDNS 面板
const perms = vi.hoisted(() => ({ dns: true, ddns: true }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: (p: string) => (p === 'dns:write' ? perms.dns : p === 'ddns:write' ? perms.ddns : true) }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')

const zones = [
  { id: 'z1', name: 'example.com', in_cmdb: true },
  { id: 'z2', name: 'unmanaged.io', in_cmdb: false },
]

const records = {
  records: [
    { id: 'r1', type: 'A', name: 'www', content: '1.2.3.4', ttl: 300, proxied: true },
    { id: 'r2', type: 'TXT', name: '_acme', content: 'v=spf1', ttl: 1, proxied: false },
  ],
  total_pages: 1,
}

const mkAgent = (id: string, hostname: string, status = 'online'): Agent =>
  ({ id, hostname, ip: '10.0.0.1', location: { region: 'cn', zone: 'z1' }, status, lastSeen: '0',
     capabilities: [] }) as unknown as Agent

const ddnsAgents = [mkAgent('ag-1', 'edge-01'), mkAgent('ag-2', 'off-02', 'offline')]

const ddnsConfigs = [
  { id: 1, agentId: 'ag-1', zoneId: 'z1', zoneName: 'example.com', recordName: 'home.example.com',
    type: 'A', enabled: true, lastStatus: 'ok', lastIP: '203.0.113.9', lastError: '', checkedAt: 1759000000 },
  { id: 2, agentId: 'ag-2', zoneId: 'z1', zoneName: 'example.com', recordName: 'v6.example.com',
    type: 'AAAA', enabled: true, lastStatus: 'failed', lastIP: '', lastError: 'agent offline', checkedAt: 0 },
  { id: 3, agentId: 'ag-9', zoneId: 'z1', zoneName: 'example.com', recordName: 'old.example.com',
    type: 'A', enabled: false, lastStatus: 'never', lastIP: '', lastError: '', checkedAt: 0 },
]

const renderPage = (provider = 'cloudflare') => {
  apiMock.getDNSStatus.mockResolvedValue({ configured: true, provider })
  apiMock.getDNSZones.mockResolvedValue(zones)
  apiMock.getDNSRecords.mockResolvedValue(records)
  apiMock.getDDNSConfigs.mockResolvedValue(ddnsConfigs)
  apiMock.getAgents.mockResolvedValue(ddnsAgents)
  apiMock.getDDNSScanConfig.mockResolvedValue({ scan_interval_seconds: 300 })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <DNS />
    </QueryClientProvider>,
  )
}

// rc-select：mouseDown 打 .ant-select-selector，option 文本在 -content 子元素
const openSelect = (ph: string) => {
  const sel = screen.getByText(ph).closest('.ant-select')!.querySelector('.ant-select-selector')!
  fireEvent.mouseDown(sel)
}

const recRow = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('.ant-card tr.ant-table-row'))
    .find((tr) => tr.textContent?.includes(cell)) as HTMLTableRowElement

const chooseZone = async (label: string) => {
  // zone Select 是页面第一个 Select；选中后 placeholder 消失，按容器定位
  fireEvent.mouseDown(document.querySelector('.ant-select')!.querySelector('.ant-select-selector')!)
  // 未登记 zone 的 option label 带「（未登记 CMDB）」后缀，用子串正则
  fireEvent.click(await screen.findByText(new RegExp(label), { selector: '.ant-select-item-option-content' }))
  await screen.findByText(`${label} 的 DNS 记录`)
  // 记录表数据异步渲染，等首行
  await screen.findByText('www')
}

describe('DNS', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    perms.dns = true
    perms.ddns = true
  })

  it('未配置引导：按 provider 分流且不渲染 Tabs', async () => {
    apiMock.getDNSStatus.mockResolvedValue({ configured: false, provider: 'dnspod' })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={qc}><DNS /></QueryClientProvider>)
    expect(await screen.findByText('DNS 服务商未配置')).toBeInTheDocument()
    expect(screen.getByText('dns.dnspod.login_token')).toBeInTheDocument()
    expect(screen.getByText('DNSPOD_LOGIN_TOKEN')).toBeInTheDocument()
    expect(screen.getByText(/ID,Token/)).toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: '记录管理' })).not.toBeInTheDocument()
  })

  it('记录管理：zone 选择、未登记警告、CF 代理列与 TTL auto', async () => {
    renderPage()
    await chooseZone('example.com')
    await waitFor(() => expect(apiMock.getDNSRecords).toHaveBeenCalledWith('z1', undefined, 1))
    expect(await screen.findByText('www')).toBeInTheDocument()
    expect(within(recRow('www')).getByText('已代理')).toBeInTheDocument()
    expect(within(recRow('_acme')).getByText('auto')).toBeInTheDocument()
    expect(within(recRow('_acme')).queryByText('300')).not.toBeInTheDocument()
    // 未登记 CMDB 的 zone 警告
    await chooseZone('unmanaged.io')
    expect(await screen.findByText('该域名未登记在「资源 → 域名」，探测与证书管理不会覆盖它')).toBeInTheDocument()
    expect(screen.getByText('unmanaged.io（未登记 CMDB）', { selector: '.ant-select-selection-item' })).toBeInTheDocument()
  })

  it('记录管理：类型过滤与刷新接线', async () => {
    renderPage()
    await chooseZone('example.com')
    // placeholder「类型」与表头撞名，限定 placeholder 元素定位
    fireEvent.mouseDown(screen.getByText('类型', { selector: '.ant-select-selection-placeholder' }).closest('.ant-select')!.querySelector('.ant-select-selector')!)
    fireEvent.click(await screen.findByText('AAAA', { selector: '.ant-select-item-option-content' }))
    await waitFor(() => expect(apiMock.getDNSRecords).toHaveBeenCalledWith('z1', 'AAAA', 1))
    fireEvent.click(screen.getByRole('button', { name: /刷新$/ }))
    await waitFor(() => expect(apiMock.getDNSRecords.mock.calls.filter((c) => c[1] === 'AAAA').length).toBeGreaterThanOrEqual(2))
  })

  it('新建记录：必填校验、proxied 类型联动与提交参数', async () => {
    apiMock.createDNSRecord.mockResolvedValue({})
    renderPage()
    await chooseZone('example.com')
    fireEvent.click(screen.getByRole('button', { name: /新建记录/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal-title')?.textContent).toBe('新建 DNS 记录'))
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    expect(await screen.findByText('记录名不能为空')).toBeInTheDocument()
    expect(screen.getByText('内容不能为空')).toBeInTheDocument()
    expect(apiMock.createDNSRecord).not.toHaveBeenCalled()
    // 填写 A 记录并开橙云
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: ' home ' } })
    fireEvent.change(screen.getByLabelText('内容'), { target: { value: ' 5.6.7.8 ' } })
    fireEvent.click(screen.getByLabelText('Cloudflare 代理（橙云）').closest('.ant-form-item')!.querySelector('.ant-switch')!)
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.createDNSRecord).toHaveBeenCalledWith('z1', {
      type: 'A', name: 'home', content: '5.6.7.8', ttl: 0, proxied: true }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('记录已创建'))
  })

  it('新建记录：TXT 类型橙云开关禁用且提交 proxied=false', async () => {
    apiMock.createDNSRecord.mockResolvedValue({})
    renderPage()
    await chooseZone('example.com')
    fireEvent.click(screen.getByRole('button', { name: /新建记录/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal-title')?.textContent).toBe('新建 DNS 记录'))
    // 当前选中值「A」与表格 Tag 撞名，直接取 Modal 内第一个 Select（类型）
    fireEvent.mouseDown(document.querySelector('.ant-modal .ant-select')!.querySelector('.ant-select-selector')!)
    fireEvent.click(await screen.findByText('TXT', { selector: '.ant-select-item-option-content' }))
    const sw = screen.getByLabelText('Cloudflare 代理（橙云）').closest('.ant-form-item')!.querySelector('.ant-switch')!
    expect(sw.className).toContain('disabled')
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'txt1' } })
    fireEvent.change(screen.getByLabelText('内容'), { target: { value: 'v=chk' } })
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.createDNSRecord).toHaveBeenCalledWith('z1', {
      type: 'TXT', name: 'txt1', content: 'v=chk', ttl: 0, proxied: false }))
  })

  it('编辑记录：回填 TTL auto、类型禁用并走更新', async () => {
    apiMock.updateDNSRecord.mockResolvedValue({})
    renderPage()
    await chooseZone('example.com')
    fireEvent.click(within(recRow('_acme')).getByRole('button', { name: /编\s*辑/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal-title')?.textContent).toBe('编辑记录 _acme'))
    // TTL=1 归一化为 auto（空）；类型 Select 禁用
    expect((screen.getByLabelText('TTL（秒，留空 = auto）') as HTMLInputElement).value).toBe('')
    expect(screen.getByLabelText('类型').closest('.ant-select')!.className).toContain('disabled')
    fireEvent.change(screen.getByLabelText('内容'), { target: { value: 'v=spf2' } })
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.updateDNSRecord).toHaveBeenCalledWith('z1', 'r2', {
      type: 'TXT', name: '_acme', content: 'v=spf2', ttl: 0, proxied: false }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('记录已更新'))
  })

  it('删除记录：Popconfirm 确认', async () => {
    apiMock.deleteDNSRecord.mockResolvedValue({})
    renderPage()
    await chooseZone('example.com')
    fireEvent.click(within(recRow('www')).getByRole('button', { name: /删\s*除/ }))
    expect(await screen.findByText('删除立即生效，不可恢复')).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.deleteDNSRecord).toHaveBeenCalledWith('z1', 'r1'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('记录已删除'))
  })

  it('DDNS：状态三态、失败告警、IP/时间兜底与主机名解析', async () => {
    renderPage()
    fireEvent.click(screen.getByRole('tab', { name: 'DDNS' }))
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    expect(within(recRow('home.example.com')).getByText('正常')).toBeInTheDocument()
    expect(within(recRow('home.example.com')).getByText('203.0.113.9')).toBeInTheDocument()
    const r2 = recRow('v6.example.com')
    expect(within(r2).getByText('失败')).toBeInTheDocument()
    expect(within(r2).getAllByText('—').length).toBeGreaterThanOrEqual(1)
    expect(within(recRow('old.example.com')).getByText('未检查')).toBeInTheDocument()
    // 绑定主机列解析 hostname（ag-9 无对应 agent → 原样显示）
    expect(within(recRow('home.example.com')).getByText('edge-01')).toBeInTheDocument()
    expect(within(recRow('old.example.com')).getByText('ag-9')).toBeInTheDocument()
    expect(screen.getByText(/部分 DDNS 配置最近一次检查失败/)).toBeInTheDocument()
  })

  it('DDNS：巡检开关保存与分钟换算；检查三分支', async () => {
    apiMock.putDDNSScanConfig.mockResolvedValue({})
    apiMock.checkDDNSConfig
      .mockResolvedValueOnce({ status: 'ok', changed: true, ip: '203.0.113.99' })
      .mockResolvedValueOnce({ status: 'ok', changed: false, ip: '203.0.113.9' })
      .mockResolvedValueOnce({ status: 'failed', error: 'no ipv6 route' })
    renderPage()
    fireEvent.click(screen.getByRole('tab', { name: 'DDNS' }))
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    // 改间隔 10 分钟 → 600 秒
    const minutes = document.querySelector('.ant-input-number-input') as HTMLInputElement
    fireEvent.change(minutes, { target: { value: '10' } })
    fireEvent.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => expect(apiMock.putDDNSScanConfig).toHaveBeenCalledWith(600))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已开启自动巡检'))
    // 立即检查：更新 / 无变化 / 失败
    const checkBtn = (cell: string) =>
      recRow(cell).querySelector('button .anticon-check-circle')?.closest('button') as HTMLButtonElement
    fireEvent.click(checkBtn('home.example.com'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已更新 DNS 记录 → 203.0.113.99'))
    fireEvent.click(checkBtn('home.example.com'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('IP 无变化（203.0.113.9）'))
    fireEvent.click(checkBtn('home.example.com'))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('no ipv6 route'))
  })

  it('DDNS 新建：pattern 校验、agent 排序与提交 zoneName；编辑回填走更新', async () => {
    apiMock.createDDNSConfig.mockResolvedValue({})
    apiMock.updateDDNSConfig.mockResolvedValue({})
    renderPage()
    fireEvent.click(screen.getByRole('tab', { name: 'DDNS' }))
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建 DDNS/ }))
    await waitFor(() => expect(document.querySelector('.ant-modal-title')?.textContent).toBe('新建 DDNS 配置'))
    // 非完整域名 → pattern 拦截
    fireEvent.change(screen.getByLabelText('记录名'), { target: { value: 'home' } })
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    expect(await screen.findByText('填写完整记录名，如 home.example.com')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('记录名'), { target: { value: 'dyn.example.com' } })
    // agent 下拉：离线标注（online 排序在前由源码 sort 保证）
    await openSelect('选择探测公网 IP 的主机')
    const opts = Array.from(document.querySelectorAll('.ant-select-item-option-content')).map((o) => o.textContent)
    expect(opts.join()).toContain('off-02（离线）')
    fireEvent.click(await screen.findByText('edge-01', { selector: '.ant-select-item-option-content' }))
    await openSelect('选择 Cloudflare 托管域名')
    fireEvent.click(await screen.findByText('example.com', { selector: '.ant-select-item-option-content' }))
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.createDDNSConfig).toHaveBeenCalledWith({
      agentId: 'ag-1', zoneId: 'z1', zoneName: 'example.com',
      recordName: 'dyn.example.com', type: 'A', enabled: true }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已创建'))
    // 编辑回填 → 更新（切换 enabled）
    const editBtn = recRow('v6.example.com').querySelector('button .anticon-edit')?.closest('button') as HTMLButtonElement
    fireEvent.click(editBtn)
    await waitFor(() => expect(document.querySelector('.ant-modal-title')?.textContent).toBe('编辑 DDNS 配置'))
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.updateDDNSConfig).toHaveBeenCalledWith(2, {
      agentId: 'ag-2', zoneId: 'z1', zoneName: 'example.com',
      recordName: 'v6.example.com', type: 'AAAA', enabled: true }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已更新'))
  })

  it('RBAC：记录操作列隐藏、DDNS 降级只读且入口隐藏', async () => {
    perms.dns = false
    perms.ddns = false
    renderPage()
    await chooseZone('example.com')
    expect(screen.queryByRole('button', { name: /新建记录/ })).not.toBeInTheDocument()
    expect(recRow('www').querySelectorAll('button').length).toBe(0)
    fireEvent.click(screen.getByRole('tab', { name: 'DDNS' }))
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    expect(screen.getAllByText('只读').length).toBeGreaterThanOrEqual(1)
    expect(screen.queryByRole('button', { name: /新建 DDNS/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^保\s*存$/ })).not.toBeInTheDocument()
  })
})
