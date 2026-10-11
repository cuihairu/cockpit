import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('axios', () => {
  const instance = {
    get: vi.fn().mockResolvedValue({}),
    post: vi.fn().mockResolvedValue({}),
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
import { approveApproval, listApprovals, rejectApproval } from './approvals'

const mockInstance = axios.create() as unknown as Record<'get' | 'post', ReturnType<typeof vi.fn>>

// 模块加载期的 interceptors.use 记录在 vitest mock 环境下不可靠；
// resetModules 重放模块注册可稳定拿到 handler
async function captureInterceptors() {
  vi.resetModules()
  await import('./approvals')
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

describe('approvals 服务', () => {
  beforeEach(() => {
    for (const m of [mockInstance.get, mockInstance.post]) {
      m.mockClear()
      m.mockResolvedValue({})
    }
    localStorage.clear()
  })

  it('listApprovals 解包 approvals 数组，status 过滤走 query', async () => {
    mockInstance.get.mockResolvedValue({ approvals: [{ id: 'ap1', status: 'pending' }] })
    await expect(listApprovals()).resolves.toEqual([{ id: 'ap1', status: 'pending' }])
    expect(mockInstance.get).toHaveBeenCalledWith('', { params: undefined })
    // 空数据也回退空数组
    mockInstance.get.mockResolvedValue({})
    await expect(listApprovals()).resolves.toEqual([])
    // 状态过滤透传
    await listApprovals('pending')
    expect(mockInstance.get).toHaveBeenCalledWith('', { params: { status: 'pending' } })
  })

  it('approveApproval POST approve 路径（重复决策/过期 409 由调用方提示）', async () => {
    mockInstance.post.mockResolvedValue({ id: 'ap1', status: 'approved' })
    const v = await approveApproval('ap1')
    expect(mockInstance.post).toHaveBeenCalledWith('/ap1/approve')
    expect(v.status).toBe('approved')
  })

  it('rejectApproval POST reject 路径', async () => {
    mockInstance.post.mockResolvedValue({ id: 'ap1', status: 'rejected' })
    const v = await rejectApproval('ap1')
    expect(mockInstance.post).toHaveBeenCalledWith('/ap1/reject')
    expect(v.status).toBe('rejected')
  })
})

describe('approvals 拦截器', () => {
  it('请求拦截器注入 Bearer，无 token 不注入', async () => {
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
    localStorage.setItem('username', 'u')
    await expect(onResErr({ response: { status: 401 } })).rejects.toMatchObject({ response: { status: 401 } })
    expect(localStorage.getItem('token')).toBeNull()
    expect(locationStub.href).toBe('/login')

    await expect(onResErr(new Error('net'))).rejects.toThrow('net')
  })
})
