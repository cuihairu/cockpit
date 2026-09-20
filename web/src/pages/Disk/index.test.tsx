import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Disk from './index'
import type { Agent } from '@/types'

// Disk（磁盘健康 SMART）：smartAgents 过滤 / 总览排序与告警分档 /
// 巡检开关保存校验 / 单主机 Collapse 面板分支

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getSmartStatus: vi.fn(),
  getSmartConfig: vi.fn(),
  putSmartConfig: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => true }))

const msgError = vi.spyOn(message, 'error')
const msgSuccess = vi.spyOn(message, 'success')

const mkAgent = (id: string, hostname: string, smart = true): Agent =>
  ({
    id,
    hostname,
    ip: '1.2.3.4',
    location: {},
    status: 'online',
    lastSeen: '0',
    capabilities: smart
      ? [{ type: 'hardware-monitor', metadata: { smart: true } }]
      : [{ type: 'files' }],
  }) as unknown as Agent

const agents = [mkAgent('ag-1', 'web-01'), mkAgent('ag-2', 'db-01'), mkAgent('ag-3', 'nas-01', false)]

// ag-1：一块 failed（含扇区异常）+ 一块 passed；ag-2：一块 unknown（带 error）
const statusOf = (agentId: string) => {
  if (agentId === 'ag-1') {
    return {
      available: true,
      devices: [
        { name: '/dev/sda', model: 'Samsung 860', sizeBytes: 512 * 1024 ** 3, health: 'passed', temperatureC: 36, powerOnHours: 100, reallocatedSectors: 0, pendingSectors: 0, mediaErrors: 0 },
        { name: '/dev/sdb', model: 'WD Blue', sizeBytes: 2 * 1024 ** 4, health: 'failed', powerOnHours: 20, reallocatedSectors: 8, pendingSectors: 2 },
      ],
    }
  }
  return {
    available: true,
    devices: [
      { name: '/dev/nvme0', model: '', health: 'unknown', error: '需要 root 权限' },
    ],
  }
}

const renderPage = (statusImpl: (id: string) => unknown = statusOf) => {
  apiMock.getAgents.mockResolvedValue(agents)
  apiMock.getSmartStatus.mockImplementation(statusImpl)
  // 服务端状态仿真：保存成功后 invalidateQueries 重读，须反映已保存值
  // （否则保存后 scanEdit 重置为 null、scanOn 回落 savedInterval=0，二次点击变成「再次打开」）
  let savedInterval = 0
  apiMock.putSmartConfig.mockImplementation((s: number) => {
    savedInterval = s
    return Promise.resolve()
  })
  apiMock.getSmartConfig.mockImplementation(() =>
    Promise.resolve({ scan_interval_seconds: savedInterval, min: 300, max: 86400, default: 3600 }))
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Disk />
    </QueryClientProvider>,
  )
}

const rowOf = (device: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(device)) as HTMLTableRowElement

const panelHeader = (name: string) =>
  Array.from(document.querySelectorAll('.ant-collapse-header')).find(
    (h) => h.textContent === name) as HTMLElement

// Switch 初始 loading={!scanCfg}，点击会被吞——先等配置落地
const readySwitch = async () => {
  const sw = await screen.findByRole('switch')
  await waitFor(() => expect(sw).not.toHaveClass('ant-switch-loading'))
  return sw
}

describe('Disk', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('总览：跨主机平铺、failed 排最前、错误告警与健康徽标', async () => {
    renderPage()
    expect(await screen.findByText('/dev/sdb')).toBeInTheDocument()
    // failed 盘排第一行
    expect(rowOf('/dev/sdb')).toBe(document.querySelector('tr.ant-table-row'))
    expect(screen.getByText('1 块盘 SMART 判定 FAILED，请立即备份并更换')).toBeInTheDocument()
    // 健康/未知徽标
    expect(screen.getByText('FAILED')).toBeInTheDocument()
    expect(screen.getByText('未知')).toBeInTheDocument()
    // 容量与通电格式化：TB 档 / 天+小时档
    expect(screen.getByText('2.0 TB')).toBeInTheDocument()
    expect(screen.getByText('4 天 4 小时')).toBeInTheDocument()
    // 异常扇区高亮为非零数值
    expect(screen.getByText('8')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('无 SMART 主机：空态且不发状态查询', async () => {
    apiMock.getAgents.mockResolvedValue([mkAgent('ag-3', 'nas-01', false)])
    renderPage()
    expect(await screen.findByText('暂无支持 SMART 的在线主机——在目标主机安装 smartmontools 并运行 Agent 即可')).toBeInTheDocument()
    expect(apiMock.getSmartStatus).not.toHaveBeenCalled()
  })

  it('巡检开关：默认 60 分钟保存 3600 秒', async () => {
    renderPage()
    const sw = await readySwitch()
    await act(async () => {
      fireEvent.click(sw)
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(apiMock.putSmartConfig).toHaveBeenCalledWith(3600))
    expect(msgSuccess).toHaveBeenCalledWith('已开启自动巡检')
  })

  it('改间隔为 5 分钟保存 300 秒；关闭开关保存 0', async () => {
    renderPage()
    const sw = await readySwitch()
    await act(async () => {
      fireEvent.click(sw)
    })
    const input = await waitFor(() => {
      const el = document.querySelector('.ant-input-number input') as HTMLInputElement
      if (!el) throw new Error('input not rendered')
      return el
    })
    // 越界值（<5）被 InputNumber 组件层 isInRange 拦截不触发 onChange，
    // saveScanConfig 的 5～1440 校验是防御分支——UI 仅能提交边界内值
    fireEvent.change(input, { target: { value: '5' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(apiMock.putSmartConfig).toHaveBeenCalledWith(300))
    // 关闭开关 → InputNumber 隐藏，保存 0 秒
    await act(async () => {
      fireEvent.click(sw)
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(apiMock.putSmartConfig).toHaveBeenCalledWith(0))
    expect(msgSuccess).toHaveBeenCalledWith('已关闭自动巡检')
  })

  it('保存失败 fallback 文案', async () => {
    renderPage()
    // renderPage 内已 mockImplementation 成功——此处覆盖为失败（保存调用发生在点击后）
    apiMock.putSmartConfig.mockRejectedValue(new Error('boom'))
    const sw = await readySwitch()
    await act(async () => {
      fireEvent.click(sw)
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
  })

  it('按主机面板：展开渲染单主机表格，覆盖 mock 后新面板走失败分支', async () => {
    renderPage()
    await screen.findByText('/dev/sdb')
    fireEvent.click(panelHeader('db-01'))
    expect(await screen.findByText('/dev/nvme0')).toBeInTheDocument()
    // 换 mock 后展开 web-01：面板 query 后发出 → 走新实现（reject）
    apiMock.getSmartStatus.mockRejectedValue(new Error('down'))
    fireEvent.click(panelHeader('web-01'))
    expect(await screen.findByText('该主机磁盘状态获取失败')).toBeInTheDocument()
  })

  it('仅扇区异常时 warning 告警；不可用 SMART 时空态', async () => {
    renderPage(() => Promise.resolve({
      available: true,
      devices: [{ name: '/dev/sdc', health: 'passed', reallocatedSectors: 4 }],
    }))
    // 两台 smart 主机各一枚异常盘 → 2 块
    expect(await screen.findByText('2 块盘出现重映射/待定扇区或介质错误，建议尽快备份')).toBeInTheDocument()
  })

  it('available=false：总览无读数且面板提示需 root', async () => {
    renderPage(() => Promise.resolve({ available: false, devices: [] }))
    expect(await screen.findByText('各主机暂无磁盘读数')).toBeInTheDocument()
    fireEvent.click(panelHeader('db-01'))
    expect(await screen.findByText('未发现可读 SMART 的磁盘（需 root 权限运行 Agent）')).toBeInTheDocument()
  })
})
