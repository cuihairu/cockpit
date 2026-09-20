import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import DesktopModal from './index'

// DesktopModal：RDP 远程桌面集成——凭据/连接状态机/分辨率/断开/配置持久化

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

// jsdom 无 2d context / ImageData
const ctx2d = { fillStyle: '', fillRect: vi.fn(), putImageData: vi.fn() }
class FakeImageData {
  width: number
  height: number
  data: Uint8ClampedArray
  constructor(arg: number | Uint8ClampedArray, h?: number) {
    if (typeof arg === 'number') {
      this.width = arg
      this.height = h ?? 0
      this.data = new Uint8ClampedArray(arg * this.height * 4)
    } else {
      this.data = arg
      this.width = 0
      this.height = 0
    }
  }
}

const msgInfo = vi.spyOn(message, 'info')
const msgError = vi.spyOn(message, 'error')

const props = {
  visible: true,
  onClose: vi.fn(),
  agentId: 'ag1',
  host: '10.0.0.9',
  port: 3389,
}

const clickConnect = () => fireEvent.click(screen.getByText('连 接').closest('button')!)

const push = async (msg: Record<string, unknown>) => {
  const ws = FakeWebSocket.instances[FakeWebSocket.instances.length - 1]
  await act(async () => {
    ws.onmessage!({ data: JSON.stringify(msg) })
  })
}

describe('DesktopModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    FakeWebSocket.instances = []
    localStorage.clear()
    createRemoteTicket.mockReset()
    createRemoteTicket.mockResolvedValue({ ticket: 'tk', expiresAt: '' })
    vi.stubGlobal('WebSocket', FakeWebSocket)
    vi.stubGlobal('ImageData', FakeImageData)
    HTMLCanvasElement.prototype.getContext = vi.fn(() => ctx2d) as never
  })

  it('初始渲染凭据 Modal；无存量不预填', () => {
    render(<DesktopModal {...props} />)
    expect(screen.getByText('RDP 远程桌面 - 10.0.0.9:3389')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('administrator')).toHaveValue('')
    expect(screen.getByPlaceholderText('(可选)')).toHaveValue('')
  })

  it('有存量配置自动预填用户名/域（不含密码）', () => {
    localStorage.setItem('desktop_configs', JSON.stringify([{
      id: 'c1', lastUsed: 1,
      name: 'n', agentId: 'ag1', host: '10.0.0.9', port: 3389,
      username: 'admin', domain: 'CORP', width: 1280, height: 800,
    }]))
    render(<DesktopModal {...props} />)
    expect(screen.getByPlaceholderText('administrator')).toHaveValue('admin')
    expect(screen.getByPlaceholderText('(可选)')).toHaveValue('CORP')
  })

  it('点连接保存配置（无密码）→ WS 建立进入 connecting', async () => {
    render(<DesktopModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), { target: { value: 'u1' } })
    fireEvent.change(screen.getByPlaceholderText('password'), { target: { value: 'secret' } })
    clickConnect()
    await waitFor(() => expect(FakeWebSocket.instances.length).toBe(1))
    expect(screen.getByText(/正在连接到 10\.0\.0\.9:3389/)).toBeInTheDocument()
    const saved = JSON.parse(localStorage.getItem('desktop_configs')!)
    expect(saved[0].username).toBe('u1')
    expect(JSON.stringify(saved)).not.toContain('secret')
  })

  it('connected 切桌面分支：canvas 挂载、toolbar 已连接、分辨率文本', async () => {
    render(<DesktopModal {...props} />)
    clickConnect()
    await waitFor(() => expect(FakeWebSocket.instances.length).toBe(1))
    await push({ type: 'connected', width: 1920, height: 1080 })
    expect(screen.getByText('已连接')).toBeInTheDocument()
    expect(screen.getByText('1920x1080')).toBeInTheDocument()
    expect(document.querySelector('canvas')).not.toBeNull()
  })

  it('分辨率下拉发 set_resolution 并更新文本', async () => {
    render(<DesktopModal {...props} />)
    clickConnect()
    await waitFor(() => expect(FakeWebSocket.instances.length).toBe(1))
    await push({ type: 'connected', width: 1280, height: 800 })
    fireEvent.click(screen.getByText('分辨率'))
    fireEvent.click((await screen.findAllByText('1920 x 1080'))[0])
    await waitFor(() => {
      expect(FakeWebSocket.instances[0].sent).toContain(
        JSON.stringify({ type: 'set_resolution', width: 1920, height: 1080 }),
      )
    })
    expect(screen.getByText('1920x1080')).toBeInTheDocument()
  })

  it('disconnected 消息提示 reason；error 消息报错', async () => {
    render(<DesktopModal {...props} />)
    clickConnect()
    await waitFor(() => expect(FakeWebSocket.instances.length).toBe(1))
    await push({ type: 'connected', width: 1, height: 1 })
    await push({ type: 'disconnected', reason: 'bye' })
    expect(msgInfo).toHaveBeenCalledWith('连接断开: bye')
    await push({ type: 'error', error: 'boom' })
    expect(msgError).toHaveBeenCalledWith('远程桌面错误: boom')
  })

  it('clipboard_data 写本地剪贴板', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    render(<DesktopModal {...props} />)
    clickConnect()
    await waitFor(() => expect(FakeWebSocket.instances.length).toBe(1))
    await push({ type: 'connected', width: 1, height: 1 })
    await push({ type: 'clipboard_data', text: 'copied' })
    expect(writeText).toHaveBeenCalledWith('copied')
  })

  it('断开按钮回凭据表单；关闭回调 onClose', async () => {
    render(<DesktopModal {...props} />)
    clickConnect()
    await waitFor(() => expect(FakeWebSocket.instances.length).toBe(1))
    await push({ type: 'connected', width: 1, height: 1 })
    // RemoteToolbar 断开为 icon-only：工具栏最后一个按钮
    const buttons = document.querySelectorAll('button')
    fireEvent.click(buttons[buttons.length - 1])
    expect(screen.getByPlaceholderText('administrator')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /close/i }))
    await waitFor(() => expect(props.onClose).toHaveBeenCalledTimes(1))
  })
})
