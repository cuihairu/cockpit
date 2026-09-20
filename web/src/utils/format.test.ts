import { describe, expect, it } from 'vitest'
import { formatBytes, formatDuration, formatPercent, formatUptime } from './format'

describe('format', () => {
  it('formatBytes 量级与小数位（B 整数、KB/MB 一位、GB/TB 两位）', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(500)).toBe('500 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1536)).toBe('1.5 KB')
    expect(formatBytes(3 * 1024 * 1024)).toBe('3.0 MB')
    expect(formatBytes(4 * 1024 ** 3)).toBe('4.00 GB')
    expect(formatBytes(1024 ** 4)).toBe('1.00 TB')
  })

  it('formatDuration 紧凑档位（秒/分秒/时分，小时档不带秒）', () => {
    expect(formatDuration(35000)).toBe('35s')
    expect(formatDuration((12 * 60 + 5) * 1000)).toBe('12m5s')
    expect(formatDuration((90 * 60) * 1000)).toBe('1h30m')
  })

  it('formatUptime 中文档位', () => {
    expect(formatUptime(5 * 60)).toBe('5分钟')
    expect(formatUptime(5 * 3600 + 12 * 60)).toBe('5小时 12分钟')
    expect(formatUptime(3 * 86400 + 4 * 3600)).toBe('3天 4小时')
  })

  it('formatPercent 保留一位小数', () => {
    expect(formatPercent(12.34)).toBe('12.3%')
    expect(formatPercent(0)).toBe('0.0%')
  })
})
