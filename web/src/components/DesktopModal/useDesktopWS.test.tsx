import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDesktopWS } from './useDesktopWS'

// useDesktopWS：RDP 桌面 WebSocket——票据/消息分发/输入发送/断开清理

const createRemoteTicket = vi.hoisted(() => vi.fn())
vi.mock('@/services/remote', () => ({ createRemoteTicket }))

class FakeWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static instances: FakeWebSocket[] = []
  readyState = FakeWebSocket.OPEN
  binaryType = ''
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
  close = vi.fn()
}

const connectParams = {
  agentId: 'ag1', host: '10.0.0.1', port: 3389, username: 'u', password: 'p',
}

describe('useDesktopWS', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    FakeWebSocket.instances = []
    createRemoteTicket.mockReset()
    createRemoteTicket.mockResolvedValue({ ticket: 'tk', expiresAt: '' })
    vi.stubGlobal('WebSocket', FakeWebSocket)
  })

  const setup = () => {
    const handlers = {
      onConnected: vi.fn(),
      onDisconnected: vi.fn(),
      onError: vi.fn(),
      onScreenUpdate: vi.fn(),
      onClipboard: vi.fn(),
    }
    let api!: ReturnType<typeof useDesktopWS>
    const Probe = () => {
      api = useDesktopWS(handlers)
      return <span data-testid="state">{api.state}</span>
    }
    render(<Probe />)
    return { handlers, get: () => api }
  }

  const connect = async (api: ReturnType<typeof useDesktopWS>) => {
    await act(async () => {
      await api.connect(connectParams)
    })
  }

  const push = async (msg: Record<string, unknown>) => {
    const ws = FakeWebSocket.instances[0]
    await act(async () => {
      ws.onmessage!({ data: JSON.stringify(msg) })
    })
  }

  it('connect 取票据并建 WS（rdp 协议 + 票据子协议 + arraybuffer）', async () => {
    const { get } = setup()
    await connect(get())
    expect(createRemoteTicket).toHaveBeenCalledWith({ ...connectParams, protocol: 'rdp' })
    const ws = FakeWebSocket.instances[0]
    expect(ws.url).toBe('ws://localhost:3000/api/remote/desktop')
    expect(ws.protocols).toEqual(['tk'])
    expect(ws.binaryType).toBe('arraybuffer')
  })

  it('票据失败 onError 并回 disconnected', async () => {
    createRemoteTicket.mockRejectedValue(new Error('no'))
    const { handlers, get } = setup()
    await connect(get())
    expect(handlers.onError).toHaveBeenCalledWith('Failed to create connection ticket')
  })

  it('connected 消息置态并回调宽高（缺省 1280x800）；desktopType 字段兼容', async () => {
    const { handlers, get } = setup()
    await connect(get())
    await push({ type: 'connected', width: 1920, height: 1080 })
    expect(handlers.onConnected).toHaveBeenCalledWith(1920, 1080)
    expect(document.querySelector('[data-testid="state"]')!.textContent).toBe('connected')

    await push({ desktopType: 'connected' })
    expect(handlers.onConnected).toHaveBeenCalledWith(1280, 800)
  })

  it('screen_update 传递 rects；非数组回退空', async () => {
    const { handlers, get } = setup()
    await connect(get())
    await push({ type: 'screen_update', width: 100, height: 50, rects: [{ x: 0, y: 0, width: 1, height: 1, data: '' }] })
    expect(handlers.onScreenUpdate).toHaveBeenCalledWith({
      width: 100, height: 50, rects: [{ x: 0, y: 0, width: 1, height: 1, data: '' }],
    })
    await push({ type: 'screen_update', rects: 'bad' })
    expect(handlers.onScreenUpdate).toHaveBeenLastCalledWith(
      expect.objectContaining({ rects: [] }),
    )
  })

  it('disconnected/error 消息带缺省文案；clipboard_data 透传', async () => {
    const { handlers, get } = setup()
    await connect(get())
    await push({ type: 'disconnected' })
    expect(handlers.onDisconnected).toHaveBeenCalledWith('Disconnected')
    await push({ type: 'error' })
    expect(handlers.onError).toHaveBeenCalledWith('Unknown error')
    await push({ type: 'clipboard_data', text: 'hello' })
    expect(handlers.onClipboard).toHaveBeenCalledWith('hello')
  })

  it('connecting/ping 消息无副作用', async () => {
    const { handlers, get } = setup()
    await connect(get())
    await push({ type: 'connecting' })
    await push({ type: 'ping' })
    expect(handlers.onConnected).not.toHaveBeenCalled()
    expect(handlers.onError).not.toHaveBeenCalled()
  })

  it('ws.onerror 置 disconnected 并回调', async () => {
    const { handlers, get } = setup()
    await connect(get())
    await act(async () => {
      FakeWebSocket.instances[0].onerror!()
    })
    expect(handlers.onError).toHaveBeenCalledWith('Connection error')
  })

  it('sendKeyboard/Mouse/Clipboard/SetResolution 经 OPEN 的 WS 发 JSON', async () => {
    const { get } = setup()
    await connect(get())
    const api = get()
    await act(async () => {
      api.sendKeyboard(30, true, false)
      api.sendMouse(10, 20, 1, 0, 'down')
      api.sendClipboard('txt')
      api.sendSetResolution(1024, 768)
    })
    const sent = FakeWebSocket.instances[0].sent.map((s) => JSON.parse(s))
    expect(sent).toContainEqual({ type: 'keyboard', scanCode: 30, keyDown: true, extended: false })
    expect(sent).toContainEqual({ type: 'mouse', x: 10, y: 20, buttons: 1, wheelDelta: 0, action: 'down' })
    expect(sent).toContainEqual({ type: 'clipboard', text: 'txt' })
    expect(sent).toContainEqual({ type: 'set_resolution', width: 1024, height: 768 })
  })

  it('WS 非 OPEN 时 send 系列静默', async () => {
    const { get } = setup()
    await connect(get())
    FakeWebSocket.instances[0].readyState = FakeWebSocket.CONNECTING
    await act(async () => {
      get().sendKeyboard(1, true, false)
    })
    expect(FakeWebSocket.instances[0].sent).toHaveLength(0)
  })

  it('disconnect 关 WS 置态；卸载自动关', async () => {
    const { get, unmount } = (() => {
      const r = setup()
      return { get: r.get, unmount: () => {} }
    })()
    void unmount
    await connect(get())
    await act(async () => {
      get().disconnect()
    })
    expect(FakeWebSocket.instances[0].close).toHaveBeenCalled()
    expect(document.querySelector('[data-testid="state"]')!.textContent).toBe('disconnected')
  })
})
