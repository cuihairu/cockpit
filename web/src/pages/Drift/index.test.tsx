import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Drift from './index'
import type { Agent } from '@/types'

// Drift：巡检开关换算、能力过滤与未选拦截、四态清单与 none 过滤、
// 差异 Modal 双视图与失败分支、登记/以当前为准回查、CMDB 四态与展开对照、RBAC

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getDriftConfig: vi.fn(),
  putDriftConfig: vi.fn(),
  checkDrift: vi.fn(),
  driftRecord: vi.fn(),
  driftDiff: vi.fn(),
  getInventoryConsistency: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: (p: string) => (p === 'drift:write' ? canWrite : true) }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const mkAgent = (id: string, hostname: string, caps: string[], status = 'online'): Agent =>
  ({ id, hostname, ip: '10.0.0.1', region: 'cn', zone: 'z1', status, lastSeen: '0',
     capabilities: caps.map((type) => ({ type })) }) as unknown as Agent

const agents = [
  mkAgent('ag-1', 'edge-01', ['drift', 'nginx-proxy']),
  mkAgent('ag-2', 'node-02', ['cron']),
  mkAgent('ag-plain', 'plain', ['files']),
]

// 四态 + none（应被过滤）+ 五类 kind 全覆盖
const checkItems = [
  { kind: 'nginx', name: 'blog.example.com', status: 'ok', baseline_sha: 'aaaa1111bbbb', current_sha: 'aaaa1111bbbb' },
  { kind: 'cron', name: 'cleanup', status: 'drifted', baseline_sha: 'cccc2222dddd', current_sha: 'eeee3333ffff' },
  { kind: 'stack', name: 'my-stack', status: 'missing', baseline_sha: '1234567890ab', current_sha: '' },
  { kind: 'traefik', name: 'api.example.com', status: 'no_baseline', baseline_sha: '', current_sha: 'abc' },
  { kind: 'nginx', name: 'bad.example.com', status: 'error', baseline_sha: 'x', current_sha: 'y' },
  { kind: 'cron', name: 'empty-seg', status: 'none', baseline_sha: '', current_sha: '' },
]

const report = {
  summary: { total: 4, ok: 1, mismatch: 1, unregistered: 1, undeclared: 1 },
  agents: [
    { id: 'ag-1', status: 'ok', declared: { hostname: 'edge-01', ip: '10.0.0.1' }, actual: { hostname: 'edge-01', ip: '10.0.0.1' } },
    { id: 'ag-2', status: 'mismatch',
      declared: { hostname: 'node-02', ip: '10.0.0.2' }, actual: { hostname: 'node-b', ip: '10.0.0.9' },
      mismatch: [{ field: 'hostname', declared: 'node-02', actual: 'node-b' }, { field: 'ip', declared: '10.0.0.2', actual: '10.0.0.9' }] },
    { id: 'ag-3', status: 'unregistered', declared: { hostname: 'future-03' } },
    { id: 'ag-4', status: 'undeclared', actual: { hostname: 'wild-04', ip: '10.0.0.4' } },
  ],
}

const mount = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Drift />
    </QueryClientProvider>,
  )
}

const renderPage = () => {
  apiMock.getAgents.mockResolvedValue(agents)
  apiMock.getDriftConfig.mockResolvedValue({ scan_interval_seconds: 300 })
  apiMock.checkDrift.mockResolvedValue({ agentId: 'ag-1', items: checkItems })
  apiMock.getInventoryConsistency.mockResolvedValue(report)
  return mount()
}

const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('.ant-table-tbody tr.ant-table-row'))
    .find((tr) => tr.textContent?.includes(cell)) as HTMLTableRowElement

const openSelectPick = async (ph: string, label: string) => {
  fireEvent.mouseDown(screen.getByText(ph).closest('.ant-select')!.querySelector('.ant-select-selector')!)
  fireEvent.click(await screen.findByText(label, { selector: '.ant-select-item-option-content' }))
}

const runCheckOn = async (hostname = 'edge-01') => {
  await openSelectPick('选择服务器', hostname)
  fireEvent.click(screen.getByRole('button', { name: /检\s*查/ }))
}

describe('Drift', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    canWrite = true
  })

  it('巡检开关：间隔换算保存、关闭归零', async () => {
    apiMock.putDriftConfig.mockResolvedValue({})
    renderPage()
    const minutes = await screen.findByDisplayValue('5')
    fireEvent.change(minutes, { target: { value: '15' } })
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.putDriftConfig).toHaveBeenCalledWith(900))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已开启自动巡检'))
    // 关闭开关 → 保存归零（invalidate 后配置仍 300s，编辑态 on=false 生效）。
    // 按钮进过 loading 后 aria-label="loading" 的图标常驻，名字带前缀 → 不锚定 ^
    fireEvent.click(document.querySelector('.ant-switch')!)
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.putDriftConfig).toHaveBeenCalledWith(0))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已关闭自动巡检'))
  })

  it('未选服务器拦截、检查失败透错、无能力 agent 不入候选', async () => {
    renderPage()
    apiMock.checkDrift.mockRejectedValue(new Error('boom')) // 在 renderPage 默认值之后覆盖
    fireEvent.click(screen.getByRole('button', { name: /检\s*查/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('请选择服务器'))
    // 候选只含 drift 能力主机（node-02/plain 不出现）——下拉开着时断言再选中
    fireEvent.mouseDown(screen.getByText('选择服务器').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    expect((await screen.findByText('edge-01', { selector: '.ant-select-item-option-content' }))).toBeInTheDocument()
    expect(document.querySelectorAll('.ant-select-item-option-content')).toHaveLength(1)
    fireEvent.click(screen.getByText('edge-01', { selector: '.ant-select-item-option-content' }))
    fireEvent.click(screen.getByRole('button', { name: /检\s*查/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('漂移检查失败'))
  })

  it('检查清单：四态 + error 计入告警、none 过滤、sha 截断与 kind 标签', async () => {
    renderPage()
    await runCheckOn()
    expect(await screen.findByText('发现 3 项漂移/异常，请核对是否为本人操作')).toBeInTheDocument()
    expect(screen.getByText('blog.example.com')).toBeInTheDocument()
    expect(within(rowOf('blog.example.com')).getByText('反代站点')).toBeInTheDocument()
    expect(within(rowOf('blog.example.com')).getByText('一致')).toBeInTheDocument()
    expect(within(rowOf('cleanup')).getByText('已漂移')).toBeInTheDocument()
    expect(within(rowOf('my-stack')).getByText('已丢失')).toBeInTheDocument()
    expect(within(rowOf('api.example.com')).getByText('未登记')).toBeInTheDocument()
    expect(within(rowOf('bad.example.com')).getByText('读取失败')).toBeInTheDocument()
    // none 条目不产生行
    expect(screen.queryByText('empty-seg')).not.toBeInTheDocument()
    // sha 截 8 位
    expect(within(rowOf('my-stack')).getByText('12345678')).toBeInTheDocument()
  })

  it('全部一致：success 提示且无操作按钮', async () => {
    renderPage()
    apiMock.checkDrift.mockResolvedValue({ items: [
      { kind: 'nginx', name: 'ok.example.com', status: 'ok', baseline_sha: 'a', current_sha: 'a' }] })
    await runCheckOn()
    expect(await screen.findByText('全部一致，未发现漂移')).toBeInTheDocument()
    expect(document.querySelectorAll('.ant-table-tbody button').length).toBe(0)
  })

  it('差异 Modal：对照视图增删行、切换统一视图与截断提示', async () => {
    // truncated 由前端 lineDiff 按行数自算（>1000 行），mock 数据需真超限
    const pad = Array.from({ length: 1001 }, (_, i) => `line-${i}`)
    apiMock.driftDiff.mockResolvedValue({
      expected: ['server {', '  listen 80;', ...pad, '}'].join('\n'),
      current: ['server {', '  listen 443;', ...pad, '}'].join('\n'),
      baseline_updated_at: 1759000000,
    })
    renderPage()
    await runCheckOn()
    await screen.findByText('发现 3 项漂移/异常，请核对是否为本人操作')
    fireEvent.click(within(rowOf('cleanup')).getByRole('button', { name: /差\s*异/ }))
    expect(await screen.findByText('定时任务 · cleanup', { selector: '.ant-modal-title' })).toBeInTheDocument()
    await waitFor(() => expect(apiMock.driftDiff).toHaveBeenCalledWith('ag-1', 'cron', 'cleanup'))
    expect(await screen.findByText(/内容过长，仅对比前 1000 行/)).toBeInTheDocument()
    // 对照视图：删行(-)与增行(+)并存
    await waitFor(() => expect(document.querySelectorAll('.ant-modal .ant-table-tbody').length).toBe(0))
    const modalText = document.querySelector('.ant-modal')!.textContent ?? ''
    expect(modalText).toContain('listen 80;')
    expect(modalText).toContain('listen 443;')
    // 切统一视图（Segmented 文本可点）
    fireEvent.click(screen.getByText('统一'))
    await waitFor(() => expect(document.querySelector('.ant-modal')!.textContent).toContain('listen 443;'))
  })

  it('差异失败：no content 追加补全引导', async () => {
    apiMock.driftDiff.mockRejectedValue(Object.assign(new Error('baseline has no content'), {
      isAxiosError: true, response: { status: 503, data: { error: 'baseline has no content' } } }))
    renderPage()
    await runCheckOn()
    await screen.findByText('发现 3 项漂移/异常，请核对是否为本人操作')
    fireEvent.click(within(rowOf('cleanup')).getByRole('button', { name: /差\s*异/ }))
    expect(await screen.findByText('获取差异失败')).toBeInTheDocument()
    expect(screen.getByText(/到对应管理页重新保存一次即可补全基线原文/)).toBeInTheDocument()
  })

  it('登记/以当前为准：Popconfirm 确认后重查', async () => {
    apiMock.driftRecord.mockResolvedValue({})
    renderPage()
    await runCheckOn()
    await screen.findByText('发现 3 项漂移/异常，请核对是否为本人操作')
    // 未登记 → 登记
    fireEvent.click(within(rowOf('api.example.com')).getByRole('button', { name: /登\s*记/ }))
    expect(await screen.findByText('将当前内容登记为基线？')).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.driftRecord).toHaveBeenCalledWith('ag-1', 'traefik', 'api.example.com'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith(
      '已登记：api.example.com，之后的漂移检测以当前内容为标准'))
    await waitFor(() => expect(apiMock.checkDrift).toHaveBeenCalledTimes(2))
    // 已漂移 → 以当前为准
    fireEvent.click(within(rowOf('cleanup')).getByRole('button', { name: /以当前为准/ }))
    expect(await screen.findByText('以当前磁盘内容为新基线？')).toBeInTheDocument()
  })

  it('RBAC：无写权限隐藏保存与登记入口', async () => {
    canWrite = false
    renderPage()
    expect(screen.queryByRole('button', { name: /保\s*存/ })).not.toBeInTheDocument()
    await runCheckOn()
    expect(await screen.findByText('发现 3 项漂移/异常，请核对是否为本人操作')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /登\s*记/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /以当前为准/ })).not.toBeInTheDocument()
    // 差异查看保留
    expect(within(rowOf('cleanup')).getByRole('button', { name: /差\s*异/ })).toBeInTheDocument()
  })

  it('CMDB 一致性：汇总四态、mismatch 展开字段对照与空值兜底', async () => {
    renderPage()
    fireEvent.click(screen.getByRole('tab', { name: 'CMDB 一致性' }))
    expect(await screen.findByText('共 4 台')).toBeInTheDocument()
    expect(screen.getByText('一致 1')).toBeInTheDocument()
    expect(screen.getByText('不一致 1')).toBeInTheDocument()
    expect(screen.getByText('未注册 1')).toBeInTheDocument()
    expect(screen.getByText('未声明 1')).toBeInTheDocument()
    // 四态行与声明/实报呈现；unregistered 无实报 → Facts 兜底 -
    const r3 = rowOf('ag-3')
    expect(within(r3).getByText('未注册')).toBeInTheDocument()
    expect(within(r3).getAllByText('-').length).toBeGreaterThanOrEqual(1)
    // mismatch 行展开字段级对照
    const r2 = rowOf('ag-2')
    expect(within(r2).getByText('不一致')).toBeInTheDocument()
    fireEvent.click(r2.querySelector('.ant-table-row-expand-icon')!)
    expect(await screen.findByText('主机名')).toBeInTheDocument()
    // node-b/10.0.0.9 同时出现在行内实报 Facts 与展开的字段对照表
    expect(screen.getAllByText('node-b')).toHaveLength(2)
    expect(screen.getAllByText('10.0.0.9')).toHaveLength(2)
  })

  it('CMDB 全一致/空声明/接口失败三分支', async () => {
    // 全一致
    apiMock.getInventoryConsistency.mockResolvedValueOnce({
      summary: { total: 2, ok: 2, mismatch: 0, unregistered: 0, undeclared: 0 }, agents: [] })
    const { unmount: unmount1 } = renderPage()
    fireEvent.click(screen.getByRole('tab', { name: 'CMDB 一致性' }))
    expect(await screen.findByText('声明与实报全部一致')).toBeInTheDocument()
    unmount1()
    // 空声明
    apiMock.getInventoryConsistency.mockResolvedValueOnce({
      summary: { total: 0, ok: 0, mismatch: 0, unregistered: 0, undeclared: 0 }, agents: [] })
    const qc2 = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { unmount: unmount2 } = render(<QueryClientProvider client={qc2}><Drift /></QueryClientProvider>)
    fireEvent.click(screen.getByRole('tab', { name: 'CMDB 一致性' }))
    expect(await screen.findByText(/inventory 未声明任何 agent/)).toBeInTheDocument()
    unmount2()
    // 接口失败
    apiMock.getInventoryConsistency.mockRejectedValue(new Error('boom'))
    const qc3 = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={qc3}><Drift /></QueryClientProvider>)
    fireEvent.click(await screen.findByRole('tab', { name: 'CMDB 一致性' }))
    expect(await screen.findByText('CMDB 一致性不可用')).toBeInTheDocument()
  })

  it('巡检间隔越界拦截：<1 分钟与 >1440 分钟（派生值直传校验）', async () => {
    // scan_interval_seconds 10 → 换算 0 分钟（< 1）
    apiMock.getAgents.mockResolvedValue(agents)
    apiMock.getDriftConfig.mockResolvedValue({ scan_interval_seconds: 10 })
    apiMock.putDriftConfig.mockResolvedValue({})
    apiMock.getInventoryConsistency.mockResolvedValue(report)
    mount()
    // 等巡检配置落地（scanOn 派生为真渲染出间隔输入）再保存
    await screen.findByRole('spinbutton')
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('间隔需在 1～1440 分钟之间'))
    expect(apiMock.putDriftConfig).not.toHaveBeenCalled()
  })

  it('巡检间隔越界拦截：超大间隔（>1440）；清空输入回落 1 分钟', async () => {
    apiMock.getAgents.mockResolvedValue(agents)
    apiMock.getDriftConfig.mockResolvedValue({ scan_interval_seconds: 100000000 })
    apiMock.putDriftConfig.mockResolvedValue({})
    apiMock.getInventoryConsistency.mockResolvedValue(report)
    mount()
    // 等巡检配置落地（scanOn 派生为真渲染出间隔输入）再保存
    await screen.findByRole('spinbutton')
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('间隔需在 1～1440 分钟之间'))
    // 清空 InputNumber：onChange(null) → minutes 回落 1，保存成功（?? 右分支）
    fireEvent.change(document.querySelector<HTMLInputElement>('.ant-input-number-input')!, { target: { value: '' } })
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.putDriftConfig).toHaveBeenCalledWith(60))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已开启自动巡检'))
  })

  it('保存巡检配置失败：透出回退文案；无 scan_interval_seconds 默认 30 分钟', async () => {
    apiMock.getAgents.mockResolvedValue(agents)
    apiMock.getDriftConfig.mockResolvedValue({})
    apiMock.putDriftConfig.mockRejectedValue(new Error('cfg down'))
    apiMock.getInventoryConsistency.mockResolvedValue(report)
    mount()
    // 等配置加载完（Switch loading 解除）再开开关：关闭态不渲染间隔输入
    await waitFor(() => expect(document.querySelector('.ant-switch-loading')).toBeNull())
    fireEvent.click(document.querySelector('.ant-switch')!)
    expect(await screen.findByDisplayValue('30')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
  })

  it('登记失败：透出回退文案', async () => {
    apiMock.driftRecord.mockRejectedValue(new Error('record down'))
    renderPage()
    await runCheckOn()
    await screen.findByText('发现 3 项漂移/异常，请核对是否为本人操作')
    fireEvent.click(within(rowOf('api.example.com')).getByRole('button', { name: /登\s*记/ }))
    await screen.findByText('将当前内容登记为基线？')
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('登记失败'))
  })

  it('差异 Modal 关闭回调收起弹窗；未知状态行不渲染状态 Tag', async () => {
    apiMock.getAgents.mockResolvedValue(agents)
    apiMock.getDriftConfig.mockResolvedValue({ scan_interval_seconds: 300 })
    apiMock.driftDiff.mockResolvedValue({
      expected: 'a\nb', current: 'a\nc', baseline_updated_at: 1759000000,
    })
    apiMock.checkDrift.mockResolvedValue({ agentId: 'ag-1', items: [
      { kind: 'nginx', name: 'ok.example.com', status: 'ok', baseline_sha: 'a', current_sha: 'a' },
      { kind: 'cron', name: 'cleanup', status: 'drifted', baseline_sha: 'c', current_sha: 'd' },
      { kind: 'nginx', name: 'odd.example.com', status: 'weird-status', baseline_sha: '', current_sha: '' },
    ] })
    apiMock.getInventoryConsistency.mockResolvedValue(report)
    mount()
    await runCheckOn()
    await screen.findByText('发现 1 项漂移/异常，请核对是否为本人操作')
    // 未知 status → STATUS_META 缺省 null，状态列不渲染 Tag（类型列仍有 kind Tag）
    const oddRow = rowOf('odd.example.com')
    expect(oddRow.querySelectorAll('td')[2].querySelector('.ant-tag')).toBeNull()
    // 打开差异后点 footer「关闭」（onClose → setDiffTarget(null)→ open=false 进入离场）
    fireEvent.click(within(rowOf('cleanup')).getByRole('button', { name: /差\s*异/ }))
    await screen.findByText('定时任务 · cleanup', { selector: '.ant-modal-title' })
    // 等 diff 加载完（footer 关闭按钮 loading 解除）
    await screen.findByText('对照')
    const modal = document.querySelector('.ant-modal') as HTMLElement
    fireEvent.click(within(modal).getByRole('button', { name: /^关\s*闭$/ }))
    await waitFor(() => expect(modal.className).toContain('ant-zoom-leave'))
  })

  it('无 capabilities 的 agent 不入候选；候选 label 缺 hostname 回退 id', async () => {
    apiMock.getAgents.mockResolvedValue([
      { id: 'ag-no', hostname: 'no-caps', status: 'online' } as unknown as Agent,
      { id: 'ag-empty', hostname: 'empty-caps', status: 'online', capabilities: [] } as unknown as Agent,
      // 具备 drift 能力但无 hostname → label 回退 agent id
      { id: 'ag-noname', status: 'online', capabilities: [{ type: 'drift' }] } as unknown as Agent,
    ])
    apiMock.getDriftConfig.mockResolvedValue({ scan_interval_seconds: 0 })
    apiMock.getInventoryConsistency.mockResolvedValue(report)
    mount()
    fireEvent.mouseDown(screen.getByText('选择服务器').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    expect(await screen.findByText('ag-noname', { selector: '.ant-select-item-option-content' })).toBeInTheDocument()
    expect(document.querySelectorAll('.ant-select-item-option-content')).toHaveLength(1)
  })
})
