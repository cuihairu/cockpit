import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import StoragesPage from './StoragesPage'

const apiMock = vi.hoisted(() => ({
  getStorages: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const settingsRef = { current: { showResourceCount: true, refreshInterval: 3600 } }
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsRef.current }),
}))

const renderPage = () => {
  apiMock.getStorages.mockResolvedValue({ data: [] })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <StoragesPage />
    </QueryClientProvider>,
  )
}

describe('StoragesPage', () => {
  beforeEach(() => vi.clearAllMocks())
  it('渲染存储列表', async () => {
    renderPage()
    expect(await screen.findByText('存储')).toBeInTheDocument()
  })
})
