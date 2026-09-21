import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
// 本文件测 Modal.confirm 静态弹窗：React 19 下需 patch 才会真实渲染（与 main.tsx 一致）
import '@ant-design/v5-patch-for-react-19'
import { message, Modal } from 'antd'
import Docker from './index'
import type { Agent } from '@/types'

// Docker：docker agent 过滤与空态 / 容器状态分档操作集与 RBAC 裁剪 /
// 危险操作二次确认 / 镜像表格式化 / 日志弹窗（tail 切换、控制头剥离）

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getContainers: vi.fn(),
  getImages: vi.fn(),
  getContainerLogs: vi.fn(),
  startContainer: vi.fn(),
  stopContainer: vi.fn(),
  restartContainer: vi.fn(),
  pauseContainer: vi.fn(),
  unpauseContainer: vi.fn(),
  removeContainer: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => canWrite }))

const msgError = vi.spyOn(message, 'error')

const mkAgent = (id: string, hostname: string, caps: string[] = ['docker-api']): Agent =>
  ({
    id,
    hostname,
    ip: '10.0.0.1',
    location: { region: 'cn', zone: 'z1' },
    status: 'online',
    lastSeen: '0',
    capabilities: caps.map((type) => ({ type })),
  }) as unknown as Agent

const agents = [
  mkAgent('ag-docker', 'web-01'),
  mkAgent('ag-plain', 'plain', ['files']),
  { ...mkAgent('ag-off', 'off-01'), status: 'offline' },
]

const containers = [
  { ID: 'c-run', Name: '/nginx', Image: 'nginx:latest', State: 'running', Status: 'Up 2 hours', Created: 1760000000 },
  { ID: 'c-exit', Name: '/redis', Image: 'redis:7', State: 'exited', Status: 'Exited (0) 1h ago', Created: 1759000000 },
  { ID: 'c-pause', Name: '/paused', Image: 'busybox', State: 'paused', Status: 'Paused', Created: 1758000000 },
]

const images = [
  { RepoTags: ['nginx:latest', 'nginx:alpine'], ID: 'sha256:abcdef1234567890abcdef', Size: 64 * 1024 * 1024, Created: 1760000000 },
  { RepoTags: [], ID: 'sha256:fedcba0987654321fedcba', Size: 0, Created: 0 },
]

const renderPage = (agentsOverride?: Agent[]) => {
  apiMock.getAgents.mockResolvedValue(agentsOverride ?? agents)
  apiMock.getContainers.mockResolvedValue(containers)
  apiMock.getImages.mockResolvedValue(images)
  apiMock.getContainerLogs.mockResolvedValue('\x01\x00\x00\x00\x00\x00\x00#2026-01-01 line1\n\x02raw line2\n')
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Docker />
    </QueryClientProvider>,
  )
}

const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(cell)) as HTMLTableRowElement

const btnIn = (row: HTMLTableRowElement, label: string) =>
  Array.from(row.querySelectorAll('button')).find((b) =>
    new RegExp(label.split('').join('\\s*')).test(b.textContent || '')) as HTMLButtonElement

describe('Docker', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    canWrite = true
  })

  // patch 后静态方法的 root 真实渲染：先 destroy 并排干调度再清 body，
  // 否则残留任务在 jsdom 销毁后触发 window is not defined
  afterEach(async () => {
    Modal.destroyAll()
    message.destroy()
    await new Promise((r) => setTimeout(r, 10))
  })

  it('无 docker agent：空态提示且不发容器查询', async () => {
    renderPage([mkAgent('ag-plain', 'plain', ['files'])])
    expect(await screen.findByText('暂无在线的 Docker Agent。请在目标主机部署 Agent 并确保可访问 /var/run/docker.sock', {}, { timeout: 5000 })).toBeInTheDocument()
    expect(apiMock.getContainers).not.toHaveBeenCalled()
  })

  it('默认选中首个 docker agent：容器表与状态分档操作集', async () => {
    renderPage()
    expect(await screen.findByText('nginx', {}, { timeout: 5000 })).toBeInTheDocument()
    expect(apiMock.getContainers).toHaveBeenCalledWith('ag-docker', true)
    expect(apiMock.getImages).toHaveBeenCalledWith('ag-docker')
    // 底部 agent 信息条
    expect(screen.getByText(/Agent：web-01/)).toBeInTheDocument()
    // running：日志/停止/重启/暂停；状态 Tag 与时间格式化
    const r1 = rowOf('nginx')
    expect(within(r1).getByText('running')).toBeInTheDocument()
    expect(btnIn(r1, '停止')).toBeTruthy()
    expect(btnIn(r1, '重启')).toBeTruthy()
    expect(btnIn(r1, '暂停')).toBeTruthy()
    // exited：启动/删除；paused：恢复；表计数
    const r2 = rowOf('redis')
    expect(btnIn(r2, '启动')).toBeTruthy()
    expect(btnIn(r2, '删除')).toBeTruthy()
    expect(btnIn(rowOf('paused'), '恢复')).toBeTruthy()
    expect(screen.getByText('容器 (3)')).toBeInTheDocument()
  })

  it('RBAC 无写权限：操作只剩日志', async () => {
    canWrite = false
    renderPage()
    expect(await screen.findByText('nginx', {}, { timeout: 5000 })).toBeInTheDocument()
    const r1 = rowOf('nginx')
    expect(btnIn(r1, '日志')).toBeTruthy()
    expect(btnIn(r1, '停止')).toBeFalsy()
    const r2 = rowOf('redis')
    expect(btnIn(r2, '启动')).toBeFalsy()
    expect(btnIn(r2, '删除')).toBeFalsy()
  })

  it('镜像 tab：Tag 与 sha256 截断、大小格式化', async () => {
    renderPage()
    await screen.findByText('nginx', {}, { timeout: 5000 })
    await act(async () => {
      fireEvent.click(screen.getByText(/镜像 \(2\)/))
    })
    expect(await screen.findByText('nginx:alpine', {}, { timeout: 5000 })).toBeInTheDocument()
    expect(screen.getByText('abcdef123456')).toBeInTheDocument()
    // >=10 取整；0/无时间戳显示 '-'
    expect(screen.getByText('64 MB')).toBeInTheDocument()
    expect(screen.getAllByText('-').length).toBeGreaterThanOrEqual(2)
  })

  it('日志弹窗：标题与 tail 切换、剥离流控制头', async () => {
    renderPage()
    await screen.findByText('nginx', {}, { timeout: 5000 })
    await act(async () => {
      fireEvent.click(btnIn(rowOf('nginx'), '日志'))
    })
    expect(await screen.findByText('容器日志 — nginx', {}, { timeout: 5000 })).toBeInTheDocument()
    expect(await screen.findByText(/2026-01-01 line1/, {}, { timeout: 5000 })).toBeInTheDocument()
    expect(await screen.findByText(/raw line2/, {}, { timeout: 5000 })).toBeInTheDocument()
    await waitFor(() => expect(apiMock.getContainerLogs).toHaveBeenCalledWith('ag-docker', 'c-run', { tail: '100', timestamps: true }))
    // 切 tail 500 → 重拉
    fireEvent.mouseDown(document.querySelector('.ant-modal .ant-select-selector')!)
    const opt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent === '500 行')
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(opt)
    await waitFor(() =>
      expect(apiMock.getContainerLogs).toHaveBeenCalledWith('ag-docker', 'c-run', { tail: '500', timestamps: true }))
  })

  it('危险操作走确认弹窗：停止确认后调 stopContainer', async () => {
    apiMock.stopContainer.mockResolvedValue({})
    renderPage()
    await screen.findByText('nginx', {}, { timeout: 5000 })
    await act(async () => {
      fireEvent.click(btnIn(rowOf('nginx'), '停止'))
    })
    expect(await screen.findByText('确定要停止「nginx」吗？', {}, { timeout: 5000 })).toBeInTheDocument()
    expect(apiMock.stopContainer).not.toHaveBeenCalled()
    await act(async () => {
      fireEvent.click(document.querySelector('.ant-modal-confirm-btns .ant-btn-primary')!)
    })
    await waitFor(() => expect(apiMock.stopContainer).toHaveBeenCalledWith('ag-docker', 'c-run', 10))
  })

  it('非危险操作直发：启动 exited 容器不弹确认', async () => {
    apiMock.startContainer.mockResolvedValue({})
    renderPage()
    await screen.findByText('nginx', {}, { timeout: 5000 })
    await act(async () => {
      fireEvent.click(btnIn(rowOf('redis'), '启动'))
    })
    await waitFor(() => expect(apiMock.startContainer).toHaveBeenCalledWith('ag-docker', 'c-exit'))
    expect(document.querySelector('.ant-modal-confirm')).toBeNull()
  })

  it('操作失败：message.error 带错误信息', async () => {
    apiMock.startContainer.mockRejectedValue(new Error('daemon down'))
    renderPage()
    await screen.findByText('nginx', {}, { timeout: 5000 })
    await act(async () => {
      fireEvent.click(btnIn(rowOf('redis'), '启动'))
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('操作失败: daemon down'))
  })
})
