import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Nas from './index'
import type { Agent } from '@/types'

// Nas（存储观测）：nasAgents 过滤 / 三表聚合与池排序 / 告警分档（error/warning）/
// 容量高亮 / 巡检配置保存 / 单主机 Collapse 面板分支

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getNASStatus: vi.fn(),
  getNASConfig: vi.fn(),
  putNASConfig: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => true }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const mkAgent = (id: string, hostname: string, nas = true): Agent =>
  ({
    id,
    hostname,
    ip: '1.2.3.4',
    status: 'online',
    lastSeen: '0',
    capabilities: nas ? [{ type: 'nas' }] : [{ type: 'files' }],
  }) as unknown as Agent

const agents = [mkAgent('ag-1', 'web-01'), mkAgent('ag-2', 'db-01'), mkAgent('ag-3', 'nas-x', false)]

// ag-1：failed 池（带 detail）+ healthy 池 + 超阈值挂载 + SMB 共享；ag-2：degraded 池
const statusOf = (agentId: string) => {
  if (agentId === 'ag-1') {
    return {
      available: true,
      source: 'linux',
      pools: [
        { name: 'md0', kind: 'mdadm' as const, state: 'healthy' as const, totalGB: 500, devices: ['sda', 'sdb'] },
        { name: 'tank', kind: 'zfs' as const, state: 'failed' as const, totalGB: 2048, detail: 'one or more devices faulted' },
      ],
      mounts: [
        { device: '/dev/md0', mountPath: '/data', fsType: 'ext4', totalGB: 500, usedGB: 470 }, // 94% 超 80
      ],
      shares: [
        { protocol: 'smb' as const, name: 'media', path: '/srv/media', comment: '媒体', hosts: 'all' },
      ],
    }
  }
  return {
    available: true,
    source: 'linux',
    pools: [
      { name: 'vol0', kind: 'lvm' as const, state: 'degraded' as const, totalGB: 100, host: 'truenas-box' },
    ],
    mounts: [],
    shares: [],
  }
}

const renderPage = (statusImpl: (id: string) => unknown = statusOf) => {
  apiMock.getAgents.mockResolvedValue(agents)
  apiMock.getNASStatus.mockImplementation(statusImpl)
  apiMock.getNASConfig.mockResolvedValue({
    scan_interval_seconds: 0, min: 300, max: 86400, default: 1800,
    usage_warn_percent: 80, usageMin: 50, usageMax: 99,
  })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Nas />
    </QueryClientProvider>,
  )
}

const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(cell)) as HTMLTableRowElement

const panelHeader = (name: string) =>
  Array.from(document.querySelectorAll('.ant-collapse-header')).find(
    (h) => h.textContent === name) as HTMLElement

describe('Nas', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('总览：failed 池排前、error 告警、三表聚合与主机列', async () => {
    renderPage()
    expect(await screen.findByText('tank')).toBeInTheDocument()
    // severity：ag-1 failed(0) → ag-2 degraded(1) → ag-1 healthy(3)
    expect(rowOf('tank')).toBe(document.querySelector('tr.ant-table-row'))
    expect(screen.getByText('1 个存储池故障，数据访问可能中断，请立即检查')).toBeInTheDocument()
    // 徽标与格式化：故障/健康、TB 档容量、成员盘顿号连接
    expect(screen.getByText('故障')).toBeInTheDocument()
    expect(screen.getByText('健康')).toBeInTheDocument()
    expect(screen.getByText('2.0 TB')).toBeInTheDocument()
    expect(screen.getByText('sda、sdb')).toBeInTheDocument()
    // 挂载点容量卡 + 网络共享卡
    expect(screen.getByText('挂载点容量')).toBeInTheDocument()
    expect(rowOf('/data').textContent).toContain('94%')
    expect(screen.getByText('SMB')).toBeInTheDocument()
    expect(screen.getByText('网络共享')).toBeInTheDocument()
    // 总览行带主机列，网络 NAS host 拼接「agent · 设备」
    expect(screen.getByText('db-01 · truenas-box')).toBeInTheDocument()
  })

  it('无降级仅容量超限时 warning 告警且文本拼接', async () => {
    renderPage(() => Promise.resolve({
      available: true,
      source: 'linux',
      pools: [{ name: 'p0', kind: 'zfs' as const, state: 'healthy' as const }],
      mounts: [{ device: '/dev/sda1', mountPath: '/', fsType: 'ext4', totalGB: 100, usedGB: 85 }],
      shares: [],
    }))
    // 两台 nas 主机各一枚超限挂载 → 2 个
    expect(await screen.findByText('2 个挂载点容量超过 80%')).toBeInTheDocument()
    // 无共享：网络共享卡不渲染
    expect(screen.queryByText('网络共享')).toBeNull()
  })

  it('无 NAS 主机：空态且不发状态查询', async () => {
    apiMock.getAgents.mockResolvedValue([mkAgent('ag-3', 'nas-x', false)])
    renderPage()
    expect(await screen.findByText('暂无支持存储观测的在线主机——装有 mdadm/ZFS/LVM 或 Samba/NFS 的主机运行 Agent 即可')).toBeInTheDocument()
    expect(apiMock.getNASStatus).not.toHaveBeenCalled()
  })

  it('巡检配置：改阈值与间隔保存；warnPct 越界拦截', async () => {
    renderPage()
    const sw = await screen.findByRole('switch')
    await waitFor(() => expect(sw).not.toHaveClass('ant-switch-loading'))
    await act(async () => {
      fireEvent.click(sw)
    })
    // 两个 InputNumber：分钟与容量阈值
    const minuteInput = document.querySelector('.ant-input-number input') as HTMLInputElement
    await waitFor(() => expect(minuteInput).toBeTruthy())
    fireEvent.change(minuteInput, { target: { value: '30' } })
    const pctInput = document.querySelectorAll('.ant-input-number input')[1] as HTMLInputElement
    fireEvent.change(pctInput, { target: { value: '90' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(apiMock.putNASConfig).toHaveBeenCalledWith(1800, 90))
    expect(msgSuccess).toHaveBeenCalledWith('巡检设置已保存')
  })

  it('按主机面板：展开渲染三段表格，覆盖 mock 后走失败分支', async () => {
    renderPage()
    await screen.findByText('tank')
    fireEvent.click(panelHeader('db-01'))
    expect(await screen.findByText('vol0')).toBeInTheDocument()
    apiMock.getNASStatus.mockRejectedValue(new Error('down'))
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText('该主机存储状态获取失败')).toBeInTheDocument()
  })

  it('available=false：总览空表与面板未发现存储', async () => {
    renderPage(() => Promise.resolve({ available: false, source: '', pools: [], mounts: [], shares: [] }))
    expect(await screen.findByText('未发现存储池')).toBeInTheDocument()
    fireEvent.click(panelHeader('db-01'))
    expect(await screen.findByText('未发现可观测的存储（mdadm/ZFS/LVM/SMB/NFS 均无数据）')).toBeInTheDocument()
  })

  // ---- 巡检校验 / 保存分支 ----

  const renderWithCfg = (cfg: unknown) => {
    apiMock.getAgents.mockResolvedValue(agents)
    apiMock.getNASStatus.mockImplementation(statusOf)
    apiMock.getNASConfig.mockResolvedValue(cfg)
    apiMock.putNASConfig.mockResolvedValue({})
    return render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <Nas />
      </QueryClientProvider>,
    )
  }

  // Switch 的 loading={!scanCfg} 会吞点击——等配置落地
  const waitForCfg = async () => {
    const sw = await screen.findByRole('switch')
    await waitFor(() => expect(sw).not.toHaveClass('ant-switch-loading'))
    return sw
  }

  it('服务端已开巡检但间隔过小：保存触发间隔校验告警', async () => {
    // scan_interval_seconds=60 → 派生 minutes=1 <5
    renderWithCfg({ scan_interval_seconds: 60, usage_warn_percent: 80, usageMin: 50, usageMax: 99 })
    const sw = await screen.findByRole('switch')
    await waitFor(() => expect(sw).not.toHaveClass('ant-switch-loading'))
    expect(sw).toBeChecked()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('间隔需在 5～1440 分钟之间'))
    expect(apiMock.putNASConfig).not.toHaveBeenCalled()
  })

  it('容量阈值越界：带 min/max 与缺省 min/max 两种配置均拦截', async () => {
    // A：服务端带 min/max 且已存阈值低于下限
    renderWithCfg({ scan_interval_seconds: 0, usage_warn_percent: 40, usageMin: 50, usageMax: 99 })
    await waitForCfg()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('容量阈值需在 50～99 之间'))
    expect(apiMock.putNASConfig).not.toHaveBeenCalled()
  })

  it('容量阈值越界：配置缺省 min/max 时用 50～99 兜底文案', async () => {
    renderWithCfg({ scan_interval_seconds: 0, usage_warn_percent: 40 })
    await waitForCfg()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('容量阈值需在 50～99 之间'))
  })

  it('关闭态保存 0 秒；配置缺省字段时条件两侧 ?? 兜底走完；保存失败报错', async () => {
    renderWithCfg({ scan_interval_seconds: 0 }) // usageMin/Max/usage_warn_percent 均缺省
    await waitForCfg()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    // scanOn=false → seconds=0；warnPct=80 落在兜底 50～99 内
    await waitFor(() => expect(apiMock.putNASConfig).toHaveBeenCalledWith(0, 80))
    expect(msgSuccess).toHaveBeenCalledWith('巡检设置已保存')
    // 失败分支
    apiMock.putNASConfig.mockRejectedValue(new Error('boom'))
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
  })

  it('服务端已开巡检：间隔分钟按服务端值派生；输入改动与清空回落默认', async () => {
    renderWithCfg({ scan_interval_seconds: 3600, usage_warn_percent: 80, usageMin: 50, usageMax: 99 })
    const sw = await screen.findByRole('switch')
    await waitFor(() => expect(sw).not.toHaveClass('ant-switch-loading'))
    expect(sw).toBeChecked()
    const minuteInput = await waitFor(() => {
      const el = document.querySelector('.ant-input-number input') as HTMLInputElement
      if (!el) throw new Error('input not rendered')
      return el
    })
    // 3600s → 60 分钟派生显示
    expect(minuteInput.value).toBe('60')
    fireEvent.change(minuteInput, { target: { value: '45' } }) // v ?? 30 左侧
    fireEvent.change(minuteInput, { target: { value: '' } }) // v ?? 30 右侧（回落 30）
    const pctInput = document.querySelectorAll('.ant-input-number input')[1] as HTMLInputElement
    fireEvent.change(pctInput, { target: { value: '' } }) // v ?? 80 右侧
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(apiMock.putNASConfig).toHaveBeenCalledWith(1800, 80))
  })

  // ---- 渲染分支边界 ----

  it('池状态/类型与共享协议未知值回落；降级-only 出 warning 拼接文案', async () => {
    renderPage(() => Promise.resolve({
      available: true,
      source: 'linux',
      pools: [
        { name: 'u0', kind: 'ceph' as never, state: 'unknown' as const, totalGB: 10 },
        { name: 'r0', kind: 'zfs' as const, state: 'resync' as const, totalGB: 10 },
        { name: 'w0', kind: 'zfs' as const, state: 'weird' as never, totalGB: 10, detail: 'x' },
      ],
      mounts: [],
      shares: [{ protocol: 'webdav' as never, name: 's1', path: '/s1', comment: '', hosts: '' }],
    }))
    expect((await screen.findAllByRole('cell', { name: 'u0' })).length).toBeGreaterThanOrEqual(1)
    // unknown 状态（severity 2）与 KIND_LABEL 缺省回落
    expect(screen.getAllByText('未知').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('ceph').length).toBeGreaterThanOrEqual(2)
    // resync → 降级/同步中计入 degraded 文案
    expect(screen.getByText('2 个存储池降级/同步中')).toBeInTheDocument()
    // 未知协议回落原值；说明/允许主机空串 → —
    expect(screen.getAllByText('webdav').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(2)
  })

  it('挂载点边界：无容量、无已用、未超阈值与超阈值并存', async () => {
    renderPage(() => Promise.resolve({
      available: true,
      source: 'linux',
      pools: [{ name: 'p0', kind: 'lvm' as const, state: 'healthy' as const }],
      mounts: [
        { device: '/dev/a', mountPath: '/a', fsType: 'ext4' }, // 无 totalGB → —
        { device: '/dev/b', mountPath: '/b', fsType: 'ext4', totalGB: 100 }, // 无 usedGB → 0%
        { device: '/dev/c', mountPath: '/c', fsType: 'ext4', totalGB: 100, usedGB: 10 }, // 10% 未超阈值
      ],
      shares: [],
    }))
    expect((await screen.findAllByRole('cell', { name: '/a' })).length).toBeGreaterThanOrEqual(1)
    expect(rowOf('/b').textContent).toContain('0%（—）')
    expect(rowOf('/c').textContent).toContain('10%')
    // 无超阈值挂载 → 不出容量告警（仅 healthy 池）
    expect(screen.queryByText(/挂载点容量超过/)).toBeNull()
  })

  it('单主机面板：三段表格齐渲染；字段缺省走 ?? [] 兜底', async () => {
    renderPage((id: string) => Promise.resolve(
      id === 'ag-1'
        ? {
            available: true,
            source: 'linux',
            // pools/mounts/shares 均缺省 → ?? [] 兜底
          }
        : {
            available: true,
            source: 'linux',
            pools: [{ name: 'vp', kind: 'dsm' as const, state: 'healthy' as const }],
            mounts: [{ device: '/dev/v', mountPath: '/v', fsType: 'btrfs', totalGB: 10, usedGB: 1 }],
            shares: [{ protocol: 'nfs' as const, name: 'exp', path: '/exp' }],
          },
    ))
    expect(await screen.findByText('vp')).toBeInTheDocument()
    // ag-2 面板：三段（含 ShareTable showAgent=false 分支），面板与总览各一份
    fireEvent.click(panelHeader('db-01'))
    await waitFor(() => expect(screen.getAllByText('/exp').length).toBeGreaterThanOrEqual(2))
    expect(screen.getAllByText('NFS').length).toBeGreaterThanOrEqual(2)
    expect(screen.getAllByText('/v').length).toBeGreaterThanOrEqual(2)
    // ag-1 面板：pools/mounts/shares 缺省 → 两空表 + 无共享卡
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText('未发现挂载点')).toBeInTheDocument()
    expect(screen.getAllByText('未发现存储池').length).toBeGreaterThanOrEqual(1)
  })

  it('无 id/hostname 的主机：主机列与排序的 agent 兜底为空串', async () => {
    const ghost = {
      ip: '9.9.9.9',
      status: 'online',
      lastSeen: '0',
      capabilities: [{ type: 'nas' }],
    } as unknown as Agent
    // capabilities 字段缺省 → 过滤处 ?? [] 兜底（被过滤掉）
    const noCaps = { id: 'ag-nocaps', hostname: 'nocaps', ip: '8.8.8.8', status: 'online', lastSeen: '0' } as unknown as Agent
    apiMock.getAgents.mockResolvedValue([ghost, noCaps])
    apiMock.getNASConfig.mockResolvedValue({ scan_interval_seconds: 0 })
    apiMock.getNASStatus.mockResolvedValue({
      available: true,
      source: 'linux',
      pools: [
        { name: 'g1', kind: 'zfs' as const, state: 'healthy' as const, host: 'net-disk' },
        { name: 'g2', kind: 'zfs' as const, state: 'healthy' as const },
      ],
      mounts: [],
      shares: [],
    })
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <Nas />
      </QueryClientProvider>,
    )
    // host 非空 → 「 · 设备」拼接（agent 兜底 ''）；无 host → agent ?? ''
    expect(await screen.findByText('· net-disk')).toBeInTheDocument()
    expect(screen.getByText('g2')).toBeInTheDocument()
    // 同 severity 两池触发 localeCompare 两侧 agent 兜底
    expect(document.querySelectorAll('tr.ant-table-row').length).toBe(2)
    expect(apiMock.getNASStatus).not.toHaveBeenCalledWith('ag-nocaps')
  })
})
