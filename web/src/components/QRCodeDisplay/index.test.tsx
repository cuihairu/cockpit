import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import QRCodeDisplay from './index'

// QRCodeDisplay：canvas 二维码渲染——成功隐藏 loading / 失败显示错误

const toCanvas = vi.hoisted(() => vi.fn())
vi.mock('qrcode', () => ({ default: { toCanvas } }))

describe('QRCodeDisplay', () => {
  beforeEach(() => {
    toCanvas.mockReset()
  })

  it('渲染标题并调 toCanvas，成功后 loading 消失', async () => {
    toCanvas.mockImplementation((_c: unknown, _v: string, _o: unknown, cb: (e: Error | null) => void) => {
      cb(null)
    })
    const { container } = render(<QRCodeDisplay value="otpauth://x" title="扫码" />)
    expect(screen.getByText('扫码')).toBeInTheDocument()
    expect(toCanvas).toHaveBeenCalledWith(
      expect.anything(),
      'otpauth://x',
      { width: 200 },
      expect.any(Function),
    )
    await waitFor(() => {
      expect(container.querySelector('.qrcode-loading')).toBeNull()
    })
    expect(container.querySelector('.qrcode-error')).toBeNull()
  })

  it('生成失败显示错误文案', async () => {
    toCanvas.mockImplementation((_c: unknown, _v: string, _o: unknown, cb: (e: Error | null) => void) => {
      cb(new Error('bad value'))
    })
    const { container } = render(<QRCodeDisplay value="x" />)
    await waitFor(() => {
      expect(container.querySelector('.qrcode-error')?.textContent).toBe('生成 QR 码失败')
    })
  })

  it('空 value 不触发渲染', () => {
    render(<QRCodeDisplay value="" />)
    expect(toCanvas).not.toHaveBeenCalled()
  })
})
