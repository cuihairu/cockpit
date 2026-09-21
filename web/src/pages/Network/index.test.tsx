import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Network from './index'
import type { Agent } from '@/types'

// Network：mesh 聚合（跨 agent 合并取最低延迟、unavailable 跳过、在线置顶）、
// 单主机面板（身份 chip/工具卡/interfaces/peers）、云端管理（ZT 授权除名、
// TS 授权与删除确认、未纳管汇总）、未配置引导、RBAC

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getOverlayStatus: vi.fn(),
  getOverlayCloud: vi.fn(),
  setOverlayZTMemberAuthorized: vi.fn(),
  removeOverlayZTMember: vi.fn(),
  authorizeOverlayTSDevice: vi.fn(),
  removeOverlayTSDevice: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => canWrite }))

const msgSuccess = vi.spyOn(message, 'success')

const mkOverlayAgent = (id: string, hostname: string, identity?: object, status = 'online'): Agent =>
  ({
    id,
    hostname,
    ip: '10.0.0.1',
    location: { region: 'cn', zone: 'z1' },
    status,
    lastSeen: '0',
    capabilities: [{ type: 'overlay', metadata: identity ? { identity } : {} }],
  }) as unknown as Agent

const agents = [
  mkOverlayAgent('ag-1', 'node-01', { nodeId: 'zt123', id: 'ts456' }),
  mkOverlayAgent('ag-2', 'node-02'),
  mkOverlayAgent('ag-3', 'empty-01'),
  { id: 'ag-plain', hostname: 'plain', ip: '10.0.0.9', location: { region: 'cn', zone: 'z1' },
    status: 'online', lastSeen: '0', capabilities: [{ type: 'files' }] } as unknown as Agent,
  mkOverlayAgent('ag-off', 'off-01', undefined, 'offline'),
]

// ag-1/ag-2 的 zerotier 都能看到 p1（跨 agent 合并取最低延迟）；
// ag-1 另有 wg 接口 peer（endpoint 兜底）；ag-2 有离线 p2；tailscale unavailable 跳过
const statusOf = (id: string) => {
  if (id === 'ag-1')
    return {
      tools: [
        { tool: 'zerotier', status: 'ok', version: '1.12.2', peers: [
          { id: 'p1', name: 'peer-a', online: true, virtualIps: ['10.0.0.5'], latencyMs: 20, version: '1.12' }] },
        { tool: 'wireguard', status: 'ok', peers: [], interfaces: [
          { name: 'wg0', peerCount: 1, listenPort: 51820, peers: [
            { id: 'wp1', online: false, endpoint: '1.2.3.4:51820' }] }] },
        { tool: 'tailscale', status: 'unavailable' },
      ],
    }
  if (id === 'ag-2')
    return {
      tools: [
        { tool: 'zerotier', status: 'degraded', peers: [
          { id: 'p1', name: 'peer-a', online: true, virtualIps: ['10.0.0.5'], latencyMs: 50 },
          { id: 'p2', name: 'peer-b', online: false, virtualIps: [], latencyMs: 0 }] },
      ],
    }
  return { tools: [{ tool: 'tailscale', status: 'unavailable' }] }
}

const cloud = {
  zerotier: {
    configured: true,
    networks: [
      { id: 'net1', name: 'prod', members: [
        { id: 'm1', name: 'mbp', authorized: true, online: true, ips: ['10.0.0.9'], version: '1.12', managed: true },
        { id: 'm2', name: '', authorized: false, online: false, ips: [], managed: false }] },
    ],
  },
  tailscale: {
    configured: true,
    devices: [
      { id: 'd1', name: 'phone', addresses: ['100.64.0.1'], user: 'ops@x.com', os: 'iOS', online: true,
        authorized: true, keyExpiry: '2099-01-01', managed: true },
      { id: 'd2', name: 'vm-2', addresses: [], user: '', os: '', online: false,
        authorized: false, keyExpiry: '2000-01-01', managed: false },
    ],
  },
}

const renderPage = (cloudOverride?: unknown) => {
  apiMock.getAgents.mockResolvedValue(agents)
  apiMock.getOverlayStatus.mockImplementation(statusOf)
  apiMock.getOverlayCloud.mockResolvedValue(cloudOverride ?? cloud)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Network />
    </QueryClientProvider>,
  )
}

const meshRow = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('.ant-card tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(cell)) as HTMLTableRowElement

const switchToCloud = async () => {
  fireEvent.click(screen.getByText('云端管理'))
  // 配置/未配置两态都有的汇总 Alert 作锚点
  await screen.findByText(/云端共 \d+ 台设备/)
}

describe('Network', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    canWrite = true
  })

  it('mesh 视图：跨 agent 合并取最低延迟、unavailable 跳过、在线置顶与 endpoint 兜底', async () => {
    renderPage()
    expect(await screen.findByText('peer-a')).toBeInTheDocument()
    // p1 合并两台来源、延迟取 20（最低）、IP 去重
    const r1 = meshRow('peer-a')
    expect(within(r1).getByText('ZeroTier')).toBeInTheDocument()
    expect(within(r1).getByText('10.0.0.5')).toBeInTheDocument()
    expect(within(r1).getByText('20ms')).toBeInTheDocument()
    expect(within(r1).getByText('node-01, node-02')).toBeInTheDocument()
    // 在线置顶：peer-a(true) 在 wp1/p2(false) 之前；wireguard 按 tool 序在 zerotier 前
    const rows = Array.from(document.querySelectorAll('.ant-card tr.ant-table-row'))
      .map((tr) => tr.querySelector('td:nth-child(2)')?.textContent)
    expect(rows[0]).toContain('peer-a')
    expect(rows[1]).toContain('wp1')
    expect(rows[2]).toContain('peer-b')
    // wg peer 无虚拟 IP → mesh 虚拟 IP 列兜底；仅 ag-1 可见 → 单来源 Tag
    expect(within(meshRow('wp1')).getAllByText('—').length).toBeGreaterThanOrEqual(1)
    expect(within(meshRow('wp1')).getByText('node-01')).toBeInTheDocument()
    expect(within(meshRow('peer-b')).getByText('离线')).toBeInTheDocument()
    // tailscale unavailable 不出现在 mesh
    expect(screen.queryByText('Tailscale', { selector: '.ant-table-cell .ant-tag' })).not.toBeInTheDocument()
    // 按主机查看面板标题（offline/无能力主机被过滤）
    expect(screen.getByText('node-01', { selector: '.ant-collapse-header-text' })).toBeInTheDocument()
    expect(screen.getByText('node-02', { selector: '.ant-collapse-header-text' })).toBeInTheDocument()
    expect(screen.getByText('empty-01', { selector: '.ant-collapse-header-text' })).toBeInTheDocument()
    expect(screen.queryByText('off-01', { selector: '.ant-collapse-header-text' })).not.toBeInTheDocument()
  })

  it('主机面板：身份 chip、工具卡状态/版本、interfaces 与 peers 表', async () => {
    renderPage()
    expect(await screen.findByText('peer-a')).toBeInTheDocument()
    // 展开 node-01 面板（Collapse 第一个）
    fireEvent.click(screen.getByText('node-01', { selector: '.ant-collapse-header-text' }))
    expect(await screen.findByText('本机身份（云端对照用）：')).toBeInTheDocument()
    expect(screen.getByText('ZeroTier zt123')).toBeInTheDocument()
    expect(screen.getByText('Tailscale ts456')).toBeInTheDocument()
    expect(screen.getByText('v1.12.2')).toBeInTheDocument()
    // wireguard 卡：接口描述 + 接口内 peer
    expect(screen.getByText('1 个 peer · 监听 51820')).toBeInTheDocument()
    expect(screen.getAllByText('1.2.3.4:51820').length).toBeGreaterThanOrEqual(1)
    // ok 状态映射「正常」（zerotier/wireguard 两张卡）
    expect(screen.getAllByText('正常').length).toBeGreaterThanOrEqual(2)
  })

  it('主机面板：全部 unavailable 出空态、拉取失败出告警', async () => {
    renderPage()
    expect(await screen.findByText('peer-a')).toBeInTheDocument()
    fireEvent.click(screen.getByText('empty-01', { selector: '.ant-collapse-header-text' }))
    expect(await screen.findByText('未检测到组网工具')).toBeInTheDocument()
    apiMock.getOverlayStatus.mockImplementation((id: string) =>
      id === 'ag-1' ? Promise.reject(new Error('boom')) : Promise.resolve(statusOf(id)))
    fireEvent.click(screen.getByText('node-01', { selector: '.ant-collapse-header-text' }))
    await waitFor(() => expect(apiMock.getOverlayStatus).toHaveBeenCalledWith('ag-1'))
    // 面板独立查询失败 → 告警
    expect(await screen.findByText('该主机状态获取失败')).toBeInTheDocument()
  })

  it('无 overlay 主机：空态且不发状态查询', async () => {
    apiMock.getAgents.mockResolvedValue([agents[3]])
    renderPage()
    expect(await screen.findByText(/暂无带组网能力（overlay）的在线主机/)).toBeInTheDocument()
    expect(apiMock.getOverlayStatus).not.toHaveBeenCalled()
  })

  it('云端视图：未纳管汇总、ZT 成员表、TS 设备表与密钥过期标红', async () => {
    renderPage()
    await switchToCloud()
    expect(await screen.findByText(/云端共 4 台设备，其中 2 台未纳管/)).toBeInTheDocument()
    // ZT 网络标题带成员数
    expect(screen.getByText('prod（2 台成员）')).toBeInTheDocument()
    const m1 = meshRow('mbp')
    expect(within(m1).getByText('10.0.0.9')).toBeInTheDocument()
    expect(within(m1).getByText('面板纳管')).toBeInTheDocument()
    const m2 = meshRow('m2')
    expect(within(m2).getByText('未纳管')).toBeInTheDocument()
    expect(within(m2).getAllByText('—').length).toBeGreaterThanOrEqual(1)
    // TS 设备：d2 密钥过期标红、未授权出授权按钮、地址空兜底
    const d1 = meshRow('phone')
    expect(within(d1).getByText('100.64.0.1')).toBeInTheDocument()
    expect(within(d1).getByText('已授权')).toBeInTheDocument()
    const d2 = meshRow('vm-2')
    expect(within(d2).getByText(/已过期/)).toBeInTheDocument()
    expect(within(d2).getByRole('button', { name: /授\s*权/ })).toBeInTheDocument()
    expect(within(d2).getAllByText('—').length).toBeGreaterThanOrEqual(3)
  })

  it('ZT 操作：授权开关翻转与除名确认', async () => {
    apiMock.setOverlayZTMemberAuthorized.mockResolvedValue({})
    apiMock.removeOverlayZTMember.mockResolvedValue({})
    renderPage()
    await switchToCloud()
    expect(await screen.findByText('prod（2 台成员）')).toBeInTheDocument()
    fireEvent.click(meshRow('mbp').querySelector('.ant-switch')!)
    await waitFor(() =>
      expect(apiMock.setOverlayZTMemberAuthorized).toHaveBeenCalledWith('net1', 'm1', false))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已取消授权'))
    fireEvent.click(within(meshRow('m2')).getByRole('button', { name: /除\s*名/ }))
    expect(await screen.findByText('除名该成员？')).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.removeOverlayZTMember).toHaveBeenCalledWith('net1', 'm2'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已除名（设备重新加入后可再授权）'))
  })

  it('TS 操作：授权与设备名确认删除', async () => {
    apiMock.authorizeOverlayTSDevice.mockResolvedValue({})
    apiMock.removeOverlayTSDevice.mockResolvedValue({})
    renderPage()
    await switchToCloud()
    expect(await screen.findByText('phone')).toBeInTheDocument()
    fireEvent.click(within(meshRow('vm-2')).getByRole('button', { name: /授\s*权/ }))
    await waitFor(() => expect(apiMock.authorizeOverlayTSDevice).toHaveBeenCalledWith('d2'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已授权'))
    // 删除确认：输入设备名前 ok 禁用
    fireEvent.click(within(meshRow('phone')).getByRole('button', { name: /删\s*除/ }))
    expect(await screen.findByText('删除 Tailscale 设备')).toBeInTheDocument()
    const okBtn = document.querySelector('.ant-modal-footer .ant-btn-primary')! as HTMLButtonElement
    expect(okBtn.disabled).toBe(true)
    fireEvent.change(document.querySelector('.ant-modal input')!, { target: { value: 'pho' } })
    expect(okBtn.disabled).toBe(true)
    fireEvent.change(document.querySelector('.ant-modal input')!, { target: { value: 'phone' } })
    expect(okBtn.disabled).toBe(false)
    fireEvent.click(okBtn)
    await waitFor(() => expect(apiMock.removeOverlayTSDevice).toHaveBeenCalledWith('d1'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('设备已删除'))
  })

  it('云端未配置：两个 provider 引导卡片', async () => {
    renderPage({
      zerotier: { configured: false },
      tailscale: { configured: false, tailnet: '' },
    })
    await switchToCloud()
    expect(await screen.findByText(/ZeroTier Central（未配置）/)).toBeInTheDocument()
    expect(screen.getByText(/Tailscale（未配置）/)).toBeInTheDocument()
    expect(screen.getByText('overlay.zerotier.api_token')).toBeInTheDocument()
    expect(screen.getByText('TAILSCALE_API_TOKEN')).toBeInTheDocument()
  })

  it('RBAC 无写权限：开关降级禁用、操作按钮隐藏', async () => {
    canWrite = false
    renderPage()
    await switchToCloud()
    expect(await screen.findByText('prod（2 台成员）')).toBeInTheDocument()
    expect(meshRow('mbp').querySelector('.ant-switch')!.className).toContain('disabled')
    expect(within(meshRow('m2')).queryByRole('button', { name: /除\s*名/ })).not.toBeInTheDocument()
    expect(within(meshRow('vm-2')).queryByRole('button', { name: /授\s*权/ })).not.toBeInTheDocument()
    expect(within(meshRow('phone')).queryByRole('button', { name: /删\s*除/ })).not.toBeInTheDocument()
  })
})
