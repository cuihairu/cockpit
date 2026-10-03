import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import GatewaysPage from './GatewaysPage'

const apiMock = vi.hoisted(() => ({
  getGateways: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const settingsRef = { current: { showResourceCount: true, refreshInterval: 3600 } }
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsRef.current }),
}))

const renderPage = () => {
  apiMock.getGateways.mockResolvedValue({ data: [] })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <GatewaysPage />
    </QueryClientProvider>,
  )
}

describe('GatewaysPage', () => {
  beforeEach(() => vi.clearAllMocks())
  it('渲染网关列表', async () => {
    renderPage()
    expect(await screen.findByText('网关')).toBeInTheDocument()
  })
})
