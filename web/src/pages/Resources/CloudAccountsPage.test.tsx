import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import CloudAccountsPage from './CloudAccountsPage'
import { api } from '@/services/api'

vi.mock('@/services/api', () => ({
  api: {
    getCloudAccounts: vi.fn(),
    saveCloudAccount: vi.fn(),
    deleteCloudAccount: vi.fn(),
  },
}))

vi.spyOn(message, 'success')
vi.spyOn(message, 'error')

const apiMock = vi.mocked(api, true)
const account = {
  id: 'cloud-1',
  vendor: 'aliyun',
  alias: 'prod',
  credentialRef: '${CLOUD_ALIYUN_PROD}',
  scopes: ['ecs:read'],
  enabled: true,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const permRef = { admin: true, write: true }
vi.mock('@/hooks/usePerm', () => ({ usePerm: (permission: string) => (permission === 'cloud:admin' ? permRef.admin : permRef.write) }))
const renderPage = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <CloudAccountsPage />
    </QueryClientProvider>,
  )
}

const submitModal = async () => {
  fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn-primary')!)
}

describe('CloudAccountsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    permRef.admin = true
    permRef.write = true
    apiMock.getCloudAccounts.mockResolvedValue([account])
    window.prompt = vi.fn(() => 'cloud-1')
  })

  it('renders credential references without secrets and refreshes', async () => {
    renderPage()
    expect(await screen.findByText('${CLOUD_ALIYUN_PROD}')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
    await waitFor(() => expect(apiMock.getCloudAccounts).toHaveBeenCalledTimes(2))
  })

  it('requires exact vendor/alias confirmation before save', async () => {
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: /添\s*加/ }))
    fireEvent.change(screen.getByLabelText('厂商'), { target: { value: 'aliyun' } })
    fireEvent.change(screen.getByLabelText('别名'), { target: { value: 'prod' } })
    fireEvent.change(screen.getByLabelText(/^Secret/), { target: { value: 'secret' } })
    fireEvent.change(screen.getByLabelText(/输入 vendor\/alias 确认/), { target: { value: 'wrong' } })
    await submitModal()
    await waitFor(() => expect(screen.getByText('确认文本不一致')).toBeInTheDocument())
    expect(apiMock.saveCloudAccount).not.toHaveBeenCalled()
    expect(await screen.findByText('确认文本不一致')).toBeInTheDocument()
  })

  it('submits only after exact confirmation', async () => {
    apiMock.saveCloudAccount.mockResolvedValue(account)
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: /添\s*加/ }))
    fireEvent.change(screen.getByLabelText('厂商'), { target: { value: 'aliyun' } })
    fireEvent.change(screen.getByLabelText('别名'), { target: { value: 'prod' } })
    fireEvent.change(screen.getByLabelText(/^Secret/), { target: { value: 'secret' } })
    fireEvent.change(screen.getByLabelText(/输入 vendor\/alias 确认/), { target: { value: 'aliyun/prod' } })
    await submitModal()
    await waitFor(() => expect(apiMock.saveCloudAccount).toHaveBeenCalled())
    expect(apiMock.saveCloudAccount).toHaveBeenCalledWith(
      expect.objectContaining({ vendor: 'aliyun', alias: 'prod', secret: 'secret', confirmKey: 'aliyun/prod' }),
      expect.objectContaining({ client: expect.anything() }),
    )
  })

  it('requires the exact account id for delete', async () => {
    apiMock.deleteCloudAccount.mockResolvedValue(undefined)
    renderPage()
    fireEvent.click((await screen.findAllByRole('button', { name: /删\s*除/ }))[0])
    await waitFor(() => expect(screen.getByText('输入账号 ID 二次确认')).toBeInTheDocument())
    fireEvent.click(await screen.findByRole('button', { name: '确 认' }))
    await waitFor(() => expect(apiMock.deleteCloudAccount).toHaveBeenCalled())
    expect(apiMock.deleteCloudAccount).toHaveBeenCalledWith('cloud-1', expect.objectContaining({ client: expect.anything() }))
    expect(message.success).toHaveBeenCalledWith('云账号已删除')
  })

  it('hides the add action without cloud:write', async () => {
    permRef.write = false
    permRef.admin = false
    renderPage()
    expect(await screen.findByText('aliyun')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /添\s*加/ })).not.toBeInTheDocument()
  })
})
