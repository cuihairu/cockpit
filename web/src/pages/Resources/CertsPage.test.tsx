import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import CertsPage from './CertsPage'

const apiMock = vi.hoisted(() => ({
  getCertificates: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const settingsRef = { current: { showResourceCount: true, refreshInterval: 3600 } }
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsRef.current }),
}))

const renderPage = () => {
  apiMock.getCertificates.mockResolvedValue({ data: [] })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <CertsPage />
    </QueryClientProvider>,
  )
}

describe('CertsPage', () => {
  beforeEach(() => vi.clearAllMocks())
  it('渲染证书列表', async () => {
    renderPage()
    expect(await screen.findByText('证书')).toBeInTheDocument()
  })
})
