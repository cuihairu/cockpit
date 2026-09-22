import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import NotificationDropdown from './index'
import type { Alert } from './index'

// NotificationDropdown：通知中心——展开拉取/已读/全部已读/清空/失败静默

const apiMock = vi.hoisted(() => ({
  getAlerts: vi.fn(),
  markAlertRead: vi.fn(),
  markAllAlertsRead: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const alert = (overrides: Partial<Alert> = {}): Alert => ({
  id: 'a1', type: 'warning', title: 'CPU 高', message: 'cpu 90%',
  created_at: '2026-09-20T10:00:00Z', read: false, ...overrides,
})

const open = async () => {
  fireEvent.click(document.querySelector('.ant-dropdown-trigger')!)
  // 菜单渲染在 portal
  await screen.findByText('通知中心')
}

describe('NotificationDropdown', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.getAlerts.mockResolvedValue({ data: [] })
  })

  it('初始徽标 0；展开拉取，空数据显示「暂无通知」', async () => {
    render(<NotificationDropdown />)
    expect(document.querySelector('.ant-badge-count')?.textContent ?? '').not.toMatch(/^[1-9]/)
    await open()
    expect(apiMock.getAlerts).toHaveBeenCalledTimes(1)
    expect(await screen.findByText('暂无通知')).toBeInTheDocument()
  })

  it('渲染通知列表：标题/消息/类型 tag/未读底色', async () => {
    apiMock.getAlerts.mockResolvedValue({ data: [alert(), alert({ id: 'a2', type: 'error', read: true })] })
    render(<NotificationDropdown />)
    await open()
    expect((await screen.findAllByText('CPU 高')).length).toBe(2)
    expect(screen.getAllByText('cpu 90%').length).toBe(2)
    expect(screen.getByText('warning')).toBeInTheDocument()
    expect(screen.getByText('error')).toBeInTheDocument()
    // 未读徽标 1
    expect(document.querySelector('.ant-badge-count')?.textContent).toBe('1')
  })

  it('点未读条目标记已读；已读条目点击不触发', async () => {
    apiMock.getAlerts.mockResolvedValue({ data: [alert(), alert({ id: 'a2', title: '另一个' })] })
    render(<NotificationDropdown />)
    await open()
    const item = (await screen.findAllByText('CPU 高'))[0].closest('.ant-list-item')!
    fireEvent.click(item)
    await waitFor(() => expect(apiMock.markAlertRead).toHaveBeenCalledWith('a1'))
    // map 命中项置 read，未命中项原样保留
    await waitFor(() => expect(document.querySelector('.ant-badge-count')?.textContent).toBe('1'))
  })

  it('「全部已读」标记所有；「清空」清列表', async () => {
    // 仅首次拉取有数据；清空后重新展开回落到 beforeEach 的空数据
    apiMock.getAlerts.mockResolvedValueOnce({ data: [alert(), alert({ id: 'a2' })] })
    render(<NotificationDropdown />)
    await open()
    await screen.findAllByText('CPU 高')
    fireEvent.click(screen.getByText('全部已读'))
    await waitFor(() => expect(apiMock.markAllAlertsRead).toHaveBeenCalledTimes(1))
    fireEvent.click(screen.getByText('清空'))
    // 点菜单项 Dropdown 收起，重新展开验证空态
    await open()
    expect(await screen.findByText('暂无通知')).toBeInTheDocument()
  })

  it('拉取失败静默显示空态', async () => {
    apiMock.getAlerts.mockRejectedValue(new Error('down'))
    render(<NotificationDropdown />)
    await open()
    expect(await screen.findByText('暂无通知')).toBeInTheDocument()
  })

  it('空列表时无「全部已读/清空」操作', async () => {
    render(<NotificationDropdown />)
    await open()
    await screen.findByText('暂无通知')
    expect(screen.queryByText('全部已读')).toBeNull()
    expect(screen.queryByText('清空')).toBeNull()
  })

  it('success/info 类型走默认图标与 tag 配色分支', async () => {
    apiMock.getAlerts.mockResolvedValue({ data: [
      alert({ id: 's1', type: 'success', title: '备份完成' }),
      alert({ id: 'i1', type: 'info', title: '提示一条' }),
    ] })
    render(<NotificationDropdown />)
    await open()
    expect(await screen.findByText('备份完成')).toBeInTheDocument()
    expect(screen.getByText('提示一条')).toBeInTheDocument()
    expect(screen.getByText('success')).toBeInTheDocument()
    expect(screen.getByText('info')).toBeInTheDocument()
  })

  it('标记已读/全部已读失败静默', async () => {
    apiMock.getAlerts.mockResolvedValue({ data: [alert(), alert({ id: 'a2' })] })
    apiMock.markAlertRead.mockRejectedValue(new Error('mark down'))
    apiMock.markAllAlertsRead.mockRejectedValue(new Error('mark all down'))
    render(<NotificationDropdown />)
    await open()
    const item = (await screen.findAllByText('CPU 高'))[0].closest('.ant-list-item')!
    fireEvent.click(item)
    await waitFor(() => expect(apiMock.markAlertRead).toHaveBeenCalledWith('a1'))
    fireEvent.click(screen.getByText('全部已读'))
    await waitFor(() => expect(apiMock.markAllAlertsRead).toHaveBeenCalledTimes(1))
    // 失败后列表仍在（未误清空）
    expect(screen.getAllByText('CPU 高').length).toBeGreaterThan(0)
  })

  it('getAlerts 无 data 字段回落空列表；拉取中显示 Spin', async () => {
    let resolveAlerts: (v: unknown) => void = () => {}
    apiMock.getAlerts.mockImplementation(() => new Promise((r) => { resolveAlerts = r }))
    render(<NotificationDropdown />)
    fireEvent.click(document.querySelector('.ant-dropdown-trigger')!)
    await screen.findByText('通知中心')
    expect(document.querySelector('.ant-spin')).not.toBeNull()
    resolveAlerts({})
    expect(await screen.findByText('暂无通知')).toBeInTheDocument()
  })
})
