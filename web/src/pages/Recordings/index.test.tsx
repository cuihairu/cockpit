import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import Recordings from './index'
import type { RecordingConfig, TerminalRecording } from '@/types'

// Recordings：录制列表（时长/大小/协议格式化）+ 配置行（校验/保存）+
// 回放器（cast 解析调度写 xterm、倍速重放）+ 下载/删除/补推

const terminalMock = vi.hoisted(() => ({
  loadAddon: vi.fn(),
  open: vi.fn(),
  write: vi.fn(),
  reset: vi.fn(),
  dispose: vi.fn(),
}))
const fitMock = vi.hoisted(() => ({ fit: vi.fn(), loadAddon: vi.fn() }))
vi.mock('@xterm/xterm', () => ({ Terminal: vi.fn(function () { return terminalMock }) }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: vi.fn(function () { return fitMock }) }))

const apiMock = vi.hoisted(() => ({
  getRecordings: vi.fn(),
  getRecordingsConfig: vi.fn(),
  putRecordingsConfig: vi.fn(),
  getRecordingCast: vi.fn(),
  syncRecordingRemote: vi.fn(),
  deleteRecording: vi.fn(),
  downloadRecording: vi.fn(),
}))
vi.mock('@/services/api', () => ({ api: apiMock }))
vi.mock('@/hooks/usePerm', () => ({ usePerm: () => true }))

const msgSuccess = vi.spyOn(message, 'success')
const msgError = vi.spyOn(message, 'error')

// header + 三个事件（相对秒小，真实 timer 10ms 内写完）
const castText = [
  '{"version":2,"width":80,"height":24}',
  '[0.005, "o", "hello"]',
  '[0.01, "o", " world"]',
  'not-json-truncated',
].join('\n')

const mkRec = (over: Partial<TerminalRecording>): TerminalRecording =>
  ({
    id: 1,
    sessionId: 'sess-1',
    username: 'admin',
    agentId: 'ag-1',
    host: '10.0.0.1',
    port: 22,
    protocol: 'ssh',
    startedAt: '2026-01-02T03:04:05Z',
    durationMs: 0,
    bytes: 0,
    ...over,
  }) as TerminalRecording

const recordings = [
  mkRec({ sessionId: 'sess-1', durationMs: 35000, bytes: 500 }),
  mkRec({ sessionId: 'sess-2', protocol: 'telnet', durationMs: (90 * 60 + 5) * 1000, bytes: 3 * 1024 * 1024 }),
  mkRec({ sessionId: 'sess-3', durationMs: 0, bytes: 2048 }), // 进行中：无回放
]

const renderPage = (
  cfg: Partial<RecordingConfig> = { enabled: true, retention_days: 7, remote_dest: '', max_retention_days: 365 },
  recs: TerminalRecording[] = recordings,
) => {
  apiMock.getRecordings.mockResolvedValue(recs)
  apiMock.getRecordingsConfig.mockResolvedValue(cfg)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <Recordings />
    </QueryClientProvider>,
  )
}

// 表格无 sessionId 列，按行内唯一文本（时长/大小）定位
const rowOf = (cell: string) =>
  Array.from(document.querySelectorAll<HTMLTableRowElement>('tr.ant-table-row')).find((tr) =>
    tr.textContent?.includes(cell)) as HTMLTableRowElement

// 首开时 Modal 内容（termDiv）挂载晚于 sessionId effect 一帧，
// 先关再开才建立终端并触发 play（与现有回放用例同构）
const openPlayer = async (rowText: string) => {
  const playBtn = () => rowOf(rowText).querySelectorAll('button')[0] as HTMLButtonElement
  await act(async () => {
    fireEvent.click(playBtn())
  })
  await act(async () => {
    fireEvent.click(document.querySelector('.ant-modal-close')!)
  })
  await act(async () => {
    fireEvent.click(playBtn())
  })
}

const closePlayer = async () => {
  await act(async () => {
    fireEvent.click(document.querySelector('.ant-modal-close')!)
  })
}

describe('Recordings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('URL', Object.assign(URL, {
      createObjectURL: vi.fn(() => 'blob:x'),
      revokeObjectURL: vi.fn(),
    }))
    apiMock.getRecordingCast.mockResolvedValue(castText)
  })

  it('列表：时长/大小/协议格式化，进行中禁用回放', async () => {
    renderPage()
    expect(await screen.findByText('35s')).toBeInTheDocument()
    expect(screen.getByText('1h30m')).toBeInTheDocument() // 小时档不带秒
    expect(screen.getByText('500 B')).toBeInTheDocument()
    expect(screen.getByText('2.0 KB')).toBeInTheDocument()
    expect(screen.getByText('3.0 MB')).toBeInTheDocument()
    expect(screen.getByText('telnet')).toBeInTheDocument()
    // 进行中：时长列 Tag；回放按钮禁用
    expect(screen.getByText('进行中')).toBeInTheDocument()
    const playing = rowOf('进行中').querySelectorAll('button')[0] as HTMLButtonElement
    expect(playing).toBeDisabled()
    expect(new Date('2026-01-02T03:04:05Z').toLocaleString()).toBeTruthy()
  })

  it('配置：异地目标非法格式标记 error 并拦截保存；合法保存 payload', async () => {
    renderPage()
    await screen.findByText('35s')
    const destInput = screen.getByPlaceholderText('gdrive:recordings')
    fireEvent.change(destInput, { target: { value: 'bad-format' } })
    // status 类加在 allowClear 的 affix 包装上而非 input 本身
    expect(destInput.closest('.ant-input-affix-wrapper')).toHaveClass('ant-input-status-error')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    expect(msgError).toHaveBeenCalledWith('异地目标需为 remote:path 形态（如 gdrive:recordings）')
    expect(apiMock.putRecordingsConfig).not.toHaveBeenCalled()
    // 改合法 + 关录制开关 → 保存派生值
    fireEvent.change(destInput, { target: { value: 'gdrive:recordings' } })
    fireEvent.click(screen.getByRole('switch'))
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() =>
      expect(apiMock.putRecordingsConfig).toHaveBeenCalledWith({
        enabled: false,
        retention_days: 7,
        remote_dest: 'gdrive:recordings',
      }))
    expect(msgSuccess).toHaveBeenCalledWith('录制配置已保存')
  })

  it('已配置异地目标：出现补推按钮且 rclone 缺失告警', async () => {
    renderPage({ enabled: true, retention_days: 7, remote_dest: 'gdrive:recordings', rclone_available: false, max_retention_days: 365 })
    await screen.findByText('35s')
    expect(screen.getByText('server 主机未检测到 rclone，推送将失败')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(rowOf('35s').querySelector('.anticon-cloud-upload')!.closest('button')!)
    })
    await waitFor(() => expect(apiMock.syncRecordingRemote).toHaveBeenCalledWith('sess-1'))
    expect(msgSuccess).toHaveBeenCalledWith('已推送到异地')
  })

  it('回放：cast 事件调度写 xterm，倍速切换重拉', async () => {
    renderPage()
    await screen.findByText('35s')
    const playBtn = () => rowOf('35s').querySelectorAll('button')[0] as HTMLButtonElement
    await act(async () => {
      fireEvent.click(playBtn())
    })
    expect(await screen.findByText(/回放：admin@10\.0\.0\.1（ssh）/)).toBeInTheDocument()
    // 首开时 Modal 内容（termDiv）挂载晚于 sessionId effect 的一帧——
    // 按现有行为先关再开才建立终端（jsdom 与浏览器同构时序，改动留观）
    await act(async () => {
      fireEvent.click(document.querySelector('.ant-modal-close')!)
    })
    await act(async () => {
      fireEvent.click(playBtn())
    })
    await waitFor(() => expect(apiMock.getRecordingCast).toHaveBeenCalledWith('sess-1'))
    await waitFor(() => expect(terminalMock.write).toHaveBeenCalledWith('hello'))
    expect(terminalMock.reset).toHaveBeenCalled()
    expect(terminalMock.open).toHaveBeenCalled()
    // 倍速切 2x → 重放（cast 重拉）
    const calls = apiMock.getRecordingCast.mock.calls.length
    await act(async () => {
      fireEvent.click(screen.getByText('2x'))
    })
    await waitFor(() =>
      expect(apiMock.getRecordingCast.mock.calls.length).toBeGreaterThan(calls))
  })

  it('下载与删除：blob 下载、Popconfirm 确认后删并失效列表', async () => {
    apiMock.downloadRecording.mockResolvedValue(new Blob(['x']))
    renderPage()
    await screen.findByText('35s')
    await act(async () => {
      fireEvent.click(rowOf('35s').querySelector('.anticon-download')!.closest('button')!)
    })
    await waitFor(() => expect(apiMock.downloadRecording).toHaveBeenCalledWith('sess-1'))
    await waitFor(() => expect(URL.createObjectURL).toHaveBeenCalled())
    await act(async () => {
      fireEvent.click(withinRow(rowOf('35s'), /删\s*除/))
    })
    expect(await screen.findByText('删除该录制？')).toBeInTheDocument()
    await act(async () => {
      fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    })
    await waitFor(() => expect(apiMock.deleteRecording).toHaveBeenCalledWith('sess-1'))
    expect(msgSuccess).toHaveBeenCalledWith('已删除')
  })

  it('下载失败 fallback 文案', async () => {
    apiMock.downloadRecording.mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByText('35s')
    await act(async () => {
      fireEvent.click(rowOf('35s').querySelector('.anticon-download')!.closest('button')!)
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('下载失败'))
  })

  it('回放：cast 加载失败提示并退出加载态', async () => {
    apiMock.getRecordingCast.mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByText('35s')
    await openPlayer('35s')
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('加载录制内容失败'))
  })

  it('回放：cast 未返回时关闭弹窗，终端释放后不再写入', async () => {
    let resolveCast!: (v: string) => void
    apiMock.getRecordingCast.mockReturnValue(new Promise<string>((r) => { resolveCast = r }))
    renderPage()
    await screen.findByText('35s')
    await openPlayer('35s')
    await waitFor(() => expect(apiMock.getRecordingCast).toHaveBeenCalledWith('sess-1'))
    await closePlayer()
    await act(async () => {
      resolveCast(castText)
    })
    expect(terminalMock.reset).not.toHaveBeenCalled()
    expect(terminalMock.write).not.toHaveBeenCalled()
  })

  it('回放：空行与坏行跳过，无有效事件不调度写入', async () => {
    apiMock.getRecordingCast.mockResolvedValue([
      '{"version":2,"width":80,"height":24}',
      '',
      'not-json',
      '[1, 2]',
      '["a", "o", "x"]',
      '   ',
    ].join('\n'))
    renderPage()
    await screen.findByText('35s')
    await openPlayer('35s')
    await waitFor(() => expect(apiMock.getRecordingCast).toHaveBeenCalledWith('sess-1'))
    expect(await screen.findByText(/进度 0s \/ 0s/)).toBeInTheDocument()
    expect(terminalMock.write).not.toHaveBeenCalled()
  })

  it('回放：零时长事件进度按 0 计', async () => {
    apiMock.getRecordingCast.mockResolvedValue([
      '{"version":2,"width":80,"height":24}',
      '[0, "o", "x"]',
    ].join('\n'))
    renderPage()
    await screen.findByText('35s')
    await openPlayer('35s')
    await waitFor(() => expect(terminalMock.write).toHaveBeenCalledWith('x'))
  })

  it('配置保存失败：提示保存失败', async () => {
    apiMock.putRecordingsConfig.mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByText('35s')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('保存失败'))
  })

  it('补推失败：提示推送失败', async () => {
    apiMock.syncRecordingRemote.mockRejectedValue(new Error('boom'))
    renderPage({ enabled: true, retention_days: 7, remote_dest: 'gdrive:recordings', rclone_available: true, max_retention_days: 365 })
    await screen.findByText('35s')
    await act(async () => {
      fireEvent.click(rowOf('35s').querySelector('.anticon-cloud-upload')!.closest('button')!)
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('推送失败'))
  })

  it('删除失败：提示删除失败', async () => {
    apiMock.deleteRecording.mockRejectedValue(new Error('boom'))
    renderPage()
    await screen.findByText('35s')
    await act(async () => {
      fireEvent.click(withinRow(rowOf('35s'), /删\s*除/))
    })
    await act(async () => {
      fireEvent.click(document.querySelector('.ant-popover .ant-btn-primary')!)
    })
    await waitFor(() => expect(msgError).toHaveBeenCalledWith('删除失败'))
  })

  it('保留天数：输入保存派生值，清空走 7 兜底', async () => {
    renderPage()
    await screen.findByText('35s')
    const spin = screen.getByRole('spinbutton') as HTMLInputElement
    fireEvent.change(spin, { target: { value: '30' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() =>
      expect(apiMock.putRecordingsConfig).toHaveBeenCalledWith({
        enabled: true,
        retention_days: 30,
        remote_dest: '',
      }))
    // 清空 → onChange(null) → v ?? 7
    fireEvent.change(spin, { target: { value: '' } })
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    })
    await waitFor(() =>
      expect(apiMock.putRecordingsConfig).toHaveBeenCalledWith({
        enabled: true,
        retention_days: 7,
        remote_dest: '',
      }))
  })

  it('列表：未知协议回退 default 色，0 字节显示 -', async () => {
    renderPage(undefined, [
      ...recordings,
      mkRec({ sessionId: 'sess-4', protocol: 'rdp', durationMs: 45000, bytes: 0 }),
    ])
    await screen.findByText('35s')
    expect(screen.getByText('rdp')).toBeInTheDocument()
    expect(screen.getByText('rdp').closest('.ant-tag')).toHaveClass('ant-tag-default')
    expect(within(rowOf('45s')).getByText('-')).toBeInTheDocument()
  })
})

const withinRow = (row: HTMLTableRowElement, text: RegExp) =>
  Array.from(row.querySelectorAll('button')).find(
    (b) => text.test(b.textContent || '')) as HTMLButtonElement
