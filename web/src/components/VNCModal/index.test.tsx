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
})

function lastToolbarButton() {
  const buttons = document.querySelectorAll('.ant-modal-root button')
  return buttons[buttons.length - 1] as HTMLElement
}
