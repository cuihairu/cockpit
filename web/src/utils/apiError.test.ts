import { describe, expect, it } from 'vitest'
import { getApiErrorMessage } from './apiError'

describe('getApiErrorMessage', () => {
  it('非对象/ null 直接回退 fallback', () => {
    expect(getApiErrorMessage(undefined, '操作失败')).toBe('操作失败')
    expect(getApiErrorMessage('boom', '操作失败')).toBe('操作失败')
    expect(getApiErrorMessage(null, '操作失败')).toBe('操作失败')
  })

  it('无 response 结构回退', () => {
    expect(getApiErrorMessage(new Error('network'), '操作失败')).toBe('操作失败')
    expect(getApiErrorMessage({}, '操作失败')).toBe('操作失败')
    expect(getApiErrorMessage({ response: null }, '操作失败')).toBe('操作失败')
    expect(getApiErrorMessage({ response: 'string' }, '操作失败')).toBe('操作失败')
  })

  it('response.data 无 error 字段回退', () => {
    expect(getApiErrorMessage({ response: {} }, '操作失败')).toBe('操作失败')
    expect(getApiErrorMessage({ response: { data: null } }, '操作失败')).toBe('操作失败')
    expect(getApiErrorMessage({ response: { data: 'oops' } }, '操作失败')).toBe('操作失败')
    expect(getApiErrorMessage({ response: { data: { other: 1 } } }, '操作失败')).toBe('操作失败')
  })

  it('error 字段非字符串回退', () => {
    expect(getApiErrorMessage({ response: { data: { error: 42 } } }, '操作失败')).toBe('操作失败')
  })

  it('提取后端 error 文案', () => {
    const axiosLike = { response: { data: { error: '角色不存在' } } }
    expect(getApiErrorMessage(axiosLike, '操作失败')).toBe('角色不存在')
  })
})
