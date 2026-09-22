import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Services from './index'
import type { Agent } from '@/types'

// Services：agent 后端变体（systemd/Windows/无能力/离线）/ 状态排序与自启 Tag /
// 操作显隐与 RBAC / daemon-reload / unit 文件编辑 / journal 抽屉

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getServiceStatus: vi.fn(),
  getAgentServices: vi.fn(),
  serviceAction: vi.fn(),
  serviceDaemonReload: vi.fn(),
  getServiceUnitFile: vi.fn(),
  saveServiceUnitFile: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

vi.mock('@/workbench/LogsPanel', () => ({
  default: ({ initialSource }: { initialSource: string }) => (
    <div data-testid="logs-panel">{initialSource}</div>
  ),
}))

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => canWrite }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')

const mkSvcAgent = (id: string, hostname: string, backend: string | null, status = 'online'): Agent =>
  ({
    id,
    hostname,
    ip: '10.0.0.1',
    region: 'cn', zone: 'z1',
    status,
    lastSeen: '0',
    capabilities: backend ? [{ type: 'service', metadata: { backend } }] : [{ type: 'files' }],
  }) as unknown as Agent

const agents = [
  mkSvcAgent('ag-linux', 'linux-01', 'systemd'),
  mkSvcAgent('ag-win', 'win-01', 'windows-scm'),
  mkSvcAgent('ag-none', 'none-01', null),
  mkSvcAgent('ag-off', 'off-01', 'systemd', 'offline'),
]

const units = [
  { name: 'nginx.service', description: 'web server', activeState: 'active', subState: 'running', unitFileState: 'enabled' },
  { name: 'redis.service', description: 'cache db', activeState: 'inactive', subState: 'dead', unitFileState: 'disabled' },
  { name: 'bad.service', description: 'broken unit', activeState: 'failed', subState: 'failed', unitFileState: 'enabled' },
  { name: 'cron.service', description: '', activeState: 'inactive', subState: 'dead', unitFileState: 'static' },
  { name: 'ssh.service', description: '', activeState: 'inactive', subState: 'dead', unitFileState: 'masked' },
]

const renderPage = () => {
  apiMock.getAgents.mockResolvedValue(agents)
  apiMock.getServiceStatus.mockResolvedValue({ systemState: 'degraded', total: 5, active: 1, failed: 1 })
  apiMock.getAgentServices.mockResolvedValue({ services: units })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Services />
    </QueryClientProvider>,
  )
}

const selectAgent = async (label: string) => {
  fireEvent.mouseDown(screen.getByText('选择主机'))
  const opt = await waitFor(() => {
    const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
      (o) => o.textContent === label)
    if (!el) throw new Error('option not found')
    return el as HTMLElement
  })
  fireEvent.click(opt)
}

const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(cell)) as HTMLTableRowElement

const btnIn = (row: HTMLTableRowElement, label: string) =>
  Array.from(row.querySelectorAll('button')).find((b) =>
    new RegExp(`^\\s*${label.split('').join('\\s*')}\\s*$`).test(b.textContent || ''))

const iconBtnIn = (row: HTMLTableRowElement, iconClass: string) =>
  Array.from(row.querySelectorAll(`button .${iconClass}`))[0]?.closest('button') as HTMLButtonElement | undefined

describe('Services', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    canWrite = true
  })

  it('未选主机：空态提示且不发服务查询', async () => {
    renderPage()
    expect(await screen.findByText('选择一台主机管理服务')).toBeInTheDocument()
    expect(apiMock.getAgentServices).not.toHaveBeenCalled()
  })

  it('选中 systemd 主机：系统状态条、Alert 文案、failed 置顶排序', async () => {
    renderPage()
    await screen.findByText('选择主机')
    await selectAgent('linux-01')
    // 服务列渲染时剥 .service 后缀（页面实现），DOM 里是短名
    expect(await screen.findByText('bad')).toBeInTheDocument()
    // degraded 携带说明、失败计数标红
    expect(screen.getByText(/degraded：有 unit 处于 failed 状态/)).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText(/作用于 systemd 并记入审计日志/)).toBeInTheDocument()
    // 排序：failed 最前、active 次之、其余按名序
    const names = Array.from(document.querySelectorAll('tr.ant-table-row'))
      .map((tr) => tr.querySelector('td')?.textContent)
    expect(names[0]).toContain('bad')
    expect(names[1]).toContain('nginx')
    expect(names.map((n) => n?.slice(0, 4))).toEqual(['badb', 'ngin', 'cron', 'redi', 'ssh'])
    // 状态徽标带 subState、自启 Tag 映射
    expect(screen.getByText('active (running)')).toBeInTheDocument()
    expect(within(rowOf('nginx')).getByText('自启')).toBeInTheDocument()
    expect(within(rowOf('redis')).getByText('手动')).toBeInTheDocument()
    expect(within(rowOf('cron')).getByText('静态')).toBeInTheDocument()
    expect(within(rowOf('ssh')).getByText('屏蔽')).toBeInTheDocument()
  })

  it('搜索过滤按名称与描述，无匹配时空文案切换', async () => {
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    fireEvent.change(screen.getByPlaceholderText('搜索服务名或描述'), { target: { value: 'cache' } })
    expect(await screen.findByText('redis')).toBeInTheDocument()
    expect(screen.queryByText('nginx')).not.toBeInTheDocument()
    expect(screen.getByText('共 1 个服务')).toBeInTheDocument()
    fireEvent.change(screen.getByPlaceholderText('搜索服务名或描述'), { target: { value: 'nope' } })
    expect(await screen.findByText('无匹配服务')).toBeInTheDocument()
  })

  it('操作显隐：active 行停止/重启/重载，inactive 行启动，masked 行解屏蔽', async () => {
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    const rActive = rowOf('nginx')
    expect(btnIn(rActive, '重启')).toBeTruthy()
    expect(btnIn(rActive, '重载')).toBeTruthy()
    expect(iconBtnIn(rActive, 'anticon-poweroff')).toBeTruthy()
    expect(btnIn(rowOf('redis'), '启动')).toBeTruthy()
    // static 无自启切换
    expect(btnIn(rowOf('cron'), '设自启')).toBeFalsy()
    expect(btnIn(rowOf('ssh'), '启动')).toBeFalsy()
    expect(btnIn(rowOf('ssh'), '解屏蔽')).toBeTruthy()
    expect(btnIn(rowOf('nginx'), '停自启')).toBeTruthy()
    expect(btnIn(rowOf('redis'), '设自启')).toBeTruthy()
  })

  it('RBAC 无写权限：操作与编辑入口全部隐藏，仅剩日志', async () => {
    canWrite = false
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    expect(btnIn(rowOf('nginx'), '重启')).toBeFalsy()
    expect(btnIn(rowOf('redis'), '启动')).toBeFalsy()
    expect(iconBtnIn(rowOf('nginx'), 'anticon-poweroff')).toBeFalsy()
    expect(iconBtnIn(rowOf('nginx'), 'anticon-file')).toBeFalsy()
    expect(iconBtnIn(rowOf('nginx'), 'anticon-file-text')).toBeTruthy()
    expect(screen.queryByText('重载配置')).not.toBeInTheDocument()
  })

  it('动作成功与失败：成功提示并刷新，失败出错误 Alert', async () => {
    apiMock.serviceAction.mockResolvedValueOnce({})
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    fireEvent.click(btnIn(rowOf('nginx'), '重启')!)
    await waitFor(() => expect(apiMock.serviceAction).toHaveBeenCalledWith('ag-linux', 'nginx.service', 'restart'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('nginx.service 重启成功'))
    // 失败：带 response.error 的错误对象
    apiMock.serviceAction.mockRejectedValue({ response: { data: { error: 'unit not loaded' } } })
    fireEvent.click(btnIn(rowOf('redis'), '启动')!)
    expect(await screen.findByText('redis.service 操作失败')).toBeInTheDocument()
    expect(screen.getByText('unit not loaded')).toBeInTheDocument()
  })

  it('daemon-reload：确认后调用并提示', async () => {
    apiMock.serviceDaemonReload.mockResolvedValue({})
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /重载配置/ }))
    expect(await screen.findByText('重新加载 systemd 配置')).toBeInTheDocument()
    expect(apiMock.serviceDaemonReload).not.toHaveBeenCalled()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.serviceDaemonReload).toHaveBeenCalledWith('ag-linux'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('systemd 配置已重新加载'))
  })

  it('unit 文件编辑：读取、保存成功与读取失败', async () => {
    apiMock.getServiceUnitFile.mockResolvedValue({
      fragmentPath: '/lib/systemd/system/nginx.service',
      content: '[Unit]\nDescription=old\n',
    })
    apiMock.saveServiceUnitFile.mockResolvedValue({ path: '/etc/systemd/system/nginx.service' })
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    fireEvent.click(iconBtnIn(rowOf('nginx'), 'anticon-file')!)
    expect(await screen.findByText('编辑 unit 文件：nginx.service')).toBeInTheDocument()
    expect(screen.getByText('/lib/systemd/system/nginx.service')).toBeInTheDocument()
    const textarea = document.querySelector('.ant-modal textarea') as HTMLTextAreaElement
    expect(textarea.value).toContain('Description=old')
    fireEvent.change(textarea, { target: { value: '[Unit]\nDescription=new\n' } })
    fireEvent.click(screen.getByRole('button', { name: /保存并重载/ }))
    await waitFor(() =>
      expect(apiMock.saveServiceUnitFile).toHaveBeenCalledWith('ag-linux', 'nginx.service', '[Unit]\nDescription=new\n'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('nginx.service 已保存（/etc/systemd/system/nginx.service），配置已重载'))
    // 读取失败：报错并收起 Modal
    apiMock.getServiceUnitFile.mockRejectedValue(new Error('down'))
    fireEvent.click(iconBtnIn(rowOf('redis'), 'anticon-file')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('读取 unit 文件失败'))
  })

  it('journal 抽屉：锁定当前 unit 为日志源', async () => {
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    fireEvent.click(iconBtnIn(rowOf('nginx'), 'anticon-file-text')!)
    expect(await screen.findByText('日志：nginx.service')).toBeInTheDocument()
    expect(screen.getByTestId('logs-panel').textContent).toBe('nginx.service')
  })

  it('Windows 主机：SCM 文案、无系统状态与重载语义', async () => {
    renderPage()
    await selectAgent('win-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    expect(screen.getByText(/直接作用于 Windows 服务控制管理器/)).toBeInTheDocument()
    expect(screen.queryByText('系统状态')).not.toBeInTheDocument()
    expect(btnIn(rowOf('nginx'), '重载')).toBeFalsy()
    expect(screen.queryByText('重载配置')).not.toBeInTheDocument()
  })
})
