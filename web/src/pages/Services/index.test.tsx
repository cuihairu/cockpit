import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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

const renderPage = (over?: {
  agents?: Agent[]
  status?: unknown
  services?: unknown
}) => {
  apiMock.getAgents.mockResolvedValue(over?.agents ?? agents)
  apiMock.getServiceStatus.mockResolvedValue(over?.status ?? { systemState: 'degraded', total: 5, active: 1, failed: 1 })
  apiMock.getAgentServices.mockResolvedValue(over?.services ?? { services: units })
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

  it('launchd 后端：launchd 文案且无系统状态/重载/journal', async () => {
    renderPage({
      agents: [mkSvcAgent('ag-mac', 'mac-01', 'launchd')],
      status: { systemState: 'running', total: 1, active: 0, failed: 0 },
      services: { services: [units[0]] },
    })
    await selectAgent('mac-01')
    expect(await screen.findByText('nginx')).toBeInTheDocument()
    expect(screen.getByText(/直接作用于 launchd/)).toBeInTheDocument()
    expect(screen.queryByText('系统状态')).not.toBeInTheDocument()
    expect(btnIn(rowOf('nginx'), '重载')).toBeFalsy()
    expect(iconBtnIn(rowOf('nginx'), 'anticon-file-text')).toBeFalsy()
    expect(screen.queryByText('重载配置')).not.toBeInTheDocument()
  })

  it('未知 activeState/subState/系统状态回落；失败数为 0 显示 0', async () => {
    renderPage({
      agents: [mkSvcAgent('ag-linux', 'linux-01', 'systemd')],
      status: { systemState: 'weird-state', total: 2, active: 0, failed: 0 },
      services: {
        services: [
          { name: 'x.service', description: '', activeState: 'reloading', subState: '', unitFileState: 'enabled' },
          { name: 'y.service', description: '', activeState: 'inactive', subState: 'dead', unitFileState: 'enabled' },
        ],
      },
    })
    await selectAgent('linux-01')
    expect(await screen.findByText('x')).toBeInTheDocument()
    // ACTIVE_META 缺省键回落原值；subState 空 → 无括号后缀
    expect(screen.getByText('reloading')).toBeInTheDocument()
    // SYSTEM_META 缺省键回落原值
    expect(screen.getByText('weird-state')).toBeInTheDocument()
    // failed=0 → 普通 0（active 计数同样是 0，允许多处）
    expect(screen.getAllByText('0').length).toBeGreaterThanOrEqual(1)
  })

  it('unitFileState：alias/linked/其他值/空串的 Tag 映射', async () => {
    renderPage({
      agents: [mkSvcAgent('ag-linux', 'linux-01', 'systemd')],
      services: {
        services: [
          { name: 'alpha.service', description: '', activeState: 'inactive', subState: 'dead', unitFileState: 'alias' },
          { name: 'bravo.service', description: '', activeState: 'inactive', subState: 'dead', unitFileState: 'linked' },
          { name: 'charlie.service', description: '', activeState: 'inactive', subState: 'dead', unitFileState: 'generated' },
          { name: 'delta.service', description: '', activeState: 'inactive', subState: 'dead', unitFileState: '' },
        ],
      },
    })
    await selectAgent('linux-01')
    expect(await screen.findByText('alpha')).toBeInTheDocument()
    expect(within(rowOf('alpha')).getByText('别名')).toBeInTheDocument()
    expect(within(rowOf('bravo')).getByText('链接')).toBeInTheDocument()
    expect(within(rowOf('charlie')).getByText('generated')).toBeInTheDocument()
    expect(within(rowOf('delta')).getByText('—')).toBeInTheDocument()
  })

  it('动作按钮全覆盖：停止/重载/屏蔽/停自启/设自启/解屏蔽各触发一次', async () => {
    apiMock.serviceAction.mockResolvedValue({})
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    // 逐次点击：前一次 settle 后 actingUnit 清空，否则 loading 会吞掉同行后续点击
    const fire = async (btn: HTMLButtonElement | undefined) => {
      expect(btn).toBeTruthy()
      await act(async () => {
        fireEvent.click(btn!)
      })
    }
    await fire(iconBtnIn(rowOf('nginx'), 'anticon-poweroff')!) // stop
    await fire(btnIn(rowOf('nginx'), '重载')!) // reload
    await fire(btnIn(rowOf('nginx'), '屏蔽')!) // mask
    await fire(btnIn(rowOf('nginx'), '停自启')!) // disable
    await fire(btnIn(rowOf('redis'), '设自启')!) // enable
    await fire(btnIn(rowOf('ssh'), '解屏蔽')!) // unmask
    expect(apiMock.serviceAction).toHaveBeenCalledTimes(6)
    expect(apiMock.serviceAction).toHaveBeenCalledWith('ag-linux', 'nginx.service', 'stop')
    expect(apiMock.serviceAction).toHaveBeenCalledWith('ag-linux', 'nginx.service', 'reload')
    expect(apiMock.serviceAction).toHaveBeenCalledWith('ag-linux', 'nginx.service', 'mask')
    expect(apiMock.serviceAction).toHaveBeenCalledWith('ag-linux', 'nginx.service', 'disable')
    expect(apiMock.serviceAction).toHaveBeenCalledWith('ag-linux', 'redis.service', 'enable')
    expect(apiMock.serviceAction).toHaveBeenCalledWith('ag-linux', 'ssh.service', 'unmask')
  })

  it('动作错误 Alert 可关闭；并发动作 settle 时 actingUnit 不匹配不误清', async () => {
    // nginx 动作永挂起；redis 动作立即完成——后者 settle 时 onSettled 闭包里
    // actingUnit 仍是 nginx.service（effect 未及刷新）→ 走「不匹配」分支
    apiMock.serviceAction.mockImplementation((_a: string, unit: string) =>
      unit === 'redis.service' ? Promise.resolve() : new Promise<void>(() => {}))
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(btnIn(rowOf('nginx'), '重启')!)
    })
    await waitFor(() =>
      expect(apiMock.serviceAction).toHaveBeenCalledWith('ag-linux', 'nginx.service', 'restart'))
    // 不包 act：redis 的立即 settle（微任务）抢在 passive effect 刷新 onSettled
    // 闭包之前执行——此时闭包里的 actingUnit 仍是 nginx.service → 不匹配分支
    fireEvent.click(btnIn(rowOf('redis'), '启动')!)
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('redis.service 启动成功'))
    // 制造失败后关闭错误 Alert（触发 onClose）
    apiMock.serviceAction.mockRejectedValue({ response: { data: { error: 'boom' } } })
    await act(async () => {
      fireEvent.click(btnIn(rowOf('bad'), '启动')!)
    })
    await screen.findByText('bad.service 操作失败')
    fireEvent.click(document.querySelector('.ant-alert .anticon-close')!)
  })

  it('daemon-reload 失败报错；unit 弹窗 X/取消；保存失败报错；日志抽屉可关', async () => {
    apiMock.serviceDaemonReload.mockRejectedValue(new Error('down'))
    apiMock.getServiceUnitFile.mockResolvedValue({
      fragmentPath: '/lib/systemd/system/nginx.service',
      content: '[Unit]\n',
    })
    apiMock.saveServiceUnitFile.mockRejectedValue(new Error('save down'))
    renderPage()
    await selectAgent('linux-01')
    expect(await screen.findByText('bad')).toBeInTheDocument()
    // daemon-reload 失败
    fireEvent.click(screen.getByRole('button', { name: /重载配置/ }))
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('重载配置失败'))
    // unit 编辑：取消按钮 + X 各触发一次 onCancel/取消
    fireEvent.click(iconBtnIn(rowOf('nginx'), 'anticon-file')!)
    expect(await screen.findByText('编辑 unit 文件：nginx.service')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }))
    fireEvent.click(iconBtnIn(rowOf('nginx'), 'anticon-file')!)
    await screen.findByText('编辑 unit 文件：nginx.service')
    // 保存失败 → fallback 文案
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保存并重载/ }))
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
    fireEvent.click(document.querySelector('.ant-modal-close')!)
    // 日志抽屉关闭
    fireEvent.click(iconBtnIn(rowOf('nginx'), 'anticon-file-text')!)
    expect(await screen.findByText('日志：nginx.service')).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-drawer-close')!)
  })

  it('capabilities 缺省与 hostname 缺省的主机选项标签', async () => {
    renderPage({
      agents: [
        { id: 'ag-nocaps', hostname: 'nocaps', ip: '1.1.1.1', status: 'online', lastSeen: '0' } as unknown as Agent,
        { id: 'ag-noid', hostname: '', ip: '1.1.1.2', status: 'online', lastSeen: '0',
          capabilities: [{ type: 'service', metadata: { backend: 'systemd' } }] } as unknown as Agent,
      ],
    })
    fireEvent.mouseDown(screen.getByText('选择主机'))
    const optText = (label: string) =>
      Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent?.startsWith(label))?.textContent ?? ''
    await waitFor(() =>
      expect(document.querySelectorAll('.ant-select-item-option').length).toBeGreaterThan(0))
    expect(optText('nocaps')).toBe('nocaps（未检测到服务管理）')
    // hostname 空 → 用 id 展示
    expect(optText('ag-noid')).toBe('ag-noid')
  })
})

