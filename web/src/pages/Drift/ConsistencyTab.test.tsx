import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ConsistencyTab from './ConsistencyTab'
import type { AgentConsistency, ConsistencyReport } from '@/types'

// ConsistencyTab：汇总条/全一致/空声明/失败、Facts 单侧缺省、mismatch 展开与
// 不可展开、>20 台分页、刷新按钮重查

const apiMock = vi.hoisted(() => ({ getInventoryConsistency: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const renderTab = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <ConsistencyTab />
    </QueryClientProvider>,
  )
}

const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('.ant-table-tbody tr.ant-table-row'))
    .find((tr) => tr.textContent?.includes(cell)) as HTMLTableRowElement

describe('ConsistencyTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('Facts 缺省三分支：无 facts / 仅 ip / 有 ip 并列', async () => {
    apiMock.getInventoryConsistency.mockResolvedValue({
      summary: { total: 4, ok: 1, mismatch: 1, unregistered: 1, undeclared: 1 },
      agents: [
        { id: 'ag-1', status: 'ok', declared: { hostname: 'h1', ip: '10.0.0.1' }, actual: { hostname: 'h1', ip: '10.0.0.1' } },
        // declared 缺失 → Facts 走空态
        { id: 'ag-2', status: 'undeclared', actual: { hostname: 'w2' } },
        // 仅 ip 无 hostname → hostname 兜底 -
        { id: 'ag-3', status: 'unregistered', declared: { ip: '10.0.0.3' } },
        // declared/actual 均为空对象 → 空态
        { id: 'ag-4', status: 'ok', declared: {}, actual: {} },
      ],
    } as unknown as ConsistencyReport)
    renderTab()
    expect(await screen.findByText('共 4 台')).toBeInTheDocument()
    const r2 = rowOf('ag-2')
    expect(r2.textContent).toContain('-')
    const r3 = rowOf('ag-3')
    expect(r3.textContent).toContain('10.0.0.3')
    // 空对象 facts 同样走空态
    expect(rowOf('ag-4').textContent).toContain('-')
  })

  it('mismatch 可展开字段对照；mismatch 为空数组不可展开', async () => {
    apiMock.getInventoryConsistency.mockResolvedValue({
      summary: { total: 2, ok: 0, mismatch: 2, unregistered: 0, undeclared: 0 },
      agents: [
        { id: 'ag-a', status: 'mismatch',
          declared: { hostname: 'ha' }, actual: { hostname: 'hb' },
          mismatch: [{ field: 'ip', declared: '1.1.1.1', actual: '2.2.2.2' }] },
        // status mismatch 但无 mismatch 明细 → rowExpandable 右分支 false
        { id: 'ag-b', status: 'mismatch', declared: { hostname: 'hc' }, actual: { hostname: 'hd' } },
      ],
    })
    renderTab()
    expect(await screen.findByText('不一致 2')).toBeInTheDocument()
    const ra = rowOf('ag-a')
    fireEvent.click(ra.querySelector('.ant-table-row-expand-icon')!)
    expect(await screen.findByText('IP')).toBeInTheDocument()
    expect(screen.getByText('1.1.1.1')).toBeInTheDocument()
    // 展开表字段名映射：hostname → 主机名
    expect(screen.queryByText('主机名')).not.toBeInTheDocument()
    // 无 mismatch 明细 → 展开图标置灰（spaced）
    expect(rowOf('ag-b').querySelector('.ant-table-row-expand-icon')!.className).toContain('spaced')
  })

  it('超过 20 台出分页；刷新按钮重查', async () => {
    const agents = Array.from({ length: 21 }, (_, i) => ({
      id: `ag-${i}`, status: 'ok', declared: { hostname: `h${i}` }, actual: { hostname: `h${i}` },
    })) as unknown as AgentConsistency[]
    apiMock.getInventoryConsistency.mockResolvedValue({
      summary: { total: 21, ok: 21, mismatch: 0, unregistered: 0, undeclared: 0 }, agents,
    })
    renderTab()
    expect(await screen.findByText('共 21 台')).toBeInTheDocument()
    expect(document.querySelector('.ant-pagination')).not.toBeNull()
    const before = apiMock.getInventoryConsistency.mock.calls.length
    fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
    await waitFor(() => expect(apiMock.getInventoryConsistency.mock.calls.length).toBeGreaterThan(before))
  })

  it('mismatch 字段映射 hostname → 主机名', async () => {
    apiMock.getInventoryConsistency.mockResolvedValue({
      summary: { total: 1, ok: 0, mismatch: 1, unregistered: 0, undeclared: 0 },
      agents: [
        { id: 'ag-h', status: 'mismatch',
          declared: { hostname: 'ha' }, actual: { hostname: 'hb' },
          mismatch: [{ field: 'hostname', declared: 'ha', actual: 'hb' }] },
      ],
    })
    renderTab()
    await screen.findByText('不一致 1')
    fireEvent.click(rowOf('ag-h').querySelector('.ant-table-row-expand-icon')!)
    expect(await screen.findByText('主机名')).toBeInTheDocument()
  })
})
