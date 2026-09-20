import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import RemoteToolbar from './index'

// RemoteToolbar：远程会话工具栏——状态徽标/分辨率/粘贴/全屏/额外动作/断开

const base = {
  onToggleFullscreen: vi.fn(),
  onDisconnect: vi.fn(),
}

describe('RemoteToolbar', () => {
  it('disconnected：显示已断开、断开按钮禁用、无操作按钮', () => {
    const { container } = render(<RemoteToolbar {...base} state="disconnected" isFullscreen={false} />)
    expect(screen.getByText('已断开')).toBeInTheDocument()
    expect(screen.queryByText('分辨率')).toBeNull()
    const buttons = container.querySelectorAll('button')
    // 仅 extraActions 无、全部操作按钮都不渲染；断开按钮禁用
    expect(buttons.length).toBe(1)
    expect(buttons[0].disabled).toBe(true)
  })

  it('connected：显示分辨率文本 + 分辨率按钮（需 onResolutionChange）', () => {
    render(
      <RemoteToolbar
        {...base}
        state="connected"
        isFullscreen={false}
        resolution="1280x800"
        onResolutionChange={vi.fn()}
      />,
    )
    expect(screen.getByText('已连接')).toBeInTheDocument()
    expect(screen.getByText('1280x800')).toBeInTheDocument()
    expect(screen.getByText('分辨率')).toBeInTheDocument()
    expect(screen.queryByText('已断开')).toBeNull()
  })

  it('分辨率下拉选择回调宽高', async () => {
    const onResolutionChange = vi.fn()
    render(
      <RemoteToolbar
        {...base}
        state="connected"
        isFullscreen={false}
        onResolutionChange={onResolutionChange}
      />,
    )
    fireEvent.click(screen.getByText('分辨率'))
    const item = (await screen.findAllByText('1920 x 1080'))[0]
    fireEvent.click(item)
    expect(onResolutionChange).toHaveBeenCalledWith(1920, 1080)
  })

  it('showResolution=false 不渲染分辨率文本与下拉', () => {
    render(
      <RemoteToolbar {...base} state="connected" isFullscreen={false} resolution="1x1" showResolution={false} />,
    )
    expect(screen.queryByText('1x1')).toBeNull()
    expect(screen.queryByText('分辨率')).toBeNull()
  })

  it('onClipboardPaste 有则渲染粘贴按钮并回调；缺省不渲染', () => {
    const onClipboardPaste = vi.fn()
    const { container, rerender } = render(
      <RemoteToolbar {...base} state="connected" isFullscreen={false} onClipboardPaste={onClipboardPaste} />,
    )
    // 无 onResolutionChange 时按钮序：粘贴 / 全屏 / 断开
    const withPaste = container.querySelectorAll('button')
    expect(withPaste.length).toBe(3)
    fireEvent.click(withPaste[0])
    expect(onClipboardPaste).toHaveBeenCalledTimes(1)

    rerender(<RemoteToolbar {...base} state="connected" isFullscreen={false} />)
    expect(container.querySelectorAll('button').length).toBe(2) // 全屏 / 断开
  })

  it('全屏按钮回调 onToggleFullscreen（connected 才有）', () => {
    render(<RemoteToolbar {...base} state="connected" isFullscreen={false} />)
    // 工具栏按钮：分辨率 / 粘贴(无) / 全屏 / 断开
    const buttons = document.querySelectorAll('button')
    fireEvent.click(buttons[buttons.length - 2])
    expect(base.onToggleFullscreen).toHaveBeenCalledTimes(1)
  })

  it('extraActions：有 icon 只渲染 icon；无 icon 渲染 label；disabled 生效', () => {
    render(
      <RemoteToolbar
        {...base}
        state="disconnected"
        isFullscreen={false}
        extraActions={[
          { key: 'a', label: '带图标', icon: <span data-testid="ic" />, onClick: vi.fn() },
          { key: 'b', label: '纯文本', onClick: vi.fn(), disabled: true },
        ]}
      />,
    )
    expect(screen.getByTestId('ic')).toBeInTheDocument()
    expect(screen.queryByText('带图标')).toBeNull()
    expect(screen.getByText('纯文本')).toBeInTheDocument()
    expect(screen.getByText('纯文本').closest('button')!.disabled).toBe(true)
  })

  it('children 渲染在左侧', () => {
    render(
      <RemoteToolbar {...base} state="connected" isFullscreen={false}>
        <span data-testid="kid">附加区</span>
      </RemoteToolbar>,
    )
    expect(screen.getByTestId('kid')).toBeInTheDocument()
  })
})
