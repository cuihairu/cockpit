import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import DomainsPage from './index'
import type { Agent, DomainBinding } from '@/types'

// Domains：绑定列表（自动化 Tag/apply 状态）+ 登记/编辑表单校验与保存 /
// apply 三分支与步骤弹窗 + 漂移检查 Tag 分支 + 片段弹窗

const apiMock = vi.hoisted(() => ({
  getDomainBindings: vi.fn(),
  saveDomainBinding: vi.fn(),
  applyDomainBinding: vi.fn(),
  deleteDomainBinding: vi.fn(),
  getDomainDrift: vi.fn(),
  getAgentDomainsSnippet: vi.fn(),
  getAgents: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => true }))

const msgSuccess = vi.spyOn(message, 'success')
const msgWarning = vi.spyOn(message, 'warning')
const msgError = vi.spyOn(message, 'error')

const agents = [
  { id: 'ag-1', hostname: 'web-01', ip: '1.2.3.4', status: 'online', lastSeen: '0', capabilities: [] },
] as unknown as Agent[]

const mkBinding = (over: Partial<DomainBinding>): DomainBinding =>
  ({
    id: 1,
    domain: 'blog.example.com',
    agentId: 'ag-1',
    target: '127.0.0.1:8080',
    enabled: true,
    autoDns: true,
    autoProxy: true,
    autoCert: true,
    lastApplyStatus: 'never',
    lastError: '',
    appliedAt: 0,
    createdAt: '',
    updatedAt: '',
    ...over,
  }) as DomainBinding

const bindings = [
  mkBinding({ id: 1, domain: 'blog.example.com' }),
  mkBinding({
    id: 2,
    domain: 'api.example.com',
    enabled: false,
    autoDns: false,
    autoCert: false,
    lastApplyStatus: 'ok',
    appliedAt: 1758000000,
  }),
  mkBinding({
    id: 3,
    domain: 'down.example.com',
    lastApplyStatus: 'failed',
    lastError: 'agent 离线',
    appliedAt: 1758000100,
    autoProxy: false,
    autoCert: false,
  }),
]

const renderPage = (data = bindings) => {
  apiMock.getDomainBindings.mockResolvedValue(data)
  apiMock.getAgents.mockResolvedValue(agents)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <DomainsPage />
    </QueryClientProvider>,
  )
}

const rowOf = (domain: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(domain)) as HTMLTableRowElement

const modalOk = async () => {
  await act(async () => {
    fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn-primary')!)
  })
}

describe('Domains', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('列表：自动化 Tag、apply 状态与 Agent 名映射', async () => {
    renderPage()
    expect(await screen.findByText('blog.example.com')).toBeInTheDocument()
    const r1 = rowOf('blog.example.com')
    expect(within(r1).getByText('DNS')).toBeInTheDocument()
    expect(within(r1).getByText('反代')).toBeInTheDocument()
    expect(within(r1).getByText('证书')).toBeInTheDocument()
    expect(within(r1).getByText('从未')).toBeInTheDocument()
    // Agent 列映射主机名而非 id
    expect(within(r1).getByText('web-01')).toBeInTheDocument()
    // 停用绑定：停用 Tag + 成功时间；失败绑定：失败 Tag
    const r2 = rowOf('api.example.com')
    expect(within(r2).getByText('停用')).toBeInTheDocument()
    expect(within(r2).getByText(/成功·/)).toBeInTheDocument()
    expect(within(r2).queryByText('DNS')).toBeNull()
    const r3 = rowOf('down.example.com')
    expect(within(r3).getByText(/失败·/)).toBeInTheDocument()
  })

  it('登记：必填与格式校验、合法 payload 保存', async () => {
    apiMock.saveDomainBinding.mockResolvedValue({})
    renderPage()
    await screen.findByText('blog.example.com')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /登记绑定/ }))
    })
    await modalOk()
    // 域名/Agent/目标必填（required 无 message → antd 默认英文，锚定错误结构）
    await waitFor(() =>
      expect(document.querySelectorAll('.ant-form-item-explain-error').length).toBeGreaterThanOrEqual(2))
    // 目标格式：非法 → host:port 提示
    fireEvent.change(screen.getByPlaceholderText('blog.example.com'), { target: { value: 'blog.example.com' } })
    fireEvent.change(screen.getByPlaceholderText('127.0.0.1:8080 或 docker://web'), { target: { value: 'not valid' } })
    // Modal 内的 Agent Select（卡片 extra 的片段下拉在 DOM 更前）
    fireEvent.mouseDown(document.querySelector('.ant-modal .ant-select-selector')!)
    const opt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent === 'web-01')
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(opt)
    await modalOk()
    expect(await screen.findByText('host:port 或 docker://服务名[:端口]')).toBeInTheDocument()
    // 改合法 docker 形态 → 保存
    fireEvent.change(screen.getByPlaceholderText('127.0.0.1:8080 或 docker://web'), { target: { value: 'docker://web:8080' } })
    await modalOk()
    await waitFor(() =>
      expect(apiMock.saveDomainBinding).toHaveBeenCalledWith(
        expect.objectContaining({ domain: 'blog.example.com', agentId: 'ag-1', target: 'docker://web:8080' })))
    expect(msgSuccess).toHaveBeenCalledWith('绑定已登记')
  })

  it('编辑：回填且域名禁用，保存走更新提示', async () => {
    apiMock.saveDomainBinding.mockResolvedValue({})
    renderPage()
    await screen.findByText('blog.example.com')
    await act(async () => {
      fireEvent.click(within(rowOf('blog.example.com')).getByRole('button', { name: /编辑/ }))
    })
    expect(await screen.findByText('编辑绑定 · blog.example.com')).toBeInTheDocument()
    const domainInput = screen.getByPlaceholderText('blog.example.com') as HTMLInputElement
    expect(domainInput.disabled).toBe(true)
    expect(domainInput.value).toBe('blog.example.com')
    await modalOk()
    await waitFor(() => expect(apiMock.saveDomainBinding).toHaveBeenCalled())
    expect(msgSuccess).toHaveBeenCalledWith('绑定已更新')
  })

  it('apply：ok 成功提示与步骤弹窗；部分失败 warning', async () => {
    apiMock.applyDomainBinding.mockResolvedValue({
      status: 'ok',
      steps: [{ name: 'dns', ok: true }, { name: 'proxy', ok: true }],
    })
    renderPage()
    await screen.findByText('blog.example.com')
    await act(async () => {
      fireEvent.click(within(rowOf('blog.example.com')).getByRole('button', { name: /Apply/ }))
    })
    expect(msgSuccess).toHaveBeenCalledWith('联动完成')
    expect(await screen.findByText('apply 步骤明细')).toBeInTheDocument()
    expect(screen.getByText('dns')).toBeInTheDocument()
    // 部分失败：失败步骤带 error、warning 提示
    apiMock.applyDomainBinding.mockResolvedValue({
      status: 'partial',
      steps: [{ name: 'cert', ok: false, error: 'ca 超时' }, { name: 'dns', ok: true }],
    })
    fireEvent.click(document.querySelector('.ant-modal-close')!)
    await act(async () => {
      fireEvent.click(within(rowOf('blog.example.com')).getByRole('button', { name: /Apply/ }))
    })
    expect(await screen.findByText(/ca 超时/)).toBeInTheDocument()
    expect(msgWarning).toHaveBeenCalledWith('联动部分失败，详见步骤明细')
  })

  it('apply 空步骤提示未启用联动；接口失败 fallback', async () => {
    apiMock.applyDomainBinding.mockResolvedValue({ status: 'ok', steps: [] })
    renderPage()
    await screen.findByText('blog.example.com')
    await act(async () => {
      fireEvent.click(within(rowOf('blog.example.com')).getByRole('button', { name: /Apply/ }))
    })
    expect(await screen.findByText('未启用任何 auto 联动开关')).toBeInTheDocument()
    apiMock.applyDomainBinding.mockRejectedValue(new Error('boom'))
    fireEvent.click(document.querySelector('.ant-modal-close')!)
    await act(async () => {
      fireEvent.click(within(rowOf('blog.example.com')).getByRole('button', { name: /Apply/ }))
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('联动失败'))
  })

  it('漂移检查：成功后 Tag 四分支渲染', async () => {
    apiMock.getDomainDrift.mockResolvedValue({
      checkedAt: 1,
      items: [
        {
          domain: 'blog.example.com',
          enabled: true,
          dns: { checked: true, ok: true },
          proxy: { checked: true, ok: false, status: 'mismatch', expected: 'a', actual: 'b' },
          cert: { checked: false, ok: false },
        },
        {
          domain: 'api.example.com',
          enabled: true,
          dns: { checked: true, ok: false, error: 'provider 429' },
          proxy: { checked: true, ok: true },
          cert: { checked: true, ok: false, status: 'missing' },
        },
      ],
    })
    renderPage()
    await screen.findByText('blog.example.com')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /漂移检查/ }))
    })
    expect(msgSuccess).toHaveBeenCalledWith('漂移检查完成（2 条绑定）')
    const r1 = rowOf('blog.example.com')
    expect(within(r1).getByText('反代·不符')).toBeInTheDocument()
    expect(within(r1).getByText('证书·未启用')).toBeInTheDocument()
    const r2 = rowOf('api.example.com')
    expect(within(r2).getByText('DNS·异常')).toBeInTheDocument()
    expect(within(r2).getByText('证书·缺失')).toBeInTheDocument()
    // ok 分支无后缀：api 行自动化列也有「反代」Tag，按 success 色锚定漂移 Tag
    expect(within(r2).getByText(
      (_, el) => el?.classList.contains('ant-tag-success') === true && el.textContent === '反代',
    )).toBeInTheDocument()
    // 未包含在检查结果里的行保持「未检查」
    expect(within(rowOf('down.example.com')).getByText('未检查')).toBeInTheDocument()
  })

  it('片段弹窗：选 agent 拉取并可复制；删除走 Popconfirm', async () => {
    apiMock.getAgentDomainsSnippet.mockResolvedValue('# snippet for web-01')
    const writeText = vi.fn(() => Promise.resolve())
    Object.assign(navigator, { clipboard: { writeText } })
    renderPage()
    await screen.findByText('blog.example.com')
    // 顶部片段下拉（卡片 extra 里）
    fireEvent.mouseDown(screen.getAllByText('选择 agent 生成配置片段')[0].closest('.ant-select')!.querySelector('.ant-select-selector')!)
    const opt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent === 'web-01')
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(opt)
    expect(await screen.findByText(/# snippet for web-01/)).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /复\s*制/ }))
    })
    expect(writeText).toHaveBeenCalledWith('# snippet for web-01')
    expect(msgSuccess).toHaveBeenCalledWith('已复制')
    // 删除：Popconfirm 确认
    fireEvent.click(document.querySelector('.ant-modal-close')!)
    await act(async () => {
      fireEvent.click(within(rowOf('blog.example.com')).getByRole('button', { name: /删\s*除/ }))
    })
    expect(await screen.findByText('只删登记，已下发的 DNS/站点/监控项保留')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    })
    await waitFor(() => expect(apiMock.deleteDomainBinding).toHaveBeenCalledWith('blog.example.com'))
    expect(msgSuccess).toHaveBeenCalledWith('已删除登记（已下发的 DNS/站点/监控项保留，需手动清理）')
  })

  it('漂移检查失败与保存失败：错误透出', async () => {
    apiMock.getDomainDrift.mockRejectedValue(new Error('drift fail'))
    renderPage()
    await screen.findByText('blog.example.com')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /漂移检查/ }))
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('漂移检查失败'))
    // 保存失败：合法表单提交但接口拒绝
    apiMock.saveDomainBinding.mockRejectedValue(new Error('save fail'))
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /登记绑定/ }))
    })
    fireEvent.change(screen.getByPlaceholderText('blog.example.com'), { target: { value: 'new.example.com' } })
    fireEvent.mouseDown(document.querySelector('.ant-modal .ant-select-selector')!)
    const opt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent === 'web-01')
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(opt)
    fireEvent.change(screen.getByPlaceholderText('127.0.0.1:8080 或 docker://web'), { target: { value: 'docker://web' } })
    await modalOk()
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
  })

  it('删除失败错误透出；登记 Modal 取消与刷新按钮', async () => {
    apiMock.deleteDomainBinding.mockRejectedValue(new Error('del fail'))
    renderPage()
    await screen.findByText('blog.example.com')
    await act(async () => {
      fireEvent.click(within(rowOf('blog.example.com')).getByRole('button', { name: /删\s*除/ }))
    })
    expect(await screen.findByText('只删登记，已下发的 DNS/站点/监控项保留')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('删除失败'))
    // 登记 Modal 取消 → 关闭
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /登记绑定/ }))
    })
    await waitFor(() => expect(document.querySelector('.ant-modal-title')?.textContent).toContain('登记绑定'))
    fireEvent.click(document.querySelector('.ant-modal-footer .ant-btn:not(.ant-btn-primary)')!)
    // jsdom 无过渡动画不做 DOM 消失断言；行为验证：重开登记表单已重置（destroyOnClose）
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /登记绑定/ }))
    })
    await waitFor(() => expect(document.querySelector('.ant-modal-title')?.textContent).toContain('登记绑定'))
    expect(screen.getByPlaceholderText('blog.example.com')).toHaveValue('')
    // 刷新按钮 → 绑定查询重新拉取
    const before = apiMock.getDomainBindings.mock.calls.length
    const reloadBtn = document.querySelector('.ant-card-head button .anticon-reload')!.closest('button')!
    await act(async () => {
      fireEvent.click(reloadBtn)
    })
    await waitFor(() => expect(apiMock.getDomainBindings.mock.calls.length).toBeGreaterThan(before))
  })

  it('漂移：未知与缺失 status 均走「漂移」兜底', async () => {
    apiMock.getDomainDrift.mockResolvedValue({
      checkedAt: 1,
      items: [
        {
          domain: 'blog.example.com',
          enabled: true,
          dns: { checked: true, ok: false, status: 'weird' }, // 未知名 → 兜底
          proxy: { checked: true, ok: false }, // 无 status → ?? '' 再兜底
          cert: { checked: true, ok: true },
        },
      ],
    })
    renderPage()
    await screen.findByText('blog.example.com')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /漂移检查/ }))
    })
    expect(msgSuccess).toHaveBeenCalledWith('漂移检查完成（1 条绑定）')
    const r1 = rowOf('blog.example.com')
    expect(within(r1).getByText('DNS·漂移')).toBeInTheDocument()
    expect(within(r1).getByText('反代·漂移')).toBeInTheDocument()
    expect(within(r1).getByText((_, el) => el?.classList.contains('ant-tag-success') === true && el.textContent === '证书')).toBeInTheDocument()
  })

  it('防御分支：hostname 空 agent 显示 id、未知 agentId 原值、失败行空 lastError 兜底', async () => {
    apiMock.getDomainBindings.mockResolvedValue([
      mkBinding({ id: 1, domain: 'empty.example.com', agentId: 'ag-2' }),
      mkBinding({
        id: 2,
        domain: 'ghost.example.com',
        agentId: 'ag-ghost',
        lastApplyStatus: 'failed',
        lastError: '',
        appliedAt: 1758000200,
      }),
    ])
    apiMock.getAgents.mockResolvedValue([
      ...agents,
      { id: 'ag-2', hostname: '', ip: '1.2.3.5', status: 'online', lastSeen: '0', capabilities: [] },
    ] as unknown as Agent[])
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <DomainsPage />
      </QueryClientProvider>,
    )
    await screen.findByText('empty.example.com')
    // hostname 空 → Agent 列回退显示 id
    expect(within(rowOf('empty.example.com')).getByText('ag-2')).toBeInTheDocument()
    // agentId 不在 agent 列表 → 原样显示 id
    expect(within(rowOf('ghost.example.com')).getByText('ag-ghost')).toBeInTheDocument()
    // 失败行 lastError 为空串 → tooltip 兜底「失败」
    fireEvent.mouseEnter(within(rowOf('ghost.example.com')).getByText(/失败·/))
    fireEvent.mouseMove(within(rowOf('ghost.example.com')).getByText(/失败·/))
    expect(await screen.findByText('失败')).toBeInTheDocument()
    fireEvent.mouseLeave(document.body)
    // 登记 Modal 内 Agent 下拉 option：hostname 空回退 id（下拉 portal 在 body，须非作用域查询）
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /登记绑定/ }))
    })
    fireEvent.mouseDown(document.querySelector('.ant-modal .ant-select-selector')!)
    await waitFor(() =>
      expect(Array.from(document.querySelectorAll('.ant-select-item-option')).map((o) => o.textContent)).toEqual(['web-01', 'ag-2']))
    // 卡片 extra 片段下拉 option：同样回退 id
    fireEvent.mouseDown(screen.getAllByText('选择 agent 生成配置片段')[0].closest('.ant-select')!.querySelector('.ant-select-selector')!)
    await waitFor(() =>
      expect(Array.from(document.querySelectorAll('.ant-select-item-option')).map((o) => o.textContent)).toContain('ag-2'))
  })

  it('防御分支：apply 无 steps 字段兜底空数组；片段为 null 复制空串', async () => {
    apiMock.applyDomainBinding.mockResolvedValue({ status: 'ok' }) // 无 steps 字段
    apiMock.getAgentDomainsSnippet.mockResolvedValue(null)
    const writeText = vi.fn(() => Promise.resolve())
    Object.assign(navigator, { clipboard: { writeText } })
    renderPage()
    await screen.findByText('blog.example.com')
    await act(async () => {
      fireEvent.click(within(rowOf('blog.example.com')).getByRole('button', { name: /Apply/ }))
    })
    // steps ?? [] → 空数组 → 「未启用任何 auto 联动开关」
    expect(await screen.findByText('未启用任何 auto 联动开关')).toBeInTheDocument()
    // 片段弹窗：snippet 为 null → 复制空串兜底
    fireEvent.mouseDown(screen.getAllByText('选择 agent 生成配置片段')[0].closest('.ant-select')!.querySelector('.ant-select-selector')!)
    const opt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent === 'web-01')
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(opt)
    expect(await screen.findByText('域名配置片段 · web-01')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /复\s*制/ }))
    })
    expect(writeText).toHaveBeenCalledWith('')
    expect(msgSuccess).toHaveBeenCalledWith('已复制')
  })
})
