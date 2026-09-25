import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { message } from 'antd'
import VNCModal from './index'

// antd message 静态方法在 jsdom 下不渲染 notice——spy 断言调用参数
const msgError = vi.spyOn(message, 'error')

// VNCModal：noVNC RFB 封装——凭据表单/连接状态机/断开清理/CAD

// mock 工厂被 hoist，类定义须放 vi.hoisted 内
const { FakeRFB } = vi.hoisted(() => {
  class FakeRFB {
    static instances: FakeRFB[] = []
    static throwError = false
    listeners: Record<string, Array<(e: { detail?: unknown }) => void>> = {}
    scaleViewport = false
    resizeSession = false
    sendCtrlAltDel = vi.fn()
    sendCredentials = vi.fn()
    disconnect = vi.fn()
    constructor(
      public target: HTMLElement,
      public url: string,
      public opts: unknown,
    ) {
      if (FakeRFB.throwError) throw new Error('RFB boom')
      FakeRFB.instances.push(this)
    }
    addEventListener(type: string, cb: (e: { detail?: unknown }) => void) {
      ;(this.listeners[type] ??= []).push(cb)
    }
    emit(type: string, detail?: unknown) {
      for (const cb of this.listeners[type] ?? []) cb({ detail })
    }
  }
  return { FakeRFB }
})

const createRemoteTicket = vi.hoisted(() => vi.fn())
vi.mock('@/services/remote', () => ({ createRemoteTicket }))
vi.mock('@novnc/novnc', () => ({ default: FakeRFB }))

describe('VNCModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    msgError.mockClear()
    FakeRFB.instances = []
    createRemoteTicket.mockReset()
    createRemoteTicket.mockResolvedValue({ ticket: 'tk', expiresAt: '' })
  })

  const props = {
    visible: true,
    onClose: vi.fn(),
    agentId: 'ag1',
    host: '10.0.0.1',
    port: 5900,
  }

  it('初始渲染凭据 Modal 与标题', () => {
    render(<VNCModal {...props} />)
    expect(screen.getByText('VNC 远程桌面 - 10.0.0.1:5900')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('(可选) VNC 密码')).toBeInTheDocument()
  })

  it('点连接：取票据、构造 RFB、connect 事件置已连接', async () => {
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(createRemoteTicket).toHaveBeenCalled())
    expect(createRemoteTicket).toHaveBeenCalledWith({
      agentId: 'ag1', host: '10.0.0.1', port: 5900, protocol: 'vnc', password: '',
    })
    const rfb = FakeRFB.instances[0]
    expect(rfb.url).toBe('ws://localhost:3000/api/remote/vnc')
    expect(rfb.opts).toMatchObject({ wsProtocols: ['tk'] })
    expect(screen.getByText(/正在连接到 10\.0\.0\.1:5900/)).toBeInTheDocument()

    await act(async () => {
      rfb.emit('connect')
    })
    expect(screen.getByText('已连接')).toBeInTheDocument()
    expect(rfb.scaleViewport).toBe(true)
    expect(rfb.resizeSession).toBe(false)
  })

  it('desktopname 事件显示桌面名；CAD 按钮调 sendCtrlAltDel', async () => {
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    const rfb = FakeRFB.instances[0]
    await act(async () => {
      rfb.emit('connect')
      rfb.emit('desktopname', { name: 'Win11' })
    })
    expect(screen.getByText('Win11')).toBeInTheDocument()
    fireEvent.click(screen.getByText('CAD'))
    expect(rfb.sendCtrlAltDel).toHaveBeenCalledTimes(1)
  })

  it('非正常断开提示异常；正常断开显示已断开', async () => {
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    const rfb = FakeRFB.instances[0]
    await act(async () => {
      rfb.emit('connect')
    })
    await act(async () => {
      rfb.emit('disconnect', { clean: false })
    })
    expect(msgError).toHaveBeenCalledWith('VNC 连接异常断开')
    expect(screen.getByText('连接已断开')).toBeInTheDocument()
  })

  it('票据失败提示连接失败', async () => {
    createRemoteTicket.mockRejectedValue(new Error('no ticket'))
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(msgError).toHaveBeenCalled())
    expect(msgError.mock.calls[0][0]).toMatch(/VNC 连接失败/)

// RemoteToolbar 断开按钮为 icon-only（无可访问名）——取工具栏最后一个按钮
  })

  it('连接挂起 30s 超时：提示超时并清理回到已断开（fake timers 推进）', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      render(<VNCModal {...props} />)
      fireEvent.click(screen.getByText('连 接').closest('button')!)
      await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
      const rfb = FakeRFB.instances[0]
      // 不 emit connect：停在 connecting，等 useConnectionTimeout 计时到点
      await act(async () => {
        await vi.advanceTimersByTimeAsync(30000)
      })
      expect(msgError).toHaveBeenCalledWith('连接超时，请检查网络或重试')
      expect(rfb.disconnect).toHaveBeenCalled()
      // showCredentials 从未置 false：超时后回凭据表单分支而非桌面 UI
      expect(screen.getByPlaceholderText('(可选) VNC 密码')).toBeInTheDocument()
      expect(screen.queryByText(/正在连接到/)).not.toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })

  it('断开按钮清理 RFB 回到凭据表单', async () => {
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    const rfb = FakeRFB.instances[0]
    await act(async () => {
      rfb.emit('connect')
    })
    fireEvent.click(lastToolbarButton())
    expect(rfb.disconnect).toHaveBeenCalled()
    expect(screen.getByPlaceholderText('(可选) VNC 密码')).toBeInTheDocument()
  })

  it('关闭 Modal 回调 onClose', async () => {
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByRole('button', { name: /close/i }))
    await waitFor(() => expect(props.onClose).toHaveBeenCalledTimes(1))
  })

  it('credentialsrequired：表单有密码则回填，否则 prompt 取密码', async () => {
    const promptSpy = vi.spyOn(window, 'prompt').mockReturnValue('prompted')
    render(<VNCModal {...props} />)
    fireEvent.change(screen.getByPlaceholderText('(可选) VNC 密码'), { target: { value: '' } })
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    const rfb = FakeRFB.instances[0]
    await act(async () => {
      rfb.emit('credentialsrequired')
    })
    expect(promptSpy).toHaveBeenCalled()
    expect(rfb.sendCredentials).toHaveBeenCalledWith({ password: 'prompted' })
    promptSpy.mockRestore()
  })

  it('credentialsrequired：prompt 取消则不发凭据', async () => {
    const promptSpy = vi.spyOn(window, 'prompt').mockReturnValue(null)
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    const rfb = FakeRFB.instances[0]
    await act(async () => {
      rfb.emit('credentialsrequired')
    })
    expect(rfb.sendCredentials).not.toHaveBeenCalled()
    promptSpy.mockRestore()
  })

  it('RFB 构造失败：提示连接失败并回 disconnected', async () => {
    FakeRFB.throwError = true
    try {
      render(<VNCModal {...props} />)
      fireEvent.click(screen.getByText('连 接').closest('button')!)
      await waitFor(() => expect(msgError).toHaveBeenCalled())
      expect(msgError.mock.calls.some((c) => /VNC 连接失败/.test(String(c[0])))).toBe(true)
    } finally {
      FakeRFB.throwError = false
    }
  })

  it('fullscreenchange 同步 isFullscreen（开/关两分支）', async () => {
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    await act(async () => {
      FakeRFB.instances[0].emit('connect')
    })

    await act(async () => {
      Object.defineProperty(document, 'fullscreenElement', { value: document.body, configurable: true })
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    // 全屏态：Modal 宽度 100vw（isFullscreen 传导）
    expect(document.querySelector('.ant-modal')!.getAttribute('style')).toContain('100vw')

    await act(async () => {
      Object.defineProperty(document, 'fullscreenElement', { value: null, configurable: true })
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    expect(document.querySelector('.ant-modal')!.getAttribute('style') ?? '').not.toContain('100vw')
  })

  it('disconnect clean:true 不弹异常提示（只显示已断开）', async () => {
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    await act(async () => {
      FakeRFB.instances[0].emit('connect')
    })
    await act(async () => {
      FakeRFB.instances[0].emit('disconnect', { clean: true })
    })
    expect(msgError).not.toHaveBeenCalledWith('VNC 连接异常断开')
    expect(screen.getByText('连接已断开')).toBeInTheDocument()
  })

  it('关闭清理：移除容器注入的子节点（cleanup while 循环）', () => {
    render(<VNCModal {...props} />)
    // 凭据阶段预挂载的 RFB 宿主（隐藏 div），模拟 noVNC 注入 canvas
    const host = document.querySelector('.ant-modal div[style*="display: none"]') as HTMLElement
    expect(host).not.toBeNull()
    host.appendChild(document.createElement('canvas'))
    fireEvent.click(screen.getByRole('button', { name: /close/i }))
    expect(host.firstChild).toBeNull()
    expect(props.onClose).toHaveBeenCalled()
  })

  it('全屏按钮切换：requestFullscreen 与退出分支', async () => {
    const requestFullscreen = vi.fn()
    const exitFullscreen = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(document, 'exitFullscreen', { value: exitFullscreen, configurable: true })
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    await act(async () => {
      FakeRFB.instances[0].emit('connect')
    })

    const container = document.querySelector('.ant-modal-body > div') as HTMLElement
    Object.defineProperty(container, 'requestFullscreen', { value: requestFullscreen, configurable: true })

    // 进入全屏分支
    fireEvent.click(document.querySelector('.ant-modal button .anticon-fullscreen')!.closest('button')!)
    expect(requestFullscreen).toHaveBeenCalled()

    // fullscreenchange 同步 → 退出全屏分支
    await act(async () => {
      Object.defineProperty(document, 'fullscreenElement', { value: document.body, configurable: true })
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    fireEvent.click(
      document.querySelector('.ant-modal button .anticon-fullscreen-exit')!.closest('button')!,
    )
    await waitFor(() => expect(exitFullscreen).toHaveBeenCalled())
    Object.defineProperty(document, 'fullscreenElement', { value: null, configurable: true })
    document.dispatchEvent(new Event('fullscreenchange'))
  })

  it('断开按钮在全屏态：退出全屏并回凭据表单', async () => {
    const exitFullscreen = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(document, 'exitFullscreen', { value: exitFullscreen, configurable: true })
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    await act(async () => {
      FakeRFB.instances[0].emit('connect')
    })
    await act(async () => {
      Object.defineProperty(document, 'fullscreenElement', { value: document.body, configurable: true })
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    fireEvent.click(lastToolbarButton())
    await waitFor(() => expect(exitFullscreen).toHaveBeenCalled())
    expect(screen.getByPlaceholderText('(可选) VNC 密码')).toBeInTheDocument()
    Object.defineProperty(document, 'fullscreenElement', { value: null, configurable: true })
    document.dispatchEvent(new Event('fullscreenchange'))
  })

  it('desktopname 无 name 清空桌面名', async () => {
    render(<VNCModal {...props} />)
    fireEvent.click(screen.getByText('连 接').closest('button')!)
    await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
    await act(async () => {
      FakeRFB.instances[0].emit('connect')
      FakeRFB.instances[0].emit('desktopname', { name: 'Win11' })
    })
    expect(screen.getByText('Win11')).toBeInTheDocument()
    await act(async () => {
      FakeRFB.instances[0].emit('desktopname', {})
    })
    expect(screen.queryByText('Win11')).not.toBeInTheDocument()
  })

  it('https 页面下 RFB 隧道用 wss', async () => {
    vi.stubGlobal('location', { protocol: 'https:', host: 'localhost:3000' })
    try {
      render(<VNCModal {...props} />)
      fireEvent.click(screen.getByText('连 接').closest('button')!)
      await waitFor(() => expect(FakeRFB.instances.length).toBe(1))
      expect(FakeRFB.instances[0].url).toBe('wss://localhost:3000/api/remote/vnc')
    } finally {
      vi.unstubAllGlobals()
    }
  })
})

function lastToolbarButton() {
  const buttons = document.querySelectorAll('.ant-modal-root button')
  return buttons[buttons.length - 1] as HTMLElement
}
