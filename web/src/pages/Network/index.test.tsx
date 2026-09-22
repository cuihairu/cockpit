import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
    region: 'cn', zone: 'z1',
    status,
    lastSeen: '0',
    capabilities: [{ type: 'overlay', metadata: identity ? { identity } : {} }],
  }) as unknown as Agent

const agents = [
  mkOverlayAgent('ag-1', 'node-01', { nodeId: 'zt123', id: 'ts456' }),
  mkOverlayAgent('ag-2', 'node-02'),
  mkOverlayAgent('ag-3', 'empty-01'),
  { id: 'ag-plain', hostname: 'plain', ip: '10.0.0.9', region: 'cn', zone: 'z1',
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
    expect(screen.getByText('node-01 · cn', { selector: '.ant-collapse-header-text' })).toBeInTheDocument()
    expect(screen.getByText('node-02 · cn', { selector: '.ant-collapse-header-text' })).toBeInTheDocument()
    expect(screen.getByText('empty-01 · cn', { selector: '.ant-collapse-header-text' })).toBeInTheDocument()
    expect(screen.queryByText('off-01 · cn', { selector: '.ant-collapse-header-text' })).not.toBeInTheDocument()
  })

  it('主机面板：身份 chip、工具卡状态/版本、interfaces 与 peers 表', async () => {
    renderPage()
    expect(await screen.findByText('peer-a')).toBeInTheDocument()
    // 展开 node-01 面板（Collapse 第一个）
    fireEvent.click(screen.getByText('node-01 · cn', { selector: '.ant-collapse-header-text' }))
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
    fireEvent.click(screen.getByText('empty-01 · cn', { selector: '.ant-collapse-header-text' }))
    expect(await screen.findByText('未检测到组网工具')).toBeInTheDocument()
    apiMock.getOverlayStatus.mockImplementation((id: string) =>
      id === 'ag-1' ? Promise.reject(new Error('boom')) : Promise.resolve(statusOf(id)))
    fireEvent.click(screen.getByText('node-01 · cn', { selector: '.ant-collapse-header-text' }))
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

  // ---- 边界分支 ----

  const renderCustom = (
    customAgents: Agent[],
    statusImpl: (id: string) => unknown,
    cloudOverride?: unknown,
  ) => {
    apiMock.getAgents.mockResolvedValue(customAgents)
    apiMock.getOverlayStatus.mockImplementation(statusImpl as never)
    apiMock.getOverlayCloud.mockResolvedValue(cloudOverride ?? cloud)
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return render(
      <QueryClientProvider client={qc}>
        <Network />
      </QueryClientProvider>,
    )
  }

  it('mesh 聚合：peers/interfaces/tools 缺省兜底，补名/补版本与非法时间', async () => {
    // 无 hostname → mesh agent 名用 id；无 capabilities 的主机被过滤
    const a1 = {
      id: 'a1', hostname: '', ip: '1.1.1.1', status: 'online', lastSeen: '0',
      capabilities: [{ type: 'overlay', metadata: {} }],
    } as unknown as Agent
    const a2 = {
      id: 'a2', hostname: 'second', ip: '1.1.1.2', status: 'online', lastSeen: '0',
      capabilities: [{ type: 'overlay', metadata: { identity: {} } }],
    } as unknown as Agent
    const bare = { id: 'a3', hostname: 'bare', ip: '1.1.1.3', status: 'online', lastSeen: '0' } as unknown as Agent
    renderCustom([a1, a2, bare], (id: string) =>
      id === 'a1'
        ? {
            tools: [
              // 同对端同时出现在 tool.peers 与 iface.peers → agents 去重走 includes 短路
              {
                tool: 'wireguard', status: 'degraded',
                peers: [{ id: 'p', online: true }],
                interfaces: [{ name: 'wg0', peerCount: 1, peers: [{ id: 'p', online: true }] }],
              },
              // 自定义工具名回落
              { tool: 'customvpn', status: 'my-status', peers: [{ id: 'q', name: 'Q', online: false, lastHandshake: 'not-a-date', virtualIps: [], endpoint: '' }] },
            ],
          }
        : id === 'a2'
          ? {
              tools: [
                { tool: 'wireguard', status: 'ok', peers: [{ id: 'p', name: 'peer-p', version: '1.0', online: true, latencyMs: 5 }] },
              ],
            }
          : {}, // tools 缺省 → ?? [] 兜底
    )
    expect(await screen.findByText('peer-p')).toBeInTheDocument()
    // 自定义工具名回落（mesh 工具列）；无虚拟 IP 无 endpoint → —
    expect(screen.getByText('customvpn')).toBeInTheDocument()
    // 补名后显示短 id
    expect(screen.getAllByText('p').length).toBeGreaterThanOrEqual(1)
    // 面板内：未知状态回落与非法时间原样返回
    fireEvent.click(screen.getByText('a1', { selector: '.ant-collapse-header-text' }))
    expect(await screen.findByText('my-status')).toBeInTheDocument()
    expect(screen.getByText('not-a-date')).toBeInTheDocument()
  })

  it('主机面板：身份缺省/空身份、工具卡缺省字段、网络段与接口段全分支', async () => {
    const mkA = (id: string, hostname: string, identity?: object) =>
      ({
        id, hostname, ip: '2.2.2.2', region: 'cn', status: 'online', lastSeen: '0',
        capabilities: [{ type: 'overlay', metadata: identity ? { identity } : {} }],
      }) as unknown as Agent
    renderCustom(
      [mkA('p1', 'has-tools'), mkA('p2', 'no-identity'), mkA('p3', 'empty-id', {})],
      (id: string) => {
        if (id === 'p1') {
          return {
            tools: [
              {
                tool: 'zerotier', status: 'weird', error: 'daemon down', version: '1.0',
                // 无 peers；网络段三分支：status OK / online / 全无
                networks: [
                  { id: 'n1', name: 'net-a', status: 'OK', ips: ['10.0.0.1'] },
                  { id: 'n2', status: 'SOME', online: true },
                  { id: 'n3', name: 'net-c' }, // status 缺省 → —
                ],
                peers: [{ id: 'x', online: true, latencyMs: 3, lastHandshake: '2026-01-01' }],
              },
              {
                tool: 'wireguard', status: 'error',
                // networks + 仅 interfaces（peers 空）→ marginBottom 的 || 走右侧
                networks: [{ id: 'n4', name: 'net-d', status: 'OK' }],
                interfaces: [{ name: 'wg0', peerCount: 0, peers: [] }], // 无 listenPort
              },
              {
                tool: 'tailscale', status: 'ok',
                networks: [{ id: 'n5', name: 'net-e', status: 'OK' }],
                interfaces: [{ name: 'ts0', peerCount: 2, listenPort: '41641' }],
                peers: [{ id: 'y', name: 'y', online: false, endpoint: '9.9.9.9:1' }],
              },
              {
                tool: 'frp', status: 'ok',
                // 仅 networks（无 peers/interfaces）→ marginBottom 取 0
                networks: [{ id: 'n6', name: 'net-f', status: 'OK' }],
              },
            ],
          }
        }
        if (id === 'p2') return { tools: undefined } // ?? [] 兜底
        return { tools: [{ tool: 'frp', status: 'ok', peers: [{ id: 'z', online: true }] }] }
      },
    )
    // 先展开主面板：error 详情 Tooltip + 网络段三分支 + 接口段（无监听端口）
    fireEvent.click(await screen.findByText('has-tools · cn', { selector: '.ant-collapse-header-text' }))
    // 锚点用面板独有内容（mesh 工具列也会出现工具名）
    expect(await screen.findByText('net-a')).toBeInTheDocument()
    expect(await screen.findByText('详情')).toBeInTheDocument()
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
    expect(screen.getByText('net-d')).toBeInTheDocument()
    expect(screen.getByText('net-e')).toBeInTheDocument()
    expect(screen.getByText('net-f')).toBeInTheDocument()
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('0 个 peer')).toBeInTheDocument() // 无 listenPort 后缀
    expect(screen.getByText('2 个 peer · 监听 41641')).toBeInTheDocument()
    // no-identity 面板：IdentityChips 直接 null；tools 缺省 → 未检测到组网工具
    fireEvent.click(screen.getByText('no-identity · cn', { selector: '.ant-collapse-header-text' }))
    expect(await screen.findByText('未检测到组网工具')).toBeInTheDocument()
    // 空 identity（{}）：nodeId/id 均缺 → chips 空返回 null；锚点为面板对端（mesh 也有一份）
    fireEvent.click(screen.getByText('empty-id · cn', { selector: '.ant-collapse-header-text' }))
    await waitFor(() => expect(screen.getAllByText('z').length).toBeGreaterThanOrEqual(2))
  })

  it('云端：网络/成员/设备字段缺省兜底、错误提示与 tailnet、删除取消', async () => {
    renderCustom(
      [mkOverlayAgent('ag-1', 'node-01')],
      () => ({ tools: [] }),
      {
        zerotier: {
          configured: true,
          error: 'zt api 500',
          // 无 name → 用 id；无 members → ?? 0 与 ?? [] 兜底
          networks: [{ id: 'net-plain' }],
        },
        tailscale: {
          configured: true,
          tailnet: 'example.ts.net',
          error: 'ts api 500',
          devices: [
            // addresses 缺省 → —；keyExpiry 缺省 → —
            { id: 'd-bare', name: 'bare', authorized: true, online: true, managed: true },
            // name 缺省 → 回落 id
            { id: 'd-noname', authorized: true, online: false, managed: false, addresses: ['100.64.0.9'] },
          ],
        },
      },
    )
    await switchToCloud()
    expect(await screen.findByText(/ZeroTier API 错误/)).toBeInTheDocument()
    expect(screen.getByText(/Tailscale API 错误/)).toBeInTheDocument()
    expect(screen.getByText(/tailnet: example\.ts\.net/)).toBeInTheDocument()
    expect(screen.getByText('net-plain（0 台成员）')).toBeInTheDocument()
    expect(screen.getByText('bare')).toBeInTheDocument()
    // name 缺省回落 id：strong 与 code 两处都是 d-noname
    expect(screen.getAllByText('d-noname').length).toBeGreaterThanOrEqual(2)
    // 删除确认后取消（onCancel）
    fireEvent.click(within(meshRow('bare')).getByRole('button', { name: /删\s*除/ }))
    expect(await screen.findByText('删除 Tailscale 设备')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^Cancel$/ }))
  })

  it('云端未配置网络列表：Empty 分支；全部纳管文案', async () => {
    renderCustom([mkOverlayAgent('ag-1', 'node-01')], () => ({ tools: [] }), {
      zerotier: { configured: true, networks: undefined },
      tailscale: { configured: true, devices: [] },
    })
    await switchToCloud()
    expect(await screen.findByText('无网络或拉取失败')).toBeInTheDocument()
    expect(screen.getByText('云端共 0 台设备，全部与面板 Agent 身份对上。')).toBeInTheDocument()
  })

  it('云端拉取失败出错误 Alert；四个操作失败各报错；授权打开成功提示', async () => {
    // 先测成功授权（vars.authorized=true 分支）
    apiMock.setOverlayZTMemberAuthorized.mockResolvedValue({})
    renderPage()
    await switchToCloud()
    expect(await screen.findByText('prod（2 台成员）')).toBeInTheDocument()
    fireEvent.click(meshRow('m2').querySelector('.ant-switch')!) // false → true
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已授权'))
  })

  it('ZT/TS 四类操作失败：错误文案透出', async () => {
    apiMock.setOverlayZTMemberAuthorized.mockRejectedValue(new Error('zt authz'))
    apiMock.removeOverlayZTMember.mockRejectedValue(new Error('zt rm'))
    apiMock.authorizeOverlayTSDevice.mockRejectedValue(new Error('ts authz'))
    apiMock.removeOverlayTSDevice.mockRejectedValue(new Error('ts rm'))
    const msgError = vi.spyOn(message, 'error')
    renderPage()
    await switchToCloud()
    expect(await screen.findByText('prod（2 台成员）')).toBeInTheDocument()
    fireEvent.click(meshRow('mbp').querySelector('.ant-switch')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith(expect.stringContaining('授权失败')))
    fireEvent.click(within(meshRow('m2')).getByRole('button', { name: /除\s*名/ }))
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith(expect.stringContaining('除名失败')))
    fireEvent.click(within(meshRow('vm-2')).getByRole('button', { name: /授\s*权/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith(expect.stringContaining('授权失败')))
    fireEvent.click(within(meshRow('phone')).getByRole('button', { name: /删\s*除/ }))
    const okBtn = document.querySelector('.ant-modal-footer .ant-btn-primary')! as HTMLButtonElement
    fireEvent.change(document.querySelector('.ant-modal input')!, { target: { value: 'phone' } })
    await act(async () => {
      fireEvent.click(okBtn)
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith(expect.stringContaining('删除失败')))
  })

  it('操作 pending 期间 loading 判定分支；云端整体拉取失败', async () => {
    const deferred: Array<() => void> = []
    apiMock.setOverlayZTMemberAuthorized.mockImplementation(
      () => new Promise<void>((res) => deferred.push(() => res())),
    )
    apiMock.removeOverlayZTMember.mockImplementation(
      () => new Promise<void>((res) => deferred.push(() => res())),
    )
    apiMock.authorizeOverlayTSDevice.mockImplementation(
      () => new Promise<void>((res) => deferred.push(() => res())),
    )
    renderPage()
    await switchToCloud()
    expect(await screen.findByText('prod（2 台成员）')).toBeInTheDocument()
    // 授权开关 pending：右侧比较表达式求值（variables.memberId 对比）
    fireEvent.click(meshRow('mbp').querySelector('.ant-switch')!)
    await waitFor(() => expect(apiMock.setOverlayZTMemberAuthorized).toHaveBeenCalled())
    // 除名 pending
    fireEvent.click(within(meshRow('m2')).getByRole('button', { name: /除\s*名/ }))
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.removeOverlayZTMember).toHaveBeenCalled())
    // TS 授权 pending
    fireEvent.click(within(meshRow('vm-2')).getByRole('button', { name: /授\s*权/ }))
    await waitFor(() => expect(apiMock.authorizeOverlayTSDevice).toHaveBeenCalled())
    await act(async () => {
      deferred.forEach((r) => r())
    })
  })

  it('云端整体拉取失败出错误 Alert', async () => {
    renderCustom([mkOverlayAgent('ag-1', 'node-01')], () => ({ tools: [] }), Promise.reject(new Error('down')))
    fireEvent.click(screen.getByText('云端管理'))
    expect(await screen.findByText('云端成员获取失败')).toBeInTheDocument()
  })
})
