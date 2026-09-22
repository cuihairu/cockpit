import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import LogsPanel, { GrepLine, lineTone } from './LogsPanel'

// LogsPanel：远程日志查询面板——源派生/查询/着色高亮/NDJSON 尾随状态机

const apiMock = vi.hoisted(() => ({
  getLogsStatus: vi.fn(),
  getLogsSources: vi.fn(),
  queryLogs: vi.fn(),
  followLogs: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))

const msgError = vi.spyOn(message, 'error')
const msgWarning = vi.spyOn(message, 'warning')

const wrap = (ui: React.ReactNode) => {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
}

const ready = (opts: { journalctl?: boolean; docker?: boolean } = {}) => {
  apiMock.getLogsStatus.mockResolvedValue({
    journalctl: opts.journalctl ?? true, docker: opts.docker ?? true,
  })
  apiMock.getLogsSources.mockResolvedValue({
    systemd: ['nginx.service', 'ssh.service'], docker: ['web', 'db'],
  })
}

const ndjson = (frames: object[]) => {
  const enc = new TextEncoder()
  const chunks = frames.map((f) => enc.encode(JSON.stringify(f) + '\n'))
  let i = 0
  return new ReadableStream({
    pull(controller) {
      if (i < chunks.length) controller.enqueue(chunks[i++])
      else controller.close()
    },
  })
}

const followResp = (frames: object[]) =>
  ({ ok: true, status: 200, body: ndjson(frames) }) as unknown as Response

// 单 chunk 推送完整 NDJSON 文本（含空行/坏帧时才走 continue 分支）
const oneChunk = (text: string) =>
  ({
    ok: true,
    status: 200,
    body: new ReadableStream({
      pull(controller) {
        controller.enqueue(new TextEncoder().encode(text))
        controller.close()
      },
    }),
  }) as unknown as Response

// 按钮 disabled 后 React/antd 层都不派发 onClick（getListener 与 antd 守卫），
// 沿 fiber 上溯取 LogsPanel 传给 Button 的原始 handler（0 参闭包）直调，
// 覆盖 startFollow 的 !source 防御分支
const callButtonOnClick = (el: HTMLElement) => {
  const key = Object.keys(el).find((k) => k.startsWith('__reactFiber$'))
  if (!key) throw new Error('未找到 react fiber')
  let fiber: unknown = (el as unknown as Record<string, unknown>)[key]
  let latest: (() => void) | null = null
  while (fiber) {
    const p = (fiber as { memoizedProps?: Record<string, unknown> }).memoizedProps
    if (p && typeof p.onClick === 'function' && (p.onClick as () => void).length === 0) {
      latest = p.onClick as () => void
    }
    fiber = (fiber as { return?: unknown }).return
  }
  if (!latest) throw new Error('未找到 Button onClick')
  latest()
}

describe('lineTone / GrepLine（纯渲染）', () => {
  it('级别着色：ERROR/FATAL/PANIC 红、WARN 橙、其余 null', () => {
    expect(lineTone('got ERROR here')).toBe('#ff6b6b')
    expect(lineTone('FATAL at x')).toBe('#ff6b6b')
    expect(lineTone('panic mode')).toBe('#ff6b6b')
    expect(lineTone('WARN: disk')).toBe('#ffa940')
    expect(lineTone('info ok')).toBeNull()
  })

  it('GrepLine：命中片段 mark 高亮（可多次）；未命中/空 grep 原样；空行占位', () => {
    const { container, rerender } = render(<GrepLine line="error one error two" grep="error" />)
    expect(container.querySelectorAll('mark')).toHaveLength(2)
    expect(container.querySelectorAll('mark')[0].textContent).toBe('error')

    rerender(<GrepLine line="plain line" grep="error" />)
    expect(container.querySelector('mark')).toBeNull()
    expect(container.textContent).toBe('plain line')

    rerender(<GrepLine line="anything" grep="" />)
    expect(container.querySelector('mark')).toBeNull()

    rerender(<GrepLine line="" grep="" />)
    expect(container.textContent).toBe(' ')
  })
})

describe('LogsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    msgError.mockClear()
    msgWarning.mockClear()
    ready()
    apiMock.queryLogs.mockReset()
    apiMock.followLogs.mockReset()
  })

  it('加载 status/sources；systemd 源列表默认选第一项；空态提示', async () => {
    render(wrap(<LogsPanel agentId="ag1" />))
    expect(await screen.findByText('nginx.service')).toBeInTheDocument()
    expect(screen.getByText(/选择日志源后点击「查询」/)).toBeInTheDocument()
    expect(apiMock.getLogsStatus).toHaveBeenCalledWith('ag1')
    expect(apiMock.getLogsSources).toHaveBeenCalledWith('ag1')
  })

  it('systemd 不可用：提示 + Segmented 对应项禁用（默认类型即出 Alert）', async () => {
    ready({ journalctl: false })
    render(wrap(<LogsPanel agentId="ag1" />))
    expect(await screen.findByText('该主机没有 journalctl（非 systemd 或未安装）')).toBeInTheDocument()
    // docker 容器选项可用（未被禁用），systemd 项禁用
    const dockerItem = screen.getByText('docker 容器').closest('.ant-segmented-item')!
    const systemdItem = screen.getByText('systemd 服务').closest('.ant-segmented-item')!
    expect(systemdItem.className).toContain('ant-segmented-item-disabled')
    expect(dockerItem.className).not.toContain('ant-segmented-item-disabled')
  })

  it('点「查询」按当前参数调用并渲染结果/行数/截断提示', async () => {
    apiMock.queryLogs.mockResolvedValue({ lines: 'line1\nERROR bad\nline3', truncated: true })
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.click(screen.getByText('查 询').closest('button')!)
    await screen.findByText('line1')
    expect(apiMock.queryLogs).toHaveBeenCalledWith('ag1', {
      type: 'systemd', source: 'nginx.service', tail: 200, since_minutes: 0, grep: '',
    })
    expect(screen.getByText(/共 3 行/)).toBeInTheDocument()
    expect(screen.getByText('输出过大已截断，建议缩小行数或时间范围')).toBeInTheDocument()
    // ERROR 行着色
    const bad = screen.getByText('ERROR bad')
    expect(bad.style.color).toBe('rgb(255, 107, 107)')
  })

  it('查询失败 message.error；关键词过滤透传 grep 且高亮命中', async () => {
    apiMock.queryLogs.mockRejectedValue(new Error('down'))
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.change(screen.getByPlaceholderText('关键词过滤'), { target: { value: 'bad' } })
    fireEvent.click(screen.getByText('查 询').closest('button')!)
    await waitFor(() => expect(msgError).toHaveBeenCalled())

    apiMock.queryLogs.mockResolvedValue({ lines: 'bad thing', truncated: false })
    await waitFor(() => expect(screen.getByText('查 询').closest('button')!.className).not.toContain('loading'))
    fireEvent.click(screen.getByText('查 询').closest('button')!)
    await waitFor(() => expect(apiMock.queryLogs).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(document.querySelector('mark')).not.toBeNull())
    expect(apiMock.queryLogs).toHaveBeenLastCalledWith('ag1', {
      type: 'systemd', source: 'nginx.service', tail: 200, since_minutes: 0, grep: 'bad',
    })
  })

  it('切 docker 类型源回落第一项（systemd 手选不在 docker 列表）', async () => {
    apiMock.queryLogs.mockResolvedValue({ lines: 'x', truncated: false })
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.click(screen.getByText('docker 容器'))
    fireEvent.click(screen.getByText('查 询').closest('button')!)
    await waitFor(() =>
      expect(apiMock.queryLogs).toHaveBeenLastCalledWith('ag1', expect.objectContaining({
        type: 'docker', source: 'web',
      })))
  })

  it('实时尾随：data 帧追加行；eof limit 显示上限 Tag', async () => {
    apiMock.followLogs.mockResolvedValue(followResp([
      { data: 'stream1\nstream2\n' },
      { data: 'stream3\n' },
      { eof: true, reason: 'limit' },
    ]))
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.click(followButton())
    expect(await screen.findByText('stream1')).toBeInTheDocument()
    expect(screen.getByText('stream3')).toBeInTheDocument()
    expect(screen.getByText('输出达上限（4MB），已自动停止')).toBeInTheDocument()
    expect(apiMock.followLogs).toHaveBeenCalledWith(
      'ag1',
      { type: 'systemd', source: 'nginx.service', tail: 200, since_minutes: 0, grep: '' },
      expect.any(AbortSignal),
    )
  })

  it('尾随 HTTP 失败与流异常断开', async () => {
    apiMock.followLogs.mockResolvedValue(
      { ok: false, status: 500, json: async () => ({ error: 'agent down' }) } as unknown as Response,
    )
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.click(followButton())
    // getApiErrorMessage 对非 axios 错误回退默认文案（普通 Error.message 被丢弃）
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('实时尾随失败'))

    // 流正常结束但无 eof 帧 → disconnected
    apiMock.followLogs.mockResolvedValue(followResp([{ data: 'a\n' }]))
    fireEvent.click(followButton())
    await screen.findByText('a')
    expect(await screen.findByText('连接中断')).toBeInTheDocument()
  })

  it('点「停止尾随」abort 流；尾随中查询按钮禁用', async () => {
    // 永不结束的流（pull 挂起不 close），保证 following 持续
    apiMock.followLogs.mockImplementation(() =>
      Promise.resolve({
        ok: true, status: 200,
        body: new ReadableStream({
          start(controller) {
            controller.enqueue(new TextEncoder().encode(JSON.stringify({ data: 'live\n' }) + '\n'))
          },
        }),
      } as unknown as Response))
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.click(followButton())
    await screen.findByText('live')
    expect(screen.getByText('查 询').closest('button')!.disabled).toBe(true)
    fireEvent.click(followButton())
    await waitFor(() => expect(screen.getByText('查 询').closest('button')!.disabled).toBe(false))
  })

  it('initialSource 预选且 status 就绪后自动首查', async () => {
    apiMock.queryLogs.mockResolvedValue({ lines: 'auto queried', truncated: false })
    render(wrap(<LogsPanel agentId="ag1" initialSource="cron.service" />))
    await screen.findByText('auto queried')
    expect(apiMock.queryLogs).toHaveBeenCalledWith('ag1', expect.objectContaining({
      source: 'cron.service',
    }))
  })

  it('initialSource 但主机无 journalctl：跳过自动首查', async () => {
    ready({ journalctl: false })
    apiMock.queryLogs.mockResolvedValue({ lines: 'x', truncated: false })
    render(wrap(<LogsPanel agentId="ag1" initialSource="cron.service" />))
    await screen.findByText('该主机没有 journalctl（非 systemd 或未安装）')
    expect(apiMock.queryLogs).not.toHaveBeenCalled()
  })

  it('无可用日志源：查询/尾随前置校验 message.warning', async () => {
    apiMock.getLogsSources.mockResolvedValue({ systemd: [], docker: [] })
    apiMock.queryLogs.mockResolvedValue({ lines: 'x', truncated: false })
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('选择日志源', { selector: '.ant-select-selection-placeholder' })
    fireEvent.click(screen.getByText('查 询').closest('button')!)
    await waitFor(() => expect(msgWarning).toHaveBeenCalledWith('请选择日志源'))
    expect(apiMock.queryLogs).not.toHaveBeenCalled()
    // 尾随按钮 disabled={!source}，点击到不了 handler——直调 props.onClick 覆盖防御分支
    const btn = followButton()
    expect((btn as HTMLButtonElement).disabled).toBe(true)
    callButtonOnClick(btn)
    await waitFor(() => expect(msgWarning).toHaveBeenCalledTimes(2))
    expect(apiMock.followLogs).not.toHaveBeenCalled()
  })

  it('手选源保留选择；不在新类型列表时回落第一项', async () => {
    apiMock.queryLogs.mockResolvedValue({ lines: 'x', truncated: false })
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    const sel = screen.getByText('nginx.service', { selector: '.ant-select-selection-item' })
      .closest('.ant-select')!
    fireEvent.mouseDown(sel.querySelector('.ant-select-selector')!)
    fireEvent.click(await screen.findByText('ssh.service', { selector: '.ant-select-item-option-content' }))
    fireEvent.click(screen.getByText('查 询').closest('button')!)
    await waitFor(() =>
      expect(apiMock.queryLogs).toHaveBeenLastCalledWith('ag1', expect.objectContaining({
        source: 'ssh.service',
      })))
    fireEvent.click(screen.getByText('docker 容器'))
    fireEvent.click(screen.getByText('查 询').closest('button')!)
    await waitFor(() =>
      expect(apiMock.queryLogs).toHaveBeenLastCalledWith('ag1', expect.objectContaining({
        type: 'docker', source: 'web',
      })))
  })

  it('行数清空回落 200、回车触发查询、刷新按钮复用查询', async () => {
    apiMock.queryLogs.mockResolvedValue({ lines: 'x', truncated: false })
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    const tailInput = document.querySelector<HTMLInputElement>('.ant-input-number-input')!
    fireEvent.change(tailInput, { target: { value: '50' } })
    fireEvent.click(screen.getByText('查 询').closest('button')!)
    await waitFor(() =>
      expect(apiMock.queryLogs).toHaveBeenLastCalledWith('ag1', expect.objectContaining({ tail: 50 })))
    fireEvent.change(tailInput, { target: { value: '' } })
    fireEvent.click(screen.getByText('刷新').closest('button')!)
    await waitFor(() =>
      expect(apiMock.queryLogs).toHaveBeenLastCalledWith('ag1', expect.objectContaining({ tail: 200 })))
    const grepInput = screen.getByPlaceholderText('关键词过滤')
    fireEvent.change(grepInput, { target: { value: 'err' } })
    fireEvent.keyDown(grepInput, { key: 'Enter' })
    await waitFor(() =>
      expect(apiMock.queryLogs).toHaveBeenLastCalledWith('ag1', expect.objectContaining({ grep: 'err' })))
  })

  it('尾随失败：错误体无 error 字段保留 HTTP 提示；无流 body 报不支持', async () => {
    apiMock.followLogs.mockResolvedValue(
      { ok: false, status: 502, json: async () => ({}) } as unknown as Response,
    )
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.click(followButton())
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('实时尾随失败'))

    apiMock.followLogs.mockResolvedValue({ ok: true, status: 200, body: undefined } as unknown as Response)
    fireEvent.click(followButton())
    await waitFor(() => expect(msgError).toHaveBeenCalledTimes(2))
  })

  it('尾随帧解析：空行/坏 JSON/无 data 跳过；eof 缺省 reason 显示已停止', async () => {
    // 单 chunk 内含空行与坏帧（分 chunk 时空行会进残 buf，走不到 continue 分支）
    apiMock.followLogs.mockResolvedValue(oneChunk(
      '  \nnot-json\n{}\n' + JSON.stringify({ data: 'x\n' }) + '\n' + JSON.stringify({ eof: true }) + '\n',
    ))
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.click(followButton())
    expect(await screen.findByText('x')).toBeInTheDocument()
    expect(await screen.findByText('已停止')).toBeInTheDocument()
  })

  it('eof 未知 reason 原样展示', async () => {
    apiMock.followLogs.mockResolvedValue(followResp([
      { data: 'y\n' }, { eof: true, reason: 'mystery' },
    ]))
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.click(followButton())
    expect(await screen.findByText('y')).toBeInTheDocument()
    expect(await screen.findByText('mystery')).toBeInTheDocument()
  })

  it('停止尾随触发 AbortError 静默返回不报错', async () => {
    apiMock.followLogs.mockImplementation((_id: string, _p: unknown, signal: AbortSignal) =>
      Promise.resolve({
        ok: true, status: 200,
        body: new ReadableStream({
          start(controller) {
            controller.enqueue(new TextEncoder().encode(JSON.stringify({ data: 'live\n' }) + '\n'))
            signal.addEventListener('abort', () => {
              controller.error(Object.assign(new Error('aborted'), { name: 'AbortError' }))
            })
          },
        }),
      } as unknown as Response))
    render(wrap(<LogsPanel agentId="ag1" />))
    await screen.findByText('nginx.service')
    fireEvent.click(followButton())
    await screen.findByText('live')
    fireEvent.click(followButton())
    await waitFor(() => expect(screen.getByText('实时尾随')).toBeInTheDocument())
    expect(msgError).not.toHaveBeenCalled()
  })

  it('先选 docker 再感知 docker 缺失：提示该主机没有 docker 命令', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={qc}><LogsPanel agentId="ag1" /></QueryClientProvider>)
    await screen.findByText('nginx.service')
    fireEvent.click(screen.getByText('docker 容器'))
    ready({ journalctl: true, docker: false })
    await qc.invalidateQueries({ queryKey: ['logs-status', 'ag1'] })
    expect(await screen.findByText('该主机没有 docker 命令')).toBeInTheDocument()
  })
})

// 按钮文本与空态提示文案重叠，按可访问名定位按钮
function followButton() {
  return screen.getByRole('button', { name: /实时尾随|停止尾随/ })
}
