import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GuacPlayer from './index'

// GuacPlayer：.guac 回放——Guacamole.SessionRecording 承担解析与回放，
// 本组件只管挂载 Display + 播放控制 + 生命周期（todo.md M3 D5）

const recMock = vi.hoisted(() => ({
  play: vi.fn(),
  pause: vi.fn(),
  seek: vi.fn(),
  connect: vi.fn(),
  disconnect: vi.fn(),
  getDisplay: vi.fn(),
  getDuration: vi.fn(() => 5000),
  getPosition: vi.fn(() => 0),
  isPlaying: vi.fn(() => false),
  onplay: undefined as unknown,
  onpause: undefined as unknown,
  onseek: undefined as unknown,
  onerror: undefined as unknown,
}))

// Tunnel 基类 mock：makeBlobTunnel 会覆写 connect/disconnect（注入指令流），
// receiveInstruction/setState 保留 mock 供断言
const tunnelMock = vi.hoisted(() => ({
  connect: vi.fn(),
  disconnect: vi.fn(),
  receiveInstruction: vi.fn(),
  setState: vi.fn(),
}))

vi.mock('guacamole-common-js', () => ({
  default: {
    SessionRecording: vi.fn(function () {
      return recMock
    }),
    // State.CLOSED 常量被 duck tunnel 的收尾/异常路径读取，必须挂在构造器上
    Tunnel: Object.assign(
      vi.fn(function () {
        return tunnelMock
      }),
      { State: { CLOSED: 4 } },
    ),
  },
}))

const blob = new Blob(['fake-guac'], { type: 'application/octet-stream' })

describe('GuacPlayer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    recMock.getDisplay.mockReturnValue({
      getElement: () => {
        const el = document.createElement('div')
        el.setAttribute('data-testid', 'guac-display')
        return el
      },
    })
    recMock.getDuration.mockReturnValue(5000)
    recMock.isPlaying.mockReturnValue(false)
  })

  it('挂载 Display 并出播放控制；经 duck tunnel 构造并立即 connect 注入指令流', async () => {
    render(<GuacPlayer blob={blob} />)
    await waitFor(() => expect(recMock.getDisplay).toHaveBeenCalled())
    expect(screen.getByTestId('guac-display')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /播\s*放/ })).toBeInTheDocument()
    // 1.5.0 的 SessionRecording(Blob) 分支官方即坏（恒 0 帧）：组件必须走
    // tunnel 分支（构造入参是 Tunnel 实例）且构造后立即 connect 触发注入
    const Guacamole = (await import('guacamole-common-js')).default
    const ctor = Guacamole.SessionRecording as unknown as ReturnType<typeof vi.fn>
    expect(ctor).toHaveBeenCalledWith(expect.objectContaining({ receiveInstruction: expect.any(Function) }))
    expect(recMock.connect).toHaveBeenCalled()
  })

  it('播放时读 duration；isPlaying 切换到暂停态', async () => {
    render(<GuacPlayer blob={blob} />)
    await waitFor(() => expect(recMock.getDisplay).toHaveBeenCalled())

    fireEvent.click(screen.getByRole('button', { name: /播\s*放/ }))
    expect(recMock.play).toHaveBeenCalled()
    expect(recMock.getDuration).toHaveBeenCalled()

    recMock.isPlaying.mockReturnValue(true)
    await act(async () => {
      ;(recMock.onplay as () => void)?.()
    })
    expect(screen.getByRole('button', { name: /暂\s*停/ })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /暂\s*停/ }))
    expect(recMock.pause).toHaveBeenCalled()
  })

  it('onseek 更新位置显示（先播放让 duration 就绪）', async () => {
    render(<GuacPlayer blob={blob} />)
    await waitFor(() => expect(recMock.getDisplay).toHaveBeenCalled())
    // duration 在 play 事件处理器里读（避免 set-state-in-effect）
    fireEvent.click(screen.getByRole('button', { name: /播\s*放/ }))

    await act(async () => {
      ;(recMock.onseek as (p: number) => void)?.(1234)
    })
    expect(screen.getByText(/1s \/ 5s/)).toBeInTheDocument()
  })

  it('onerror 抛错走 onError；卸载时 pause 并清理', async () => {
    const onError = vi.fn()
    const { unmount } = render(<GuacPlayer blob={blob} onError={onError} />)
    await waitFor(() => expect(recMock.getDisplay).toHaveBeenCalled())

    await act(async () => {
      ;(recMock.onerror as (s: { message: string }) => void)?.({ message: 'boom' })
    })
    expect(onError).toHaveBeenCalledWith('boom')

    recMock.pause.mockClear()
    unmount()
    expect(recMock.pause).toHaveBeenCalled()
  })

  it('SessionRecording 构造抛错走 onError（解析失败不炸界面）', async () => {
    const Guacamole = (await import('guacamole-common-js')).default
    ;(Guacamole.SessionRecording as unknown as ReturnType<typeof vi.fn>).mockImplementationOnce(
      // new 调用需 function 形式（箭头函数不是构造函数）
      function () {
        throw new Error('parse fail')
      },
    )
    const onError = vi.fn()
    render(<GuacPlayer blob={blob} onError={onError} />)
    await waitFor(() => expect(onError).toHaveBeenCalledWith('parse fail'))
  })

  it('构造抛非 Error：兜底「录制解析失败」；rec 未建时播放按钮早退', async () => {
    const Guacamole = (await import('guacamole-common-js')).default
    ;(Guacamole.SessionRecording as unknown as ReturnType<typeof vi.fn>).mockImplementationOnce(
      function () {
        throw 'plain-string'
      },
    )
    const onError = vi.fn()
    render(<GuacPlayer blob={blob} onError={onError} />)
    await waitFor(() => expect(onError).toHaveBeenCalledWith('录制解析失败'))
    // recRef 未建：toggle 的 !rec 防御早退
    fireEvent.click(screen.getByRole('button', { name: /播\s*放/ }))
    expect(recMock.play).not.toHaveBeenCalled()
  })

  it('onerror 无 message 字段：兜底「录制回放失败」', async () => {
    const onError = vi.fn()
    render(<GuacPlayer blob={blob} onError={onError} />)
    await waitFor(() => expect(recMock.getDisplay).toHaveBeenCalled())
    await act(async () => {
      ;(recMock.onerror as (s: unknown) => void)?.(undefined)
    })
    expect(onError).toHaveBeenCalledWith('录制回放失败')
  })

  it('Slider 拖动触发 rec.seek 并以回调更新位置', async () => {
    const { container } = render(<GuacPlayer blob={blob} />)
    await screen.findByTestId('guac-display')
    // 先播放让 duration 就绪（5s），再经 onseek 把位置推到 1s
    fireEvent.click(screen.getByRole('button', { name: /播放/ }))
    ;(recMock.onseek as (p: number) => void)?.(1234)
    await screen.findByText((_, el) => el?.tagName === 'SPAN' && el.textContent === '1s / 5s')
    // mousedown 轨道 → onChange(v) → seek(v, cb) → cb 回填位置（jsdom 计算值为 0）
    recMock.seek.mockImplementation((_v: number, cb: () => void) => cb())
    fireEvent.mouseDown(container.querySelector('.ant-slider-rail') as HTMLElement, { clientX: 60, clientY: 10 })
    await screen.findByText((_, el) => el?.tagName === 'SPAN' && el.textContent === '5s / 5s')
    expect(recMock.seek).toHaveBeenCalled()
  })

  // ==== duck tunnel 指令注入（makeBlobTunnel 的 connect 体）====
  // recMock.connect 是空桩、不会回落调 tunnel.connect，blob 指令流解析器
  // （parseGuacInstructions）与 CLOSED 收尾因此从未执行——渲染后手动驱动
  // tunnel.connect（已被 makeBlobTunnel 覆写为真实现）覆盖解析器全分支形态
  const driveTunnel = async (text: string) => {
    const { unmount } = render(<GuacPlayer blob={new Blob([text])} />)
    await waitFor(() => expect(recMock.getDisplay).toHaveBeenCalled())
    tunnelMock.receiveInstruction.mockClear()
    tunnelMock.setState.mockClear()
    await act(async () => {
      ;(tunnelMock.connect as () => void)()
      await new Promise((r) => setTimeout(r, 0))
    })
    unmount()
  }

  it('duck tunnel：有效指令流全解析（前导分号跳过/指令收集/EOF 与分号双收口）', async () => {
    // ';4.size;;1.x'：前导 ';' 跳过；两条指令间双分号走内层 ';' 退出侧
    await driveTunnel(';4.size;;1.x')
    expect(tunnelMock.receiveInstruction).toHaveBeenNthCalledWith(1, 'size', [])
    expect(tunnelMock.receiveInstruction).toHaveBeenNthCalledWith(2, 'x', [])
    expect(tunnelMock.setState).toHaveBeenCalledWith(4)

    // '4.size'：结尾无分号，内层 while 走 i >= len 退出（与 ';' 退出互补）
    await driveTunnel('4.size')
    expect(tunnelMock.receiveInstruction).toHaveBeenCalledWith('size', [])
    expect(tunnelMock.setState).toHaveBeenCalledWith(4)
  })

  it('duck tunnel：畸形流三形态（缺分隔点/长度非数字/长度越界）抛错后均收口 CLOSED', async () => {
    for (const bad of ['abc;', 'abc.size;', '99.size;']) {
      await driveTunnel(bad)
      expect(tunnelMock.receiveInstruction).not.toHaveBeenCalled()
      expect(tunnelMock.setState).toHaveBeenCalledWith(4)
    }
  })
})
