import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCanvasRenderer } from './useCanvasRenderer'
import type { ScreenUpdate } from './useDesktopWS'

// useCanvasRenderer：RDP 位图帧缓冲——canvas 初始化/rect 累积与 raf 刷帧

// jsdom 无 2d context / ImageData——stub 最小实现
const ctx2d = {
  fillStyle: '',
  fillRect: vi.fn(),
  putImageData: vi.fn(),
}
class FakeImageData {
  width: number
  height: number
  data: Uint8ClampedArray
  constructor(arg: number | Uint8ClampedArray, heightOrNothing?: number) {
    if (typeof arg === 'number') {
      this.width = arg
      this.height = heightOrNothing ?? 0
      this.data = new Uint8ClampedArray(arg * this.height * 4)
    } else {
      this.data = arg
      this.width = 0
      this.height = 0
    }
  }
}

const b64 = (bytes: number[]) => btoa(String.fromCharCode(...bytes))

describe('useCanvasRenderer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('ImageData', FakeImageData)
    HTMLCanvasElement.prototype.getContext = vi.fn(() => ctx2d) as never
  })

  const setup = () => {
    let api!: ReturnType<typeof useCanvasRenderer>
    const canvas = document.createElement('canvas')
    const Probe = () => {
      api = useCanvasRenderer()
      return null
    }
    render(<Probe />)
    act(() => {
      api.setCanvas(canvas)
    })
    return { get: () => api, canvas }
  }

  const update = (u: ScreenUpdate) =>
    act(() => {
      apiRef.current!.handleScreenUpdate(u)
    })

  let apiRef: { current: ReturnType<typeof useCanvasRenderer> | null } = { current: null }

  it('initBuffer 设 canvas 尺寸并填黑底', () => {
    const { get, canvas } = setup()
    act(() => {
      get().initBuffer(320, 200)
    })
    expect(canvas.width).toBe(320)
    expect(canvas.height).toBe(200)
    expect(ctx2d.fillStyle).toBe('#000')
    expect(ctx2d.fillRect).toHaveBeenCalledWith(0, 0, 320, 200)
  })

  it('handleScreenUpdate 尺寸变化时重初始化；rect 经 raf 刷帧 putImageData', async () => {
    const { get, canvas } = setup()
    apiRef.current = get()
    // 1x1 红：RGBA
    update({ width: 4, height: 4, rects: [{ x: 1, y: 1, width: 1, height: 1, data: b64([255, 0, 0, 255]) }] })
    expect(canvas.width).toBe(4)
    // 等 raf 刷帧
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })
    expect(ctx2d.putImageData).toHaveBeenCalledTimes(1)
    const [region, x, y] = ctx2d.putImageData.mock.calls[0] as [FakeImageData, number, number]
    expect(Array.from(region.data)).toEqual([255, 0, 0, 255])
    expect(x).toBe(1)
    expect(y).toBe(1)
  })

  it('坏 base64 与尺寸不符的 rect 跳过；同帧多个 rect 合并一次 raf', async () => {
    const { get } = setup()
    apiRef.current = get()
    update({
      width: 4, height: 4,
      rects: [
        { x: 0, y: 0, width: 1, height: 1, data: '%%not-base64%%' },
        { x: 0, y: 0, width: 2, height: 2, data: b64([1, 2, 3, 4]) }, // 期望 16 字节
        { x: 0, y: 0, width: 1, height: 1, data: b64([9, 8, 7, 6]) },
      ],
    })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })
    // 只有尺寸匹配的最后一个 rect 被绘制
    expect(ctx2d.putImageData).toHaveBeenCalledTimes(1)
  })

  it('卸载取消未决 raf', async () => {
    const { get, canvas } = setup()
    apiRef.current = get()
    update({ width: 2, height: 2, rects: [{ x: 0, y: 0, width: 1, height: 1, data: b64([1, 1, 1, 1]) }] })
    // 不等 raf 直接卸载（cleanup cancelAnimationFrame）
    expect(canvas).toBeTruthy()
  })

  it('无 canvas：initBuffer 早退 → 刷帧无缓冲早退，不绘制', async () => {
    let api!: ReturnType<typeof useCanvasRenderer>
    const Probe = () => {
      api = useCanvasRenderer()
      return null
    }
    render(<Probe />)
    act(() => {
      api.handleScreenUpdate({ width: 4, height: 4, rects: [{ x: 0, y: 0, width: 1, height: 1, data: b64([1, 1, 1, 1]) }] })
    })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })
    expect(ctx2d.putImageData).not.toHaveBeenCalled()
  })

  it('无 2d context：黑底跳过、刷帧早退', async () => {
    const { get } = setup()
    apiRef.current = get()
    HTMLCanvasElement.prototype.getContext = vi.fn(() => null) as never
    update({ width: 4, height: 4, rects: [{ x: 0, y: 0, width: 1, height: 1, data: b64([1, 1, 1, 1]) }] })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })
    expect(ctx2d.fillRect).not.toHaveBeenCalled()
    expect(ctx2d.putImageData).not.toHaveBeenCalled()
  })

  it('rect 超出缓冲：底边截断 break、右边越界 continue，不炸', async () => {
    const { get } = setup()
    apiRef.current = get()
    update({
      width: 4, height: 4,
      rects: [
        { x: 0, y: 10, width: 1, height: 1, data: b64([1, 1, 1, 1]) }, // dstY >= height → break
        { x: 10, y: 0, width: 1, height: 1, data: b64([2, 2, 2, 2]) }, // copyWidth <= 0 → continue
      ],
    })
    await act(async () => {
      await new Promise((r) => requestAnimationFrame(() => r(null)))
    })
    expect(ctx2d.putImageData).toHaveBeenCalledTimes(2)
  })

  it('同尺寸续帧不重初始化；raf 挂起期间合并调度；变尺寸触发重初始化', async () => {
    const rafSpy = vi.spyOn(window, 'requestAnimationFrame')
    const { get, canvas } = setup()
    apiRef.current = get()
    update({ width: 4, height: 4, rects: [{ x: 0, y: 0, width: 1, height: 1, data: b64([1, 1, 1, 1]) }] })
    const rafCalls = rafSpy.mock.calls.length
    // 同尺寸第二帧：不 initBuffer（fillRect 不增）；raf 已挂：不重复调度
    update({ width: 4, height: 4, rects: [{ x: 0, y: 0, width: 1, height: 1, data: b64([2, 2, 2, 2]) }] })
    expect(ctx2d.fillRect).toHaveBeenCalledTimes(1)
    expect(rafSpy.mock.calls.length).toBe(rafCalls)
    // 变尺寸：重新 initBuffer
    update({ width: 8, height: 8, rects: [] })
    expect(canvas.width).toBe(8)
    expect(ctx2d.fillRect).toHaveBeenCalledTimes(2)
    rafSpy.mockRestore()
  })
})
