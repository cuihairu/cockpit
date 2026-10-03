import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ComputePage from './ComputePage'
import type { ComputeInstance } from '@/types'

// ComputePage：子页拆分（3e6adea）后的计算实例独立页——表渲染 + 展开行
// （ComputeDetail）与配置列/操作列的类型分支。四行 fixture 刻意覆盖
// columns 配置列的 if 链四条路径与操作列 vm/container/baremetal 三态。

const apiMock = vi.hoisted(() => ({
  getComputeInstances: vi.fn(),
  getDomains: vi.fn(),
  getCertificates: vi.fn(),
  getServices: vi.fn(),
  getGateways: vi.fn(),
  getStorages: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const settingsRef = { current: { showResourceCount: true, refreshInterval: 3600 } }
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsRef.current }),
}))

const rows = [
  // 满字段 vm：配置列三段全出（|| 左侧全真）、操作列命中 vm 分支
  {
    id: 'c1', name: 'full-vm', type: 'vm', agentId: 'ag-1', region: 'cn', zone: 'a',
    status: 'running', cpuCores: 4, memoryMb: 8192, diskGb: 100, ipv4: '10.0.0.1',
  },
  // container 只有内存：cpu/disk 走 || 0 右侧，!cpu 真而 !mem 假（链第二段断）
  {
    id: 'c2', name: 'thin-container', type: 'container', agentId: '', region: '', zone: '',
    status: 'stopped', memoryMb: 512, ipv4: '',
  },
  // vm 只有磁盘：链走满三段在 !disk 断（— 不出、磁盘段出）
  {
    id: 'c3', name: 'disk-only', type: 'vm', agentId: '', status: 'running', diskGb: 8, ipv4: '',
  },
  // baremetal 全缺省：链全真出「—」；操作列走 else（无动作按钮）
  {
    id: 'c4', name: 'bare', type: 'baremetal', agentId: '', status: 'error', ipv4: '',
  },
] as unknown as ComputeInstance[]

const renderPage = (data: ComputeInstance[] = rows) => {
  // useResources 六查询并发：即使本页只消费计算实例，其余五个也必须可解析
  apiMock.getComputeInstances.mockResolvedValue({ data })
  apiMock.getDomains.mockResolvedValue({ data: [] })
  apiMock.getCertificates.mockResolvedValue({ data: [] })
  apiMock.getServices.mockResolvedValue({ data: [] })
  apiMock.getGateways.mockResolvedValue({ data: [] })
  apiMock.getStorages.mockResolvedValue({ data: [] })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <ComputePage />
    </QueryClientProvider>,
  )
}

const rowOf = async (name: string) => {
  const cell = await screen.findByText(name)
  return cell.closest('tr') as HTMLElement
}

describe('ComputePage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    settingsRef.current = { showResourceCount: true, refreshInterval: 3600 }
  })

  it('渲染计算实例列表', async () => {
    renderPage([])
    expect(await screen.findByText('计算实例')).toBeInTheDocument()
  })

  it('配置列/操作列类型分支：满字段、部分字段、全缺省与 baremetal', async () => {
    renderPage()
    const r1 = await rowOf('full-vm')
    expect(within(r1).getByText('4 核')).toBeInTheDocument()
    expect(within(r1).getByText('8 GB')).toBeInTheDocument()
    expect(within(r1).getByText('100 GB')).toBeInTheDocument()

    const r2 = await rowOf('thin-container')
    expect(within(r2).getByText('512 MB')).toBeInTheDocument()

    const r3 = await rowOf('disk-only')
    expect(within(r3).getByText('8 GB')).toBeInTheDocument()

    const r4 = await rowOf('bare')
    // 全缺省 → 配置列出「—」；baremetal 走操作列 else 分支（云主机占位按钮）
    expect(within(r4).getByText('—')).toBeInTheDocument()
    // 行内还有展开图标按钮（antd 渲染为 <button>），动作按钮取操作列单元格
    const actionBtn = (row: HTMLElement) => row.querySelector('td:last-child button') as HTMLElement
    expect(actionBtn(r4)).toBeDisabled()
    // vm/container 行的动作按钮渲染（Tooltip 包装的禁用电源按钮）
    expect(actionBtn(r1)).toBeDisabled()
    expect(actionBtn(r2)).toBeDisabled()
  })

  it('展开行渲染 ComputeDetail（expandedRowRender/rowExpandable 接线）', async () => {
    renderPage()
    await rowOf('full-vm')
    fireEvent.click(document.querySelector('.ant-table-row-expand-icon') as HTMLElement)
    // 展开行内容出实例 ID（表格行 key 不渲染文本，ID 唯一来自详情卡）
    await waitFor(() => expect(screen.getByText('c1')).toBeInTheDocument())
  })

  it('刷新按钮触发 fetchAll（refetch 通道）', async () => {
    renderPage()
    await rowOf('full-vm')
    apiMock.getComputeInstances.mockClear()
    fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
    await waitFor(() => expect(apiMock.getComputeInstances).toHaveBeenCalled())
  })
})
