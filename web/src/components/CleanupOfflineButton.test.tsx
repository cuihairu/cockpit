import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import CleanupOfflineButton from './CleanupOfflineButton'

// 清理离线 agent：阈值自由填（30m/2h/7d）+ 留空默认全清 + 口径旁注；
// 格式无效禁用确认；取消不发起请求；失败 message.error 且弹窗保留

const apiMock = vi.hoisted(() => ({ cleanupAgents: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')

const onCleaned = vi.fn()

const renderButton = () => {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <CleanupOfflineButton onCleaned={onCleaned} />
    </QueryClientProvider>,
  )
}

const openModal = () => {
  fireEvent.click(screen.getByRole('button', { name: /清理离线 agent/ }))
  expect(screen.getByText('一键清理离线 Agent')).toBeInTheDocument()
}

describe('CleanupOfflineButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.cleanupAgents.mockResolvedValue({ status: 'ok', removed: [], count: 0 })
  })

  it('留空 = 默认全清：确认后不带阈值调用、回报数量并触发刷新、弹窗关闭', async () => {
    apiMock.cleanupAgents.mockResolvedValue({ status: 'ok', removed: ['a', 'b'], count: 2 })
    renderButton()
    // 按钮旁注默认「全部离线」
    expect(screen.getByText('全部离线')).toBeInTheDocument()

    openModal()
    expect(screen.getByText('当前口径：全部离线')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /确认清理/ }))
    await waitFor(() => expect(apiMock.cleanupAgents).toHaveBeenCalledWith({}))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已清理 2 台离线 Agent'))
    expect(onCleaned).toHaveBeenCalled()
    // 成功后弹窗关闭（jsdom 无过渡终点事件，按离场态断言）
    await waitFor(() =>
      expect(document.querySelector('.ant-modal')?.className).toContain('ant-zoom-leave'))
  })

  it('自由填 2h：换算 120 分钟调用，旁注与弹窗口径同步为「离线超过 2 小时」', async () => {
    renderButton()
    openModal()
    fireEvent.change(screen.getByPlaceholderText(/留空 = 全部离线/), { target: { value: '2h' } })
    expect(screen.getByText('当前口径：离线超过 2 小时')).toBeInTheDocument()
    // 旁注在弹窗外实时同步
    expect(screen.getByText('离线超过 2 小时')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /确认清理/ }))
    await waitFor(() => expect(apiMock.cleanupAgents).toHaveBeenCalledWith({ thresholdMinutes: 120 }))
  })

  it('自由填 7d + 回车确认：换算 10080 分钟调用', async () => {
    renderButton()
    openModal()
    const input = screen.getByPlaceholderText(/留空 = 全部离线/)
    fireEvent.change(input, { target: { value: '7d' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    await waitFor(() => expect(apiMock.cleanupAgents).toHaveBeenCalledWith({ thresholdMinutes: 10080 }))
  })

  it('格式无效：提示格式错误并禁用确认，不发起请求', async () => {
    renderButton()
    openModal()
    fireEvent.change(screen.getByPlaceholderText(/留空 = 全部离线/), { target: { value: '2w' } })
    expect(screen.getByText(/格式无效：支持 30m/)).toBeInTheDocument()
    expect(screen.getByText('口径无效')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /确认清理/ })).toBeDisabled()
    // 无效口径下回车也不触发
    fireEvent.keyDown(screen.getByPlaceholderText(/留空 = 全部离线/), { key: 'Enter' })
    expect(apiMock.cleanupAgents).not.toHaveBeenCalled()
  })

  it('取消关闭弹窗且不发起请求', async () => {
    renderButton()
    openModal()
    // 默认 locale 下取消按钮文案为 Cancel，按 footer 非 primary 按钮定位（同 DNS 用例）
    fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn:not(.ant-btn-primary)')!)
    await waitFor(() =>
      expect(document.querySelector('.ant-modal')?.className).toContain('ant-zoom-leave'))
    expect(apiMock.cleanupAgents).not.toHaveBeenCalled()
  })

  it('失败路径 message.error，弹窗保留待重试', async () => {
    apiMock.cleanupAgents.mockRejectedValue(new Error('boom'))
    renderButton()
    openModal()
    fireEvent.click(screen.getByRole('button', { name: /确认清理/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalled())
    expect(screen.getByText('一键清理离线 Agent')).toBeInTheDocument()
    expect(onCleaned).not.toHaveBeenCalled()
  })
})
