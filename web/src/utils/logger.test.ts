import { afterEach, describe, expect, it, vi } from 'vitest'
import { logger } from './logger'

// 测试进程 MODE=test（非 development）：info/warn/debug 静默、error 恒输出。

afterEach(() => {
  vi.restoreAllMocks()
})

describe('logger', () => {
  it('error 恒记录并带时间戳与级别前缀', () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    logger.error('boom', { code: 1 })
    expect(err).toHaveBeenCalledTimes(1)
    const msg = err.mock.calls[0][0] as string
    expect(msg).toMatch(/^\[.*\] \[ERROR\] boom {"code":1}$/)
  })

  it('对象参数 JSON 序列化，循环引用回退 String', () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    logger.error({ a: 1 })
    expect(err.mock.calls[0][0]).toMatch(/\[ERROR\] {"a":1}$/)
    const cyc: Record<string, unknown> = {}
    cyc.self = cyc
    logger.error(cyc)
    expect(err.mock.calls[1][0]).toMatch(/\[ERROR\] \[object Object\]$/)
  })

  it('非 development 下 info/warn/debug 静默', () => {
    const info = vi.spyOn(console, 'info').mockImplementation(() => {})
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const log = vi.spyOn(console, 'log').mockImplementation(() => {})
    logger.info('i')
    logger.warn('w')
    logger.debug('d')
    expect(info).not.toHaveBeenCalled()
    expect(warn).not.toHaveBeenCalled()
    expect(log).not.toHaveBeenCalled()
  })
})
