import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import GitBar from './GitBar'

// GitBar：接入状态四态（未接入/无 git/drift/behind）、接入弹窗校验与提交、
// 拉取成功/409/其他错误、无权限裁剪、老 agent 静默

const apiMock = vi.hoisted(() => ({
  getStackGit: vi.fn(),
  pullStackGit: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => canWrite }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const agents = [
  { id: 'ag-1111111111112', hostname: 'node-01' },
  { id: 'ag-2222222222223', hostname: 'node-02' },
]

const mount = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <GitBar agents={agents} />
    </QueryClientProvider>,
  )
}

const axiosErr = (status: number, data: unknown) =>
  Object.assign(new Error('Request failed'), { isAxiosError: true, response: { status, data } })

beforeEach(() => {
  vi.clearAllMocks()
  canWrite = true
})

describe('GitBar', () => {
  it('无 agent → 不渲染', () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { container } = render(
      <QueryClientProvider client={qc}>
        <GitBar agents={[]} />
      </QueryClientProvider>,
    )
    expect(container.textContent).toBe('')
    expect(apiMock.getStackGit).not.toHaveBeenCalled()
  })

  it('加载中不渲染告警，完成后未接入显示接入入口', async () => {
    apiMock.getStackGit.mockImplementation(
      (id: string) =>
        new Promise((resolve) =>
          setTimeout(() => resolve({ enabled: false, reason: 'not-a-repo', ahead: 0, behind: 0 }), id === 'ag-1111111111112' ? 10 : 0),
        ),
    )
    const { container, unmount } = mount()
    // 查询在途：无告警内容
    expect(screen.queryByText(/未接入/)).toBeNull()
    await waitFor(() => {
      expect(screen.getAllByText(/stacks 目录未接入 Git 仓库/).length).toBe(2)
    })
    expect(container.querySelector('.ant-alert')).not.toBeNull()
    unmount()
    // 无权限重挂载：接入按钮不渲染
    canWrite = false
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <GitBar agents={agents} />
      </QueryClientProvider>,
    )
    await waitFor(() => {
      expect(screen.getAllByText(/stacks 目录未接入 Git 仓库/).length).toBe(2)
    })
    expect(screen.queryByText('接入 Git 仓库')).toBeNull()
  })

  it('agent 无 git：warning 且无操作按钮', async () => {
    apiMock.getStackGit.mockResolvedValue({ enabled: false, reason: 'git-unavailable', ahead: 0, behind: 0 })
    mount()
    await waitFor(() => {
      expect(screen.getAllByText(/agent 主机未安装 git/).length).toBe(2)
    })
    expect(screen.queryByText('接入 Git 仓库')).toBeNull()
    expect(screen.queryByText('拉取更新')).toBeNull()
  })

  it('老 agent 报错（不认识 git.status）：静默跳过', async () => {
    apiMock.getStackGit.mockRejectedValue(axiosErr(502, { error: 'unknown stack action: stack.git.status' }))
    const { container } = mount()
    await waitFor(() => {
      expect(apiMock.getStackGit).toHaveBeenCalledTimes(2)
    })
    await waitFor(() => {
      expect(container.textContent).toBe('')
    })
  })

  it('已接入且干净：不渲染任何告警', async () => {
    apiMock.getStackGit.mockResolvedValue({
      enabled: true, remote: 'https://git.example/ops/stacks.git', branch: 'main',
      head: 'abc1234', remoteHead: 'abc1234', ahead: 0, behind: 0,
    })
    const { container } = mount()
    await waitFor(() => {
      expect(apiMock.getStackGit).toHaveBeenCalledTimes(2)
    })
    expect(container.querySelector('.ant-alert')).toBeNull()
  })

  it('drift：warning 列出改动文件（超出 5 条折叠）+ 拉取按钮可用', async () => {
    const changes = [
      { code: 'M', path: 'web/compose.yml' },
      { code: '??', path: 'newstack/compose.yml' },
      { code: 'M', path: 'b/1' },
      { code: 'M', path: 'b/2' },
      { code: 'M', path: 'b/3' },
      { code: 'M', path: 'b/4' },
    ]
    apiMock.getStackGit.mockResolvedValue({
      enabled: true, remote: 'https://git.example/ops/stacks.git', branch: 'main',
      head: 'abc1234', remoteHead: 'def5678', ahead: 0, behind: 0, changes,
    })
    apiMock.pullStackGit.mockResolvedValue({
      enabled: true, remote: 'https://git.example/ops/stacks.git', branch: 'main',
      head: 'def5678', remoteHead: 'def5678', ahead: 0, behind: 0,
    })
    mount()
    await waitFor(() => {
      expect(screen.getAllByText(/本地改动 6 处/).length).toBe(2)
    })
    expect(screen.getAllByText(/M web\/compose.yml；\?\? newstack\/compose\.yml/).length).toBe(2)
    expect(screen.getAllByText(/等 6 处/).length).toBe(2)

    fireEvent.click(screen.getAllByText('拉取更新')[0])
    await waitFor(() => {
      expect(apiMock.pullStackGit).toHaveBeenCalledWith('ag-1111111111112', undefined)
      expect(msgSuccess).toHaveBeenCalled()
    })
  })

  it('仅远端有新提交：info 提示 + 拉取；响应缺字段时占位符兜底', async () => {
    apiMock.getStackGit.mockResolvedValue({
      enabled: true, remote: 'https://git.example/ops/stacks.git', branch: 'main',
      head: 'abc1234', remoteHead: 'def5678', ahead: 0, behind: 2,
    })
    // 拉取响应缺 branch/head/remote：成功文案与告警行走 "-" 占位
    apiMock.pullStackGit.mockResolvedValue({ enabled: true, ahead: 0, behind: 0 })
    mount()
    await waitFor(() => {
      expect(screen.getAllByText(/远端有新提交/).length).toBe(2)
    })
    fireEvent.click(screen.getAllByText('拉取更新')[0])
    await waitFor(() => {
      expect(msgSuccess).toHaveBeenCalledWith(expect.stringContaining('已同步到 -@-（远端 -）'))
    })
  })

  it('drift 状态缺 remote/branch/head：告警行 "-" 占位', async () => {
    apiMock.getStackGit.mockResolvedValue({
      enabled: true, ahead: 0, behind: 0,
      changes: [{ code: 'M', path: 'web/compose.yml' }],
    })
    mount()
    await waitFor(() => {
      expect(screen.getAllByText(/仓库 -（-@-）/).length).toBe(2)
    })
  })

  it('拉取 409：warning 引导先处理 drift', async () => {
    apiMock.getStackGit.mockResolvedValue({
      enabled: true, remote: 'https://git.example/ops/stacks.git', branch: 'main',
      head: 'abc1234', remoteHead: 'def5678', ahead: 0, behind: 1,
    })
    apiMock.pullStackGit.mockRejectedValue(axiosErr(409, { error: 'merge conflict, resolve local changes first' }))
    mount()
    await waitFor(() => {
      expect(screen.getAllByText(/远端有新提交/).length).toBe(2)
    })
    fireEvent.click(screen.getAllByText('拉取更新')[0])
    await waitFor(() => {
      expect(msgWarning).toHaveBeenCalledWith(expect.stringContaining('存在本地改动，与远端冲突'))
    })
  })

  it('拉取其他错误：error 提示', async () => {
    apiMock.getStackGit.mockResolvedValue({
      enabled: true, remote: 'r', branch: 'main', head: 'a', remoteHead: 'b', ahead: 0, behind: 1,
    })
    apiMock.pullStackGit.mockRejectedValue(axiosErr(502, { error: 'fetch: connection refused' }))
    mount()
    await waitFor(() => {
      expect(screen.getAllByText(/远端有新提交/).length).toBe(2)
    })
    fireEvent.click(screen.getAllByText('拉取更新')[0])
    await waitFor(() => {
      expect(msgError).toHaveBeenCalledWith(expect.stringContaining('fetch: connection refused'))
    })
  })

  it('接入弹窗：必填校验 + 提交带 payload + 成功关闭', async () => {
    apiMock.getStackGit.mockResolvedValue({ enabled: false, reason: 'not-a-repo', ahead: 0, behind: 0 })
    apiMock.pullStackGit.mockResolvedValue({
      enabled: true, remote: 'https://git.example/ops/stacks.git', branch: 'main',
      head: 'abc1234', remoteHead: 'abc1234', ahead: 0, behind: 0,
    })
    mount()
    await waitFor(() => {
      expect(screen.getAllByText('接入 Git 仓库').length).toBe(2)
    })
    fireEvent.click(screen.getAllByText('接入 Git 仓库')[0])
    expect(screen.getByText(/接入 Git 仓库 — node-01/)).toBeInTheDocument()

    // 必填校验：直接提交被拦
    fireEvent.click(screen.getByText('接入并拉取'))
    await waitFor(() => {
      expect(screen.getByText('请输入仓库地址')).toBeInTheDocument()
    })
    expect(apiMock.pullStackGit).not.toHaveBeenCalled()

    fireEvent.change(screen.getByPlaceholderText('https://git.example.com/ops/stacks.git'), {
      target: { value: 'https://git.example/ops/stacks.git' },
    })
    fireEvent.change(screen.getByPlaceholderText('main'), { target: { value: 'release' } })
    fireEvent.click(screen.getByText('接入并拉取'))
    await waitFor(() => {
      expect(apiMock.pullStackGit).toHaveBeenCalledWith('ag-1111111111112', {
        remote: 'https://git.example/ops/stacks.git',
        branch: 'release',
      })
    })
    await waitFor(() => {
      expect(msgSuccess).toHaveBeenCalled()
    })
    // jsdom 无过渡动画不做 DOM 消失断言；行为验证：重新打开弹窗表单已重置
    fireEvent.click(screen.getAllByText('接入 Git 仓库')[0])
    await waitFor(() => {
      const remoteInput = screen.getByPlaceholderText('https://git.example.com/ops/stacks.git') as HTMLInputElement
      expect(remoteInput.value).toBe('')
    })
  })

  it('接入弹窗取消：清空待接入 agent 且不提交', async () => {
    apiMock.getStackGit.mockResolvedValue({ enabled: false, reason: 'not-a-repo', ahead: 0, behind: 0 })
    mount()
    await waitFor(() => {
      expect(screen.getAllByText('接入 Git 仓库').length).toBe(2)
    })
    fireEvent.click(screen.getAllByText('接入 Git 仓库')[0])
    fireEvent.change(screen.getByPlaceholderText('https://git.example.com/ops/stacks.git'), {
      target: { value: 'https://git.example/x.git' },
    })
    // antd 按钮对两字文案自动插空格（取 消）
    fireEvent.click(screen.getByText('取 消'))
    expect(apiMock.pullStackGit).not.toHaveBeenCalled()
    // 行为验证：取消后重开弹窗，表单已重置（attachAgent 已被清空再置回）
    fireEvent.click(screen.getAllByText('接入 Git 仓库')[0])
    await waitFor(() => {
      const remoteInput = screen.getByPlaceholderText('https://git.example.com/ops/stacks.git') as HTMLInputElement
      expect(remoteInput.value).toBe('')
    })
  })
})
