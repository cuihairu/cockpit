import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GuacamoleModal from './index'
import * as desktop from '@/services/desktop'
import { message as messageError } from 'antd'

// GuacamoleModal：guacamole-common-js 承担渲染/输入/剪贴板，本组件只管
// 「申请票据 → 建隧道 → 生命周期」。票据走 URL query（common-js 硬编码
// subprotocol "guacamole"），凭据只进 ticket 不进 URL（见设计 D3）。

const created: Array<{ url?: string; connectData?: string }> = []
const readers: Array<{ ondata?: (c: string) => void; onend?: () => void }> = []
const mice: Array<Record<string, unknown>> = []
const keyboards: Array<Record<string, unknown>> = []
// 超时回调手动触发（start 只登记不启动，避免污染其他用例）
const timeoutCtl = { onTimeout: null as (() => void) | null }
const clientMock = vi.hoisted(() => ({
  connect: vi.fn((data?: string) => {
    created.push({ connectData: data })
  }),
  disconnect: vi.fn(),
  getDisplay: vi.fn(),
  sendMouseState: vi.fn(),
  sendKeyEvent: vi.fn(),
  sendSize: vi.fn(),
  onclipboard: undefined as unknown,
  onerror: undefined as unknown,
  onstatechange: undefined as unknown,
}))

vi.mock('guacamole-common-js', () => ({
  default: {
    AudioContextFactory: {
      getAudioContext: () => acMock,
    },
    WebSocketTunnel: vi.fn(function (url: string) {
      created.push({ url })
      return {}
    }),
    Client: vi.fn(function () {
      return clientMock
    }),
    Mouse: vi.fn(function () {
      const inst: Record<string, unknown> = {}
      mice.push(inst)
      return inst
    }),
    Keyboard: vi.fn(function () {
      const inst: Record<string, unknown> = {}
      keyboards.push(inst)
      return inst
    }),
    StringReader: vi.fn(function () {
      const inst: { ondata?: (c: string) => void; onend?: () => void } = {}
      readers.push(inst)
      return inst
    }),
  },
}))

const acMock = vi.hoisted(() => ({
  suspend: vi.fn().mockResolvedValue(undefined),
  resume: vi.fn().mockResolvedValue(undefined),
  state: 'running',
}))

const createRemoteTicket = vi.hoisted(() => vi.fn())
vi.mock('@/services/remote', () => ({ createRemoteTicket }))

vi.mock('@/hooks/useConnectionTimeout', () => ({
  useConnectionTimeout: ({ onTimeout }: { onTimeout: () => void }) => ({
    start: () => {
      timeoutCtl.onTimeout = onTimeout
    },
    clear: vi.fn(),
    reset: vi.fn(),
  }),
}))

const msgError = vi.spyOn(console, 'error').mockImplementation(() => {})
const msgMessageError = vi.spyOn(messageError, 'error')

const props = {
  visible: true,
  onClose: vi.fn(),
  agentId: 'ag1',
  host: '10.0.0.9',
  port: 3389,
  protocol: 'rdp' as const,
}

describe('GuacamoleModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    created.length = 0
    readers.length = 0
    mice.length = 0
    keyboards.length = 0
    timeoutCtl.onTimeout = null
    msgMessageError.mockClear()
    clientMock.getDisplay.mockReturnValue({ getElement: () => document.createElement('canvas') })
    acMock.suspend.mockClear()
    acMock.resume.mockClear()
    createRemoteTicket.mockResolvedValue({ ticket: 'tk-1', expiresAt: '' })
  })

  it('初始出凭据表单（RDP：用户名必填 + 密码/域），不建隧道', () => {
    render(<GuacamoleModal {...props} />)
    expect(screen.getByText('用户名')).toBeInTheDocument()
    expect(screen.getByText('密码')).toBeInTheDocument()
    expect(screen.getByText('域')).toBeInTheDocument()
    expect(createRemoteTicket).not.toHaveBeenCalled()
    expect(clientMock.connect).not.toHaveBeenCalled()
  })

  it('凭据阶段「取消」：handleClose 清理并通知 onClose', () => {
    render(<GuacamoleModal {...props} />)
    fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }))
    expect(props.onClose).toHaveBeenCalledTimes(1)
    expect(createRemoteTicket).not.toHaveBeenCalled()
  })

  it('VNC 只出密码框', () => {
    render(<GuacamoleModal {...props} protocol="vnc" port={5900} />)
    expect(screen.getByText('VNC 密码')).toBeInTheDocument()
    expect(screen.queryByText('用户名')).not.toBeInTheDocument()
  })

  it('空用户名拦截，不取票据', async () => {
    render(<GuacamoleModal {...props} />)
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    expect(await screen.findByText('请输入用户名')).toBeInTheDocument()
    expect(createRemoteTicket).not.toHaveBeenCalled()
  })

  it('提交凭据：票据带凭据、connect 落 URL query、Display 挂载', async () => {
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.change(screen.getByPlaceholderText('password'), {
      target: { value: 'pw' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())

    // 凭据只进 ticket（服务端放进 guacd 的 connect 指令），浏览器侧零凭据
    expect(createRemoteTicket).toHaveBeenCalledWith(
      expect.objectContaining({
        agentId: 'ag1',
        host: '10.0.0.9',
        port: 3389,
        protocol: 'rdp',
        username: 'alice',
        password: 'pw',
      }),
    )
    // connect(data) 落到 WS URL query（common-js 硬编码 subprotocol）
    expect(clientMock.connect).toHaveBeenCalledWith('ticket=tk-1')
    expect(clientMock.getDisplay).toHaveBeenCalled()
  })

  it('票据失败回凭据表单并提示', async () => {
    createRemoteTicket.mockRejectedValue(new Error('ticket down'))
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(screen.getByText('ticket down')).toBeInTheDocument())
    expect(screen.getByText('用户名')).toBeInTheDocument()
  })

  it('分辨率切换发 sendSize；断开按钮 disconnect 并回表单', async () => {
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())

    // 分辨率下拉（RemoteToolbar 的 Select）：与 DesktopModal 测试同款交互
    fireEvent.click(screen.getByText('分辨率'))
    fireEvent.click((await screen.findAllByText('1920 x 1080'))[0])
    await waitFor(() => expect(clientMock.sendSize).toHaveBeenCalledWith(1920, 1080))

    // RemoteToolbar 断开按钮为 icon-only：取工具栏最后一个按钮
    const btns = document.querySelectorAll('button')
    fireEvent.click(btns[btns.length - 1])
    await waitFor(() => expect(clientMock.disconnect).toHaveBeenCalled())
    expect(screen.getByText('用户名')).toBeInTheDocument()
  })

  it('onerror 断开回表单并提示；卸载清理 disconnect', async () => {
    const { unmount } = render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())

    await act(async () => {
      ;(clientMock.onerror as (s: { message: string }) => void)({ message: 'boom' })
    })
    expect(screen.getByText('用户名')).toBeInTheDocument()

    clientMock.disconnect.mockClear()
    unmount()
    expect(clientMock.disconnect).toHaveBeenCalled()
    void msgError
  })

  it('visible=false：不读最近配置、不取票据', () => {
    const recentSpy = vi.spyOn(desktop, 'getRecentDesktopConfig')
    render(<GuacamoleModal {...props} visible={false} />)
    expect(recentSpy).not.toHaveBeenCalled()
    expect(createRemoteTicket).not.toHaveBeenCalled()
    recentSpy.mockRestore()
  })

  it('VNC 提交：username 缺省以空串落最近配置', async () => {
    localStorage.clear()
    render(<GuacamoleModal {...props} protocol="vnc" port={5900} />)
    fireEvent.change(screen.getByPlaceholderText('(可选) VNC 密码'), { target: { value: 'pw' } })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())
    await waitFor(() => {
      const saved = JSON.parse(localStorage.getItem('desktop_configs') ?? '[]') as Array<{ username: string }>
      expect(saved.some((c) => c.username === '')).toBe(true)
    })
  })

  it('onerror 无 message：兜底文案「远程桌面连接失败」', async () => {
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), { target: { value: 'alice' } })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())
    await act(async () => {
      ;(clientMock.onerror as (s: unknown) => void)(undefined)
    })
    expect(screen.getByText('远程桌面连接失败')).toBeInTheDocument()
    expect(msgMessageError).toHaveBeenCalledWith('远程桌面连接失败')
  })

  it('票据拒绝非 Error：兜底文案「连接失败」', async () => {
    createRemoteTicket.mockRejectedValue('plain-string-reason')
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), { target: { value: 'alice' } })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    expect(await screen.findByText('连接失败')).toBeInTheDocument()
    expect(msgMessageError).toHaveBeenCalledWith('连接失败')
  })

  it('https 页面隧道走 wss', async () => {
    vi.stubGlobal('location', { protocol: 'https:' })
    try {
      render(<GuacamoleModal {...props} />)
      fireEvent.change(screen.getByPlaceholderText('administrator'), { target: { value: 'alice' } })
      fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
      await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())
      expect(created.some((c) => c.url?.startsWith('wss://'))).toBe(true)
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('静音按钮：suspend/resume 切换（AudioContext 单例全局静音）', async () => {
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())

    // RemoteToolbar 的静音按钮（extraActions）
    fireEvent.click(screen.getByRole('button', { name: /静\s*音/ }))
    await waitFor(() => expect(acMock.suspend).toHaveBeenCalled())

    fireEvent.click(screen.getByRole('button', { name: /取消静音/ }))
    await waitFor(() => expect(acMock.resume).toHaveBeenCalled())
  })

  it('连接超时：清理并回凭据表单且提示', async () => {
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())
    expect(timeoutCtl.onTimeout).not.toBeNull()

    await act(async () => {
      timeoutCtl.onTimeout!()
    })
    expect(clientMock.disconnect).toHaveBeenCalled()
    expect(msgMessageError).toHaveBeenCalledWith('连接超时，请检查网络或重试')
    expect(screen.getByText('用户名')).toBeInTheDocument()
  })

  it('onclipboard：text/plain 聚合写剪贴板，非文本忽略', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())
    const onclipboard = clientMock.onclipboard as (s: unknown[], m: string) => void

    onclipboard([{}], 'image/png')
    expect(readers.length).toBe(0)

    onclipboard([{}], 'text/plain')
    expect(readers.length).toBe(1)
    readers[0].ondata!('hel')
    readers[0].ondata!('lo')
    readers[0].onend!()
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('hello'))
  })

  it('Mouse/Keyboard 事件转发 sendMouseState/sendKeyEvent', async () => {
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())

    expect(mice.length).toBe(1)
    const mouse = mice[0] as { onmousedown?: (s: unknown) => void }
    mouse.onmousedown!({ x: 1, y: 2 })
    expect(clientMock.sendMouseState).toHaveBeenCalledWith({ x: 1, y: 2 })

    expect(keyboards.length).toBe(1)
    const kb = keyboards[0] as { onkeydown?: (k: number) => void; onkeyup?: (k: number) => void }
    kb.onkeydown!(65)
    expect(clientMock.sendKeyEvent).toHaveBeenCalledWith(1, 65)
    kb.onkeyup!(65)
    expect(clientMock.sendKeyEvent).toHaveBeenCalledWith(0, 65)
  })

  it('断开清理：display 容器注入的元素被移除', async () => {
    const displayEl = document.createElement('canvas')
    clientMock.getDisplay.mockReturnValue({ getElement: () => displayEl })
    // 手动 resolve：等待桌面分支（displayRef 挂载）提交后再放行票据，
    // 否则微任务先于 React 提交，appendChild 静默跳过
    let resolveTicket!: (v: { ticket: string }) => void
    createRemoteTicket.mockImplementation(
      () => new Promise((r) => { resolveTicket = r }),
    )
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    expect(await screen.findByText(/正在连接到 10\.0\.0\.9:3389/)).toBeInTheDocument()
    resolveTicket({ ticket: 'tk-1' })
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())
    expect(displayEl.parentNode).not.toBeNull()

    const btns = document.querySelectorAll('button')
    fireEvent.click(btns[btns.length - 1])
    await waitFor(() => expect(clientMock.disconnect).toHaveBeenCalled())
    expect(displayEl.parentNode).toBeNull()
  })

  it('全屏切换两分支；全屏态断开退出全屏', async () => {
    const requestFullscreen = vi.fn()
    const exitFullscreen = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(document, 'exitFullscreen', { value: exitFullscreen, configurable: true })
    render(<GuacamoleModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('administrator'), {
      target: { value: 'alice' },
    })
    fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
    await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())

    const container = document.querySelector('.ant-modal-body > div') as HTMLElement
    Object.defineProperty(container, 'requestFullscreen', { value: requestFullscreen, configurable: true })

    // 进入全屏分支
    fireEvent.click(document.querySelector('.ant-modal button .anticon-fullscreen')!.closest('button')!)
    expect(requestFullscreen).toHaveBeenCalled()

    // 退出全屏分支
    await act(async () => {
      Object.defineProperty(document, 'fullscreenElement', { value: document.body, configurable: true })
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    fireEvent.click(
      document.querySelector('.ant-modal button .anticon-fullscreen-exit')!.closest('button')!,
    )
    await waitFor(() => expect(exitFullscreen).toHaveBeenCalled())

    // 再进全屏后点断开：handleDisconnect 的 isFullscreen 分支
    await act(async () => {
      Object.defineProperty(document, 'fullscreenElement', { value: document.body, configurable: true })
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    const btns = document.querySelectorAll('button')
    fireEvent.click(btns[btns.length - 1])
    await waitFor(() => expect(exitFullscreen).toHaveBeenCalledTimes(2))
    expect(screen.getByText('用户名')).toBeInTheDocument()
    Object.defineProperty(document, 'fullscreenElement', { value: null, configurable: true })
    document.dispatchEvent(new Event('fullscreenchange'))
  })

  it('AudioContext 不可用（jsdom/旧浏览器）静音不炸', async () => {
    const Guacamole = (await import('guacamole-common-js')).default
    const orig = Guacamole.AudioContextFactory
    ;(Guacamole as unknown as { AudioContextFactory: unknown }).AudioContextFactory = {
      getAudioContext: () => null,
    }
    try {
      render(<GuacamoleModal {...props} />)
      fireEvent.change(screen.getByPlaceholderText('administrator'), {
        target: { value: 'alice' },
      })
      fireEvent.click(screen.getByRole('button', { name: /连\s*接/ }))
      await waitFor(() => expect(clientMock.connect).toHaveBeenCalled())
      fireEvent.click(screen.getByRole('button', { name: /静\s*音/ }))
      // 不抛错即可（无 ctx 时静默返回）
    } finally {
      ;(Guacamole as unknown as { AudioContextFactory: unknown }).AudioContextFactory = orig
    }
  })
})
