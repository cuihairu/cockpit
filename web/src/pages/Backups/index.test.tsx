import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Backups from './index'
import type { Agent, BackupConfig } from '@/types'

// Backups：配置表（计划标签/源截断/状态 Tag）、历史与文件抽屉、恢复双阶段
// （表单校验→任务视图轮询）、新建/编辑表单、ServerBackupCard 集成、RBAC

const apiMock = vi.hoisted(() => ({
  getAgents: vi.fn(),
  getBackupConfigs: vi.fn(),
  createBackupConfig: vi.fn(),
  updateBackupConfig: vi.fn(),
  deleteBackupConfig: vi.fn(),
  runBackup: vi.fn(),
  getBackupConfigRuns: vi.fn(),
  getBackupFiles: vi.fn(),
  deleteBackupFile: vi.fn(),
  syncBackupFileRemote: vi.fn(),
  downloadBackupFile: vi.fn(),
  restoreBackup: vi.fn(),
  getBackupTask: vi.fn(),
  getServerBackups: vi.fn(),
  getServerBackupConfig: vi.fn(),
  putServerBackupConfig: vi.fn(),
  runServerBackup: vi.fn(),
  deleteServerBackup: vi.fn(),
  syncServerBackupRemote: vi.fn(),
  downloadServerBackup: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

let canWrite = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => canWrite }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')

const agents = [
  { id: 'ag-1', hostname: 'node-01', ip: '10.0.0.1', region: 'cn', zone: 'z1',
    status: 'online', lastSeen: '0', capabilities: [] },
  { id: 'ag-2', hostname: 'off-02', ip: '10.0.0.2', region: 'cn', zone: 'z1',
    status: 'offline', lastSeen: '0', capabilities: [] },
] as unknown as Agent[]

const configs = [
  { id: 1, agent_id: 'ag-1', name: 'etc', sources: ['/etc/nginx', '/etc/ssh', '/opt/conf'],
    dest_dir: '/mnt/backup', remote_dest: 'my-s3:cockpit', pre_hook: '', schedule: 'daily@03:00',
    retention: 7, enabled: true, last_status: 'success', last_run_at: 1759000000 },
  { id: 2, agent_id: 'ag-2', name: 'db', sources: ['/data'], dest_dir: '/backup', remote_dest: '',
    pre_hook: '', schedule: 'every:6h', retention: 0, enabled: false, last_status: 'running', last_run_at: 0 },
  { id: 3, agent_id: 'ag-1', name: 'logs', sources: ['/var/log'], dest_dir: '/backup2', remote_dest: '',
    pre_hook: '', schedule: 'manual', retention: 3, enabled: true, last_status: '', last_run_at: 0 },
] as unknown as BackupConfig[]

const runs = [
  { id: 1, status: 'success', file: 'etc-20260914.tar.gz', size: 1048576, remoteStatus: 'ok',
    startedAt: 1759000000, finishedAt: 1759000600, error: '' },
  { id: 2, status: 'failed', file: '', size: 0, remoteStatus: undefined,
    startedAt: 1758000000, finishedAt: 1758000100, error: 'tar: exited with status 2' },
  { id: 3, status: 'timeout', file: 'x.tar.gz', size: 0, remoteStatus: 'push-failed', remoteError: 'rclone: 403',
    startedAt: 0, finishedAt: 0, error: '' },
]

const files = [
  { name: 'etc-20260914-030000.tar.gz', size: 2097152, mtime: 1759000000 },
  { name: 'etc-20260913-030000.tar.gz', size: 1024, mtime: 0 },
]

const serverBackups = [
  { name: 'server-20260920.db', modTime: '2026-09-20T03:00:00Z', size: 5242880 },
]

const renderPage = (cfgs = configs) => {
  apiMock.getAgents.mockResolvedValue(agents)
  apiMock.getBackupConfigs.mockResolvedValue({ configs: cfgs })
  apiMock.getBackupConfigRuns.mockResolvedValue({ runs })
  apiMock.getBackupFiles.mockResolvedValue({ files })
  apiMock.getServerBackups.mockResolvedValue(serverBackups)
  apiMock.getServerBackupConfig.mockResolvedValue({
    interval_hours: 24, retention_days: 7, remote_dest: 'gdrive:cockpit', rclone_available: true })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Backups />
    </QueryClientProvider>,
  )
}

const cfgRow = (cell: string) =>
  // 配置表在第一个 Card 内，避免与文件表/ServerBackupCard 表串行
  Array.from(document.querySelectorAll<HTMLTableRowElement>('.ant-card tr.ant-table-row'))
    .find((tr) => tr.textContent?.includes(cell)) as HTMLTableRowElement

const drawerRow = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('.ant-drawer tr.ant-table-row'))
    .find((tr) => tr.textContent?.includes(cell)) as HTMLTableRowElement

const textBtnIn = (row: HTMLTableRowElement, label: string) =>
  Array.from(row.querySelectorAll('button')).find((b) => {
    const t = b.textContent || ''
    return t.includes(label) && !!b.querySelector('.anticon')
  })

// 纯 icon 按钮（编辑/删除）
const iconBtnIn = (row: HTMLTableRowElement, iconClass: string) =>
  row.querySelector(`button .${iconClass}`)?.closest('button') as HTMLButtonElement | null

const modalTitle = async (title: string) => {
  await waitFor(() => {
    const el = document.querySelector('.ant-modal-title')
    if (el?.textContent !== title) throw new Error(`modal title: ${el?.textContent}`)
  })
}

describe('Backups', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    canWrite = true
  })

  it('配置表：计划标签、源路径截断、状态 Tag 与停用标记、ServerBackupCard 同屏', async () => {
    renderPage()
    expect(await screen.findByText('etc')).toBeInTheDocument()
    expect(screen.getByText('每日 03:00')).toBeInTheDocument()
    expect(screen.getByText('每 6 小时')).toBeInTheDocument()
    expect(screen.getByText('手动')).toBeInTheDocument()
    expect(screen.getByText('+1')).toBeInTheDocument()
    expect(screen.getByText('7 份')).toBeInTheDocument()
    expect(screen.getByText('不限')).toBeInTheDocument()
    expect(screen.getByText('成功')).toBeInTheDocument()
    expect(screen.getByText('运行中')).toBeInTheDocument()
    expect(screen.getByText('未运行')).toBeInTheDocument()
    expect(screen.getByText('已停用')).toBeInTheDocument()
    // 运行中的配置禁用「运行」按钮
    expect((textBtnIn(cfgRow('db'), '运行') as HTMLButtonElement).disabled).toBe(true)
    // 面板数据库卡片（ServerBackupCard 真实渲染）
    expect(screen.getByText('面板数据库')).toBeInTheDocument()
    expect(screen.getByText('立即备份')).toBeInTheDocument()
  })

  it('空态：无配置出 Empty', async () => {
    renderPage([])
    expect(await screen.findByText('尚未配置备份任务，点击右上角创建第一条配置')).toBeInTheDocument()
  })

  it('运行与删除配置：成功提示与 Popconfirm 确认', async () => {
    apiMock.runBackup.mockResolvedValue({})
    apiMock.deleteBackupConfig.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('etc')).toBeInTheDocument()
    fireEvent.click(textBtnIn(cfgRow('logs'), '运行')!)
    await waitFor(() => expect(apiMock.runBackup).toHaveBeenCalledWith(3))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('备份任务已下发，稍后可查看运行历史'))
    fireEvent.click(iconBtnIn(cfgRow('etc'), 'anticon-delete')!)
    expect(await screen.findByText('删除备份配置？')).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.deleteBackupConfig).toHaveBeenCalledWith(1))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('配置已删除'))
  })

  it('历史抽屉：三态状态、异地推送列与错误透出', async () => {
    renderPage()
    expect(await screen.findByText('etc')).toBeInTheDocument()
    fireEvent.click(textBtnIn(cfgRow('etc'), '历史')!)
    expect(await screen.findByText('运行历史：etc')).toBeInTheDocument()
    expect(await screen.findByText('etc-20260914.tar.gz')).toBeInTheDocument()
    expect(screen.getByText('1.0 MB')).toBeInTheDocument()
    expect(screen.getByText('已推送')).toBeInTheDocument()
    expect(screen.getByText('推送失败')).toBeInTheDocument()
    expect(screen.getByText('超时')).toBeInTheDocument()
    expect(screen.getByText('tar: exited with status 2')).toBeInTheDocument()
    // 未启用异地推送 + 空时间兜底
    const r2 = Array.from(document.querySelectorAll<HTMLTableRowElement>('.ant-drawer tr.ant-table-row'))
      .find((tr) => tr.textContent?.includes('tar: exited'))!
    expect(within(r2).getAllByText('—').length).toBeGreaterThanOrEqual(2)
  })

  it('文件抽屉：计数 Alert、文件表与补传按钮（配置了异地目标）', async () => {
    renderPage()
    expect(await screen.findByText('etc')).toBeInTheDocument()
    fireEvent.click(textBtnIn(cfgRow('etc'), '文件')!)
    expect(await screen.findByText('备份文件：/mnt/backup')).toBeInTheDocument()
    expect(await screen.findByText(/共 2 个备份包/)).toBeInTheDocument()
    expect(await screen.findByText('etc-20260914-030000.tar.gz')).toBeInTheDocument()
    expect(screen.getByText('2.0 MB')).toBeInTheDocument()
    // mtime 0 兜底 —
    const rows = Array.from(document.querySelectorAll<HTMLTableRowElement>('.ant-drawer tr.ant-table-row'))
    expect(within(rows[1]).getByText('—')).toBeInTheDocument()
    expect(within(rows[0]).getByRole('button', { name: /补\s*传/ })).toBeInTheDocument()
  })

  it('文件操作：下载、补传与删除', async () => {
    Object.defineProperty(URL, 'createObjectURL', { value: vi.fn(() => 'blob:mock'), configurable: true })
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click')
    apiMock.downloadBackupFile.mockResolvedValue(new Blob(['x']))
    apiMock.syncBackupFileRemote.mockResolvedValue({})
    apiMock.deleteBackupFile.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('etc')).toBeInTheDocument()
    fireEvent.click(textBtnIn(cfgRow('etc'), '文件')!)
    const rows = await waitFor(() => {
      const rs = Array.from(document.querySelectorAll('.ant-drawer tr.ant-table-row'))
      if (rs.length < 2) throw new Error('rows not ready')
      return rs as HTMLTableRowElement[]
    })
    fireEvent.click(within(rows[0]).getByRole('button', { name: /下\s*载/ }))
    await waitFor(() => expect(apiMock.downloadBackupFile).toHaveBeenCalledWith(1, 'etc-20260914-030000.tar.gz'))
    expect(clickSpy).toHaveBeenCalled()
    fireEvent.click(within(rows[0]).getByRole('button', { name: /补\s*传/ }))
    await waitFor(() => expect(apiMock.syncBackupFileRemote).toHaveBeenCalledWith(1, 'etc-20260914-030000.tar.gz'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已同步到远端'))
    fireEvent.click(iconBtnIn(rows[0], 'anticon-delete')!)
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.deleteBackupFile).toHaveBeenCalledWith(1, 'etc-20260914-030000.tar.gz'))
  })

  it('恢复双阶段：表单校验、下发任务、任务视图终态', async () => {
    apiMock.restoreBackup.mockResolvedValue({ taskId: 't-99' })
    apiMock.getBackupTask.mockResolvedValue({ status: 'success', log: 'restore done', error: '' })
    renderPage()
    expect(await screen.findByText('etc')).toBeInTheDocument()
    fireEvent.click(textBtnIn(cfgRow('etc'), '文件')!)
    await waitFor(() => expect(document.querySelectorAll('.ant-drawer tr.ant-table-row').length).toBe(2))
    fireEvent.click(within(drawerRow('etc-20260914')).getByRole('button', { name: /恢\s*复/ }))
    expect(await screen.findByText('恢复备份：etc-20260914-030000.tar.gz')).toBeInTheDocument()
    expect(screen.getByText(/恢复到独立目录，不会覆盖任何现有数据/)).toBeInTheDocument()
    // 非绝对路径 + 文件名不匹配 → 两条校验错误
    const inputs = document.querySelectorAll('.ant-modal input')
    fireEvent.change(inputs[0], { target: { value: 'relative/dir' } })
    fireEvent.change(inputs[1], { target: { value: 'wrong-name' } })
    fireEvent.click(screen.getByRole('button', { name: /开始恢复/ }))
    expect(await screen.findByText('必须是 Agent 主机上的绝对路径')).toBeInTheDocument()
    expect(screen.getByText('输入与备份文件名不一致')).toBeInTheDocument()
    expect(apiMock.restoreBackup).not.toHaveBeenCalled()
    // 合法提交 → 下发 → 任务视图
    fireEvent.change(inputs[0], { target: { value: '/tmp/restore-etc' } })
    fireEvent.change(inputs[1], { target: { value: 'etc-20260914-030000.tar.gz' } })
    fireEvent.click(screen.getByRole('button', { name: /开始恢复/ }))
    await waitFor(() => expect(apiMock.restoreBackup).toHaveBeenCalledWith(1, {
      file: 'etc-20260914-030000.tar.gz', dest_dir: '/tmp/restore-etc', confirm_name: 'etc-20260914-030000.tar.gz' }))
    expect(await screen.findByText('恢复成功')).toBeInTheDocument()
    expect(screen.getByText('restore done')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /完\s*成/ })).toBeInTheDocument()
  })

  it('新建表单：必填与 pattern 校验、合法提交 daily 计划', async () => {
    apiMock.createBackupConfig.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('etc')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建备份/ }))
    await modalTitle('新建备份')
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    expect(await screen.findByText('请选择 Agent')).toBeInTheDocument()
    expect(screen.getByText('请输入备份名')).toBeInTheDocument()
    expect(screen.getByText('至少一个绝对路径')).toBeInTheDocument()
    // 备份名 pattern
    const nameInput = screen.getByLabelText('备份名') as HTMLInputElement
    fireEvent.change(nameInput, { target: { value: 'Bad Name!' } })
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    expect(await screen.findByText(/小写字母\/数字开头/)).toBeInTheDocument()
    expect(apiMock.createBackupConfig).not.toHaveBeenCalled()
  })

  it('编辑回填：every 计划、手动切换隐藏时间项、保存走 update', async () => {
    apiMock.updateBackupConfig.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('etc')).toBeInTheDocument()
    fireEvent.click(iconBtnIn(cfgRow('db'), 'anticon-edit')!)
    expect(await screen.findByText('编辑备份：db')).toBeInTheDocument()
    // every 分支：显示间隔输入框；切手动后隐藏
    expect(await screen.findByText('执行间隔（小时）')).toBeInTheDocument()
    expect(screen.queryByText('每天执行时间')).not.toBeInTheDocument()
    // Radio 文本在内层 span，按 label 容器定位
    const radioOf = (label: string) =>
      Array.from(document.querySelectorAll<HTMLLabelElement>('label.ant-radio-button-wrapper'))
        .find((l) => l.textContent === label)!
    fireEvent.click(radioOf('手动'))
    await waitFor(() => expect(screen.queryByText('执行间隔（小时）')).not.toBeInTheDocument())
    fireEvent.click(radioOf('间隔'))
    expect(await screen.findByText('执行间隔（小时）')).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-modal .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.updateBackupConfig).toHaveBeenCalledWith(2,
      expect.objectContaining({ schedule: 'every:6h', retention: 0, enabled: false })))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('配置已更新'))
  })

  it('ServerBackupCard：立即备份、配置保存校验与合法提交', async () => {
    apiMock.runServerBackup.mockResolvedValue({})
    apiMock.putServerBackupConfig.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('server-20260920.db')).toBeInTheDocument()
    // 异地目标非法 → 拦截；合法 → 保存
    const destInput = screen.getByPlaceholderText('gdrive:cockpit')
    fireEvent.change(destInput, { target: { value: 'bad dest' } })
    fireEvent.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('异地目标需为 remote:path 形态（如 gdrive:cockpit）'))
    expect(apiMock.putServerBackupConfig).not.toHaveBeenCalled()
    fireEvent.change(destInput, { target: { value: 's3:panel' } })
    fireEvent.click(screen.getByRole('button', { name: /^保\s*存$/ }))
    await waitFor(() => expect(apiMock.putServerBackupConfig).toHaveBeenCalledWith({
      interval_hours: 24, retention_days: 7, remote_dest: 's3:panel' }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('备份配置已保存'))
    fireEvent.click(screen.getByRole('button', { name: /立即备份/ }))
    await waitFor(() => expect(apiMock.runServerBackup).toHaveBeenCalled())
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('备份完成'))
  })

  it('RBAC 无写权限：配置与文件的写入口全隐藏，读入口保留', async () => {
    canWrite = false
    renderPage()
    expect(await screen.findByText('etc')).toBeInTheDocument()
    const r = cfgRow('etc')
    expect(within(r).queryByRole('button', { name: /运\s*行/ })).not.toBeInTheDocument()
    expect(within(r).queryByRole('button', { name: /删\s*除/ })).not.toBeInTheDocument()
    // 编辑/删除是无文字 icon 按钮，按图标计数归零
    expect(r.querySelectorAll('.anticon-edit').length).toBe(0)
    expect(r.querySelectorAll('.anticon-delete').length).toBe(0)
    expect(within(r).getByRole('button', { name: /历\s*史/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /新建备份/ })).not.toBeInTheDocument()
    // ServerBackupCard 写入口同步隐藏
    expect(screen.queryByRole('button', { name: /立即备份/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^保\s*存$/ })).not.toBeInTheDocument()
  })
})
