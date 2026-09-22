import { describe, expect, it } from 'vitest'
import {
  DEFAULT_COMPOSE_TEMPLATE,
  STACK_NAME_PATTERN,
  STACK_TEMPLATES,
  extractApiError,
  formatTimestamp,
  lastStatusColor,
  serviceStateColor,
  serviceStatusColor,
  tailOptions,
  taskActionLabel,
} from './shared'

// Stacks/shared：extractApiError 全分支、时间戳/配色/模板常量

// 模拟 axios 错误（axios.isAxiosError 依据 isAxiosError 标志）
const axiosErr = (data?: unknown, message = 'Request failed') =>
  Object.assign(new Error(message), {
    isAxiosError: true,
    ...(data === undefined ? {} : { response: { status: 500, data } }),
  })

describe('Stacks/shared', () => {
  it('tailOptions / 栈名正则 / 默认模板', () => {
    expect(tailOptions.map((t) => t.value)).toEqual([50, 100, 200, 500, 2000])
    for (const ok of ['a', 'my-blog', '1x_y-z', '0abc']) {
      expect(STACK_NAME_PATTERN.test(ok)).toBe(true)
    }
    for (const bad of ['', 'A', '-lead', '_lead', 'has space', 'my.blog', '上']) {
      expect(STACK_NAME_PATTERN.test(bad)).toBe(false)
    }
    expect(DEFAULT_COMPOSE_TEMPLATE).toContain('nginx')
    expect(DEFAULT_COMPOSE_TEMPLATE).toContain('services:')
  })

  it('模板库：5 个常见服务模板均有 compose 内容', () => {
    expect(STACK_TEMPLATES.map((t) => t.key)).toEqual([
      'empty', 'nginx', 'wordpress', 'npm', 'uptime-kuma',
    ])
    for (const t of STACK_TEMPLATES) {
      expect(t.label.length).toBeGreaterThan(0)
      expect(t.compose.length).toBeGreaterThan(0)
    }
  })

  it('serviceStateColor / taskActionLabel 映射表', () => {
    expect(serviceStateColor.running).toBe('green')
    expect(serviceStateColor.dead).toBe('red')
    expect(taskActionLabel.up).toBe('启动')
    expect(taskActionLabel.delete).toBe('删除')
  })

  describe('extractApiError', () => {
    it('非 axios 的 Error 用其 message；无 message 回退默认', () => {
      expect(extractApiError(new Error('boom'))).toBe('boom')
      expect(extractApiError(new Error(''))).toBe('请求失败')
      expect(extractApiError(new Error(''), '自定义回退')).toBe('自定义回退')
      expect(extractApiError(null)).toBe('请求失败')
    })

    it('axios 错误：字符串响应体直接返回（多行 YAML 错误输出）', () => {
      expect(extractApiError(axiosErr('yaml: line 3: bad'))).toBe('yaml: line 3: bad')
    })

    it('axios 错误：空白/空字符串响应体走 message 兜底', () => {
      expect(extractApiError(axiosErr('  ', 'net down'))).toBe('net down')
      expect(extractApiError(axiosErr('', 'net down'))).toBe('net down')
    })

    it('axios 错误：对象响应体取 error / message 字段', () => {
      expect(extractApiError(axiosErr({ error: 'e-msg' }))).toBe('e-msg')
      expect(extractApiError(axiosErr({ message: 'm-msg' }))).toBe('m-msg')
      expect(extractApiError(axiosErr({ error: 'e', message: 'm' }))).toBe('e')
    })

    it('axios 错误：对象响应体字段为空/非字符串时走 message 兜底', () => {
      expect(extractApiError(axiosErr({ error: '' }, 'net down'))).toBe('net down')
      expect(extractApiError(axiosErr({ error: 123 }, 'net down'))).toBe('net down')
      expect(extractApiError(axiosErr({ error: null, message: null }, 'net down'))).toBe('net down')
      expect(extractApiError(axiosErr({}, 'net down'))).toBe('net down')
      expect(extractApiError(axiosErr({ error: '   ' }, 'net down'))).toBe('net down')
    })

    it('axios 错误：无响应体取 err.message；message 为空再回退默认', () => {
      expect(extractApiError(axiosErr(undefined, 'timeout'))).toBe('timeout')
      const noMsg = Object.assign(new Error(''), { isAxiosError: true })
      expect(extractApiError(noMsg)).toBe('请求失败')
    })
  })

  describe('formatTimestamp', () => {
    it('0/-/空值输出 -；有效秒级时间戳格式化', () => {
      expect(formatTimestamp(0)).toBe('-')
      expect(formatTimestamp(1759000000)).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/)
    })
  })

  describe('serviceStatusColor', () => {
    it('全运行绿 / 部分橙 / 全停灰', () => {
      expect(serviceStatusColor(2, 2)).toBe('success')
      expect(serviceStatusColor(3, 2)).toBe('success')
      expect(serviceStatusColor(1, 3)).toBe('warning')
      expect(serviceStatusColor(5, 0)).toBe('warning')
      expect(serviceStatusColor(0, 3)).toBe('default')
      expect(serviceStatusColor(0, 0)).toBe('default')
    })
  })

  describe('lastStatusColor', () => {
    it('成功/失败/执行中/未知状态映射', () => {
      expect(lastStatusColor('success')).toBe('success')
      expect(lastStatusColor('ok')).toBe('success')
      expect(lastStatusColor('failed')).toBe('error')
      expect(lastStatusColor('error')).toBe('error')
      expect(lastStatusColor('running')).toBe('processing')
      expect(lastStatusColor('weird')).toBe('default')
    })
  })
})
