import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import DomainsPage from './DomainsPage'
import type { Domain } from '@/types'

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

const renderPage = () => {
  apiMock.getComputeInstances.mockResolvedValue({ data: [] })
  apiMock.getDomains.mockResolvedValue({ data: [] })
  apiMock.getCertificates.mockResolvedValue({ data: [] })
  apiMock.getServices.mockResolvedValue({ data: [] })
  apiMock.getGateways.mockResolvedValue({ data: [] })
  apiMock.getStorages.mockResolvedValue({ data: [] })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <DomainsPage />
    </QueryClientProvider>,
  )
}

const domains = [
  { id: 'd1', domain: 'example.com', status: 'active', provider: 'cloudflare', autoRenew: true, expiresAt: '2026-06-01' },
] as unknown as Domain[]

const renderPageWithData = () => {
  apiMock.getComputeInstances.mockResolvedValue({ data: [] })
  apiMock.getDomains.mockResolvedValue({ data: domains })
  apiMock.getCertificates.mockResolvedValue({ data: [] })
  apiMock.getServices.mockResolvedValue({ data: [] })
  apiMock.getGateways.mockResolvedValue({ data: [] })
  apiMock.getStorages.mockResolvedValue({ data: [] })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <DomainsPage />
    </QueryClientProvider>,
  )
}

describe('DomainsPage', () => {
  beforeEach(() => vi.clearAllMocks())
  it('渲染域名列表', () => {
    renderPage()
    // Card title contains "域名" - multiple elements match, use getAllByText
    expect(screen.getAllByText('域名').length).toBeGreaterThan(0)
  })

  it('渲染域名列表带数据', async () => {
    renderPageWithData()
    expect(await screen.findByText('example.com')).toBeInTheDocument()
    expect(await screen.findByText('active')).toBeInTheDocument()
    expect(await screen.findByText('cloudflare')).toBeInTheDocument()
    expect(await screen.findByText('是')).toBeInTheDocument()
  })
})
