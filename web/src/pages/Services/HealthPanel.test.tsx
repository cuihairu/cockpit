import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import HealthPanel from './HealthPanel'
import type { HealthOverview } from '@/types'

// HealthPanel：状态徽标与自愈 Tag / 离线灰态提示 / 未配置空态 / 立即探测 /
// 编辑抽屉保存（白名单与探针归一化）/ 保存失败提示

const apiMock = vi.hoisted(() => ({
  getAgentHealth: vi.fn(),
  saveAgentHealth: vi.fn(),
  checkAgentProbe: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const msgSuccess = vi.spyOn(message, 'success')
const msgWarning = vi.spyOn(message, 'warning')
const msgError = vi.spyOn(message, 'error')

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => canWrite }))

const overview = (over?: Partial<HealthOverview>): HealthOverview => ({
  config: {
    probes: [
      {
        id: 'cloudflared-ready',
        type: 'http',
        target: 'http://127.0.0.1:20241/ready',
        expectStatus: 200,
        intervalSec: 30,
        heal: true,
        unit: 'cloudflared.service',
      },
      { id: 'sshd-active', type: 'systemd', target: 'ssh.service' },
    ],
    whitelist: ['cloudflared.service'],
    updatedAt: 1759000000,
    updatedBy: 'admin',
  },
  states: {
    'cloudflared-ready': {
      status: 'ok',
      lastCheck: 1759000100,
      consecutiveFails: 0,
      lastHeal: { time: 1758990000, unit: 'cloudflared.service', result: 'restarted', detail: 'ok', durationMs: 1200 },
    },
    'sshd-active': { status: 'fail', lastCheck: 1759000100, consecutiveFails: 3, lastError: 'systemctl is-active: failed' },
  },
  online: true,
  ...over,
})

const renderPanel = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const res = render(
    <QueryClientProvider client={qc}>
      <HealthPanel agentId="ag-1" />
    </QueryClientProvider>,
  )
  return res
}

beforeEach(() => {
  vi.clearAllMocks()
  canWrite = true
  apiMock.getAgentHealth.mockResolvedValue(overview())
})

describe('HealthPanel', () => {
  it('渲染探针行：状态徽标/连续失败/最近自愈 Tag/白名单摘要', async () => {
    renderPanel()
    await screen.findByText('cloudflared-ready')
    expect(screen.getByText('sshd-active')).toBeInTheDocument()
    expect(screen.getByText('正常')).toBeInTheDocument()
    expect(screen.getByText('失败')).toBeInTheDocument()
    expect(screen.getByText('连续 3 次')).toBeInTheDocument()
    expect(screen.getByText(/已自愈/)).toBeInTheDocument()
    expect(screen.getByText(/自愈目标白名单：cloudflared.service/)).toBeInTheDocument()
    expect(screen.getByText(/by admin/)).toBeInTheDocument()
  })

  it('主机离线：提示灰态快照且状态仍可见', async () => {
    apiMock.getAgentHealth.mockResolvedValue(overview({ online: false }))
    renderPanel()
    expect(await screen.findByText(/主机离线：以下为最近一次归集的状态快照/)).toBeInTheDocument()
    await screen.findByText('cloudflared-ready')
  })

  it('未配置探针：空态保留编辑入口（有写权限时即是配置入口）', async () => {
    apiMock.getAgentHealth.mockResolvedValue({ config: null, states: {}, online: true })
    renderPanel()
    expect(await screen.findByText('该主机尚未配置健康探针')).toBeInTheDocument()
    expect(screen.getByText(/点右上角「编辑探针」添加/)).toBeInTheDocument()
    expect(screen.getByText('编辑探针')).toBeInTheDocument()
  })

  it('无写权限：不渲染编辑与探测按钮', async () => {
    canWrite = false
    renderPanel()
    await screen.findByText('cloudflared-ready')
    expect(screen.queryByText('编辑探针')).not.toBeInTheDocument()
    expect(screen.queryByText('探测')).not.toBeInTheDocument()
  })

  it('立即探测：成功提示正常并刷新（行序与配置一致，第二行是 sshd-active）', async () => {
    apiMock.checkAgentProbe.mockResolvedValue({ probe: 'sshd-active', status: 'ok', consecutiveFails: 0 })
    renderPanel()
    await screen.findByText('sshd-active')
    fireEvent.click(screen.getAllByText('探测')[1])
    await waitFor(() => expect(apiMock.checkAgentProbe).toHaveBeenCalledWith('ag-1', 'sshd-active'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('探针 sshd-active 本次探测正常'))
  })

  it('立即探测失败：警告提示带 lastError', async () => {
    apiMock.checkAgentProbe.mockResolvedValue({ probe: 'cloudflared-ready', status: 'fail', lastError: 'HTTP 502' })
    renderPanel()
    await screen.findByText('cloudflared-ready')
    fireEvent.click(screen.getAllByText('探测')[0])
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('探针 cloudflared-ready 本次探测失败：HTTP 502'))
  })

  it('编辑抽屉：保存调用 saveAgentHealth（推送成功文案）', async () => {
    apiMock.saveAgentHealth.mockResolvedValue({ applied: 2, pushed: true })
    renderPanel()
    await screen.findByText('cloudflared-ready')
    fireEvent.click(screen.getByText('编辑探针'))
    const drawer = await screen.findByText('编辑健康探针')
    expect(drawer).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(apiMock.saveAgentHealth).toHaveBeenCalled())
    const body = apiMock.saveAgentHealth.mock.calls[0][1]
    expect(body.probes).toHaveLength(2)
    expect(body.probes[0].id).toBe('cloudflared-ready')
    expect(body.whitelist).toEqual(['cloudflared.service'])
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已保存并下发（2 条探针）'))
  })

  it('保存离线：pushed=false 提示补推文案', async () => {
    apiMock.saveAgentHealth.mockResolvedValue({ applied: 2, pushed: false })
    renderPanel()
    await screen.findByText('cloudflared-ready')
    fireEvent.click(screen.getByText('编辑探针'))
    await screen.findByText('编辑健康探针')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已保存（2 条探针）；agent 离线，上线后自动补推'))
  })

  it('保存失败：错误提示透出 server 校验文案', async () => {
    apiMock.saveAgentHealth.mockRejectedValue(new Error('heal unit "rogue.service" is not in the self-heal whitelist'))
    renderPanel()
    await screen.findByText('cloudflared-ready')
    fireEvent.click(screen.getByText('编辑探针'))
    await screen.findByText('编辑健康探针')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(msgError).toHaveBeenCalled())
  })

  it('立即探测请求失败：错误提示透出响应文案（agent 不在线等 transport 错误）', async () => {
    apiMock.checkAgentProbe.mockRejectedValue({ response: { data: { error: 'agent offline' } } })
    renderPanel()
    await screen.findByText('cloudflared-ready')
    fireEvent.click(screen.getAllByText('探测')[0])
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('agent offline'))
  })

  it('校验失败：必填 id 清空后点保存不发请求，字段上标错', async () => {
    renderPanel()
    await screen.findByText('cloudflared-ready')
    fireEvent.click(screen.getByText('编辑探针'))
    const idInputs = await screen.findAllByPlaceholderText('id（如 cloudflared-ready）')
    expect(idInputs).toHaveLength(2)
    fireEvent.change(idInputs[0], { target: { value: '' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    // antd 字段级标错出现，且未触达保存 API
    expect(await screen.findByText('必填')).toBeInTheDocument()
    expect(apiMock.saveAgentHealth).not.toHaveBeenCalled()
  })

  it('编辑抽屉：取消按钮直接关闭不保存', async () => {
    renderPanel()
    await screen.findByText('cloudflared-ready')
    fireEvent.click(screen.getByText('编辑探针'))
    await screen.findByText('编辑健康探针')
    fireEvent.click(screen.getByRole('button', { name: '取 消' }))
    // Drawer 未设 destroyOnClose：关闭后内容 DOM 保留，可靠信号是
    // wrapper 落 hidden class + mask 卸载（探针实验实证）
    await waitFor(() =>
      expect(document.querySelector('.ant-drawer-content-wrapper')).toHaveClass(
        'ant-drawer-content-wrapper-hidden',
      ),
    )
    expect(document.querySelector('.ant-drawer-mask')).not.toBeInTheDocument()
    expect(apiMock.saveAgentHealth).not.toHaveBeenCalled()
  })

  it('编辑抽屉：添加探针行 / 删除探针行', async () => {
    renderPanel()
    await screen.findByText('cloudflared-ready')
    fireEvent.click(screen.getByText('编辑探针'))
    await screen.findByText('编辑健康探针')
    // 添加：默认 http 类型的新行
    fireEvent.click(screen.getByText('添加探针'))
    expect(screen.getAllByPlaceholderText('id（如 cloudflared-ready）')).toHaveLength(3)
    // 删除：移除第一行（cloudflared-ready）
    fireEvent.click(screen.getAllByText('删除')[0])
    expect(screen.getAllByPlaceholderText('id（如 cloudflared-ready）')).toHaveLength(2)
    expect((screen.getAllByPlaceholderText('id（如 cloudflared-ready）')[0] as HTMLInputElement).value).toBe('sshd-active')
  })

  it('编辑抽屉：切换探针类型清空 target（换类型后旧目标无效）', async () => {
    renderPanel()
    await screen.findByText('cloudflared-ready')
    fireEvent.click(screen.getByText('编辑探针'))
    await screen.findByText('编辑健康探针')
    // 第一行是 http 探针；其 target 输入框初始带值
    const targetInput = screen.getAllByDisplayValue('http://127.0.0.1:20241/ready')[0] as HTMLInputElement
    // 「HTTP 状态码」文本在主表格 Tag 与抽屉 Select 各有一份，取抽屉内
    // 第一个 Select（第一行的 type）；rc-select 监听在 .ant-select-selector
    // 上（父容器事件不冒泡给子），mouseDown 须打 selector
    const selector = document.querySelector('.ant-drawer .ant-select .ant-select-selector')!
    fireEvent.mouseDown(selector)
    const opt = await screen.findByText('TCP 端口', { selector: '.ant-select-item-option-content' })
    fireEvent.click(opt)
    await waitFor(() => expect(targetInput.value).toBe(''))
  })
})
