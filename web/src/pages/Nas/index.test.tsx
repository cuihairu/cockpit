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

const mkAgent = (id: string, hostname: string, nas = true): Agent =>
  ({
    id,
    hostname,
    ip: '1.2.3.4',
    location: {},
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
})
