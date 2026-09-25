import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GuacPlayer from './index'

// GuacPlayer：.guac 回放——Guacamole.SessionRecording 承担解析与回放，
// 本组件只管挂载 Display + 播放控制 + 生命周期（todo.md M3 D5）

const recMock = vi.hoisted(() => ({
  play: vi.fn(),
  pause: vi.fn(),
  seek: vi.fn(),
  getDisplay: vi.fn(),
  getDuration: vi.fn(() => 5000),
  getPosition: vi.fn(() => 0),
  isPlaying: vi.fn(() => false),
  onplay: undefined as unknown,
  onpause: undefined as unknown,
  onseek: undefined as unknown,
  onerror: undefined as unknown,
}))

vi.mock('guacamole-common-js', () => ({
  default: {
    SessionRecording: vi.fn(function () {
      return recMock
    }),
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

  it('挂载 Display 并出播放控制', async () => {
    render(<GuacPlayer blob={blob} />)
    await waitFor(() => expect(recMock.getDisplay).toHaveBeenCalled())
    expect(screen.getByTestId('guac-display')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /播\s*放/ })).toBeInTheDocument()
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
})
