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
import type { WorkflowDef } from './workflows'
import {
  cancelWorkflowRun,
  createWorkflow,
  deleteWorkflow,
  getWorkflow,
  getWorkflowRun,
  isRunTerminal,
  isStepTerminal,
  listWorkflowRuns,
  listWorkflows,
  runWorkflow,
  updateWorkflow,
} from './workflows'

const mockInstance = axios.create() as unknown as Record<
  'get' | 'post' | 'put' | 'delete',
  ReturnType<typeof vi.fn>
>

const def: WorkflowDef = {
  id: 'wf-1',
  name: 'upgrade chain',
  steps: [{ type: 'agent.exec', target: 'a1', parameters: { name: 's1', command: 'uptime' } }],
  createdBy: 'cui',
  createdAt: '2026-10-09T00:00:00Z',
  updatedAt: '2026-10-09T00:00:00Z',
}

describe('workflows 服务', () => {
  beforeEach(() => {
    for (const m of [mockInstance.get, mockInstance.post, mockInstance.put, mockInstance.delete]) {
      m.mockClear()
      m.mockResolvedValue({})
    }
    localStorage.clear()
  })

  it('listWorkflows 解包 workflows 数组', async () => {
    mockInstance.get.mockResolvedValue({ workflows: [def] })
    await expect(listWorkflows()).resolves.toEqual([def])
    expect(mockInstance.get).toHaveBeenCalledWith('/workflows')
    mockInstance.get.mockResolvedValue({})
    await expect(listWorkflows()).resolves.toEqual([])
  })

  it('getWorkflow 按 id 拼路径', async () => {
    await getWorkflow('wf-1')
    expect(mockInstance.get).toHaveBeenCalledWith('/workflows/wf-1')
  })

  it('createWorkflow POST 定义体', async () => {
    mockInstance.post.mockResolvedValue(def)
    const payload = { name: 'upgrade chain', steps: def.steps }
    const out = await createWorkflow(payload)
    expect(mockInstance.post).toHaveBeenCalledWith('/workflows', payload)
    expect(out).toEqual(def)
  })

  it('updateWorkflow PUT 定义体', async () => {
    mockInstance.put.mockResolvedValue(def)
    const payload = { name: 'renamed', steps: def.steps }
    await updateWorkflow('wf-1', payload)
    expect(mockInstance.put).toHaveBeenCalledWith('/workflows/wf-1', payload)
  })

  it('deleteWorkflow DELETE 路径', async () => {
    await deleteWorkflow('wf-1')
    expect(mockInstance.delete).toHaveBeenCalledWith('/workflows/wf-1')
  })

  it('runWorkflow POST run 路径返回 run 视图', async () => {
    const run = { id: 'r-1', status: 'running', steps: [] }
    mockInstance.post.mockResolvedValue(run)
    await expect(runWorkflow('wf-1')).resolves.toEqual(run)
    expect(mockInstance.post).toHaveBeenCalledWith('/workflows/wf-1/run')
  })

  it('listWorkflowRuns 解包 runs 数组并透传过滤', async () => {
    mockInstance.get.mockResolvedValue({ runs: [{ id: 'r-1' }] })
    await expect(listWorkflowRuns('wf-1')).resolves.toEqual([{ id: 'r-1' }])
    expect(mockInstance.get).toHaveBeenCalledWith('/workflows/wf-1/runs', { params: undefined })
    await listWorkflowRuns('wf-1', { status: 'failed' })
    expect(mockInstance.get).toHaveBeenCalledWith('/workflows/wf-1/runs', {
      params: { status: 'failed' },
    })
    // runs 字段缺失回退空数组
    mockInstance.get.mockResolvedValue({ runs: null })
    await expect(listWorkflowRuns('wf-1')).resolves.toEqual([])
  })

  it('getWorkflowRun / cancelWorkflowRun 走 workflow-runs 面', async () => {
    await getWorkflowRun('r-1')
    expect(mockInstance.get).toHaveBeenCalledWith('/workflow-runs/r-1')
    mockInstance.post.mockResolvedValue({ id: 'r-1', status: 'cancelled' })
    const run = await cancelWorkflowRun('r-1')
    expect(mockInstance.post).toHaveBeenCalledWith('/workflow-runs/r-1/cancel')
    expect(run.status).toBe('cancelled')
  })

  it('isRunTerminal / isStepTerminal 判定终态', () => {
    expect(isRunTerminal('running')).toBe(false)
    expect(isRunTerminal('success')).toBe(true)
    expect(isRunTerminal('failed')).toBe(true)
    expect(isRunTerminal('cancelled')).toBe(true)
    expect(isStepTerminal('pending')).toBe(false)
    expect(isStepTerminal('running')).toBe(false)
    expect(isStepTerminal('success')).toBe(true)
    expect(isStepTerminal('failed')).toBe(true)
  })
})

// 模块加载期的 interceptors.use 记录在 vitest mock 环境下不可靠；
// resetModules 重放模块注册可稳定拿到 handler（同 jobs.test.ts 口径）
async function captureInterceptors() {
  vi.resetModules()
  await import('./workflows')
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

describe('workflows 拦截器', () => {
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
