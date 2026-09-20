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
  createRemoteSession,
  createRemoteTicket,
  deleteRemoteSession,
  getRemoteSession,
  getRemoteSessions,
} from './remote'

const mockInstance = axios.create() as unknown as Record<'get' | 'post' | 'put' | 'delete', ReturnType<typeof vi.fn>>

// 模块加载期的 interceptors.use 记录在 vitest mock 环境下不可靠；
// resetModules 重放模块注册可稳定拿到 handler
async function captureInterceptors() {
  vi.resetModules()
  await import('./remote')
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

describe('remote 服务', () => {
  beforeEach(() => {
    for (const m of [mockInstance.get, mockInstance.post, mockInstance.put, mockInstance.delete]) {
      m.mockClear()
      m.mockResolvedValue({})
    }
    localStorage.clear()
  })

  it('createRemoteTicket 转蛇形 body 并映射 expires_at', async () => {
    mockInstance.post.mockResolvedValue({ ticket: 't1', expires_at: '2026-01-01T00:00:00Z' })
    const ticket = await createRemoteTicket({
      agentId: 'ag', host: 'h', port: 22, protocol: 'ssh', username: 'u', password: 'p',
    })
    expect(mockInstance.post).toHaveBeenCalledWith('/tickets', {
      agent_id: 'ag', host: 'h', port: 22, protocol: 'ssh',
      username: 'u', password: 'p', domain: undefined, width: undefined, height: undefined,
    })
    expect(ticket).toEqual({ ticket: 't1', expiresAt: '2026-01-01T00:00:00Z' })
  })

  it('sessions CRUD', async () => {
    mockInstance.get.mockResolvedValue({ data: [{ id: 's1' }] })
    await expect(getRemoteSessions()).resolves.toEqual([{ id: 's1' }])
    expect(mockInstance.get).toHaveBeenCalledWith('/sessions')

    await createRemoteSession({ agentId: 'ag', protocol: 'vnc', host: 'h', port: 5900 })
    expect(mockInstance.post).toHaveBeenCalledWith('/sessions', { agentId: 'ag', protocol: 'vnc', host: 'h', port: 5900 })

    await getRemoteSession('s1')
    expect(mockInstance.get).toHaveBeenCalledWith('/sessions/s1')

    await deleteRemoteSession('s1')
    expect(mockInstance.delete).toHaveBeenCalledWith('/sessions/s1')
  })
})

describe('remote 拦截器', () => {
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
