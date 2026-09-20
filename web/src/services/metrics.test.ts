import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('axios', () => {
  const instance = {
    get: vi.fn().mockResolvedValue({}),
    post: vi.fn().mockResolvedValue({}),
    put: vi.fn().mockResolvedValue({}),
    delete: vi.fn().mockResolvedValue({}),
    interceptors: {
      request: { use: vi.fn() },
      response: { use: vi.fn() },
    },
  }
  return { default: { create: vi.fn(() => instance) } }
})

import axios from 'axios'

// jsdom 的 location 不可直接赋 href，整体替换为可写对象
const locationStub = { href: '' }
vi.stubGlobal('location', locationStub)
import {
  getMetricsHistory,
  getSystemSnapshot,
  getSystemSnapshots,
} from './metrics'

const mockGet = (axios.create() as unknown as { get: ReturnType<typeof vi.fn> }).get

// 模块加载期的 interceptors.use 记录不可靠；resetModules 重放注册取 handler
async function captureInterceptors() {
  vi.resetModules()
  await import('./metrics')
  const ax = (await import('axios')).default as unknown as {
    create: () => {
      interceptors: {
        request: { use: { mock: { calls: unknown[][] } } }
        response: { use: { mock: { calls: unknown[][] } } }
      }
    }
  }
  return ax.create().interceptors
}

describe('metrics 服务', () => {
  beforeEach(() => {
    mockGet.mockClear()
    mockGet.mockResolvedValue({})
  })

  it('快照列表：data 缺失回退空数组', async () => {
    mockGet.mockResolvedValue({})
    await expect(getSystemSnapshots()).resolves.toEqual([])
    expect(mockGet).toHaveBeenCalledWith('/metrics/snapshots')
  })

  it('单快照按 agent_id 查询', async () => {
    await getSystemSnapshot('ag1')
    expect(mockGet).toHaveBeenCalledWith('/metrics/snapshot?agent_id=ag1')
  })

  it('历史指标：缺字段回退默认值', async () => {
    mockGet.mockResolvedValue({})
    const r = await getMetricsHistory('ag1', { limit: 10 })
    expect(mockGet).toHaveBeenCalledWith('/metrics/history?agent_id=ag1', { params: { limit: 10 } })
    expect(r).toEqual({ data: [], start: 0, end: 0, count: 0 })
  })
})


describe('metrics 拦截器', () => {
  it('请求拦截器注入 Bearer', async () => {
    const { request } = await captureInterceptors()
    type ReqCfg = { headers: Record<string, string> }
    const [onReq] = request.use.mock.calls[0] as [(c: ReqCfg) => ReqCfg]
    localStorage.setItem('token', 'tk')
    expect(onReq({ headers: {} }).headers.Authorization).toBe('Bearer tk')
    localStorage.removeItem('token')
    expect(onReq({ headers: {} }).headers.Authorization).toBeUndefined()
  })

  it('请求拦截器错误分支原样 reject', async () => {
    const { request } = await captureInterceptors()
    const [, onErr] = request.use.mock.calls[0] as [unknown, (e: Error) => Promise<never>]
    await expect(onErr(new Error('boom'))).rejects.toThrow('boom')
  })

  it('响应拦截器直通 data；401 清凭证跳登录；非 401 原样 reject', async () => {
    const { response } = await captureInterceptors()
    const [onRes, onResErr] = response.use.mock.calls[0] as [
      (r: { data: unknown }) => unknown,
      (e: unknown) => Promise<never>,
    ]
    expect(onRes({ data: 'x' })).toBe('x')

    localStorage.setItem('token', 't')
    await expect(onResErr({ response: { status: 401 } })).rejects.toMatchObject({ response: { status: 401 } })
    expect(localStorage.getItem('token')).toBeNull()
    expect(locationStub.href).toBe('/login')

    await expect(onResErr(new Error('net'))).rejects.toThrow('net')
  })
})
