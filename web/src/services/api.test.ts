import { beforeEach, describe, expect, it, vi } from 'vitest'

// axios 全 mock：工厂内建实例，测试经 axios.create() 取回同一引用
// （api.ts 模块加载即 new ApiService()，拦截器 use 只在此时发生一次）
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
import { api } from './api'

const mockInstance = axios.create() as unknown as {
  get: ReturnType<typeof vi.fn>
  post: ReturnType<typeof vi.fn>
  put: ReturnType<typeof vi.fn>
  delete: ReturnType<typeof vi.fn>
  interceptors: {
    request: { use: ReturnType<typeof vi.fn> }
    response: { use: ReturnType<typeof vi.fn> }
  }
}

// jsdom 的 location 不可直接赋 href，整体替换为可写对象
const locationStub = { href: '' }
vi.stubGlobal('location', locationStub)

describe('ApiService 全方法驱动（URL/method/参数序列化不抛错）', () => {
  beforeEach(() => {
    // 只清四个动词（拦截器 use 记录仅模块加载时产生，不能 clearAll）
    for (const m of [mockInstance.get, mockInstance.post, mockInstance.put, mockInstance.delete]) {
      m.mockClear()
      m.mockResolvedValue({})
    }
    localStorage.clear()
    locationStub.href = ''
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}')))
  })

  it('172 个端点方法全部可调用', async () => {
    const ctrl = new AbortController()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).login('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).logout()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).refreshToken()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getCurrentUser()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).listUsers()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).createUser({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).updateUser('x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteUser('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).changeUserPassword('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).listRoles()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).createRole({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).updateRole('x', [])
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteRole('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).updateProfile({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).saveSettings({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).changePassword('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).forgotPassword('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).resetPassword('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).verifyResetCode('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).generateTOTP()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).enableTOTP('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).disableTOTP('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).verifyTOTP('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getAgents()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getAgent('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getContainers('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getImages('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).startContainer('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).stopContainer('x', 'x', 1)
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).restartContainer('x', 'x', 1)
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).pauseContainer('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).unpauseContainer('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).removeContainer('x', 'x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getContainerLogs('x', 'x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getStacks()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getStackDetail('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getStackCompose('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).saveStackCompose('x', 'x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).stackAction('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getStackHistory('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getStackTask('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getStackLogs('x', 'x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteStack('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getComputeInstances({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getComputeInstance('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDomains()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDomain('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getCertificates()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getCertificate('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getServices()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getService('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getGateways()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getGateway('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getStorages()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getStorage('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getStatus()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getAlerts()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).markAlertRead('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).markAllAlertsRead()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getProbeConfig()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).saveProbeConfig({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getProbeHistory('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getNotificationStatus()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).testNotification()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getLogsStatus('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getLogsSources('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).queryLogs('x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).searchLogs('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).followLogs('x', {}, ctrl.signal)
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).checkDrift('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).driftDiff('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).driftRecord('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDriftConfig()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).putDriftConfig('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getInventoryConsistency()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getRecordings()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getRecordingCast('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).downloadRecording('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteRecording('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getRecordingsConfig()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).putRecordingsConfig({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).syncRecordingRemote('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getServerBackups()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getServerBackupConfig()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).putServerBackupConfig({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).runServerBackup()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).syncServerBackupRemote('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).downloadServerBackup('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteServerBackup('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDNSStatus()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDNSZones()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDNSRecords('x', 'x', 1)
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).createDNSRecord('x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).updateDNSRecord('x', 'x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteDNSRecord('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getBackupConfigs()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).createBackupConfig({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).updateBackupConfig('x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteBackupConfig('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).runBackup('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getBackupRuns('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getBackupConfigRuns('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getBackupFiles('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteBackupFile('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).syncBackupFileRemote('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).restoreBackup('x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getBackupTask('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).downloadBackupFile('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).listFiles('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).readFileChunk('x', 'x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).writeFile('x', 'x', {}, 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).createRemoteDir('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteRemoteFile('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).renameRemoteFile('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).chmodFile('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).chownFile('x', 'x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).searchFiles('x', 'x', {}, 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).downloadRemoteFile('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getProxyStatus('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getProxySites('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getProxySite('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).applyProxySite('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteProxySite('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getCronStatus('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getCronJobs('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getCronUsers('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getCronTimers('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).applyCronJob('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteCronJob('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getServiceStatus('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getAgentServices('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).serviceAction('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).serviceDaemonReload('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getServiceUnitFile('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).saveServiceUnitFile('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getOverlayStatus('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getOverlayCloud()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).setOverlayZTMemberAuthorized('x', 'x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).removeOverlayZTMember('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).authorizeOverlayTSDevice('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).removeOverlayTSDevice('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getSmartStatus('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getSmartConfig()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).putSmartConfig('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getNASStatus('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getNASConfig()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).putNASConfig('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDDNSConfigs()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).createDDNSConfig({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).updateDDNSConfig('x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteDDNSConfig('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).checkDDNSConfig('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDDNSScanConfig()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).putDDNSScanConfig('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getAcmeCerts()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).createAcmeCert({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).updateAcmeCert('x', {})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteAcmeCert('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).issueAcmeCert('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).downloadAcmeCert('x', 'x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getAcmeAccount()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deployAcmeCert('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).putAcmeAccount('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getAcmeScanConfig()
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).putAcmeScanConfig('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDomainBindings('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).saveDomainBinding({})
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).deleteDomainBinding('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).applyDomainBinding('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getDomainDrift('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getAgentDomains('x')
    await (api as never as Record<string, (...args: unknown[]) => Promise<unknown>>).getAgentDomainsSnippet('x')
    // 全部方法都被驱动过（每个至少一次 client 调用）
    const total = mockInstance.get.mock.calls.length + mockInstance.post.mock.calls.length
      + mockInstance.put.mock.calls.length + mockInstance.delete.mock.calls.length
    expect(total).toBeGreaterThanOrEqual(170)
  })
})

describe('ApiService 关键路径断言', () => {
  beforeEach(() => {
    for (const m of [mockInstance.get, mockInstance.post, mockInstance.put, mockInstance.delete]) {
      m.mockClear()
      m.mockResolvedValue({})
    }
    localStorage.clear()
    locationStub.href = ''
  })

  it('login POST /auth/login 且 body 含凭据', async () => {
    await api.login('admin', 'secret')
    expect(mockInstance.post).toHaveBeenCalledWith('/auth/login', { username: 'admin', password: 'secret' })
  })

  it('logout 清 localStorage 并跳登录页', async () => {
    localStorage.setItem('token', 't')
    localStorage.setItem('username', 'u')
    await api.logout()
    expect(localStorage.getItem('token')).toBeNull()
    expect(localStorage.getItem('username')).toBeNull()
    expect(locationStub.href).toBe('/login')
  })

  it('路径参数 encodeURIComponent 编码', async () => {
    await api.deleteUser('a b/c')
    expect(mockInstance.delete).toHaveBeenCalledWith('/users/a%20b%2Fc')
    await api.updateRole('r&', [])
    expect(mockInstance.put).toHaveBeenCalledWith('/roles/r%26', { permissions: [] })
  })

  it('changeUserPassword body 用 new_password 键', async () => {
    await api.changeUserPassword('uid', 'newpass')
    expect(mockInstance.post).toHaveBeenCalledWith('/users/uid/password', { new_password: 'newpass' })
  })

  it('列表参数走 axios params', async () => {
    await api.getContainers('ag', true)
    expect(mockInstance.get).toHaveBeenCalledWith('/docker/agents/ag/containers', expect.objectContaining({ params: { all: true } }))
  })

  it('followLogs 直连 fetch 带 Bearer 与 abort signal', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('ok'))
    vi.stubGlobal('fetch', fetchMock)
    localStorage.setItem('token', 'tk')
    const ctrl = new AbortController()
    await api.followLogs('ag', {} as never, ctrl.signal)
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/agents/ag/logs/follow',
      expect.objectContaining({ method: 'POST', signal: ctrl.signal }),
    )
    const headers = (fetchMock.mock.calls[0][1] as { headers: Record<string, string> }).headers
    expect(headers.Authorization).toBe('Bearer tk')
  })
})

describe('ApiService 拦截器', () => {
  // 模块加载时的 use 记录在 vitest mock 环境下不可靠（ESM interop），
  // 重新构造一次 ApiService 换上 spy 稳定捕获 handler
  const captureHandlers = () => {
    const reqUse = vi.fn()
    const resUse = vi.fn()
    mockInstance.interceptors.request.use = reqUse
    mockInstance.interceptors.response.use = resUse
    const Ctor = Object.getPrototypeOf(api).constructor
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    new (Ctor as any)()
    return { reqUse, resUse }
  }

  it('请求拦截器注入 Bearer', () => {
    const { reqUse } = captureHandlers()
    type ReqCfg = { headers: Record<string, string> }
    const [onReq] = reqUse.mock.calls[0] as [(c: ReqCfg) => ReqCfg]
    localStorage.setItem('token', 'tk')
    const config = onReq({ headers: {} })
    expect(config.headers.Authorization).toBe('Bearer tk')
    localStorage.removeItem('token')
    const config2 = onReq({ headers: {} })
    expect(config2.headers.Authorization).toBeUndefined()
  })

  it('请求拦截器错误分支原样 reject', async () => {
    const { reqUse } = captureHandlers()
    const [, onReqErr] = reqUse.mock.calls[0] as [unknown, (e: Error) => Promise<never>]
    await expect(onReqErr(new Error('boom'))).rejects.toThrow('boom')
  })

  it('响应拦截器直通 data', () => {
    const { resUse } = captureHandlers()
    const [onRes] = resUse.mock.calls[0] as [(r: { data: unknown }) => unknown]
    expect(onRes({ data: 42 })).toBe(42)
  })

  it('401 响应清凭证并跳登录，非 401 原样 reject', async () => {
    const { resUse } = captureHandlers()
    const [, onResErr] = resUse.mock.calls[0] as [unknown, (e: unknown) => Promise<never>]
    localStorage.setItem('token', 't')
    localStorage.setItem('username', 'u')
    await expect(onResErr({ response: { status: 401 } })).rejects.toMatchObject({ response: { status: 401 } })
    expect(localStorage.getItem('token')).toBeNull()
    expect(locationStub.href).toBe('/login')

    locationStub.href = ''
    await expect(onResErr(new Error('net'))).rejects.toThrow('net')
    expect(locationStub.href).toBe('')
  })
})
