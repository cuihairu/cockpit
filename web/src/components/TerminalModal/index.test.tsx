import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalModal from './index'

// TerminalModal：xterm + WebSocket 远程终端——票据/消息分支/输入转发/清理

const terminalMock = vi.hoisted(() => ({
  loadAddon: vi.fn(),
  open: vi.fn(),
  writeln: vi.fn(),
  write: vi.fn(),
  onData: vi.fn(),
  reset: vi.fn(),
  dispose: vi.fn(),
  rows: 24,
  cols: 80,
}))
const fitMock = vi.hoisted(() => ({ fit: vi.fn() }))
// new Terminal() 须返回 mock 对象：普通 function 构造器显式 return
vi.mock('@xterm/xterm', () => ({ Terminal: vi.fn(function () { return terminalMock }) }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: vi.fn(function () { return fitMock }) }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: vi.fn(function () { return {} }) }))

const createRemoteTicket = vi.hoisted(() => vi.fn())
vi.mock('@/services/remote', () => ({ createRemoteTicket }))

class FakeWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static instances: FakeWebSocket[] = []
  readyState = FakeWebSocket.CONNECTING
  sent: string[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null
  constructor(
    public url: string,
    public protocols?: string[] | string,
  ) {
    FakeWebSocket.instances.push(this)
  }
  send(data: string) {
    this.sent.push(data)
  }
  close = vi.fn(() => {
    this.readyState = 3
  })
}

const flush = () => act(async () => {
  await new Promise((r) => setTimeout(r, 0))
})

describe('TerminalModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    FakeWebSocket.instances = []
    createRemoteTicket.mockReset()
    createRemoteTicket.mockResolvedValue({ ticket: 'tk1', expiresAt: '' })
    vi.stubGlobal('WebSocket', FakeWebSocket)
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      unobserve() {}
      disconnect() {}
    })
  })

  const props = {
    visible: true,
    onClose: vi.fn(),
    agentId: 'ag1',
    host: '10.0.0.1',
    port: 22,
    protocol: 'ssh' as const,
    // SSH 需要认证凭据；传入 username 跳过凭据表单直接连接
    username: 'testuser',
    password: 'testpass',
  }

  it('打开即建终端、输出欢迎语并凭票据连 WS', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    expect(terminalMock.open).toHaveBeenCalled()
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('Cockpit 远程终端'))
    expect(createRemoteTicket).toHaveBeenCalledWith({
      agentId: 'ag1', host: '10.0.0.1', port: 22, protocol: 'ssh',
      username: 'testuser', password: 'testpass',
    })
    const ws = FakeWebSocket.instances[0]
    expect(ws.url).toBe('ws://localhost:3000/api/remote/terminal')
    expect(ws.protocols).toEqual(['tk1'])
  })

  it('票据失败输出错误并保持未连接', async () => {
    createRemoteTicket.mockRejectedValue(new Error('no'))
    render(<TerminalModal {...props} />)
    await flush()
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('创建连接票据失败'))
    expect(FakeWebSocket.instances).toHaveLength(0)
    expect(screen.getByText(/重\s*连/)).toBeInTheDocument()
  })

  it('onopen 置已连接；data 消息写入终端', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
    })
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('连接成功'))
    expect(screen.getByText(/已\s*连接/).closest('button')!.disabled).toBe(true)

    terminalMock.write.mockClear()
    await act(async () => {
      ws.onmessage!({ data: JSON.stringify({ type: 'data', data: 'hello' }) })
      ws.onmessage!({ data: JSON.stringify({ type: 'resize' }) })
      ws.onmessage!({ data: JSON.stringify({ type: 'error', message: 'boom' }) })
      ws.onmessage!({ data: '{broken' })
    })
    expect(terminalMock.write).toHaveBeenCalledWith('hello')
    expect(fitMock.fit).toHaveBeenCalled()
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('错误: boom'))
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('消息解析错误'))
  })

  it('close 消息置断开；终端输入在 OPEN 时经 WS 发送', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
      ws.onmessage!({ data: JSON.stringify({ type: 'close' }) })
    })
    expect(screen.getByText(/重\s*连/)).toBeInTheDocument()

    const onDataCb = terminalMock.onData.mock.calls[0][0] as (d: string) => void
    await act(async () => {
      onDataCb('ls\n')
    })
    expect(ws.sent).toContain(JSON.stringify({ type: 'input', data: 'ls\n' }))
  })

  it('卸载清理：关 WS、dispose 终端', async () => {
    const { unmount } = render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    unmount()
    expect(ws.close).toHaveBeenCalled()
    expect(terminalMock.dispose).toHaveBeenCalled()
  })

  it('「重连」重置终端并再次取票据', async () => {
    createRemoteTicket.mockRejectedValueOnce(new Error('first fails'))
    render(<TerminalModal {...props} />)
    await flush()
    fireEvent.click(screen.getByText(/重\s*连/))
    await flush()
    expect(terminalMock.reset).toHaveBeenCalled()
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('正在重连'))
    expect(createRemoteTicket).toHaveBeenCalledTimes(2)
  })

  it('ws.onerror 置断开并输出连接错误', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
      ws.onerror!()
    })
    expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('连接错误'))
    expect(screen.getByText(/重\s*连/)).toBeInTheDocument()
  })

  it('ws.onclose 置断开（不覆盖文案）', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
      ws.onclose!()
    })
    expect(screen.getByText(/重\s*连/)).toBeInTheDocument()
  })

  it('连接超时：CONNECTING 状态下 30s 后关 WS 并提示', async () => {
    vi.useFakeTimers()
    try {
      render(<TerminalModal {...props} />)
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30_000)
      })
      const ws = FakeWebSocket.instances[0]
      expect(ws.close).toHaveBeenCalled()
      expect(terminalMock.writeln).toHaveBeenCalledWith(expect.stringContaining('连接超时'))
    } finally {
      vi.useRealTimers()
    }
  })

  it('visible=false 时清理 WS（effect cleanup）', async () => {
    const { rerender } = render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
    })
    expect(screen.getByText(/已\s*连接/)).toBeInTheDocument()
    rerender(<TerminalModal {...props} visible={false} />)
    // 第一个 useEffect 的 cleanup 负责关 WS + dispose 终端
    expect(ws.close).toHaveBeenCalled()
    expect(terminalMock.dispose).toHaveBeenCalled()
  })

  it('终端输入在非 OPEN 状态不发送', async () => {
    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    const onDataCb = terminalMock.onData.mock.calls[0][0] as (d: string) => void
    await act(async () => {
      onDataCb('ls\n') // CONNECTING 状态
    })
    expect(ws.sent).toHaveLength(0)
  })

  // ============ SSH 凭据表单 ============

  it('SSH 无 username：先出凭据表单，提交后才建终端', async () => {
    render(<TerminalModal {...props} username={undefined} password={undefined} />)
    // 表单阶段不建终端、不取票据
    expect(terminalMock.open).not.toHaveBeenCalled()
    expect(createRemoteTicket).not.toHaveBeenCalled()
    expect(screen.getByText('用户名')).toBeInTheDocument()
    expect(screen.getByText('口令')).toBeInTheDocument()

    fireEvent.change(screen.getByPlaceholderText('root'), { target: { value: 'alice' } })
    fireEvent.change(screen.getByPlaceholderText('登录口令'), { target: { value: 'pw' } })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    // antd Form onFinish 异步 → effect 建终端，等重渲染
    await waitFor(() => expect(terminalMock.open).toHaveBeenCalled())
    expect(createRemoteTicket).toHaveBeenCalledWith({
      agentId: 'ag1', host: '10.0.0.1', port: 22, protocol: 'ssh',
      username: 'alice', password: 'pw',
    })
  })

  it('凭据表单空用户名拦截，不建终端', async () => {
    render(<TerminalModal {...props} username={undefined} password={undefined} />)
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    expect(await screen.findByText('请输入用户名')).toBeInTheDocument()
    expect(terminalMock.open).not.toHaveBeenCalled()
    expect(createRemoteTicket).not.toHaveBeenCalled()
  })

  it('telnet 免认证：无 username 也直接建终端（不进凭据表单）', async () => {
    render(<TerminalModal {...props} protocol="telnet" username={undefined} password={undefined} />)
    await flush()
    expect(screen.queryByText('用户名')).not.toBeInTheDocument()
    expect(terminalMock.open).toHaveBeenCalled()
    expect(createRemoteTicket).toHaveBeenCalledWith({
      agentId: 'ag1', host: '10.0.0.1', port: 22, protocol: 'telnet',
    })
  })

  // ============ PTY 尺寸上报 ============

  it('窗口变化发 resize（rows/cols）到服务端', async () => {
    // ResizeObserver 手工驱动，验证 fit + resize 上报
    let roCb: ResizeObserverCallback | null = null
    vi.stubGlobal('ResizeObserver', class {
      constructor(cb: ResizeObserverCallback) { roCb = cb }
      observe() {}
      unobserve() {}
      disconnect() {}
    })

    render(<TerminalModal {...props} />)
    await flush()
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.readyState = FakeWebSocket.OPEN
      ws.onopen!()
    })
    ws.sent.length = 0

    await act(async () => {
      roCb?.([], {} as ResizeObserver)
    })
    expect(fitMock.fit).toHaveBeenCalled()
    expect(ws.sent).toContain(JSON.stringify({ type: 'resize', rows: 24, cols: 80 }))
  })
})
