import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import BackupCodesDisplay from './index'

// BackupCodesDisplay：恢复码展示——默认模糊/显隐切换/单码复制状态/全部复制

const writeText = vi.fn().mockResolvedValue(undefined)
Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

describe('BackupCodesDisplay', () => {
  beforeEach(() => {
    writeText.mockClear()
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('默认模糊，列表带 blurred 类；点「显示代码」解除', () => {
    const { container } = render(<BackupCodesDisplay codes={['a1', 'b2']} />)
    expect(container.querySelector('.backup-codes-list')?.className).toContain('blurred')
    fireEvent.click(screen.getByText('显示代码'))
    expect(container.querySelector('.backup-codes-list')?.className).not.toContain('blurred')
    fireEvent.click(screen.getByText('隐藏代码'))
    expect(container.querySelector('.backup-codes-list')?.className).toContain('blurred')
  })

  it('渲染序号与码值；自定义 title/warning', () => {
    render(<BackupCodesDisplay codes={['a1', 'b2']} title="T" warning="W" />)
    expect(screen.getByText('T')).toBeInTheDocument()
    expect(screen.getByText('W')).toBeInTheDocument()
    expect(screen.getByText('1.')).toBeInTheDocument()
    expect(screen.getByText('2.')).toBeInTheDocument()
    expect(screen.getByText('a1', { selector: 'code' })).toBeInTheDocument()
  })

  it('点单码图标复制对应码并标记 copied，2s 后重置', async () => {
    const { container } = render(<BackupCodesDisplay codes={['a1', 'b2']} />)
    const item = container.querySelectorAll('.backup-code-item')[0]
    const btn = item.querySelector('button')!
    await act(async () => {
      fireEvent.click(btn)
    })
    expect(writeText).toHaveBeenCalledWith('a1')
    expect(btn.className).toContain('copied')
    act(() => {
      vi.advanceTimersByTime(2100)
    })
    expect(btn.className).not.toContain('copied')
  })

  it('全部复制以换行拼接', () => {
    render(<BackupCodesDisplay codes={['a1', 'b2', 'c3']} />)
    fireEvent.click(screen.getByText('全部复制'))
    expect(writeText).toHaveBeenCalledWith('a1\nb2\nc3')
  })
})
