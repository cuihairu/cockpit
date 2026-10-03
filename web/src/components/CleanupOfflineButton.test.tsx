import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import CleanupOfflineButton from './CleanupOfflineButton'

// 清理离线按钮：二次确认 + 阈值三档（默认 3 天）+ 清理数量回报；
// 取消不发起请求；失败 message.error 且弹窗保留

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

describe('CleanupOfflineButton', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMock.cleanupAgents.mockResolvedValue({ status: 'ok', removed: [], count: 0 })
  })

  it('默认 72h 档：确认后按阈值调用、回报数量并触发刷新、弹窗关闭', async () => {
    apiMock.cleanupAgents.mockResolvedValue({ status: 'ok', removed: ['a', 'b'], count: 2 })
    renderButton()

    fireEvent.click(screen.getByRole('button', { name: /清理离线/ }))
    expect(screen.getByText('一键清理离线 Agent')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /确认清理/ }))
    await waitFor(() => expect(apiMock.cleanupAgents).toHaveBeenCalledWith({ thresholdHours: 72 }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已清理 2 台离线 Agent'))
    expect(onCleaned).toHaveBeenCalled()
    // 成功后弹窗关闭（jsdom 无过渡终点事件，按离场态断言）
    await waitFor(() =>
      expect(document.querySelector('.ant-modal')?.className).toContain('ant-zoom-leave'))
  })

  it('切换 24 小时档后按新档位调用', async () => {
    renderButton()
    fireEvent.click(screen.getByRole('button', { name: /清理离线/ }))
    fireEvent.click(screen.getByRole('radio', { name: /离线超过 24 小时/ }))
    fireEvent.click(screen.getByRole('button', { name: /确认清理/ }))
    await waitFor(() => expect(apiMock.cleanupAgents).toHaveBeenCalledWith({ thresholdHours: 24 }))
  })

  it('取消关闭弹窗且不发起请求', async () => {
    renderButton()
    fireEvent.click(screen.getByRole('button', { name: /清理离线/ }))
    // 默认 locale 下取消按钮文案为 Cancel，按 footer 非 primary 按钮定位（同 DNS 用例）
    fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn:not(.ant-btn-primary)')!)
    await waitFor(() =>
      expect(document.querySelector('.ant-modal')?.className).toContain('ant-zoom-leave'))
    expect(apiMock.cleanupAgents).not.toHaveBeenCalled()
  })

  it('失败路径 message.error，弹窗保留待重试', async () => {
    apiMock.cleanupAgents.mockRejectedValue(new Error('boom'))
    renderButton()
    fireEvent.click(screen.getByRole('button', { name: /清理离线/ }))
    fireEvent.click(screen.getByRole('button', { name: /确认清理/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalled())
    expect(screen.getByText('一键清理离线 Agent')).toBeInTheDocument()
    expect(onCleaned).not.toHaveBeenCalled()
  })
})
