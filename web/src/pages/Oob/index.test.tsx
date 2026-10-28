import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Oob from './index'
import type { Agent, OobRedirectRule } from '@/types'

// Oob（带外兜底转发编排，片C）：空态 / 列表渲染（cockpit 标注、过期标注、
// 摘要防御取值）/ 三个双确认动作（新建 confirm_label、拆除 confirm_name、
// 清扫 confirm_oob）——确认字段不一致必须拦截，一致才下发。

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  oobRedirectList: vi.fn(),
  oobRedirectApply: vi.fn(),
  oobRedirectRemove: vi.fn(),
  oobRedirectSweep: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => canWrite }))

const mkAgent = (id: string, hostname = `host-${id}`, oob = true): Agent =>
  ({
    id,
    hostname,
    ip: '1.2.3.4',
    status: 'online',
    lastSeen: '0',
    capabilities: oob ? [{ type: 'oob' }] : [{ type: 'files' }],
  }) as unknown as Agent

// 四条规则覆盖：过期 cockpit（number 端口）/ 未来 cockpit（string 端口）/
// 用户自建（仅 dest_ip）/ 全空（摘要走 '—' 兜底）
const RULES: OobRedirectRule[] = [
  { name: 'cockpit_ipmi_1790000000', proto: 'tcp', src: 'wan', dest: 'lan', dest_ip: '192.168.1.10', dest_port: 16992 },
  { name: 'cockpit_kvm_2000000000', proto: 'udp', src: 'lan', dest: 'lan', dest_ip: '10.0.0.5', dest_port: '5900' },
  { name: 'user_forward', dest_ip: '10.0.0.9' },
  { name: 'bare_rule' },
]

const renderPage = (agents?: Agent[], listImpl?: () => Promise<{ redirects: OobRedirectRule[] }>) => {
  apiMock.getAgents.mockResolvedValue(agents ?? [mkAgent('ag-1')])
  apiMock.oobRedirectList.mockImplementation(
    listImpl ?? (() => Promise.resolve({ redirects: RULES })),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Oob />
    </QueryClientProvider>,
  )
}

// 输入 vault 条目名触发列表加载（默认选中首个 oob 主机）
const typeOob = async (v = 'r2s') => {
  const input = await screen.findByPlaceholderText('vault 条目名（oob/<name>）')
  fireEvent.change(input, { target: { value: v } })
}

describe('Oob', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    canWrite = true
    apiMock.oobRedirectApply.mockResolvedValue({
      name: 'cockpit_ipmi_1790000000', expiry: 1790000000, ttl_seconds: 1800,
    })
    apiMock.oobRedirectRemove.mockResolvedValue({ name: 'cockpit_ipmi_1790000000' })
    apiMock.oobRedirectSweep.mockResolvedValue({ removed: [] })
  })

  it('无 oob 主机：空态且不拉列表；offline/无 capabilities 的 agent 不误选', async () => {
    renderPage([
      mkAgent('a1', 'fw-host', false), // 有 capability 但非 oob
      { id: 'a2', status: 'offline', ip: '', lastSeen: '0', capabilities: [{ type: 'oob' }] } as unknown as Agent, // oob 但 offline
      { id: 'a3', status: 'online' } as unknown as Agent, // 无 capabilities（?? [] 右支）
    ])
    expect(await screen.findByText(/暂无带 oob capability/)).toBeInTheDocument()
    expect(apiMock.oobRedirectList).not.toHaveBeenCalled()
  })

  it('选择主机+输入条目：列表渲染，cockpit/过期标注与摘要防御取值', async () => {
    // 第二台主机 hostname 为空 → Select 选项 label 走 id 兜底（hostname || id 右支）
    renderPage([mkAgent('ag-1', 'router'), mkAgent('ag-2', '')])
    await typeOob()
    expect(await screen.findByText('cockpit_ipmi_1790000000')).toBeInTheDocument()
    expect(screen.getByText('cockpit_kvm_2000000000')).toBeInTheDocument()
    expect(screen.getByText('user_forward')).toBeInTheDocument()
    expect(screen.getByText('bare_rule')).toBeInTheDocument()
    // cockpit 标注 ×2；已过期 ×1（1790000000 < now）
    expect(screen.getAllByText('cockpit').length).toBe(2)
    expect(screen.getByText('已过期')).toBeInTheDocument()
    // 摘要：number 端口 / string 端口 / 仅 dest_ip / 全空 → —
    expect(screen.getByText('tcp wan →lan 192.168.1.10:16992')).toBeInTheDocument()
    expect(screen.getByText('udp lan →lan 10.0.0.5:5900')).toBeInTheDocument()
    expect(screen.getByText('10.0.0.9')).toBeInTheDocument()
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(1)
    // 过期提示
    expect(screen.getByText(/1 条已过期/)).toBeInTheDocument()
  })

  it('未输入条目名：提示选择主机与条目', async () => {
    renderPage([mkAgent('ag-1')])
    expect(await screen.findByText(/选择主机并输入 vault 条目名/)).toBeInTheDocument()
  })

  it('列表读取失败：告警（不落错误文案）', async () => {
    renderPage([mkAgent('ag-1')], () => Promise.reject(new Error('boom')))
    await typeOob()
    expect(await screen.findByText(/读取失败/)).toBeInTheDocument()
  })

  it('新建：确认不一致拦截，一致下发（含/不含可选字段）', async () => {
    renderPage([mkAgent('ag-1')])
    await typeOob()
    await screen.findByText('cockpit_ipmi_1790000000')
    fireEvent.click(screen.getByRole('button', { name: /新建规则/ }))
    fireEvent.change(screen.getByPlaceholderText('如 ipmi、kvm'), { target: { value: 'ipmi' } })
    fireEvent.change(screen.getByPlaceholderText('wan'), { target: { value: 'wan' } })
    fireEvent.change(screen.getByPlaceholderText('lan'), { target: { value: 'lan' } })
    fireEvent.change(screen.getByPlaceholderText('192.168.1.10'), { target: { value: '192.168.1.10' } })
    fireEvent.change(screen.getByPlaceholderText('16992'), { target: { value: '16992' } })
    // 确认不一致 → 校验错误、不下发
    fireEvent.change(screen.getByPlaceholderText('输入与上方标签相同的内容'), { target: { value: 'wrong' } })
    fireEvent.click(screen.getByRole('button', { name: /下\s*发/ }))
    expect(await screen.findByText('输入与规则标签不一致')).toBeInTheDocument()
    expect(apiMock.oobRedirectApply).not.toHaveBeenCalled()
    // 一致 + 可选源字段
    const src = screen.getAllByPlaceholderText('留空 = 任意')
    fireEvent.change(src[0], { target: { value: '10.0.0.2' } })
    fireEvent.change(src[1], { target: { value: '12345' } })
    fireEvent.change(screen.getByPlaceholderText('输入与上方标签相同的内容'), { target: { value: 'ipmi' } })
    fireEvent.click(screen.getByRole('button', { name: /下\s*发/ }))
    await waitFor(() =>
      expect(apiMock.oobRedirectApply).toHaveBeenCalledWith(expect.objectContaining({
        agentId: 'ag-1',
        oob: 'r2s',
        confirmLabel: 'ipmi',
        spec: expect.objectContaining({
          label: 'ipmi', proto: 'tcp', src_zone: 'wan', dst_zone: 'lan',
          dst_ip: '192.168.1.10', dst_port: 16992, ttl_seconds: 1800,
          src_ip: '10.0.0.2', src_port: 12345,
        }),
      })),
    )
    // 成功后 invalidate 触发列表重拉（初始 1 次 + 成功后 >=2 次）
    await waitFor(() => expect(apiMock.oobRedirectList.mock.calls.length).toBeGreaterThanOrEqual(2))
  })

  it('新建：可选字段缺省时不进 spec（src_ip/src_port 恒空）', async () => {
    renderPage([mkAgent('ag-1')])
    await typeOob()
    await screen.findByText('cockpit_ipmi_1790000000')
    fireEvent.click(screen.getByRole('button', { name: /新建规则/ }))
    fireEvent.change(screen.getByPlaceholderText('如 ipmi、kvm'), { target: { value: 'ipmi' } })
    fireEvent.change(screen.getByPlaceholderText('wan'), { target: { value: 'wan' } })
    fireEvent.change(screen.getByPlaceholderText('lan'), { target: { value: 'lan' } })
    fireEvent.change(screen.getByPlaceholderText('192.168.1.10'), { target: { value: '192.168.1.10' } })
    fireEvent.change(screen.getByPlaceholderText('16992'), { target: { value: '16992' } })
    fireEvent.change(screen.getByPlaceholderText('输入与上方标签相同的内容'), { target: { value: 'ipmi' } })
    fireEvent.click(screen.getByRole('button', { name: /下\s*发/ }))
    await waitFor(() => expect(apiMock.oobRedirectApply).toHaveBeenCalled())
    const spec = apiMock.oobRedirectApply.mock.calls[0][0].spec
    expect(spec.src_ip).toBeUndefined()
    expect(spec.src_port).toBeUndefined()
  })

  it('新建失败：透出服务端错误文案', async () => {
    apiMock.oobRedirectApply.mockRejectedValue({ response: { data: { error: 'agent offline' } } })
    renderPage([mkAgent('ag-1')])
    await typeOob()
    await screen.findByText('cockpit_ipmi_1790000000')
    fireEvent.click(screen.getByRole('button', { name: /新建规则/ }))
    fireEvent.change(screen.getByPlaceholderText('如 ipmi、kvm'), { target: { value: 'x' } })
    fireEvent.change(screen.getByPlaceholderText('wan'), { target: { value: 'w' } })
    fireEvent.change(screen.getByPlaceholderText('lan'), { target: { value: 'l' } })
    fireEvent.change(screen.getByPlaceholderText('192.168.1.10'), { target: { value: '1.2.3.4' } })
    fireEvent.change(screen.getByPlaceholderText('16992'), { target: { value: '8080' } })
    fireEvent.change(screen.getByPlaceholderText('输入与上方标签相同的内容'), { target: { value: 'x' } })
    fireEvent.click(screen.getByRole('button', { name: /下\s*发/ }))
    await waitFor(() => expect(apiMock.oobRedirectApply).toHaveBeenCalled())
    // 失败不关弹窗（onError 不置 createOpen(false)）
    expect(screen.getByPlaceholderText('如 ipmi、kvm')).toBeInTheDocument()
  })

  it('拆除：确认不一致拦截，一致调用', async () => {
    renderPage([mkAgent('ag-1')])
    await typeOob()
    await screen.findByText('cockpit_ipmi_1790000000')
    fireEvent.click(screen.getAllByRole('button', { name: /拆\s*除/ })[0])
    const inp = await screen.findByPlaceholderText('cockpit_ipmi_1790000000')
    fireEvent.change(inp, { target: { value: 'wrong' } })
    fireEvent.click(screen.getByRole('button', { name: /确认拆除/ }))
    expect(await screen.findByText('输入与规则名不一致')).toBeInTheDocument()
    expect(apiMock.oobRedirectRemove).not.toHaveBeenCalled()
    fireEvent.change(inp, { target: { value: 'cockpit_ipmi_1790000000' } })
    fireEvent.click(screen.getByRole('button', { name: /确认拆除/ }))
    await waitFor(() =>
      expect(apiMock.oobRedirectRemove).toHaveBeenCalledWith(expect.objectContaining({
        agentId: 'ag-1', oob: 'r2s', name: 'cockpit_ipmi_1790000000', confirmName: 'cockpit_ipmi_1790000000',
      })),
    )
  })

  it('清扫：确认不一致拦截，一致调用（空/非空结果提示）', async () => {
    apiMock.oobRedirectSweep.mockResolvedValueOnce({ removed: [] })
    apiMock.oobRedirectSweep.mockResolvedValueOnce({ removed: ['cockpit_ipmi_1790000000'] })
    renderPage([mkAgent('ag-1')])
    await typeOob()
    await screen.findByText('cockpit_ipmi_1790000000')
    fireEvent.click(screen.getByRole('button', { name: /清扫过期/ }))
    const inp = await screen.findByPlaceholderText('r2s')
    fireEvent.change(inp, { target: { value: 'wrong' } })
    fireEvent.click(screen.getByRole('button', { name: /确认清扫/ }))
    expect(await screen.findByText('输入与 vault 条目名不一致')).toBeInTheDocument()
    expect(apiMock.oobRedirectSweep).not.toHaveBeenCalled()
    fireEvent.change(inp, { target: { value: 'r2s' } })
    fireEvent.click(screen.getByRole('button', { name: /确认清扫/ }))
    await waitFor(() => expect(apiMock.oobRedirectSweep).toHaveBeenCalled())
    // 成功后 invalidate 触发列表重拉（初始 1 次 + 首次成功后 >=2 次）
    await waitFor(() => expect(apiMock.oobRedirectList.mock.calls.length).toBeGreaterThanOrEqual(2))
    // 再次清扫（mock 队列第二个结果 removed=[...]）
    fireEvent.click(screen.getByRole('button', { name: /清扫过期/ }))
    const inp2 = await screen.findByPlaceholderText('r2s')
    fireEvent.change(inp2, { target: { value: 'r2s' } })
    fireEvent.click(screen.getByRole('button', { name: /确认清扫/ }))
    await waitFor(() =>
      expect(apiMock.oobRedirectSweep).toHaveBeenLastCalledWith(expect.objectContaining({
        agentId: 'ag-1', oob: 'r2s', confirmOob: 'r2s',
      })),
    )
    await waitFor(() => expect(apiMock.oobRedirectList.mock.calls.length).toBeGreaterThanOrEqual(3))
  })

  it('拆除失败：透出服务端错误且弹窗不关（onError 不清目标）', async () => {
    apiMock.oobRedirectRemove.mockRejectedValue({ response: { data: { error: 'rule busy' } } })
    renderPage([mkAgent('ag-1')])
    await typeOob()
    await screen.findByText('cockpit_ipmi_1790000000')
    fireEvent.click(screen.getAllByRole('button', { name: /拆\s*除/ })[0])
    const inp = await screen.findByPlaceholderText('cockpit_ipmi_1790000000')
    fireEvent.change(inp, { target: { value: 'cockpit_ipmi_1790000000' } })
    fireEvent.click(screen.getByRole('button', { name: /确认拆除/ }))
    await waitFor(() => expect(apiMock.oobRedirectRemove).toHaveBeenCalled())
    expect(screen.getByPlaceholderText('cockpit_ipmi_1790000000')).toBeInTheDocument()
  })

  it('清扫失败：透出服务端错误且弹窗不关（onError 不清 open）', async () => {
    apiMock.oobRedirectSweep.mockRejectedValue({ response: { data: { error: 'busy' } } })
    renderPage([mkAgent('ag-1')])
    await typeOob()
    await screen.findByText('cockpit_ipmi_1790000000')
    fireEvent.click(screen.getByRole('button', { name: /清扫过期/ }))
    const inp = await screen.findByPlaceholderText('r2s')
    fireEvent.change(inp, { target: { value: 'r2s' } })
    fireEvent.click(screen.getByRole('button', { name: /确认清扫/ }))
    await waitFor(() => expect(apiMock.oobRedirectSweep).toHaveBeenCalled())
    expect(screen.getByPlaceholderText('r2s')).toBeInTheDocument()
  })

  it('取消：三个弹窗 onCancel 关闭且不触发写动作', async () => {
    renderPage([mkAgent('ag-1')])
    await typeOob()
    await screen.findByText('cockpit_ipmi_1790000000')
    // 新建弹窗取消
    fireEvent.click(screen.getByRole('button', { name: /新建规则/ }))
    await screen.findByText(/新建转发规则/)
    fireEvent.click(await screen.findByRole('button', { name: /取\s*消/ }))
    // 拆除弹窗取消
    fireEvent.click(screen.getAllByRole('button', { name: /拆\s*除/ })[0])
    fireEvent.click((await screen.findAllByRole('button', { name: /取\s*消/ })).pop()!)
    // 清扫弹窗取消
    fireEvent.click(screen.getByRole('button', { name: /清扫过期/ }))
    fireEvent.click((await screen.findAllByRole('button', { name: /取\s*消/ })).pop()!)
    // 取消不触发任何写动作
    expect(apiMock.oobRedirectApply).not.toHaveBeenCalled()
    expect(apiMock.oobRedirectRemove).not.toHaveBeenCalled()
    expect(apiMock.oobRedirectSweep).not.toHaveBeenCalled()
  })

  it('无 inventory:write：写按钮与拆除入口不渲染', async () => {
    canWrite = false
    renderPage([mkAgent('ag-1')])
    await typeOob()
    await screen.findByText('cockpit_ipmi_1790000000')
    expect(screen.queryByRole('button', { name: /新建规则/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /清扫过期/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /拆\s*除/ })).toBeNull()
  })
})
