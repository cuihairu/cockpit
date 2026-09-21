import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Settings from './index'
import type { UserInfo } from '@/types'

// Settings：Tab 级 RBAC 裁剪、通用设置双表单回填与保存、TOTP 状态与禁用流程
// （含验证码过滤/位数拦截）、getCurrentUser 失败降级 localStorage、
// 拨测表单回填校验与保存、通知渠道四态（空/全成功/部分失败/503）、系统信息

const apiMock = vi.hoisted(() => ({
  getCurrentUser: vi.fn(),
  saveSettings: vi.fn(),
  disableTOTP: vi.fn(),
  getProbeConfig: vi.fn(),
  saveProbeConfig: vi.fn(),
  getNotificationStatus: vi.fn(),
  testNotification: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

let admin = true
vi.mock('@/hooks/usePerm', () => ({ usePerm: (p: string) => (p === 'settings:admin' ? admin : true) }))

const updateSettings = vi.hoisted(() => vi.fn())
const settingsBase = vi.hoisted(() => ({
  siteName: 'Cockpit',
  refreshInterval: 30,
  enableNotifications: true,
  theme: 'light',
  compactMode: false,
  showResourceCount: true,
}))
vi.mock('@/contexts/useSettingsContext', () => ({
  useSettingsContext: () => ({ settings: settingsBase, updateSettings }),
}))

const navigate = vi.hoisted(() => vi.fn())
vi.mock('react-router-dom', () => ({ useNavigate: () => navigate }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const probeConfig = {
  interval_seconds: 300, fail_threshold: 2, disk_percent: 85, memory_percent: 80,
  cert_warn_days: 14, cert_info_days: 30,
  min_interval_seconds: 30, max_interval_seconds: 3600,
  min_fail_threshold: 1, max_fail_threshold: 10, min_percent: 50, max_percent: 99,
}

const notifStatus = {
  enabled: true,
  channels: [
    { channel: 'herald', target: 'https://h.example.com/x' },
    { channel: 'ntfy', target: 'ntfy.example.com/topic' },
    { channel: 'webhook', target: 'https://hooks.example/w' },
    { channel: 'telegram', target: '@ops' },
  ],
  events: [
    { type: 'service.down', enabled: true },
    { type: 'service.up', enabled: false },
  ],
}

const mkUser = (over: Partial<UserInfo> = {}): UserInfo =>
  ({ id: 'u1', username: 'ops', role: 'admin', permissions: [], totp_enabled: false, ...over })

const renderPage = (user: UserInfo | 'reject' = mkUser(), notifOverride?: unknown) => {
  if (user === 'reject') apiMock.getCurrentUser.mockRejectedValue(new Error('net down'))
  else apiMock.getCurrentUser.mockResolvedValue(user)
  apiMock.getProbeConfig.mockResolvedValue(probeConfig)
  apiMock.getNotificationStatus.mockResolvedValue(notifOverride ?? notifStatus)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Settings />
    </QueryClientProvider>,
  )
}

// Form label 关联的可能是 input 本身或容器
const fieldEl = (label: string, tag = 'input') => {
  const el = screen.getByLabelText(label)
  return (el.matches(tag) ? el : el.querySelector(tag)!) as HTMLInputElement
}

// antd Tabs 懒渲染：安全面板内容须激活后可见
const openSecurity = async () => {
  fireEvent.click(screen.getByRole('tab', { name: '安全设置' }))
  await screen.findByText('关于二次验证')
}

// 拨测表单 layout=inline 且嵌套 noStyle，label 未关联控件，按 Form.Item label 容器取
const probeField = (label: string) =>
  Array.from(document.querySelectorAll('.ant-form-item'))
    .find((f) => f.querySelector('label')?.textContent === label)!
    .querySelector('.ant-input-number-input') as HTMLInputElement

describe('Settings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    admin = true
    localStorage.clear()
  })

  it('通用设置：四 Tab 就位、基础表单回填与保存联动 context', async () => {
    apiMock.saveSettings.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('站点名称')).toBeInTheDocument()
    for (const t of ['通用设置', '安全设置', '告警设置', '系统信息']) {
      expect(screen.getByRole('tab', { name: t })).toBeInTheDocument()
    }
    expect((fieldEl('站点名称') as HTMLInputElement).value).toBe('Cockpit')
    expect(fieldEl('数据刷新间隔 (秒)').value).toBe('30')
    fireEvent.change(fieldEl('站点名称'), { target: { value: 'My Panel' } })
    // icon 拼进 accessible name，且 antd 对两字按钮插空格
    fireEvent.click(screen.getByRole('button', { name: /保\s*存设置$/ }))
    await waitFor(() => expect(apiMock.saveSettings).toHaveBeenCalledWith({
      siteName: 'My Panel', refreshInterval: 30, enableNotifications: true }))
    await waitFor(() => expect(updateSettings).toHaveBeenCalled())
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('设置已保存'))
  })

  it('显示设置：主题切换、开关保存与保存失败透出', async () => {
    apiMock.saveSettings.mockRejectedValueOnce(new Error('boom')).mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('站点名称')).toBeInTheDocument()
    fireEvent.click(screen.getByText('显示设置'))
    // 主题 Select：mouseDown 须打在 .ant-select-selector（option 文本在 -content 子元素）
    fireEvent.mouseDown(screen.getByLabelText('主题').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    fireEvent.click(await screen.findByText('深色', { selector: '.ant-select-item-option-content' }))
    expect(screen.getByText('深色', { selector: '.ant-select-selection-item' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /保\s*存设置$/ }))
    // 普通 Error 无 response.data.error → fallback 文案
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存设置失败'))
    fireEvent.click(screen.getByRole('button', { name: /保\s*存设置$/ }))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('设置已保存'))
  })

  it('安全设置：TOTP 未启用出引导与认证器链接、立即启用跳转', async () => {
    renderPage()
    await openSecurity()
    expect(await screen.findByText('TOTP 未启用')).toBeInTheDocument()
    expect(screen.getByText('建议启用 TOTP 以保护账户安全')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Google Authenticator \(iOS\)/ })).toHaveAttribute(
      'href', 'https://apps.apple.com/app/google-authenticator/id388497605')
    fireEvent.click(screen.getByRole('button', { name: /立即启用/ }))
    expect(navigate).toHaveBeenCalledWith('/settings/setup-totp')
  })

  it('安全设置：已启用状态、禁用确认后进 Modal 流程（过滤/位数/成功）', async () => {
    apiMock.disableTOTP.mockResolvedValue({})
    renderPage(mkUser({ totp_enabled: true, totp_setup_at: '2026-01-15T08:00:00Z' }))
    await openSecurity()
    expect(await screen.findByText('TOTP 已启用')).toBeInTheDocument()
    expect(screen.getByText(/启用时间:/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /禁用 TOTP/ }))
    expect(await screen.findByText('禁用后账户安全性会降低，确定要继续吗？')).toBeInTheDocument()
    // Popconfirm 确认 → 打开验证码 Modal
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    // Popconfirm title 与 Modal title 同文案，按 Modal 标题限定
    expect(await screen.findByText('禁用二次验证', { selector: '.ant-modal-title' })).toBeInTheDocument()
    const codeInput = screen.getByPlaceholderText('123456')
    // 空码确认 → 拦截
    fireEvent.click(screen.getByRole('button', { name: /确认禁用/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('请输入 6 位验证码'))
    // 非数字被过滤 + 位数不足
    fireEvent.change(codeInput, { target: { value: '12a4' } })
    expect(codeInput).toHaveValue('124')
    fireEvent.click(screen.getByRole('button', { name: /确认禁用/ }))
    await waitFor(() => expect(apiMock.disableTOTP).not.toHaveBeenCalled())
    // 6 位 → 成功并关闭
    fireEvent.change(codeInput, { target: { value: '124567' } })
    fireEvent.click(screen.getByRole('button', { name: /确认禁用/ }))
    await waitFor(() => expect(apiMock.disableTOTP).toHaveBeenCalledWith('124567'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('TOTP 已禁用'))
    // 成功后 userInfo 更新：面板翻转为未启用态（行为断言，Modal 关闭动效在 jsdom 不可靠）
    await waitFor(() => expect(screen.getByText('TOTP 未启用')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /立即启用/ })).toBeInTheDocument()
  })

  it('禁用 TOTP 失败：错误透出且 Modal 保留', async () => {
    apiMock.disableTOTP.mockRejectedValue(new Error('code mismatch'))
    renderPage(mkUser({ totp_enabled: true }))
    await openSecurity()
    expect(await screen.findByText('TOTP 已启用')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /禁用 TOTP/ }))
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    const codeInput = await screen.findByPlaceholderText('123456')
    fireEvent.change(codeInput, { target: { value: '000000' } })
    fireEvent.click(screen.getByRole('button', { name: /确认禁用/ }))
    // 普通 Error → getApiErrorMessage fallback
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('操作失败'))
    expect(screen.getByText('请输入当前 TOTP 验证码以确认禁用：')).toBeInTheDocument()
  })

  it('getCurrentUser 失败：降级 localStorage 用户名', async () => {
    localStorage.setItem('username', 'fallback-user')
    localStorage.setItem('role', 'admin')
    renderPage('reject')
    await openSecurity()
    // 降级用户 totp_enabled:false → 未启用面板
    expect(await screen.findByText('TOTP 未启用')).toBeInTheDocument()
  })

  it('拨测表单：回填、跨字段校验与保存全量字段', async () => {
    apiMock.saveProbeConfig.mockResolvedValue({ ...probeConfig, interval_seconds: 120 })
    renderPage()
    fireEvent.click(screen.getByRole('tab', { name: '告警设置' }))
    await waitFor(() => expect(probeField('证书提醒').value).toBe('30'))
    // 提醒天数 < 警告天数 → validator 拦截
    fireEvent.change(probeField('证书提醒'), { target: { value: '7' } })
    fireEvent.click(screen.getByRole('button', { name: /保\s*存$/ }))
    expect(await screen.findByText('提醒天数需不小于警告天数')).toBeInTheDocument()
    expect(apiMock.saveProbeConfig).not.toHaveBeenCalled()
    // 恢复提醒天数并改间隔后保存
    fireEvent.change(probeField('证书提醒'), { target: { value: '30' } })
    fireEvent.change(probeField('探测间隔'), { target: { value: '120' } })
    fireEvent.click(screen.getByRole('button', { name: /保\s*存$/ }))
    await waitFor(() => expect(apiMock.saveProbeConfig).toHaveBeenCalledWith(
      expect.objectContaining({ interval_seconds: 120, cert_info_days: 30, fail_threshold: 2 })))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith(
      '拨测配置已保存（间隔 120 秒、失败阈值 2 次），下一轮探测生效'))
  })

  it('通知渠道：四渠道表、事件开关 Tag 与测试全成功', async () => {
    apiMock.testNotification.mockResolvedValue({
      results: notifStatus.channels.map((c) => ({ ...c, ok: true })),
    })
    renderPage()
    fireEvent.click(screen.getByRole('tab', { name: '告警设置' }))
    expect(await screen.findByText('https://h.example.com/x')).toBeInTheDocument()
    expect(screen.getByText('Herald')).toBeInTheDocument()
    expect(screen.getByText('Telegram')).toBeInTheDocument()
    expect(screen.getByText('service.down 开')).toBeInTheDocument()
    expect(screen.getByText('service.up 关')).toBeInTheDocument()
    expect(screen.getByText(/未列出的事件类型默认不发送/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /发送测试通知/ }))
    await waitFor(() => expect(apiMock.testNotification).toHaveBeenCalled())
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('测试通知已发送到全部渠道'))
    // 结果 Descriptions：四条全「发送成功」
    expect(await waitFor(() => screen.getAllByText('发送成功').length)).toBe(4)
  })

  it('测试通知部分失败与 503 未启用分支', async () => {
    apiMock.testNotification
      .mockResolvedValueOnce({
        results: [
          { channel: 'herald', target: 'https://h.example.com/x', ok: true },
          { channel: 'ntfy', target: 'ntfy.example.com/topic', ok: false, error: 'dial timeout' },
        ],
      })
      .mockRejectedValueOnce(Object.assign(new Error('x'), {
        isAxiosError: true, response: { status: 503, data: '' } }))
    renderPage()
    fireEvent.click(screen.getByRole('tab', { name: '告警设置' }))
    await screen.findByText('https://h.example.com/x')
    fireEvent.click(screen.getByRole('button', { name: /发送测试通知/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('部分渠道发送失败，详见下方结果'))
    expect(await screen.findByText('dial timeout')).toBeInTheDocument()
    expect(screen.getByText('发送成功')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /发送测试通知/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('通知未启用：请在服务端 config.yaml 的 notification 段配置渠道'))
  })

  it('通知渠道为空：引导 Alert 与测试按钮禁用', async () => {
    renderPage(mkUser(), { enabled: false, channels: [], events: [] })
    fireEvent.click(screen.getByRole('tab', { name: '告警设置' }))
    expect(await screen.findByText('尚未配置通知渠道')).toBeInTheDocument()
    expect(screen.getByText(/在服务端 config.yaml 的/)).toBeInTheDocument()
    const btn = screen.getByRole('button', { name: /发送测试通知/ }) as HTMLButtonElement
    expect(btn.disabled).toBe(true)
  })

  it('系统信息：版本描述与统计卡片；非 admin 仅两个 Tab', async () => {
    const view = renderPage()
    fireEvent.click(screen.getByRole('tab', { name: '系统信息' }))
    expect(await screen.findByText('v0.1.0')).toBeInTheDocument()
    expect(screen.getByText('SQLite3')).toBeInTheDocument()
    fireEvent.click(screen.getByText('数据统计'))
    expect(screen.getByText('计算实例')).toBeInTheDocument()
    // RBAC：通用/告警裁剪，安全/系统保留
    admin = false
    view.unmount()
    renderPage()
    await screen.findByText('系统设置')
    expect(screen.queryByRole('tab', { name: '通用设置' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: '告警设置' })).not.toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '安全设置' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '系统信息' })).toBeInTheDocument()
  })
})
