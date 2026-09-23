import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GuacamoleModal from './index'

// GuacamoleModal：guacamole-common-js 承担渲染/输入/剪贴板，本组件只管
// 「申请票据 → 建隧道 → 生命周期」。票据走 URL query（common-js 硬编码
// subprotocol "guacamole"），凭据只进 ticket 不进 URL（见设计 D3）。

const created: Array<{ url?: string; connectData?: string }> = []
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
    WebSocketTunnel: vi.fn(function (url: string) {
      created.push({ url })
      return {}
    }),
    Client: vi.fn(function () {
      return clientMock
    }),
    Mouse: vi.fn(function () {
      return {}
    }),
    Keyboard: vi.fn(function () {
      return {}
    }),
    StringReader: vi.fn(function () {
      return { ondata: undefined, onend: undefined }
    }),
  },
}))

const createRemoteTicket = vi.hoisted(() => vi.fn())
vi.mock('@/services/remote', () => ({ createRemoteTicket }))

const msgError = vi.spyOn(console, 'error').mockImplementation(() => {})

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
    clientMock.getDisplay.mockReturnValue({ getElement: () => document.createElement('canvas') })
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
})
