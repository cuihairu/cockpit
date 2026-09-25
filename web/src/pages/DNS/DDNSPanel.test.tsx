import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import DDNSPanel from './DDNSPanel'
import type { Agent, DDNSConfig, DDNSScanConfig } from '@/types'

// DDNSPanel：独立面板补测（DNS 页测试覆盖主流程之外的缺口）——
// 巡检越界警告/保存失败、立即检查三分支与网络错误、删除成功与失败、
// 新建校验/创建成功失败、编辑回填与更新、取消重开重置

const apiMock = vi.hoisted(() => ({
  getDDNSConfigs: vi.fn(),
  getDNSZones: vi.fn(),
  getAgents: vi.fn(),
  getDDNSScanConfig: vi.fn(),
  putDDNSScanConfig: vi.fn(),
  checkDDNSConfig: vi.fn(),
  createDDNSConfig: vi.fn(),
  updateDDNSConfig: vi.fn(),
  deleteDDNSConfig: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => true }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const zones = [{ id: 'z1', name: 'example.com' }]

const mkAgent = (id: string, hostname: string, status = 'online'): Agent =>
  ({ id, hostname, ip: '10.0.0.1', region: 'cn', zone: 'z1', status, lastSeen: '0', capabilities: [] }) as unknown as Agent

const agents = [mkAgent('ag-1', 'edge-01'), mkAgent('ag-2', 'off-02', 'offline'), mkAgent('ag-3', '')]

const mkCfg = (over: Partial<DDNSConfig>): DDNSConfig =>
  ({
    id: 1,
    agentId: 'ag-1',
    zoneId: 'z1',
    zoneName: 'example.com',
    recordName: 'home.example.com',
    type: 'A',
    enabled: true,
    lastIP: '1.2.3.4',
    lastStatus: 'ok',
    lastError: '',
    checkedAt: 1767225600,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...over,
  }) as DDNSConfig

const configs = [
  mkCfg({ id: 1 }),
  mkCfg({ id: 2, recordName: 'nas.example.com', agentId: 'ag-2', lastIP: '', lastStatus: 'failed', lastError: 'CF 7003' }),
  mkCfg({ id: 3, recordName: 'vps.example.com', agentId: 'ag-2', lastIP: '', lastStatus: 'never', lastError: '', checkedAt: 0 }),
  mkCfg({ id: 4, recordName: 'weird.example.com', lastStatus: 'weird' as never, lastIP: '' }),
  mkCfg({ id: 5, recordName: 'ghost.example.com', agentId: 'ag-gone', lastStatus: 'never', lastIP: '', checkedAt: 0 }),
  mkCfg({ id: 7, recordName: 'zmiss.example.com', zoneId: 'z-missing', zoneName: '', lastStatus: 'never', lastIP: '', checkedAt: 0 }),
]

const scanCfg: DDNSScanConfig = { scan_interval_seconds: 600, min: 1, max: 3600, default: 5 }

const renderPanel = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <DDNSPanel />
    </QueryClientProvider>,
  )
}

const rowOf = (text: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find(
    (tr) => tr.textContent?.includes(text),
  ) as HTMLTableRowElement

// 立即检查（CheckCircle）/ 编辑（Edit）/ 删除（Delete）均为 icon-only 小按钮
const rowIconBtn = (row: HTMLTableRowElement, icon: string) =>
  row.querySelector(`button .anticon-${icon}`)?.closest('button') as HTMLButtonElement

const modalSave = () => fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
const modalCancel = () =>
  fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn:not(.ant-btn-primary)')!)

const pickOption = async (text: string) => {
  const opt = await waitFor(() => {
    const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
      (o) => o.textContent === text,
    )
    if (!el) throw new Error('option not found: ' + text)
    return el as HTMLElement
  })
  fireEvent.click(opt)
}

describe('DDNSPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.getDDNSConfigs.mockResolvedValue(configs)
    apiMock.getDNSZones.mockResolvedValue(zones)
    apiMock.getAgents.mockResolvedValue(agents)
    apiMock.getDDNSScanConfig.mockResolvedValue(scanCfg)
  })

  it('列表：状态三态、离线主机名解析、未检查显示 —、失败 Alert', async () => {
    renderPanel()
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    expect(screen.getByText('正常')).toBeInTheDocument()
    expect(screen.getByText('失败')).toBeInTheDocument()
    // 兜底 fixture（未知状态/无 agent/zone 缺失）也归入未检查
    expect(screen.getAllByText('未检查').length).toBeGreaterThanOrEqual(1)
    // id1/id4 同绑 ag-1（weird fixture 继承默认 agentId）：断言收窄到行内
    expect(rowOf('home.example.com').textContent).toContain('edge-01')
    expect(screen.getAllByText('off-02').length).toBe(2)
    expect(screen.getByText('1.2.3.4')).toBeInTheDocument()
    // checkedAt=0 → '—'
    expect(rowOf('vps.example.com').textContent).toContain('—')
    // 存在 failed 配置 → 顶部警告 Alert
    expect(screen.getByText(/部分 DDNS 配置最近一次检查失败/)).toBeInTheDocument()
  })

  it('巡检间隔越界警告；调小后保存成功', async () => {
    apiMock.putDDNSScanConfig.mockResolvedValue(undefined)
    // 服务端存量超界（>1440 分钟）：InputNumber 的 min/max 只 clamp UI 键入，
    // 服务端派生值可直接命中防御分支
    apiMock.getDDNSScanConfig.mockResolvedValue({ scan_interval_seconds: 200000, min: 1, max: 3600, default: 5 })
    renderPanel()
    const spin = (await screen.findByDisplayValue('3333')) as HTMLInputElement
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    expect(msgWarning).toHaveBeenCalledWith('间隔需在 1～1440 分钟之间')
    expect(apiMock.putDDNSScanConfig).not.toHaveBeenCalled()

    // jsdom 下 antd InputNumber 需 focus→change→blur 序列才提交 onChange
    fireEvent.focus(spin)
    fireEvent.change(spin, { target: { value: '15' } })
    fireEvent.blur(spin)
    await waitFor(() => expect(spin.value).toBe('15'))
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.putDDNSScanConfig).toHaveBeenCalledWith(900))
    expect(msgSuccess).toHaveBeenCalledWith('已开启自动巡检')
  })

  it('Switch 关闭自动巡检：保存 payload 0 并提示已关闭', async () => {
    apiMock.putDDNSScanConfig.mockResolvedValue(undefined)
    renderPanel()
    expect(await screen.findByDisplayValue('10')).toBeInTheDocument()
    // scanOn 当前为 true（存量 600s），点击 Switch 关闭
    fireEvent.click(document.querySelector('.ant-switch')!)
    expect(screen.queryByRole('spinbutton')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.putDDNSScanConfig).toHaveBeenCalledWith(0))
    expect(msgSuccess).toHaveBeenCalledWith('已关闭自动巡检')
  })

  it('巡检保存失败：错误透出', async () => {
    apiMock.putDDNSScanConfig.mockRejectedValue(new Error('scan save fail'))
    renderPanel()
    fireEvent.click(await screen.findByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
  })

  it('立即检查：IP 变化 / 无变化 / 检查失败 / 网络错误 四分支', async () => {
    renderPanel()
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    const check1 = rowIconBtn(rowOf('home.example.com'), 'check-circle')
    const check2 = rowIconBtn(rowOf('nas.example.com'), 'check-circle')

    apiMock.checkDDNSConfig.mockResolvedValueOnce({ ip: '5.6.7.8', changed: true, status: 'ok', error: '' })
    fireEvent.click(check1)
    await waitFor(() =>
      expect(msgSuccess).toHaveBeenCalledWith('已更新 DNS 记录 → 5.6.7.8'))

    apiMock.checkDDNSConfig.mockResolvedValueOnce({ ip: '5.6.7.8', changed: false, status: 'ok', error: '' })
    fireEvent.click(check1)
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('IP 无变化（5.6.7.8）'))

    apiMock.checkDDNSConfig.mockResolvedValueOnce({ ip: '', changed: false, status: 'failed', error: 'CF 7003' })
    fireEvent.click(check2)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('CF 7003'))

    apiMock.checkDDNSConfig.mockRejectedValueOnce(new Error('network down'))
    fireEvent.click(check2)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('检查失败'))
  })

  it('删除：确认成功提示已删除；失败错误透出', async () => {
    apiMock.deleteDDNSConfig.mockResolvedValue(undefined)
    renderPanel()
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    const confirmDel = async (row: HTMLTableRowElement, id: number) => {
      fireEvent.click(rowIconBtn(row, 'delete'))
      // 旧 Popconfirm DOM 可能因关闭动效残留：取最后出现的确认气泡
      await screen.findAllByText(/删除该 DDNS 配置/)
      const btns = document.querySelectorAll('.ant-popover .ant-btn-primary')
      fireEvent.click(btns[btns.length - 1] as HTMLElement)
      await waitFor(() => expect(apiMock.deleteDDNSConfig).toHaveBeenCalledWith(id))
    }
    apiMock.deleteDDNSConfig.mockResolvedValue(undefined)
    renderPanel()
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    await confirmDel(rowOf('home.example.com'), 1)
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已删除'))

    apiMock.deleteDDNSConfig.mockRejectedValue(new Error('del fail'))
    await confirmDel(rowOf('nas.example.com'), 2)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('删除失败'))
  })

  it('新建：必填校验、合法提交创建成功', async () => {
    apiMock.createDDNSConfig.mockResolvedValue(undefined)
    renderPanel()
    fireEvent.click(await screen.findByRole('button', { name: /新建 DDNS/ }))
    expect(await screen.findByText('新建 DDNS 配置')).toBeInTheDocument()
    // 空表单直接保存 → 两个必填错误
    modalSave()
    await waitFor(() =>
      expect(document.querySelectorAll('.ant-form-item-explain-error').length).toBeGreaterThanOrEqual(2))
    expect(apiMock.createDDNSConfig).not.toHaveBeenCalled()

    fireEvent.mouseDown(document.querySelector('.ant-modal .ant-select-selector')!)
    await pickOption('edge-01')
    // 第二个 Select：zone
    fireEvent.mouseDown(document.querySelectorAll('.ant-modal .ant-select-selector')[1]!)
    await pickOption('example.com')
    fireEvent.change(screen.getByLabelText('记录名'), { target: { value: 'shop.example.com' } })
    modalSave()
    await waitFor(() =>
      expect(apiMock.createDDNSConfig).toHaveBeenCalledWith({
        agentId: 'ag-1',
        zoneId: 'z1',
        zoneName: 'example.com',
        recordName: 'shop.example.com',
        type: 'A',
        enabled: true,
      }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已创建'))
  })

  it('创建失败：错误透出且不误报成功', async () => {
    apiMock.createDDNSConfig.mockRejectedValue(new Error('create fail'))
    renderPanel()
    fireEvent.click(await screen.findByRole('button', { name: /新建 DDNS/ }))
    await screen.findByText('新建 DDNS 配置')
    fireEvent.mouseDown(document.querySelector('.ant-modal .ant-select-selector')!)
    await pickOption('edge-01')
    fireEvent.mouseDown(document.querySelectorAll('.ant-modal .ant-select-selector')[1]!)
    await pickOption('example.com')
    fireEvent.change(screen.getByLabelText('记录名'), { target: { value: 'shop.example.com' } })
    modalSave()
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
    expect(msgSuccess).not.toHaveBeenCalledWith('已创建')
  })

  it('编辑：回填并更新成功', async () => {
    apiMock.updateDDNSConfig.mockResolvedValue(undefined)
    renderPanel()
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    fireEvent.click(rowIconBtn(rowOf('home.example.com'), 'edit'))
    expect(await screen.findByText('编辑 DDNS 配置')).toBeInTheDocument()
    expect(screen.getByLabelText('记录名')).toHaveValue('home.example.com')
    modalSave()
    await waitFor(() =>
      expect(apiMock.updateDDNSConfig).toHaveBeenCalledWith(1, {
        agentId: 'ag-1',
        zoneId: 'z1',
        zoneName: 'example.com',
        recordName: 'home.example.com',
        type: 'A',
        enabled: true,
      }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已更新'))
  })

  it('取消后重开新建：表单被显式重置', async () => {
    renderPanel()
    expect(await screen.findByText('home.example.com')).toBeInTheDocument()
    fireEvent.click(rowIconBtn(rowOf('home.example.com'), 'edit'))
    await screen.findByText('编辑 DDNS 配置')
    fireEvent.change(screen.getByLabelText('记录名'), { target: { value: 'dirty.example.com' } })
    modalCancel()
    // 重开新建：openCreate 的 setFieldsValue 显式重置（不依赖 Modal 动画）
    fireEvent.click(await screen.findByRole('button', { name: /新建 DDNS/ }))
    expect(await screen.findByText('新建 DDNS 配置')).toBeInTheDocument()
    expect(screen.getByLabelText('记录名')).toHaveValue('')
  })

  it('列表兜底：未知状态归未检查、无 agent 显示原 id、空 error 检查兜底', async () => {
    renderPanel()
    expect(await screen.findByText('weird.example.com')).toBeInTheDocument()
    // 未知 lastStatus → never 元数据（未检查）
    expect(rowOf('weird.example.com').textContent).toContain('未检查')
    // agentId 无对应 agent → 原样显示 id
    expect(rowOf('ghost.example.com').textContent).toContain('ag-gone')
    // 立即检查返回 failed 且 error 为空 → 兜底「检查失败」
    apiMock.checkDDNSConfig.mockResolvedValueOnce({ ip: '', changed: false, status: 'failed', error: '' })
    fireEvent.click(rowIconBtn(rowOf('weird.example.com'), 'check-circle'))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('检查失败'))
  })

  it('弹窗 agent 下拉：hostname 缺省用 id、离线带后缀', async () => {
    renderPanel()
    fireEvent.click(await screen.findByRole('button', { name: /新建 DDNS/ }))
    expect(await screen.findByText('新建 DDNS 配置')).toBeInTheDocument()
    fireEvent.mouseDown(document.querySelector('.ant-modal .ant-select-selector')!)
    expect(await screen.findByText('ag-3')).toBeInTheDocument()
    expect(screen.getByText('off-02（离线）')).toBeInTheDocument()
  })

  it('编辑：zone 不在列表时 zoneName 回填空串', async () => {
    apiMock.updateDDNSConfig.mockResolvedValue(undefined)
    renderPanel()
    expect(await screen.findByText('zmiss.example.com')).toBeInTheDocument()
    fireEvent.click(rowIconBtn(rowOf('zmiss.example.com'), 'edit'))
    await screen.findByText('编辑 DDNS 配置')
    modalSave()
    await waitFor(() =>
      expect(apiMock.updateDDNSConfig).toHaveBeenCalledWith(7, expect.objectContaining({ zoneName: '' })))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已更新'))
  })

  it('巡检分钟清空 blur 后按 1 分钟保存（v ?? 1 兜底）', async () => {
    apiMock.putDDNSScanConfig.mockResolvedValue(undefined)
    renderPanel()
    const spin = (await screen.findByDisplayValue('10')) as HTMLInputElement
    fireEvent.focus(spin)
    fireEvent.change(spin, { target: { value: '' } })
    fireEvent.blur(spin)
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.putDDNSScanConfig).toHaveBeenCalledWith(60))
  })
})

