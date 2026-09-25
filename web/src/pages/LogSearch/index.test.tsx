import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import LogSearch from './index'
import type { Agent } from '@/types'

// LogSearch：跨机日志检索——空源拦截 / 参数组装（agents 子集语义）/
// skipped 告警 / 成功截断失败三分支 / Enter 提交

const apiMock = vi.hoisted(() => ({ getAgents: vi.fn(), searchLogs: vi.fn() }))
vi.mock('@/services/api', () => ({ api: apiMock }))

const grepLines = vi.hoisted(() => ({ lines: [] as string[] }))
vi.mock('@/workbench/LogsPanel', () => ({
  GrepLine: ({ line }: { line: string }) => {
    grepLines.lines.push(line)
    return <span data-testid="grep-line">{line}</span>
  },
}))

const msgWarning = vi.spyOn(message, 'warning')
const msgError = vi.spyOn(message, 'error')

const mkAgent = (id: string, hostname: string, withLogs: boolean): Agent =>
  ({
    id,
    hostname,
    ip: '1.2.3.4',
    capabilities: withLogs ? [{ type: 'logs' }] : [{ type: 'files' }],
    status: 'online',
    lastSeen: '0',
  }) as unknown as Agent

const result = {
  results: [
    { agentId: 'ag-1', hostname: 'web-01', ok: true, lines: 'line-a\nline-b', truncated: false },
    { agentId: 'ag-2', hostname: 'db-01', ok: true, lines: 'x', truncated: true },
    { agentId: 'ag-3', hostname: '', ok: false, error: '连接超时' },
  ],
  skipped: [
    { agentId: 'ag-4', reason: 'offline' },
    { agentId: 'ag-5', reason: 'weird-reason' },
  ],
}

const renderPage = () => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <LogSearch />
    </QueryClientProvider>,
  )
}

describe('LogSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    grepLines.lines = []
    apiMock.getAgents.mockReset()
    apiMock.getAgents.mockResolvedValue([mkAgent('ag-1', 'web-01', true), mkAgent('ag-2', 'db-01', true)])
    apiMock.searchLogs.mockReset()
    apiMock.searchLogs.mockResolvedValue(result)
  })

  it('渲染表单；空源提交拦截', async () => {
    renderPage()
    expect(await screen.findByText('日志检索')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /检索/ }))
    })
    expect(msgWarning).toHaveBeenCalledWith('请填写日志源（systemd unit 名或 docker 容器名）')
    expect(apiMock.searchLogs).not.toHaveBeenCalled()
  })

  it('提交成功：汇总行、skipped 告警与三分支结果卡', async () => {
    renderPage()
    await screen.findByText('日志检索')
    fireEvent.change(screen.getByPlaceholderText('日志源，如 nginx.service'), { target: { value: 'nginx.service' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /检索/ }))
    })
    await waitFor(() => expect(apiMock.searchLogs).toHaveBeenCalled())
    // 2/3 台返回，行数 = 2 + 1（截断的 x 也计入）
    expect(screen.getByText('2/3 台返回，共 3 行')).toBeInTheDocument()
    expect(screen.getByText('2 台主机已跳过')).toBeInTheDocument()
    expect(screen.getByText(/ag-4：离线/)).toBeInTheDocument()
    expect(screen.getByText(/ag-5：weird-reason/)).toBeInTheDocument() // 未知 reason 原样透出
    // 成功卡：行数 Tag；截断卡：橙 Tag；失败卡：error 文本
    expect(screen.getByText('2 行')).toBeInTheDocument()
    expect(screen.getByText('已截断（超 1MB）')).toBeInTheDocument()
    expect(screen.getByText('连接超时')).toBeInTheDocument()
    // GrepLine 桩逐行透传（含空行过滤前的原样 split）
    expect(grepLines.lines).toEqual(expect.arrayContaining(['line-a', 'line-b']))
  })

  it('未选子集时 agents 参数为 undefined', async () => {
    renderPage()
    await screen.findByText('日志检索')
    fireEvent.change(screen.getByPlaceholderText('日志源，如 nginx.service'), { target: { value: 'nginx.service' } })
    fireEvent.change(screen.getByPlaceholderText('过滤关键字（可选）'), { target: { value: 'error' } })
    await act(async () => {
      fireEvent.keyDown(screen.getByPlaceholderText('日志源，如 nginx.service'), { key: 'Enter' })
      fireEvent.keyUp(screen.getByPlaceholderText('日志源，如 nginx.service'), { key: 'Enter' })
    })
    await waitFor(() =>
      expect(apiMock.searchLogs).toHaveBeenCalledWith({
        type: 'systemd',
        source: 'nginx.service',
        tail: 200,
        since_minutes: 0,
        grep: 'error',
        agents: undefined,
      }))
  })

  it('选定子集时 agents 传 ID 列表', async () => {
    renderPage()
    await screen.findByText('日志检索')
    fireEvent.change(screen.getByPlaceholderText('日志源，如 nginx.service'), { target: { value: 'nginx.service' } })
    fireEvent.mouseDown(screen.getByText('全部主机').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    const opt = await waitFor(() => {
      const el = Array.from(document.querySelectorAll('.ant-select-item-option')).find(
        (o) => o.textContent === 'web-01')
      if (!el) throw new Error('option not found')
      return el as HTMLElement
    })
    fireEvent.click(opt)
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /检索/ }))
    })
    await waitFor(() =>
      expect(apiMock.searchLogs).toHaveBeenCalledWith(
        expect.objectContaining({ agents: ['ag-1'] })))
  })

  it('接口失败：fallback 文案', async () => {
    apiMock.searchLogs.mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByText('日志检索')
    fireEvent.change(screen.getByPlaceholderText('日志源，如 nginx.service'), { target: { value: 'nginx.service' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /检索/ }))
    })
    expect(msgError).toHaveBeenCalledWith('检索失败')
  })

  it('结果为空数组时提示无可检索主机', async () => {
    apiMock.searchLogs.mockResolvedValue({ results: [], skipped: [] })
    renderPage()
    await screen.findByText('日志检索')
    fireEvent.change(screen.getByPlaceholderText('日志源，如 nginx.service'), { target: { value: 'nginx.service' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /检索/ }))
    })
    expect(await screen.findByText('没有可检索的主机（全部离线或无日志能力）')).toBeInTheDocument()
  })

  it('类型 Segmented 切换与 tail 数值调整', async () => {
    renderPage()
    expect(await screen.findByText('日志检索')).toBeInTheDocument()
    // Segmented onChange：systemd → docker → systemd
    fireEvent.click(screen.getByText('docker'))
    expect(document.querySelector('.ant-segmented-item-selected')?.textContent).toBe('docker')
    fireEvent.click(screen.getByText('systemd'))
    expect(document.querySelector('.ant-segmented-item-selected')?.textContent).toBe('systemd')
    // InputNumber onChange：调整 tail 行数
    const tail = screen.getByRole('spinbutton')
    fireEvent.change(tail, { target: { value: '50' } })
    expect((tail as HTMLInputElement).value).toBe('50')
  })

  it('tail 清空（onChange null）回落 200 并入参', async () => {
    renderPage()
    await screen.findByText('日志检索')
    const tail = screen.getByRole('spinbutton')
    // jsdom 下 InputNumber 须 focus → change → blur 序列 onChange 才触发
    await act(async () => {
      fireEvent.focus(tail)
      fireEvent.change(tail, { target: { value: '50' } })
      fireEvent.blur(tail)
    })
    fireEvent.change(screen.getByPlaceholderText('日志源，如 nginx.service'), { target: { value: 'nginx.service' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /检索/ }))
    })
    await waitFor(() =>
      expect(apiMock.searchLogs).toHaveBeenCalledWith(expect.objectContaining({ tail: 50 })))
    // 清空输入 → onChange(null) → ?? 200 兜底
    await act(async () => {
      fireEvent.focus(tail)
      fireEvent.change(tail, { target: { value: '' } })
      fireEvent.blur(tail)
    })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /检索/ }))
    })
    await waitFor(() =>
      expect(apiMock.searchLogs).toHaveBeenLastCalledWith(expect.objectContaining({ tail: 200 })))
  })

  it('防御分支：capabilities 缺失过滤、hostname 空、ok 行无 lines、失败行无 error', async () => {
    apiMock.getAgents.mockResolvedValue([
      mkAgent('ag-1', 'web-01', true),
      // capabilities 缺失 → || [] 兜底后无 logs 能力，被过滤
      { id: 'ag-7', hostname: 'db-07', ip: '1.2.3.6', capabilities: undefined, status: 'online', lastSeen: '0' } as unknown as Agent,
      // hostname 空 → 下拉 label 回退 id
      { id: 'ag-9', hostname: '', ip: '1.2.3.7', capabilities: [{ type: 'logs' }], status: 'online', lastSeen: '0' } as unknown as Agent,
    ])
    apiMock.searchLogs.mockResolvedValue({
      results: [
        { agentId: 'ag-9', hostname: '', ok: true }, // 无 lines → ?? '' 兜底
        { agentId: 'ag-8', hostname: 'app-01', ok: false }, // 无 error → '未知错误'
      ],
      skipped: [],
    })
    renderPage()
    await screen.findByText('日志检索')
    // 主机下拉：无 logs 能力被过滤（capabilities || [] 不炸）；hostname 空显示 id
    fireEvent.mouseDown(screen.getByText('全部主机').closest('.ant-select')!.querySelector('.ant-select-selector')!)
    await waitFor(() =>
      expect(Array.from(document.querySelectorAll('.ant-select-item-option')).map((o) => o.textContent)).toEqual(['web-01', 'ag-9']))
    fireEvent.change(screen.getByPlaceholderText('日志源，如 nginx.service'), { target: { value: 'nginx.service' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /检索/ }))
    })
    // ok 行无 lines → ?? '' → '0 行'（非截断分支）；汇总行共 0 行
    expect(await screen.findByText('0 行')).toBeInTheDocument()
    expect(screen.getByText('1/2 台返回，共 0 行')).toBeInTheDocument()
    // 失败行无 error → '未知错误'
    expect(screen.getByText('未知错误')).toBeInTheDocument()
  })
})
