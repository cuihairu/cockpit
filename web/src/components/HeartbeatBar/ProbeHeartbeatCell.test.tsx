import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ProbeHeartbeatCell } from './ProbeHeartbeatCell'

// ProbeHeartbeatCell：react-query 拉取探测历史并渲染心跳条

const apiMock = vi.hoisted(() => ({ getProbeHistory: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const wrap = (ui: React.ReactNode) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
}

describe('ProbeHeartbeatCell', () => {
  beforeEach(() => {
    apiMock.getProbeHistory.mockReset()
  })

  it('加载中显示 …，完成后渲染心跳格', async () => {
    apiMock.getProbeHistory.mockResolvedValue({
      results: [
        { id: '1', resourceType: 'site', resourceId: 'r', name: 'n', status: 'up', latencyMs: 5, message: '', checkedAt: '2026-09-20T10:00:00Z' },
      ],
    })
    const { container } = render(wrap(
      <ProbeHeartbeatCell resourceType="site" resourceId="r" />,
    ))
    expect(container.textContent).toBe('…')
    await waitFor(() => {
      expect(container.querySelector('div[style*="border-radius"]')).not.toBeNull()
    })
    expect(apiMock.getProbeHistory).toHaveBeenCalledWith('site', 'r', 30)
  })

  it('请求失败静默显示「暂无记录」', async () => {
    apiMock.getProbeHistory.mockRejectedValue(new Error('down'))
    render(wrap(<ProbeHeartbeatCell resourceType="site" resourceId="r" limit={10} />))
    await waitFor(() => {
      expect(screen.getByText('暂无记录')).toBeInTheDocument()
    })
    expect(apiMock.getProbeHistory).toHaveBeenCalledWith('site', 'r', 10)
  })
})
