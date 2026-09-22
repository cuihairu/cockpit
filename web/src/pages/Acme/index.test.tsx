import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Acme from './index'
import type { AcmeCertView, Agent } from '@/types'

// Acme：DNS 凭据引导分叉、工具条（巡检开关/账户邮箱）、证书表全列渲染、
// 签发/部署/下载/删除、新建与编辑表单（默认路径回填）、RBAC 分档

const apiMock = vi.hoisted(() => ({
  getAcmeCerts: vi.fn(),
  getAcmeScanConfig: vi.fn(),
  getAcmeAccount: vi.fn(),
  getAgents: vi.fn(),
  putAcmeScanConfig: vi.fn(),
  putAcmeAccount: vi.fn(),
  createAcmeCert: vi.fn(),
  updateAcmeCert: vi.fn(),
  issueAcmeCert: vi.fn(),
  deleteAcmeCert: vi.fn(),
  deployAcmeCert: vi.fn(),
  downloadAcmeCert: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

// acme:admin（签发/部署）与 acme:write（编辑/删除/工具条）分档
const permRef = { admin: true, write: true }
vi.mock('@/hooks/usePerm', () => ({ usePerm: (p: string) => (p === 'acme:admin' ? permRef.admin : permRef.write) }))

const msgSuccess = vi.spyOn(message, 'success')
const msgInfo = vi.spyOn(message, 'info')
const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const agents = [
  { id: 'ag-1', hostname: 'web-01', ip: '10.0.0.1', region: 'cn', zone: 'z1', status: 'online',
    lastSeen: '0', capabilities: [{ type: 'nginx-proxy' }] },
  { id: 'ag-2', hostname: 'db-01', ip: '10.0.0.2', region: 'cn', zone: 'z1', status: 'offline',
    lastSeen: '0', capabilities: [] },
  { id: 'ag-3', hostname: '', ip: '10.0.0.3', region: 'cn', zone: 'z1', status: 'online',
    lastSeen: '0' },
] as unknown as Agent[]

// 到期色阶：now+20d → 橙（15 ≤ 20 < 30），避免长期漂移
const soonIso = new Date(Date.now() + 20 * 86400000 + 3600000).toISOString()

const certs = [
  { id: 1, status: 'issued', primaryDomain: 'a.com', domains: ['a.com', '*.a.com'], caDirectory: 'production',
    autoRenew: true, renewBeforeDays: 30, expiresAt: '2026-12-01', lastRenewAt: 0,
    deployAgentId: 'ag-1', deployCertPath: '/etc/cockpit/certs/a.com.crt.pem', lastDeployAt: 1759000000 },
  { id: 2, status: 'failed', primaryDomain: 'b.io', domains: ['b.io'], caDirectory: 'staging',
    autoRenew: false, renewBeforeDays: 30, expiresAt: '', lastRenewAt: 0, lastError: 'dns txt 校验超时',
    deployAgentId: 'ag-1', deployCertPath: '/x', lastDeployAt: 0, lastDeployError: 'agent 离线' },
  { id: 3, status: 'pending', primaryDomain: 'c.dev', domains: ['c.dev'], caDirectory: 'staging',
    autoRenew: false, renewBeforeDays: 14, expiresAt: '', lastRenewAt: 0 },
  { id: 4, status: 'issued', primaryDomain: 'd.net', domains: ['d.net'], caDirectory: 'staging',
    autoRenew: true, renewBeforeDays: 21, expiresAt: soonIso, lastRenewAt: 0 },
] as unknown as AcmeCertView[]

const renderPage = (scanCfg?: unknown, opts?: { certs?: AcmeCertView[]; account?: unknown }) => {
  apiMock.getAcmeCerts.mockResolvedValue(opts?.certs ?? certs)
  apiMock.getAcmeScanConfig.mockResolvedValue(
    scanCfg ?? { scan_interval_seconds: 7200, dns: { provider: 'dnspod', configured: true } })
  apiMock.getAcmeAccount.mockResolvedValue(opts?.account ?? { email: 'ops@example.com' })
  apiMock.getAgents.mockResolvedValue(agents)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Acme />
    </QueryClientProvider>,
  )
}

const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(cell)) as HTMLTableRowElement

const iconBtnIn = (row: HTMLTableRowElement, iconClass: string) =>
  row.querySelector(`button .${iconClass}`)?.closest('button') as HTMLButtonElement | null

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

const modalCancel = () => {
  const btns = Array.from(document.querySelectorAll<HTMLButtonElement>('.ant-modal-footer button'))
  fireEvent.click(btns.find((b) => !b.classList.contains('ant-btn-primary'))!)
}

// jsdom 不派发 transitionend，弹窗关闭动画停在 leave；用 leave 类断言关闭已发起
const expectModalLeaving = async () => {
  await waitFor(() => expect(document.querySelector('.ant-modal')?.className).toContain('ant-zoom-leave'))
}

const openSelect = (label: string) => {
  const input = screen.getByLabelText(label)
  fireEvent.mouseDown(input.closest('.ant-select-selector')!)
}

const pickOption = async (label: string) => {
  fireEvent.click(await screen.findByText(label))
}

const addDomainTag = (value: string) => {
  const domInput = document.querySelector('.ant-modal .ant-select-selection-search-input') as HTMLInputElement
  fireEvent.change(domInput, { target: { value } })
}

describe('Acme', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    permRef.admin = true
    permRef.write = true
  })

  it('DNS 凭据未配置：按 provider 出引导文案', async () => {
    renderPage({ dns: { provider: 'cloudflare', configured: false } })
    // 引导句拆在多个文本节点，查卡片容器整串
    await waitFor(() => {
      expect(document.querySelector('.ant-card')?.textContent).toContain('ACME DNS-01 验证使用 Cloudflare')
    })
    expect(screen.getByText('dns.cloudflare.api_token')).toBeInTheDocument()
    expect(screen.getByText('CLOUDFLARE_API_TOKEN')).toBeInTheDocument()
    expect(screen.queryByText('新建签发')).not.toBeInTheDocument()
  })

  it('证书表全列：状态徽标、多域名、CA、部署、到期色阶与续期 Tag、失败警示', async () => {
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    // issued 两行（a.com / d.net）各一枚徽标；b.io 徽标+部署失败 Tag 各一枚「失败」
    expect(screen.getAllByText('已签发').length).toBe(2)
    expect(within(rowOf('b.io')).getAllByText('失败').length).toBe(2)
    expect(screen.getByText('待签发')).toBeInTheDocument()
    expect(screen.getByText('共 2 个（含泛域名）')).toBeInTheDocument()
    expect(screen.getByText('正式')).toBeInTheDocument()
    // 部署成功（绿 Tag）与部署失败（红 Tag）
    expect(rowOf('b.io').querySelector('.ant-tag-red')?.textContent).toContain('失败')
    expect(rowOf('a.com').querySelector('.ant-tag-green')?.textContent).toBeTruthy()
    // 到期 20 天 → 橙色「剩 20 天」；空到期兜底 —
    expect(within(rowOf('d.net')).getByText(/剩 20 天/)).toHaveStyle({ color: '#d46b08' })
    expect(within(rowOf('c.dev')).getAllByText('—').length).toBeGreaterThanOrEqual(1)
    expect(within(rowOf('a.com')).getByText('开（30天前）')).toBeInTheDocument()
    expect(within(rowOf('b.io')).getByText('关')).toBeInTheDocument()
    expect(screen.getByText(/有配置最近一次签发失败/)).toBeInTheDocument()
  })

  it('签发与手动部署：成功提示带域名/目标', async () => {
    apiMock.issueAcmeCert.mockResolvedValue({ primaryDomain: 'a.com', expiresAt: '2026-12-01' })
    apiMock.deployAcmeCert.mockResolvedValue({ deployAgentId: 'ag-1' })
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(iconBtnIn(rowOf('c.dev'), 'anticon-cloud-upload')!)
    await waitFor(() => expect(apiMock.issueAcmeCert).toHaveBeenCalledWith(3))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith(expect.stringContaining('签发成功：a.com')))
    fireEvent.click(iconBtnIn(rowOf('a.com'), 'anticon-send')!)
    await waitFor(() => expect(apiMock.deployAcmeCert).toHaveBeenCalledWith(1))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已部署到 ag-1'))
  })

  it('下载证书与私钥：分片参数与审计提示', async () => {
    Object.defineProperty(URL, 'createObjectURL', { value: vi.fn(() => 'blob:mock'), configurable: true })
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click')
    apiMock.downloadAcmeCert.mockResolvedValue('-----BEGIN CERTIFICATE-----')
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(within(rowOf('a.com')).getByRole('button', { name: /证\s*书/ }))
    await waitFor(() => expect(apiMock.downloadAcmeCert).toHaveBeenCalledWith(1, 'cert'))
    fireEvent.click(within(rowOf('a.com')).getByRole('button', { name: /私\s*钥/ }))
    await waitFor(() => expect(apiMock.downloadAcmeCert).toHaveBeenCalledWith(1, 'key'))
    expect(msgInfo).toHaveBeenCalledWith('私钥已下载（该操作已记入审计日志）')
    expect(clickSpy).toHaveBeenCalledTimes(2)
  })

  it('删除：Popconfirm 确认后调用', async () => {
    apiMock.deleteAcmeCert.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(iconBtnIn(rowOf('a.com'), 'anticon-delete')!)
    expect(await screen.findByText(/删除该签发配置/)).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(apiMock.deleteAcmeCert).toHaveBeenCalledWith(1))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已删除（不影响 CA 侧已签发的证书）'))
  })

  it('编辑：回填、按主域名填默认路径、保存走 update', async () => {
    apiMock.updateAcmeCert.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(iconBtnIn(rowOf('a.com'), 'anticon-edit')!)
    await modalTitle('编辑签发配置')
    expect(fieldEl('证书路径（agent 上）').value).toBe('/etc/cockpit/certs/a.com.crt.pem')
    fireEvent.click(screen.getByRole('button', { name: /按主域名填默认路径/ }))
    expect(fieldEl('证书路径（agent 上）').value).toBe('/etc/cockpit/certs/a.com.crt.pem')
    expect(fieldEl('私钥路径（agent 上，落盘权限 0600）').value).toBe('/etc/cockpit/certs/a.com.key.pem')
    fireEvent.change(fieldEl('提前续期（天）'), { target: { value: '14' } })
    modalOk()
    await waitFor(() => expect(apiMock.updateAcmeCert).toHaveBeenCalledWith(1,
      expect.objectContaining({ domains: ['a.com', '*.a.com'], caDirectory: 'production', renewBeforeDays: 14 })))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已保存（域名/CA 变更后需重新签发）'))
  })

  it('新建：空域名校验拦截，合法提交走 create 并小写化', async () => {
    apiMock.createAcmeCert.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建签发/ }))
    await modalTitle('新建签发配置')
    modalOk()
    expect(await screen.findByText('请填写至少一个域名')).toBeInTheDocument()
    expect(apiMock.createAcmeCert).not.toHaveBeenCalled()
    // tags 模式：逗号 tokenSeparator 落 token（tag 显示原文，小写化在 submit）
    const domInput = document.querySelector('.ant-modal .ant-select-selection-search-input') as HTMLInputElement
    fireEvent.change(domInput, { target: { value: 'New.Example.COM,' } })
    expect(await screen.findByText('New.Example.COM', { selector: '.ant-select-selection-item-content' })).toBeInTheDocument()
    modalOk()
    await waitFor(() => expect(apiMock.createAcmeCert).toHaveBeenCalledWith(
      expect.objectContaining({ domains: ['new.example.com'], caDirectory: 'staging', autoRenew: true, renewBeforeDays: 30 })))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已创建，点「签发」获取证书'))
  })

  it('巡检保存：调档开启与关停两分支', async () => {
    apiMock.putAcmeScanConfig.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    // 初始 7200s → 120 分钟；手输 10 → put(600)。越界输入被 min/max 拦截不可达
    const minutesInput = document.querySelector('.ant-input-number-input') as HTMLInputElement
    expect(minutesInput.value).toBe('120')
    // loading 期间按钮 disabled 且 name 带 loading 前缀，等空闲再点击
    const clickSave = async () => {
      await waitFor(() => {
        const b = screen.getByRole('button', { name: /保\s*存/ }) as HTMLButtonElement
        if (b.disabled) throw new Error('save button busy')
      })
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    }
    fireEvent.change(minutesInput, { target: { value: '10' } })
    await clickSave()
    await waitFor(() => expect(apiMock.putAcmeScanConfig).toHaveBeenCalledWith(600))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已开启自动续期'))
    // 关闭开关后保存 → 0
    fireEvent.click(document.querySelector('.ant-switch')!)
    await clickSave()
    await waitFor(() => expect(apiMock.putAcmeScanConfig).toHaveBeenLastCalledWith(0))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已关闭自动续期'))
  })

  it('账户邮箱：回填、格式校验与保存', async () => {
    apiMock.putAcmeAccount.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /账户邮箱：ops@example.com/ }))
    await modalTitle('ACME 账户邮箱')
    expect(fieldEl('邮箱').value).toBe('ops@example.com')
    fireEvent.change(fieldEl('邮箱'), { target: { value: 'bad' } })
    modalOk()
    expect(await screen.findByText('邮箱格式不正确')).toBeInTheDocument()
    fireEvent.change(fieldEl('邮箱'), { target: { value: 'new@example.com' } })
    modalOk()
    await waitFor(() => expect(apiMock.putAcmeAccount).toHaveBeenCalledWith('new@example.com'))
    await waitFor(() => expect(msgSuccess).toHaveBeenCalledWith('已保存'))
  })

  it('RBAC 双档全关：签发/部署/编辑/删除与工具条入口隐藏，下载保留', async () => {
    permRef.admin = false
    permRef.write = false
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    const r1 = rowOf('a.com')
    expect(iconBtnIn(r1, 'anticon-cloud-upload')).toBeFalsy()
    expect(iconBtnIn(r1, 'anticon-send')).toBeFalsy()
    expect(iconBtnIn(r1, 'anticon-edit')).toBeFalsy()
    expect(iconBtnIn(r1, 'anticon-delete')).toBeFalsy()
    expect(screen.queryByRole('button', { name: /新建签发/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^保\s*存$/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /账户邮箱/ })).not.toBeInTheDocument()
    // 下载不在 PermGuard 内，issued 行保留
    expect(within(r1).getByRole('button', { name: /私\s*钥/ })).toBeInTheDocument()
  })

  it('巡检间隔越界拦截，改合法后保存失败走 message.error', async () => {
    apiMock.putAcmeScanConfig.mockRejectedValue(new Error('busy'))
    renderPage({ scan_interval_seconds: 120, dns: { provider: 'dnspod', configured: true } })
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    const minutesInput = document.querySelector('.ant-input-number-input') as HTMLInputElement
    expect(minutesInput.value).toBe('2')
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('间隔需在 5～1440 分钟之间'))
    expect(apiMock.putAcmeScanConfig).not.toHaveBeenCalled()
    fireEvent.change(minutesInput, { target: { value: '10' } })
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.putAcmeScanConfig).toHaveBeenCalledWith(600))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
  })

  it('清空分钟输入：onChange 回退默认 5 分钟', async () => {
    apiMock.putAcmeScanConfig.mockResolvedValue({})
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    const minutesInput = document.querySelector('.ant-input-number-input') as HTMLInputElement
    expect(minutesInput.value).toBe('120')
    fireEvent.change(minutesInput, { target: { value: '' } })
    expect(minutesInput.value).toBe('5')
    await waitFor(() => {
      const b = screen.getByRole('button', { name: /保\s*存/ }) as HTMLButtonElement
      if (b.disabled) throw new Error('save button busy')
    })
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(apiMock.putAcmeScanConfig).toHaveBeenCalledWith(300))
  })

  it('账户邮箱未设：按钮文案「未设置」、空回填、保存失败与取消', async () => {
    apiMock.putAcmeAccount.mockRejectedValue(new Error('bad mail'))
    renderPage(undefined, { account: {} })
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /账户邮箱未设置/ }))
    await modalTitle('ACME 账户邮箱')
    expect(fieldEl('邮箱').value).toBe('')
    fireEvent.change(fieldEl('邮箱'), { target: { value: 'ops@example.com' } })
    modalOk()
    await waitFor(() => expect(apiMock.putAcmeAccount).toHaveBeenCalledWith('ops@example.com'))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
    modalCancel()
    await expectModalLeaving()
  })

  it('新建取消与编辑无部署字段证书', async () => {
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建签发/ }))
    await modalTitle('新建签发配置')
    modalCancel()
    await expectModalLeaving()
    fireEvent.click(iconBtnIn(rowOf('c.dev'), 'anticon-edit')!)
    await modalTitle('编辑签发配置')
    expect(fieldEl('域名（首个为主域名）').id).toBeTruthy()
    expect(screen.queryByLabelText('证书路径（agent 上）')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('私钥路径（agent 上，落盘权限 0600）')).not.toBeInTheDocument()
  })

  it('新建表单：部署目标选项兜底与空域名默认路径', async () => {
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建签发/ }))
    await modalTitle('新建签发配置')
    openSelect('自动部署目标（可选）')
    expect(await screen.findByText('web-01（Nginx）')).toBeInTheDocument()
    expect(screen.getByText('db-01（离线）')).toBeInTheDocument()
    expect(screen.getByText('ag-3')).toBeInTheDocument()
    await pickOption('web-01（Nginx）')
    await screen.findByLabelText('证书路径（agent 上）')
    fireEvent.click(screen.getByRole('button', { name: /按主域名填默认路径/ }))
    expect(fieldEl('证书路径（agent 上）').value).toBe('/etc/cockpit/certs/example.com.crt.pem')
    expect(fieldEl('私钥路径（agent 上，落盘权限 0600）').value).toBe('/etc/cockpit/certs/example.com.key.pem')
  })

  it('仅空白域名：提交被拦截提示且不调接口', async () => {
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建签发/ }))
    await modalTitle('新建签发配置')
    addDomainTag('\t,')
    expect(document.querySelector('.ant-modal .ant-select-selection-item')).toBeTruthy()
    modalOk()
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('请至少填写一个域名'))
    expect(apiMock.createAcmeCert).not.toHaveBeenCalled()
  })

  it('新建保存失败：message.error', async () => {
    apiMock.createAcmeCert.mockRejectedValue(new Error('dup'))
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新建签发/ }))
    await modalTitle('新建签发配置')
    addDomainTag('x.com,')
    modalOk()
    await waitFor(() => expect(apiMock.createAcmeCert).toHaveBeenCalled())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
  })

  it('到期红色/待推送/未知状态兜底/最后签发时间', async () => {
    const redIso = new Date(Date.now() + 10 * 86400000 + 3600000).toISOString()
    const edgeCerts = [
      { id: 11, status: 'issued', primaryDomain: 'e.org', domains: ['e.org'], caDirectory: 'staging',
        autoRenew: true, renewBeforeDays: 7, expiresAt: redIso, lastRenewAt: 1759000000,
        deployAgentId: 'ag-1', deployCertPath: '/e.crt.pem', lastDeployAt: 0 },
      { id: 12, status: 'oops', primaryDomain: 'g.dev', domains: ['g.dev'], caDirectory: 'staging',
        autoRenew: false, renewBeforeDays: 30, expiresAt: '', lastRenewAt: 0 },
    ] as unknown as AcmeCertView[]
    renderPage(undefined, { certs: edgeCerts })
    expect(await screen.findByText('e.org')).toBeInTheDocument()
    expect(within(rowOf('e.org')).getByText('待推送')).toBeInTheDocument()
    expect(within(rowOf('e.org')).getByText(/剩 10 天/)).toHaveStyle({ color: '#cf1322' })
    expect(within(rowOf('e.org')).getByText(new Date(1759000000 * 1000).toLocaleString())).toBeInTheDocument()
    expect(within(rowOf('g.dev')).getByText('待签发')).toBeInTheDocument()
    expect(screen.queryByText(/有配置最近一次签发失败/)).not.toBeInTheDocument()
  })

  it('签发失败与部署失败：message.error', async () => {
    apiMock.issueAcmeCert.mockRejectedValue(new Error('ca down'))
    apiMock.deployAcmeCert.mockRejectedValue(new Error('agent down'))
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(iconBtnIn(rowOf('c.dev'), 'anticon-cloud-upload')!)
    await waitFor(() => expect(apiMock.issueAcmeCert).toHaveBeenCalledWith(3))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('签发失败'))
    fireEvent.click(iconBtnIn(rowOf('a.com'), 'anticon-send')!)
    await waitFor(() => expect(apiMock.deployAcmeCert).toHaveBeenCalledWith(1))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('部署失败'))
  })

  it('下载失败与删除失败：message.error', async () => {
    Object.defineProperty(URL, 'createObjectURL', { value: vi.fn(() => 'blob:mock'), configurable: true })
    apiMock.downloadAcmeCert.mockRejectedValue(new Error('net'))
    apiMock.deleteAcmeCert.mockRejectedValue(new Error('ref'))
    renderPage()
    expect(await screen.findByText('a.com')).toBeInTheDocument()
    fireEvent.click(within(rowOf('a.com')).getByRole('button', { name: /证\s*书/ }))
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('下载失败'))
    fireEvent.click(iconBtnIn(rowOf('a.com'), 'anticon-delete')!)
    expect(await screen.findByText(/删除该签发配置/)).toBeInTheDocument()
    fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('删除失败'))
  })
})
