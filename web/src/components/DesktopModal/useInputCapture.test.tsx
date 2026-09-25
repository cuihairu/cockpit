import { fireEvent, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useInputCapture } from './useInputCapture'
import {
  codeToScanCode,
  getBaseScanCode,
  isExtendedKey,
} from '@/utils/scancodes'

// useInputCapture：RDP 输入捕获——鼠标坐标换算/滚轮方向/键盘扫描码

const setup = (enabled: boolean) => {
  const sendKeyboard = vi.fn()
  const sendMouse = vi.fn()
  let canvasEl!: HTMLCanvasElement
  let cap!: ReturnType<typeof useInputCapture>
  // setCanvas 须在 useEffect 挂监听前执行——用 ref callback（commit 期先于 effect）
  const Probe = () => {
    cap = useInputCapture({ sendKeyboard, sendMouse, enabled })
    return (
      <canvas
        ref={(c) => {
          if (c) {
            canvasEl = c
            c.width = 200
            c.height = 100
            // 换算基准：显示 100x50 → 比例 2
            c.getBoundingClientRect = () =>
              ({ left: 0, top: 0, width: 100, height: 50, right: 100, bottom: 50, x: 0, y: 0, toJSON: () => '' }) as DOMRect
          }
          cap.setCanvas(c)
        }}
      />
    )
  }
  render(<Probe />)
  return { canvas: canvasEl, sendKeyboard, sendMouse, setCanvas: cap.setCanvas }
}

describe('useInputCapture', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('mousedown/up 换算坐标并按 action 发送；contextmenu 阻止默认', () => {
    const { canvas, sendMouse } = setup(true)
    fireEvent.mouseDown(canvas, { button: 0, buttons: 1, clientX: 10, clientY: 20 })
    expect(sendMouse).toHaveBeenCalledWith(20, 40, 0, 0, 'down')
    fireEvent.mouseUp(canvas, { button: 2, clientX: 10, clientY: 20 })
    expect(sendMouse).toHaveBeenLastCalledWith(20, 40, 2, 0, 'up')
    fireEvent.mouseMove(canvas, { clientX: 5, clientY: 5 })
    expect(sendMouse).toHaveBeenLastCalledWith(10, 10, 0, 0, 'move')
    expect(sendMouse).toHaveBeenCalledTimes(3)
  })

  it('wheel deltaY 正负映射 ±1；零不发送', () => {
    const { canvas, sendMouse } = setup(true)
    fireEvent.wheel(canvas, { deltaY: 100 })
    expect(sendMouse).toHaveBeenCalledWith(0, 0, 0, -1, 'move')
    fireEvent.wheel(canvas, { deltaY: -100 })
    expect(sendMouse).toHaveBeenLastCalledWith(0, 0, 0, 1, 'move')
    fireEvent.wheel(canvas, { deltaY: 0 })
    expect(sendMouse).toHaveBeenCalledTimes(2)
  })

  it('keydown/keyup 映射扫描码（extended/base 由 utils 决定）', () => {
    const { sendKeyboard } = setup(true)
    fireEvent.keyDown(window, { code: 'KeyA' })
    const scan = codeToScanCode('KeyA')
    expect(sendKeyboard).toHaveBeenCalledWith(getBaseScanCode(scan), true, isExtendedKey(scan))
    fireEvent.keyUp(window, { code: 'KeyA' })
    expect(sendKeyboard).toHaveBeenLastCalledWith(getBaseScanCode(scan), false, isExtendedKey(scan))
  })

  it('未知 code 扫描码为 0 不发送（keydown 与 keyup 同）', () => {
    const { sendKeyboard } = setup(true)
    fireEvent.keyDown(window, { code: 'NotAKey' })
    expect(sendKeyboard).not.toHaveBeenCalled()
    fireEvent.keyUp(window, { code: 'NotAKey' })
    expect(sendKeyboard).not.toHaveBeenCalled()
  })

  it('canvas 置空后 mousemove 坐标兜底 0,0', () => {
    const { canvas, sendMouse, setCanvas } = setup(true)
    setCanvas(null)
    fireEvent.mouseMove(canvas, { clientX: 5, clientY: 5 })
    expect(sendMouse).toHaveBeenLastCalledWith(0, 0, 0, 0, 'move')
  })

  it('enabled=false 时全部静默', () => {
    const { canvas, sendKeyboard, sendMouse } = setup(false)
    fireEvent.mouseDown(canvas, { button: 0 })
    fireEvent.mouseUp(canvas, { button: 0 })
    fireEvent.mouseMove(canvas, { clientX: 1, clientY: 1 })
    fireEvent.wheel(canvas, { deltaY: 10 })
    fireEvent.keyDown(window, { code: 'KeyA' })
    fireEvent.keyUp(window, { code: 'KeyA' })
    expect(sendMouse).not.toHaveBeenCalled()
    expect(sendKeyboard).not.toHaveBeenCalled()
  })

  it('拦截键（F5）走 preventDefault 且扫描码照常发送', () => {
    const { sendKeyboard } = setup(true)
    const evt = new KeyboardEvent('keydown', { code: 'F5', bubbles: true, cancelable: true })
    const spy = vi.spyOn(evt, 'preventDefault')
    window.dispatchEvent(evt)
    expect(spy).toHaveBeenCalled()
    const scan = codeToScanCode('F5')
    expect(sendKeyboard).toHaveBeenCalledWith(getBaseScanCode(scan), true, isExtendedKey(scan))
  })
})
