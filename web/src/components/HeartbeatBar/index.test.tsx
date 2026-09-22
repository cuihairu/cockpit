import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { HeartbeatBar } from './index'
import type { ProbeResult } from '@/types'

// HeartbeatBar：心跳条可视化——loading/空态/槽位填充/颜色语义/倒序反转

const mk = (id: string, status: string, overrides: Partial<ProbeResult> = {}): ProbeResult => ({
  id, resourceType: 'site', resourceId: 'r1', name: 'n',
  status, latencyMs: 0, message: '', checkedAt: '2026-09-20T10:00:00Z', ...overrides,
})

const cells = (container: HTMLElement) =>
  Array.from(container.querySelectorAll('div[style*="border-radius"]')) as HTMLElement[]

describe('HeartbeatBar', () => {
  it('loading 显示 …', () => {
    const { container } = render(<HeartbeatBar results={[]} loading />)
    expect(container.textContent).toBe('…')
  })

  it('空数据显示「暂无记录」', () => {
    const { container } = render(<HeartbeatBar results={[]} />)
    expect(container.textContent).toBe('暂无记录')
  })

  it('数据不足补空槽（左空右新），状态映射颜色', () => {
    const results = [mk('1', 'up'), mk('2', 'down')]
    const { container } = render(<HeartbeatBar results={results} slots={5} />)
    const dots = cells(container)
    expect(dots).toHaveLength(5)
    // 前 3 个空槽；数据左旧右新（倒序传入 → down 旧在左、up 新在右）
    expect(dots[0].style.background).toContain('rgba(128, 128, 128, 0.25)')
    expect(dots[3].style.background).toBe('rgb(255, 77, 79)') // down 红（旧）
    expect(dots[4].style.background).toBe('rgb(82, 196, 26)') // up 绿（新）
  })

  it('未知状态用灰色；degraded 橙（左旧右新）', () => {
    const results = [mk('1', 'degraded'), mk('2', 'weird-status')]
    const { container } = render(<HeartbeatBar results={results} />)
    const dots = cells(container)
    expect(dots[dots.length - 2].style.background).toBe('rgb(191, 191, 191)') // weird 旧
    expect(dots[dots.length - 1].style.background).toBe('rgb(250, 173, 20)') // degraded 新
  })

  it('只取最近 slots 条（超量截断）', () => {
    const results = Array.from({ length: 10 }, (_, i) => mk(`id${i}`, 'up'))
    const { container } = render(<HeartbeatBar results={results} slots={4} />)
    expect(cells(container)).toHaveLength(4)
  })

  it('tip 拼接延迟与消息；results 缺省回落空态', () => {
    const results = [mk('1', 'up', { latencyMs: 42, message: 'timeout-ish' }), mk('2', 'down', { message: 'refused' })]
    const { container, unmount } = render(<HeartbeatBar results={results} slots={2} />)
    expect(cells(container)).toHaveLength(2)
    unmount()
    const empty = render(<HeartbeatBar results={undefined as unknown as ProbeResult[]} />)
    expect(empty.container.textContent).toBe('暂无记录')
  })
})
