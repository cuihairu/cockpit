import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import CloudInstancesPage from './CloudInstancesPage'
import { api } from '@/services/api'

vi.mock('@/services/api', () => ({
  api: {
    getCloudAccounts: vi.fn(),
    getCloudInstances: vi.fn(),
    cloudInstanceAction: vi.fn(),
    cloudInstanceCreate: vi.fn(),
    getCloudOperation: vi.fn(),
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
  region: 'cn-hangzhou',
  scopes: ['ecs:read'],
  enabled: true,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}
const instance = {
  id: 'inst-1',
  accountId: 'cloud-1',
  nativeId: 'i-1',
  name: 'web-1',
  region: 'cn-hangzhou',
  spec: 'ecs.g6.large',
  status: 'running',
  billingMode: 'on-demand',
  billingContinues: false,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const permRef = { admin: true, write: true }
vi.mock('@/hooks/usePerm', () => ({ usePerm: (permission: string) => (permission === 'cloud:admin' ? permRef.admin : permRef.write) }))

const renderPage = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <CloudInstancesPage />
    </QueryClientProvider>,
  )
}

const selectAccount = async () => {
  fireEvent.mouseDown(screen.getByText('选择云账号'))
  fireEvent.click(await screen.findByText('aliyun/prod'))
  await waitFor(() => expect(apiMock.getCloudInstances).toHaveBeenCalledWith('cloud-1'))
}

describe('CloudInstancesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    permRef.admin = true
    permRef.write = true
    apiMock.getCloudAccounts.mockResolvedValue([account])
    apiMock.getCloudInstances.mockResolvedValue([instance])
    apiMock.getCloudOperation.mockResolvedValue({ id: 'op-1', kind: 'start', vendor_request_id: 'r1', status: 'succeeded', updated_at: '' })
    window.prompt = vi.fn(() => 'inst-1')
  })

  it('lists instances after selecting an account', async () => {
    renderPage()
    await selectAccount()
    expect(await screen.findByText('web-1')).toBeInTheDocument()
    expect(screen.getByText('运行中')).toBeInTheDocument()
  })

  it('performs start action with confirm', async () => {
    apiMock.cloudInstanceAction.mockResolvedValue({ id: 'op-1', kind: 'start', vendor_request_id: 'r1', status: 'pending', updated_at: '' })
    apiMock.getCloudInstances.mockResolvedValue([{ ...instance, status: 'stopped' }])
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /启\s*动/ }))
    await waitFor(() => expect(apiMock.cloudInstanceAction).toHaveBeenCalledWith('cloud-1', 'start', 'inst-1'))
    expect(message.success).toHaveBeenCalledWith('操作已受理')
  })

  it('rejects action on prompt mismatch', async () => {
    window.prompt = vi.fn(() => 'wrong')
    apiMock.getCloudInstances.mockResolvedValue([{ ...instance, status: 'stopped' }])
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /启\s*动/ }))
    await waitFor(() => expect(message.error).toHaveBeenCalledWith('确认文本不一致'))
    expect(apiMock.cloudInstanceAction).not.toHaveBeenCalled()
  })

  it('cancels action on null prompt', async () => {
    window.prompt = vi.fn(() => null)
    apiMock.getCloudInstances.mockResolvedValue([{ ...instance, status: 'stopped' }])
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /启\s*动/ }))
    expect(apiMock.cloudInstanceAction).not.toHaveBeenCalled()
  })

  it('performs stop and delete actions', async () => {
    apiMock.cloudInstanceAction.mockResolvedValue({ id: 'op-1', kind: 'stop', vendor_request_id: 'r1', status: 'pending', updated_at: '' })
    apiMock.getCloudInstances.mockResolvedValue([{ ...instance, status: 'running' }])
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /停\s*止/ }))
    await waitFor(() => expect(apiMock.cloudInstanceAction).toHaveBeenCalledWith('cloud-1', 'stop', 'inst-1'))
    fireEvent.click(screen.getByRole('button', { name: /释\s*放/ }))
    await waitFor(() => expect(apiMock.cloudInstanceAction).toHaveBeenCalledWith('cloud-1', 'delete', 'inst-1'))
  })

  it('shows error on action failure', async () => {
    apiMock.cloudInstanceAction.mockRejectedValue(new Error('boom'))
    apiMock.getCloudInstances.mockResolvedValue([{ ...instance, status: 'stopped' }])
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /启\s*动/ }))
    await waitFor(() => expect(message.error).toHaveBeenCalled())
  })

  it('creates an instance', async () => {
    apiMock.cloudInstanceCreate.mockResolvedValue({ id: 'op-2', kind: 'create', vendor_request_id: 'r2', status: 'pending', updated_at: '' })
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /创\s*建/ }))
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'web-2' } })
    fireEvent.change(screen.getByLabelText('规格'), { target: { value: 'ecs.g6.large' } })
    fireEvent.change(screen.getByLabelText('镜像 ID'), { target: { value: 'ami-1' } })
    fireEvent.change(screen.getByLabelText('安全组 ID'), { target: { value: 'sg-1' } })
    fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.cloudInstanceCreate).toHaveBeenCalledWith('cloud-1', expect.objectContaining({ name: 'web-2', spec: 'ecs.g6.large', image_id: 'ami-1' })))
    expect(message.success).toHaveBeenCalledWith('创建已受理')
  })

  it('shows error on create failure', async () => {
    apiMock.cloudInstanceCreate.mockRejectedValue(new Error('boom'))
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /创\s*建/ }))
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'web-2' } })
    fireEvent.change(screen.getByLabelText('规格'), { target: { value: 'ecs.g6.large' } })
    fireEvent.change(screen.getByLabelText('镜像 ID'), { target: { value: 'ami-1' } })
    fireEvent.change(screen.getByLabelText('安全组 ID'), { target: { value: 'sg-1' } })
    fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn-primary')!)
    await waitFor(() => expect(message.error).toHaveBeenCalled())
  })

  it('renders other status tags and refresh', async () => {
    apiMock.getCloudInstances.mockResolvedValue([
      { ...instance, id: 'inst-2', status: 'creating' },
      { ...instance, id: 'inst-3', status: 'deleting' },
      { ...instance, id: 'inst-4', status: 'unknown', billingContinues: true },
    ])
    renderPage()
    await selectAccount()
    expect(await screen.findByText('创建中')).toBeInTheDocument()
    expect(screen.getByText('释放中')).toBeInTheDocument()
    expect(screen.getByText('未知')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /刷\s*新/ }))
    await waitFor(() => expect(apiMock.getCloudInstances.mock.calls.length).toBeGreaterThanOrEqual(2))
  })

  it('create button disabled without account', async () => {
    renderPage()
    expect(await screen.findByText('选择云账号')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /创\s*建/ })).toBeDisabled()
  })

  // pollOperation 轮询（审核发现 :272）：action 受理后 setInterval 3000ms 轮询
  // getCloudOperation 到终态。真实推进定时器（fake timers 会与 waitFor 的
  // interval 轮询冲突，见 web-v8-branch-lock-and-artifact 注记）。
  it('polls operation to terminal state and notifies', async () => {
    apiMock.cloudInstanceAction.mockResolvedValue({ id: 'op-1', kind: 'start', vendor_request_id: 'r1', status: 'pending', updated_at: '' })
    apiMock.getCloudOperation.mockResolvedValue({ id: 'op-1', kind: 'start', vendor_request_id: 'r1', status: 'succeeded', updated_at: '' })
    apiMock.getCloudInstances.mockResolvedValue([{ ...instance, status: 'stopped' }])
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /启\s*动/ }))
    await waitFor(() => expect(apiMock.cloudInstanceAction).toHaveBeenCalledWith('cloud-1', 'start', 'inst-1'))
    expect(message.success).toHaveBeenCalledWith('操作已受理')
    await new Promise((r) => setTimeout(r, 3400))
    await waitFor(() => expect(apiMock.getCloudOperation).toHaveBeenCalledWith('cloud-1', 'op-1'))
    expect(message.success).toHaveBeenCalledWith('操作已完成')
  }, 20000)

  it('keeps polling when an operation poll transiently fails', async () => {
    apiMock.cloudInstanceAction.mockResolvedValue({ id: 'op-1', kind: 'start', vendor_request_id: 'r1', status: 'pending', updated_at: '' })
    apiMock.getCloudOperation.mockRejectedValue(new Error('net'))
    apiMock.getCloudInstances.mockResolvedValue([{ ...instance, status: 'stopped' }])
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /启\s*动/ }))
    await waitFor(() => expect(apiMock.cloudInstanceAction).toHaveBeenCalledWith('cloud-1', 'start', 'inst-1'))
    await new Promise((r) => setTimeout(r, 3400))
    await waitFor(() => expect(apiMock.getCloudOperation).toHaveBeenCalledWith('cloud-1', 'op-1'))
    // 轮询出错不打断、无终态提示；下一轮重试直到终态
    expect(message.success).not.toHaveBeenCalledWith('操作已完成')
  }, 20000)

  it('polls operation to failed terminal state and notifies failure', async () => {
    apiMock.cloudInstanceAction.mockResolvedValue({ id: 'op-1', kind: 'start', vendor_request_id: 'r1', status: 'pending', updated_at: '' })
    apiMock.getCloudOperation.mockResolvedValue({ id: 'op-1', kind: 'start', vendor_request_id: 'r1', status: 'failed', updated_at: '' })
    apiMock.getCloudInstances.mockResolvedValue([{ ...instance, status: 'stopped' }])
    renderPage()
    await selectAccount()
    await screen.findByText('web-1')
    fireEvent.click(screen.getByRole('button', { name: /启\s*动/ }))
    await waitFor(() => expect(apiMock.cloudInstanceAction).toHaveBeenCalledWith('cloud-1', 'start', 'inst-1'))
    await new Promise((r) => setTimeout(r, 3400))
    await waitFor(() => expect(apiMock.getCloudOperation).toHaveBeenCalledWith('cloud-1', 'op-1'))
    expect(message.success).toHaveBeenCalledWith('操作失败')
  }, 20000)

})
