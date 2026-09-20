import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import SystemInfoCard from './index'
import type { SystemInfoSnapshot } from '@/services/metrics'

// SystemInfoCard：系统信息快照渲染——标题/统计卡/负载/运行时间/流量

const snap = (overrides: Partial<SystemInfoSnapshot> = {}): SystemInfoSnapshot => ({
  id: 1, agentId: 'ag1',
  cpuUsage: 12.5, cpuCores: 8, cpuFreqMhz: 2600,
  memTotal: 8 * 1024 ** 3, memUsed: 4 * 1024 ** 3, memAvailable: 4 * 1024 ** 3, memUsagePercent: 50,
  diskTotal: 100 * 1024 ** 3, diskUsed: 25 * 1024 ** 3, diskFree: 75 * 1024 ** 3, diskUsagePercent: 25,
  netBytesSent: 1024, netBytesRecv: 2048,
  osName: 'Ubuntu', osVersion: '24.04', arch: 'amd64',
  uptime: 3661, hostname: 'web-01',
  load1: 0.5, load5: 0.4, load15: 0.3, updatedAt: '2026-09-20T10:00:00Z',
  ...overrides,
})

describe('SystemInfoCard', () => {
  it('标题渲染主机名/OS/arch', () => {
    render(<SystemInfoCard systemInfo={snap()} />)
    expect(screen.getByText('web-01')).toBeInTheDocument()
    expect(screen.getByText('Ubuntu')).toBeInTheDocument()
    expect(screen.getByText('amd64')).toBeInTheDocument()
  })

  it('CPU/内存/磁盘统计与规格文本', () => {
    render(<SystemInfoCard systemInfo={snap()} />)
    expect(screen.getByText('CPU 使用率')).toBeInTheDocument()
    expect(screen.getByText('核心数: 8')).toBeInTheDocument()
    // formatBytes(4GiB/8GiB) toFixed(2)
    expect(screen.getByText('4.00 GB / 8.00 GB')).toBeInTheDocument()
    expect(screen.getByText('25.00 GB / 100.00 GB')).toBeInTheDocument()
    // uptime 3661s → 1小时 1分钟
    expect(screen.getAllByText('1小时 1分钟').length).toBeGreaterThan(0)
  })

  it('系统负载三元组与网络流量', () => {
    render(<SystemInfoCard systemInfo={snap()} />)
    expect(screen.getByText('1分钟')).toBeInTheDocument()
    expect(screen.getByText('5分钟')).toBeInTheDocument()
    expect(screen.getByText('15分钟')).toBeInTheDocument()
    expect(screen.getByText('上传')).toBeInTheDocument()
    expect(screen.getByText('下载')).toBeInTheDocument()
  })

  it('高负载（>80%）显示异常色进度条', () => {
    render(<SystemInfoCard systemInfo={snap({ cpuUsage: 95, memUsagePercent: 90, diskUsagePercent: 85 })} />)
    const exceptions = document.querySelectorAll('.ant-progress-status-exception')
    expect(exceptions.length).toBe(3)
  })
})
