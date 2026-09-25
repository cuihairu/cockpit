import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Cron from './index'
import type { Agent } from '@/types'

// Cron：主机/目标用户两级选择、任务表三态下次触发、启停与删除、
// 新建/编辑表单（名称/表达式/命令校验）、RBAC 裁剪、systemd timer 过滤

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getCronUsers: vi.fn(),
  getCronStatus: vi.fn(),
  getCronJobs: vi.fn(),
  getCronTimers: vi.fn(),
  applyCronJob: vi.fn(),
  deleteCronJob: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => canWrite }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')

const mkAgent = (id: string, hostname: string, caps: string[], status = 'online'): Agent =>
  ({
    id,
    hostname,
    ip: '10.0.0.1',
    region: 'cn', zone: 'z1',
    status,
    lastSeen: '0',
    capabilities: caps.map((type) => ({ type })),
  }) as unknown as Agent

const agents = [
  mkAgent('ag-cron', 'cron-01', ['cron']),
  mkAgent('ag-plain', 'plain', ['files']),
  mkAgent('ag-off', 'off-01', ['cron'], 'offline'),
]

const jobs = [
  { name: 'backup', schedule: '0 3 * * *', command: '/opt/backup.sh', enabled: true, next_run: 1760000000 },
  // @reboot + enabled：下次触发显示「开机时」（!enabled 分支优先级更高）
  { name: 'cleanup', schedule: '@reboot', command: 'rm -f /tmp/x', enabled: true },
  { name: 'rotate', schedule: '0 5 * * *', command: 'logrotate', enabled: false },
]

const timers = [
  { unit: 'logrotate.timer', description: 'Rotate logs', schedule: 'daily', unitFileState: 'enabled', last_trigger: 1759000000, next_run: 1760000000 },
  { unit: 'mnt-data.timer', description: '', schedule: '', unitFileState: '', last_trigger: 0, next_run: 0 },
]

const renderPage = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Cron />
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
    new RegExp(label.split('').join('\\s*')).test(b.textContent || ''))

// getByLabelText 可能直接命中 input/textarea 本身（antd Form label htmlFor 关联）
const fieldEl = (label: string, tag: 'input' | 'textarea' = 'input') => {
  const el = screen.getByLabelText(label)
  return (el.matches(tag) ? el : el.querySelector(tag)!) as HTMLInputElement
}

const modalTitle = async (title: string) => {
  await waitFor(() => {
    const el = document.querySelector('.ant-modal-title')
    if (el?.textContent !== title) throw new Error(`modal title: ${el?.textContent}`)
  })
}

const modalOk = () => {
  fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn-primary')!)
}

describe('Cron', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    canWrite = true
    apiMock.getAgents.mockResolvedValue(agents)
    apiMock.getCronUsers.mockResolvedValue({ users: [{ name: 'root', shell: '/bin/bash' }, { name: 'www', shell: '' }] })
    apiMock.getCronStatus.mockResolvedValue({ user: 'cui', cockpitCount: 2, externalCount: 1 })
    apiMock.getCronJobs.mockResolvedValue({ jobs, external: '5 0 * * * /manual/job\n' })
    apiMock.getCronTimers.mockResolvedValue({ timers })
  })

  it('未选主机：空态且不发任务查询', async () => {
    renderPage()
    expect(await screen.findByText('选择一台主机开始管理定时任务')).toBeInTheDocument()
    expect(apiMock.getCronJobs).not.toHaveBeenCalled()
  })

  it('选中主机：状态条、任务表与下次触发三态、外部条目与 timer 面板标题', async () => {
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    expect(screen.getByText('cui')).toBeInTheDocument()
    expect(screen.getAllByText('2').length).toBeGreaterThan(0)
    expect(screen.getByText(/只管理 Cockpit 下发的任务/)).toBeInTheDocument()
    const r1 = rowOf('backup')
    expect(within(r1).getByText('0 3 * * *')).toBeInTheDocument()
    expect(within(r1).getByText('/opt/backup.sh')).toBeInTheDocument()
    const r2 = rowOf('cleanup')
    expect(within(r2).getByText('开机时')).toBeInTheDocument()
    expect(within(rowOf('rotate')).getByText('已禁用')).toBeInTheDocument()
    expect(screen.getByText(/外部条目（只读，共 1 行非空条目）/)).toBeInTheDocument()
    expect(screen.getByText(/systemd 定时器（只读，共 2 个）/)).toBeInTheDocument()
  })

  it('目标用户切换：Alert 文案与查询参数跟随', async () => {
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    // AutoComplete 的 placeholder 渲染在 span 上，从它定位内部 input
    const userSelect = screen.getByText('当前用户', { selector: '.ant-select-selection-placeholder' })
      .closest('.ant-select')!
    fireEvent.change(userSelect.querySelector('input')!, { target: { value: 'root' } })
    await waitFor(() => expect(apiMock.getCronJobs).toHaveBeenLastCalledWith('ag-cron', 'root'))
    expect(await screen.findByText(/正在管理 root 的 crontab/)).toBeInTheDocument()
  })

  it('启停开关：翻转 enabled 原样 apply，失败走 message.error', async () => {
    apiMock.applyCronJob.mockResolvedValue({})
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    fireEvent.click(rowOf('backup').querySelector('.ant-switch')!)
    await waitFor(() =>
      expect(apiMock.applyCronJob).toHaveBeenCalledWith('ag-cron', { ...jobs[0], enabled: false }, undefined))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('任务 backup 已停用'))
    apiMock.applyCronJob.mockRejectedValue({ response: { data: { error: 'crontab: install failed' } } })
    fireEvent.click(rowOf('cleanup').querySelector('.ant-switch')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('crontab: install failed'))
  })

  it('删除：Popconfirm 确认后调用，取消不触发', async () => {
    apiMock.deleteCronJob.mockResolvedValue({})
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    fireEvent.click(btnIn(rowOf('backup'), '删除')!)
    expect(await screen.findByText(/将从 crontab 移除 backup/)).toBeInTheDocument()
    expect(apiMock.deleteCronJob).not.toHaveBeenCalled()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.deleteCronJob).toHaveBeenCalledWith('ag-cron', 'backup', undefined))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('任务 backup 已删除'))
  })

  it('编辑：表单回填且名称锁定，保存走 apply', async () => {
    apiMock.applyCronJob.mockResolvedValue({})
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    fireEvent.click(btnIn(rowOf('backup'), '编辑')!)
    await modalTitle('编辑任务 backup')
    const nameInput = fieldEl('任务名称')
    expect(nameInput.disabled).toBe(true)
    expect(nameInput.value).toBe('backup')
    fireEvent.change(fieldEl('命令', 'textarea'), { target: { value: '/opt/backup2.sh' } })
    modalOk()
    await waitFor(() =>
      expect(apiMock.applyCronJob).toHaveBeenCalledWith('ag-cron',
        { name: 'backup', schedule: '0 3 * * *', command: '/opt/backup2.sh', enabled: true }, undefined))
  })

  it('新建：表达式与命令校验分支，合法写入成功', async () => {
    apiMock.applyCronJob.mockResolvedValue({})
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建任务/ }))
    await modalTitle('新建任务')
    // 表达式非法：4 字段
    fireEvent.change(fieldEl('任务名称'), { target: { value: 'job1' } })
    fireEvent.change(fieldEl('表达式'), { target: { value: '0 3 * *' } })
    fireEvent.change(fieldEl('命令', 'textarea'), { target: { value: 'echo hi' } })
    modalOk()
    expect(await screen.findByText('需要 5 个字段（分 时 日 月 周）')).toBeInTheDocument()
    // 分钟越界
    fireEvent.change(fieldEl('表达式'), { target: { value: '60 * * * *' } })
    modalOk()
    expect(await screen.findByText(/超出范围 \[0,59\]/)).toBeInTheDocument()
    // 非法简写
    fireEvent.change(fieldEl('表达式'), { target: { value: '@sometimes' } })
    modalOk()
    expect(await screen.findByText('不支持的简写 @sometimes')).toBeInTheDocument()
    // 命令带换行
    fireEvent.change(fieldEl('表达式'), { target: { value: '@hourly' } })
    fireEvent.change(fieldEl('命令', 'textarea'), { target: { value: 'echo a\nb' } })
    modalOk()
    expect(await screen.findByText(/命令不能包含换行/)).toBeInTheDocument()
    // 合法提交
    fireEvent.change(fieldEl('命令', 'textarea'), { target: { value: 'echo ok' } })
    modalOk()
    await waitFor(() =>
      expect(apiMock.applyCronJob).toHaveBeenCalledWith('ag-cron',
        { name: 'job1', schedule: '@hourly', command: 'echo ok', enabled: true }, undefined))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('任务 job1 已写入 crontab'))
  })

  it('写入失败：applyError Alert 展示后端错误', async () => {
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    apiMock.applyCronJob.mockRejectedValue({ response: { data: { error: '自检失败：命令行格式异常' } } })
    fireEvent.click(screen.getByRole('button', { name: /新建任务/ }))
    await modalTitle('新建任务')
    fireEvent.change(fieldEl('任务名称'), { target: { value: 'job2' } })
    fireEvent.change(fieldEl('命令', 'textarea'), { target: { value: 'echo x' } })
    modalOk()
    expect(await screen.findByText('写入失败')).toBeInTheDocument()
    expect(screen.getByText('自检失败：命令行格式异常')).toBeInTheDocument()
  })

  it('RBAC 无写权限：入口与开关全部锁定', async () => {
    canWrite = false
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /新建任务/ })).not.toBeInTheDocument()
    expect(btnIn(rowOf('backup'), '编辑')).toBeFalsy()
    expect(btnIn(rowOf('backup'), '删除')).toBeFalsy()
    expect(rowOf('backup').querySelector('.ant-switch')!.className).toContain('disabled')
  })

  it('timer 面板：展开后按关键字过滤', async () => {
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText(/systemd 定时器（只读，共 2 个）/)).toBeInTheDocument()
    fireEvent.click(screen.getByText(/systemd 定时器（只读，共 2 个）/))
    expect(await screen.findByText('logrotate.timer')).toBeInTheDocument()
    expect(screen.getByText('Rotate logs')).toBeInTheDocument()
    expect(screen.getByText('daily')).toBeInTheDocument()
    // 空值兜底行
    expect(within(rowOf('mnt-data.timer')).getAllByText('-').length).toBeGreaterThanOrEqual(3)
    fireEvent.change(screen.getByPlaceholderText('按 unit 或描述过滤'), { target: { value: 'logrotate' } })
    await waitFor(() => expect(screen.queryByText('mnt-data.timer')).not.toBeInTheDocument())
  })

  it('表达式步长非法校验与常用预设回填', async () => {
    apiMock.applyCronJob.mockResolvedValue({})
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建任务/ }))
    await modalTitle('新建任务')
    // */0：步长 <1 → 第 1 字段步长非法
    fireEvent.change(fieldEl('任务名称'), { target: { value: 'job2' } })
    fireEvent.change(fieldEl('表达式'), { target: { value: '*/0 * * * *' } })
    fireEvent.change(fieldEl('命令', 'textarea'), { target: { value: 'echo hi' } })
    modalOk()
    expect(await screen.findByText('第 1 字段步长非法：/0')).toBeInTheDocument()
    // 常用预设选择 → 表达式回填
    const presetItem = Array.from(document.querySelectorAll('.ant-form-item')).find(
      (el) => el.textContent?.includes('常用预设'))!
    fireEvent.mouseDown(presetItem.querySelector('.ant-select-selector')!)
    const presetOpt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent?.includes('每小时整点'))
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(presetOpt)
    await waitFor(() => expect((fieldEl('表达式') as HTMLInputElement).value).toBe('0 * * * *'))
  })

  it('表达式校验补充：空值、格式非法、步长超上限、range 合法提交', async () => {
    apiMock.applyCronJob.mockResolvedValue({})
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建任务/ }))
    await modalTitle('新建任务')
    fireEvent.change(fieldEl('任务名称'), { target: { value: 'job9' } })
    fireEvent.change(fieldEl('命令', 'textarea'), { target: { value: 'echo hi' } })
    // 字段格式非法（正则不匹配）
    fireEvent.change(fieldEl('表达式'), { target: { value: '0 3 * * a!' } })
    modalOk()
    expect(await screen.findByText('第 5 字段格式非法：a!')).toBeInTheDocument()
    // 步长超过字段上限（分钟 hi=59）
    fireEvent.change(fieldEl('表达式'), { target: { value: '*/61 * * * *' } })
    modalOk()
    expect(await screen.findByText('第 1 字段步长非法：/61')).toBeInTheDocument()
    // range 语法合法：直接提交成功
    fireEvent.change(fieldEl('表达式'), { target: { value: '10-20 * * * *' } })
    modalOk()
    await waitFor(() =>
      expect(apiMock.applyCronJob).toHaveBeenCalledWith('ag-cron',
        expect.objectContaining({ schedule: '10-20 * * * *' }), undefined))
    // 合法步长：提交成功
    fireEvent.change(fieldEl('表达式'), { target: { value: '*/15 * * * *' } })
    modalOk()
    await waitFor(() =>
      expect(apiMock.applyCronJob).toHaveBeenLastCalledWith('ag-cron',
        expect.objectContaining({ schedule: '*/15 * * * *' }), undefined))
  })

  it('列表边界：空 hostname、未检测 crontab 标注、无 next_run、status 空用户、外部计数缺省、timer 无描述', async () => {
    // capabilities 缺失（undefined）的 agent：?? [] 兜底
    const noCap = mkAgent('ag-nocap', 'nocap', ['files']) as unknown as Record<string, unknown>
    delete noCap.capabilities
    apiMock.getAgents.mockResolvedValue([...agents, mkAgent('ag-nohost', '', ['cron']), noCap as unknown as Agent])
    apiMock.getCronStatus.mockResolvedValue({ user: '', cockpitCount: 0 })
    apiMock.getCronJobs.mockResolvedValue({
      jobs: [...jobs, { name: 'nextra', schedule: '0 3 * * *', command: 'x', enabled: true }],
      external: '5 0 * * * /manual/job\n',
    })
    apiMock.getCronTimers.mockResolvedValue({ timers: [...timers, { unit: 'orph.timer', schedule: '' }] })
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    // 空 hostname 的 agent 在主机下拉显示 id；plain 显示「未检测到 crontab」且被禁用
    const hostSelect = Array.from(document.querySelectorAll('.ant-select')).find(
      (s) => s.textContent === 'cron-01')
    fireEvent.mouseDown(hostSelect!.querySelector('.ant-select-selector')!)
    expect(await screen.findByText('ag-nohost')).toBeInTheDocument()
    const plainOpts = await screen.findAllByText((_, el) =>
      el?.classList.contains('ant-select-item-option') === true &&
      (el.textContent ?? '').includes('未检测到 crontab'))
    expect(plainOpts[0]!.classList).toContain('ant-select-item-option-disabled')
    fireEvent.click(document.body)
    // enabled 且非 @reboot 且无 next_run → '-'
    expect(rowOf('nextra').textContent).toContain('-')
    // status.user 为空 → 运行用户显示 '—'；无 externalCount → 共 0 行
    expect(await screen.findByText('—')).toBeInTheDocument()
    await waitFor(() =>
      expect(document.body.textContent).toContain('共 0 行非空条目'))
    // timer 无 description 字段：搜 unit 不含的 'logs' 强制评估 ?? '' 右侧；
    // 或按 unit 关键字过滤仍命中
    fireEvent.click(screen.getByText(/systemd 定时器（只读，共 3 个）/))
    expect(await screen.findByText('orph.timer')).toBeInTheDocument()
    fireEvent.change(screen.getByPlaceholderText('按 unit 或描述过滤'), { target: { value: 'logs' } })
    await waitFor(() => expect(screen.queryByText('orph.timer')).not.toBeInTheDocument())
    expect(screen.getByText('logrotate.timer')).toBeInTheDocument() // description 命中
    fireEvent.change(screen.getByPlaceholderText('按 unit 或描述过滤'), { target: { value: 'orph' } })
    await waitFor(() => expect(screen.queryByText('logrotate.timer')).not.toBeInTheDocument())
    expect(screen.getByText('orph.timer')).toBeInTheDocument()
  })

  it('选目标用户写入：成功文案带用户名；已禁用任务再启用提示已启用', async () => {
    apiMock.applyCronJob.mockResolvedValue({})
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    // 已禁用的 rotate → 启用：'已启用' 分支
    fireEvent.click(rowOf('rotate').querySelector('.ant-switch')!)
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('任务 rotate 已启用'))
    // 选目标用户后新建 → 成功文案带用户名
    const userSelect = screen.getByText('当前用户', { selector: '.ant-select-selection-placeholder' })
      .closest('.ant-select')!
    fireEvent.change(userSelect.querySelector('input')!, { target: { value: 'root' } })
    await screen.findByText(/正在管理 root 的 crontab/)
    fireEvent.click(screen.getByRole('button', { name: /新建任务/ }))
    await modalTitle('新建任务')
    fireEvent.change(fieldEl('任务名称'), { target: { value: 'job10' } })
    fireEvent.change(fieldEl('表达式'), { target: { value: '0 4 * * *' } })
    fireEvent.change(fieldEl('命令', 'textarea'), { target: { value: 'echo u' } })
    modalOk()
    await waitFor(() =>
      expect(apiMock.applyCronJob).toHaveBeenLastCalledWith('ag-cron',
        expect.objectContaining({ name: 'job10' }), 'root'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('任务 job10 已写入 root 的 crontab'))
  })

  it('删除失败错误透出；编辑弹窗取消关闭', async () => {
    apiMock.deleteCronJob.mockRejectedValue(new Error('cron del fail'))
    renderPage()
    await selectAgent('cron-01')
    expect(await screen.findByText('backup')).toBeInTheDocument()
    fireEvent.click(btnIn(rowOf('backup'), '删除')!)
    expect(await screen.findByText(/将从 crontab 移除 backup/)).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('删除失败'))
    // 编辑弹窗取消 → 关闭
    fireEvent.click(btnIn(rowOf('backup'), '编辑')!)
    await modalTitle('编辑任务 backup')
    fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn:not(.ant-btn-primary)')!)
    // jsdom 无过渡动画不做 DOM 消失断言；行为验证：重开编辑仍正常回填
    fireEvent.click(btnIn(rowOf('backup'), '编辑')!)
    await modalTitle('编辑任务 backup')
    expect((fieldEl('任务名称') as HTMLInputElement).value).toBe('backup')
  })
})
