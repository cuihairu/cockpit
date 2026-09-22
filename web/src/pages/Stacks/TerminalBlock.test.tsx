import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import TerminalBlock from './TerminalBlock'

// TerminalBlock：加载中 / 内容 / 空态默认与自定义文案、默认与自定义 props

describe('TerminalBlock', () => {
  it('默认 props：渲染内容', () => {
    render(<TerminalBlock content="line1\nline2" />)
    expect(screen.getByText(/line1/)).toBeInTheDocument()
  })

  it('loading 显示加载中文案', () => {
    render(<TerminalBlock content="x" loading />)
    expect(screen.getByText('加载中...')).toBeInTheDocument()
  })

  it('空内容：默认空文案 / 自定义空文案', () => {
    const { unmount } = render(<TerminalBlock content="" />)
    expect(screen.getByText('（无日志输出）')).toBeInTheDocument()
    unmount()
    render(<TerminalBlock content="" emptyText="自定义空态" />)
    expect(screen.getByText('自定义空态')).toBeInTheDocument()
  })

  it('自定义 maxHeight 生效', () => {
    const { container } = render(<TerminalBlock content="x" maxHeight={100} />)
    expect((container.firstElementChild as HTMLElement).style.maxHeight).toBe('100px')
  })
})
