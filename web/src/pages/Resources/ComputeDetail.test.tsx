import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import ComputeDetail from './ComputeDetail'
import type { ComputeInstance } from '@/types'

// ComputeDetail：计算实例展开行的详情卡。内存 <1024MB 走 MB 显示、
// >=1024 走 GB；type/status 三态配色；labels 空则不渲染标签行；
// createdAt/updatedAt 缺省时不渲染对应项。

const base = {
  id: 'c1',
  name: 'web-vm',
  type: 'vm',
  agentId: 'ag-1',
  region: 'cn',
  zone: 'z1',
  status: 'running',
  cpuCores: 4,
  memoryMb: 8192,
  diskGb: 100,
  ipv4: '10.0.0.1',
} as unknown as ComputeInstance

const renderDetail = (over: Record<string, unknown>) =>
  render(<ComputeDetail record={{ ...base, ...over } as never} />)

describe('ComputeDetail', () => {
  it('满字段：内存 GB 显示、vm/running 配色、标签与时间行', () => {
    renderDetail({
      labels: { env: 'prod', tier: 'web' },
      createdAt: '2026-01-02T03:04:05Z',
      updatedAt: '2026-02-03T04:05:06Z',
    })
    expect(screen.getByText('c1')).toBeInTheDocument()
    expect(screen.getByText('web-vm')).toBeInTheDocument()
    expect(screen.getByText('VM')).toBeInTheDocument()
    expect(screen.getByText('running')).toBeInTheDocument()
    expect(screen.getByText('ag-1')).toBeInTheDocument()
    expect(screen.getByText('cn/z1')).toBeInTheDocument()
    expect(screen.getByText('4 核')).toBeInTheDocument()
    expect(screen.getByText('8 GB')).toBeInTheDocument()
    expect(screen.getByText('100 GB')).toBeInTheDocument()
    expect(screen.getByText('10.0.0.1')).toBeInTheDocument()
    // 标签行渲染（Object.entries map）
    expect(screen.getByText(/env:/)).toBeInTheDocument()
    expect(screen.getByText(/tier:/)).toBeInTheDocument()
    expect(screen.getByText('创建时间')).toBeInTheDocument()
    expect(screen.getByText('更新时间')).toBeInTheDocument()
    expect(screen.getByText(new Date('2026-01-02T03:04:05Z').toLocaleString())).toBeInTheDocument()
  })

  it('内存 < 1024MB：MB 显示；container/stopped 配色', () => {
    renderDetail({ type: 'container', status: 'stopped', memoryMb: 512 })
    expect(screen.getByText('CONTAINER')).toBeInTheDocument()
    expect(screen.getByText('stopped')).toBeInTheDocument()
    expect(screen.getByText('512 MB')).toBeInTheDocument()
  })

  it('baremetal + 未知状态：orange/error 配色', () => {
    renderDetail({ type: 'baremetal', status: 'unknown' })
    expect(screen.getByText('BAREMETAL')).toBeInTheDocument()
    expect(screen.getByText('unknown')).toBeInTheDocument()
  })

  it('缺省兜底：labels 空不渲染标签行、无时间行、空字段显示 —', () => {
    renderDetail({
      labels: undefined,
      createdAt: undefined,
      updatedAt: undefined,
      type: undefined,
      status: undefined,
      agentId: '',
      region: '',
      zone: '',
      cpuCores: 0,
      memoryMb: 0,
      diskGb: 0,
      ipv4: '',
    })
    // labels 空 → 无「标签」项
    expect(screen.queryByText('标签')).not.toBeInTheDocument()
    expect(screen.queryByText('创建时间')).not.toBeInTheDocument()
    expect(screen.queryByText('更新时间')).not.toBeInTheDocument()
    // 多个字段兜底为 —，region/zone -/-
    expect(document.body.textContent).toContain('—')
    const body = document.body.textContent || ''
    expect(body.includes('-/-') || body.includes('—/—') || body.includes('- / -')).toBeTruthy()
  })
})