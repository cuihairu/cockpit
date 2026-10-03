import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ServicesPage from './ServicesPage'

const apiMock = vi.hoisted(() => ({
  getServices: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const settingsRef = { current: { showResourceCount: true, refreshInterval: 3600 } }
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsRef.current }),
}))

const renderPage = () => {
  apiMock.getServices.mockResolvedValue({ data: [] })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <ServicesPage />
    </QueryClientProvider>,
  )
}

describe('ServicesPage', () => {
  beforeEach(() => vi.clearAllMocks())
  it('渲染服务列表', async () => {
    renderPage()
    expect(await screen.findByText('服务')).toBeInTheDocument()
  })
})
